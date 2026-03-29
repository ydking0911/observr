// Package patterns groups events by normalized message fingerprint.
package patterns

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
)

// Pattern represents a group of events sharing the same message fingerprint.
type Pattern struct {
	Fingerprint   string    `json:"fingerprint"`
	Count         int       `json:"count"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	Level         string    `json:"level"`
	Services      []string  `json:"services"`
	SampleEventID string    `json:"sample_event_id"`
}

// Normalization regexes applied in order: UUID → IP → hex → number.
// UUID must precede hex to avoid partial replacement of UUID segments.
var (
	reUUID = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	reIP   = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	reHex  = regexp.MustCompile(`(?i)\b[0-9a-f]{8,}\b`)
	reNum  = regexp.MustCompile(`\b\d+\b`)
)

// Normalize replaces variable tokens (UUIDs, IPs, hex strings, integers) with
// stable placeholders so that similar messages share the same fingerprint.
func Normalize(msg string) string {
	msg = reUUID.ReplaceAllString(msg, "<uuid>")
	msg = reIP.ReplaceAllString(msg, "<ip>")
	msg = reHex.ReplaceAllString(msg, "<hex>")
	msg = reNum.ReplaceAllString(msg, "<N>")
	return strings.TrimSpace(msg)
}

type querier interface {
	Query(f storage.QueryFilter) ([]storage.Event, error)
}

// Fetch queries the store and returns patterns grouped by fingerprint.
func Fetch(s querier, since time.Duration, level string, minCount int) ([]Pattern, error) {
	filter := storage.QueryFilter{
		Last:  10000,
		Level: level,
		Since: time.Now().Add(-since),
	}
	events, err := s.Query(filter)
	if err != nil {
		return nil, err
	}
	return Group(events, minCount), nil
}

// Group groups events by fingerprint and returns patterns sorted by count desc.
func Group(events []storage.Event, minCount int) []Pattern {
	type acc struct {
		count         int
		firstSeen     time.Time
		lastSeen      time.Time
		level         string
		services      map[string]struct{}
		sampleEventID string
	}

	byFP := map[string]*acc{}

	for _, e := range events {
		fp := Normalize(e.Message)
		if fp == "" {
			continue
		}
		a, ok := byFP[fp]
		if !ok {
			a = &acc{
				firstSeen:     e.Timestamp,
				lastSeen:      e.Timestamp,
				level:         e.Level,
				services:      map[string]struct{}{},
				sampleEventID: e.ID,
			}
			byFP[fp] = a
		}
		a.count++
		if e.Timestamp.Before(a.firstSeen) {
			a.firstSeen = e.Timestamp
		}
		if e.Timestamp.After(a.lastSeen) {
			a.lastSeen = e.Timestamp
		}
		a.services[e.Service] = struct{}{}
		if levelRank(e.Level) > levelRank(a.level) {
			a.level = e.Level
		}
	}

	out := make([]Pattern, 0, len(byFP))
	for fp, a := range byFP {
		if a.count < minCount {
			continue
		}
		svcs := make([]string, 0, len(a.services))
		for svc := range a.services {
			svcs = append(svcs, svc)
		}
		sort.Strings(svcs)
		out = append(out, Pattern{
			Fingerprint:   fp,
			Count:         a.count,
			FirstSeen:     a.firstSeen,
			LastSeen:      a.lastSeen,
			Level:         a.level,
			Services:      svcs,
			SampleEventID: a.sampleEventID,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Count > out[j].Count
	})
	return out
}

// NewHandler returns an http.Handler for GET /patterns.
//
// Query parameters:
//   - since=15m    time window (any time.Duration string; default 15m)
//   - level=error  pre-filter events to a specific level before grouping
//   - min_count=3  minimum group size to include (default 1)
func NewHandler(s querier) http.Handler {
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

		ps, err := Fetch(s, sinceDur, q.Get("level"), minCount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if ps == nil {
			ps = []Pattern{} // always return array, never null
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ps)
	})
}

func levelRank(level string) int {
	switch level {
	case "debug":
		return 0
	case "info":
		return 1
	case "warn":
		return 2
	case "error":
		return 3
	default:
		return -1
	}
}
