package main

import (
	"context"
	"log"

	observr "github.com/ydking0911/observr/sdk/go"
)

func main() {
	observr.Init(observr.Config{Service: "example-agent"})
	defer observr.GetClient().Shutdown()

	ctx, end := observr.StartAgentSpan(context.Background(), "summarize-doc", observr.AgentSpanOptions{
		Intent: "summarize",
		Model:  "claude-3-5-sonnet",
		Tool:   "read_file",
	})
	defer end()

	_, endChild := observr.StartSpan(ctx, "read_file", map[string]any{"path": "/tmp/doc.txt"})
	defer endChild()

	log.Println("spans emitted to observrd")
}
