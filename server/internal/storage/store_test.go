package storage_test

import (
	"os"
	"testing"
	"time"

	"github.com/your-org/observr/server/internal/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "observr-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	s, err := storage.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInsertAndQuery(t *testing.T) {
	s := newTestStore(t)

	events := []storage.Event{
		{
			Service:   "svc-a",
			Timestamp: time.Now().UTC(),
			Type:      "http_request",
			Level:     "info",
			Method:    "GET",
			Path:      "/health",
			Message:   "GET /health",
		},
		{
			Service:   "svc-a",
			Timestamp: time.Now().UTC(),
			Type:      "log",
			Level:     "error",
			Message:   "database timeout",
			Attributes: map[string]any{
				"user_id": "u_123",
			},
		},
	}

	if err := s.Insert(events); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := s.Query(storage.QueryFilter{Last: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
}

func TestQueryFilterByLevel(t *testing.T) {
	s := newTestStore(t)

	events := []storage.Event{
		{Service: "svc", Timestamp: time.Now().UTC(), Type: "log", Level: "info", Message: "ok"},
		{Service: "svc", Timestamp: time.Now().UTC(), Type: "log", Level: "error", Message: "fail"},
		{Service: "svc", Timestamp: time.Now().UTC(), Type: "log", Level: "error", Message: "fail2"},
	}
	s.Insert(events) //nolint:errcheck

	got, err := s.Query(storage.QueryFilter{Level: "error", Last: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 error events, got %d", len(got))
	}
	for _, e := range got {
		if e.Level != "error" {
			t.Errorf("unexpected level %q", e.Level)
		}
	}
}

func TestQueryFilterByPath(t *testing.T) {
	s := newTestStore(t)

	s.Insert([]storage.Event{ //nolint:errcheck
		{Service: "svc", Timestamp: time.Now().UTC(), Type: "http_request", Level: "info", Path: "/checkout", Message: "POST /checkout"},
		{Service: "svc", Timestamp: time.Now().UTC(), Type: "http_request", Level: "info", Path: "/users", Message: "GET /users"},
	})

	got, err := s.Query(storage.QueryFilter{Path: "/checkout", Last: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].Path != "/checkout" {
		t.Errorf("wrong path %q", got[0].Path)
	}
}

func TestQueryRespectLastLimit(t *testing.T) {
	s := newTestStore(t)

	events := make([]storage.Event, 20)
	for i := range events {
		events[i] = storage.Event{
			Service:   "svc",
			Timestamp: time.Now().UTC(),
			Type:      "log",
			Level:     "info",
			Message:   "msg",
		}
	}
	s.Insert(events) //nolint:errcheck

	got, err := s.Query(storage.QueryFilter{Last: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5, got %d", len(got))
	}
}

func TestInsertSetsIDIfEmpty(t *testing.T) {
	s := newTestStore(t)

	e := storage.Event{
		Service:   "svc",
		Timestamp: time.Now().UTC(),
		Type:      "log",
		Level:     "info",
		Message:   "hello",
	}
	s.Insert([]storage.Event{e}) //nolint:errcheck

	got, _ := s.Query(storage.QueryFilter{Last: 1})
	if got[0].ID == "" {
		t.Error("expected non-empty ID to be assigned")
	}
}

func TestAttributesRoundtrip(t *testing.T) {
	s := newTestStore(t)

	attrs := map[string]any{
		"user_id": "u_999",
		"amount":  float64(9900),
	}
	s.Insert([]storage.Event{{ //nolint:errcheck
		Service: "svc", Timestamp: time.Now().UTC(),
		Type: "log", Level: "error", Message: "payment failed",
		Attributes: attrs,
	}})

	got, _ := s.Query(storage.QueryFilter{Last: 1})
	if got[0].Attributes["user_id"] != "u_999" {
		t.Errorf("attribute user_id not preserved: %v", got[0].Attributes)
	}
}
