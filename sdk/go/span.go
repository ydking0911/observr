package observr

import "context"

type contextKey struct{}

// Span holds causal attribution for one unit of work.
type Span struct {
	TraceID    string
	SpanID     string
	ParentID   string
	Attributes map[string]any
}

// SetAttribute stores an arbitrary key-value pair on the span.
func (s *Span) SetAttribute(key string, value any) {
	if s.Attributes == nil {
		s.Attributes = make(map[string]any)
	}
	s.Attributes[key] = value
}

// ContextWithSpan returns ctx with span embedded.
func ContextWithSpan(ctx context.Context, s *Span) context.Context {
	return context.WithValue(ctx, contextKey{}, s)
}

// SpanFromContext retrieves the active span from ctx, or nil.
func SpanFromContext(ctx context.Context) *Span {
	s, _ := ctx.Value(contextKey{}).(*Span)
	return s
}
