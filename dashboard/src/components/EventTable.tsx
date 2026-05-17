import { useState } from "react";
import type { ObservrEvent } from "../types";
import { LevelBadge } from "./LevelBadge";
import { EventDetail } from "./EventDetail";

interface Props {
  events: ObservrEvent[];
  onSelectTrace?: (traceId: string) => void;
}

function formatTime(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString("en-US", { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function formatDuration(ms?: number): string {
  if (!ms) return "—";
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

function getMethodColor(method?: string): string {
  switch (method?.toUpperCase()) {
    case "GET":    return "var(--status-ok)";
    case "POST":   return "var(--accent)";
    case "PUT":    return "var(--status-warn)";
    case "PATCH":  return "var(--status-warn)";
    case "DELETE": return "var(--status-error)";
    default:       return "var(--text-tertiary)";
  }
}

export function EventTable({ events, onSelectTrace }: Props) {
  const [selected, setSelected] = useState<ObservrEvent | null>(null);

  if (events.length === 0) {
    return (
      <div
        style={{
          textAlign: "center",
          padding: "var(--space-12) var(--space-8)",
          color: "var(--text-tertiary)",
          fontSize: "var(--text-sm)",
        }}
      >
        <div style={{ fontSize: "var(--text-2xl)", marginBottom: "var(--space-3)", opacity: 0.4 }}>⊘</div>
        No events yet. Start your service with <code style={{ fontFamily: "var(--font-mono)" }}>observr.init()</code>.
      </div>
    );
  }

  return (
    <>
      <div style={{ overflowX: "auto" }}>
        <table
          style={{
            width: "100%",
            borderCollapse: "collapse",
            fontSize: "var(--text-sm)",
          }}
        >
          <thead>
            <tr
              style={{
                borderBottom: "2px solid var(--border)",
                textAlign: "left",
              }}
            >
              {["Time", "Level", "Service", "Method", "Path / Message", "Status", "Duration"].map((h) => (
                <th
                  key={h}
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
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {events.map((evt, i) => (
              <EventRow
                key={evt.id}
                event={evt}
                index={i}
                selected={selected?.id === evt.id}
                onClick={() => setSelected(selected?.id === evt.id ? null : evt)}
                onSelectTrace={onSelectTrace}
                getMethodColor={getMethodColor}
                formatTime={formatTime}
                formatDuration={formatDuration}
              />
            ))}
          </tbody>
        </table>
      </div>

      {selected && (
        <EventDetail event={selected} onClose={() => setSelected(null)} />
      )}
    </>
  );
}

interface RowProps {
  event: ObservrEvent;
  index: number;
  selected: boolean;
  onClick: () => void;
  onSelectTrace?: (traceId: string) => void;
  getMethodColor: (m?: string) => string;
  formatTime: (s: string) => string;
  formatDuration: (ms?: number) => string;
}

function EventRow({ event: evt, index, selected, onClick, onSelectTrace, getMethodColor, formatTime, formatDuration }: RowProps) {
  const isError = evt.level === "error";
  const statusOk = evt.status_code && evt.status_code < 400;

  return (
    <tr
      onClick={onClick}
      style={{
        borderBottom: "1px solid var(--border)",
        cursor: "pointer",
        background: selected
          ? "var(--accent-subtle)"
          : isError
            ? "oklch(97% 0.015 25 / 0.5)"
            : "transparent",
        animation: `slideIn ${Math.min(index * 20 + 80, 300)}ms var(--ease-out-quart) both`,
        transition: "background var(--duration-fast) var(--ease-out-quart)",
      }}
      onMouseEnter={(e) => {
        if (!selected) e.currentTarget.style.background = "var(--bg-subtle)";
      }}
      onMouseLeave={(e) => {
        if (!selected) e.currentTarget.style.background = isError ? "oklch(97% 0.015 25 / 0.5)" : "transparent";
      }}
    >
      {/* Time */}
      <td style={{ padding: "var(--space-3) var(--space-4)", whiteSpace: "nowrap" }}>
        <span style={{ fontFamily: "var(--font-mono)", color: "var(--text-tertiary)", fontSize: "var(--text-xs)" }}>
          {formatTime(evt.timestamp)}
        </span>
      </td>

      {/* Level */}
      <td style={{ padding: "var(--space-3) var(--space-4)" }}>
        <LevelBadge level={evt.level} />
      </td>

      {/* Service */}
      <td style={{ padding: "var(--space-3) var(--space-4)" }}>
        <span style={{ fontSize: "var(--text-xs)", color: "var(--text-secondary)", fontWeight: 500 }}>
          {evt.service}
        </span>
      </td>

      {/* Method */}
      <td style={{ padding: "var(--space-3) var(--space-4)" }}>
        {evt.method ? (
          <span
            style={{
              fontFamily: "var(--font-mono)",
              fontSize: "var(--text-xs)",
              fontWeight: 600,
              color: getMethodColor(evt.method),
            }}
          >
            {evt.method}
          </span>
        ) : (
          <span style={{ color: "var(--text-tertiary)" }}>—</span>
        )}
      </td>

      {/* Path / Message */}
      <td style={{ padding: "var(--space-3) var(--space-4)", maxWidth: 400 }}>
        <span
          style={{
            display: "block",
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            fontFamily: evt.path ? "var(--font-mono)" : "var(--font-sans)",
            fontSize: evt.path ? "var(--text-xs)" : "var(--text-sm)",
            color: isError ? "var(--status-error)" : "var(--text-primary)",
            fontWeight: isError ? 500 : 400,
          }}
          title={evt.path ?? evt.message}
        >
          {evt.path ?? evt.message}
        </span>
        {evt.path && evt.message && (
          <span
            style={{
              display: "block",
              fontSize: "var(--text-xs)",
              color: "var(--text-tertiary)",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {evt.message}
          </span>
        )}
        {evt.trace_id && onSelectTrace && (
          <button
            onClick={(e) => { e.stopPropagation(); onSelectTrace(evt.trace_id!); }}
            aria-label={`View trace ${evt.trace_id}`}
            style={{
              marginTop: 2,
              display: "inline-block",
              fontFamily: "var(--font-mono)",
              fontSize: 10,
              lineHeight: "16px",
              padding: "0 5px",
              borderRadius: 4,
              background: "var(--accent-subtle)",
              color: "var(--accent)",
              border: "1px solid oklch(55% 0.18 250 / 0.25)",
              cursor: "pointer",
              fontWeight: 500,
              letterSpacing: "0.02em",
            }}
            title={`View trace ${evt.trace_id}`}
          >
            ⎇ {evt.trace_id.slice(0, 8)}
          </button>
        )}
      </td>

      {/* Status code */}
      <td style={{ padding: "var(--space-3) var(--space-4)", whiteSpace: "nowrap" }}>
        {evt.status_code ? (
          <span
            style={{
              fontFamily: "var(--font-mono)",
              fontWeight: 600,
              fontSize: "var(--text-sm)",
              color: statusOk ? "var(--status-ok)" : "var(--status-error)",
            }}
          >
            {evt.status_code}
          </span>
        ) : (
          <span style={{ color: "var(--text-tertiary)" }}>—</span>
        )}
      </td>

      {/* Duration */}
      <td style={{ padding: "var(--space-3) var(--space-4)", whiteSpace: "nowrap" }}>
        <span
          style={{
            fontFamily: "var(--font-mono)",
            fontSize: "var(--text-xs)",
            color: evt.duration_ms && evt.duration_ms > 1000
              ? "var(--status-warn)"
              : "var(--text-secondary)",
          }}
        >
          {formatDuration(evt.duration_ms)}
        </span>
      </td>
    </tr>
  );
}
