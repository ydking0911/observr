// Package storage provides SQLite-backed event persistence.
package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
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

// PatternSummary is the SQLite representation of an aggregated pattern.
type PatternSummary struct {
	Fingerprint   string
	GroupBy       string
	GroupValue    string
	Count         int
	FirstSeen     time.Time
	LastSeen      time.Time
	Level         string
	Services      []string
	Tools         []string
	Intents       []string
	Models        []string
	Trend         string
	AnomalyScore  float64
	Anomaly       bool
	BucketsJSON   string
	SampleEventID string
	UpdatedAt     time.Time
}

// Store wraps SQLite and exposes typed read/write methods.
type Store struct {
	db          *sql.DB
	path        string
	broadcaster Broadcaster
	writeMu     sync.Mutex
}

// VerifyEvent is the minimal stored form needed to recompute the audit hash chain.
type VerifyEvent struct {
	RowID   int64  `json:"rowid"`
	ID      string `json:"id"`
	RawJSON string `json:"raw_json"`
	Hash    string `json:"hash"`
}

// ChainMeta is the persisted audit-chain anchor used to detect tail truncation.
type ChainMeta struct {
	HeadHash     string
	Count        int
	LastID       string
	BaseRowID    int64
	BasePrevHash string
	UpdatedAt    time.Time
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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO events
		  (id, trace_id, span_id, parent_span_id, service, timestamp, type, level,
		   method, path, status_code, duration_ms, message, attributes, raw_json, hash)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	meta, err := loadChainMetaTx(tx)
	if err != nil {
		return err
	}
	prevHash := meta.HeadHash
	count := meta.Count
	lastID := meta.LastID

	for i := range events {
		e := &events[i]
		if e.ID == "" {
			e.ID = newID()
		}
		attrs, err := json.Marshal(e.Attributes)
		if err != nil {
			return fmt.Errorf("marshal attributes for event %d (%s): %w", i, e.ID, err)
		}
		raw, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal event %d (%s) for hash: %w", i, e.ID, err)
		}
		hash := hashEvent(string(raw), prevHash)
		_, err = stmt.Exec(
			e.ID, e.TraceID, e.SpanID, e.ParentSpanID, e.Service,
			e.Timestamp.UTC().Format(time.RFC3339Nano),
			e.Type, e.Level,
			e.Method, e.Path, e.StatusCode, e.DurationMS,
			e.Message, string(attrs), string(raw), hash,
		)
		if err != nil {
			return err
		}
		prevHash = hash
		count++
		lastID = e.ID
	}

	if len(events) > 0 {
		if err := upsertChainMetaTx(tx, ChainMeta{
			HeadHash:     prevHash,
			Count:        count,
			LastID:       lastID,
			BaseRowID:    meta.BaseRowID,
			BasePrevHash: meta.BasePrevHash,
			UpdatedAt:    time.Now().UTC(),
		}); err != nil {
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

// QueryByTrace returns all events for the given trace ID sorted by timestamp ASC.
// Returns nil (not an empty slice) when no events match.
func (s *Store) QueryByTrace(traceID string) ([]Event, error) {
	rows, err := s.db.Query(`
		SELECT id, trace_id, span_id, parent_span_id, service, timestamp, type, level,
		       method, path, status_code, duration_ms, message, attributes
		FROM events
		WHERE trace_id = ?
		ORDER BY timestamp ASC
	`, traceID)
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

// ForEachVerifyEvent streams events in insertion order (rowid ASC) and invokes
// fn for each one. It loads a single row at a time so audit verification stays
// O(1) in memory regardless of how large the audit log has grown. If fn returns
// an error, iteration stops and that error is propagated to the caller.
func (s *Store) ForEachVerifyEvent(fn func(VerifyEvent) error) error {
	rows, err := s.db.Query(`
		SELECT rowid, id, COALESCE(raw_json, ''), COALESCE(hash, '')
		FROM events
		ORDER BY rowid ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var e VerifyEvent
		if err := rows.Scan(&e.RowID, &e.ID, &e.RawJSON, &e.Hash); err != nil {
			return err
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ChainMeta returns the current persisted chain anchor.
func (s *Store) ChainMeta() (*ChainMeta, error) {
	return loadChainMetaDB(s.db)
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
			attributes     TEXT,
			raw_json       TEXT,
			hash           TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_events_level     ON events(level);
		CREATE INDEX IF NOT EXISTS idx_events_trace_id  ON events(trace_id);
		CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
		CREATE INDEX IF NOT EXISTS idx_events_path      ON events(path);

		CREATE TABLE IF NOT EXISTS patterns (
			fingerprint     TEXT NOT NULL,
			group_by        TEXT NOT NULL DEFAULT '',
			group_value     TEXT NOT NULL DEFAULT '',
			count           INTEGER NOT NULL,
			first_seen      TEXT NOT NULL,
			last_seen       TEXT NOT NULL,
			level           TEXT NOT NULL,
			services        TEXT NOT NULL,
			tools           TEXT NOT NULL,
			intents         TEXT NOT NULL,
			models          TEXT NOT NULL,
			trend           TEXT NOT NULL,
			anomaly_score   REAL NOT NULL,
			anomaly         INTEGER NOT NULL,
			buckets         TEXT NOT NULL,
			sample_event_id TEXT,
			updated_at      TEXT NOT NULL,
			PRIMARY KEY (fingerprint, group_by, group_value)
		);
		CREATE INDEX IF NOT EXISTS idx_patterns_updated_at ON patterns(updated_at);
		CREATE INDEX IF NOT EXISTS idx_patterns_anomaly    ON patterns(anomaly);

		CREATE TABLE IF NOT EXISTS chain_meta (
			id             INTEGER PRIMARY KEY CHECK (id = 1),
			head_hash      TEXT NOT NULL,
			count          INTEGER NOT NULL,
			last_id        TEXT NOT NULL,
			base_rowid     INTEGER NOT NULL,
			base_prev_hash TEXT NOT NULL,
			updated_at     TEXT NOT NULL
		);
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
	if _, err := s.db.Exec(`ALTER TABLE events ADD COLUMN raw_json TEXT`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("migrate add raw_json: %w", err)
		}
	}
	if _, err := s.db.Exec(`ALTER TABLE events ADD COLUMN hash TEXT`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("migrate add hash: %w", err)
		}
	}
	if err := s.backfillChainMeta(); err != nil {
		return err
	}
	return nil
}

// ── Pattern summaries ─────────────────────────────────────────────────────

func (s *Store) SavePatternSummaries(patterns []PatternSummary) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO patterns
		  (fingerprint, group_by, group_value, count, first_seen, last_seen, level,
		   services, tools, intents, models, trend, anomaly_score, anomaly, buckets,
		   sample_event_id, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(fingerprint, group_by, group_value) DO UPDATE SET
			count=excluded.count,
			first_seen=excluded.first_seen,
			last_seen=excluded.last_seen,
			level=excluded.level,
			services=excluded.services,
			tools=excluded.tools,
			intents=excluded.intents,
			models=excluded.models,
			trend=excluded.trend,
			anomaly_score=excluded.anomaly_score,
			anomaly=excluded.anomaly,
			buckets=excluded.buckets,
			sample_event_id=excluded.sample_event_id,
			updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, p := range patterns {
		updatedAt := p.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = time.Now().UTC()
		}
		services, _ := json.Marshal(p.Services)
		tools, _ := json.Marshal(p.Tools)
		intents, _ := json.Marshal(p.Intents)
		models, _ := json.Marshal(p.Models)
		anomaly := 0
		if p.Anomaly {
			anomaly = 1
		}
		if _, err := stmt.Exec(
			p.Fingerprint, p.GroupBy, p.GroupValue, p.Count,
			p.FirstSeen.UTC().Format(time.RFC3339Nano),
			p.LastSeen.UTC().Format(time.RFC3339Nano),
			p.Level, string(services), string(tools), string(intents), string(models),
			p.Trend, p.AnomalyScore, anomaly, p.BucketsJSON, p.SampleEventID,
			updatedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LoadPatternSummaries(limit int) ([]PatternSummary, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`
		SELECT fingerprint, group_by, group_value, count, first_seen, last_seen, level,
		       services, tools, intents, models, trend, anomaly_score, anomaly, buckets,
		       sample_event_id, updated_at
		FROM patterns
		ORDER BY anomaly DESC, count DESC, datetime(updated_at) DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PatternSummary
	for rows.Next() {
		var p PatternSummary
		var firstSeen, lastSeen, updatedAt string
		var services, tools, intents, models string
		var anomaly int
		if err := rows.Scan(
			&p.Fingerprint, &p.GroupBy, &p.GroupValue, &p.Count,
			&firstSeen, &lastSeen, &p.Level, &services, &tools, &intents, &models,
			&p.Trend, &p.AnomalyScore, &anomaly, &p.BucketsJSON,
			&p.SampleEventID, &updatedAt,
		); err != nil {
			return nil, err
		}
		var err error
		if p.FirstSeen, err = time.Parse(time.RFC3339Nano, firstSeen); err != nil {
			return nil, fmt.Errorf("parse first_seen %q: %w", firstSeen, err)
		}
		if p.LastSeen, err = time.Parse(time.RFC3339Nano, lastSeen); err != nil {
			return nil, fmt.Errorf("parse last_seen %q: %w", lastSeen, err)
		}
		if p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
			return nil, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
		}
		p.Anomaly = anomaly == 1
		if services != "" {
			if err := json.Unmarshal([]byte(services), &p.Services); err != nil {
				return nil, fmt.Errorf("parse services: %w", err)
			}
		}
		if tools != "" {
			if err := json.Unmarshal([]byte(tools), &p.Tools); err != nil {
				return nil, fmt.Errorf("parse tools: %w", err)
			}
		}
		if intents != "" {
			if err := json.Unmarshal([]byte(intents), &p.Intents); err != nil {
				return nil, fmt.Errorf("parse intents: %w", err)
			}
		}
		if models != "" {
			if err := json.Unmarshal([]byte(models), &p.Models); err != nil {
				return nil, fmt.Errorf("parse models: %w", err)
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteStalePatterns removes pattern rows whose updated_at is older than olderThan.
func (s *Store) DeleteStalePatterns(olderThan time.Time) error {
	_, err := s.db.Exec(
		`DELETE FROM patterns WHERE datetime(updated_at) < datetime(?)`,
		olderThan.Format(time.RFC3339Nano),
	)
	return err
}

// ── Retention ──────────────────────────────────────────────────────────────

// DeleteBefore removes events with a timestamp older than t and returns the
// number of deleted rows.
func (s *Store) DeleteBefore(t time.Time) (int64, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	meta, err := loadChainMetaTx(tx)
	if err != nil {
		return 0, err
	}

	var baseRowID sql.NullInt64
	var baseHash sql.NullString
	var deletedHashed int
	if err := tx.QueryRow(`
		SELECT rowid, hash
		FROM events
		WHERE datetime(timestamp) < datetime(?)
		  AND rowid > ?
		  AND raw_json IS NOT NULL AND raw_json != ''
		  AND hash IS NOT NULL AND hash != ''
		ORDER BY rowid DESC
		LIMIT 1
	`, t.UTC().Format(time.RFC3339Nano), meta.BaseRowID).Scan(&baseRowID, &baseHash); err != nil {
		if err != sql.ErrNoRows {
			return 0, err
		}
	}
	if err := tx.QueryRow(`
		SELECT COUNT(*)
		FROM events
		WHERE datetime(timestamp) < datetime(?)
		  AND rowid > ?
		  AND raw_json IS NOT NULL AND raw_json != ''
		  AND hash IS NOT NULL AND hash != ''
	`, t.UTC().Format(time.RFC3339Nano), meta.BaseRowID).Scan(&deletedHashed); err != nil {
		return 0, err
	}

	// Use datetime() to compare so SQLite parses both sides as timestamps
	// rather than relying on lexicographic TEXT ordering of RFC3339Nano
	// strings (which can be unreliable when the fractional-second part
	// has different widths, e.g. "...00Z" vs "...00.5Z").
	res, err := tx.Exec(
		`DELETE FROM events WHERE datetime(timestamp) < datetime(?)`,
		t.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}

	if deletedHashed > 0 && baseRowID.Valid && baseHash.Valid {
		meta.BaseRowID = baseRowID.Int64
		meta.BasePrevHash = baseHash.String
		meta.Count -= deletedHashed
		if meta.Count < 0 {
			meta.Count = 0
		}
		if meta.Count == 0 {
			meta.HeadHash = meta.BasePrevHash
			meta.LastID = ""
		}
		meta.UpdatedAt = time.Now().UTC()
		if err := upsertChainMetaTx(tx, meta); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
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

func (s *Store) backfillChainMeta() error {
	meta, err := loadChainMetaDB(s.db)
	if err != nil {
		return err
	}
	if meta != nil {
		return nil
	}

	rows, err := s.db.Query(`
		SELECT id, COALESCE(raw_json, ''), COALESCE(hash, '')
		FROM events
		ORDER BY rowid ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	headHash := ""
	lastID := ""
	started := false
	for rows.Next() {
		var id, rawJSON, hash string
		if err := rows.Scan(&id, &rawJSON, &hash); err != nil {
			return err
		}
		isLegacy := rawJSON == "" || hash == ""
		if !started {
			if isLegacy {
				continue
			}
			started = true
		}
		if isLegacy {
			break
		}
		headHash = hash
		lastID = id
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	return upsertChainMetaDB(s.db, ChainMeta{
		HeadHash:  headHash,
		Count:     count,
		LastID:    lastID,
		UpdatedAt: time.Now().UTC(),
	})
}

func loadChainMetaDB(db *sql.DB) (*ChainMeta, error) {
	return scanChainMeta(db.QueryRow(`
		SELECT head_hash, count, last_id, base_rowid, base_prev_hash, updated_at
		FROM chain_meta
		WHERE id = 1
	`))
}

func loadChainMetaTx(tx *sql.Tx) (ChainMeta, error) {
	meta, err := scanChainMeta(tx.QueryRow(`
		SELECT head_hash, count, last_id, base_rowid, base_prev_hash, updated_at
		FROM chain_meta
		WHERE id = 1
	`))
	if err != nil {
		return ChainMeta{}, err
	}
	if meta == nil {
		return ChainMeta{}, nil
	}
	return *meta, nil
}

type chainMetaRow interface {
	Scan(dest ...any) error
}

func scanChainMeta(row chainMetaRow) (*ChainMeta, error) {
	var meta ChainMeta
	var updatedAt string
	err := row.Scan(&meta.HeadHash, &meta.Count, &meta.LastID, &meta.BaseRowID, &meta.BasePrevHash, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if updatedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse chain_meta updated_at %q: %w", updatedAt, err)
		}
		meta.UpdatedAt = parsed
	}
	return &meta, nil
}

func upsertChainMetaDB(db *sql.DB, meta ChainMeta) error {
	_, err := db.Exec(chainMetaUpsertSQL(),
		meta.HeadHash, meta.Count, meta.LastID, meta.BaseRowID, meta.BasePrevHash,
		meta.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func upsertChainMetaTx(tx *sql.Tx, meta ChainMeta) error {
	_, err := tx.Exec(chainMetaUpsertSQL(),
		meta.HeadHash, meta.Count, meta.LastID, meta.BaseRowID, meta.BasePrevHash,
		meta.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func chainMetaUpsertSQL() string {
	return `
		INSERT INTO chain_meta
		  (id, head_hash, count, last_id, base_rowid, base_prev_hash, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			head_hash=excluded.head_hash,
			count=excluded.count,
			last_id=excluded.last_id,
			base_rowid=excluded.base_rowid,
			base_prev_hash=excluded.base_prev_hash,
			updated_at=excluded.updated_at
	`
}

func hashEvent(rawJSON, prevHash string) string {
	sum := sha256.Sum256([]byte(prevHash + rawJSON))
	return hex.EncodeToString(sum[:])
}

// HashForVerify computes the audit-chain hash for stored raw event JSON.
func HashForVerify(rawJSON, prevHash string) string {
	return hashEvent(rawJSON, prevHash)
}
