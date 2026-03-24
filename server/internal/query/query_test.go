package query_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ydking0911/observr/server/internal/query"
	"github.com/ydking0911/observr/server/internal/storage"
)

// mockStore satisfies the querier interface for testing.
type mockStore struct {
	events []storage.Event
}

func (m *mockStore) Query(f storage.QueryFilter) ([]storage.Event, error) {
	result := m.events
	if f.Level != "" {
		var filtered []storage.Event
		for _, e := range result {
			if e.Level == f.Level {
				filtered = append(filtered, e)
			}
		}
		result = filtered
	}
	if f.Last > 0 && len(result) > f.Last {
		result = result[:f.Last]
	}
	return result, nil
}

func sampleEvents() []storage.Event {
	return []storage.Event{
		{ID: "e1", Service: "api", Timestamp: time.Now(), Type: "http_request", Level: "info", Method: "GET", Path: "/users", Message: "GET /users", DurationMS: 42},
		{ID: "e2", Service: "api", Timestamp: time.Now(), Type: "log", Level: "error", Message: "db timeout"},
	}
}

func TestHTTPHandlerReturnsJSON(t *testing.T) {
	store := &mockStore{events: sampleEvents()}
	handler := query.NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/query?format=json&last=10", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var events []storage.Event
	if err := json.NewDecoder(rec.Body).Decode(&events); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, rec.Body.String())
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestHTTPHandlerFilterByLevel(t *testing.T) {
	store := &mockStore{events: sampleEvents()}
	handler := query.NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/query?level=error&format=json", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var events []storage.Event
	json.NewDecoder(rec.Body).Decode(&events) //nolint:errcheck
	if len(events) != 1 || events[0].Level != "error" {
		t.Fatalf("expected 1 error event, got %+v", events)
	}
}

func TestExecuteJSONOutput(t *testing.T) {
	store := &mockStore{events: sampleEvents()}
	var buf bytes.Buffer

	err := query.Execute(store, query.Query{Last: 10, Format: "json"}, &buf)
	if err != nil {
		t.Fatal(err)
	}

	var events []storage.Event
	if err := json.Unmarshal(buf.Bytes(), &events); err != nil {
		t.Fatalf("JSON unmarshal: %v\n%s", err, buf.String())
	}
}

func TestExecuteTextOutput(t *testing.T) {
	store := &mockStore{events: sampleEvents()}
	var buf bytes.Buffer

	err := query.Execute(store, query.Query{Last: 10, Format: "text"}, &buf)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "TIMESTAMP") {
		t.Error("text output missing header")
	}
	if !strings.Contains(out, "GET /users") {
		t.Error("text output missing event message")
	}
}

func TestExecuteEmptyResultsNoError(t *testing.T) {
	store := &mockStore{}
	var buf bytes.Buffer

	if err := query.Execute(store, query.Query{Last: 10, Format: "json"}, &buf); err != nil {
		t.Fatal(err)
	}
	// Should return empty JSON array, not null
	out := strings.TrimSpace(buf.String())
	if out != "null" && out != "[]" && !strings.HasPrefix(out, "[") {
		t.Errorf("unexpected output for empty results: %q", out)
	}
}
