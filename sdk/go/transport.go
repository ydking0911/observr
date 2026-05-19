package observr

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

const (
	queueSize     = 10_000
	flushInterval = time.Second
)

// Transport batches events and POSTs them to the collector.
type Transport struct {
	url    string
	queue  chan map[string]any
	done   chan struct{}
	client *http.Client
}

// NewTransport creates a Transport targeting collectorURL.
func NewTransport(collectorURL string) *Transport {
	return &Transport{
		url:    collectorURL + "/events",
		queue:  make(chan map[string]any, queueSize),
		done:   make(chan struct{}),
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Start begins the background flush goroutine.
func (t *Transport) Start() {
	go t.loop()
}

// Enqueue adds an event to the send queue. Drops silently if full.
func (t *Transport) Enqueue(event map[string]any) {
	select {
	case t.queue <- event:
	default:
	}
}

// Shutdown flushes remaining events and stops the goroutine.
func (t *Transport) Shutdown() {
	close(t.done)
	var batch []map[string]any
	for {
		select {
		case e := <-t.queue:
			batch = append(batch, e)
		default:
			if len(batch) > 0 {
				t.flush(batch)
			}
			return
		}
	}
}

func (t *Transport) loop() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	var batch []map[string]any
	for {
		select {
		case e := <-t.queue:
			batch = append(batch, e)
		case <-ticker.C:
			if len(batch) > 0 {
				t.flush(batch)
				batch = nil
			}
		case <-t.done:
			return
		}
	}
}

func (t *Transport) flush(events []map[string]any) {
	body, err := json.Marshal(events)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
