package session

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/database"
)

var noon = time.Date(2026, time.November, 15, 12, 0, 0, 0, time.UTC)

// newTestStore returns a store on an empty database whose clock always says noon.
func newTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := database.Open(t.Context(), filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	store := NewStore(db)
	store.now = func() time.Time { return noon }
	return store, db
}

func countSessions(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT count(*) FROM sessions").Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return count
}

func TestStoreSavesFindsAndDeletes(t *testing.T) {
	store, db := newTestStore(t)

	if _, found, err := store.FindCtx(t.Context(), "token"); err != nil || found {
		t.Fatalf("before saving: found = %t, error = %v, want not found", found, err)
	}

	if err := store.CommitCtx(t.Context(), "token", []byte("first"), noon.Add(time.Minute)); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := store.CommitCtx(t.Context(), "token", []byte("second"), noon.Add(time.Hour)); err != nil {
		t.Fatalf("save again: %v", err)
	}
	data, found, err := store.FindCtx(t.Context(), "token")
	if err != nil || !found || string(data) != "second" {
		t.Errorf("after saving twice: data = %q, found = %t, error = %v, want the second save", data, found, err)
	}
	if got := countSessions(t, db); got != 1 {
		t.Errorf("rows after saving the same token twice = %d, want 1", got)
	}

	if err := store.DeleteCtx(t.Context(), "token"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, found, _ := store.FindCtx(t.Context(), "token"); found {
		t.Error("the session was found after it was deleted")
	}
	if err := store.DeleteCtx(t.Context(), "token"); err != nil {
		t.Errorf("deleting a session that is not there: %v, want no error", err)
	}
}

func TestStoreDoesNotFindAnExpiredSession(t *testing.T) {
	store, _ := newTestStore(t)

	testCases := []struct {
		name      string
		expiry    time.Time
		wantFound bool
	}{
		{name: "expires in a second", expiry: noon.Add(time.Second), wantFound: true},
		{name: "expires now", expiry: noon, wantFound: false},
		{name: "expired a second ago", expiry: noon.Add(-time.Second), wantFound: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := store.CommitCtx(t.Context(), testCase.name, []byte("data"), testCase.expiry); err != nil {
				t.Fatalf("save: %v", err)
			}

			_, found, err := store.FindCtx(t.Context(), testCase.name)

			if err != nil {
				t.Fatalf("find: %v", err)
			}
			if found != testCase.wantFound {
				t.Errorf("found = %t, want %t", found, testCase.wantFound)
			}
		})
	}
}

func TestDeleteExpiredKeepsLiveSessions(t *testing.T) {
	store, db := newTestStore(t)
	for token, expiry := range map[string]time.Time{
		"live":    noon.Add(time.Minute),
		"expired": noon.Add(-time.Minute),
		"ancient": noon.Add(-24 * time.Hour),
	} {
		if err := store.CommitCtx(t.Context(), token, []byte("data"), expiry); err != nil {
			t.Fatalf("save %s: %v", token, err)
		}
	}

	if err := store.DeleteExpired(t.Context()); err != nil {
		t.Fatalf("delete expired: %v", err)
	}

	if got := countSessions(t, db); got != 1 {
		t.Errorf("rows left = %d, want 1", got)
	}
	if _, found, _ := store.FindCtx(t.Context(), "live"); !found {
		t.Error("the live session was deleted")
	}
}

func TestStoreWorksWithoutAContext(t *testing.T) {
	store, _ := newTestStore(t)

	if err := store.Commit("token", []byte("data"), noon.Add(time.Minute)); err != nil {
		t.Fatalf("save: %v", err)
	}
	if data, found, err := store.Find("token"); err != nil || !found || string(data) != "data" {
		t.Errorf("find: data = %q, found = %t, error = %v", data, found, err)
	}
	if err := store.Delete("token"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, found, _ := store.Find("token"); found {
		t.Error("the session was found after it was deleted")
	}
}
