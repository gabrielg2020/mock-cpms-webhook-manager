package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/seed"
)

// The proxy's address on the server's private network, as the app sees it.
const proxyPeer = "172.18.0.2:51000"

// sendFrom makes one request that arrives from peer. forwardedFor is the X-Forwarded-For header, if any.
func sendFrom(t *testing.T, routes http.Handler, method, target, peer, forwardedFor string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(form.Encode()))
	request.RemoteAddr = peer
	if forwardedFor != "" {
		request.Header.Set("X-Forwarded-For", forwardedFor)
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
	}

	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, request)
	return recorder
}

// lockOf returns the failed sign-in count and the lock stored for an account.
func lockOf(t *testing.T, db *sql.DB, email string) (failures int, lockedUntil sql.NullString) {
	t.Helper()
	row := db.QueryRowContext(t.Context(), "SELECT failed_logins, locked_until FROM users WHERE email = ?", email)
	if err := row.Scan(&failures, &lockedUntil); err != nil {
		t.Fatalf("read lock of %s: %v", email, err)
	}
	return failures, lockedUntil
}

func TestAccountLocksAfterFiveWrongPasswords(t *testing.T) {
	routes, db, logs := newSignInApplication(t, true)
	wrong := credentials(seed.MemberEmail, "not the password")

	// The first wrong password is an ordinary failure. The fifth is the one that locks the account.
	var firstWrong, fifthWrong *httptest.ResponseRecorder
	for attempt := 1; attempt <= 5; attempt++ {
		fifthWrong = send(t, routes, http.MethodPost, "/sign-in", wrong)
		if fifthWrong.Code != http.StatusUnprocessableEntity {
			t.Fatalf("wrong password %d: status = %d, want 422", attempt, fifthWrong.Code)
		}
		if attempt == 1 {
			firstWrong = fifthWrong
		}
	}
	if failures, lockedUntil := lockOf(t, db, seed.MemberEmail); !lockedUntil.Valid || failures != 0 {
		t.Fatalf("after five wrong passwords: lock = %v and count = %d, want a lock and the count back at 0", lockedUntil, failures)
	}

	refused := send(t, routes, http.MethodPost, "/sign-in", credentials(seed.MemberEmail, testMemberPassword))
	if refused.Code != http.StatusUnprocessableEntity || sessionCookie(refused) != nil {
		t.Errorf("correct password while locked: status = %d, want 422 and no session", refused.Code)
	}
	// A page that said the account was locked would confirm to a guesser that the account exists,
	// so both locked replies must be the same as an ordinary wrong password.
	if fifthWrong.Body.String() != firstWrong.Body.String() {
		t.Error("the page for the wrong password that locks the account differs from the page for any wrong password")
	}
	if refused.Body.String() != firstWrong.Body.String() {
		t.Error("the page for a locked account differs from the page for a wrong password")
	}
	if got := len(logLines(t, logs, "sign-in refused: account locked")); got != 2 {
		t.Errorf("locked refusals logged = %d, want 2: the fifth wrong password and the correct one", got)
	}
	if strings.Contains(logs.String(), testMemberPassword) {
		t.Error("the password was logged")
	}

	signIn(t, routes, seed.AdminEmail, testAdminPassword)

	// Fifteen minutes later, as far as the database can tell.
	if _, err := db.ExecContext(t.Context(), "UPDATE users SET locked_until = '2000-01-01T00:00:00Z' WHERE email = ?", seed.MemberEmail); err != nil {
		t.Fatalf("end the lock: %v", err)
	}
	signIn(t, routes, seed.MemberEmail, testMemberPassword)
	if failures, lockedUntil := lockOf(t, db, seed.MemberEmail); lockedUntil.Valid || failures != 0 {
		t.Errorf("after signing in: lock = %v and count = %d, want both cleared", lockedUntil, failures)
	}
}

func TestSigningInStartsTheCountAgain(t *testing.T) {
	routes, db, _ := newSignInApplication(t, true)
	wrong := credentials(seed.MemberEmail, "not the password")

	for range 2 {
		send(t, routes, http.MethodPost, "/sign-in", wrong)
	}
	if failures, _ := lockOf(t, db, seed.MemberEmail); failures != 2 {
		t.Fatalf("count after two wrong passwords = %d, want 2", failures)
	}
	signIn(t, routes, seed.MemberEmail, testMemberPassword)
	for range 4 {
		send(t, routes, http.MethodPost, "/sign-in", wrong)
	}

	signIn(t, routes, seed.MemberEmail, testMemberPassword)
}

func TestSignInIsLimitedForEachAddress(t *testing.T) {
	routes, _, _ := newSignInApplication(t, true)
	guess := credentials("nobody@example.com", "not the password")

	for attempt := 1; attempt <= signInAttemptsPerMinute; attempt++ {
		response := sendFrom(t, routes, http.MethodPost, "/sign-in", "203.0.113.5:40000", "", guess)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("attempt %d: status = %d, want 422", attempt, response.Code)
		}
	}

	limited := sendFrom(t, routes, http.MethodPost, "/sign-in", "203.0.113.5:40001", "", guess)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt over the limit: status = %d, want 429", limited.Code)
	}
	if !strings.Contains(limited.Body.String(), "too many sign-in attempts") {
		t.Error("the refusal is not the styled page")
	}
	if wait, err := strconv.Atoi(limited.Header().Get("Retry-After")); err != nil || wait < 1 || wait > 60 {
		t.Errorf("Retry-After = %q, want 1 to 60 seconds", limited.Header().Get("Retry-After"))
	}

	correct := sendFrom(t, routes, http.MethodPost, "/sign-in", "203.0.113.5:40002", "", credentials(seed.AdminEmail, testAdminPassword))
	if correct.Code != http.StatusTooManyRequests || sessionCookie(correct) != nil {
		t.Errorf("correct password over the limit: status = %d, want 429 and no session", correct.Code)
	}
	forged := sendFrom(t, routes, http.MethodPost, "/sign-in", "203.0.113.5:40003", "198.51.100.9", guess)
	if forged.Code != http.StatusTooManyRequests {
		t.Errorf("a forged X-Forwarded-For got round the limit: status = %d, want 429", forged.Code)
	}

	if page := sendFrom(t, routes, http.MethodGet, "/sign-in", "203.0.113.5:40004", "", nil); page.Code != http.StatusOK {
		t.Errorf("sign-in page for a limited address: status = %d, want 200", page.Code)
	}
	if other := sendFrom(t, routes, http.MethodPost, "/sign-in", "203.0.113.6:40000", "", guess); other.Code != http.StatusUnprocessableEntity {
		t.Errorf("another address: status = %d, want 422", other.Code)
	}
}

func TestClientsBehindTheProxyAreLimitedSeparately(t *testing.T) {
	routes, _, _ := newSignInApplication(t, true)
	guess := credentials("nobody@example.com", "not the password")

	for range signInAttemptsPerMinute {
		sendFrom(t, routes, http.MethodPost, "/sign-in", proxyPeer, "203.0.113.5", guess)
	}
	if limited := sendFrom(t, routes, http.MethodPost, "/sign-in", proxyPeer, "203.0.113.5", guess); limited.Code != http.StatusTooManyRequests {
		t.Errorf("client over the limit: status = %d, want 429", limited.Code)
	}
	if other := sendFrom(t, routes, http.MethodPost, "/sign-in", proxyPeer, "203.0.113.6", guess); other.Code != http.StatusUnprocessableEntity {
		t.Errorf("another client through the same proxy: status = %d, want 422", other.Code)
	}

	// Addresses in one IPv6 /64 are one customer's to choose from, so they share a count.
	for last := range signInAttemptsPerMinute {
		sendFrom(t, routes, http.MethodPost, "/sign-in", proxyPeer, "2001:db8:1:2::"+strconv.Itoa(last+1), guess)
	}
	if sameNetwork := sendFrom(t, routes, http.MethodPost, "/sign-in", proxyPeer, "2001:db8:1:2:aaaa::1", guess); sameNetwork.Code != http.StatusTooManyRequests {
		t.Errorf("new address in a limited IPv6 network: status = %d, want 429", sameNetwork.Code)
	}
	if otherNetwork := sendFrom(t, routes, http.MethodPost, "/sign-in", proxyPeer, "2001:db8:1:3::1", guess); otherNetwork.Code != http.StatusUnprocessableEntity {
		t.Errorf("another IPv6 network: status = %d, want 422", otherNetwork.Code)
	}
}

func TestRequestLogSaysWhoAndFromWhere(t *testing.T) {
	routes, db, logs := newSignInApplication(t, true)
	var adminID float64
	if err := db.QueryRowContext(t.Context(), "SELECT id FROM users WHERE email = ?", seed.AdminEmail).Scan(&adminID); err != nil {
		t.Fatalf("find admin: %v", err)
	}
	last := func() map[string]any {
		lines := logLines(t, logs, "request completed")
		return lines[len(lines)-1]
	}

	sendFrom(t, routes, http.MethodGet, "/sign-in", proxyPeer, "203.0.113.5", nil)
	if line := last(); line["client_ip"] != "203.0.113.5" {
		t.Errorf("client_ip through the proxy = %v, want 203.0.113.5", line["client_ip"])
	}
	if _, present := last()["user_id"]; present {
		t.Error("a signed-out request was logged with a user")
	}

	sendFrom(t, routes, http.MethodGet, "/sign-in", "203.0.113.5:40000", "198.51.100.9", nil)
	if line := last(); line["client_ip"] != "203.0.113.5" {
		t.Errorf("client_ip with a forged header = %v, want the real address 203.0.113.5", line["client_ip"])
	}

	cookie := signIn(t, routes, seed.AdminEmail, testAdminPassword)
	send(t, routes, http.MethodGet, "/", nil, cookie)
	if line := last(); line["user_id"] != adminID || line["client_ip"] != "192.0.2.1" {
		t.Errorf("signed-in request logged user %v from %v, want user %v from 192.0.2.1", line["user_id"], line["client_ip"], adminID)
	}
}
