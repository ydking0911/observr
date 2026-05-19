package patterns_test

import (
	"testing"

	"github.com/ydking0911/observr/server/internal/patterns"
	"github.com/ydking0911/observr/server/internal/storage"
)

func TestExtractAgentAttrs(t *testing.T) {
	e := storage.Event{
		Attributes: map[string]any{
			"tool":   "web_search",
			"intent": "research",
			"model":  "claude-sonnet-4-6",
		},
	}
	tool, intent, model := patterns.ExtractAgentAttrs(e)
	if tool != "web_search" {
		t.Errorf("tool = %q, want \"web_search\"", tool)
	}
	if intent != "research" {
		t.Errorf("intent = %q, want \"research\"", intent)
	}
	if model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want \"claude-sonnet-4-6\"", model)
	}
}

func TestExtractAgentAttrsNilAttributes(t *testing.T) {
	e := storage.Event{Attributes: nil}
	tool, intent, model := patterns.ExtractAgentAttrs(e)
	if tool != "" || intent != "" || model != "" {
		t.Errorf("expected empty strings, got tool=%q intent=%q model=%q", tool, intent, model)
	}
}

func TestCollectAttrs(t *testing.T) {
	events := []storage.Event{
		{Message: "timeout at port 5432", Attributes: map[string]any{"tool": "db_query", "intent": "data_fetch"}},
		{Message: "timeout at port 5433", Attributes: map[string]any{"tool": "db_query", "intent": "report"}},
		{Message: "other error", Attributes: map[string]any{"tool": "llm_call"}},
	}
	tools, intents, models := patterns.CollectAttrs(events, "timeout at port <N>")
	if len(tools) != 1 || tools[0] != "db_query" {
		t.Errorf("tools = %v, want [db_query]", tools)
	}
	if len(intents) != 2 || intents[0] != "data_fetch" || intents[1] != "report" {
		t.Errorf("intents = %v, want [data_fetch report]", intents)
	}
	if len(models) != 0 {
		t.Errorf("models = %v, want []", models)
	}
}

func TestCollectAttrsSorted(t *testing.T) {
	events := []storage.Event{
		{Message: "timeout at port 5432", Attributes: map[string]any{"tool": "z_tool"}},
		{Message: "timeout at port 5433", Attributes: map[string]any{"tool": "a_tool"}},
	}
	tools, _, _ := patterns.CollectAttrs(events, "timeout at port <N>")
	if len(tools) != 2 || tools[0] != "a_tool" {
		t.Errorf("tools not sorted: %v", tools)
	}
}
