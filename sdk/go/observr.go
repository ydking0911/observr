package observr

import "context"

var _client *ObservrClient

// Init initialises the global client. Call once at startup.
func Init(cfg Config) *ObservrClient {
	_client = NewClient(cfg)
	_client.Start()
	return _client
}

// GetClient returns the global client. Panics if Init was not called.
func GetClient() *ObservrClient {
	if _client == nil {
		panic("observr.Init() must be called before GetClient()")
	}
	return _client
}

// StartSpan is a convenience wrapper around GetClient().Span().
func StartSpan(ctx context.Context, name string, attrs map[string]any) (context.Context, func()) {
	return GetClient().Span(ctx, name, attrs)
}

// StartAgentSpan is a convenience wrapper around GetClient().AgentSpan().
func StartAgentSpan(ctx context.Context, name string, opts AgentSpanOptions) (context.Context, func()) {
	return GetClient().AgentSpan(ctx, name, opts)
}
