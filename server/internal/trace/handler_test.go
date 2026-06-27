package trace_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
	"github.com/ydking0911/observr/server/internal/trace"
)

func openStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestHandlerReturnsEventsForTrace(t *testing.T) {
	s := openStore(t)
	now := time.Now().UTC()
	events := []storage.Event{
		{ID: "evt_1", TraceID: "abc123", Service: "svc-a", Type: "span", Level: "info", Message: "op-1", Timestamp: now},
		{ID: "evt_2", TraceID: "abc123", Service: "svc-b", Type: "span", Level: "info", Message: "op-2", Timestamp: now.Add(time.Millisecond)},
		{ID: "evt_3", TraceID: "other", Service: "svc-c", Type: "span", Level: "info", Message: "unrelated", Timestamp: now},
	}
	if err := s.Insert(events); err != nil {
		t.Fatal(err)
	}

	h := trace.NewHandler(s)
	req := httptest.NewRequest(http.MethodGet, "/trace/abc123", nil)
	req.SetPathValue("trace_id", "abc123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var got []storage.Event
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	if got[0].ID != "evt_1" || got[1].ID != "evt_2" {
		t.Fatalf("wrong order: %s, %s", got[0].ID, got[1].ID)
	}
}

func TestHandlerReturnsEmptyArrayForUnknownTrace(t *testing.T) {
	s := openStore(t)
	h := trace.NewHandler(s)
	req := httptest.NewRequest(http.MethodGet, "/trace/nope", nil)
	req.SetPathValue("trace_id", "nope")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got []storage.Event
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty array, got %v", got)
	}
}
