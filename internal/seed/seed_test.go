package seed

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/auth"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/database"
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

func countRows(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), query).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}

func TestDemoDataAddsEverythingTheSiteNeeds(t *testing.T) {
	db := openMigratedDatabase(t)

	if err := DemoData(t.Context(), db, testPasswords); err != nil {
		t.Fatalf("seed: %v", err)
	}

	wantRows := map[string]int{
		"SELECT count(*) FROM teams":                                2,
		"SELECT count(*) FROM environments":                         4,
		"SELECT count(*) FROM environments WHERE is_production = 1": 1,
		"SELECT count(*) FROM event_types":                          4,
		"SELECT count(*) FROM users":                                2,
	}
	for query, want := range wantRows {
		if got := countRows(t, db, query); got != want {
			t.Errorf("%s = %d, want %d", query, got, want)
		}
	}
}

func TestDemoDataAccounts(t *testing.T) {
	db := openMigratedDatabase(t)
	if err := DemoData(t.Context(), db, testPasswords); err != nil {
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
	if err := DemoData(t.Context(), db, testPasswords); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE users SET display_name = 'Renamed' WHERE email = ?", AdminEmail); err != nil {
		t.Fatalf("rename admin: %v", err)
	}

	if err := DemoData(t.Context(), db, Passwords{Admin: "a-different-admin-password", Member: "a-different-member-password"}); err != nil {
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

	if err := DemoData(t.Context(), db, testPasswords); err == nil {
		t.Fatal("expected an error when the users table is missing, got nil")
	}

	if got := countRows(t, db, "SELECT count(*) FROM teams"); got != 0 {
		t.Errorf("teams has %d rows after a failed seed, want 0 because the transaction is rolled back", got)
	}
}
