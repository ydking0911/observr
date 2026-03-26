import { useMemo, useState } from "react";
import { useEventStream } from "./hooks/useEventStream";
import { usePatterns } from "./hooks/usePatterns";
import { MetricCard } from "./components/MetricCard";
import { FilterBar, type Filters } from "./components/FilterBar";
import { EventTable } from "./components/EventTable";
import { PatternCard } from "./components/PatternCard";
import { StatusDot } from "./components/StatusDot";
import type { Level, ObservrEvent } from "./types";

function computeStats(events: ObservrEvent[]) {
  const httpEvents = events.filter((e) => e.type === "http_request" && e.duration_ms != null);
  const durations = httpEvents.map((e) => e.duration_ms!).sort((a, b) => a - b);

  const p50 = durations[Math.floor(durations.length * 0.5)] ?? 0;
  const p99 = durations[Math.floor(durations.length * 0.99)] ?? 0;

  const errors = events.filter((e) => e.level === "error").length;
  const warnings = events.filter((e) => e.level === "warn").length;

  // RPS: events in last 60s
  const since = Date.now() - 60_000;
  const recent = events.filter((e) => new Date(e.timestamp).getTime() > since);
  const rps = (recent.filter((e) => e.type === "http_request").length / 60).toFixed(1);

  return { total: events.length, errors, warnings, p50, p99, rps };
}

function applyFilters(events: ObservrEvent[], filters: Filters): ObservrEvent[] {
  return events.filter((e) => {
    if (filters.level && e.level !== filters.level) return false;
    if (filters.search) {
      const q = filters.search.toLowerCase();
      const haystack = `${e.message} ${e.path ?? ""} ${e.service}`.toLowerCase();
      if (!haystack.includes(q)) return false;
    }
    return true;
  });
}

type Tab = "events" | "patterns";

const SINCE_OPTIONS = ["15m", "1h", "6h", "24h"];

export default function App() {
  const { events, connected, clear } = useEventStream();
  const [filters, setFilters] = useState<Filters>({ level: "", search: "" });
  const [activeTab, setActiveTab] = useState<Tab>("events");

  // Patterns tab state
  const [patternSince, setPatternSince] = useState("15m");
  const [patternLevel, setPatternLevel] = useState<Level | "">("");
  const [patternMinCount, setPatternMinCount] = useState(1);
  const { patterns, loading: patternsLoading } = usePatterns({
    since: patternSince,
    level: patternLevel,
    minCount: patternMinCount,
  });

  const stats = useMemo(() => computeStats(events), [events]);
  const filtered = useMemo(() => applyFilters(events, filters), [events, filters]);

  return (
    <div
      style={{
        minHeight: "100dvh",
        display: "flex",
        flexDirection: "column",
      }}
    >
      {/* ── Header ─────────────────────────────────────────────────── */}
      <header
        style={{
          background: "var(--bg-inverse)",
          color: "var(--text-inverse)",
          padding: "0 var(--space-8)",
          height: 52,
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          position: "sticky",
          top: 0,
          zIndex: 40,
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)" }}>
          {/* Wordmark */}
          <span
            style={{
              fontWeight: 600,
              fontSize: "var(--text-base)",
              letterSpacing: "-0.01em",
              color: "var(--text-inverse)",
            }}
          >
            observr
          </span>
          <span
            style={{
              fontSize: "var(--text-xs)",
              background: "oklch(55% 0.18 250 / 0.3)",
              color: "oklch(75% 0.12 250)",
              padding: "1px 6px",
              borderRadius: "4px",
              fontWeight: 500,
            }}
          >
            v0.2
          </span>

          {/* Tab navigation */}
          <div
            style={{
              display: "flex",
              marginLeft: "var(--space-6)",
              gap: 2,
            }}
          >
            {(["events", "patterns"] as Tab[]).map((tab) => (
              <button
                key={tab}
                onClick={() => setActiveTab(tab)}
                style={{
                  padding: "4px 14px",
                  border: "none",
                  borderRadius: "4px",
                  cursor: "pointer",
                  fontSize: "var(--text-sm)",
                  fontWeight: activeTab === tab ? 600 : 400,
                  fontFamily: "var(--font-sans)",
                  background: activeTab === tab ? "oklch(55% 0.18 250 / 0.25)" : "transparent",
                  color: activeTab === tab ? "var(--text-inverse)" : "oklch(75% 0.08 250)",
                  textTransform: "capitalize",
                  transition: "all var(--duration-fast)",
                }}
              >
                {tab}
              </button>
            ))}
          </div>
        </div>
        <StatusDot connected={connected} />
      </header>

      {/* ── Metrics strip ──────────────────────────────────────────── */}
      <section
        style={{
          padding: "var(--space-6) var(--space-8)",
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(140px, 1fr))",
          gap: "var(--space-4)",
          borderBottom: "1px solid var(--border)",
        }}
      >
        <MetricCard label="Total Events" value={stats.total} />
        <MetricCard label="Errors" value={stats.errors} accent={stats.errors > 0 ? "error" : "ok"} />
        <MetricCard label="Warnings" value={stats.warnings} accent={stats.warnings > 0 ? "warn" : "default"} />
        <MetricCard label="p50 Latency" value={stats.p50 ? `${Math.round(stats.p50)}ms` : "—"} />
        <MetricCard label="p99 Latency" value={stats.p99 ? `${Math.round(stats.p99)}ms` : "—"} accent={stats.p99 > 1000 ? "warn" : "default"} />
        <MetricCard label="Req/s (1m)" value={stats.rps} sub="last 60 seconds" />
      </section>

      {/* ── Main ───────────────────────────────────────────────────── */}
      <main style={{ flex: 1, display: "flex", flexDirection: "column" }}>

        {activeTab === "events" && (
          <>
            {/* Filter bar */}
            <div
              style={{
                padding: "var(--space-4) var(--space-8)",
                borderBottom: "1px solid var(--border)",
                background: "var(--bg-surface)",
                position: "sticky",
                top: 52,
                zIndex: 30,
              }}
            >
              <FilterBar
                filters={filters}
                onChange={setFilters}
                onClear={clear}
                total={filtered.length}
              />
            </div>

            {/* Table */}
            <div style={{ flex: 1, padding: "0 var(--space-8) var(--space-8)" }}>
              <EventTable events={filtered} />
            </div>
          </>
        )}

        {activeTab === "patterns" && (
          <>
            {/* Patterns filter bar */}
            <div
              style={{
                padding: "var(--space-4) var(--space-8)",
                borderBottom: "1px solid var(--border)",
                background: "var(--bg-surface)",
                position: "sticky",
                top: 52,
                zIndex: 30,
                display: "flex",
                alignItems: "center",
                gap: "var(--space-3)",
                flexWrap: "wrap",
              }}
            >
              {/* Since selector */}
              <div style={{ display: "flex", background: "var(--bg-subtle)", borderRadius: "var(--radius-sm)", padding: 3, gap: 2 }}>
                {SINCE_OPTIONS.map((s) => (
                  <button
                    key={s}
                    onClick={() => setPatternSince(s)}
                    style={{
                      padding: "4px 12px",
                      borderRadius: "4px",
                      border: "none",
                      cursor: "pointer",
                      fontSize: "var(--text-sm)",
                      fontWeight: patternSince === s ? 600 : 400,
                      fontFamily: "var(--font-sans)",
                      background: patternSince === s ? "var(--bg-surface)" : "transparent",
                      color: patternSince === s ? "var(--text-primary)" : "var(--text-secondary)",
                      boxShadow: patternSince === s ? "var(--shadow-sm)" : "none",
                    }}
                  >
                    {s}
                  </button>
                ))}
              </div>

              {/* Level filter */}
              <select
                value={patternLevel}
                onChange={(e) => setPatternLevel(e.target.value as Level | "")}
                style={{
                  padding: "5px 10px",
                  border: "1px solid var(--border)",
                  borderRadius: "var(--radius-sm)",
                  background: "var(--bg-surface)",
                  color: "var(--text-primary)",
                  fontSize: "var(--text-sm)",
                  fontFamily: "var(--font-sans)",
                  cursor: "pointer",
                }}
              >
                <option value="">All levels</option>
                <option value="error">Error</option>
                <option value="warn">Warn</option>
                <option value="info">Info</option>
                <option value="debug">Debug</option>
              </select>

              {/* Min count */}
              <label style={{ fontSize: "var(--text-sm)", color: "var(--text-secondary)", display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
                min count
                <input
                  type="number"
                  min={1}
                  value={patternMinCount}
                  onChange={(e) => setPatternMinCount(Math.max(1, Number(e.target.value)))}
                  style={{
                    width: 56,
                    padding: "4px 8px",
                    border: "1px solid var(--border)",
                    borderRadius: "var(--radius-sm)",
                    background: "var(--bg-surface)",
                    color: "var(--text-primary)",
                    fontSize: "var(--text-sm)",
                    fontFamily: "var(--font-sans)",
                  }}
                />
              </label>

              <span style={{ fontSize: "var(--text-sm)", color: "var(--text-tertiary)", marginLeft: "auto" }}>
                {patternsLoading ? "loading…" : `${patterns.length} patterns`}
              </span>
            </div>

            {/* Pattern cards */}
            <div
              style={{
                flex: 1,
                padding: "var(--space-6) var(--space-8)",
                display: "flex",
                flexDirection: "column",
                gap: "var(--space-3)",
              }}
            >
              {!patternsLoading && patterns.length === 0 && (
                <div style={{ color: "var(--text-tertiary)", fontSize: "var(--text-sm)", textAlign: "center", paddingTop: "var(--space-8)" }}>
                  No patterns found in the last {patternSince}.
                </div>
              )}
              {patterns.map((p) => (
                <PatternCard key={p.fingerprint} pattern={p} />
              ))}
            </div>
          </>
        )}
      </main>
    </div>
  );
}
