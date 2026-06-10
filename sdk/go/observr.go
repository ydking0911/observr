package observr

import (
	"context"
	"sync"
)

var (
	_mu     sync.Mutex
	_client *ObservrClient
)

// Init initialises the global client. Call once at startup.
// If called again, the previous client is shut down first.
func Init(cfg Config) *ObservrClient {
	_mu.Lock()
	defer _mu.Unlock()
	if _client != nil {
		_client.Shutdown()
	}
	_client = NewClient(cfg)
	_client.Start()
	return _client
}

// GetClient returns the global client. Panics if Init was not called.
func GetClient() *ObservrClient {
	_mu.Lock()
	defer _mu.Unlock()
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
