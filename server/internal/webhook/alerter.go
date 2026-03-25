// Package webhook delivers Slack and Discord alerts when event thresholds are exceeded.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
)

// Config holds webhook alerter configuration.
type Config struct {
	SlackURL   string
	DiscordURL string
	// Level is the minimum event level that triggers an alert (debug|info|warn|error).
	Level string
	// Threshold is the number of matching events within Window before an alert fires.
	Threshold int
	// Window is the time window used for threshold counting.
	Window time.Duration
	// Cooldown is the minimum time between consecutive alerts.
	Cooldown time.Duration
}

// Alerter implements storage.Broadcaster and sends alerts to Slack and/or Discord.
type Alerter struct {
	cfg       Config
	queue     chan storage.Event
	mu        sync.Mutex
	recentTs  []time.Time
	lastAlert time.Time
	client    *http.Client
	stopCh    chan struct{}
	stopOnce  sync.Once
}

// New creates an Alerter and starts its background processing goroutine.
func New(cfg Config) *Alerter {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 1
	}
	if cfg.Window <= 0 {
		cfg.Window = 60 * time.Second
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 5 * time.Minute
	}
	if cfg.Level == "" {
		cfg.Level = "error"
	}
	a := &Alerter{
		cfg:    cfg,
		queue:  make(chan storage.Event, 256),
		client: &http.Client{Timeout: 5 * time.Second},
		stopCh: make(chan struct{}),
	}
	go a.run()
	return a
}

// Stop shuts down the background goroutine gracefully.
// Safe to call multiple times.
func (a *Alerter) Stop() {
	a.stopOnce.Do(func() { close(a.stopCh) })
}

// Broadcast implements storage.Broadcaster. Events below the configured level are
// dropped immediately; matching events are queued for async processing.
func (a *Alerter) Broadcast(e storage.Event) {
	if levelRank(e.Level) < levelRank(a.cfg.Level) {
		return
	}
	select {
	case a.queue <- e:
	default:
		// slow — drop silently, same pattern as tail/hub
	}
}

func (a *Alerter) run() {
	for {
		select {
		case <-a.stopCh:
			return
		case e := <-a.queue:
			a.handleEvent(e)
		}
	}
}

func (a *Alerter) handleEvent(e storage.Event) {
	now := time.Now()

	a.mu.Lock()

	// Compact recentTs: keep only timestamps within the window.
	cutoff := now.Add(-a.cfg.Window)
	n := 0
	for _, ts := range a.recentTs {
		if !ts.Before(cutoff) {
			a.recentTs[n] = ts
			n++
		}
	}
	a.recentTs = append(a.recentTs[:n], now)

	if len(a.recentTs) < a.cfg.Threshold {
		a.mu.Unlock()
		return
	}
	if now.Sub(a.lastAlert) < a.cfg.Cooldown {
		a.mu.Unlock()
		return
	}

	a.lastAlert = now
	count := len(a.recentTs)
	a.mu.Unlock() // release before spawning goroutine

	go a.send(e, count)
}

func (a *Alerter) send(e storage.Event, count int) {
	if a.cfg.SlackURL != "" {
		if err := a.sendSlack(e, count); err != nil {
			log.Printf("webhook: slack error: %v", err)
		}
	}
	if a.cfg.DiscordURL != "" {
		if err := a.sendDiscord(e, count); err != nil {
			log.Printf("webhook: discord error: %v", err)
		}
	}
}

func (a *Alerter) sendSlack(e storage.Event, count int) error {
	payload := map[string]any{
		"blocks": []map[string]any{
			{
				"type": "section",
				"text": map[string]any{
					"type": "mrkdwn",
					"text": formatText(e, count, a.cfg.Window),
				},
			},
		},
	}
	return a.post(a.cfg.SlackURL, payload)
}

func (a *Alerter) sendDiscord(e storage.Event, count int) error {
	color := 0xE74C3C // red
	if e.Level == "warn" {
		color = 0xF1C40F // yellow/gold
	}
	payload := map[string]any{
		"embeds": []map[string]any{
			{
				"title":       "observr alert",
				"description": formatText(e, count, a.cfg.Window),
				"color":       color,
			},
		},
	}
	return a.post(a.cfg.DiscordURL, payload)
}

func (a *Alerter) post(url string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Drain body to enable HTTP keep-alive connection reuse.
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// formatText builds the human-readable alert message shared by Slack and Discord.
func formatText(e storage.Event, count int, window time.Duration) string {
	var summary string
	switch e.Type {
	case "http_request":
		summary = fmt.Sprintf("`%s` — %s %s → %d (%.0fms)", e.Service, e.Method, e.Path, e.StatusCode, e.DurationMS)
	case "span":
		summary = fmt.Sprintf("`%s` — span `%s` (%.0fms)", e.Service, e.Message, e.DurationMS)
	default:
		summary = fmt.Sprintf("`%s` — %s", e.Service, e.Message)
	}

	countLine := ""
	if count > 1 {
		countLine = fmt.Sprintf("\n_%d %s events in the last %s_", count, e.Level, window)
	}

	return fmt.Sprintf(":rotating_light: *[%s]* %s%s", strings.ToUpper(e.Level), summary, countLine)
}

// levelRank maps level names to a numeric rank for comparison.
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
		log.Printf("webhook: unknown level %q", level)
		return -1
	}
}
