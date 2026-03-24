// observrd — the observr collector daemon.
//
// Usage:
//
//	observrd [--port 7676] [--db ./observr.db] [--no-browser]
package main

import (
	"context"
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
)

func main() {
	port := flag.Int("port", 7676, "Port to listen on")
	dbPath := flag.String("db", "./observr.db", "SQLite database path")
	noBrowser := flag.Bool("no-browser", false, "Don't open browser automatically")
	flag.Parse()

	// Subcommand: "query" — machine-readable output for AI agents
	if len(os.Args) > 1 && os.Args[1] == "query" {
		runQuery(os.Args[2:], *dbPath)
		return
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

	// WebSocket for real-time dashboard streaming
	hub := dashboard.NewHub(store)
	go hub.Run()
	store.SetBroadcaster(hub)
	mux.Handle("GET /ws", hub)

	// Static dashboard (embedded)
	mux.Handle("/", dashboard.StaticHandler())

	// ── Server ───────────────────────────────────────────────────────
	addr := fmt.Sprintf(":%d", *port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
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

func runQuery(args []string, dbPath string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	last := fs.Int("last", 100, "Number of recent events")
	level := fs.String("level", "", "Filter by level (error, warn, info, debug)")
	traceID := fs.String("trace-id", "", "Filter by trace ID")
	path := fs.String("path", "", "Filter by HTTP path")
	format := fs.String("format", "json", "Output format: json | text")
	_ = fs.Parse(args)

	store, err := storage.Open(dbPath)
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
	}
}
