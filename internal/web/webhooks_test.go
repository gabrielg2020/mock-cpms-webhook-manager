package web

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/seed"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/store"
)

const (
	memberTeam = "MOT"
	otherTeam  = "Theory Test"
)

var (
	memberTeamURLs = []string{
		"https://mot-dev01.example.com/cpms/webhooks",
		"https://mot-preprod.example.com/cpms/webhooks",
		"https://mot.example.com/cpms/webhooks",
	}
	otherTeamURLs = []string{
		"https://theory-test-dev01.example.com/cpms/webhooks",
		"https://theory-test.example.com/cpms/webhooks",
	}
)

// newWebhookApplication returns the app itself, so that a test can replace its queries.
func newWebhookApplication(t *testing.T) (*Application, *sql.DB, *bytes.Buffer) {
	t.Helper()
	db := newTestDatabase(t)
	var logs bytes.Buffer
	app, err := New(Options{
		Logger:        slog.New(slog.NewJSONHandler(&logs, nil)),
		HealthCheck:   healthy,
		Database:      db,
		SecureCookies: true,
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	return app, db, &logs
}

func webhookPath(t *testing.T, db *sql.DB, team, environment string) string {
	t.Helper()
	const query = `
		SELECT webhooks.id FROM webhooks
		JOIN teams ON teams.id = webhooks.team_id
		JOIN environments ON environments.id = webhooks.environment_id
		WHERE teams.name = ? AND environments.name = ?`
	var id int64
	if err := db.QueryRowContext(t.Context(), query, team, environment).Scan(&id); err != nil {
		t.Fatalf("find the %s webhook for %s: %v", team, environment, err)
	}
	return "/webhooks/" + strconv.FormatInt(id, 10)
}

func teamID(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	team, err := store.New(db).GetTeamByName(t.Context(), name)
	if err != nil {
		t.Fatalf("find team %s: %v", name, err)
	}
	return team.ID
}

func TestWebhookListShowsAMemberOnlyTheirTeam(t *testing.T) {
	app, _, logs := newWebhookApplication(t)
	routes := app.Routes()
	member := signIn(t, routes, seed.MemberEmail, testMemberPassword)

	response := send(t, routes, http.MethodGet, "/webhooks", nil, member)
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	for _, url := range memberTeamURLs {
		if !strings.Contains(body, url) {
			t.Errorf("the list does not show the member's own webhook %s", url)
		}
	}
	for _, url := range otherTeamURLs {
		if strings.Contains(body, url) {
			t.Errorf("the list shows another team's webhook %s", url)
		}
	}
	if strings.Contains(body, otherTeam) {
		t.Error("the list names another team")
	}
	if strings.Contains(body, `<th scope="col" class="govuk-table__header">Team</th>`) {
		t.Error("the list has a Team column for a member")
	}
	if strings.Count(body, "govuk-tag--grey") != 1 || strings.Count(body, "govuk-tag--green") != 2 {
		t.Error("the list does not show one inactive and two active webhooks")
	}
	if strings.Count(body, ">Production</strong>") != 1 {
		t.Error("the list does not mark exactly one webhook as production")
	}
	// The page alone cannot show that the query filtered by team, because the policy would hide
	// another team's rows anyway. A refusal in the log means the query returned rows it should not have.
	if refusals := logLines(t, logs, "a query returned a webhook the policy refuses"); len(refusals) != 0 {
		t.Errorf("the policy refused %d rows, want 0 because the query filters by team", len(refusals))
	}
}

func TestWebhookListShowsAnAdminEveryTeam(t *testing.T) {
	app, _, _ := newWebhookApplication(t)
	routes := app.Routes()
	admin := signIn(t, routes, seed.AdminEmail, testAdminPassword)

	response := send(t, routes, http.MethodGet, "/webhooks", nil, admin)
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	for _, url := range append(memberTeamURLs, otherTeamURLs...) {
		if !strings.Contains(body, url) {
			t.Errorf("the list does not show %s", url)
		}
	}
	if !strings.Contains(body, `<th scope="col" class="govuk-table__header">Team</th>`) {
		t.Error("the list has no Team column for an admin")
	}
	// Ordered by team, then by the environments' own order.
	if strings.Index(body, memberTeamURLs[0]) > strings.Index(body, memberTeamURLs[2]) || strings.Index(body, memberTeamURLs[2]) > strings.Index(body, otherTeamURLs[0]) {
		t.Error("the list is not ordered by team and then environment")
	}
}

func TestWebhookListWithNothingToShow(t *testing.T) {
	app, db, _ := newWebhookApplication(t)
	routes := app.Routes()
	if _, err := db.ExecContext(t.Context(), "DELETE FROM webhooks WHERE team_id = ?", teamID(t, db, memberTeam)); err != nil {
		t.Fatalf("delete the member's webhooks: %v", err)
	}
	member := signIn(t, routes, seed.MemberEmail, testMemberPassword)

	response := send(t, routes, http.MethodGet, "/webhooks", nil, member)
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if !strings.Contains(body, "There are no webhooks to show.") || strings.Contains(body, "<table") {
		t.Error("an empty list is not shown as a sentence without a table")
	}
}

func TestWebhookDetailShowsAMemberTheirOwnWebhook(t *testing.T) {
	app, db, _ := newWebhookApplication(t)
	routes := app.Routes()
	member := signIn(t, routes, seed.MemberEmail, testMemberPassword)
	path := webhookPath(t, db, memberTeam, "prod")

	var ciphertext []byte
	var lastFour string
	row := db.QueryRowContext(t.Context(), "SELECT secret_ciphertext, secret_last4 FROM webhooks WHERE url = ?", memberTeamURLs[2])
	if err := row.Scan(&ciphertext, &lastFour); err != nil {
		t.Fatalf("read the stored secret: %v", err)
	}
	secret, err := testSecrets(t).Open(ciphertext)
	if err != nil {
		t.Fatalf("open the stored secret: %v", err)
	}

	response := send(t, routes, http.MethodGet, path, nil, member)
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	for _, want := range []string{
		"MOT webhook for prod",
		memberTeamURLs[2],
		">Production</strong>",
		">Active</strong>",
		"payment.succeeded: A payment was taken",
		"refund.succeeded: A refund was paid",
		"Ends with " + lastFour,
		`href="/webhooks" class="govuk-back-link"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not contain %q", want)
		}
	}
	for _, unwanted := range []string{"payment.failed", "refund.failed"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the page shows %s, which this webhook is not subscribed to", unwanted)
		}
	}
	if strings.Contains(body, string(secret)) {
		t.Error("the page contains the signing secret")
	}
	if strings.Contains(body, `href="`+memberTeamURLs[2]) {
		t.Error("the destination URL is a link")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Error("the page may be kept by a browser or proxy")
	}
}

func TestWebhookDetailShowsAnInactiveWebhookWithNoEvents(t *testing.T) {
	app, db, _ := newWebhookApplication(t)
	routes := app.Routes()
	member := signIn(t, routes, seed.MemberEmail, testMemberPassword)
	path := webhookPath(t, db, memberTeam, "preprod")
	if _, err := db.ExecContext(t.Context(), "DELETE FROM webhook_events WHERE webhook_id = ?", strings.TrimPrefix(path, "/webhooks/")); err != nil {
		t.Fatalf("unsubscribe the webhook: %v", err)
	}

	body := send(t, routes, http.MethodGet, path, nil, member).Body.String()

	if !strings.Contains(body, ">Inactive</strong>") || strings.Contains(body, ">Production</strong>") {
		t.Error("the page does not show an inactive webhook outside production")
	}
	if !strings.Contains(body, "None") || strings.Contains(body, "payment.succeeded") {
		t.Error("the page does not say the webhook has no events")
	}
}

func TestWebhookDetailShowsAnAdminAnyTeamsWebhook(t *testing.T) {
	app, db, _ := newWebhookApplication(t)
	routes := app.Routes()
	admin := signIn(t, routes, seed.AdminEmail, testAdminPassword)

	for _, team := range []string{memberTeam, otherTeam} {
		response := send(t, routes, http.MethodGet, webhookPath(t, db, team, "prod"), nil, admin)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), team+" webhook for prod") {
			t.Errorf("%s: status = %d, want 200 with that team's webhook", team, response.Code)
		}
	}
}

// Another team's webhook, one that does not exist and an address that is not a plain number
// must be impossible to tell apart, or the IDs could be used to find out what exists.
func TestWebhookDetailGivesOneAnswerForEverythingAMemberMayNotSee(t *testing.T) {
	app, db, logs := newWebhookApplication(t)
	routes := app.Routes()
	member := signIn(t, routes, seed.MemberEmail, testMemberPassword)
	own := webhookPath(t, db, memberTeam, "dev01")
	ownID := strings.TrimPrefix(own, "/webhooks/")

	missing := send(t, routes, http.MethodGet, "/webhooks/999999", nil, member)
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "Page not found") {
		t.Fatalf("a webhook that does not exist: status = %d, want the 404 page", missing.Code)
	}

	targets := map[string]string{
		"another team's webhook":            webhookPath(t, db, otherTeam, "dev01"),
		"another team's production webhook": webhookPath(t, db, otherTeam, "prod"),
		"zero":                              "/webhooks/0",
		"a negative number":                 "/webhooks/-1",
		"letters":                           "/webhooks/abc",
		"the member's own ID with a plus":   "/webhooks/+" + ownID,
		"the member's own ID with a zero":   "/webhooks/0" + ownID,
		"the member's own ID as a decimal":  "/webhooks/" + ownID + ".0",
		"the member's own ID with a space":  "/webhooks/%20" + ownID,
		"a number too large to store":       "/webhooks/99999999999999999999",
	}
	for name, target := range targets {
		t.Run(name, func(t *testing.T) {
			response := send(t, routes, http.MethodGet, target, nil, member)

			if response.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404", response.Code)
			}
			if response.Body.String() != missing.Body.String() {
				t.Error("the page differs from the one for a webhook that does not exist")
			}
		})
	}

	if response := send(t, routes, http.MethodGet, own, nil, member); response.Code != http.StatusOK {
		t.Errorf("the member's own webhook: status = %d, want 200", response.Code)
	}
	if refusals := logLines(t, logs, "a query returned a webhook the policy refuses"); len(refusals) != 0 {
		t.Errorf("the policy refused %d rows, want 0 because the queries filter by team", len(refusals))
	}
}

func TestTeamQueriesReturnOnlyThatTeamsWebhooks(t *testing.T) {
	db := newTestDatabase(t)
	queries := store.New(db)
	memberTeamID := teamID(t, db, memberTeam)
	otherTeamID := teamID(t, db, otherTeam)

	rows, err := queries.ListWebhooksForTeam(t.Context(), memberTeamID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != len(memberTeamURLs) {
		t.Errorf("listed %d webhooks, want %d", len(rows), len(memberTeamURLs))
	}
	for _, row := range rows {
		if row.TeamID != memberTeamID {
			t.Errorf("listed webhook %d, which belongs to team %d", row.ID, row.TeamID)
		}
	}
	if none, err := queries.ListWebhooksForTeam(t.Context(), 0); err != nil || len(none) != 0 {
		t.Errorf("team 0 listed %d webhooks (error %v), want none", len(none), err)
	}

	others, err := queries.ListWebhooksForTeam(t.Context(), otherTeamID)
	if err != nil || len(others) == 0 {
		t.Fatalf("list the other team: %d rows, error %v", len(others), err)
	}
	_, err = queries.GetWebhookForTeam(t.Context(), store.GetWebhookForTeamParams{ID: others[0].ID, TeamID: memberTeamID})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("another team's webhook: error = %v, want no rows", err)
	}
	own, err := queries.GetWebhookForTeam(t.Context(), store.GetWebhookForTeamParams{ID: rows[0].ID, TeamID: memberTeamID})
	if err != nil || own.TeamID != memberTeamID {
		t.Errorf("the team's own webhook: team %d, error %v", own.TeamID, err)
	}
}

// queriesWithoutTeamFilter behaves as if the ForTeam queries had lost their WHERE clause.
type queriesWithoutTeamFilter struct {
	store.Querier
}

func (queries queriesWithoutTeamFilter) ListWebhooksForTeam(ctx context.Context, _ int64) ([]store.ListWebhooksForTeamRow, error) {
	rows, err := queries.ListWebhooks(ctx)
	unfiltered := make([]store.ListWebhooksForTeamRow, 0, len(rows))
	for _, row := range rows {
		unfiltered = append(unfiltered, store.ListWebhooksForTeamRow(row))
	}
	return unfiltered, err
}

func (queries queriesWithoutTeamFilter) GetWebhookForTeam(ctx context.Context, arg store.GetWebhookForTeamParams) (store.GetWebhookForTeamRow, error) {
	row, err := queries.GetWebhook(ctx, arg.ID)
	return store.GetWebhookForTeamRow(row), err
}

func TestThePolicyStillHidesOtherTeamsWhenTheQueriesDoNotFilter(t *testing.T) {
	app, db, logs := newWebhookApplication(t)
	app.queries = queriesWithoutTeamFilter{app.queries}
	routes := app.Routes()
	member := signIn(t, routes, seed.MemberEmail, testMemberPassword)

	list := send(t, routes, http.MethodGet, "/webhooks", nil, member).Body.String()
	for _, url := range memberTeamURLs {
		if !strings.Contains(list, url) {
			t.Errorf("the list does not show the member's own webhook %s", url)
		}
	}
	for _, url := range otherTeamURLs {
		if strings.Contains(list, url) {
			t.Errorf("the list shows another team's webhook %s", url)
		}
	}

	missing := send(t, routes, http.MethodGet, "/webhooks/999999", nil, member)
	detail := send(t, routes, http.MethodGet, webhookPath(t, db, otherTeam, "prod"), nil, member)
	if detail.Code != http.StatusNotFound || detail.Body.String() != missing.Body.String() {
		t.Errorf("another team's webhook: status = %d, want the same 404 page as a webhook that does not exist", detail.Code)
	}

	// Two rows refused in the list and one on the detail page, each naming the user and the webhook.
	refusals := logLines(t, logs, "a query returned a webhook the policy refuses")
	if len(refusals) != len(otherTeamURLs)+1 {
		t.Fatalf("logged %d refusals, want %d", len(refusals), len(otherTeamURLs)+1)
	}
	for _, refusal := range refusals {
		if refusal["level"] != "ERROR" || refusal["user_id"] == nil || refusal["webhook_id"] == nil || refusal["request_id"] == nil {
			t.Errorf("refusal %v is not an error naming the request, user and webhook", refusal)
		}
	}
}

func TestWebhookPagesEscapeWhatTheyShow(t *testing.T) {
	app, db, _ := newWebhookApplication(t)
	routes := app.Routes()
	member := signIn(t, routes, seed.MemberEmail, testMemberPassword)
	path := webhookPath(t, db, memberTeam, "dev01")
	const attack = `https://example.com/"><script>alert(1)</script>`
	if _, err := db.ExecContext(t.Context(), "UPDATE webhooks SET url = ? WHERE url = ?", attack, memberTeamURLs[0]); err != nil {
		t.Fatalf("change the URL: %v", err)
	}

	for _, target := range []string{"/webhooks", path} {
		body := send(t, routes, http.MethodGet, target, nil, member).Body.String()

		if strings.Contains(body, "<script>alert(1)</script>") {
			t.Errorf("%s: the URL was written into the page as HTML", target)
		}
		if !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") {
			t.Errorf("%s: the URL is not shown escaped", target)
		}
	}
}

func TestWebhookPagesReportADatabaseFailure(t *testing.T) {
	testCases := map[string]struct {
		breakDatabase string
		target        func(t *testing.T, db *sql.DB) string
	}{
		"list":                {breakDatabase: "DROP TABLE webhook_events; DROP TABLE webhooks", target: func(*testing.T, *sql.DB) string { return "/webhooks" }},
		"detail":              {breakDatabase: "DROP TABLE webhook_events; DROP TABLE webhooks", target: func(*testing.T, *sql.DB) string { return "/webhooks/1" }},
		"events for a detail": {breakDatabase: "DROP TABLE webhook_events", target: func(t *testing.T, db *sql.DB) string { return webhookPath(t, db, memberTeam, "dev01") }},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			app, db, logs := newWebhookApplication(t)
			routes := app.Routes()
			member := signIn(t, routes, seed.MemberEmail, testMemberPassword)
			target := testCase.target(t, db)
			if _, err := db.ExecContext(t.Context(), testCase.breakDatabase); err != nil {
				t.Fatalf("break the database: %v", err)
			}

			response := send(t, routes, http.MethodGet, target, nil, member)

			if response.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500", response.Code)
			}
			if strings.Contains(response.Body.String(), "no such table") {
				t.Error("the page shows the database's error")
			}
			if len(logLines(t, logs, "request failed")) != 1 {
				t.Error("the failure was not logged once")
			}
		})
	}
}

func TestPagesLinkToTheWebhookList(t *testing.T) {
	app, _, _ := newWebhookApplication(t)
	routes := app.Routes()

	signedOut := send(t, routes, http.MethodGet, "/sign-in", nil).Body.String()
	if strings.Contains(signedOut, `href="/webhooks"`) {
		t.Error("the sign-in page links to the webhook list")
	}

	member := signIn(t, routes, seed.MemberEmail, testMemberPassword)
	home := send(t, routes, http.MethodGet, "/", nil, member).Body.String()
	if !strings.Contains(home, `<a class="govuk-service-navigation__link" href="/webhooks">Webhooks</a>`) {
		t.Error("the navigation has no Webhooks link")
	}
	if !strings.Contains(home, `<a href="/webhooks" role="button"`) || strings.Contains(home, "Webhooks are not available yet") {
		t.Error("the home page does not send people to the webhook list")
	}
}

func TestDisplayDate(t *testing.T) {
	testCases := map[string]string{
		"2026-11-15T21:29:43Z": "15 November 2026",
		"2026-01-02T00:00:00Z": "2 January 2026",
		"not a timestamp":      "not a timestamp",
		"":                     "",
	}

	for stored, want := range testCases {
		if got := displayDate(stored); got != want {
			t.Errorf("displayDate(%q) = %q, want %q", stored, got, want)
		}
	}
}
