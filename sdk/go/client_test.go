package observr_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	observr "github.com/ydking0911/observr/sdk/go"
)

func TestClientSpanEmitsEvent(t *testing.T) {
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

	c := observr.NewClient(observr.Config{Service: "svc", CollectorURL: srv.URL})
	c.Start()

	ctx, end := c.Span(context.Background(), "test-span", nil)
	observr.SpanFromContext(ctx).SetAttribute("key", "val")
	end()

	select {
	case e := <-received:
		if e["type"] != "span" {
			t.Fatalf("expected type=span, got %v", e["type"])
		}
		if e["message"] != "test-span" {
			t.Fatalf("unexpected message: %v", e["message"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	c.Shutdown()
}

func TestClientAgentSpanSetsAttrs(t *testing.T) {
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

	c := observr.NewClient(observr.Config{Service: "svc", CollectorURL: srv.URL})
	c.Start()

	_, end := c.AgentSpan(context.Background(), "agent-op", observr.AgentSpanOptions{
		Intent: "summarize",
		Tool:   "search",
		Model:  "gpt-4o",
	})
	end()

	select {
	case e := <-received:
		attrs, _ := e["attributes"].(map[string]any)
		if attrs["observr.intent"] != "summarize" {
			t.Fatalf("intent not set: %v", attrs)
		}
		if attrs["observr.tool"] != "search" {
			t.Fatalf("tool not set: %v", attrs)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	c.Shutdown()
}
