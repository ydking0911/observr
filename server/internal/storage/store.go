// Package storage provides SQLite-backed event persistence.
package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Broadcaster is implemented by the WebSocket hub to push events in real time.
type Broadcaster interface {
	Broadcast(event Event)
}

// Event is the canonical representation of a single observability event.
type Event struct {
	ID           string         `json:"id"`
	TraceID      string         `json:"trace_id,omitempty"`
	SpanID       string         `json:"span_id,omitempty"`
	ParentSpanID string         `json:"parent_span_id,omitempty"`
	Service      string         `json:"service"`
	Timestamp    time.Time      `json:"timestamp"`
	Type         string         `json:"type"`
	Level        string         `json:"level"`
	Method       string         `json:"method,omitempty"`
	Path         string         `json:"path,omitempty"`
	StatusCode   int            `json:"status_code,omitempty"`
	DurationMS   float64        `json:"duration_ms,omitempty"`
	Message      string         `json:"message"`
	Attributes   map[string]any `json:"attributes,omitempty"`
}

// QueryFilter contains filter params for event queries.
type QueryFilter struct {
	Last    int
	Level   string
	TraceID string
	Path    string
	Since   time.Time
}

// StoreStats holds summary statistics about the database.
type StoreStats struct {
	EventCount  int64
	OldestEvent *time.Time
	DBSizeBytes int64
}

// Store wraps SQLite and exposes typed read/write methods.
type Store struct {
	db          *sql.DB
	path        string
	broadcaster Broadcaster
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SetBroadcaster(b Broadcaster) {
	s.broadcaster = b
}

// ── Write ──────────────────────────────────────────────────────────────────

func (s *Store) Insert(events []Event) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO events
		  (id, trace_id, span_id, parent_span_id, service, timestamp, type, level,
		   method, path, status_code, duration_ms, message, attributes)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range events {
		e := &events[i]
		if e.ID == "" {
			e.ID = newID()
		}
		attrs, _ := json.Marshal(e.Attributes)
		_, err = stmt.Exec(
			e.ID, e.TraceID, e.SpanID, e.ParentSpanID, e.Service,
			e.Timestamp.UTC().Format(time.RFC3339Nano),
			e.Type, e.Level,
			e.Method, e.Path, e.StatusCode, e.DurationMS,
			e.Message, string(attrs),
		)
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Broadcast to WebSocket clients
	if s.broadcaster != nil {
		for _, e := range events {
			s.broadcaster.Broadcast(e)
		}
	}

	return nil
}

// ── Read ───────────────────────────────────────────────────────────────────

func (s *Store) Query(f QueryFilter) ([]Event, error) {
	q := `SELECT id, trace_id, span_id, parent_span_id, service, timestamp, type, level,
	             method, path, status_code, duration_ms, message, attributes
	      FROM events WHERE 1=1`
	args := []any{}

	if f.Level != "" {
		q += " AND level = ?"
		args = append(args, f.Level)
	}
	if f.TraceID != "" {
		q += " AND trace_id = ?"
		args = append(args, f.TraceID)
	}
	if f.Path != "" {
		q += " AND path LIKE ?"
		args = append(args, "%"+f.Path+"%")
	}
	if !f.Since.IsZero() {
		q += " AND timestamp >= ?"
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}

	limit := 100
	if f.Last > 0 {
		limit = f.Last
	}
	q += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var tsStr, attrsStr string
		if err := rows.Scan(
			&e.ID, &e.TraceID, &e.SpanID, &e.ParentSpanID, &e.Service, &tsStr,
			&e.Type, &e.Level, &e.Method, &e.Path, &e.StatusCode,
			&e.DurationMS, &e.Message, &attrsStr,
		); err != nil {
			return nil, err
		}
		e.Timestamp, _ = time.Parse(time.RFC3339Nano, tsStr)
		if attrsStr != "" {
			_ = json.Unmarshal([]byte(attrsStr), &e.Attributes)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// ── Migration ──────────────────────────────────────────────────────────────

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id             TEXT PRIMARY KEY,
			trace_id       TEXT,
			span_id        TEXT,
			parent_span_id TEXT,
			service        TEXT NOT NULL,
			timestamp      TEXT NOT NULL,
			type           TEXT NOT NULL,
			level          TEXT NOT NULL,
			method         TEXT,
			path           TEXT,
			status_code    INTEGER,
			duration_ms    REAL,
			message        TEXT,
			attributes     TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_events_level     ON events(level);
		CREATE INDEX IF NOT EXISTS idx_events_trace_id  ON events(trace_id);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
		CREATE INDEX IF NOT EXISTS idx_events_path      ON events(path);
	`)
	if err != nil {
		return err
	}
	// Idempotent column addition for databases created before this migration.
	// Only suppress the expected "duplicate column name" error; surface anything else.
	if _, err := s.db.Exec(`ALTER TABLE events ADD COLUMN parent_span_id TEXT`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("migrate add parent_span_id: %w", err)
		}
	}
	return nil
}

// ── Retention ──────────────────────────────────────────────────────────────

// DeleteBefore removes events with a timestamp older than t and returns the
// number of deleted rows.
func (s *Store) DeleteBefore(t time.Time) (int64, error) {
	// Use datetime() to compare so SQLite parses both sides as timestamps
	// rather than relying on lexicographic TEXT ordering of RFC3339Nano
	// strings (which can be unreliable when the fractional-second part
	// has different widths, e.g. "...00Z" vs "...00.5Z").
	res, err := s.db.Exec(
		`DELETE FROM events WHERE datetime(timestamp) < datetime(?)`,
		t.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Vacuum reclaims disk space freed by prior deletions.
func (s *Store) Vacuum() error {
	_, err := s.db.Exec(`VACUUM`)
	return err
}

// Stats returns a summary of the current database state.
func (s *Store) Stats() (StoreStats, error) {
	var st StoreStats

	if err := s.db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&st.EventCount); err != nil {
		return st, err
	}

	var oldest sql.NullString
	if err := s.db.QueryRow(`SELECT MIN(timestamp) FROM events`).Scan(&oldest); err != nil {
		return st, err
	}
	if oldest.Valid && oldest.String != "" {
		t, parseErr := time.Parse(time.RFC3339Nano, oldest.String)
		if parseErr == nil {
			st.OldestEvent = &t
		}
	}

	if s.path != "" {
		if info, err := os.Stat(s.path); err == nil {
			st.DBSizeBytes = info.Size()
		}
	}

	return st, nil
}

// ── Helpers ────────────────────────────────────────────────────────────────

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "evt_" + hex.EncodeToString(b)
}
