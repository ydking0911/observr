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

function fmtScore(score: number) {
  if (!Number.isFinite(score) || score <= 0) return "0.0σ";
  return `${score.toFixed(1)}σ`;
}

function TrendBadge({ trend }: { trend: Pattern["trend"] }) {
  const style =
    trend === "rising"
      ? { color: "var(--status-error)", background: "var(--status-error-bg)" }
      : trend === "falling"
        ? { color: "var(--status-ok)", background: "var(--status-ok-bg)" }
        : { color: "var(--accent)", background: "var(--accent-subtle)" };
  const label = trend === "rising" ? "↑ rising" : trend === "falling" ? "↓ falling" : "→ stable";

  return (
    <span
      style={{
        ...style,
        flexShrink: 0,
        fontSize: "var(--text-xs)",
        fontWeight: 600,
        padding: "2px 7px",
        borderRadius: 4,
        whiteSpace: "nowrap",
      }}
    >
      {label}
    </span>
  );
}

function sparkBarColor(count: number, max: number, isAnomaly: boolean): string {
  if (max === 0 || count === 0) return "oklch(35% 0.04 250)";
  const ratio = count / max;
  if (ratio >= 0.7) return isAnomaly ? "var(--status-error)" : "var(--accent)";
  if (ratio >= 0.4) return "oklch(55% 0.12 250)";
  return "oklch(35% 0.04 250)";
}

function Sparkline({ pattern }: { pattern: Pattern }) {
  const buckets = pattern.buckets ?? [];
  if (buckets.length === 0) return null;

  const max = Math.max(...buckets.map((b) => b.count), 1);
  return (
    <div
      aria-label={`Frequency sparkline for ${pattern.fingerprint}`}
      style={{
        display: "grid",
        gridTemplateColumns: `repeat(${buckets.length}, minmax(3px, 1fr))`,
        alignItems: "end",
        gap: 2,
        height: 34,
        minWidth: 120,
      }}
    >
      {buckets.map((bucket, i) => (
        <span
          key={`${bucket.t}-${i}`}
          title={`${new Date(bucket.t).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}: ${bucket.count}`}
          style={{
            display: "block",
            height: `${Math.max((bucket.count / max) * 100, bucket.count > 0 ? 12 : 3)}%`,
            borderRadius: "2px 2px 0 0",
            background: sparkBarColor(bucket.count, max, pattern.anomaly),
          }}
        />
      ))}
    </div>
  );
}

const ATTR_LABEL: Record<string, never> = {} as never;
void ATTR_LABEL;

function AttrChip({ value, tone }: { value: string; tone: "tool" | "intent" | "model" | "service" }) {
  const tones = {
    tool:    { bg: "oklch(94% 0.04 250)",   color: "var(--accent)",          border: "oklch(80% 0.08 250)" },
    intent:  { bg: "oklch(96% 0.03 145)",   color: "var(--status-ok)",       border: "oklch(84% 0.08 145)" },
    model:   { bg: "oklch(96% 0.025 300)",  color: "oklch(45% 0.12 300)",    border: "oklch(84% 0.06 300)" },
    service: { bg: "var(--bg-subtle)",       color: "var(--text-secondary)",  border: "var(--border)" },
  };
  const t = tones[tone];
  return (
    <span
      title={value}
      style={{
        fontSize: "var(--text-xs)",
        background: t.bg,
        color: t.color,
        border: `1px solid ${t.border}`,
        padding: "1px 6px",
        borderRadius: 4,
        fontFamily: "var(--font-mono)",
        maxWidth: 180,
        overflow: "hidden",
        textOverflow: "ellipsis",
        whiteSpace: "nowrap",
      }}
    >
      {value}
    </span>
  );
}

const labelStyle = {
  fontSize: "10px",
  color: "var(--text-tertiary)",
  textTransform: "uppercase" as const,
  letterSpacing: "0.05em",
};

interface Props {
  pattern: Pattern;
}

export function PatternCard({ pattern }: Props) {
  const levelColor = LEVEL_COLOR[pattern.level] ?? "var(--text-tertiary)";
  const borderColor = pattern.anomaly ? "oklch(65% 0.16 55)" : levelColor;

  return (
    <div
      style={{
        background: pattern.anomaly ? "oklch(97% 0.03 70 / 0.3)" : "var(--bg-surface)",
        border: "1px solid var(--border)",
        borderLeft: `3px solid ${borderColor}`,
        borderRadius: "var(--radius-sm)",
        padding: "var(--space-4)",
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-2)",
      }}
    >
      {/* Top row: fingerprint + sparkline */}
      <div style={{ display: "grid", gridTemplateColumns: "minmax(0, 1fr) minmax(120px, 180px)", gap: "var(--space-4)", alignItems: "start" }}>
        <div style={{ minWidth: 0, display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
          <div style={{ display: "flex", alignItems: "flex-start", gap: "var(--space-2)", minWidth: 0 }}>
            <span
              style={{
                flex: 1,
                fontFamily: "var(--font-mono)",
                fontSize: "var(--text-sm)",
                color: "var(--text-primary)",
                wordBreak: "break-word",
                lineHeight: 1.5,
                minWidth: 0,
              }}
            >
              {pattern.fingerprint}
            </span>
            {pattern.group_by && pattern.group_value && (
              <span style={{
                fontSize: "var(--text-xs)",
                background: "var(--bg-subtle)",
                color: "var(--text-secondary)",
                border: "1px solid var(--border)",
                padding: "1px 6px",
                borderRadius: 4,
                fontFamily: "var(--font-mono)",
                whiteSpace: "nowrap",
                flexShrink: 0,
              }}>
                {pattern.group_by}: {pattern.group_value}
              </span>
            )}
          </div>

          {/* Badges: count + trend + anomaly */}
          <div style={{ display: "flex", gap: "var(--space-2)", alignItems: "center", flexWrap: "wrap" }}>
            <span
              style={{
                background: levelColor,
                color: "oklch(99% 0.003 250)",
                fontWeight: 700,
                fontSize: "var(--text-xs)",
                padding: "2px 8px",
                borderRadius: 999,
                minWidth: 28,
                textAlign: "center",
              }}
            >
              {pattern.count}×
            </span>
            <TrendBadge trend={pattern.trend} />
            {pattern.anomaly && (
              <span
                style={{
                  color: "oklch(48% 0.16 55)",
                  background: "oklch(96% 0.04 70)",
                  border: "1px solid oklch(82% 0.08 70)",
                  fontSize: "var(--text-xs)",
                  fontWeight: 600,
                  padding: "2px 7px",
                  borderRadius: 4,
                  display: "flex",
                  alignItems: "center",
                  gap: 3,
                }}
              >
                ⚠ anomaly
              </span>
            )}
          </div>
        </div>

        <Sparkline pattern={pattern} />
      </div>

      {/* Agent attribute chips with labels */}
      <div style={{ display: "flex", gap: "var(--space-1)", flexWrap: "wrap", alignItems: "center" }}>
        {(pattern.tools ?? []).length > 0 && (
          <>
            <span style={labelStyle}>tool</span>
            {(pattern.tools ?? []).slice(0, 3).map((v) => <AttrChip key={`tool-${v}`} value={v} tone="tool" />)}
          </>
        )}
        {(pattern.intents ?? []).length > 0 && (
          <>
            <span style={labelStyle}>intent</span>
            {(pattern.intents ?? []).slice(0, 3).map((v) => <AttrChip key={`intent-${v}`} value={v} tone="intent" />)}
          </>
        )}
        {(pattern.models ?? []).length > 0 && (
          <>
            <span style={labelStyle}>model</span>
            {(pattern.models ?? []).slice(0, 2).map((v) => <AttrChip key={`model-${v}`} value={v} tone="model" />)}
          </>
        )}
        {pattern.services.length > 0 && (
          <>
            <span style={labelStyle}>svc</span>
            {pattern.services.map((v) => <AttrChip key={`svc-${v}`} value={v} tone="service" />)}
          </>
        )}
      </div>

      {/* Meta row */}
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
        {pattern.anomaly && <span>anomaly score {fmtScore(pattern.anomaly_score)}</span>}
      </div>
    </div>
  );
}
