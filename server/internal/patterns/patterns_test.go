package patterns_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ydking0911/observr/server/internal/patterns"
	"github.com/ydking0911/observr/server/internal/storage"
)

// ── Normalize ────────────────────────────────────────────────────────────────

func TestNormalizeUUID(t *testing.T) {
	in := "request 550e8400-e29b-41d4-a716-446655440000 failed"
	got := patterns.Normalize(in)
	if strings.Contains(got, "550e8400") {
		t.Errorf("UUID not normalized: %q", got)
	}
	if !strings.Contains(got, "<uuid>") {
		t.Errorf("expected <uuid> placeholder, got: %q", got)
	}
}

func TestNormalizeIP(t *testing.T) {
	in := "client 192.168.1.100 disconnected"
	got := patterns.Normalize(in)
	if !strings.Contains(got, "<ip>") {
		t.Errorf("expected <ip> placeholder, got: %q", got)
	}
	if strings.Contains(got, "192") {
		t.Errorf("IP not normalized: %q", got)
	}
}

func TestNormalizeNumber(t *testing.T) {
	in := "port 5432 connection refused"
	got := patterns.Normalize(in)
	if strings.Contains(got, "5432") {
		t.Errorf("number not normalized: %q", got)
	}
	if !strings.Contains(got, "<N>") {
		t.Errorf("expected <N> placeholder, got: %q", got)
	}
}

func TestNormalizeHex(t *testing.T) {
	in := "hash deadbeef1234abcd stored"
	got := patterns.Normalize(in)
	if !strings.Contains(got, "<hex>") {
		t.Errorf("expected <hex> placeholder, got: %q", got)
	}
}

func TestNormalizeNoChange(t *testing.T) {
	in := "no special values here"
	got := patterns.Normalize(in)
	if got != in {
		t.Errorf("Normalize(%q) = %q, want unchanged", in, got)
	}
}

// UUIDs contain hex chars; UUID replacement must happen first to avoid
// the hex regex partially matching UUID segments.
func TestNormalizeUUIDBeforeHex(t *testing.T) {
	in := "id 550e8400-e29b-41d4-a716-446655440000 ok"
	got := patterns.Normalize(in)
	if strings.Count(got, "<uuid>") != 1 {
		t.Errorf("expected exactly one <uuid>, got: %q", got)
	}
	if strings.Contains(got, "<hex>") {
		t.Errorf("UUID segments were double-replaced as <hex>: %q", got)
	}
}

func TestNormalizeEmpty(t *testing.T) {
	if got := patterns.Normalize(""); got != "" {
		t.Errorf("Normalize(\"\") = %q, want \"\"", got)
	}
}

// ── Group ────────────────────────────────────────────────────────────────────

func makeEvent(id, service, level, msg string, ts time.Time) storage.Event {
	return storage.Event{ID: id, Service: service, Level: level, Message: msg, Timestamp: ts}
}

func TestGroupCountsCorrectly(t *testing.T) {
	now := time.Now()
	events := []storage.Event{
		makeEvent("e1", "api", "error", "db connection refused at port 5432", now),
		makeEvent("e2", "api", "error", "db connection refused at port 5433", now.Add(time.Second)),
		makeEvent("e3", "api", "error", "db connection refused at port 5434", now.Add(2*time.Second)),
	}
	ps := patterns.Group(events, 1)
	if len(ps) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(ps))
	}
	if ps[0].Count != 3 {
		t.Errorf("expected count=3, got %d", ps[0].Count)
	}
}

func TestGroupMinCount(t *testing.T) {
	now := time.Now()
	events := []storage.Event{
		makeEvent("e1", "api", "error", "db connection refused at port 5432", now),
		makeEvent("e2", "api", "error", "db connection refused at port 5433", now),
		makeEvent("e3", "api", "warn", "timeout expired", now),
	}
	ps := patterns.Group(events, 2)
	if len(ps) != 1 {
		t.Fatalf("expected 1 pattern (count >= 2), got %d", len(ps))
	}
	if ps[0].Count != 2 {
		t.Errorf("expected count=2, got %d", ps[0].Count)
	}
}

func TestGroupServices(t *testing.T) {
	now := time.Now()
	events := []storage.Event{
		makeEvent("e1", "api", "error", "db connection refused at port 5432", now),
		makeEvent("e2", "worker", "error", "db connection refused at port 5433", now),
	}
	ps := patterns.Group(events, 1)
	if len(ps) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(ps))
	}
	if len(ps[0].Services) != 2 {
		t.Errorf("expected 2 services, got %v", ps[0].Services)
	}
}

func TestGroupHighestLevelWins(t *testing.T) {
	now := time.Now()
	events := []storage.Event{
		makeEvent("e1", "api", "warn", "db connection refused at port 5432", now),
		makeEvent("e2", "api", "error", "db connection refused at port 5433", now),
	}
	ps := patterns.Group(events, 1)
	if len(ps) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(ps))
	}
	if ps[0].Level != "error" {
		t.Errorf("expected level=error, got %q", ps[0].Level)
	}
}

func TestGroupSortedByCount(t *testing.T) {
	now := time.Now()
	events := []storage.Event{
		makeEvent("e1", "api", "error", "timeout expired", now),
		makeEvent("e2", "api", "error", "db connection refused at port 5432", now),
		makeEvent("e3", "api", "error", "db connection refused at port 5433", now),
	}
	ps := patterns.Group(events, 1)
	if len(ps) < 2 {
		t.Fatal("expected at least 2 patterns")
	}
	if ps[0].Count < ps[1].Count {
		t.Errorf("patterns not sorted by count desc: first=%d, second=%d", ps[0].Count, ps[1].Count)
	}
}

func TestGroupTimestamps(t *testing.T) {
	t1 := time.Now().Add(-10 * time.Minute)
	t2 := time.Now().Add(-5 * time.Minute)
	t3 := time.Now()
	events := []storage.Event{
		makeEvent("e1", "api", "error", "db connection refused at port 5432", t2),
		makeEvent("e2", "api", "error", "db connection refused at port 5433", t1),
		makeEvent("e3", "api", "error", "db connection refused at port 5434", t3),
	}
	ps := patterns.Group(events, 1)
	if len(ps) != 1 {
		t.Fatal("expected 1 pattern")
	}
	if !ps[0].FirstSeen.Equal(t1) {
		t.Errorf("FirstSeen: got %v, want %v", ps[0].FirstSeen, t1)
	}
	if !ps[0].LastSeen.Equal(t3) {
		t.Errorf("LastSeen: got %v, want %v", ps[0].LastSeen, t3)
	}
}

func TestGroupEmptyMessageSkipped(t *testing.T) {
	events := []storage.Event{
		makeEvent("e1", "api", "error", "", time.Now()),
	}
	ps := patterns.Group(events, 1)
	if len(ps) != 0 {
		t.Errorf("expected 0 patterns for empty message, got %d", len(ps))
	}
}

func TestGroupServicesSorted(t *testing.T) {
	now := time.Now()
	events := []storage.Event{
		makeEvent("e1", "worker", "error", "db connection refused at port 5432", now),
		makeEvent("e2", "api", "error", "db connection refused at port 5433", now),
	}
	ps := patterns.Group(events, 1)
	if len(ps) != 1 {
		t.Fatal("expected 1 pattern")
	}
	if ps[0].Services[0] != "api" {
		t.Errorf("services not sorted: %v", ps[0].Services)
	}
}

func TestGroupSampleEventIDSet(t *testing.T) {
	now := time.Now()
	events := []storage.Event{
		makeEvent("evt_first", "api", "error", "db connection refused at port 5432", now),
	}
	ps := patterns.Group(events, 1)
	if len(ps) != 1 {
		t.Fatal("expected 1 pattern")
	}
	if ps[0].SampleEventID == "" {
		t.Error("expected SampleEventID to be set")
	}
}

func TestGroupWithOptionsAddsBucketsTrendAnomalyAndAgentAttrs(t *testing.T) {
	start := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	events := []storage.Event{
		{ID: "e1", Service: "api", Level: "error", Message: "tool timeout after 100 ms", Timestamp: start.Add(time.Minute), Attributes: map[string]any{"agent.tool": "web_search", "agent.intent": "research", "agent.model": "gpt-5.4"}},
		{ID: "e2", Service: "api", Level: "error", Message: "tool timeout after 200 ms", Timestamp: start.Add(6 * time.Minute), Attributes: map[string]any{"agent.tool": "web_search", "agent.intent": "research", "agent.model": "gpt-5.4"}},
		{ID: "e3", Service: "api", Level: "error", Message: "tool timeout after 300 ms", Timestamp: start.Add(11 * time.Minute), Attributes: map[string]any{"agent.tool": "web_search", "agent.intent": "checkout", "agent.model": "gpt-5.4"}},
		{ID: "e4", Service: "api", Level: "error", Message: "tool timeout after 400 ms", Timestamp: start.Add(12 * time.Minute), Attributes: map[string]any{"agent.tool": "web_search", "agent.intent": "checkout", "agent.model": "gpt-5.4"}},
		{ID: "e5", Service: "api", Level: "error", Message: "tool timeout after 500 ms", Timestamp: start.Add(13 * time.Minute), Attributes: map[string]any{"agent.tool": "web_search", "agent.intent": "checkout", "agent.model": "gpt-5.4"}},
		{ID: "e6", Service: "api", Level: "error", Message: "tool timeout after 600 ms", Timestamp: start.Add(14 * time.Minute), Attributes: map[string]any{"agent.tool": "web_search", "agent.intent": "checkout", "agent.model": "gpt-5.4"}},
	}

	ps := patterns.GroupWithOptions(events, patterns.GroupOptions{
		MinCount:         1,
		BucketStart:      start,
		BucketSize:       5 * time.Minute,
		BucketCount:      3,
		IncludeBuckets:   true,
		AnomalyThreshold: 2,
	})

	if len(ps) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(ps))
	}
	p := ps[0]
	if p.Trend != "rising" {
		t.Errorf("Trend = %q, want rising", p.Trend)
	}
	if !p.Anomaly {
		t.Errorf("expected anomaly=true, score=%v", p.AnomalyScore)
	}
	if len(p.Buckets) != 3 || p.Buckets[2].Count != 4 {
		t.Errorf("Buckets = %+v, want 3 buckets with last count 4", p.Buckets)
	}
	if len(p.Tools) != 1 || p.Tools[0] != "web_search" {
		t.Errorf("Tools = %v, want [web_search]", p.Tools)
	}
	if len(p.Intents) != 2 || p.Intents[0] != "checkout" || p.Intents[1] != "research" {
		t.Errorf("Intents = %v, want [checkout research]", p.Intents)
	}
	if len(p.Models) != 1 || p.Models[0] != "gpt-5.4" {
		t.Errorf("Models = %v, want [gpt-5.4]", p.Models)
	}
}

func TestGroupWithOptionsGroupsByTool(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	events := []storage.Event{
		{ID: "e1", Service: "api", Level: "error", Message: "timeout 100", Timestamp: now, Attributes: map[string]any{"agent.tool": "web_search"}},
		{ID: "e2", Service: "api", Level: "error", Message: "timeout 200", Timestamp: now, Attributes: map[string]any{"agent.tool": "db_query"}},
	}

	ps := patterns.GroupWithOptions(events, patterns.GroupOptions{MinCount: 1, GroupBy: "tool"})
	if len(ps) != 2 {
		t.Fatalf("expected 2 grouped patterns, got %d", len(ps))
	}
	seen := map[string]bool{}
	for _, p := range ps {
		if p.GroupBy != "tool" {
			t.Errorf("GroupBy = %q, want tool", p.GroupBy)
		}
		seen[p.GroupValue] = true
	}
	if !seen["web_search"] || !seen["db_query"] {
		t.Errorf("group values = %v, want web_search and db_query", seen)
	}
}

func newTestStore(t *testing.T, events ...storage.Event) *storage.Store {
	t.Helper()
	s, err := storage.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Insert(events); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestHandlerIncludesBucketsOnlyWhenRequested(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t,
		storage.Event{Service: "api", Level: "error", Message: "timeout 100", Timestamp: now.Add(-2 * time.Minute)},
		storage.Event{Service: "api", Level: "error", Message: "timeout 200", Timestamp: now.Add(-1 * time.Minute)},
	)
	handler := patterns.NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/patterns?since=15m&buckets=true", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var withBuckets []patterns.Pattern
	if err := json.NewDecoder(rec.Body).Decode(&withBuckets); err != nil {
		t.Fatal(err)
	}
	if len(withBuckets) != 1 || len(withBuckets[0].Buckets) == 0 {
		t.Fatalf("expected buckets in response, got %+v", withBuckets)
	}

	req = httptest.NewRequest(http.MethodGet, "/patterns?since=15m", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	var withoutBuckets []patterns.Pattern
	if err := json.NewDecoder(rec.Body).Decode(&withoutBuckets); err != nil {
		t.Fatal(err)
	}
	if len(withoutBuckets) != 1 || withoutBuckets[0].Buckets != nil {
		t.Fatalf("expected buckets omitted by default, got %+v", withoutBuckets)
	}
}
