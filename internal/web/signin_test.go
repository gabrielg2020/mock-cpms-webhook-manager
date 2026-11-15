package web

import (
	"bytes"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/database"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/seed"
	"github.com/go-chi/chi/v5"
)

const (
	testAdminPassword  = "correct horse battery staple"
	testMemberPassword = "another long demo password"
)

// newTestDatabase returns an empty database in a temporary folder, with the demo accounts.
func newTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	passwords := seed.Passwords{Admin: testAdminPassword, Member: testMemberPassword}
	if err := seed.DemoData(t.Context(), db, passwords); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	return db
}

// newSignInApplication returns an app, its routes, its database and what it logs.
func newSignInApplication(t *testing.T, secureCookies bool) (http.Handler, *sql.DB, *bytes.Buffer) {
	t.Helper()
	db := newTestDatabase(t)
	var logs bytes.Buffer
	app, err := New(Options{
		Logger:        slog.New(slog.NewJSONHandler(&logs, nil)),
		HealthCheck:   healthy,
		Database:      db,
		SecureCookies: secureCookies,
	})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	return app.Routes(), db, &logs
}

// send makes one request. A form makes it a POST, and a browser's same-origin header is added
// so it passes the cross-origin check unless the test replaces it.
func send(t *testing.T, routes http.Handler, method, target string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}

	request := httptest.NewRequestWithContext(t.Context(), method, target, body)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}

	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, request)
	return recorder
}

func credentials(email, password string) url.Values {
	return url.Values{"email": {email}, "password": {password}}
}

// sessionCookie returns the session cookie a response set, or nil if it set none.
func sessionCookie(response *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "__Host-session" || cookie.Name == "session" {
			return cookie
		}
	}
	return nil
}

// signIn signs in and returns the session cookie. Sending it back uses only its name and value.
func signIn(t *testing.T, routes http.Handler, email, password string) *http.Cookie {
	t.Helper()
	response := send(t, routes, http.MethodPost, "/sign-in", credentials(email, password))
	if response.Code != http.StatusSeeOther {
		t.Fatalf("sign-in status = %d, want 303", response.Code)
	}
	cookie := sessionCookie(response)
	if cookie == nil || cookie.Value == "" {
		t.Fatal("sign-in did not set a session cookie")
	}
	return cookie
}

func TestSignInPageShowsTheForm(t *testing.T) {
	routes, _, _ := newSignInApplication(t, true)

	response := send(t, routes, http.MethodGet, "/sign-in", nil)
	body := response.Body.String()

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	for _, want := range []string{`action="/sign-in"`, `name="email"`, `name="password"`, `autocomplete="current-password"`} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %s", want)
		}
	}
	if strings.Contains(body, "There is a problem") {
		t.Error("a new form shows the error summary")
	}
	if sessionCookie(response) != nil {
		t.Error("visiting the sign-in page set a session cookie")
	}
}

func TestSignInRejectsMissingFields(t *testing.T) {
	routes, _, _ := newSignInApplication(t, true)

	response := send(t, routes, http.MethodPost, "/sign-in", credentials("  ", ""))
	body := response.Body.String()

	if response.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", response.Code)
	}
	for _, want := range []string{
		"<title>Error: Sign in",
		`<a href="#email">Enter your email address</a>`,
		`<a href="#password">Enter your password</a>`,
		`aria-describedby="email-error"`,
		`aria-describedby="password-error"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %s", want)
		}
	}
	if sessionCookie(response) != nil {
		t.Error("a rejected sign-in set a session cookie")
	}
}

func TestSignInFailuresLookTheSame(t *testing.T) {
	routes, _, _ := newSignInApplication(t, true)

	wrongPassword := send(t, routes, http.MethodPost, "/sign-in", credentials(seed.AdminEmail, "not the password"))
	unknownEmail := send(t, routes, http.MethodPost, "/sign-in", credentials("nobody@example.com", "not the password"))

	for name, response := range map[string]*httptest.ResponseRecorder{"wrong password": wrongPassword, "unknown email": unknownEmail} {
		if response.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d, want 422", name, response.Code)
		}
		if !strings.Contains(response.Body.String(), "Enter a correct email address and password") {
			t.Errorf("%s: page does not show the sign-in error", name)
		}
		if sessionCookie(response) != nil {
			t.Errorf("%s: a failed sign-in set a session cookie", name)
		}
	}

	// Apart from the address typed in, nothing on the page may differ, or it would say which accounts exist.
	first := strings.ReplaceAll(wrongPassword.Body.String(), seed.AdminEmail, "EMAIL")
	second := strings.ReplaceAll(unknownEmail.Body.String(), "nobody@example.com", "EMAIL")
	if first != second {
		t.Error("the page for a wrong password differs from the page for an unknown email address")
	}
}

func TestSignInKeepsTheEmailAndNeverThePassword(t *testing.T) {
	routes, _, logs := newSignInApplication(t, true)

	response := send(t, routes, http.MethodPost, "/sign-in", credentials(seed.AdminEmail, "a-wrong-password-9731"))
	body := response.Body.String()

	if !strings.Contains(body, `value="`+seed.AdminEmail+`"`) {
		t.Error("the email address was not kept in the form")
	}
	if strings.Contains(body, "a-wrong-password-9731") {
		t.Error("the password was sent back in the page")
	}
	if strings.Contains(logs.String(), "a-wrong-password-9731") {
		t.Error("the password was logged")
	}
	if len(logLines(t, logs, "sign-in failed")) != 1 {
		t.Error("the failed sign-in was not logged")
	}
}

func TestSignInSetsAHardenedCookie(t *testing.T) {
	testCases := []struct {
		name          string
		secureCookies bool
		wantName      string
	}{
		{name: "production", secureCookies: true, wantName: "__Host-session"},
		{name: "local development over HTTP", secureCookies: false, wantName: "session"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			routes, _, logs := newSignInApplication(t, testCase.secureCookies)

			response := send(t, routes, http.MethodPost, "/sign-in", credentials(seed.MemberEmail, testMemberPassword))
			if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" {
				t.Fatalf("status = %d to %q, want 303 to /", response.Code, response.Header().Get("Location"))
			}
			cookie := sessionCookie(response)
			if cookie == nil {
				t.Fatal("no session cookie was set")
			}

			if cookie.Name != testCase.wantName {
				t.Errorf("cookie name = %q, want %q", cookie.Name, testCase.wantName)
			}
			if cookie.Secure != testCase.secureCookies {
				t.Errorf("Secure = %t, want %t", cookie.Secure, testCase.secureCookies)
			}
			if !cookie.HttpOnly {
				t.Error("the cookie is not HttpOnly, so a script could read it")
			}
			if cookie.SameSite != http.SameSiteLaxMode {
				t.Errorf("SameSite = %v, want Lax", cookie.SameSite)
			}
			if cookie.Path != "/" || cookie.Domain != "" {
				t.Errorf("Path = %q and Domain = %q, want / and none", cookie.Path, cookie.Domain)
			}
			if cookie.MaxAge != 0 || !cookie.Expires.IsZero() {
				t.Error("the cookie has an expiry, so it would outlive the browser session")
			}
			if len(cookie.Value) < 32 {
				t.Errorf("the token is %d characters, too short to be unguessable", len(cookie.Value))
			}

			if strings.Contains(logs.String(), testMemberPassword) || strings.Contains(logs.String(), cookie.Value) {
				t.Error("the password or the session token was logged")
			}
			if len(logLines(t, logs, "signed in")) != 1 {
				t.Error("the sign-in was not logged")
			}
		})
	}
}

func TestSessionTokenIsStoredHashed(t *testing.T) {
	routes, db, _ := newSignInApplication(t, true)
	cookie := signIn(t, routes, seed.AdminEmail, testAdminPassword)

	var sessions, withThisToken int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*), count(*) FILTER (WHERE token = ?) FROM sessions", cookie.Value).Scan(&sessions, &withThisToken); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 1 {
		t.Errorf("sessions stored = %d, want 1", sessions)
	}
	if withThisToken != 0 {
		t.Error("the database holds the session token itself, not a hash of it")
	}
}

func TestPagesNeedSignIn(t *testing.T) {
	routes, _, _ := newSignInApplication(t, true)

	signedOut := send(t, routes, http.MethodGet, "/", nil)
	if signedOut.Code != http.StatusSeeOther || signedOut.Header().Get("Location") != "/sign-in" {
		t.Errorf("signed out: status = %d to %q, want 303 to /sign-in", signedOut.Code, signedOut.Header().Get("Location"))
	}

	madeUp := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	madeUp.Header.Set("Cookie", "__Host-session=made-up-token")
	forged := httptest.NewRecorder()
	routes.ServeHTTP(forged, madeUp)
	if forged.Code != http.StatusSeeOther {
		t.Errorf("made-up token: status = %d, want 303", forged.Code)
	}

	cookie := signIn(t, routes, seed.AdminEmail, testAdminPassword)
	signedIn := send(t, routes, http.MethodGet, "/", nil, cookie)
	if signedIn.Code != http.StatusOK {
		t.Fatalf("signed in: status = %d, want 200", signedIn.Code)
	}
	for _, want := range []string{"Signed in as Demo Admin", `href="/sign-out"`} {
		if !strings.Contains(signedIn.Body.String(), want) {
			t.Errorf("signed in: page does not contain %s", want)
		}
	}

	again := send(t, routes, http.MethodGet, "/sign-in", nil, cookie)
	if again.Code != http.StatusSeeOther || again.Header().Get("Location") != "/" {
		t.Errorf("sign-in page while signed in: status = %d to %q, want 303 to /", again.Code, again.Header().Get("Location"))
	}
}

func TestEveryRouteNeedsSignInUnlessListed(t *testing.T) {
	db := newTestDatabase(t)
	app, err := New(Options{Logger: slog.New(slog.DiscardHandler), HealthCheck: healthy, Database: db, SecureCookies: true})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	routes := app.Routes()

	public := map[string]bool{
		"GET /healthz":                 true,
		"GET " + app.staticPath + "/*": true,
		"GET /sign-in":                 true,
		"POST /sign-in":                true,
	}

	checked := 0
	walk := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if public[method+" "+route] {
			return nil
		}
		checked++
		response := send(t, routes, method, route, nil)
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/sign-in" {
			t.Errorf("%s %s without signing in: status = %d, want 303 to /sign-in", method, route, response.Code)
		}
		return nil
	}
	if err := chi.Walk(routes.(chi.Routes), walk); err != nil {
		t.Fatalf("walk routes: %v", err)
	}
	if checked == 0 {
		t.Fatal("no routes were checked")
	}
}

func TestSignInGivesANewToken(t *testing.T) {
	routes, _, _ := newSignInApplication(t, true)
	before := signIn(t, routes, seed.MemberEmail, testMemberPassword)

	response := send(t, routes, http.MethodPost, "/sign-in", credentials(seed.AdminEmail, testAdminPassword), before)
	after := sessionCookie(response)
	if after == nil {
		t.Fatal("signing in again did not set a session cookie")
	}

	if after.Value == before.Value {
		t.Error("signing in kept the old token, so a token known before sign-in would still work")
	}
	if old := send(t, routes, http.MethodGet, "/", nil, before); old.Code != http.StatusSeeOther {
		t.Errorf("the old token still works: status = %d, want 303", old.Code)
	}
	current := send(t, routes, http.MethodGet, "/", nil, after)
	if !strings.Contains(current.Body.String(), "Signed in as Demo Admin") {
		t.Error("the new token does not belong to the account that just signed in")
	}
}

func TestSignOut(t *testing.T) {
	routes, db, logs := newSignInApplication(t, true)
	cookie := signIn(t, routes, seed.AdminEmail, testAdminPassword)

	page := send(t, routes, http.MethodGet, "/sign-out", nil, cookie)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Are you sure you want to sign out?") {
		t.Errorf("confirmation page: status = %d, want 200 with the question", page.Code)
	}
	if stillIn := send(t, routes, http.MethodGet, "/", nil, cookie); stillIn.Code != http.StatusOK {
		t.Errorf("opening the confirmation page signed the user out: status = %d", stillIn.Code)
	}

	response := send(t, routes, http.MethodPost, "/sign-out", url.Values{}, cookie)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/sign-in" {
		t.Errorf("status = %d to %q, want 303 to /sign-in", response.Code, response.Header().Get("Location"))
	}
	cleared := sessionCookie(response)
	if cleared == nil || cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Errorf("the browser was not told to delete the cookie: %+v", cleared)
	}

	if after := send(t, routes, http.MethodGet, "/", nil, cookie); after.Code != http.StatusSeeOther {
		t.Errorf("the token still works after signing out: status = %d, want 303", after.Code)
	}
	var sessions int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sessions").Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 0 {
		t.Errorf("sessions stored after signing out = %d, want 0", sessions)
	}
	if len(logLines(t, logs, "signed out")) != 1 {
		t.Error("the sign-out was not logged")
	}
}

func TestCrossOriginPostIsRefused(t *testing.T) {
	routes, _, logs := newSignInApplication(t, true)
	cookie := signIn(t, routes, seed.AdminEmail, testAdminPassword)

	testCases := []struct {
		name    string
		headers map[string]string
	}{
		{name: "another site, modern browser", headers: map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{name: "another site under the same domain", headers: map[string]string{"Sec-Fetch-Site": "same-site"}},
		{name: "another site, older browser", headers: map[string]string{"Origin": "https://evil.example"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/sign-out", strings.NewReader(""))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			for name, value := range testCase.headers {
				request.Header.Set(name, value)
			}
			request.AddCookie(cookie)
			recorder := httptest.NewRecorder()
			routes.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusForbidden {
				t.Errorf("status = %d, want 403", recorder.Code)
			}
			if !strings.Contains(recorder.Body.String(), "did not come from this site") {
				t.Error("the refusal is not the styled page")
			}
			if stillIn := send(t, routes, http.MethodGet, "/", nil, cookie); stillIn.Code != http.StatusOK {
				t.Errorf("the forged request signed the user out: status = %d", stillIn.Code)
			}
		})
	}

	if got := len(logLines(t, logs, "cross-origin request refused")); got != len(testCases) {
		t.Errorf("refusals logged = %d, want %d", got, len(testCases))
	}
}

func TestExpiredSessionIsSignedOut(t *testing.T) {
	routes, db, _ := newSignInApplication(t, true)
	cookie := signIn(t, routes, seed.AdminEmail, testAdminPassword)

	if _, err := db.ExecContext(t.Context(), "UPDATE sessions SET expiry = 1"); err != nil {
		t.Fatalf("expire session: %v", err)
	}

	if response := send(t, routes, http.MethodGet, "/", nil, cookie); response.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", response.Code)
	}

	// The next sign-in clears expired sessions out of the table.
	signIn(t, routes, seed.MemberEmail, testMemberPassword)
	var sessions int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sessions").Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 1 {
		t.Errorf("sessions stored = %d, want only the new one", sessions)
	}
}

func TestRemovedAccountIsSignedOut(t *testing.T) {
	routes, db, _ := newSignInApplication(t, true)
	cookie := signIn(t, routes, seed.MemberEmail, testMemberPassword)

	if _, err := db.ExecContext(t.Context(), "DELETE FROM users WHERE email = ?", seed.MemberEmail); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	response := send(t, routes, http.MethodGet, "/", nil, cookie)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/sign-in" {
		t.Errorf("status = %d to %q, want 303 to /sign-in", response.Code, response.Header().Get("Location"))
	}
	if cleared := sessionCookie(response); cleared == nil || cleared.MaxAge >= 0 {
		t.Error("the browser was not told to delete the cookie")
	}
}

func TestPagesAreNotStoredByTheBrowser(t *testing.T) {
	routes, _, _ := newSignInApplication(t, true)
	cookie := signIn(t, routes, seed.AdminEmail, testAdminPassword)

	for _, target := range []string{"/", "/sign-in", "/sign-out", "/no-such-page"} {
		response := send(t, routes, http.MethodGet, target, nil, cookie)
		if !strings.Contains(strings.Join(response.Header().Values("Cache-Control"), ", "), "no-store") {
			t.Errorf("%s: Cache-Control = %q, want no-store", target, response.Header().Values("Cache-Control"))
		}
	}
}

func TestOversizedFormIsRefused(t *testing.T) {
	routes, _, _ := newSignInApplication(t, true)

	form := credentials(seed.AdminEmail, strings.Repeat("a", maxFormBytes))
	response := send(t, routes, http.MethodPost, "/sign-in", form)

	if response.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", response.Code)
	}
}
