package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/store"
)

const (
	sessionIdleTimeout = 30 * time.Minute
	sessionLifetime    = 8 * time.Hour
	sessionUserID      = "userID"
)

const userKey contextKey = "user"

func newSessionManager(sessionStore scs.Store, secure bool, onError func(http.ResponseWriter, *http.Request, error)) *scs.SessionManager {
	sessions := scs.New()
	sessions.Store = sessionStore
	sessions.HashTokenInStore = true
	sessions.IdleTimeout = sessionIdleTimeout
	sessions.Lifetime = sessionLifetime
	sessions.ErrorFunc = onError

	sessions.Cookie.Name = "session"
	sessions.Cookie.Path = "/"
	sessions.Cookie.HttpOnly = true
	sessions.Cookie.SameSite = http.SameSiteLaxMode
	sessions.Cookie.Secure = secure
	sessions.Cookie.Persist = false
	if secure {
		// A browser only accepts a __Host- cookie that is Secure, has Path=/ and no Domain,
		// so another site under the same domain cannot set or replace it.
		sessions.Cookie.Name = "__Host-session"
	}
	return sessions
}

// userFrom returns the signed-in user, or false if nobody is signed in.
func userFrom(ctx context.Context) (store.GetSignedInUserRow, bool) {
	user, ok := ctx.Value(userKey).(store.GetSignedInUserRow)
	return user, ok
}

// loadUser finds the user the session belongs to. It reads the database on every request,
// so an account that is removed or changed loses or changes its access on the next request,
// not when the session ends.
func (app *Application) loadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := app.sessions.GetInt64(r.Context(), sessionUserID)
		if userID == 0 {
			next.ServeHTTP(w, r)
			return
		}

		user, err := app.queries.GetSignedInUser(r.Context(), userID)
		if errors.Is(err, sql.ErrNoRows) {
			if err := app.sessions.Destroy(r.Context()); err != nil {
				app.serverError(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		if entry, ok := r.Context().Value(requestLogKey).(*requestLog); ok {
			entry.userID = userID
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	})
}

func (app *Application) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, signedIn := userFrom(r.Context()); !signedIn {
			http.Redirect(w, r, "/sign-in", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// noStore stops browsers and proxies keeping a copy of a page, the back button
// cannot show someone's pages after they sign out
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
