// Package tail provides the SSE (Server-Sent Events) endpoint for real-time
// event streaming, consumed by "observrd tail" and any HTTP client.
package tail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/ydking0911/observr/server/internal/storage"
)

// Hub manages SSE subscribers and broadcasts new events.
type Hub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan []byte]struct{})}
}

// Broadcast sends an event to all connected tail subscribers.
func (h *Hub) Broadcast(event storage.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- data:
		default:
			// slow client — drop silently
		}
	}
}

// ServeHTTP implements the GET /tail SSE endpoint.
//
// Filters (all optional, comma-separated for multi-value):
//
//	?level=error
//	?service=my-api
//	?type=http_request
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Parse filters
	level := r.URL.Query().Get("level")
	service := r.URL.Query().Get("service")
	eventType := r.URL.Query().Get("type")

	ch := make(chan []byte, 256)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}()

	// Initial keep-alive comment
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-ch:
			// Apply filters
			if !matchesFilter(data, level, service, eventType) {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// matchesFilter checks if the raw event JSON matches the given filters.
func matchesFilter(data []byte, level, service, eventType string) bool {
	if level == "" && service == "" && eventType == "" {
		return true
	}
	var e storage.Event
	if err := json.Unmarshal(data, &e); err != nil {
		return true
	}
	if level != "" && e.Level != level {
		return false
	}
	if service != "" && e.Service != service {
		return false
	}
	if eventType != "" && e.Type != eventType {
		return false
	}
	return true
}
