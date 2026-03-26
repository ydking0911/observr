package patterns_test

import (
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
