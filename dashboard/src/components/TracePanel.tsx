import type { ObservrEvent } from "../types";
import { useTraceEvents } from "../hooks/useTraceEvents";

interface Props {
  traceId: string;
  onClose: () => void;
}

interface TreeNode {
  event: ObservrEvent;
  children: TreeNode[];
  depth: number;
}

function buildTree(events: ObservrEvent[]): TreeNode[] {
  const bySpanId = new Map<string, TreeNode>();

  for (const evt of events) {
    bySpanId.set(evt.span_id ?? evt.id, { event: evt, children: [], depth: 0 });
  }

  const roots: TreeNode[] = [];
  for (const [, node] of bySpanId) {
    const parentKey = node.event.parent_span_id;
    if (parentKey && bySpanId.has(parentKey)) {
      bySpanId.get(parentKey)!.children.push(node);
    } else {
      roots.push(node);
    }
  }

  function setDepth(node: TreeNode, d: number) {
    node.depth = d;
    node.children.sort((a, b) => a.event.timestamp.localeCompare(b.event.timestamp));
    for (const child of node.children) setDepth(child, d + 1);
  }
  roots.sort((a, b) => a.event.timestamp.localeCompare(b.event.timestamp));
  roots.forEach((n) => setDepth(n, 0));

  return roots;
}

function flatten(nodes: TreeNode[]): TreeNode[] {
  const out: TreeNode[] = [];
  for (const n of nodes) {
    out.push(n);
    out.push(...flatten(n.children));
  }
  return out;
}

const TYPE_ICON: Record<string, string> = {
  span: "◎",
  http_request: "⇄",
  log: "·",
};

interface AgentChipStyle {
  bg: string;
  text: string;
  border: string;
}

const AGENT_CHIP_STYLES: Record<string, AgentChipStyle> = {
  "agent.intent":  { bg: "oklch(55% 0.18 250 / 0.10)", text: "oklch(40% 0.20 250)", border: "oklch(55% 0.18 250 / 0.35)" },
  "agent.trigger": { bg: "oklch(55% 0.18 290 / 0.10)", text: "oklch(40% 0.20 290)", border: "oklch(55% 0.18 290 / 0.35)" },
  "agent.model":   { bg: "oklch(50% 0.15 170 / 0.10)", text: "oklch(35% 0.15 170)", border: "oklch(50% 0.15 170 / 0.35)" },
  "agent.tool":    { bg: "oklch(55% 0.18 45 / 0.10)",  text: "oklch(40% 0.18 45)",  border: "oklch(55% 0.18 45 / 0.35)"  },
};

function getAgentAttrs(event: ObservrEvent): Array<{ key: string; label: string; value: string }> {
  if (!event.attributes) return [];
  return Object.keys(AGENT_CHIP_STYLES)
    .filter((k) => event.attributes![k] !== undefined)
    .map((k) => ({ key: k, label: k.replace("agent.", ""), value: String(event.attributes![k]) }));
}

export function TracePanel({ traceId, onClose }: Props) {
  const { events, loading } = useTraceEvents(traceId);
  const nodes = flatten(buildTree(events));

  const times = events.map((e) => new Date(e.timestamp).getTime());
  const traceStart = times.length ? Math.min(...times) : 0;
  const traceEnd = Math.max(
    ...events.map((e) => new Date(e.timestamp).getTime() + (e.duration_ms ?? 0)),
    traceStart + 1
  );
  const traceDuration = traceEnd - traceStart;

  return (
    <div style={{ position: "fixed", inset: 0, zIndex: 50, display: "flex", justifyContent: "flex-end" }}>
      {/* Backdrop */}
      <div
        onClick={onClose}
        style={{
          position: "absolute",
          inset: 0,
          background: "oklch(18% 0.02 250 / 0.25)",
          animation: "fadeIn var(--duration-base) var(--ease-out-quart)",
        }}
      />

      {/* Panel */}
      <aside
        style={{
          position: "relative",
          width: "min(640px, 95vw)",
          height: "100%",
          background: "var(--bg-surface)",
          borderLeft: "1px solid var(--border)",
          boxShadow: "-4px 0 24px oklch(18% 0.02 250 / 0.10)",
          overflowY: "auto",
          animation: "slideInRight var(--duration-slow) var(--ease-out-quart)",
          display: "flex",
          flexDirection: "column",
        }}
      >
        {/* Header */}
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "var(--space-5) var(--space-6)",
            borderBottom: "1px solid var(--border)",
            position: "sticky",
            top: 0,
            background: "var(--bg-surface)",
            zIndex: 1,
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)" }}>
            <span style={{ fontSize: "var(--text-sm)", fontWeight: 600, color: "var(--text-primary)" }}>
              Trace
            </span>
            <code
              style={{
                fontFamily: "var(--font-mono)",
                fontSize: "var(--text-xs)",
                color: "var(--text-secondary)",
                background: "var(--bg-subtle)",
                padding: "2px 6px",
                borderRadius: 4,
              }}
            >
              {traceId.slice(0, 8)}…
            </code>
            {!loading && (
              <span style={{ fontSize: "var(--text-xs)", color: "var(--text-tertiary)" }}>
                {events.length} event{events.length !== 1 ? "s" : ""}
              </span>
            )}
          </div>
          <button
            onClick={onClose}
            style={{
              background: "none",
              border: "none",
              cursor: "pointer",
              color: "var(--text-tertiary)",
              fontSize: "var(--text-lg)",
              padding: "var(--space-1)",
              lineHeight: 1,
              borderRadius: "var(--radius-sm)",
              transition: "color var(--duration-fast)",
            }}
            onMouseEnter={(e) => (e.currentTarget.style.color = "var(--text-primary)")}
            onMouseLeave={(e) => (e.currentTarget.style.color = "var(--text-tertiary)")}
            aria-label="Close trace"
          >
            ✕
          </button>
        </div>

        {/* Body */}
        <div style={{ padding: "var(--space-3) var(--space-4)", flex: 1 }}>
          {loading && (
            <p style={{ color: "var(--text-tertiary)", fontSize: "var(--text-sm)", padding: "var(--space-4) 0" }}>
              Loading…
            </p>
          )}

          {!loading && events.length === 0 && (
            <p style={{ color: "var(--text-tertiary)", fontSize: "var(--text-sm)", padding: "var(--space-4) 0" }}>
              No events found for this trace.
            </p>
          )}

          {!loading &&
            nodes.map((node, i) => {
              const { event, depth } = node;
              const startMs = new Date(event.timestamp).getTime() - traceStart;
              const barOffset = (startMs / traceDuration) * 100;
              const barWidth = Math.max(((event.duration_ms ?? 0) / traceDuration) * 100, 0.8);
              const icon = TYPE_ICON[event.type] ?? "·";
              const isError = event.level === "error";
              const agentAttrs = getAgentAttrs(event);
              const barColor = isError
                ? "var(--status-error)"
                : event.type === "http_request"
                  ? "var(--status-ok)"
                  : "var(--accent)";

              return (
                <div
                  key={event.id + i}
                  style={{ borderBottom: "1px solid var(--border)", padding: "var(--space-2) 0" }}
                >
                  {/* Main row */}
                  <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", minWidth: 0 }}>
                    {/* Depth indent */}
                    <div style={{ width: depth * 14, flexShrink: 0 }} />

                    {/* Type icon */}
                    <span
                      style={{
                        fontFamily: "var(--font-mono)",
                        fontSize: "var(--text-xs)",
                        color: isError ? "var(--status-error)" : "var(--text-tertiary)",
                        flexShrink: 0,
                        width: 14,
                        textAlign: "center",
                      }}
                    >
                      {icon}
                    </span>

                    {/* Message */}
                    <span
                      style={{
                        flex: 1,
                        fontSize: "var(--text-xs)",
                        color: isError ? "var(--status-error)" : "var(--text-primary)",
                        fontWeight: isError ? 500 : 400,
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                        fontFamily: event.type === "span" ? "var(--font-mono)" : "var(--font-sans)",
                        minWidth: 0,
                      }}
                      title={event.message}
                    >
                      {event.message}
                    </span>

                    {/* Duration label */}
                    <span
                      style={{
                        fontFamily: "var(--font-mono)",
                        fontSize: "var(--text-xs)",
                        color: "var(--text-tertiary)",
                        flexShrink: 0,
                        width: 52,
                        textAlign: "right",
                      }}
                    >
                      {event.duration_ms != null ? `${Math.round(event.duration_ms)}ms` : ""}
                    </span>

                    {/* Waterfall bar */}
                    <div
                      style={{
                        width: 140,
                        flexShrink: 0,
                        height: 8,
                        background: "var(--bg-subtle)",
                        borderRadius: 3,
                        position: "relative",
                        overflow: "hidden",
                      }}
                    >
                      <div
                        style={{
                          position: "absolute",
                          left: `${barOffset}%`,
                          width: `${barWidth}%`,
                          height: "100%",
                          borderRadius: 3,
                          background: barColor,
                          opacity: 0.7,
                        }}
                      />
                    </div>
                  </div>

                  {/* Agent attribute badges */}
                  {agentAttrs.length > 0 && (
                    <div
                      style={{
                        display: "flex",
                        flexWrap: "wrap",
                        gap: "var(--space-1)",
                        paddingTop: "var(--space-1)",
                        paddingLeft: depth * 14 + 28,
                      }}
                    >
                      {agentAttrs.map(({ key, label, value }) => {
                        const s = AGENT_CHIP_STYLES[key];
                        return (
                          <span
                            key={key}
                            title={`${key}: ${value}`}
                            style={{
                              fontSize: 10,
                              lineHeight: "18px",
                              padding: "0 6px",
                              borderRadius: 10,
                              background: s.bg,
                              color: s.text,
                              border: `1px solid ${s.border}`,
                              fontWeight: 500,
                              whiteSpace: "nowrap",
                              maxWidth: 200,
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                              display: "inline-block",
                            }}
                          >
                            {label}: {value}
                          </span>
                        );
                      })}
                    </div>
                  )}
                </div>
              );
            })}
        </div>
      </aside>

      <style>{`
        @keyframes slideInRight {
          from { transform: translateX(100%); }
          to   { transform: translateX(0); }
        }
        @keyframes fadeIn {
          from { opacity: 0; }
          to   { opacity: 1; }
        }
      `}</style>
    </div>
  );
}
