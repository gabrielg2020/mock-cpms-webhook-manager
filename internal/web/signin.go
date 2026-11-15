package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/auth"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/validation"
)

// A sign in form is two short fields. The limit stops a huge body being read or hashed
const maxFormBytes = 4096

type signInPage struct {
	Email  string
	Errors validation.Errors
}

func (app *Application) signInForm(w http.ResponseWriter, r *http.Request) {
	if _, signedIn := userFrom(r.Context()); signedIn {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	app.render(w, r, http.StatusOK, "sign-in", signInPage{})
}

func (app *Application) signIn(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		app.problem(w, r, http.StatusBadRequest, "Sorry, there is a problem with what you sent", "Go back and try again.")
		return
	}

	page := signInPage{Email: strings.TrimSpace(r.PostForm.Get("email"))}
	password := r.PostForm.Get("password")
	if page.Email == "" {
		page.Errors.Add("email", "Enter your email address")
	}
	if password == "" {
		page.Errors.Add("password", "Enter your password")
	}
	if page.Errors.Any() {
		app.render(w, r, http.StatusUnprocessableEntity, "sign-in", page)
		return
	}

	userID, err := app.authenticator.Authenticate(r.Context(), page.Email, password)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		app.logger.Warn("sign-in failed", "request_id", requestIDFrom(r.Context()))
		page.Errors.Add("email", "Enter a correct email address and password")
		app.render(w, r, http.StatusUnprocessableEntity, "sign-in", page)
		return
	case errors.Is(err, auth.ErrBusy):
		w.Header().Set("Retry-After", "5")
		app.problem(w, r, http.StatusServiceUnavailable, "Sorry, the service is busy", "Try again in a few moments.")
		return
	case errors.Is(err, context.Canceled):
		return
	case err != nil:
		app.serverError(w, r, err)
		return
	}

	// A new token at sign-in makes any token from before it worthless, which stops
	// an attacker who planted or saw the old one (session fixation).
	if err := app.sessions.RenewToken(r.Context()); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.sessions.Put(r.Context(), sessionUserID, userID)

	if err := app.sessionStore.DeleteExpired(r.Context()); err != nil {
		app.logger.Error("failed to delete expired sessions", "request_id", requestIDFrom(r.Context()), "error", err)
	}

	app.logger.Info("signed in", "request_id", requestIDFrom(r.Context()), "user_id", userID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *Application) signOutForm(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, http.StatusOK, "sign-out", nil)
}

func (app *Application) signOut(w http.ResponseWriter, r *http.Request) {
	user, _ := userFrom(r.Context())
	if err := app.sessions.Destroy(r.Context()); err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logger.Info("signed out", "request_id", requestIDFrom(r.Context()), "user_id", user.ID)
	http.Redirect(w, r, "/sign-in", http.StatusSeeOther)
}

func (app *Application) crossOriginDenied(w http.ResponseWriter, r *http.Request) {
	app.logger.Warn("cross-origin request refused", "request_id", requestIDFrom(r.Context()), "method", r.Method, "path", r.URL.Path)
	app.problem(w, r, http.StatusForbidden, "Sorry, there is a problem", "Your request did not come from this site, so it was not carried out. Go back to the page and try again.")
}
