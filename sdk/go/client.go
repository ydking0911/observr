package observr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Config holds client configuration.
type Config struct {
	Service      string
	CollectorURL string
}

// AgentSpanOptions sets standard agent attributes on a span.
type AgentSpanOptions struct {
	Intent  string
	Trigger string
	Model   string
	Tool    string
}

// ObservrClient sends events to an observrd collector.
type ObservrClient struct {
	cfg       Config
	transport *Transport
}

// NewClient creates a client. Call Start() before using.
func NewClient(cfg Config) *ObservrClient {
	if cfg.CollectorURL == "" {
		cfg.CollectorURL = "http://localhost:7676"
	}
	if cfg.Service == "" {
		cfg.Service = "app"
	}
	return &ObservrClient{
		cfg:       cfg,
		transport: NewTransport(cfg.CollectorURL),
	}
}

// Start begins background flushing.
func (c *ObservrClient) Start() { c.transport.Start() }

// Shutdown drains and stops the transport.
func (c *ObservrClient) Shutdown() { c.transport.Shutdown() }

// Span creates a child span derived from ctx, returns a new ctx and an end func.
// Call end() when the work is done — it emits the event.
func (c *ObservrClient) Span(ctx context.Context, name string, attrs map[string]any) (context.Context, func()) {
	parent := SpanFromContext(ctx)
	s := &Span{
		TraceID:    traceID(parent),
		SpanID:     newID(),
		Attributes: copyAttrs(attrs),
	}
	if parent != nil {
		s.ParentID = parent.SpanID
	}
	ctx = ContextWithSpan(ctx, s)
	start := time.Now()
	end := func() {
		event := map[string]any{
			"service":        c.cfg.Service,
			"type":           "span",
			"level":          "info",
			"message":        name,
			"trace_id":       s.TraceID,
			"span_id":        s.SpanID,
			"parent_span_id": s.ParentID,
			"duration_ms":    float64(time.Since(start).Milliseconds()),
			"attributes":     s.Attributes,
			"timestamp":      time.Now().UTC().Format(time.RFC3339Nano),
		}
		c.transport.Enqueue(event)
	}
	return ctx, end
}

// AgentSpan creates a span with standard agent attributes pre-populated.
func (c *ObservrClient) AgentSpan(ctx context.Context, name string, opts AgentSpanOptions) (context.Context, func()) {
	attrs := map[string]any{}
	if opts.Intent != "" {
		attrs["observr.intent"] = opts.Intent
	}
	if opts.Trigger != "" {
		attrs["observr.trigger"] = opts.Trigger
	}
	if opts.Model != "" {
		attrs["observr.model"] = opts.Model
	}
	if opts.Tool != "" {
		attrs["observr.tool"] = opts.Tool
	}
	return c.Span(ctx, name, attrs)
}

func copyAttrs(attrs map[string]any) map[string]any {
	if attrs == nil {
		return nil
	}
	cp := make(map[string]any, len(attrs))
	for k, v := range attrs {
		cp[k] = v
	}
	return cp
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("observr: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func traceID(parent *Span) string {
	if parent != nil && parent.TraceID != "" {
		return parent.TraceID
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("observr: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
