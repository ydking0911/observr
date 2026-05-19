import { useMemo, useState } from "react";
import type { Pattern } from "../types";

type SortKey = "count" | "anomaly" | "trend" | "last_seen";

const LEVEL_COLOR: Record<string, string> = {
  error: "var(--status-error)",
  warn:  "var(--status-warn)",
  info:  "var(--status-info)",
  debug: "var(--text-tertiary)",
};

function fmtRelative(iso: string) {
  const diff = Date.now() - new Date(iso).getTime();
  if (diff < 60_000) return `${Math.round(diff / 1000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  return `${Math.round(diff / 3_600_000)}h ago`;
}

function fmtTime(iso: string) {
  return new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function LevelDot({ level }: { level: string }) {
  return (
    <span
      style={{
        display: "inline-block",
        width: 7,
        height: 7,
        borderRadius: "50%",
        background: LEVEL_COLOR[level] ?? "var(--text-tertiary)",
        boxShadow: level === "error" ? `0 0 4px ${LEVEL_COLOR.error}80` : undefined,
      }}
    />
  );
}

function sparkBarColor(count: number, max: number, isAnomaly: boolean): string {
  if (max === 0 || count === 0) return "oklch(35% 0.04 250)";
  const ratio = count / max;
  if (ratio >= 0.7) return isAnomaly ? "var(--status-error)" : "var(--accent)";
  if (ratio >= 0.4) return "oklch(55% 0.12 250)";
  return "oklch(35% 0.04 250)";
}

function MiniSparkline({ pattern }: { pattern: Pattern }) {
  const buckets = pattern.buckets ?? [];
  if (buckets.length === 0) return <span style={{ color: "var(--text-tertiary)" }}>-</span>;
  const max = Math.max(...buckets.map((b) => b.count), 1);
  return (
    <div style={{ display: "flex", alignItems: "flex-end", gap: 1.5, height: 20, minWidth: 56 }}>
      {buckets.map((bucket, i) => (
        <span
          key={`${bucket.t}-${i}`}
          title={`${bucket.count}`}
          style={{
            width: 4,
            height: `${Math.max((bucket.count / max) * 100, bucket.count > 0 ? 16 : 4)}%`,
            borderRadius: "1px 1px 0 0",
            background: sparkBarColor(bucket.count, max, pattern.anomaly),
          }}
        />
      ))}
    </div>
  );
}

function ChipList({ values, tone }: { values?: string[]; tone: "tool" | "intent" | "service" }) {
  if (!values || values.length === 0) return <span style={{ color: "var(--text-tertiary)" }}>-</span>;

  const styles = {
    tool:    { color: "var(--accent)",         bg: "oklch(94% 0.04 250)",  border: "oklch(80% 0.08 250)" },
    intent:  { color: "var(--status-ok)",      bg: "oklch(96% 0.03 145)",  border: "oklch(84% 0.08 145)" },
    service: { color: "var(--text-secondary)", bg: "var(--bg-subtle)",     border: "var(--border)" },
  }[tone];

  return (
    <div style={{ display: "flex", gap: 4, maxWidth: 180, overflow: "hidden" }}>
      {values.slice(0, 2).map((value) => (
        <span
          key={value}
          title={value}
          style={{
            fontSize: "var(--text-xs)",
            fontFamily: "var(--font-mono)",
            color: styles.color,
            background: styles.bg,
            border: `1px solid ${styles.border}`,
            borderRadius: 4,
            padding: "1px 5px",
            whiteSpace: "nowrap",
            overflow: "hidden",
            textOverflow: "ellipsis",
          }}
        >
          {value}
        </span>
      ))}
      {values.length > 2 && (
        <span style={{ color: "var(--text-tertiary)", fontSize: "var(--text-xs)" }}>+{values.length - 2}</span>
      )}
    </div>
  );
}

const COLS: [string, string][] = [
  ["", ""],
  ["Fingerprint", ""],
  ["Count", "count"],
  ["Trend", "trend"],
  ["Freq", ""],
  ["Anomaly", "anomaly"],
  ["Tool", ""],
  ["Intent", ""],
  ["Services", ""],
  ["First seen", ""],
  ["Last seen", "last_seen"],
];

interface Props {
  patterns: Pattern[];
}

export function PatternTable({ patterns }: Props) {
  const [sort, setSort] = useState<SortKey>("count");

  const sorted = useMemo(() => [...patterns].sort((a, b) => {
    if (sort === "anomaly")   return b.anomaly_score - a.anomaly_score;
    if (sort === "trend")     return a.trend.localeCompare(b.trend);
    if (sort === "last_seen") return new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime();
    return b.count - a.count;
  }), [patterns, sort]);

  if (patterns.length === 0) return null;

  return (
    <div style={{ overflowX: "auto" }}>
      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "var(--text-sm)" }}>
        <thead>
          <tr style={{ borderBottom: "2px solid var(--border)", textAlign: "left" }}>
            {COLS.map(([label, key]) => (
              <th
                key={label || "level-col"}
                aria-sort={sort === key && key ? "descending" : undefined}
                style={{
                  padding: "var(--space-3) var(--space-4)",
                  fontWeight: 500,
                  fontSize: "var(--text-xs)",
                  letterSpacing: "0.05em",
                  textTransform: "uppercase",
                  color: "var(--text-tertiary)",
                  whiteSpace: "nowrap",
                }}
              >
                {key ? (
                  <button
                    onClick={() => setSort(key as SortKey)}
                    onKeyDown={(e) => e.key === "Enter" && setSort(key as SortKey)}
                    style={{
                      all: "unset",
                      cursor: "pointer",
                      color: sort === key ? "var(--accent)" : "var(--text-tertiary)",
                      userSelect: "none",
                    }}
                  >
                    {label}{sort === key ? " ↓" : ""}
                  </button>
                ) : label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {sorted.map((pattern) => (
            <tr
              key={`${pattern.fingerprint}-${pattern.group_by ?? ""}-${pattern.group_value ?? ""}`}
              style={{
                borderBottom: "1px solid var(--border)",
                background: pattern.anomaly ? "oklch(97% 0.03 70 / 0.3)" : "transparent",
              }}
            >
              {/* Level dot */}
              <td style={{ padding: "var(--space-3) var(--space-4)" }}>
                <LevelDot level={pattern.level} />
              </td>

              {/* Fingerprint + inline anomaly badge */}
              <td style={{ padding: "var(--space-3) var(--space-4)", maxWidth: 320 }}>
                <span
                  title={pattern.fingerprint}
                  style={{
                    display: "block",
                    fontFamily: "var(--font-mono)",
                    fontSize: "var(--text-xs)",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                    color: pattern.level === "error" ? "var(--status-error)" : "var(--text-primary)",
                  }}
                >
                  {pattern.fingerprint}
                </span>
                <div style={{ display: "flex", gap: 4, marginTop: 2 }}>
                  {pattern.anomaly && (
                    <span style={{
                      fontSize: "10px",
                      fontWeight: 600,
                      color: "oklch(48% 0.16 55)",
                      background: "oklch(96% 0.04 70)",
                      border: "1px solid oklch(82% 0.08 70)",
                      padding: "1px 5px",
                      borderRadius: 3,
                      display: "inline-flex",
                      alignItems: "center",
                      gap: 3,
                    }}>
                      ⚠ anomaly
                    </span>
                  )}
                  {pattern.group_by && pattern.group_value && (
                    <span style={{ fontSize: "10px", color: "var(--text-tertiary)", fontFamily: "var(--font-mono)" }}>
                      {pattern.group_by}: {pattern.group_value}
                    </span>
                  )}
                </div>
              </td>

              {/* Count pill */}
              <td style={{ padding: "var(--space-3) var(--space-4)" }}>
                <span style={{
                  display: "inline-block",
                  padding: "2px 8px",
                  borderRadius: 999,
                  fontWeight: 700,
                  fontSize: "var(--text-xs)",
                  color: "oklch(99% 0.003 250)",
                  background: LEVEL_COLOR[pattern.level] ?? "var(--text-tertiary)",
                  textAlign: "center",
                  minWidth: 28,
                }}>
                  {pattern.count}×
                </span>
              </td>

              {/* Trend with arrow */}
              <td style={{ padding: "var(--space-3) var(--space-4)" }}>
                <span style={{
                  fontWeight: 600,
                  fontSize: "var(--text-xs)",
                  color: pattern.trend === "rising"
                    ? "var(--status-error)"
                    : pattern.trend === "falling"
                      ? "var(--status-ok)"
                      : "var(--accent)",
                }}>
                  {pattern.trend === "rising" ? "↑ rising" : pattern.trend === "falling" ? "↓ falling" : "→ stable"}
                </span>
              </td>

              {/* Mini sparkline */}
              <td style={{ padding: "var(--space-3) var(--space-4)" }}>
                <MiniSparkline pattern={pattern} />
              </td>

              {/* Anomaly score */}
              <td style={{
                padding: "var(--space-3) var(--space-4)",
                fontFamily: "var(--font-mono)",
                fontSize: "var(--text-xs)",
                color: pattern.anomaly ? "oklch(48% 0.16 55)" : "var(--text-tertiary)",
                whiteSpace: "nowrap",
              }}>
                {pattern.anomaly_score.toFixed(1)}σ
              </td>

              {/* Tool / Intent / Services */}
              <td style={{ padding: "var(--space-3) var(--space-4)" }}>
                <ChipList values={pattern.tools} tone="tool" />
              </td>
              <td style={{ padding: "var(--space-3) var(--space-4)" }}>
                <ChipList values={pattern.intents} tone="intent" />
              </td>
              <td style={{ padding: "var(--space-3) var(--space-4)" }}>
                <ChipList values={pattern.services} tone="service" />
              </td>

              {/* First seen */}
              <td style={{ padding: "var(--space-3) var(--space-4)", color: "var(--text-tertiary)", fontSize: "var(--text-xs)", whiteSpace: "nowrap" }}>
                {fmtTime(pattern.first_seen)}
              </td>

              {/* Last seen */}
              <td style={{ padding: "var(--space-3) var(--space-4)", color: "var(--text-tertiary)", fontSize: "var(--text-xs)", whiteSpace: "nowrap" }}>
                {fmtRelative(pattern.last_seen)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
