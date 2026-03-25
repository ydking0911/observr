// Package storage provides SQLite-backed event persistence.
package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Broadcaster is implemented by the WebSocket hub to push events in real time.
type Broadcaster interface {
	Broadcast(event Event)
}

// Event is the canonical representation of a single observability event.
type Event struct {
	ID          string         `json:"id"`
	TraceID     string         `json:"trace_id,omitempty"`
	SpanID      string         `json:"span_id,omitempty"`
	Service     string         `json:"service"`
	Timestamp   time.Time      `json:"timestamp"`
	Type        string         `json:"type"`
	Level       string         `json:"level"`
	Method      string         `json:"method,omitempty"`
	Path        string         `json:"path,omitempty"`
	StatusCode  int            `json:"status_code,omitempty"`
	DurationMS  float64        `json:"duration_ms,omitempty"`
	Message     string         `json:"message"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

// QueryFilter contains filter params for event queries.
type QueryFilter struct {
	Last    int
	Level   string
	TraceID string
	Path    string
	Since   time.Time
}

// Store wraps SQLite and exposes typed read/write methods.
type Store struct {
	db          *sql.DB
	broadcaster Broadcaster
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Store{db: db}
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
		  (id, trace_id, span_id, service, timestamp, type, level,
		   method, path, status_code, duration_ms, message, attributes)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
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
			e.ID, e.TraceID, e.SpanID, e.Service,
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
	q := `SELECT id, trace_id, span_id, service, timestamp, type, level,
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
			&e.ID, &e.TraceID, &e.SpanID, &e.Service, &tsStr,
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
			id          TEXT PRIMARY KEY,
			trace_id    TEXT,
			span_id     TEXT,
			service     TEXT NOT NULL,
			timestamp   TEXT NOT NULL,
			type        TEXT NOT NULL,
			level       TEXT NOT NULL,
			method      TEXT,
			path        TEXT,
			status_code INTEGER,
			duration_ms REAL,
			message     TEXT,
			attributes  TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_events_level     ON events(level);
		CREATE INDEX IF NOT EXISTS idx_events_trace_id  ON events(trace_id);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
		CREATE INDEX IF NOT EXISTS idx_events_path      ON events(path);
	`)
	return err
}

// ── Helpers ────────────────────────────────────────────────────────────────

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "evt_" + hex.EncodeToString(b)
}
