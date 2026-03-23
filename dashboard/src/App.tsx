import { useMemo, useState } from "react";
import { useEventStream } from "./hooks/useEventStream";
import { MetricCard } from "./components/MetricCard";
import { FilterBar, type Filters } from "./components/FilterBar";
import { EventTable } from "./components/EventTable";
import { StatusDot } from "./components/StatusDot";
import type { ObservrEvent } from "./types";

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

export default function App() {
  const { events, connected, clear } = useEventStream();
  const [filters, setFilters] = useState<Filters>({ level: "", search: "" });

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
            v0.1
          </span>
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
      </main>
    </div>
  );
}
