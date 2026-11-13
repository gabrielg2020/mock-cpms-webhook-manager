package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// templateData is what every template receives. Page holds the values for one page.
type templateData struct {
	StaticPath string
	Page       any
}

// parseTemplates builds one template per page, each made of the shared layout and the page's blocks.
func parseTemplates(files fs.FS) (map[string]*template.Template, error) {
	pages, err := fs.Glob(files, "templates/pages/*.html")
	if err != nil {
		return nil, err
	}

	templates := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		parsed, err := template.ParseFS(files, "templates/layouts/base.html", page)
		if err != nil {
			return nil, err
		}

		templates[strings.TrimSuffix(path.Base(page), ".html")] = parsed
	}
	return templates, nil
}

func (app *Application) render(w http.ResponseWriter, r *http.Request, status int, page string, pageValues any) {
	parsed, ok := app.templates[page]
	if !ok {
		app.serverError(w, r, fmt.Errorf("template %q does not exist", page))
		return
	}

	// Render into a buffer first, so a template error becomes a clean 500 and not half a page.
	var buffer bytes.Buffer
	data := templateData{StaticPath: app.staticPath, Page: pageValues}
	if err := parsed.ExecuteTemplate(&buffer, "base.html", data); err != nil {
		app.serverError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buffer.WriteTo(w)
}

func (app *Application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.Error("request failed", "error", err, "method", r.Method, "path", r.URL.Path)
	http.Error(w, "Sorry, there is a problem with the service. Try again later.", http.StatusInternalServerError)
}
