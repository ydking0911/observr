import type { Pattern } from "../types";

const LEVEL_COLOR: Record<string, string> = {
  error: "var(--status-error)",
  warn: "var(--status-warn)",
  info: "var(--status-info)",
  debug: "var(--text-tertiary)",
};

function fmtTime(iso: string) {
  return new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function fmtRelative(iso: string) {
  const diff = Date.now() - new Date(iso).getTime();
  if (diff < 60_000) return `${Math.round(diff / 1000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  return `${Math.round(diff / 3_600_000)}h ago`;
}

interface Props {
  pattern: Pattern;
}

export function PatternCard({ pattern }: Props) {
  const levelColor = LEVEL_COLOR[pattern.level] ?? "var(--text-tertiary)";

  return (
    <div
      style={{
        background: "var(--bg-surface)",
        border: "1px solid var(--border)",
        borderLeft: `3px solid ${levelColor}`,
        borderRadius: "var(--radius-sm)",
        padding: "var(--space-4)",
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-2)",
      }}
    >
      {/* Top row: fingerprint + count badge */}
      <div style={{ display: "flex", alignItems: "flex-start", gap: "var(--space-3)" }}>
        <span
          style={{
            flex: 1,
            fontFamily: "var(--font-mono)",
            fontSize: "var(--text-sm)",
            color: "var(--text-primary)",
            wordBreak: "break-all",
            lineHeight: 1.5,
          }}
        >
          {pattern.fingerprint}
        </span>
        <span
          style={{
            flexShrink: 0,
            background: levelColor,
            color: "#fff",
            fontWeight: 700,
            fontSize: "var(--text-xs)",
            padding: "2px 8px",
            borderRadius: "999px",
            minWidth: 28,
            textAlign: "center",
          }}
        >
          {pattern.count}×
        </span>
      </div>

      {/* Services */}
      <div style={{ display: "flex", gap: "var(--space-1)", flexWrap: "wrap" }}>
        {pattern.services.map((svc) => (
          <span
            key={svc}
            style={{
              fontSize: "var(--text-xs)",
              background: "var(--bg-subtle)",
              color: "var(--text-secondary)",
              padding: "1px 6px",
              borderRadius: "4px",
              fontFamily: "var(--font-mono)",
            }}
          >
            {svc}
          </span>
        ))}
      </div>

      {/* Timestamps */}
      <div
        style={{
          fontSize: "var(--text-xs)",
          color: "var(--text-tertiary)",
          display: "flex",
          gap: "var(--space-4)",
        }}
      >
        <span>first {fmtTime(pattern.first_seen)}</span>
        <span>last {fmtRelative(pattern.last_seen)}</span>
      </div>
    </div>
  );
}
