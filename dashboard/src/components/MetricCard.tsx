interface Props {
  label: string;
  value: string | number;
  sub?: string;
  accent?: "default" | "error" | "warn" | "ok";
}

const accentMap = {
  default: { color: "var(--accent)", bg: "var(--accent-subtle)" },
  error:   { color: "var(--status-error)", bg: "var(--status-error-bg)" },
  warn:    { color: "var(--status-warn)", bg: "var(--status-warn-bg)" },
  ok:      { color: "var(--status-ok)", bg: "var(--status-ok-bg)" },
};

export function MetricCard({ label, value, sub, accent = "default" }: Props) {
  const { color } = accentMap[accent];

  return (
    <div
      style={{
        background: "var(--bg-surface)",
        border: "1px solid var(--border)",
        borderRadius: "var(--radius-md)",
        padding: "var(--space-5) var(--space-6)",
        display: "flex",
        flexDirection: "column",
        gap: "var(--space-1)",
        boxShadow: "var(--shadow-sm)",
      }}
    >
      <span
        style={{
          fontSize: "var(--text-xs)",
          fontWeight: 500,
          letterSpacing: "0.06em",
          textTransform: "uppercase",
          color: "var(--text-tertiary)",
        }}
      >
        {label}
      </span>
      <span
        style={{
          fontSize: "var(--text-3xl)",
          fontWeight: 600,
          lineHeight: 1.1,
          color,
          fontFamily: "var(--font-mono)",
          letterSpacing: "-0.03em",
        }}
      >
        {value}
      </span>
      {sub && (
        <span style={{ fontSize: "var(--text-xs)", color: "var(--text-tertiary)" }}>
          {sub}
        </span>
      )}
    </div>
  );
}
