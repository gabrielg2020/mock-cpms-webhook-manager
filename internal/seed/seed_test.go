package seed

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/auth"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/database"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/secretbox"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/store"
)

var testPasswords = Passwords{Admin: "admin-password-for-tests", Member: "member-password-for-tests"}

func openMigratedDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// newTestSecrets returns a box whose key is the given byte repeated.
func newTestSecrets(t *testing.T, keyByte byte) *secretbox.Box {
	t.Helper()
	box, err := secretbox.New(bytes.Repeat([]byte{keyByte}, secretbox.KeySize))
	if err != nil {
		t.Fatalf("create secret box: %v", err)
	}
	return box
}

func countRows(t *testing.T, db *sql.DB, query string, arguments ...any) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), query, arguments...).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func TestDemoDataAddsEverythingTheSiteNeeds(t *testing.T) {
	db := openMigratedDatabase(t)

	if err := DemoData(t.Context(), db, testPasswords, newTestSecrets(t, 1)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	wantRows := map[string]int{
		"SELECT count(*) FROM teams":                                2,
		"SELECT count(*) FROM environments":                         4,
		"SELECT count(*) FROM environments WHERE is_production = 1": 1,
		"SELECT count(*) FROM event_types":                          4,
		"SELECT count(*) FROM users":                                2,
		"SELECT count(*) FROM webhooks":                             5,
		"SELECT count(*) FROM webhooks WHERE active = 0":            1,
		"SELECT count(*) FROM webhook_events":                       13,
	}
	for query, want := range wantRows {
		if got := countRows(t, db, query); got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
}

func TestDemoDataAccounts(t *testing.T) {
	db := openMigratedDatabase(t)
	if err := DemoData(t.Context(), db, testPasswords, newTestSecrets(t, 1)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	queries := store.New(db)

	admin, err := queries.GetUserByEmail(t.Context(), AdminEmail)
	if err != nil {
		t.Fatalf("find admin: %v", err)
	}
	if admin.Role != "admin" || admin.TeamID.Valid {
		t.Errorf("admin has role %q and team %v, want role admin and no team", admin.Role, admin.TeamID)
	}

	member, err := queries.GetUserByEmail(t.Context(), "MEMBER@example.com")
	if err != nil {
		t.Fatalf("find member by an email in another case: %v", err)
	}
	team, err := queries.GetTeamByName(t.Context(), memberTeam)
	if err != nil {
		t.Fatalf("find team: %v", err)
	}
	if member.Role != "member" || member.TeamID.Int64 != team.ID {
		t.Errorf("member has role %q and team %v, want role member in team %d", member.Role, member.TeamID, team.ID)
	}

	testCases := []struct {
		name     string
		hash     string
		password string
		want     bool
	}{
		{name: "admin with the admin password", hash: admin.PasswordHash, password: testPasswords.Admin, want: true},
		{name: "member with the member password", hash: member.PasswordHash, password: testPasswords.Member, want: true},
		{name: "admin with the member password", hash: admin.PasswordHash, password: testPasswords.Member, want: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := auth.VerifyPassword(testCase.password, testCase.hash)
			if err != nil {
				t.Fatalf("verify: %v", err)
			}
			if got != testCase.want {
				t.Errorf("password matches = %t, want %t", got, testCase.want)
			}
		})
	}
}

func TestDemoDataLeavesExistingRowsAlone(t *testing.T) {
	db := openMigratedDatabase(t)
	if err := DemoData(t.Context(), db, testPasswords, newTestSecrets(t, 1)); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE users SET display_name = 'Renamed' WHERE email = ?", AdminEmail); err != nil {
		t.Fatalf("rename admin: %v", err)
	}

	if err := DemoData(t.Context(), db, Passwords{Admin: "a-different-admin-password", Member: "a-different-member-password"}, newTestSecrets(t, 1)); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	if got := countRows(t, db, "SELECT count(*) FROM users"); got != 2 {
		t.Errorf("users has %d rows after seeding twice, want 2", got)
	}
	admin, err := store.New(db).GetUserByEmail(t.Context(), AdminEmail)
	if err != nil {
		t.Fatalf("find admin: %v", err)
	}
	if admin.DisplayName != "Renamed" {
		t.Errorf("display name = %q, want the second seed to leave it as Renamed", admin.DisplayName)
	}
	if matches, _ := auth.VerifyPassword(testPasswords.Admin, admin.PasswordHash); !matches {
		t.Error("the second seed changed an existing account's password")
	}
}

func TestDemoDataAddsNothingWhenItFails(t *testing.T) {
	db := openMigratedDatabase(t)
	if _, err := db.ExecContext(t.Context(), "DROP TABLE webhook_events; DROP TABLE webhooks; DROP TABLE audit_log; DROP TABLE users"); err != nil {
		t.Fatalf("remove the users table: %v", err)
	}

	if err := DemoData(t.Context(), db, testPasswords, newTestSecrets(t, 1)); err == nil {
		t.Fatal("expected an error when the users table is missing, got nil")
	}

	if got := countRows(t, db, "SELECT count(*) FROM teams"); got != 0 {
		t.Errorf("teams has %d rows after a failed seed, want 0 because the transaction is rolled back", got)
	}
}

type storedWebhook struct {
	id         int64
	url        string
	active     int64
	ciphertext []byte
	lastFour   string
}

func findWebhook(t *testing.T, db *sql.DB, team, environment string) storedWebhook {
	t.Helper()
	const query = `
		SELECT webhooks.id, webhooks.url, webhooks.active, webhooks.secret_ciphertext, webhooks.secret_last4
		FROM webhooks
		JOIN teams ON teams.id = webhooks.team_id
		JOIN environments ON environments.id = webhooks.environment_id
		WHERE teams.name = ? AND environments.name = ?`
	var found storedWebhook
	row := db.QueryRowContext(t.Context(), query, team, environment)
	if err := row.Scan(&found.id, &found.url, &found.active, &found.ciphertext, &found.lastFour); err != nil {
		t.Fatalf("find the %s webhook for %s: %v", team, environment, err)
	}
	return found
}

func TestDemoDataWebhooks(t *testing.T) {
	db := openMigratedDatabase(t)
	secrets := newTestSecrets(t, 1)
	if err := DemoData(t.Context(), db, testPasswords, secrets); err != nil {
		t.Fatalf("seed: %v", err)
	}

	seenSecrets := map[string]bool{}
	for _, demo := range demoWebhooks {
		t.Run(demo.team+" "+demo.environment, func(t *testing.T) {
			found := findWebhook(t, db, demo.team, demo.environment)

			if found.url != demo.url || found.active != demo.active {
				t.Errorf("got url %q and active %d, want %q and %d", found.url, found.active, demo.url, demo.active)
			}
			if !strings.HasPrefix(found.url, "https://") || !strings.Contains(found.url, ".example.com/") {
				t.Errorf("url %q is not an https address under example.com", found.url)
			}

			secret, err := secrets.Open(found.ciphertext)
			if err != nil {
				t.Fatalf("the stored secret does not open with the key it was seeded with: %v", err)
			}
			if len(secret) != 43 || len(found.lastFour) != 4 || !strings.HasSuffix(string(secret), found.lastFour) {
				t.Errorf("secret is %d characters and last four is %q, want 43 characters ending with the last four", len(secret), found.lastFour)
			}
			if seenSecrets[string(secret)] {
				t.Error("two webhooks share a secret")
			}
			seenSecrets[string(secret)] = true

			const subscribedTo = `
				SELECT count(*) FROM webhook_events
				JOIN event_types ON event_types.id = webhook_events.event_type_id
				WHERE webhook_events.webhook_id = ? AND event_types.code = ?`
			for _, code := range demo.events {
				if got := countRows(t, db, subscribedTo, found.id, code); got != 1 {
					t.Errorf("subscribed to %s %d times, want 1", code, got)
				}
			}
			if got := countRows(t, db, "SELECT count(*) FROM webhook_events WHERE webhook_id = ?", found.id); got != len(demo.events) {
				t.Errorf("subscribed to %d events, want %d", got, len(demo.events))
			}
		})
	}

	// The member's team keeps one non-production environment free, so there is somewhere to create a webhook.
	const forEnvironment = `
		SELECT count(*) FROM webhooks
		JOIN teams ON teams.id = webhooks.team_id
		JOIN environments ON environments.id = webhooks.environment_id
		WHERE teams.name = ? AND environments.name = ?`
	if got := countRows(t, db, forEnvironment, memberTeam, "int"); got != 0 {
		t.Errorf("the member's team has %d webhooks for int, want 0", got)
	}
}

func TestDemoDataLeavesExistingWebhooksAlone(t *testing.T) {
	db := openMigratedDatabase(t)
	if err := DemoData(t.Context(), db, testPasswords, newTestSecrets(t, 1)); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	before := findWebhook(t, db, memberTeam, "dev01")
	if _, err := db.ExecContext(t.Context(), "UPDATE webhooks SET url = 'https://changed.example.com/hook', active = 0 WHERE id = ?", before.id); err != nil {
		t.Fatalf("change webhook: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "DELETE FROM webhook_events WHERE webhook_id = ?", before.id); err != nil {
		t.Fatalf("unsubscribe webhook: %v", err)
	}

	// A different key the second time: an existing secret must not be sealed again.
	if err := DemoData(t.Context(), db, testPasswords, newTestSecrets(t, 2)); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	after := findWebhook(t, db, memberTeam, "dev01")
	if !bytes.Equal(before.ciphertext, after.ciphertext) || before.lastFour != after.lastFour {
		t.Error("the second seed replaced an existing webhook's secret")
	}
	if after.url != "https://changed.example.com/hook" || after.active != 0 {
		t.Errorf("got url %q and active %d, want the second seed to leave the changes alone", after.url, after.active)
	}
	if got := countRows(t, db, "SELECT count(*) FROM webhook_events WHERE webhook_id = ?", after.id); got != 0 {
		t.Errorf("the second seed subscribed an existing webhook to %d events, want 0", got)
	}
	if got := countRows(t, db, "SELECT count(*) FROM webhooks"); got != 5 {
		t.Errorf("webhooks has %d rows after seeding twice, want 5", got)
	}
}

func TestSeedWebhookRefusesNamesThatDoNotExist(t *testing.T) {
	testCases := map[string]demoWebhook{
		"an unknown team":        {team: "No Such Team", environment: "int", url: "https://a.example.com/hook", active: 1, events: allEvents},
		"an unknown environment": {team: memberTeam, environment: "nowhere", url: "https://a.example.com/hook", active: 1, events: allEvents},
		"an unknown event type":  {team: memberTeam, environment: "int", url: "https://a.example.com/hook", active: 1, events: []string{"payment.exploded"}},
	}

	for name, demo := range testCases {
		t.Run(name, func(t *testing.T) {
			db := openMigratedDatabase(t)
			secrets := newTestSecrets(t, 1)
			if err := DemoData(t.Context(), db, testPasswords, secrets); err != nil {
				t.Fatalf("seed: %v", err)
			}

			if err := seedWebhook(t.Context(), store.New(db), secrets, demo); err == nil {
				t.Error("expected an error, got nil")
			}
		})
	}
}
