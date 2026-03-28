// Package query handles GET /query — the AI-agent-friendly interface.
package query

import (
	"encoding/csv"
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
	Format  string // "json" | "text" | "csv"
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
//	GET /query?level=error&last=50&format=csv
func NewHandler(s querier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := parseHTTPQuery(r)

		switch q.Format {
		case "csv":
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", `attachment; filename="observr-events.csv"`)
		default:
			w.Header().Set("Content-Type", "application/json")
		}

		if err := Execute(s, q, w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
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
	case "csv":
		return writeCSV(out, events)
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

func writeCSV(out io.Writer, events []storage.Event) error {
	w := csv.NewWriter(out)
	header := []string{"timestamp", "level", "service", "type", "method", "path", "status_code", "duration_ms", "message", "trace_id", "span_id", "id"}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, e := range events {
		statusCode := ""
		if e.StatusCode != 0 {
			statusCode = strconv.Itoa(e.StatusCode)
		}
		durMS := ""
		if e.DurationMS > 0 {
			durMS = fmt.Sprintf("%.3f", e.DurationMS)
		}
		row := []string{
			e.Timestamp.UTC().Format(time.RFC3339),
			e.Level,
			e.Service,
			e.Type,
			e.Method,
			e.Path,
			statusCode,
			durMS,
			e.Message,
			e.TraceID,
			e.SpanID,
			e.ID,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-3]) + "..."
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
