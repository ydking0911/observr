package webhook_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
	"github.com/ydking0911/observr/server/internal/webhook"
)

func makeEvent(level, service, message string) storage.Event {
	return storage.Event{
		ID:        "evt_test",
		Service:   service,
		Timestamp: time.Now(),
		Type:      "log",
		Level:     level,
		Message:   message,
	}
}

func TestAlerterFiltersLevelBelow(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "error",
		Threshold: 1,
		Window:    10 * time.Second,
		Cooldown:  time.Hour,
	})
	defer a.Stop()

	a.Broadcast(makeEvent("debug", "svc", "debug msg"))
	a.Broadcast(makeEvent("info", "svc", "info msg"))
	a.Broadcast(makeEvent("warn", "svc", "warn msg"))
	time.Sleep(100 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("expected 0 webhook calls for sub-error events, got %d", n)
	}
}

func TestAlerterSendsOnThreshold(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "error",
		Threshold: 3,
		Window:    10 * time.Second,
		Cooldown:  time.Hour,
	})
	defer a.Stop()

	a.Broadcast(makeEvent("error", "svc", "e1"))
	a.Broadcast(makeEvent("error", "svc", "e2"))
	time.Sleep(50 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("should not alert before threshold (2/3), got %d calls", n)
	}

	a.Broadcast(makeEvent("error", "svc", "e3"))
	time.Sleep(200 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("expected 1 webhook call after reaching threshold, got %d", n)
	}
}

func TestAlerterRespectsCooldown(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "error",
		Threshold: 1,
		Window:    10 * time.Second,
		Cooldown:  time.Hour,
	})
	defer a.Stop()

	a.Broadcast(makeEvent("error", "svc", "first"))
	time.Sleep(100 * time.Millisecond)
	a.Broadcast(makeEvent("error", "svc", "second"))
	time.Sleep(100 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("expected 1 call due to cooldown, got %d", n)
	}
}

func TestAlerterSlackPayloadHasBlocks(t *testing.T) {
	var mu sync.Mutex
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = b
		mu.Unlock()
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "error",
		Threshold: 1,
		Window:    10 * time.Second,
		Cooldown:  time.Hour,
	})
	defer a.Stop()

	a.Broadcast(makeEvent("error", "api", "db connection refused"))
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	body := captured
	mu.Unlock()

	if len(body) == 0 {
		t.Fatal("no request body received by mock Slack server")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid JSON payload: %v", err)
	}
	if _, ok := payload["blocks"]; !ok {
		t.Fatalf("Slack payload missing 'blocks' key: %s", body)
	}
}

func TestAlerterDiscordPayloadHasEmbeds(t *testing.T) {
	var mu sync.Mutex
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = b
		mu.Unlock()
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		DiscordURL: srv.URL,
		Level:      "error",
		Threshold:  1,
		Window:     10 * time.Second,
		Cooldown:   time.Hour,
	})
	defer a.Stop()

	a.Broadcast(makeEvent("error", "api", "db connection refused"))
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	body := captured
	mu.Unlock()

	if len(body) == 0 {
		t.Fatal("no request body received by mock Discord server")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid JSON payload: %v", err)
	}
	if _, ok := payload["embeds"]; !ok {
		t.Fatalf("Discord payload missing 'embeds' key: %s", body)
	}
}

func TestAlerterWarnLevelPassesWhenConfiguredForWarn(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "warn",
		Threshold: 1,
		Window:    10 * time.Second,
		Cooldown:  time.Hour,
	})
	defer a.Stop()

	a.Broadcast(makeEvent("warn", "svc", "disk usage high"))
	time.Sleep(200 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("expected 1 call for warn event with level=warn config, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// New() 기본값 적용 검증
// ---------------------------------------------------------------------------

// TestNewDefaultsApplied verifies that zero-value Config fields are replaced
// with sane defaults (Threshold=1, Level="error") so the alerter fires on the
// very first error event.
func TestNewDefaultsApplied(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()

	// Pass a completely empty Config except for the URL.
	// Threshold=0 → should default to 1, Level="" → should default to "error".
	a := webhook.New(webhook.Config{
		SlackURL: srv.URL,
	})
	defer a.Stop()

	a.Broadcast(makeEvent("error", "svc", "first error"))
	time.Sleep(200 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("expected 1 call with default config, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// levelRank: unknown 레벨은 -1 → 항상 필터링된다
// ---------------------------------------------------------------------------

// TestAlerterUnknownLevelIsFiltered ensures that events with an unrecognised
// level string are always dropped regardless of the configured minimum level.
func TestAlerterUnknownLevelIsFiltered(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "debug", // lowest known level
		Threshold: 1,
		Window:    10 * time.Second,
		Cooldown:  time.Hour,
	})
	defer a.Stop()

	a.Broadcast(makeEvent("trace", "svc", "unknown level event"))
	time.Sleep(100 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("expected 0 calls for unknown level, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// formatText: http_request 타입 포맷 검증
// ---------------------------------------------------------------------------

// TestAlerterSlackPayloadHTTPRequest verifies that http_request events include
// method, path, status code, and duration in the Slack message body.
func TestAlerterSlackPayloadHTTPRequest(t *testing.T) {
	var mu sync.Mutex
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = b
		mu.Unlock()
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "error",
		Threshold: 1,
		Window:    10 * time.Second,
		Cooldown:  time.Hour,
	})
	defer a.Stop()

	e := storage.Event{
		ID:         "evt1",
		Service:    "api",
		Timestamp:  time.Now(),
		Type:       "http_request",
		Level:      "error",
		Method:     "GET",
		Path:       "/health",
		StatusCode: 500,
		DurationMS: 42.0,
	}
	a.Broadcast(e)
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	body := captured
	mu.Unlock()

	if len(body) == 0 {
		t.Fatal("no request body received")
	}
	text := string(body)
	for _, want := range []string{"GET", "/health", "500", "42"} {
		if !strings.Contains(text, want) {
			t.Errorf("payload missing %q: %s", want, text)
		}
	}
}

// ---------------------------------------------------------------------------
// formatText: span 타입 포맷 검증
// ---------------------------------------------------------------------------

// TestAlerterSlackPayloadSpan verifies that span events include the span name
// (message field) and duration in the Slack message body.
func TestAlerterSlackPayloadSpan(t *testing.T) {
	var mu sync.Mutex
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = b
		mu.Unlock()
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "error",
		Threshold: 1,
		Window:    10 * time.Second,
		Cooldown:  time.Hour,
	})
	defer a.Stop()

	e := storage.Event{
		ID:         "evt2",
		Service:    "worker",
		Timestamp:  time.Now(),
		Type:       "span",
		Level:      "error",
		Message:    "db.query",
		DurationMS: 120.0,
	}
	a.Broadcast(e)
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	body := captured
	mu.Unlock()

	if len(body) == 0 {
		t.Fatal("no request body received")
	}
	text := string(body)
	for _, want := range []string{"db.query", "120"} {
		if !strings.Contains(text, want) {
			t.Errorf("payload missing %q: %s", want, text)
		}
	}
}

// ---------------------------------------------------------------------------
// formatText: count > 1 시 countLine 포함 여부
// ---------------------------------------------------------------------------

// TestAlerterSlackPayloadCountLine verifies that when multiple qualifying
// events accumulate before the first alert, the payload body contains the
// "N <level> events in the last …" summary line.
func TestAlerterSlackPayloadCountLine(t *testing.T) {
	var mu sync.Mutex
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = b
		mu.Unlock()
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "error",
		Threshold: 3,
		Window:    10 * time.Second,
		Cooldown:  time.Hour,
	})
	defer a.Stop()

	for i := 0; i < 3; i++ {
		a.Broadcast(makeEvent("error", "svc", "boom"))
	}
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	body := captured
	mu.Unlock()

	if len(body) == 0 {
		t.Fatal("no request body received")
	}
	text := string(body)
	// count=3 → countLine should contain "3 error events in the last"
	if !strings.Contains(text, "3") {
		t.Errorf("expected count line with '3' in payload, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// Discord: warn イベントは yellow カラー
// ---------------------------------------------------------------------------

// TestAlerterDiscordWarnColor verifies that warn-level events produce a Discord
// embed with color 0xF1C40F (yellow/gold, decimal 15772687) rather than red.
func TestAlerterDiscordWarnColor(t *testing.T) {
	var mu sync.Mutex
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = b
		mu.Unlock()
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		DiscordURL: srv.URL,
		Level:      "warn",
		Threshold:  1,
		Window:     10 * time.Second,
		Cooldown:   time.Hour,
	})
	defer a.Stop()

	a.Broadcast(makeEvent("warn", "svc", "disk usage high"))
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	body := captured
	mu.Unlock()

	if len(body) == 0 {
		t.Fatal("no request body received")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	embeds, ok := payload["embeds"].([]any)
	if !ok || len(embeds) == 0 {
		t.Fatal("missing embeds array")
	}
	embed := embeds[0].(map[string]any)
	colorVal, ok := embed["color"]
	if !ok {
		t.Fatal("missing color field in embed")
	}
	// JSON numbers unmarshal to float64.
	colorFloat, ok := colorVal.(float64)
	if !ok {
		t.Fatalf("color is not a number: %T %v", colorVal, colorVal)
	}
	const wantYellow = 0xF1C40F // 15772687
	if int(colorFloat) != wantYellow {
		t.Errorf("expected yellow color %d for warn, got %d", wantYellow, int(colorFloat))
	}
}

// ---------------------------------------------------------------------------
// Discord: error 이벤트는 red 컬러
// ---------------------------------------------------------------------------

// TestAlerterDiscordErrorColor verifies that error-level events produce a
// Discord embed with color 0xE74C3C (red, decimal 15158332).
func TestAlerterDiscordErrorColor(t *testing.T) {
	var mu sync.Mutex
	var captured []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = b
		mu.Unlock()
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		DiscordURL: srv.URL,
		Level:      "error",
		Threshold:  1,
		Window:     10 * time.Second,
		Cooldown:   time.Hour,
	})
	defer a.Stop()

	a.Broadcast(makeEvent("error", "svc", "crash"))
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	body := captured
	mu.Unlock()

	if len(body) == 0 {
		t.Fatal("no request body received")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	embeds, ok := payload["embeds"].([]any)
	if !ok || len(embeds) == 0 {
		t.Fatal("missing embeds array")
	}
	embed := embeds[0].(map[string]any)
	colorVal, ok := embed["color"]
	if !ok {
		t.Fatal("missing color field in embed")
	}
	colorFloat, ok := colorVal.(float64)
	if !ok {
		t.Fatalf("color is not a number: %T %v", colorVal, colorVal)
	}
	const wantRed = 0xE74C3C // 15158332
	if int(colorFloat) != wantRed {
		t.Errorf("expected red color %d for error, got %d", wantRed, int(colorFloat))
	}
}

// ---------------------------------------------------------------------------
// 쿨다운 만료 후 재알림
// ---------------------------------------------------------------------------

// TestAlerterFiresAfterCooldownExpires verifies that after the cooldown period
// passes, the next qualifying event triggers a second alert.
func TestAlerterFiresAfterCooldownExpires(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()

	cooldown := 100 * time.Millisecond
	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "error",
		Threshold: 1,
		Window:    10 * time.Second,
		Cooldown:  cooldown,
	})
	defer a.Stop()

	// First alert.
	a.Broadcast(makeEvent("error", "svc", "first"))
	time.Sleep(150 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("expected 1 call after first event, got %d", n)
	}

	// Wait for cooldown to expire, then send another event.
	time.Sleep(cooldown + 50*time.Millisecond)
	a.Broadcast(makeEvent("error", "svc", "second"))
	time.Sleep(150 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("expected 2 calls after cooldown expired, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// 윈도우 밖 이벤트는 임계값 카운트에서 제외된다
// ---------------------------------------------------------------------------

// TestAlerterWindowEviction verifies that events older than the configured
// Window are evicted from the recent-timestamps buffer and therefore do not
// contribute to the threshold count.
func TestAlerterWindowEviction(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()

	window := 150 * time.Millisecond
	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "error",
		Threshold: 2,
		Window:    window,
		Cooldown:  time.Hour,
	})
	defer a.Stop()

	// First event — inside window, but not yet at threshold.
	a.Broadcast(makeEvent("error", "svc", "e1"))
	// Let the window expire so the first event is evicted.
	time.Sleep(window + 50*time.Millisecond)

	// Second event arrives after the window: recentTs should only contain this
	// single timestamp, still below threshold=2, so no alert.
	a.Broadcast(makeEvent("error", "svc", "e2"))
	time.Sleep(150 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("expected 0 alerts (first event evicted from window), got %d", n)
	}
}

// ---------------------------------------------------------------------------
// HTTP 4xx/5xx 응답 → post() 에러 처리 (로그만 출력, 패닉 없음)
// ---------------------------------------------------------------------------

// TestAlerterHandlesHTTPErrorResponse verifies that a non-2xx response from
// the webhook endpoint does not cause a panic or block the alerter; the error
// is logged and processing continues normally.
func TestAlerterHandlesHTTPErrorResponse(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "error",
		Threshold: 1,
		Window:    10 * time.Second,
		Cooldown:  time.Hour,
	})
	defer a.Stop()

	a.Broadcast(makeEvent("error", "svc", "boom"))
	time.Sleep(200 * time.Millisecond)

	// The server should have received the request even though it returned 500.
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("expected server to receive 1 request, got %d", n)
	}
	// No panic = test passes implicitly.
}

// ---------------------------------------------------------------------------
// SlackURL + DiscordURL 동시 설정 → 양쪽 모두 호출
// ---------------------------------------------------------------------------

// TestAlerterSendsBothSlackAndDiscord verifies that when both SlackURL and
// DiscordURL are configured, a single qualifying event triggers exactly one
// request to each endpoint.
func TestAlerterSendsBothSlackAndDiscord(t *testing.T) {
	var slackCalls, discordCalls int32

	slackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&slackCalls, 1)
	}))
	defer slackSrv.Close()

	discordSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&discordCalls, 1)
	}))
	defer discordSrv.Close()

	a := webhook.New(webhook.Config{
		SlackURL:   slackSrv.URL,
		DiscordURL: discordSrv.URL,
		Level:      "error",
		Threshold:  1,
		Window:     10 * time.Second,
		Cooldown:   time.Hour,
	})
	defer a.Stop()

	a.Broadcast(makeEvent("error", "svc", "critical failure"))
	time.Sleep(300 * time.Millisecond)

	if n := atomic.LoadInt32(&slackCalls); n != 1 {
		t.Errorf("expected 1 Slack call, got %d", n)
	}
	if n := atomic.LoadInt32(&discordCalls); n != 1 {
		t.Errorf("expected 1 Discord call, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// Stop() 이후 이벤트는 무시된다
// ---------------------------------------------------------------------------

// TestAlerterStopPreventsProcessing verifies that events broadcast after
// Stop() is called are not forwarded to the webhook endpoint.
func TestAlerterStopPreventsProcessing(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
	}))
	defer srv.Close()

	a := webhook.New(webhook.Config{
		SlackURL:  srv.URL,
		Level:     "error",
		Threshold: 1,
		Window:    10 * time.Second,
		Cooldown:  time.Hour,
	})

	a.Stop()
	// Give the goroutine time to exit.
	time.Sleep(50 * time.Millisecond)

	// After Stop(), the queue channel is still writable (buffered), but the
	// run() goroutine has exited, so queued events are never processed.
	a.Broadcast(makeEvent("error", "svc", "post-stop event"))
	time.Sleep(150 * time.Millisecond)

	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("expected 0 calls after Stop(), got %d", n)
	}
}
