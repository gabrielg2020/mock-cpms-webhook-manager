package web

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/seed"
	"github.com/go-chi/chi/v5"
)

func TestAdminPagesNeedAnAdministrator(t *testing.T) {
	routes, _, logs := newSignInApplication(t, true)

	signedOut := send(t, routes, http.MethodGet, "/admin", nil)
	if signedOut.Code != http.StatusSeeOther || signedOut.Header().Get("Location") != "/sign-in" {
		t.Errorf("signed out: status = %d to %q, want 303 to /sign-in", signedOut.Code, signedOut.Header().Get("Location"))
	}

	member := signIn(t, routes, seed.MemberEmail, testMemberPassword)
	refused := send(t, routes, http.MethodGet, "/admin", nil, member)
	if refused.Code != http.StatusForbidden {
		t.Errorf("member: status = %d, want 403", refused.Code)
	}
	if !strings.Contains(refused.Body.String(), "You do not have permission to use this page") {
		t.Error("member: the refusal is not the styled page")
	}
	if strings.Contains(refused.Body.String(), "Administration") {
		t.Error("member: the refusal shows part of the admin page")
	}
	denials := logLines(t, logs, "access denied")
	if len(denials) != 1 || denials[0]["path"] != "/admin" || denials[0]["user_id"] == nil {
		t.Errorf("denials logged = %v, want one for /admin with the user", denials)
	}

	// Whatever a member asks for under /admin, the answer is the same,
	// so it tells them nothing about which pages exist there.
	for _, target := range []string{"/admin/", "/admin/no-such-page"} {
		if response := send(t, routes, http.MethodGet, target, nil, member); response.Code != http.StatusForbidden {
			t.Errorf("member GET %s: status = %d, want 403", target, response.Code)
		}
	}
	if response := send(t, routes, http.MethodPost, "/admin", url.Values{}, member); response.Code != http.StatusForbidden {
		t.Errorf("member POST /admin: status = %d, want 403", response.Code)
	}

	admin := signIn(t, routes, seed.AdminEmail, testAdminPassword)
	allowed := send(t, routes, http.MethodGet, "/admin", nil, admin)
	if allowed.Code != http.StatusOK || !strings.Contains(allowed.Body.String(), "Administration") {
		t.Errorf("admin: status = %d, want 200 with the admin page", allowed.Code)
	}
}

// A page added under /admin later is covered by this test without anyone remembering to add it.
func TestEveryAdminRouteRefusesAMember(t *testing.T) {
	db := newTestDatabase(t)
	app, err := New(Options{Logger: slog.New(slog.DiscardHandler), HealthCheck: healthy, Database: db, SecureCookies: true})
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	routes := app.Routes()
	member := signIn(t, routes, seed.MemberEmail, testMemberPassword)

	checked := 0
	walk := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(route, "/admin") {
			return nil
		}
		checked++
		if response := send(t, routes, method, route, nil, member); response.Code != http.StatusForbidden {
			t.Errorf("%s %s as a member: status = %d, want 403", method, route, response.Code)
		}
		return nil
	}
	if err := chi.Walk(routes.(chi.Routes), walk); err != nil {
		t.Fatalf("walk routes: %v", err)
	}
	if checked == 0 {
		t.Fatal("no admin routes were checked")
	}
}

func TestRequireAdminRefusesWhenNobodyIsSignedIn(t *testing.T) {
	app := newTestApplication(t, healthy)
	reached := false
	guarded := app.requireAdmin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))

	recorder := httptest.NewRecorder()
	guarded.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/admin", nil))

	if recorder.Code != http.StatusForbidden || reached {
		t.Errorf("status = %d and page reached = %t, want 403 and not reached", recorder.Code, reached)
	}
}

func TestARoleChangeAppliesOnTheNextRequest(t *testing.T) {
	routes, db, _ := newSignInApplication(t, true)
	admin := signIn(t, routes, seed.AdminEmail, testAdminPassword)
	member := signIn(t, routes, seed.MemberEmail, testMemberPassword)

	if _, err := db.ExecContext(t.Context(), "UPDATE users SET role = 'member', team_id = (SELECT id FROM teams LIMIT 1) WHERE email = ?", seed.AdminEmail); err != nil {
		t.Fatalf("demote admin: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), "UPDATE users SET role = 'admin' WHERE email = ?", seed.MemberEmail); err != nil {
		t.Fatalf("promote member: %v", err)
	}

	if response := send(t, routes, http.MethodGet, "/admin", nil, admin); response.Code != http.StatusForbidden {
		t.Errorf("demoted admin, same session: status = %d, want 403", response.Code)
	}
	if response := send(t, routes, http.MethodGet, "/admin", nil, member); response.Code != http.StatusOK {
		t.Errorf("promoted member, same session: status = %d, want 200", response.Code)
	}
}

func TestPagesShowWhatTheRoleAllows(t *testing.T) {
	routes, _, _ := newSignInApplication(t, true)

	testCases := []struct {
		name     string
		email    string
		password string
		want     []string
		wantNot  []string
	}{
		{
			name: "admin", email: seed.AdminEmail, password: testAdminPassword,
			want: []string{`href="/admin"`, "Administrator", "All teams"},
		},
		{
			name: "member", email: seed.MemberEmail, password: testMemberPassword,
			want:    []string{"Member", "MOT team"},
			wantNot: []string{`href="/admin"`, "Administrator", "All teams"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			cookie := signIn(t, routes, testCase.email, testCase.password)
			page := send(t, routes, http.MethodGet, "/", nil, cookie).Body.String()

			for _, want := range testCase.want {
				if !strings.Contains(page, want) {
					t.Errorf("home page does not contain %s", want)
				}
			}
			for _, wantNot := range testCase.wantNot {
				if strings.Contains(page, wantNot) {
					t.Errorf("home page contains %s", wantNot)
				}
			}
		})
	}
}
