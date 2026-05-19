package observr_test

import (
	"context"
	"testing"

	observr "github.com/ydking0911/observr/sdk/go"
)

func TestSpanContextRoundTrip(t *testing.T) {
	s := &observr.Span{
		TraceID:  "abc123",
		SpanID:   "def456",
		ParentID: "",
	}
	ctx := observr.ContextWithSpan(context.Background(), s)
	got := observr.SpanFromContext(ctx)
	if got == nil {
		t.Fatal("expected span in context, got nil")
	}
	if got.TraceID != "abc123" || got.SpanID != "def456" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestSpanSetAttribute(t *testing.T) {
	s := &observr.Span{}
	s.SetAttribute("intent", "summarize")
	if s.Attributes["intent"] != "summarize" {
		t.Fatal("attribute not set")
	}
}
