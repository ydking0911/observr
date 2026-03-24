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
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/ydking0911/observr/server/internal/collector"
	"github.com/ydking0911/observr/server/internal/dashboard"
	"github.com/ydking0911/observr/server/internal/query"
	"github.com/ydking0911/observr/server/internal/storage"
	internaltail "github.com/ydking0911/observr/server/internal/tail"
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
		}
	}

	port := flag.Int("port", 7676, "Port to listen on")
	dbPath := flag.String("db", "./observr.db", "SQLite database path")
	noBrowser := flag.Bool("no-browser", false, "Don't open browser automatically")
	flag.Parse()

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

	// WebSocket for real-time dashboard streaming
	hub := dashboard.NewHub(store)
	go hub.Run()

	// SSE tail endpoint — also receives broadcasts from the store
	tailHub := internaltail.NewHub()
	store.SetBroadcaster(&multiBroadcaster{ws: hub, sse: tailHub})
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

// multiBroadcaster fans out to both the WebSocket hub and the SSE tail hub.
type multiBroadcaster struct {
	ws  storage.Broadcaster
	sse storage.Broadcaster
}

func (m *multiBroadcaster) Broadcast(e storage.Event) {
	m.ws.Broadcast(e)
	m.sse.Broadcast(e)
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
	_ = fs.Parse(args)

	store, err := storage.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

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

	url := fmt.Sprintf("http://localhost:%d/tail", *port)
	params := ""
	if *level != "" {
		params += "&level=" + *level
	}
	if *service != "" {
		params += "&service=" + *service
	}
	if *eventType != "" {
		params += "&type=" + *eventType
	}
	if params != "" {
		url += "?" + params[1:]
	}

	fmt.Fprintf(os.Stderr, "observrd tail — connected to %s\n\n", url)

	// Interrupt handler so the user can Ctrl-C cleanly
	ctx, cancel := context.WithCancel(context.Background())
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-quit; cancel() }()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
		fmt.Fprintf(os.Stderr, "could not connect to observrd at %s\n", url)
		fmt.Fprintf(os.Stderr, "make sure `observrd` is running (observrd --port %d)\n", *port)
		os.Exit(1)
	}
	defer resp.Body.Close()

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
