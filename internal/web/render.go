package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gabrielg2020/mock-cpms-webhook-manager/internal/store"
)

// templateData is what every template receives. Page holds the values for one page.
type templateData struct {
	StaticPath string
	User       *store.GetSignedInUserRow
	Page       any
}

// parseTemplates builds one template per page, each made of the shared layout, the partials and the page's blocks.
func parseTemplates(files fs.FS) (map[string]*template.Template, error) {
	pages, err := fs.Glob(files, "templates/pages/*.html")
	if err != nil {
		return nil, err
	}
	shared := []string{"templates/layouts/base.html"}
	partials, err := fs.Glob(files, "templates/partials/*.html")
	if err != nil {
		return nil, err
	}
	shared = append(shared, partials...)

	templates := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		parsed, err := template.ParseFS(files, append(shared, page)...)
		if err != nil {
			return nil, err
		}

		templates[strings.TrimSuffix(path.Base(page), ".html")] = parsed
	}
	return templates, nil
}

func (app *Application) render(w http.ResponseWriter, r *http.Request, status int, page string, pageValues any) {
	rendered, err := app.renderToBuffer(r, page, pageValues)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	writePage(w, status, rendered)
}

// renderToBuffer builds the whole page in memory, so a template error becomes a clean 500 and not half a page.
func (app *Application) renderToBuffer(r *http.Request, page string, pageValues any) (*bytes.Buffer, error) {
	parsed, ok := app.templates[page]
	if !ok {
		return nil, fmt.Errorf("template %q does not exist", page)
	}

	data := templateData{StaticPath: app.staticPath, Page: pageValues}
	if user, signedIn := userFrom(r.Context()); signedIn {
		data.User = &user
	}

	var buffer bytes.Buffer
	if err := parsed.ExecuteTemplate(&buffer, "base.html", data); err != nil {
		return nil, err
	}
	return &buffer, nil
}

type problemPage struct {
	Heading string
	Detail  string
}

// problem is for a request the app refuses for a reason the user can do something about.
func (app *Application) problem(w http.ResponseWriter, r *http.Request, status int, heading, detail string) {
	app.render(w, r, status, "problem", problemPage{Heading: heading, Detail: detail})
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

	rendered, err := app.renderToBuffer(r, "server-error", serverErrorPage{Reference: requestID})
	if err != nil {
		// The error page itself failed, so send plain text. Calling render here could loop forever.
		app.logger.Error("failed to render the server error page", "request_id", requestID, "error", err)
		http.Error(w, "Sorry, there is a problem with the service. Try again later.", http.StatusInternalServerError)
		return
	}
	writePage(w, http.StatusInternalServerError, rendered)
}
