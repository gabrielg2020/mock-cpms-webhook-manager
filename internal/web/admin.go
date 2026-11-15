package web

import "net/http"

func (app *Application) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := userFrom(r.Context())
		if !user.IsAdmin() {
			app.logger.Warn("access denied",
				"request_id", requestIDFrom(r.Context()),
				"user_id", user.ID,
				"method", r.Method,
				"path", r.URL.Path,
			)
			app.problem(w, r, http.StatusForbidden, "You do not have permission to use this page", "If you think you should have access, ask an administrator.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (app *Application) adminHome(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, http.StatusOK, "admin", nil)
}
