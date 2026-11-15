package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/store"
)

// Each check uses 19 MiB, so four at once is at most 76 MiB of the server's 512 MB.
const (
	maxConcurrentChecks = 4
	maxWaitForCheck     = 3 * time.Second
)

// ErrInvalidCredentials is returned for an unkown email address and for a wrong password alike,
// so the called cannot tell someone which of the two it was.
var ErrInvalidCredentials = errors.New("email address or password is not correct")

// ErrBusy is returned when too many passwords are already being checked
var ErrBusy = errors.New("too many passwords are being checked")

type UserFinder interface {
	GetUserByEmail(ctx context.Context, email string) (store.User, error)
}

type Authenticator struct {
	users     UserFinder
	slots     chan struct{}
	maxWait   time.Duration
	dummyHash string
}

func NewAuthenticator(users UserFinder) *Authenticator {
	return &Authenticator{
		users:   users,
		slots:   make(chan struct{}, maxConcurrentChecks),
		maxWait: maxWaitForCheck,
		// Checked when the email address is unkown, so that attempt takes as long as a real one
		dummyHash: HashPassword(rand.Text()),
	}
}

// Authenticate returns the ID of the user with this email address and password.
func (a *Authenticator) Authenticate(ctx context.Context, email, password string) (int64, error) {
	user, err := a.users.GetUserByEmail(ctx, email)
	known := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("find user by email: %w", err)
	}

	hash := a.dummyHash
	if known {
		hash = user.PasswordHash
	}

	if err := a.waitForSlot(ctx); err != nil {
		return 0, err
	}
	matches, err := VerifyPassword(password, hash)
	<-a.slots
	if err != nil {
		return 0, fmt.Errorf("verify password of user %d: %w", user.ID, err)
	}

	if !known || !matches {
		return 0, ErrInvalidCredentials
	}
	return user.ID, nil
}

// waitForSlot limits how many passwords are checked at once. Without a limit, a burst of
// sign-in attempt could use all the server's memory.
func (a *Authenticator) waitForSlot(ctx context.Context) error {
	timer := time.NewTimer(a.maxWait)
	defer timer.Stop()

	select {
	case a.slots <- struct{}{}:
		return nil
	case <-timer.C:
		return ErrBusy
	case <-ctx.Done():
		return ctx.Err()
	}
}
