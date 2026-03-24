package tail_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
	internaltail "github.com/ydking0911/observr/server/internal/tail"
)

// sseLines opens a /tail SSE stream and returns a channel of received lines.
// The stream is cancelled when ctx is done.
func sseLines(t *testing.T, hub *internaltail.Hub, query string, ctx context.Context) <-chan string {
	t.Helper()
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)

	url := srv.URL + "/tail"
	if query != "" {
		url += "?" + query
	}

	ch := make(chan string, 256)
	go func() {
		defer close(ch)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			ch <- scanner.Text()
		}
	}()
	// Give the server time to register the subscriber
	time.Sleep(40 * time.Millisecond)
	return ch
}

// collect reads from ch until the given timeout, returning all lines.
func collect(ch <-chan string, timeout time.Duration) []string {
	var lines []string
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return lines
			}
			lines = append(lines, line)
		case <-deadline:
			return lines
		}
	}
}

func TestSSEConnectsAndReceivesEvent(t *testing.T) {
	hub := internaltail.NewHub()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch := sseLines(t, hub, "", ctx)

	hub.Broadcast(storage.Event{
		ID: "1", Service: "svc", Timestamp: time.Now(),
		Type: "log", Level: "info", Message: "hello tail",
	})

	lines := collect(ch, 500*time.Millisecond)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, "hello tail") {
		t.Fatalf("expected 'hello tail' in SSE output:\n%s", combined)
	}
	if !strings.Contains(combined, "data: ") {
		t.Fatalf("expected 'data: ' SSE prefix:\n%s", combined)
	}
}

func TestSSEKeepAliveComment(t *testing.T) {
	hub := internaltail.NewHub()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch := sseLines(t, hub, "", ctx)
	lines := collect(ch, 200*time.Millisecond)
	combined := strings.Join(lines, "\n")
	if !strings.Contains(combined, ": connected") {
		t.Fatalf("expected keep-alive comment:\n%s", combined)
	}
}

func TestSSEFilterByLevel(t *testing.T) {
	hub := internaltail.NewHub()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch := sseLines(t, hub, "level=error", ctx)

	hub.Broadcast(storage.Event{
		ID: "1", Service: "svc", Timestamp: time.Now(),
		Type: "log", Level: "info", Message: "filtered out",
	})
	hub.Broadcast(storage.Event{
		ID: "2", Service: "svc", Timestamp: time.Now(),
		Type: "log", Level: "error", Message: "critical failure",
	})

	lines := collect(ch, 300*time.Millisecond)
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "filtered out") {
		t.Fatalf("info event should be filtered:\n%s", combined)
	}
	if !strings.Contains(combined, "critical failure") {
		t.Fatalf("error event should pass filter:\n%s", combined)
	}
}

func TestSSEFilterByService(t *testing.T) {
	hub := internaltail.NewHub()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch := sseLines(t, hub, "service=agent-a", ctx)

	hub.Broadcast(storage.Event{
		ID: "1", Service: "agent-b", Timestamp: time.Now(),
		Type: "log", Level: "info", Message: "from b",
	})
	hub.Broadcast(storage.Event{
		ID: "2", Service: "agent-a", Timestamp: time.Now(),
		Type: "log", Level: "info", Message: "from a",
	})

	lines := collect(ch, 300*time.Millisecond)
	combined := strings.Join(lines, "\n")
	if strings.Contains(combined, "from b") {
		t.Fatalf("agent-b should be filtered:\n%s", combined)
	}
	if !strings.Contains(combined, "from a") {
		t.Fatalf("agent-a should pass filter:\n%s", combined)
	}
}

func TestSSEMultipleClients(t *testing.T) {
	hub := internaltail.NewHub()
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	const numClients = 3
	channels := make([]<-chan string, numClients)
	for i := range channels {
		ch := make(chan string, 256)
		channels[i] = ch
		go func(out chan<- string) {
			defer close(out)
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/tail", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				out <- scanner.Text()
			}
		}(ch)
	}
	time.Sleep(50 * time.Millisecond) // wait for all subscribers to register

	hub.Broadcast(storage.Event{
		ID: "x", Service: "svc", Timestamp: time.Now(),
		Type: "log", Level: "info", Message: "broadcast to all",
	})

	for i, ch := range channels {
		lines := collect(ch, 300*time.Millisecond)
		combined := strings.Join(lines, "\n")
		if !strings.Contains(combined, "broadcast to all") {
			t.Errorf("client %d did not receive broadcast:\n%s", i, combined)
		}
	}
}
