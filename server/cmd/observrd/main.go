// observrd — the observr collector daemon.
//
// Usage:
//
//	observrd [--port 7676] [--db ./observr.db] [--no-browser]
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ydking0911/observr/server/internal/collector"
	"github.com/ydking0911/observr/server/internal/dashboard"
	"github.com/ydking0911/observr/server/internal/patterns"
	"github.com/ydking0911/observr/server/internal/query"
	"github.com/ydking0911/observr/server/internal/storage"
	internaltail "github.com/ydking0911/observr/server/internal/tail"
	"github.com/ydking0911/observr/server/internal/webhook"
)

func main() {
	// Subcommand detection must happen before flag.Parse so that global
	// flags do not shadow per-subcommand flags (e.g. --db, --port).
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "query":
			runQuery(os.Args[2:])
			return
		case "tail":
			runTail(os.Args[2:])
			return
		case "patterns":
			runPatterns(os.Args[2:])
			return
		}
	}

	port := flag.Int("port", 7676, "Port to listen on")
	dbPath := flag.String("db", "./observr.db", "SQLite database path")
	noBrowser := flag.Bool("no-browser", false, "Don't open browser automatically")
	retention := flag.String("retention", "7d", "Event retention period (e.g. 7d, 24h). Use 0 to disable.")
	slackWebhook := flag.String("slack-webhook", "", "Slack incoming webhook URL for alerts")
	discordWebhook := flag.String("discord-webhook", "", "Discord webhook URL for alerts")
	alertLevel := flag.String("alert-level", "error", "Minimum event level to alert (debug|info|warn|error)")
	alertThreshold := flag.Int("alert-threshold", 1, "Number of matching events before alerting")
	alertWindow := flag.Duration("alert-window", 60*time.Second, "Time window for threshold counting")
	alertCooldown := flag.Duration("alert-cooldown", 5*time.Minute, "Minimum time between alerts")
	flag.Parse()

	retentionDur, err := parseRetention(*retention)
	if err != nil {
		log.Fatalf("invalid --retention %q: %v", *retention, err)
	}

	// Validate --alert-level before touching the database.
	switch *alertLevel {
	case "debug", "info", "warn", "error":
		// valid
	default:
		log.Fatalf("invalid --alert-level %q: must be one of debug|info|warn|error", *alertLevel)
	}

	// ── Storage ──────────────────────────────────────────────────────
	store, err := storage.Open(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer store.Close()

	// ── HTTP router ──────────────────────────────────────────────────
	mux := http.NewServeMux()

	// SDK events intake
	mux.Handle("POST /events", collector.NewHandler(store))

	// Query API (for CLI + AI agents)
	mux.Handle("GET /query", query.NewHandler(store))

	// Pattern detection API
	mux.Handle("GET /patterns", patterns.NewHandler(store))

	// WebSocket for real-time dashboard streaming
	hub := dashboard.NewHub(store)
	go hub.Run()

	// SSE tail endpoint — also receives broadcasts from the store
	tailHub := internaltail.NewHub()

	// Webhook alerter (optional — only when a webhook URL is provided)
	var alerter *webhook.Alerter
	if *slackWebhook != "" || *discordWebhook != "" {
		alerter = webhook.New(webhook.Config{
			SlackURL:   *slackWebhook,
			DiscordURL: *discordWebhook,
			Level:      *alertLevel,
			Threshold:  *alertThreshold,
			Window:     *alertWindow,
			Cooldown:   *alertCooldown,
		})
		log.Printf("webhook alerts enabled (level=%s threshold=%d window=%s cooldown=%s)",
			*alertLevel, *alertThreshold, *alertWindow, *alertCooldown)
	}

	store.SetBroadcaster(&multiBroadcaster{ws: hub, sse: tailHub, alert: alerter})
	mux.Handle("GET /tail", tailHub)
	mux.Handle("GET /ws", hub)

	// Static dashboard (embedded)
	mux.Handle("/", dashboard.StaticHandler())

	// ── Server ───────────────────────────────────────────────────────
	addr := fmt.Sprintf(":%d", *port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // SSE connections are long-lived
	}

	log.Printf("observrd listening on http://localhost%s", addr)

	if !*noBrowser {
		go openBrowser(fmt.Sprintf("http://localhost%s", addr))
	}

	// ── Retention cleanup goroutine ──────────────────────────────────
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	var cleanupWg sync.WaitGroup
	if retentionDur > 0 {
		log.Printf("retention policy: deleting events older than %s (checked every 1h)", *retention)
		cleanupWg.Add(1)
		go func() {
			defer cleanupWg.Done()
			runCleanup(cleanupCtx, store, retentionDur)
		}()
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	cleanupCancel()
	cleanupWg.Wait() // wait for cleanup goroutine to exit before store.Close()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	if alerter != nil {
		alerter.Stop()
	}
}

// multiBroadcaster fans out to the WebSocket hub, the SSE tail hub, and optionally a webhook alerter.
type multiBroadcaster struct {
	ws    storage.Broadcaster
	sse   storage.Broadcaster
	alert storage.Broadcaster // may be nil
}

func (m *multiBroadcaster) Broadcast(e storage.Event) {
	m.ws.Broadcast(e)
	m.sse.Broadcast(e)
	if m.alert != nil {
		m.alert.Broadcast(e)
	}
}

// ── "observrd query" subcommand ──────────────────────────────────────────

func runQuery(args []string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	dbPath := fs.String("db", "./observr.db", "SQLite database path")
	last := fs.Int("last", 100, "Number of recent events")
	level := fs.String("level", "", "Filter by level (error, warn, info, debug)")
	traceID := fs.String("trace-id", "", "Filter by trace ID")
	path := fs.String("path", "", "Filter by HTTP path")
	format := fs.String("format", "json", "Output format: json | text")
	stats := fs.Bool("stats", false, "Show database statistics instead of events")
	_ = fs.Parse(args)

	store, err := storage.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	if *stats {
		st, err := store.Stats()
		if err != nil {
			fmt.Fprintf(os.Stderr, "stats error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("events:    %d\n", st.EventCount)
		if st.OldestEvent != nil {
			fmt.Printf("oldest:    %s\n", st.OldestEvent.Format(time.RFC3339))
		} else {
			fmt.Printf("oldest:    —\n")
		}
		fmt.Printf("db size:   %.2f MB\n", float64(st.DBSizeBytes)/1024/1024)
		return
	}

	q := query.Query{
		Last:    *last,
		Level:   *level,
		TraceID: *traceID,
		Path:    *path,
		Format:  *format,
	}

	if err := query.Execute(store, q, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "query error: %v\n", err)
		os.Exit(1)
	}
}

// ── "observrd tail" subcommand ───────────────────────────────────────────

func runTail(args []string) {
	fs := flag.NewFlagSet("tail", flag.ExitOnError)
	port := fs.Int("port", 7676, "observrd collector port")
	level := fs.String("level", "", "Filter by level (error, warn, info, debug)")
	service := fs.String("service", "", "Filter by service name")
	eventType := fs.String("type", "", "Filter by event type (http_request, span, log)")
	format := fs.String("format", "pretty", "Output format: pretty | json")
	_ = fs.Parse(args)

	q := url.Values{}
	if *level != "" {
		q.Set("level", *level)
	}
	if *service != "" {
		q.Set("service", *service)
	}
	if *eventType != "" {
		q.Set("type", *eventType)
	}
	tailURL := fmt.Sprintf("http://localhost:%d/tail", *port)
	if len(q) > 0 {
		tailURL += "?" + q.Encode()
	}

	fmt.Fprintf(os.Stderr, "observrd tail — connected to %s\n\n", tailURL)

	// Interrupt handler so the user can Ctrl-C cleanly
	ctx, cancel := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-quit; cancel() }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tailURL, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Accept", "text/event-stream")

	sseClient := &http.Client{
		Transport: &http.Transport{
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}
	resp, err := sseClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not connect to observrd at %s\n", tailURL)
		fmt.Fprintf(os.Stderr, "make sure `observrd` is running (observrd --port %d)\n", *port)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		fmt.Fprintf(os.Stderr, "unexpected status %d from observrd: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == ':' {
			continue // keep-alive or comment
		}
		const prefix = "data: "
		if len(line) <= len(prefix) || line[:len(prefix)] != prefix {
			continue
		}
		data := line[len(prefix):]

		if *format == "json" {
			fmt.Println(data)
			continue
		}

		// Pretty format
		var e storage.Event
		if err := json.Unmarshal([]byte(data), &e); err != nil {
			fmt.Println(data)
			continue
		}
		printPretty(e)
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "stream error: %v\n", err)
		os.Exit(1)
	}
}

// ANSI colour codes
const (
	colReset  = "\033[0m"
	colGray   = "\033[90m"
	colBlue   = "\033[34m"
	colGreen  = "\033[32m"
	colYellow = "\033[33m"
	colRed    = "\033[31m"
	colCyan   = "\033[36m"
	colBold   = "\033[1m"
)

func levelColor(level string) string {
	switch level {
	case "debug":
		return colGray
	case "info":
		return colGreen
	case "warn":
		return colYellow
	case "error":
		return colRed
	default:
		return colReset
	}
}

func printPretty(e storage.Event) {
	ts := e.Timestamp.Format("15:04:05.000")
	lc := levelColor(e.Level)
	lvl := fmt.Sprintf("%s%-5s%s", lc, e.Level, colReset)

	switch e.Type {
	case "http_request":
		statusColor := colGreen
		if e.StatusCode >= 500 {
			statusColor = colRed
		} else if e.StatusCode >= 400 {
			statusColor = colYellow
		}
		fmt.Printf("%s%s%s  %s  %s%s %s%s  %s%d%s  %s%.1fms%s\n",
			colGray, ts, colReset,
			lvl,
			colCyan, e.Service, colReset,
			colBold+e.Method+colReset,
			statusColor, e.StatusCode, colReset,
			colGray, e.DurationMS, colReset,
		)
		if e.Path != "" {
			fmt.Printf("         %s%s%s\n", colBlue, e.Path, colReset)
		}
	case "span":
		fmt.Printf("%s%s%s  %s  %s%s%s  %s%s%s  %s%.1fms%s\n",
			colGray, ts, colReset,
			lvl,
			colCyan, e.Service, colReset,
			colBold, e.Message, colReset,
			colGray, e.DurationMS, colReset,
		)
	default: // log
		fmt.Printf("%s%s%s  %s  %s%s%s  %s\n",
			colGray, ts, colReset,
			lvl,
			colCyan, e.Service, colReset,
			e.Message,
		)
	}
}

// ── "observrd patterns" subcommand ───────────────────────────────────────

func runPatterns(args []string) {
	fs := flag.NewFlagSet("patterns", flag.ExitOnError)
	dbPath := fs.String("db", "./observr.db", "SQLite database path")
	since := fs.Duration("since", 15*time.Minute, "Time window (e.g. 15m, 1h)")
	level := fs.String("level", "", "Filter by level (error, warn, info, debug)")
	minCount := fs.Int("min-count", 1, "Minimum event count per pattern")
	_ = fs.Parse(args)

	store, err := storage.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	ps, err := patterns.Fetch(store, *since, *level, *minCount)
	if err != nil {
		fmt.Fprintf(os.Stderr, "patterns error: %v\n", err)
		os.Exit(1)
	}

	if len(ps) == 0 {
		fmt.Println("no patterns found")
		return
	}

	for _, p := range ps {
		lc := levelColor(p.Level)
		fmt.Printf("%s%-5s%s  %s%3d×%s  %s\n",
			lc, p.Level, colReset,
			colBold, p.Count, colReset,
			p.Fingerprint,
		)
		fmt.Printf("       services: %s  window: %s – %s\n\n",
			strings.Join(p.Services, ", "),
			p.FirstSeen.Format("15:04:05"),
			p.LastSeen.Format("15:04:05"),
		)
	}
}

// runCleanup deletes events older than retention on start and then every hour.
func runCleanup(ctx context.Context, store *storage.Store, retention time.Duration) {
	doCleanup := func() {
		cutoff := time.Now().UTC().Add(-retention)
		n, err := store.DeleteBefore(cutoff)
		if err != nil {
			log.Printf("retention cleanup error: %v", err)
			return
		}
		if n > 0 {
			log.Printf("retention: deleted %d events older than %s", n, cutoff.Format(time.RFC3339))
			if err := store.Vacuum(); err != nil {
				log.Printf("retention vacuum error: %v", err)
			}
		}
	}

	doCleanup()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			doCleanup()
		case <-ctx.Done():
			return
		}
	}
}

// maxRetentionDays is the upper bound for the "Nd" retention format.
// time.Duration overflows int64 above ~106751 days (about 292 years).
const maxRetentionDays = 106751

// parseRetention parses a retention string like "7d", "24h", "1h30m", or "0".
// Returns 0 for "0" (unlimited).
//
// Accepted formats:
//   - "Nd"  — positive integer days (e.g. "7d", "30d"); custom because
//     time.ParseDuration does not support the "d" unit.
//   - any Go duration string accepted by time.ParseDuration (e.g. "24h", "1h30m")
//
// "Nd" form: N must be a bare positive integer with no trailing characters.
// Values above maxRetentionDays are rejected to prevent int64 overflow that
// would turn the cutoff into a future timestamp and wipe all events.
func parseRetention(s string) (time.Duration, error) {
	if s == "0" {
		return 0, nil
	}
	if strings.HasSuffix(s, "d") {
		n := strings.TrimSuffix(s, "d")
		// Require that n consists solely of decimal digits so that inputs
		// like "24h1d" (Sscanf would silently parse "24") are rejected.
		if n == "" {
			return 0, fmt.Errorf("must be a positive integer days (e.g. 7d, 30d) or a Go duration (e.g. 24h, 1h30m)")
		}
		for _, c := range n {
			if c < '0' || c > '9' {
				return 0, fmt.Errorf("must be a positive integer days (e.g. 7d, 30d) or a Go duration (e.g. 24h, 1h30m)")
			}
		}
		var days int
		if _, err := fmt.Sscanf(n, "%d", &days); err != nil || days <= 0 {
			return 0, fmt.Errorf("must be a positive integer days (e.g. 7d, 30d) or a Go duration (e.g. 24h, 1h30m)")
		}
		if days > maxRetentionDays {
			return 0, fmt.Errorf("retention too large: %dd exceeds maximum of %dd (~292 years)", days, maxRetentionDays)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("must be a positive integer days (e.g. 7d, 30d) or a Go duration (e.g. 24h, 1h30m)")
	}
	if d <= 0 {
		return 0, fmt.Errorf("must be positive (use 0 to disable)")
	}
	return d, nil
}

func openBrowser(url string) {
	time.Sleep(500 * time.Millisecond)
	log.Printf("dashboard: %s", url)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux and others
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("could not open browser: %v (open %s manually)", err, url)
		return
	}
	_ = cmd.Wait()
}
