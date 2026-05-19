package patterns

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
)

// CausalCorrelation summarizes how often a root agent intent produces a
// downstream error fingerprint.
type CausalCorrelation struct {
	RootIntent       string   `json:"root_intent"`
	ErrorFingerprint string   `json:"error_fingerprint"`
	Count            int      `json:"count"`
	Probability      float64  `json:"probability"`
	Services         []string `json:"services"`
	SampleEventID    string   `json:"sample_event_id"`
}

// Correlate aggregates root_intent -> error_fingerprint pairs using span
// parent links inside each trace.
func Correlate(events []storage.Event, minCount int) []CausalCorrelation {
	if minCount <= 0 {
		minCount = 1
	}
	byTraceSpan := map[string]map[string]storage.Event{}
	rootIntentCounts := map[string]int{}
	seenRoots := map[string]struct{}{}

	for _, e := range events {
		if e.TraceID == "" || e.SpanID == "" {
			continue
		}
		if byTraceSpan[e.TraceID] == nil {
			byTraceSpan[e.TraceID] = map[string]storage.Event{}
		}
		if _, exists := byTraceSpan[e.TraceID][e.SpanID]; !exists {
			byTraceSpan[e.TraceID][e.SpanID] = e
		}
	}

	for _, spans := range byTraceSpan {
		for _, e := range spans {
			if e.ParentSpanID != "" {
				continue
			}
			_, intent, _ := ExtractAgentAttrs(e)
			if intent == "" {
				continue
			}
			key := e.TraceID + "\x00" + e.SpanID
			if _, ok := seenRoots[key]; ok {
				continue
			}
			seenRoots[key] = struct{}{}
			rootIntentCounts[intent]++
		}
	}

	type acc struct {
		count         int
		rootIntent    string
		fingerprint   string
		services      map[string]struct{}
		sampleEventID string
	}
	byPair := map[string]*acc{}
	for _, e := range events {
		if e.Level != "error" {
			continue
		}
		fp := Normalize(e.Message)
		if fp == "" {
			continue
		}
		intent := rootIntentFor(e, byTraceSpan)
		if intent == "" {
			continue
		}
		key := intent + "\x00" + fp
		a, ok := byPair[key]
		if !ok {
			a = &acc{rootIntent: intent, fingerprint: fp, services: map[string]struct{}{}, sampleEventID: e.ID}
			byPair[key] = a
		}
		a.count++
		a.services[e.Service] = struct{}{}
	}

	out := make([]CausalCorrelation, 0, len(byPair))
	for _, a := range byPair {
		if a.count < minCount {
			continue
		}
		services := sortedKeys(a.services)
		denom := rootIntentCounts[a.rootIntent]
		probability := 0.0
		if denom > 0 {
			probability = float64(a.count) / float64(denom)
			if probability > 1 {
				probability = 1
			}
		}
		out = append(out, CausalCorrelation{
			RootIntent:       a.rootIntent,
			ErrorFingerprint: a.fingerprint,
			Count:            a.count,
			Probability:      probability,
			Services:         services,
			SampleEventID:    a.sampleEventID,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			if out[i].RootIntent == out[j].RootIntent {
				return out[i].ErrorFingerprint < out[j].ErrorFingerprint
			}
			return out[i].RootIntent < out[j].RootIntent
		}
		return out[i].Count > out[j].Count
	})
	return out
}

// NewCausalHandler returns an http.Handler for GET /patterns/causal.
func NewCausalHandler(s querier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		sinceDur := 15 * time.Minute
		if sv := q.Get("since"); sv != "" {
			if d, err := time.ParseDuration(sv); err == nil {
				sinceDur = d
			}
		}

		minCount := 1
		if mc := q.Get("min_count"); mc != "" {
			if n, err := strconv.Atoi(mc); err == nil && n > 0 {
				minCount = n
			}
		}

		events, err := s.Query(storage.QueryFilter{
			Last:  10000,
			Since: time.Now().UTC().Add(-sinceDur),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cs := Correlate(events, minCount)
		if cs == nil {
			cs = []CausalCorrelation{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cs)
	})
}

func rootIntentFor(e storage.Event, byTraceSpan map[string]map[string]storage.Event) string {
	if e.TraceID != "" {
		spans := byTraceSpan[e.TraceID]
		current, ok := spans[e.SpanID]
		if !ok && e.ParentSpanID != "" {
			current, ok = spans[e.ParentSpanID]
		}
		seen := map[string]struct{}{}
		for ok {
			if current.SpanID != "" {
				if _, exists := seen[current.SpanID]; exists {
					break
				}
				seen[current.SpanID] = struct{}{}
			}
			_, currentIntent, _ := ExtractAgentAttrs(current)
			if current.ParentSpanID == "" {
				return currentIntent
			}
			next, nextOK := spans[current.ParentSpanID]
			if !nextOK {
				if currentIntent != "" {
					return currentIntent
				}
				break
			}
			current = next
			// ok is already true; keep looping
		}
	}
	_, intent, _ := ExtractAgentAttrs(e)
	return intent
}
