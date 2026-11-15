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
	maxFailedSignIns    = 5
	lockDuration        = 15 * time.Minute
)

// LockedError is return while an account locked
type LockedError struct {
	UserID int64
	Until  time.Time
}

func (e *LockedError) Error() string {
	return fmt.Sprintf("user %d is locked until %s", e.UserID, e.Until.Format(time.RFC3339))
}

// ErrInvalidCredentials is returned for an unknown email address and for a wrong password alike,
// so the caller cannot tell someone which of the two it was.
var ErrInvalidCredentials = errors.New("email address or password is not correct")

// ErrBusy is returned when too many passwords are already being checked
var ErrBusy = errors.New("too many passwords are being checked")

type UserStore interface {
	GetUserByEmail(ctx context.Context, email string) (store.User, error)
	CountFailedSignIn(ctx context.Context, id int64) (int64, error)
	LockUser(ctx context.Context, arg store.LockUserParams) error
	ClearFailedSignIns(ctx context.Context, id int64) error
}

type Authenticator struct {
	users     UserStore
	slots     chan struct{}
	maxWait   time.Duration
	dummyHash string
	now       func() time.Time
}

func NewAuthenticator(users UserStore) *Authenticator {
	return &Authenticator{
		users:   users,
		slots:   make(chan struct{}, maxConcurrentChecks),
		maxWait: maxWaitForCheck,
		// Checked when the email address is unknown, so that attempt takes as long as a real one
		dummyHash: HashPassword(rand.Text()),
		now:       time.Now,
	}
}

// Authenticate returns the ID of the user with this email address and password.
func (a *Authenticator) Authenticate(ctx context.Context, email, password string) (int64, error) {
	user, err := a.users.GetUserByEmail(ctx, email)
	known := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("find user by email: %w", err)
	}

	var lockedUntil time.Time
	if known && user.LockedUntil.Valid {
		lockedUntil, err = time.Parse(time.RFC3339, user.LockedUntil.String)
		if err != nil {
			return 0, fmt.Errorf("read when user %d is locked until: %w", user.ID, err)
		}
	}
	locked := a.now().Before(lockedUntil)

	hash := a.dummyHash
	if known {
		hash = user.PasswordHash
	}

	// The password is checked even when the answer doesn't matter, so a time locked
	// and non-time-locked account take as long as each other
	if err := a.waitForSlot(ctx); err != nil {
		return 0, err
	}
	matches, err := VerifyPassword(password, hash)
	<-a.slots
	if err != nil {
		return 0, fmt.Errorf("verify password of user %d: %w", user.ID, err)
	}

	switch {
	case !known:
		return 0, ErrInvalidCredentials
	case locked:
		return 0, &LockedError{UserID: user.ID, Until: lockedUntil}
	case !matches:
		return 0, a.recordFailure(ctx, user.ID)
	}

	if user.FailedLogins > 0 || user.LockedUntil.Valid {
		if err := a.users.ClearFailedSignIns(ctx, user.ID); err != nil {
			return 0, fmt.Errorf("clear failed sign-ins of user %d: %w", user.ID, err)
		}
	}
	return user.ID, nil
}

// recordFailure counts a wrong password and locks the accoutn at the limit.
// It returns the error the attempt should end with.
func (a *Authenticator) recordFailure(ctx context.Context, userID int64) error {
	// The database adds one in a single statement, so two attempts at the same moment
	// cannot both read the old count
	failures, err := a.users.CountFailedSignIn(ctx, userID)
	if err != nil {
		return fmt.Errorf("count failed sign-in of user %d: %w", userID, err)
	}
	if failures < maxFailedSignIns {
		return ErrInvalidCredentials
	}

	until := a.now().Add(lockDuration).UTC()
	lock := store.LockUserParams{
		ID:          userID,
		LockedUntil: sql.NullString{String: until.Format(time.RFC3339), Valid: true},
	}
	if err := a.users.LockUser(ctx, lock); err != nil {
		return fmt.Errorf("lock user %d: %w", userID, err)
	}
	return &LockedError{UserID: userID, Until: until}
}

// waitForSlot limits how many passwords are checked at once. Without a limit, a burst of
// sign-in attempts could use all the server's memory.
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
