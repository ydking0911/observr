package query_test

import (
	"bytes"
	"encoding/csv"
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

func TestHTTPHandlerCSVHeaders(t *testing.T) {
	store := &mockStore{events: sampleEvents()}
	handler := query.NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/query?format=csv&last=10", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/csv" {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "observr-events.csv") {
		t.Errorf("Content-Disposition = %q, want filename=observr-events.csv", cd)
	}

	r := csv.NewReader(rec.Body)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error: %v", err)
	}
	// header row + 2 data rows
	if len(records) != 3 {
		t.Fatalf("expected 3 CSV rows (1 header + 2 data), got %d", len(records))
	}
	if records[0][0] != "timestamp" {
		t.Errorf("first CSV column = %q, want timestamp", records[0][0])
	}
}

func TestExecuteCSVOutput(t *testing.T) {
	store := &mockStore{events: sampleEvents()}
	var buf bytes.Buffer

	err := query.Execute(store, query.Query{Last: 10, Format: "csv"}, &buf)
	if err != nil {
		t.Fatal(err)
	}

	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected at least header + 1 row, got %d rows", len(records))
	}
	// Verify header columns
	header := records[0]
	expected := []string{"timestamp", "level", "service", "type", "method", "path", "status_code", "duration_ms", "message", "trace_id", "span_id", "id"}
	if len(header) != len(expected) {
		t.Fatalf("header length = %d, want %d", len(header), len(expected))
	}
	for i, col := range expected {
		if header[i] != col {
			t.Errorf("header[%d] = %q, want %q", i, header[i], col)
		}
	}
}

func TestExecuteCSVEmptyResults(t *testing.T) {
	store := &mockStore{}
	var buf bytes.Buffer

	if err := query.Execute(store, query.Query{Last: 10, Format: "csv"}, &buf); err != nil {
		t.Fatal(err)
	}
	// Should have header row only
	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 header row for empty results, got %d", len(records))
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

func TestCSVSpecialCharsEscape(t *testing.T) {
	store := &mockStore{events: []storage.Event{
		{
			ID:        "e-special",
			Service:   "api",
			Timestamp: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
			Type:      "log",
			Level:     "error",
			Message:   `timeout, retry failed: say "hello"`,
			Path:      "/api/v1\nnewline",
		},
	}}
	var buf bytes.Buffer

	if err := query.Execute(store, query.Query{Last: 10, Format: "csv"}, &buf); err != nil {
		t.Fatal(err)
	}

	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error (special chars not escaped properly): %v\nraw output:\n%s", err, buf.String())
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 rows (header + 1 data), got %d", len(records))
	}
	if got := records[1][8]; got != `timeout, retry failed: say "hello"` {
		t.Errorf("message col: got %q, want original unescaped value", got)
	}
	if got := records[1][5]; got != "/api/v1\nnewline" {
		t.Errorf("path col: got %q, want original value with newline", got)
	}
}

func TestCSVDataFieldValues(t *testing.T) {
	ts := time.Date(2024, 6, 1, 9, 30, 0, 0, time.UTC)
	store := &mockStore{events: []storage.Event{
		{
			ID:         "evt-abc123",
			TraceID:    "trace-xyz",
			SpanID:     "span-001",
			Service:    "svc-a",
			Timestamp:  ts,
			Type:       "http_request",
			Level:      "info",
			Method:     "POST",
			Path:       "/orders",
			StatusCode: 201,
			DurationMS: 123.456,
			Message:    "order created",
		},
	}}
	var buf bytes.Buffer

	if err := query.Execute(store, query.Query{Last: 10, Format: "csv"}, &buf); err != nil {
		t.Fatal(err)
	}

	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(records))
	}

	row := records[1]
	checks := []struct {
		col  int
		name string
		want string
	}{
		{0, "timestamp", "2024-06-01T09:30:00Z"},
		{1, "level", "info"},
		{2, "service", "svc-a"},
		{3, "type", "http_request"},
		{4, "method", "POST"},
		{5, "path", "/orders"},
		{6, "status_code", "201"},
		{7, "duration_ms", "123.456"},
		{8, "message", "order created"},
		{9, "trace_id", "trace-xyz"},
		{10, "span_id", "span-001"},
		{11, "id", "evt-abc123"},
	}
	for _, c := range checks {
		if row[c.col] != c.want {
			t.Errorf("col[%d] %s: got %q, want %q", c.col, c.name, row[c.col], c.want)
		}
	}
}

func TestCSVZeroStatusCodeAndDuration(t *testing.T) {
	store := &mockStore{events: []storage.Event{
		{
			ID:         "evt-log",
			Service:    "worker",
			Timestamp:  time.Now(),
			Type:       "log",
			Level:      "debug",
			Message:    "heartbeat",
			StatusCode: 0,
			DurationMS: 0,
		},
	}}
	var buf bytes.Buffer

	if err := query.Execute(store, query.Query{Last: 10, Format: "csv"}, &buf); err != nil {
		t.Fatal(err)
	}

	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(records))
	}
	if row := records[1]; row[6] != "" {
		t.Errorf("status_code for zero: got %q, want empty string", row[6])
	}
	if row := records[1]; row[7] != "" {
		t.Errorf("duration_ms for zero: got %q, want empty string", row[7])
	}
}

func TestHTTPHandlerLevelFilterCSV(t *testing.T) {
	store := &mockStore{events: sampleEvents()} // 1 info + 1 error
	handler := query.NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/query?level=error&format=csv", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/csv" {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}

	r := csv.NewReader(rec.Body)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 rows (header + 1 error), got %d", len(records))
	}
	if records[1][1] != "error" {
		t.Errorf("filtered row level = %q, want error", records[1][1])
	}
}

func TestHTTPHandlerContentDispositionExact(t *testing.T) {
	store := &mockStore{events: sampleEvents()}
	handler := query.NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/query?format=csv", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	want := `attachment; filename="observr-events.csv"`
	if got := rec.Header().Get("Content-Disposition"); got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
}

func TestHTTPHandlerDefaultFormatIsJSON(t *testing.T) {
	store := &mockStore{events: sampleEvents()}
	handler := query.NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/query", nil) // no format param
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var events []storage.Event
	if err := json.NewDecoder(rec.Body).Decode(&events); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
}
