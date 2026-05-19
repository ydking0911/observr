package patterns_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ydking0911/observr/server/internal/patterns"
	"github.com/ydking0911/observr/server/internal/storage"
)

func TestCausalCorrelationsFromRootIntentToErrorFingerprint(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	events := []storage.Event{
		{ID: "root1", TraceID: "t1", SpanID: "s1", Service: "agent", Level: "info", Type: "span", Message: "checkout", Timestamp: now, Attributes: map[string]any{"agent.intent": "checkout"}},
		{ID: "err1", TraceID: "t1", SpanID: "s2", ParentSpanID: "s1", Service: "payments", Level: "error", Type: "log", Message: "payment timeout for user 123", Timestamp: now.Add(time.Second)},
		{ID: "root2", TraceID: "t2", SpanID: "s3", Service: "agent", Level: "info", Type: "span", Message: "checkout", Timestamp: now.Add(2 * time.Second), Attributes: map[string]any{"agent.intent": "checkout"}},
		{ID: "err2", TraceID: "t2", SpanID: "s4", ParentSpanID: "s3", Service: "payments", Level: "error", Type: "log", Message: "payment timeout for user 456", Timestamp: now.Add(3 * time.Second)},
	}

	got := patterns.Correlate(events, 1)
	if len(got) != 1 {
		t.Fatalf("expected 1 correlation, got %d", len(got))
	}
	if got[0].RootIntent != "checkout" {
		t.Errorf("RootIntent = %q, want checkout", got[0].RootIntent)
	}
	if got[0].ErrorFingerprint != "payment timeout for user <N>" {
		t.Errorf("ErrorFingerprint = %q", got[0].ErrorFingerprint)
	}
	if got[0].Count != 2 {
		t.Errorf("Count = %d, want 2", got[0].Count)
	}
	if got[0].Probability != 1 {
		t.Errorf("Probability = %v, want 1", got[0].Probability)
	}
}

func TestCausalHandlerReturnsJSON(t *testing.T) {
	now := time.Now().UTC()
	store := &patternMockStore{events: []storage.Event{
		{ID: "root1", TraceID: "t1", SpanID: "s1", Service: "agent", Level: "info", Type: "span", Message: "research", Timestamp: now, Attributes: map[string]any{"agent.intent": "research"}},
		{ID: "err1", TraceID: "t1", SpanID: "s2", ParentSpanID: "s1", Service: "search", Level: "error", Type: "log", Message: "search timeout 5000", Timestamp: now.Add(time.Second)},
	}}
	handler := patterns.NewCausalHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/patterns/causal?since=15m", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var got []patterns.CausalCorrelation
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RootIntent != "research" {
		t.Fatalf("unexpected response: %+v", got)
	}
}
