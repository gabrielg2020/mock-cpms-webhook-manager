package web

import "net/http"

func (app *Application) home(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, http.StatusOK, "home", nil)
}

func (app *Application) notFound(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, http.StatusNotFound, "not-found", nil)
}

func (app *Application) healthz(w http.ResponseWriter, r *http.Request) {
	if err := app.healthCheck(r.Context()); err != nil {
		app.logger.Error("health check failed", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
