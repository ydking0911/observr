// Package collector handles the POST /events intake from SDK clients.
package collector

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
)

type store interface {
	Insert(events []storage.Event) error
}

// NewHandler returns an http.Handler that receives batched events from SDKs.
func NewHandler(s store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Events []rawEvent `json:"events"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB limit
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		events := make([]storage.Event, 0, len(body.Events))
		for _, raw := range body.Events {
			events = append(events, raw.toEvent())
		}

		if err := s.Insert(events); err != nil {
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	})
}

// rawEvent mirrors the JSON shape sent by SDKs.
type rawEvent struct {
	ID           string         `json:"id"`
	TraceID      string         `json:"trace_id"`
	SpanID       string         `json:"span_id"`
	ParentSpanID string         `json:"parent_span_id"`
	Service      string         `json:"service"`
	Timestamp    string         `json:"timestamp"`
	Type         string         `json:"type"`
	Level        string         `json:"level"`
	Method       string         `json:"method"`
	Path         string         `json:"path"`
	StatusCode   int            `json:"status_code"`
	DurationMS   float64        `json:"duration_ms"`
	Message      string         `json:"message"`
	Attributes   map[string]any `json:"attributes"`
}

func (r rawEvent) toEvent() storage.Event {
	ts := time.Now().UTC()
	if r.Timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, r.Timestamp); err == nil {
			ts = parsed
		}
	}
	level := r.Level
	if level == "" {
		level = "info"
	}
	return storage.Event{
		ID:           r.ID,
		TraceID:      r.TraceID,
		SpanID:       r.SpanID,
		ParentSpanID: r.ParentSpanID,
		Service:      r.Service,
		Timestamp:    ts,
		Type:         r.Type,
		Level:        level,
		Method:       r.Method,
		Path:         r.Path,
		StatusCode:   r.StatusCode,
		DurationMS:   r.DurationMS,
		Message:      r.Message,
		Attributes:   r.Attributes,
	}
}
