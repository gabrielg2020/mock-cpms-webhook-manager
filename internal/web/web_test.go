package web

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func healthy(context.Context) error { return nil }

func newTestApplication(t *testing.T, healthCheck HealthCheck) *Application {
	t.Helper()
	app, err := New(slog.New(slog.DiscardHandler), healthCheck)
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	return app
}

func get(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
	return recorder
}

func TestPages(t *testing.T) {
	routes := newTestApplication(t, healthy).Routes()

	testCases := []struct {
		name       string
		target     string
		wantStatus int
		wantText   string
	}{
		{name: "home page", target: "/", wantStatus: http.StatusOK, wantText: "Manage your team's CPMS webhooks"},
		{name: "unknown address", target: "/no-such-page", wantStatus: http.StatusNotFound, wantText: "Page not found"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			response := get(t, routes, testCase.target)
			body := response.Body.String()

			if response.Code != testCase.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, testCase.wantStatus)
			}
			if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
				t.Errorf("Content-Type = %q, want text/html; charset=utf-8", got)
			}
			for _, want := range []string{testCase.wantText, "Mock CPMS Webhook Manager", "not a government service"} {
				if !strings.Contains(body, want) {
					t.Errorf("page does not contain %q", want)
				}
			}
			// The Content Security Policy blocks inline scripts and styles, so a page must not rely on them.
			for _, inline := range []string{"<script>", "<style", ` style="`} {
				if strings.Contains(body, inline) {
					t.Errorf("page contains %q, which the Content Security Policy would block", inline)
				}
			}
		})
	}
}

func TestSecurityHeadersAreOnEveryResponse(t *testing.T) {
	app := newTestApplication(t, healthy)
	routes := app.Routes()

	wantHeaders := map[string]string{
		"Content-Security-Policy": contentSecurityPolicy,
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "same-origin",
		"Permissions-Policy":      "camera=(), geolocation=(), microphone=(), payment=()",
	}

	for _, target := range []string{"/", "/no-such-page", "/healthz", app.staticPath + "/app.css"} {
		t.Run(target, func(t *testing.T) {
			response := get(t, routes, target)
			for name, want := range wantHeaders {
				if got := response.Header().Get(name); got != want {
					t.Errorf("%s = %q, want %q", name, got, want)
				}
			}
		})
	}
}

// The expected policy is written out again here on purpose. Comparing the header with the
// contentSecurityPolicy constant would pass even if the constant had a mistake in it.
func TestContentSecurityPolicyHasExactlyTheIntendedDirectives(t *testing.T) {
	want := map[string]string{
		"default-src":     "'self'",
		"object-src":      "'none'",
		"base-uri":        "'none'",
		"form-action":     "'self'",
		"frame-ancestors": "'none'",
	}

	got := map[string]string{}
	for directive := range strings.SplitSeq(contentSecurityPolicy, ";") {
		name, value, _ := strings.Cut(strings.TrimSpace(directive), " ")
		got[name] = value
	}

	if !maps.Equal(got, want) {
		t.Errorf("Content Security Policy directives = %v, want %v", got, want)
	}
}

func TestStaticFiles(t *testing.T) {
	app := newTestApplication(t, healthy)
	routes := app.Routes()

	testCases := []struct {
		name            string
		target          string
		wantStatus      int
		wantContentType string
		wantCached      bool
	}{
		{name: "stylesheet", target: app.staticPath + "/app.css", wantStatus: http.StatusOK, wantContentType: "text/css; charset=utf-8", wantCached: true},
		{name: "script", target: app.staticPath + "/app.js", wantStatus: http.StatusOK, wantContentType: "text/javascript; charset=utf-8", wantCached: true},
		{name: "GOV.UK Frontend script", target: app.staticPath + "/govuk-frontend.min.js", wantStatus: http.StatusOK, wantContentType: "text/javascript; charset=utf-8", wantCached: true},
		{name: "missing file", target: app.staticPath + "/missing.css", wantStatus: http.StatusNotFound},
		{name: "folder listing", target: app.staticPath + "/", wantStatus: http.StatusNotFound},
		{name: "another version", target: "/static/000000000000/app.css", wantStatus: http.StatusNotFound},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			response := get(t, routes, testCase.target)

			if response.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, testCase.wantStatus)
			}
			if testCase.wantContentType != "" {
				if got := response.Header().Get("Content-Type"); got != testCase.wantContentType {
					t.Errorf("Content-Type = %q, want %q", got, testCase.wantContentType)
				}
			}
			cached := response.Header().Get("Cache-Control") == "public, max-age=31536000, immutable"
			if cached != testCase.wantCached {
				t.Errorf("Cache-Control = %q, want cached = %t", response.Header().Get("Cache-Control"), testCase.wantCached)
			}
		})
	}
}

func TestPagesLinkToStaticFilesThatExist(t *testing.T) {
	app := newTestApplication(t, healthy)
	routes := app.Routes()
	page := get(t, routes, "/").Body.String()

	for _, file := range []string{"app.css", "app.js", "js-enabled.js", "favicon.svg"} {
		address := app.staticPath + "/" + file
		if !strings.Contains(page, address) {
			t.Errorf("page does not link to %s", address)
		}
		if response := get(t, routes, address); response.Code != http.StatusOK {
			t.Errorf("%s returned %d, want 200", address, response.Code)
		}
	}
}

func TestHealthz(t *testing.T) {
	testCases := []struct {
		name        string
		healthCheck HealthCheck
		wantStatus  int
	}{
		{name: "healthy", healthCheck: healthy, wantStatus: http.StatusNoContent},
		{
			name:        "database unavailable",
			healthCheck: func(context.Context) error { return errors.New("database is closed") },
			wantStatus:  http.StatusServiceUnavailable,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			response := get(t, newTestApplication(t, testCase.healthCheck).Routes(), "/healthz")
			if response.Code != testCase.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, testCase.wantStatus)
			}
		})
	}
}

func TestRenderFailureDoesNotLeakDetails(t *testing.T) {
	app := newTestApplication(t, healthy)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)

	app.render(recorder, request, http.StatusOK, "no-such-template", nil)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}
	if body := recorder.Body.String(); strings.Contains(body, "no-such-template") {
		t.Errorf("response reveals internal details: %q", body)
	}
}

func TestParseTemplatesRejectsABrokenPage(t *testing.T) {
	files := fstest.MapFS{
		"templates/layouts/base.html": {Data: []byte(`{{ template "content" . }}`)},
		"templates/pages/broken.html": {Data: []byte(`{{ define "content" }}{{ .Unclosed {{ end }}`)},
	}

	if _, err := parseTemplates(files); err == nil {
		t.Fatal("expected an error for a template that does not parse, got nil")
	}
}

func TestAssetVersionChangesWithTheFiles(t *testing.T) {
	version := func(files fstest.MapFS) string {
		t.Helper()
		got, err := assetVersion(files)
		if err != nil {
			t.Fatalf("asset version: %v", err)
		}
		return got
	}

	original := version(fstest.MapFS{"app.css": {Data: []byte("body { color: black }")}})
	unchanged := version(fstest.MapFS{"app.css": {Data: []byte("body { color: black }")}})
	edited := version(fstest.MapFS{"app.css": {Data: []byte("body { color: blue }")}})
	renamed := version(fstest.MapFS{"site.css": {Data: []byte("body { color: black }")}})

	if original != unchanged {
		t.Errorf("same files gave different versions: %s and %s", original, unchanged)
	}
	if original == edited {
		t.Error("editing a file did not change the version")
	}
	if original == renamed {
		t.Error("renaming a file did not change the version")
	}
}
