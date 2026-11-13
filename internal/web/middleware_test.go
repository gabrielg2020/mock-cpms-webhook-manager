package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newLoggedApplication is like newTestApplication, but keeps what the app logs so a test can read it.
func newLoggedApplication(t *testing.T, healthCheck HealthCheck) (*Application, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	app, err := New(slog.New(slog.NewJSONHandler(&logs, nil)), healthCheck)
	if err != nil {
		t.Fatalf("create application: %v", err)
	}
	return app, &logs
}

// logLines decodes every logged line and returns the ones with the given message.
func logLines(t *testing.T, logs *bytes.Buffer, message string) []map[string]any {
	t.Helper()
	var matching []map[string]any
	for line := range strings.Lines(logs.String()) {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		if entry["msg"] == message {
			matching = append(matching, entry)
		}
	}
	return matching
}

func TestEveryRequestGetsItsOwnID(t *testing.T) {
	app, logs := newLoggedApplication(t, healthy)
	routes := app.Routes()

	first := get(t, routes, "/").Header().Get("X-Request-Id")
	second := get(t, routes, "/").Header().Get("X-Request-Id")

	if first == "" || second == "" {
		t.Fatalf("X-Request-Id headers = %q and %q, want both set", first, second)
	}
	if first == second {
		t.Errorf("two requests were given the same ID %q", first)
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	request.Header.Set("X-Request-Id", "chosen-by-the-client")
	recorder := httptest.NewRecorder()
	routes.ServeHTTP(recorder, request)

	if got := recorder.Header().Get("X-Request-Id"); got == "chosen-by-the-client" {
		t.Error("the app used the request ID sent by the client")
	}
	if strings.Contains(logs.String(), "chosen-by-the-client") {
		t.Error("the request ID sent by the client reached the logs")
	}
}

func TestRequestsAreLogged(t *testing.T) {
	app, logs := newLoggedApplication(t, healthy)

	response := get(t, app.Routes(), "/no-such-page?token=secret")

	lines := logLines(t, logs, "request completed")
	if len(lines) != 1 {
		t.Fatalf("got %d request lines, want 1", len(lines))
	}
	line := lines[0]

	want := map[string]any{
		"level":      "INFO",
		"method":     "GET",
		"path":       "/no-such-page",
		"status":     float64(http.StatusNotFound),
		"request_id": response.Header().Get("X-Request-Id"),
	}
	for key, wantValue := range want {
		if line[key] != wantValue {
			t.Errorf("%s = %v, want %v", key, line[key], wantValue)
		}
	}
	if logged, _ := line["bytes"].(float64); int(logged) != response.Body.Len() {
		t.Errorf("bytes = %v, want %d", line["bytes"], response.Body.Len())
	}
	if _, ok := line["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms = %v, want a number", line["duration_ms"])
	}
	if strings.Contains(logs.String(), "secret") {
		t.Error("the query string reached the logs")
	}
}

func TestPanicBecomesTheServerErrorPage(t *testing.T) {
	app, logs := newLoggedApplication(t, func(context.Context) error { panic("boom: database password") })

	response := get(t, app.Routes(), "/healthz")
	body := response.Body.String()
	requestID := response.Header().Get("X-Request-Id")

	if response.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", response.Code)
	}
	if got := response.Header().Get("Content-Security-Policy"); got != contentSecurityPolicy {
		t.Errorf("Content-Security-Policy = %q, want it on the error page too", got)
	}
	for _, want := range []string{"Sorry, there is a problem with the service", requestID} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
	for _, detail := range []string{"boom", "password", "goroutine", "middleware.go"} {
		if strings.Contains(body, detail) {
			t.Errorf("page reveals %q", detail)
		}
	}

	panics := logLines(t, logs, "panic recovered")
	if len(panics) != 1 {
		t.Fatalf("got %d panic lines, want 1", len(panics))
	}
	if panics[0]["level"] != "ERROR" || panics[0]["request_id"] != requestID || panics[0]["panic"] != "boom: database password" {
		t.Errorf("panic line = %v", panics[0])
	}
	if stack, _ := panics[0]["stack"].(string); !strings.Contains(stack, "goroutine") {
		t.Errorf("stack = %q, want a stack trace", stack)
	}

	requests := logLines(t, logs, "request completed")
	if len(requests) != 1 || requests[0]["status"] != float64(http.StatusInternalServerError) || requests[0]["request_id"] != requestID {
		t.Errorf("request lines = %v, want one with status 500 and the same request ID", requests)
	}
}

func TestAbortPanicIsPassedOn(t *testing.T) {
	app := newTestApplication(t, func(context.Context) error { panic(http.ErrAbortHandler) })

	defer func() {
		recovered, _ := recover().(error)
		if !errors.Is(recovered, http.ErrAbortHandler) {
			t.Errorf("recovered %v, want http.ErrAbortHandler to be passed on", recovered)
		}
	}()
	get(t, app.Routes(), "/healthz")
}

func TestServerErrorIsLoggedWithItsCause(t *testing.T) {
	app, logs := newLoggedApplication(t, healthy)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/webhooks", nil)

	app.serverError(recorder, request, errors.New("database is closed"))

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "database is closed") {
		t.Error("page reveals the cause of the error")
	}
	lines := logLines(t, logs, "request failed")
	if len(lines) != 1 || lines[0]["level"] != "ERROR" || lines[0]["error"] != "database is closed" || lines[0]["path"] != "/webhooks" {
		t.Errorf("error lines = %v, want one with the cause and the path", lines)
	}
}

func TestServerErrorFallsBackToPlainText(t *testing.T) {
	app, logs := newLoggedApplication(t, healthy)
	app.templates = map[string]*template.Template{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)

	app.serverError(recorder, request, errors.New("database is closed"))

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "Sorry, there is a problem with the service") || strings.Contains(body, "server-error") {
		t.Errorf("body = %q, want the plain message and no internal detail", body)
	}
	if len(logLines(t, logs, "failed to render the server error page")) != 1 {
		t.Error("the failure to render the error page was not logged")
	}
}

func TestHeadRequestsAreAnswered(t *testing.T) {
	routes := newTestApplication(t, healthy).Routes()

	testCases := []struct {
		target     string
		wantStatus int
	}{
		{target: "/", wantStatus: http.StatusOK},
		{target: "/healthz", wantStatus: http.StatusNoContent},
		{target: "/no-such-page", wantStatus: http.StatusNotFound},
	}

	for _, testCase := range testCases {
		t.Run(testCase.target, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			routes.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodHead, testCase.target, nil))
			if recorder.Code != testCase.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, testCase.wantStatus)
			}
		})
	}
}
