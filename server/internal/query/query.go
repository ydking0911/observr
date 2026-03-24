// Package query handles GET /query — the AI-agent-friendly interface.
package query

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ydking0911/observr/server/internal/storage"
)

// Query holds parsed CLI / HTTP query parameters.
type Query struct {
	Last    int
	Level   string
	TraceID string
	Path    string
	Format  string // "json" | "text"
}

type querier interface {
	Query(f storage.QueryFilter) ([]storage.Event, error)
}

// NewHandler returns an http.Handler for GET /query.
// Used by the CLI and AI agents via HTTP.
//
// Example:
//
//	GET /query?level=error&last=50&format=json
func NewHandler(s querier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := parseHTTPQuery(r)

		if err := Execute(s, q, w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if q.Format == "json" {
			w.Header().Set("Content-Type", "application/json")
		}
	})
}

// Execute runs the query against the store and writes results to out.
// Used by both the HTTP handler and the CLI subcommand.
func Execute(s querier, q Query, out io.Writer) error {
	filter := storage.QueryFilter{
		Last:    q.Last,
		Level:   q.Level,
		TraceID: q.TraceID,
		Path:    q.Path,
	}

	events, err := s.Query(filter)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	switch q.Format {
	case "json":
		return writeJSON(out, events)
	default:
		return writeText(out, events)
	}
}

// ── Formatters ─────────────────────────────────────────────────────────────

func writeJSON(out io.Writer, events []storage.Event) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(events)
}

func writeText(out io.Writer, events []storage.Event) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TIMESTAMP\tLEVEL\tSERVICE\tTYPE\tMESSAGE\tDURATION")
	for _, e := range events {
		dur := ""
		if e.DurationMS > 0 {
			dur = fmt.Sprintf("%.0fms", e.DurationMS)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			e.Timestamp.Format(time.RFC3339),
			strings.ToUpper(e.Level),
			e.Service,
			e.Type,
			truncate(e.Message, 60),
			dur,
		)
	}
	return tw.Flush()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// ── HTTP param parsing ─────────────────────────────────────────────────────

func parseHTTPQuery(r *http.Request) Query {
	q := r.URL.Query()
	last, _ := strconv.Atoi(q.Get("last"))
	if last == 0 {
		last = 100
	}
	format := q.Get("format")
	if format == "" {
		format = "json"
	}
	return Query{
		Last:    last,
		Level:   q.Get("level"),
		TraceID: q.Get("trace_id"),
		Path:    q.Get("path"),
		Format:  format,
	}
}
