package observr_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	observr "github.com/ydking0911/observr/sdk/go"
)

func TestTransportSendsEvent(t *testing.T) {
	received := make(chan map[string]any, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []map[string]any
		_ = json.NewDecoder(r.Body).Decode(&events)
		for _, e := range events {
			received <- e
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	tr := observr.NewTransport(srv.URL)
	tr.Start()
	tr.Enqueue(map[string]any{"service": "test", "type": "log", "level": "info", "message": "hello"})

	select {
	case e := <-received:
		if e["message"] != "hello" {
			t.Fatalf("unexpected event: %v", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for event")
	}
	tr.Shutdown()
}

func TestTransportDrainsOnShutdown(t *testing.T) {
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []map[string]any
		_ = json.NewDecoder(r.Body).Decode(&events)
		count += len(events)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	tr := observr.NewTransport(srv.URL)
	tr.Start()
	for i := 0; i < 5; i++ {
		tr.Enqueue(map[string]any{"service": "t", "type": "log", "level": "info", "message": "x"})
	}
	tr.Shutdown()
	if count < 5 {
		t.Fatalf("expected 5 events flushed, got %d", count)
	}
}
