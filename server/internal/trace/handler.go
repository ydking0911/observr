// Package trace handles GET /trace/{trace_id}.
package trace

import (
	"encoding/json"
	"net/http"

	"github.com/ydking0911/observr/server/internal/storage"
)

type querier interface {
	QueryByTrace(traceID string) ([]storage.Event, error)
}

// NewHandler returns an HTTP handler for GET /trace/{trace_id}.
// Responds with a JSON array of all events belonging to the trace,
// sorted by timestamp ASC. Returns an empty array when the trace is unknown.
func NewHandler(s querier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.PathValue("trace_id")
		events, err := s.QueryByTrace(traceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if events == nil {
			events = []storage.Event{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(events); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}
