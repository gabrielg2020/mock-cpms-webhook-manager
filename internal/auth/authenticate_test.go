package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/store"
)

type fakeUsers struct {
	user store.User
	err  error
}

func (fake fakeUsers) GetUserByEmail(context.Context, string) (store.User, error) {
	return fake.user, fake.err
}

func TestAuthenticate(t *testing.T) {
	known := fakeUsers{user: store.User{ID: 7, PasswordHash: HashPassword("correct password")}}
	unknown := fakeUsers{err: sql.ErrNoRows}
	databaseDown := errors.New("database is closed")

	testCases := []struct {
		name     string
		users    fakeUsers
		password string
		wantID   int64
		wantErr  error
	}{
		{name: "correct password", users: known, password: "correct password", wantID: 7},
		{name: "wrong password", users: known, password: "wrong password", wantErr: ErrInvalidCredentials},
		{name: "unknown email address", users: unknown, password: "correct password", wantErr: ErrInvalidCredentials},
		{name: "database failure", users: fakeUsers{err: databaseDown}, password: "correct password", wantErr: databaseDown},
		{
			name:     "stored hash is damaged",
			users:    fakeUsers{user: store.User{ID: 7, PasswordHash: "not a hash"}},
			password: "correct password",
			wantErr:  ErrInvalidHash,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			authenticator := NewAuthenticator(testCase.users)

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
	authenticator := NewAuthenticator(fakeUsers{err: sql.ErrNoRows})
	authenticator.dummyHash = "not a hash"

	_, err := authenticator.Authenticate(t.Context(), "nobody@example.com", "any password")

	if !errors.Is(err, ErrInvalidHash) {
		t.Errorf("error = %v, want %v: no password check was made for an unknown email address", err, ErrInvalidHash)
	}
}

func TestAuthenticateRefusesWhenBusy(t *testing.T) {
	authenticator := NewAuthenticator(fakeUsers{err: sql.ErrNoRows})
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
	authenticator := NewAuthenticator(fakeUsers{err: sql.ErrNoRows})
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
	authenticator := NewAuthenticator(fakeUsers{err: sql.ErrNoRows})

	// 19 MiB for each check. The server has 512 MB for everything.
	if megabytes := cap(authenticator.slots) * memoryKiB / 1024; megabytes > 128 {
		t.Errorf("password checks can use %d MiB at once, want at most 128", megabytes)
	}
}
