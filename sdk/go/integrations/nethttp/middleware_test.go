package nethttp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	observr "github.com/ydking0911/observr/sdk/go"
	nethttp "github.com/ydking0911/observr/sdk/go/integrations/nethttp"
)

func TestMiddlewareInjectsSpan(t *testing.T) {
	var gotSpan *observr.Span
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSpan = observr.SpanFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	c := observr.NewClient(observr.Config{Service: "svc", CollectorURL: "http://localhost:9999"})
	mw := nethttp.Middleware(c)
	srv := httptest.NewServer(mw(handler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ping")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if gotSpan == nil {
		t.Fatal("expected span in handler context, got nil")
	}
	if gotSpan.TraceID == "" {
		t.Fatal("expected non-empty trace_id")
	}
}

func TestMiddlewareReadsTraceparent(t *testing.T) {
	var gotSpan *observr.Span
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSpan = observr.SpanFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	c := observr.NewClient(observr.Config{Service: "svc", CollectorURL: "http://localhost:9999"})
	mw := nethttp.Middleware(c)
	srv := httptest.NewServer(mw(handler))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if gotSpan == nil || gotSpan.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("expected trace_id from traceparent, got: %+v", gotSpan)
	}
}
