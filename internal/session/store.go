package session

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/store"
)

// Store is where the session manager saves and finds sessions
// It never sess a real session token, the manager gives it a SHA-256 of the token.
type Store struct {
	queries *store.Queries
	now     func() time.Time
}

func NewStore(db store.DBTX) *Store {
	return &Store{queries: store.New(db), now: time.Now}
}

func unixSeconds(moment time.Time) float64 {
	return float64(moment.UnixMilli()) / 1000
}

// FindCtx reports found as false for a session that doesn't exist or expired
func (s *Store) FindCtx(ctx context.Context, token string) (data []byte, found bool, err error) {
	data, err = s.queries.GetSession(ctx, store.GetSessionParams{Token: token, Expiry: unixSeconds(s.now())})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (s *Store) CommitCtx(ctx context.Context, token string, data []byte, expiry time.Time) error {
	return s.queries.SaveSession(ctx, store.SaveSessionParams{Token: token, Data: data, Expiry: unixSeconds(expiry)})
}

func (s *Store) DeleteCtx(ctx context.Context, token string) error {
	return s.queries.DeleteSession(ctx, token)
}

// DeleteExpired removes sessions that can no longer be used, so the table doesn't grow indefinatly
func (s *Store) DeleteExpired(ctx context.Context) error {
	return s.queries.DeleteExpiredSessions(ctx, unixSeconds(s.now()))
}

// The session manager's interface also ask for these three. It calls the versions above,
// which take the request's context, whenever a store has them.
func (s *Store) Find(token string) ([]byte, bool, error) {
	return s.FindCtx(context.Background(), token)
}

func (s *Store) Commit(token string, data []byte, expiry time.Time) error {
	return s.CommitCtx(context.Background(), token, data, expiry)
}

func (s *Store) Delete(token string) error {
	return s.DeleteCtx(context.Background(), token)
}
