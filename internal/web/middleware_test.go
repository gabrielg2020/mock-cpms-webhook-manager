package web

import (
	"bytes"
	"encoding/json"
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
