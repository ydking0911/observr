package storage

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func newVerifyTestStore(t *testing.T) *Store {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "observr-*.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func collectVerify(t *testing.T, s *Store) []VerifyEvent {
	t.Helper()
	var rows []VerifyEvent
	if err := s.ForEachVerifyEvent(func(e VerifyEvent) error {
		rows = append(rows, e)
		return nil
	}); err != nil {
		t.Fatalf("ForEachVerifyEvent: %v", err)
	}
	return rows
}

func TestInsertStoresHashChainRowsForVerify(t *testing.T) {
	s := newVerifyTestStore(t)

	events := []Event{
		{ID: "evt_a", Service: "svc", Timestamp: time.Unix(10, 0).UTC(), Type: "log", Level: "info", Message: "first"},
		{ID: "evt_b", Service: "svc", Timestamp: time.Unix(5, 0).UTC(), Type: "log", Level: "warn", Message: "second"},
	}
	if err := s.Insert(events); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	rows := collectVerify(t, s)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].ID != "evt_a" || rows[1].ID != "evt_b" {
		t.Fatalf("verify must use insertion order, got %q then %q", rows[0].ID, rows[1].ID)
	}
	for _, row := range rows {
		if row.RawJSON == "" {
			t.Fatalf("expected raw_json for %s", row.ID)
		}
		if row.Hash == "" {
			t.Fatalf("expected hash for %s", row.ID)
		}
	}
	if rows[1].Hash == hashEvent(rows[1].RawJSON, "") {
		t.Fatal("second row hash did not include previous hash")
	}

	meta, err := s.ChainMeta()
	if err != nil {
		t.Fatalf("ChainMeta: %v", err)
	}
	if meta == nil {
		t.Fatal("expected chain metadata")
	}
	if meta.Count != 2 {
		t.Fatalf("meta.Count = %d, want 2", meta.Count)
	}
	if meta.HeadHash != rows[1].Hash {
		t.Fatalf("meta.HeadHash = %q, want %q", meta.HeadHash, rows[1].Hash)
	}
	if meta.LastID != "evt_b" {
		t.Fatalf("meta.LastID = %q, want evt_b", meta.LastID)
	}
}

func TestForEachVerifyEventExposesLegacyRowsAsEmpty(t *testing.T) {
	s := newVerifyTestStore(t)

	_, err := s.db.Exec(`
		INSERT INTO events
		  (id, service, timestamp, type, level, message, attributes)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "evt_legacy", "svc", time.Unix(1, 0).UTC().Format(time.RFC3339Nano), "log", "info", "legacy", "{}")
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	rows := collectVerify(t, s)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RawJSON != "" || rows[0].Hash != "" {
		t.Fatalf("legacy row should expose empty raw_json/hash, got %+v", rows[0])
	}
}

func TestHashEventIsDeterministic(t *testing.T) {
	got := hashEvent(`{"id":"evt_a"}`, "previous")
	if got == "" {
		t.Fatal("expected non-empty hash")
	}
	if got != hashEvent(`{"id":"evt_a"}`, "previous") {
		t.Fatal("hashEvent is not deterministic")
	}
	if got == hashEvent(`{"id":"evt_a"}`, "") {
		t.Fatal("hashEvent did not include previous hash")
	}
}

// I5: concurrent inserts are serialized by writeMu, so the rows must still form
// a single valid chain when recomputed in insertion (rowid) order.
func TestConcurrentInsertsProduceValidChain(t *testing.T) {
	s := newVerifyTestStore(t)

	const goroutines = 8
	const perGoroutine = 25
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				ev := Event{
					Service:   "svc",
					Timestamp: time.Now().UTC(),
					Type:      "log",
					Level:     "info",
					Message:   fmt.Sprintf("g%d-i%d", g, i),
				}
				if err := s.Insert([]Event{ev}); err != nil {
					t.Errorf("Insert: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	rows := collectVerify(t, s)
	if len(rows) != goroutines*perGoroutine {
		t.Fatalf("expected %d rows, got %d", goroutines*perGoroutine, len(rows))
	}
	prev := ""
	for i, row := range rows {
		if row.Hash != hashEvent(row.RawJSON, prev) {
			t.Fatalf("row %d (%s) breaks the chain", i, row.ID)
		}
		prev = row.Hash
	}

	meta, err := s.ChainMeta()
	if err != nil {
		t.Fatalf("ChainMeta: %v", err)
	}
	if meta == nil || meta.Count != goroutines*perGoroutine {
		t.Fatalf("meta.Count = %+v, want %d", meta, goroutines*perGoroutine)
	}
}

func TestDeleteBeforeAdvancesChainBaseAnchor(t *testing.T) {
	s := newVerifyTestStore(t)
	events := []Event{
		{ID: "evt_old", Service: "svc", Timestamp: time.Unix(10, 0).UTC(), Type: "log", Level: "info", Message: "old"},
		{ID: "evt_keep1", Service: "svc", Timestamp: time.Unix(20, 0).UTC(), Type: "log", Level: "info", Message: "keep1"},
		{ID: "evt_keep2", Service: "svc", Timestamp: time.Unix(30, 0).UTC(), Type: "log", Level: "info", Message: "keep2"},
	}
	if err := s.Insert(events); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	before := collectVerify(t, s)

	deleted, err := s.DeleteBefore(time.Unix(15, 0).UTC())
	if err != nil {
		t.Fatalf("DeleteBefore: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	meta, err := s.ChainMeta()
	if err != nil {
		t.Fatalf("ChainMeta: %v", err)
	}
	if meta == nil {
		t.Fatal("expected chain metadata")
	}
	if meta.BaseRowID != before[0].RowID {
		t.Fatalf("BaseRowID = %d, want %d", meta.BaseRowID, before[0].RowID)
	}
	if meta.BasePrevHash != before[0].Hash {
		t.Fatalf("BasePrevHash = %q, want %q", meta.BasePrevHash, before[0].Hash)
	}
	if meta.Count != 2 {
		t.Fatalf("Count = %d, want 2", meta.Count)
	}
	if meta.HeadHash != before[2].Hash {
		t.Fatalf("HeadHash = %q, want %q", meta.HeadHash, before[2].Hash)
	}
}

func TestOpenBackfillsChainMetaForExistingHashedRows(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "observr-*.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	events := []Event{
		{ID: "evt_a", Service: "svc", Timestamp: time.Unix(10, 0).UTC(), Type: "log", Level: "info", Message: "a"},
		{ID: "evt_b", Service: "svc", Timestamp: time.Unix(20, 0).UTC(), Type: "log", Level: "info", Message: "b"},
	}
	if err := s.Insert(events); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	rows := collectVerify(t, s)
	if _, err := s.db.Exec(`DELETE FROM chain_meta`); err != nil {
		t.Fatalf("delete chain_meta: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	meta, err := reopened.ChainMeta()
	if err != nil {
		t.Fatalf("ChainMeta: %v", err)
	}
	if meta == nil {
		t.Fatal("expected backfilled chain metadata")
	}
	if meta.Count != 2 || meta.HeadHash != rows[1].Hash || meta.LastID != "evt_b" {
		t.Fatalf("unexpected backfilled meta: %+v", meta)
	}
}

func TestDeleteBeforeAllHashedEventsResetsChainMeta(t *testing.T) {
	s := newVerifyTestStore(t)
	now := time.Now().UTC()
	events := []Event{
		{ID: "evt_a", Service: "svc", Timestamp: now.Add(-3 * time.Hour), Type: "log", Level: "info", Message: "a"},
		{ID: "evt_b", Service: "svc", Timestamp: now.Add(-2 * time.Hour), Type: "log", Level: "info", Message: "b"},
		{ID: "evt_c", Service: "svc", Timestamp: now.Add(-1 * time.Hour), Type: "log", Level: "info", Message: "c"},
	}
	if err := s.Insert(events); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// 모든 이벤트보다 미래 cutoff → 전체 삭제
	deleted, err := s.DeleteBefore(now)
	if err != nil {
		t.Fatalf("DeleteBefore: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted = %d, want 3", deleted)
	}

	meta, err := s.ChainMeta()
	if err != nil {
		t.Fatalf("ChainMeta: %v", err)
	}
	if meta == nil {
		t.Fatal("expected chain_meta to exist after full deletion")
	}
	if meta.Count != 0 {
		t.Fatalf("meta.Count = %d after full deletion, want 0", meta.Count)
	}
}
