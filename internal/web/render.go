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
	rendered, err := app.renderToBuffer(page, pageValues)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writePage(w, status, rendered)
}

// renderToBuffer builds the whole page in memory, so a template error becomes a clean 500 and not half a page.
func (app *Application) renderToBuffer(page string, pageValues any) (*bytes.Buffer, error) {
	parsed, ok := app.templates[page]
	if !ok {
		return nil, fmt.Errorf("template %q does not exist", page)
	}

	var buffer bytes.Buffer
	data := templateData{StaticPath: app.staticPath, Page: pageValues}
	if err := parsed.ExecuteTemplate(&buffer, "base.html", data); err != nil {
		return nil, err
	}
	return &buffer, nil
}

func writePage(w http.ResponseWriter, status int, rendered *bytes.Buffer) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = rendered.WriteTo(w)
}

type serverErrorPage struct {
	Reference string
}

// serverError is for errors the user cannot fix. The log gets the detail and the page gets
// only the request ID, which is what links the two.
func (app *Application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.Error("request failed",
		"request_id", requestIDFrom(r.Context()),
		"error", err,
		"method", r.Method,
		"path", r.URL.Path,
	)
	app.sendServerErrorPage(w, r)
}

func (app *Application) sendServerErrorPage(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(r.Context())

	rendered, err := app.renderToBuffer("server-error", serverErrorPage{Reference: requestID})
	if err != nil {
		// The error page itself failed, so send plain text. Calling render here could loop forever.
		app.logger.Error("failed to render the server error page", "request_id", requestID, "error", err)
		http.Error(w, "Sorry, there is a problem with the service. Try again later.", http.StatusInternalServerError)
		return
	}
	writePage(w, http.StatusInternalServerError, rendered)
}
