package storage_test

import (
	"os"
	"testing"
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
)

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "observr-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	s, err := storage.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInsertAndQuery(t *testing.T) {
	s := newTestStore(t)

	events := []storage.Event{
		{
			Service:   "svc-a",
			Timestamp: time.Now().UTC(),
			Type:      "http_request",
			Level:     "info",
			Method:    "GET",
			Path:      "/health",
			Message:   "GET /health",
		},
		{
			Service:   "svc-a",
			Timestamp: time.Now().UTC(),
			Type:      "log",
			Level:     "error",
			Message:   "database timeout",
			Attributes: map[string]any{
				"user_id": "u_123",
			},
		},
	}

	if err := s.Insert(events); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := s.Query(storage.QueryFilter{Last: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
}

func TestQueryFilterByLevel(t *testing.T) {
	s := newTestStore(t)

	events := []storage.Event{
		{Service: "svc", Timestamp: time.Now().UTC(), Type: "log", Level: "info", Message: "ok"},
		{Service: "svc", Timestamp: time.Now().UTC(), Type: "log", Level: "error", Message: "fail"},
		{Service: "svc", Timestamp: time.Now().UTC(), Type: "log", Level: "error", Message: "fail2"},
	}
	s.Insert(events) //nolint:errcheck

	got, err := s.Query(storage.QueryFilter{Level: "error", Last: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 error events, got %d", len(got))
	}
	for _, e := range got {
		if e.Level != "error" {
			t.Errorf("unexpected level %q", e.Level)
		}
	}
}

func TestQueryFilterByPath(t *testing.T) {
	s := newTestStore(t)

	s.Insert([]storage.Event{ //nolint:errcheck
		{Service: "svc", Timestamp: time.Now().UTC(), Type: "http_request", Level: "info", Path: "/checkout", Message: "POST /checkout"},
		{Service: "svc", Timestamp: time.Now().UTC(), Type: "http_request", Level: "info", Path: "/users", Message: "GET /users"},
	})

	got, err := s.Query(storage.QueryFilter{Path: "/checkout", Last: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].Path != "/checkout" {
		t.Errorf("wrong path %q", got[0].Path)
	}
}

func TestQueryRespectLastLimit(t *testing.T) {
	s := newTestStore(t)

	events := make([]storage.Event, 20)
	for i := range events {
		events[i] = storage.Event{
			Service:   "svc",
			Timestamp: time.Now().UTC(),
			Type:      "log",
			Level:     "info",
			Message:   "msg",
		}
	}
	s.Insert(events) //nolint:errcheck

	got, err := s.Query(storage.QueryFilter{Last: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5, got %d", len(got))
	}
}

func TestInsertSetsIDIfEmpty(t *testing.T) {
	s := newTestStore(t)

	e := storage.Event{
		Service:   "svc",
		Timestamp: time.Now().UTC(),
		Type:      "log",
		Level:     "info",
		Message:   "hello",
	}
	s.Insert([]storage.Event{e}) //nolint:errcheck

	got, _ := s.Query(storage.QueryFilter{Last: 1})
	if got[0].ID == "" {
		t.Error("expected non-empty ID to be assigned")
	}
}

func TestAttributesRoundtrip(t *testing.T) {
	s := newTestStore(t)

	attrs := map[string]any{
		"user_id": "u_999",
		"amount":  float64(9900),
	}
	s.Insert([]storage.Event{{ //nolint:errcheck
		Service: "svc", Timestamp: time.Now().UTC(),
		Type: "log", Level: "error", Message: "payment failed",
		Attributes: attrs,
	}})

	got, _ := s.Query(storage.QueryFilter{Last: 1})
	if got[0].Attributes["user_id"] != "u_999" {
		t.Errorf("attribute user_id not preserved: %v", got[0].Attributes)
	}
}

func TestPatternSummariesRoundtrip(t *testing.T) {
	s := newTestStore(t)

	summaries := []storage.PatternSummary{
		{
			Fingerprint:   "payment timeout for user <N>",
			Count:         3,
			FirstSeen:     time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC),
			LastSeen:      time.Date(2026, 5, 18, 10, 5, 0, 0, time.UTC),
			Level:         "error",
			Services:      []string{"api", "worker"},
			Tools:         []string{"web_search"},
			Intents:       []string{"checkout"},
			Models:        []string{"gpt-5.4"},
			Trend:         "rising",
			AnomalyScore:  4.2,
			Anomaly:       true,
			BucketsJSON:   `[{"t":"2026-05-18T10:00:00Z","count":1}]`,
			SampleEventID: "evt_1",
		},
	}

	if err := s.SavePatternSummaries(summaries); err != nil {
		t.Fatalf("SavePatternSummaries: %v", err)
	}

	got, err := s.LoadPatternSummaries(10)
	if err != nil {
		t.Fatalf("LoadPatternSummaries: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(got))
	}
	if got[0].Fingerprint != summaries[0].Fingerprint {
		t.Errorf("Fingerprint = %q, want %q", got[0].Fingerprint, summaries[0].Fingerprint)
	}
	if got[0].Trend != "rising" || !got[0].Anomaly {
		t.Errorf("trend/anomaly not preserved: %+v", got[0])
	}
	if len(got[0].Services) != 2 || got[0].Services[0] != "api" {
		t.Errorf("Services = %v, want [api worker]", got[0].Services)
	}
}

func TestDeleteBefore(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	s.Insert([]storage.Event{ //nolint:errcheck
		{Service: "svc", Timestamp: old, Type: "log", Level: "info", Message: "old event"},
		{Service: "svc", Timestamp: recent, Type: "log", Level: "info", Message: "recent event"},
	})

	// Delete events older than 24h
	cutoff := now.Add(-24 * time.Hour)
	n, err := s.DeleteBefore(cutoff)
	if err != nil {
		t.Fatalf("DeleteBefore: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 deleted event, got %d", n)
	}

	remaining, err := s.Query(storage.QueryFilter{Last: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining event, got %d", len(remaining))
	}
	if remaining[0].Message != "recent event" {
		t.Errorf("wrong event remained: %q", remaining[0].Message)
	}
}

func TestDeleteBeforeUnlimited(t *testing.T) {
	s := newTestStore(t)

	s.Insert([]storage.Event{ //nolint:errcheck
		{Service: "svc", Timestamp: time.Now().Add(-72 * time.Hour).UTC(), Type: "log", Level: "info", Message: "old"},
		{Service: "svc", Timestamp: time.Now().UTC(), Type: "log", Level: "info", Message: "new"},
	})

	// Deleting with zero time should delete nothing (caller should skip when retention=0)
	n, err := s.DeleteBefore(time.Time{})
	if err != nil {
		t.Fatalf("DeleteBefore zero: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 deletions for zero cutoff, got %d", n)
	}
}

func TestDeleteBeforeAllDeleted(t *testing.T) {
	s := newTestStore(t)

	now := time.Now().UTC()

	s.Insert([]storage.Event{ //nolint:errcheck
		{Service: "svc", Timestamp: now.Add(-72 * time.Hour), Type: "log", Level: "info", Message: "very old 1"},
		{Service: "svc", Timestamp: now.Add(-48 * time.Hour), Type: "log", Level: "info", Message: "very old 2"},
	})

	// cutoff이 현재 시각 → 모든 이벤트가 삭제되어야 함
	n, err := s.DeleteBefore(now)
	if err != nil {
		t.Fatalf("DeleteBefore: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 deleted events, got %d", n)
	}

	remaining, err := s.Query(storage.QueryFilter{Last: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected 0 remaining events, got %d", len(remaining))
	}
}

func TestDeleteBeforeEmptyTable(t *testing.T) {
	s := newTestStore(t)

	// 빈 테이블에서 DeleteBefore 호출 → 0 반환
	n, err := s.DeleteBefore(time.Now().UTC())
	if err != nil {
		t.Fatalf("DeleteBefore on empty table: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 deleted from empty table, got %d", n)
	}
}

func TestStats(t *testing.T) {
	s := newTestStore(t)

	st, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats on empty store: %v", err)
	}
	if st.EventCount != 0 {
		t.Errorf("expected 0 events, got %d", st.EventCount)
	}
	if st.OldestEvent != nil {
		t.Errorf("expected nil OldestEvent for empty store")
	}

	old := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	s.Insert([]storage.Event{ //nolint:errcheck
		{Service: "svc", Timestamp: old, Type: "log", Level: "info", Message: "first"},
		{Service: "svc", Timestamp: time.Now().UTC(), Type: "log", Level: "info", Message: "second"},
	})

	st, err = s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.EventCount != 2 {
		t.Errorf("expected 2 events, got %d", st.EventCount)
	}
	if st.OldestEvent == nil {
		t.Fatal("expected non-nil OldestEvent")
	}
	if !st.OldestEvent.Truncate(time.Second).Equal(old) {
		t.Errorf("OldestEvent mismatch: got %v, want %v", st.OldestEvent, old)
	}
	if st.DBSizeBytes <= 0 {
		t.Errorf("expected DBSizeBytes > 0 after insert, got %d", st.DBSizeBytes)
	}
}

func TestStatsSingleEvent(t *testing.T) {
	s := newTestStore(t)

	ts := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)
	s.Insert([]storage.Event{ //nolint:errcheck
		{Service: "svc", Timestamp: ts, Type: "log", Level: "info", Message: "only one"},
	})

	st, err := s.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.EventCount != 1 {
		t.Errorf("expected 1 event, got %d", st.EventCount)
	}
	if st.OldestEvent == nil {
		t.Fatal("expected non-nil OldestEvent")
	}
	if !st.OldestEvent.Truncate(time.Second).Equal(ts) {
		t.Errorf("OldestEvent mismatch: got %v, want %v", st.OldestEvent, ts)
	}
}

func TestParentSpanIDRoundtrip(t *testing.T) {
	s := newTestStore(t)

	s.Insert([]storage.Event{{ //nolint:errcheck
		Service:      "svc",
		Timestamp:    time.Now().UTC(),
		Type:         "span",
		Level:        "info",
		Message:      "child.op",
		TraceID:      "trace-abc",
		SpanID:       "span-child",
		ParentSpanID: "span-parent",
	}})

	got, err := s.Query(storage.QueryFilter{Last: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ParentSpanID != "span-parent" {
		t.Errorf("ParentSpanID not preserved: got %q", got[0].ParentSpanID)
	}
}

func TestVacuum(t *testing.T) {
	s := newTestStore(t)

	// 데이터 삽입 후 삭제하여 vacuum이 의미 있는 상황 만들기
	now := time.Now().UTC()
	s.Insert([]storage.Event{ //nolint:errcheck
		{Service: "svc", Timestamp: now.Add(-1 * time.Hour), Type: "log", Level: "info", Message: "to be deleted"},
	})
	_, _ = s.DeleteBefore(now)

	// Vacuum이 에러 없이 실행되는지 확인
	if err := s.Vacuum(); err != nil {
		t.Errorf("Vacuum returned unexpected error: %v", err)
	}
}
