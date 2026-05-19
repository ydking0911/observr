package nethttp

import (
	"net/http"

	observr "github.com/ydking0911/observr/sdk/go"
)

// Middleware wraps an http.Handler to inject observr spans and emit http_request events.
func Middleware(c *observr.ObservrClient) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			if tp := r.Header.Get("traceparent"); tp != "" {
				if parent, err := observr.ParseTraceparent(tp); err == nil {
					ctx = observr.ContextWithSpan(ctx, parent)
				}
			}

			ctx, end := c.Span(ctx, r.Method+" "+r.URL.Path, map[string]any{
				"http.method": r.Method,
				"http.path":   r.URL.Path,
			})
			span := observr.SpanFromContext(ctx)

			w.Header().Set("traceparent", observr.FormatTraceparent(span))

			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r.WithContext(ctx))
			end()
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
