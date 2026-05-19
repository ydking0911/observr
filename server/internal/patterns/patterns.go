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
	GroupBy       string    `json:"group_by,omitempty"`
	GroupValue    string    `json:"group_value,omitempty"`
	Count         int       `json:"count"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	Level         string    `json:"level"`
	Services      []string  `json:"services"`
	SampleEventID string    `json:"sample_event_id"`
	Trend         string    `json:"trend"`
	AnomalyScore  float64   `json:"anomaly_score"`
	Anomaly       bool      `json:"anomaly"`
	Buckets       []Bucket  `json:"buckets,omitempty"`
	Tools         []string  `json:"tools,omitempty"`
	Intents       []string  `json:"intents,omitempty"`
	Models        []string  `json:"models,omitempty"`
}

// GroupOptions controls optional pattern enrichment.
type GroupOptions struct {
	MinCount         int
	GroupBy          string
	BucketStart      time.Time
	BucketSize       time.Duration
	BucketCount      int
	IncludeBuckets   bool
	AnomalyThreshold float64
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
	return FetchWithOptions(s, since, level, GroupOptions{MinCount: minCount})
}

// FetchWithOptions queries the store and returns enriched patterns.
func FetchWithOptions(s querier, since time.Duration, level string, opts GroupOptions) ([]Pattern, error) {
	if since <= 0 {
		since = 15 * time.Minute
	}
	now := time.Now().UTC()
	filter := storage.QueryFilter{
		Last:  10000,
		Level: level,
		Since: now.Add(-since),
	}
	events, err := s.Query(filter)
	if err != nil {
		return nil, err
	}
	if opts.BucketSize == 0 {
		opts.BucketSize = BucketSize(since)
	}
	if opts.BucketStart.IsZero() {
		opts.BucketStart = now.Add(-since)
	}
	if opts.BucketCount == 0 {
		opts.BucketCount = int(now.Sub(opts.BucketStart)/opts.BucketSize) + 1
	}
	return GroupWithOptions(events, opts), nil
}

// Group groups events by fingerprint and returns patterns sorted by count desc.
func Group(events []storage.Event, minCount int) []Pattern {
	return GroupWithOptions(events, GroupOptions{MinCount: minCount})
}

// GroupWithOptions groups events and optionally adds bucket, anomaly, and
// agent-attribute breakdowns.
func GroupWithOptions(events []storage.Event, opts GroupOptions) []Pattern {
	if opts.MinCount <= 0 {
		opts.MinCount = 1
	}
	if opts.AnomalyThreshold <= 0 {
		opts.AnomalyThreshold = DefaultAnomalyThreshold
	}

	type acc struct {
		count         int
		firstSeen     time.Time
		lastSeen      time.Time
		level         string
		services      map[string]struct{}
		sampleEventID string
		fingerprint   string
		groupValue    string
		tools         map[string]struct{}
		intents       map[string]struct{}
		models        map[string]struct{}
		bucketCounts  []int
	}

	byFP := map[string]*acc{}

	for _, e := range events {
		fp := Normalize(e.Message)
		if fp == "" {
			continue
		}
		tool, intent, model := ExtractAgentAttrs(e)
		groupValue := groupValueFor(opts.GroupBy, tool, intent, model)
		key := fp
		if opts.GroupBy != "" {
			key += "\x00" + groupValue
		}

		a, ok := byFP[key]
		if !ok {
			a = &acc{
				firstSeen:     e.Timestamp,
				lastSeen:      e.Timestamp,
				level:         e.Level,
				services:      map[string]struct{}{},
				sampleEventID: e.ID,
				fingerprint:   fp,
				groupValue:    groupValue,
				tools:         map[string]struct{}{},
				intents:       map[string]struct{}{},
				models:        map[string]struct{}{},
			}
			if opts.BucketCount > 0 {
				a.bucketCounts = make([]int, opts.BucketCount)
			}
			byFP[key] = a
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
		if tool != "" {
			a.tools[tool] = struct{}{}
		}
		if intent != "" {
			a.intents[intent] = struct{}{}
		}
		if model != "" {
			a.models[model] = struct{}{}
		}
		if len(a.bucketCounts) > 0 && !opts.BucketStart.IsZero() && opts.BucketSize > 0 && !e.Timestamp.Before(opts.BucketStart) {
			idx := int(e.Timestamp.Sub(opts.BucketStart) / opts.BucketSize)
			if idx >= 0 && idx < len(a.bucketCounts) {
				a.bucketCounts[idx]++
			}
		}
	}

	out := make([]Pattern, 0, len(byFP))
	for _, a := range byFP {
		if a.count < opts.MinCount {
			continue
		}
		svcs := make([]string, 0, len(a.services))
		for svc := range a.services {
			svcs = append(svcs, svc)
		}
		sort.Strings(svcs)

		var buckets []Bucket
		if len(a.bucketCounts) > 0 {
			buckets = ToBuckets(a.bucketCounts, opts.BucketStart, opts.BucketSize)
		}
		score := AnomalyScore(buckets)
		p := Pattern{
			Fingerprint:   a.fingerprint,
			GroupBy:       opts.GroupBy,
			GroupValue:    a.groupValue,
			Count:         a.count,
			FirstSeen:     a.firstSeen,
			LastSeen:      a.lastSeen,
			Level:         a.level,
			Services:      svcs,
			SampleEventID: a.sampleEventID,
			Trend:         Trend(buckets),
			AnomalyScore:  score,
			Anomaly:       score >= opts.AnomalyThreshold,
			Tools:         sortedKeys(a.tools),
			Intents:       sortedKeys(a.intents),
			Models:        sortedKeys(a.models),
		}
		if opts.IncludeBuckets {
			p.Buckets = buckets
		}
		out = append(out, p)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			if out[i].Fingerprint == out[j].Fingerprint {
				return out[i].GroupValue < out[j].GroupValue
			}
			return out[i].Fingerprint < out[j].Fingerprint
		}
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
//   - buckets=true include temporal frequency buckets
//   - group_by=tool|intent|model groups fingerprint by agent attribute value
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

		ps, err := FetchWithOptions(s, sinceDur, q.Get("level"), GroupOptions{
			MinCount:       minCount,
			GroupBy:        q.Get("group_by"),
			IncludeBuckets: q.Get("buckets") == "true",
		})
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

// groupValueFor returns the attribute value for the requested groupBy key,
// or "" if the event does not have that attribute (caller skips the event).
func groupValueFor(groupBy, tool, intent, model string) string {
	switch groupBy {
	case "tool":
		return tool
	case "intent":
		return intent
	case "model":
		return model
	}
	return ""
}
