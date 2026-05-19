package patterns

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
)

type summaryStore interface {
	Query(storage.QueryFilter) ([]storage.Event, error)
	SavePatternSummaries([]storage.PatternSummary) error
}

// Persistor recomputes pattern summaries from recent events and stores them in
// SQLite. It is wired through storage.Broadcaster so pattern persistence stays
// on the audit sink extension point.
type Persistor struct {
	store    summaryStore
	window   time.Duration
	interval time.Duration

	mu      sync.Mutex
	lastRun time.Time
}

func NewPersistor(store summaryStore, window, interval time.Duration) *Persistor {
	if window <= 0 {
		window = 24 * time.Hour
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Persistor{store: store, window: window, interval: interval}
}

func (p *Persistor) Broadcast(storage.Event) {
	p.mu.Lock()
	if time.Since(p.lastRun) < p.interval {
		p.mu.Unlock()
		return
	}
	p.lastRun = time.Now()
	p.mu.Unlock()

	go func() {
		if err := p.persist(); err != nil {
			log.Printf("pattern persistence error: %v", err)
		}
	}()
}

func (p *Persistor) persist() error {
	ps, err := FetchWithOptions(p.store, p.window, "", GroupOptions{
		MinCount:       1,
		IncludeBuckets: true,
	})
	if err != nil {
		return err
	}
	summaries := make([]storage.PatternSummary, 0, len(ps))
	now := time.Now().UTC()
	for _, pattern := range ps {
		buckets, _ := json.Marshal(pattern.Buckets)
		summaries = append(summaries, storage.PatternSummary{
			Fingerprint:   pattern.Fingerprint,
			GroupBy:       pattern.GroupBy,
			GroupValue:    pattern.GroupValue,
			Count:         pattern.Count,
			FirstSeen:     pattern.FirstSeen,
			LastSeen:      pattern.LastSeen,
			Level:         pattern.Level,
			Services:      pattern.Services,
			Tools:         pattern.Tools,
			Intents:       pattern.Intents,
			Models:        pattern.Models,
			Trend:         pattern.Trend,
			AnomalyScore:  pattern.AnomalyScore,
			Anomaly:       pattern.Anomaly,
			BucketsJSON:   string(buckets),
			SampleEventID: pattern.SampleEventID,
			UpdatedAt:     now,
		})
	}
	return p.store.SavePatternSummaries(summaries)
}
