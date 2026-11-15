// Package web serves the HTML pages and static assets.
package web

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/auth"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/ratelimit"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/session"
	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed static templates
var embedded embed.FS

const signInAttemptsPerMinute = 10

// HealthCheck reports whether the app's dependencies are working.
type HealthCheck func(ctx context.Context) error

type Options struct {
	Logger        *slog.Logger
	HealthCheck   HealthCheck
	Database      *sql.DB
	SecureCookies bool
}

type Application struct {
	logger        *slog.Logger
	healthCheck   HealthCheck
	templates     map[string]*template.Template
	static        fs.FS
	staticPath    string
	queries       *store.Queries
	authenticator *auth.Authenticator
	sessionStore  *session.Store
	sessions      *scs.SessionManager
	crossOrigin   *http.CrossOriginProtection
	signInLimiter *ratelimit.Limiter
}

func New(options Options) (*Application, error) {
	templates, err := parseTemplates(embedded)
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	static, err := fs.Sub(embedded, "static")
	if err != nil {
		return nil, fmt.Errorf("read embedded static files: %w", err)
	}
	version, err := assetVersion(static)
	if err != nil {
		return nil, fmt.Errorf("hash static files: %w", err)
	}

	queries := store.New(options.Database)
	app := &Application{
		logger:        options.Logger,
		healthCheck:   options.HealthCheck,
		templates:     templates,
		static:        static,
		staticPath:    "/static/" + version,
		queries:       queries,
		authenticator: auth.NewAuthenticator(queries),
		sessionStore:  session.NewStore(options.Database),
		crossOrigin:   http.NewCrossOriginProtection(),
		signInLimiter: ratelimit.New(signInAttemptsPerMinute, time.Minute),
	}
	app.sessions = newSessionManager(app.sessionStore, options.SecureCookies, app.serverError)
	app.crossOrigin.SetDenyHandler(http.HandlerFunc(app.crossOriginDenied))
	return app, nil
}

func (app *Application) Routes() http.Handler {
	router := chi.NewRouter()
	// The first one listed is the outermost: it runs first and wraps all the others.
	router.Use(requestID)
	router.Use(rememberClientIP)
	router.Use(securityHeaders)
	router.Use(app.logRequests)
	router.Use(app.recoverPanics)
	router.Use(app.crossOrigin.Handler)
	router.Use(middleware.GetHead)

	router.Get("/healthz", app.healthz)
	router.Method(http.MethodGet, app.staticPath+"/*", app.staticFiles())

	// Pages need to know who is signed in. The health check and static files do not,
	// so they are registered above and skip the session's database read and write.
	pages := chi.Chain(noStore, app.sessions.LoadAndSave, app.loadUser)
	router.NotFound(pages.HandlerFunc(app.notFound).ServeHTTP)

	router.Group(func(public chi.Router) {
		public.Use(pages...)
		public.Get("/sign-in", app.signInForm)
		public.With(app.limitSignIns).Post("/sign-in", app.signIn)

		// Every page registered in this group needs a signed-in user.
		public.Group(func(private chi.Router) {
			private.Use(app.requireUser)
			private.Get("/", app.home)
			private.Get("/sign-out", app.signOutForm)
			private.Post("/sign-out", app.signOut)
		})
	})

	return router
}
