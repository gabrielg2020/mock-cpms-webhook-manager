package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/store"
)

var noon = time.Date(2026, time.November, 15, 12, 0, 0, 0, time.UTC)

// fakeUsers stands in for the users table. It keeps one user and changes it the way the real
// queries would, so one test can make several sign-in attempts in a row.
type fakeUsers struct {
	user     store.User
	findErr  error
	countErr error
}

func (fake *fakeUsers) GetUserByEmail(context.Context, string) (store.User, error) {
	return fake.user, fake.findErr
}

func (fake *fakeUsers) CountFailedSignIn(context.Context, int64) (int64, error) {
	if fake.countErr != nil {
		return 0, fake.countErr
	}
	fake.user.FailedLogins++
	return fake.user.FailedLogins, nil
}

func (fake *fakeUsers) LockUser(_ context.Context, arg store.LockUserParams) error {
	fake.user.LockedUntil = arg.LockedUntil
	fake.user.FailedLogins = 0
	return nil
}

func (fake *fakeUsers) ClearFailedSignIns(context.Context, int64) error {
	fake.user.FailedLogins = 0
	fake.user.LockedUntil = sql.NullString{}
	return nil
}

func knownUser() *fakeUsers {
	return &fakeUsers{user: store.User{ID: 7, PasswordHash: HashPassword("correct password")}}
}

func unknownUser() *fakeUsers {
	return &fakeUsers{findErr: sql.ErrNoRows}
}

// newTestAuthenticator returns an authenticator whose clock says noon, and a function that moves the clock.
func newTestAuthenticator(users UserStore) (*Authenticator, func(time.Duration)) {
	now := noon
	authenticator := NewAuthenticator(users)
	authenticator.now = func() time.Time { return now }
	return authenticator, func(by time.Duration) { now = now.Add(by) }
}

func TestAuthenticate(t *testing.T) {
	databaseDown := errors.New("database is closed")

	testCases := []struct {
		name     string
		users    *fakeUsers
		password string
		wantID   int64
		wantErr  error
	}{
		{name: "correct password", users: knownUser(), password: "correct password", wantID: 7},
		{name: "wrong password", users: knownUser(), password: "wrong password", wantErr: ErrInvalidCredentials},
		{name: "unknown email address", users: unknownUser(), password: "correct password", wantErr: ErrInvalidCredentials},
		{name: "database failure", users: &fakeUsers{findErr: databaseDown}, password: "correct password", wantErr: databaseDown},
		{
			name:     "stored hash is damaged",
			users:    &fakeUsers{user: store.User{ID: 7, PasswordHash: "not a hash"}},
			password: "correct password",
			wantErr:  ErrInvalidHash,
		},
		{
			name:     "failure cannot be counted",
			users:    &fakeUsers{user: knownUser().user, countErr: databaseDown},
			password: "wrong password",
			wantErr:  databaseDown,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			authenticator, _ := newTestAuthenticator(testCase.users)

			id, err := authenticator.Authenticate(t.Context(), "someone@example.com", testCase.password)

			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("error = %v, want %v", err, testCase.wantErr)
			}
			if id != testCase.wantID {
				t.Errorf("user ID = %d, want %d", id, testCase.wantID)
			}
			if inUse := len(authenticator.slots); inUse != 0 {
				t.Errorf("slots still in use afterwards = %d, want 0", inUse)
			}
		})
	}
}

// With a damaged dummy hash, an unknown email address can only fail this way if the password
// was checked against it. Skipping the check would make unknown addresses answer faster.
func TestAuthenticateChecksAPasswordForAnUnknownEmail(t *testing.T) {
	authenticator, _ := newTestAuthenticator(unknownUser())
	authenticator.dummyHash = "not a hash"

	_, err := authenticator.Authenticate(t.Context(), "nobody@example.com", "any password")

	if !errors.Is(err, ErrInvalidHash) {
		t.Errorf("error = %v, want %v: no password check was made for an unknown email address", err, ErrInvalidHash)
	}
}

func TestAuthenticateRefusesWhenBusy(t *testing.T) {
	authenticator, _ := newTestAuthenticator(unknownUser())
	authenticator.maxWait = 10 * time.Millisecond
	for range cap(authenticator.slots) {
		authenticator.slots <- struct{}{}
	}

	_, err := authenticator.Authenticate(t.Context(), "nobody@example.com", "any password")

	if !errors.Is(err, ErrBusy) {
		t.Errorf("error = %v, want %v", err, ErrBusy)
	}
}

func TestAuthenticateStopsWaitingWhenTheRequestEnds(t *testing.T) {
	authenticator, _ := newTestAuthenticator(unknownUser())
	for range cap(authenticator.slots) {
		authenticator.slots <- struct{}{}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := authenticator.Authenticate(ctx, "nobody@example.com", "any password")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want %v", err, context.Canceled)
	}
}

func TestAuthenticatorLimitsItsMemory(t *testing.T) {
	authenticator, _ := newTestAuthenticator(unknownUser())

	// 19 MiB for each check. The server has 512 MB for everything.
	if megabytes := cap(authenticator.slots) * memoryKiB / 1024; megabytes > 128 {
		t.Errorf("password checks can use %d MiB at once, want at most 128", megabytes)
	}
}

func TestAuthenticateLocksAfterFiveWrongPasswords(t *testing.T) {
	users := knownUser()
	authenticator, advance := newTestAuthenticator(users)

	for attempt := 1; attempt <= 4; attempt++ {
		_, err := authenticator.Authenticate(t.Context(), "someone@example.com", "wrong password")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("wrong password %d: error = %v, want %v", attempt, err, ErrInvalidCredentials)
		}
	}
	if users.user.LockedUntil.Valid {
		t.Fatal("the account was locked after four wrong passwords")
	}

	_, err := authenticator.Authenticate(t.Context(), "someone@example.com", "wrong password")
	var locked *LockedError
	if !errors.As(err, &locked) {
		t.Fatalf("wrong password 5: error = %v, want a LockedError", err)
	}
	if locked.UserID != 7 || !locked.Until.Equal(noon.Add(15*time.Minute)) {
		t.Errorf("lock = user %d until %v, want user 7 until 12:15", locked.UserID, locked.Until)
	}
	if users.user.LockedUntil.String != "2026-11-15T12:15:00Z" {
		t.Errorf("stored lock = %q, want 2026-11-15T12:15:00Z", users.user.LockedUntil.String)
	}

	advance(14*time.Minute + 59*time.Second)
	if _, err := authenticator.Authenticate(t.Context(), "someone@example.com", "correct password"); !errors.As(err, &locked) {
		t.Errorf("correct password one second before the lock ends: error = %v, want a LockedError", err)
	}

	advance(time.Second)
	id, err := authenticator.Authenticate(t.Context(), "someone@example.com", "correct password")
	if err != nil || id != 7 {
		t.Errorf("correct password when the lock ends: user ID = %d, error = %v, want 7 and no error", id, err)
	}
	if users.user.LockedUntil.Valid || users.user.FailedLogins != 0 {
		t.Errorf("after signing in: lock = %v and failures = %d, want both cleared", users.user.LockedUntil, users.user.FailedLogins)
	}
}

// A locked account with a damaged hash can only fail this way if the password was still checked.
// Skipping the check would make a locked account answer faster than any other.
func TestAuthenticateChecksThePasswordOfALockedAccount(t *testing.T) {
	users := &fakeUsers{user: store.User{
		ID:           7,
		PasswordHash: "not a hash",
		LockedUntil:  sql.NullString{String: "2026-11-15T12:15:00Z", Valid: true},
	}}
	authenticator, _ := newTestAuthenticator(users)

	_, err := authenticator.Authenticate(t.Context(), "someone@example.com", "any password")

	if !errors.Is(err, ErrInvalidHash) {
		t.Errorf("error = %v, want %v: no password check was made for a locked account", err, ErrInvalidHash)
	}
}

func TestACorrectPasswordStartsTheCountAgain(t *testing.T) {
	users := knownUser()
	authenticator, _ := newTestAuthenticator(users)
	attempt := func(password string) error {
		_, err := authenticator.Authenticate(t.Context(), "someone@example.com", password)
		return err
	}

	for range 4 {
		_ = attempt("wrong password")
	}
	if err := attempt("correct password"); err != nil {
		t.Fatalf("correct password after four wrong ones: %v", err)
	}
	for range 4 {
		_ = attempt("wrong password")
	}

	if err := attempt("correct password"); err != nil {
		t.Errorf("eight wrong passwords with a correct one in the middle locked the account: %v", err)
	}
}

func TestAnUnknownEmailIsNeverCounted(t *testing.T) {
	users := unknownUser()
	authenticator, _ := newTestAuthenticator(users)

	for range 6 {
		if _, err := authenticator.Authenticate(t.Context(), "nobody@example.com", "any password"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("error = %v, want %v", err, ErrInvalidCredentials)
		}
	}

	if users.user.FailedLogins != 0 || users.user.LockedUntil.Valid {
		t.Error("a failure was counted or a lock set for an address that is not an account")
	}
}

func TestAuthenticateRefusesWhenTheLockCannotBeRead(t *testing.T) {
	users := knownUser()
	users.user.LockedUntil = sql.NullString{String: "some time next week", Valid: true}
	authenticator, _ := newTestAuthenticator(users)

	id, err := authenticator.Authenticate(t.Context(), "someone@example.com", "correct password")

	if err == nil || id != 0 {
		t.Errorf("user ID = %d and error = %v, want a refusal: the lock could not be read", id, err)
	}
}
