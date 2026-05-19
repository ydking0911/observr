package observr_test

import (
	"testing"

	observr "github.com/ydking0911/observr/sdk/go"
)

func TestParseTraceparent(t *testing.T) {
	header := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	s, err := observr.ParseTraceparent(header)
	if err != nil {
		t.Fatal(err)
	}
	if s.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("wrong trace id: %s", s.TraceID)
	}
	if s.ParentID != "00f067aa0ba902b7" {
		t.Fatalf("wrong parent id: %s", s.ParentID)
	}
}

func TestFormatTraceparent(t *testing.T) {
	s := &observr.Span{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", SpanID: "aabbccdd11223344"}
	got := observr.FormatTraceparent(s)
	want := "00-4bf92f3577b34da6a3ce929d0e0e4736-aabbccdd11223344-01"
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestParseTraceparentInvalid(t *testing.T) {
	_, err := observr.ParseTraceparent("bad-header")
	if err == nil {
		t.Fatal("expected error for invalid header")
	}
}
