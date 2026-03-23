import type { ObservrEvent } from "../types";
import { LevelBadge } from "./LevelBadge";

interface Props {
  event: ObservrEvent;
  onClose: () => void;
}

export function EventDetail({ event, onClose }: Props) {
  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 50,
        display: "flex",
        justifyContent: "flex-end",
      }}
    >
      {/* Backdrop */}
      <div
        onClick={onClose}
        style={{
          position: "absolute",
          inset: 0,
          background: "oklch(18% 0.02 250 / 0.25)",
          animation: `fadeIn var(--duration-base) var(--ease-out-quart)`,
        }}
      />

      {/* Panel */}
      <aside
        style={{
          position: "relative",
          width: "min(480px, 95vw)",
          height: "100%",
          background: "var(--bg-surface)",
          borderLeft: "1px solid var(--border)",
          boxShadow: "-4px 0 24px oklch(18% 0.02 250 / 0.10)",
          overflowY: "auto",
          animation: `slideInRight var(--duration-slow) var(--ease-out-quart)`,
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
            <LevelBadge level={event.level} />
            <span style={{ fontSize: "var(--text-sm)", color: "var(--text-secondary)" }}>
              {event.type}
            </span>
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
            aria-label="Close detail"
          >
            ✕
          </button>
        </div>

        {/* Body */}
        <div style={{ padding: "var(--space-6)", display: "flex", flexDirection: "column", gap: "var(--space-6)" }}>
          {/* Message */}
          <section>
            <p
              style={{
                fontSize: "var(--text-base)",
                fontWeight: event.level === "error" ? 500 : 400,
                color: event.level === "error" ? "var(--status-error)" : "var(--text-primary)",
                lineHeight: 1.5,
              }}
            >
              {event.message}
            </p>
          </section>

          {/* Key fields */}
          <section>
            <DetailGrid>
              <DetailRow label="Service" value={event.service} />
              <DetailRow label="Timestamp" value={new Date(event.timestamp).toLocaleString()} mono />
              {event.trace_id && <DetailRow label="Trace ID" value={event.trace_id} mono />}
              {event.span_id && <DetailRow label="Span ID" value={event.span_id} mono />}
              {event.method && <DetailRow label="Method" value={event.method} mono />}
              {event.path && <DetailRow label="Path" value={event.path} mono />}
              {event.status_code != null && <DetailRow label="Status" value={String(event.status_code)} mono />}
              {event.duration_ms != null && <DetailRow label="Duration" value={`${event.duration_ms}ms`} mono />}
            </DetailGrid>
          </section>

          {/* Attributes */}
          {event.attributes && Object.keys(event.attributes).length > 0 && (
            <section>
              <h3
                style={{
                  fontSize: "var(--text-xs)",
                  fontWeight: 500,
                  letterSpacing: "0.06em",
                  textTransform: "uppercase",
                  color: "var(--text-tertiary)",
                  marginBottom: "var(--space-3)",
                }}
              >
                Attributes
              </h3>
              <pre
                style={{
                  background: "var(--bg-subtle)",
                  border: "1px solid var(--border)",
                  borderRadius: "var(--radius-sm)",
                  padding: "var(--space-4)",
                  fontSize: "var(--text-xs)",
                  fontFamily: "var(--font-mono)",
                  color: "var(--text-primary)",
                  overflowX: "auto",
                  lineHeight: 1.7,
                  whiteSpace: "pre-wrap",
                  wordBreak: "break-word",
                }}
              >
                {JSON.stringify(event.attributes, null, 2)}
              </pre>
            </section>
          )}
        </div>
      </aside>

      <style>{`
        @keyframes slideInRight {
          from { transform: translateX(100%); }
          to   { transform: translateX(0); }
        }
      `}</style>
    </div>
  );
}

function DetailGrid({ children }: { children: React.ReactNode }) {
  return (
    <dl
      style={{
        display: "grid",
        gridTemplateColumns: "auto 1fr",
        gap: "var(--space-2) var(--space-4)",
        fontSize: "var(--text-sm)",
      }}
    >
      {children}
    </dl>
  );
}

function DetailRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <>
      <dt style={{ color: "var(--text-tertiary)", whiteSpace: "nowrap", alignSelf: "start", paddingTop: 1 }}>
        {label}
      </dt>
      <dd
        style={{
          color: "var(--text-primary)",
          fontFamily: mono ? "var(--font-mono)" : "var(--font-sans)",
          fontSize: mono ? "var(--text-xs)" : "var(--text-sm)",
          wordBreak: "break-all",
        }}
      >
        {value}
      </dd>
    </>
  );
}
