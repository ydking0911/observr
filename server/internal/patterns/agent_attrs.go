package patterns

import (
	"sort"

	"github.com/ydking0911/observr/server/internal/storage"
)

// ExtractAgentAttrs returns tool, intent, and model from an event's attributes.
func ExtractAgentAttrs(e storage.Event) (tool, intent, model string) {
	if e.Attributes == nil {
		return
	}
	tool = stringAttr(e.Attributes, "observr.tool", "agent.tool", "tool")
	intent = stringAttr(e.Attributes, "observr.intent", "agent.intent", "intent")
	model = stringAttr(e.Attributes, "observr.model", "agent.model", "model")
	return
}

// CollectAttrs returns sorted unique tools, intents, and models for events
// whose normalized message equals fp.
func CollectAttrs(events []storage.Event, fp string) (tools, intents, models []string) {
	toolSet := map[string]struct{}{}
	intentSet := map[string]struct{}{}
	modelSet := map[string]struct{}{}
	for _, e := range events {
		if Normalize(e.Message) != fp {
			continue
		}
		tool, intent, model := ExtractAgentAttrs(e)
		if tool != "" {
			toolSet[tool] = struct{}{}
		}
		if intent != "" {
			intentSet[intent] = struct{}{}
		}
		if model != "" {
			modelSet[model] = struct{}{}
		}
	}
	return sortedKeys(toolSet), sortedKeys(intentSet), sortedKeys(modelSet)
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func stringAttr(attrs map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := attrs[key].(string); ok {
			return v
		}
	}
	return ""
}
