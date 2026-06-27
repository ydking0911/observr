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
}
