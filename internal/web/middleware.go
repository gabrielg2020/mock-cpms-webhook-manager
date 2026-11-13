package web

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type contextKey string

const requestIDKey contextKey = "requestID"

// requestIDFrom returns the ID given to this request, or "" if there is none.
func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// requestID gives every request a new random ID. An ID sent by the client is never used,
// because the sender could then choose what appears in the logs.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := rand.Text()
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// logRequests writes one line for every request. It leaves out the query string, headers and body,
// which are the places a password or token could appear.
func (app *Application) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		response := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// Deferred, so the line is still written if the handler panics.
		defer func() {
			app.logger.Info("request completed",
				"request_id", requestIDFrom(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", response.Status(),
				"bytes", response.BytesWritten(),
				"duration_ms", float64(time.Since(start).Microseconds())/1000,
			)
		}()

		next.ServeHTTP(response, r)
	})
}

// recoverPanics turns a panic in a handler into a logged error and the 500 page.
// Without it Go closes the connection, and the user gets the proxy's error with no reference.
func (app *Application) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			// net/http panics with this value on purpose to abort a response, so it is passed on.
			if err, ok := recovered.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(recovered)
			}

			app.logger.Error("panic recovered",
				"request_id", requestIDFrom(r.Context()),
				"panic", fmt.Sprint(recovered),
				"stack", string(debug.Stack()),
				"method", r.Method,
				"path", r.URL.Path,
			)
			app.sendServerErrorPage(w, r)
		}()

		next.ServeHTTP(w, r)
	})
}
