// Package web serves the HTML pages and static assets.
package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed static templates
var embedded embed.FS

// HealthCheck reports whether the app's dependencies are working.
type HealthCheck func(ctx context.Context) error

type Application struct {
	logger      *slog.Logger
	healthCheck HealthCheck
	templates   map[string]*template.Template
	static      fs.FS
	staticPath  string
}

func New(logger *slog.Logger, healthCheck HealthCheck) (*Application, error) {
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

	return &Application{
		logger:      logger,
		healthCheck: healthCheck,
		templates:   templates,
		static:      static,
		staticPath:  "/static/" + version,
	}, nil
}

func (app *Application) Routes() http.Handler {
	router := chi.NewRouter()
	// The first one listed is the outermost: it runs first and wraps all the others.
	router.Use(requestID)
	router.Use(securityHeaders)
	router.Use(app.logRequests)
	router.Use(app.recoverPanics)
	router.NotFound(app.notFound)

	router.Get("/", app.home)
	router.Get("/healthz", app.healthz)
	router.Method(http.MethodGet, app.staticPath+"/*", app.staticFiles())

	return router
}
