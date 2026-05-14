package collector_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ydking0911/observr/server/internal/collector"
	"github.com/ydking0911/observr/server/internal/storage"
)

type mockStore struct {
	inserted []storage.Event
}

func (m *mockStore) Insert(events []storage.Event) error {
	m.inserted = append(m.inserted, events...)
	return nil
}

func postEvents(t *testing.T, handler http.Handler, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHandlerAcceptsValidBatch(t *testing.T) {
	store := &mockStore{}
	handler := collector.NewHandler(store)

	payload := map[string]any{
		"events": []map[string]any{
			{"service": "api", "type": "log", "level": "info", "message": "hello"},
			{"service": "api", "type": "http_request", "level": "error", "path": "/checkout", "status_code": 500, "duration_ms": 3241.5},
		},
	}

	rec := postEvents(t, handler, payload)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(store.inserted) != 2 {
		t.Fatalf("expected 2 inserted events, got %d", len(store.inserted))
	}
}

func TestHandlerDefaultsLevelToInfo(t *testing.T) {
	store := &mockStore{}
	handler := collector.NewHandler(store)

	rec := postEvents(t, handler, map[string]any{
		"events": []map[string]any{
			{"service": "api", "type": "log", "message": "no level set"},
		},
	})

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	if store.inserted[0].Level != "info" {
		t.Errorf("expected default level 'info', got %q", store.inserted[0].Level)
	}
}

func TestHandlerRejectsMalformedJSON(t *testing.T) {
	store := &mockStore{}
	handler := collector.NewHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/events", bytes.NewBufferString(`{bad json`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if len(store.inserted) != 0 {
		t.Error("no events should be inserted on bad request")
	}
}

func TestHandlerPreservesAttributes(t *testing.T) {
	store := &mockStore{}
	handler := collector.NewHandler(store)

	postEvents(t, handler, map[string]any{
		"events": []map[string]any{{
			"service": "api", "type": "log", "level": "error",
			"message": "payment failed",
			"attributes": map[string]any{"user_id": "u_123", "amount": 9900},
		}},
	})

	attrs := store.inserted[0].Attributes
	if attrs["user_id"] != "u_123" {
		t.Errorf("attribute user_id not preserved: %v", attrs)
	}
}

func TestHandlerPreservesParentSpanID(t *testing.T) {
	store := &mockStore{}
	handler := collector.NewHandler(store)

	postEvents(t, handler, map[string]any{
		"events": []map[string]any{{
			"service":        "api",
			"type":           "span",
			"level":          "info",
			"message":        "child.op",
			"trace_id":       "trace-abc",
			"span_id":        "span-child",
			"parent_span_id": "span-parent",
		}},
	})

	if store.inserted[0].ParentSpanID != "span-parent" {
		t.Errorf("ParentSpanID not passed through: got %q", store.inserted[0].ParentSpanID)
	}
}

func TestHandlerEmptyBatch(t *testing.T) {
	store := &mockStore{}
	handler := collector.NewHandler(store)

	rec := postEvents(t, handler, map[string]any{"events": []any{}})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for empty batch, got %d", rec.Code)
	}
}
