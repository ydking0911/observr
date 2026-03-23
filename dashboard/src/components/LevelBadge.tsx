import type { Level } from "../types";

const styles: Record<Level, { color: string; bg: string; label: string }> = {
  error: { color: "var(--status-error)", bg: "var(--status-error-bg)", label: "ERROR" },
  warn:  { color: "var(--status-warn)",  bg: "var(--status-warn-bg)",  label: "WARN"  },
  info:  { color: "var(--status-info)",  bg: "var(--accent-subtle)",   label: "INFO"  },
  debug: { color: "var(--text-tertiary)", bg: "var(--bg-subtle)",      label: "DEBUG" },
};

interface Props {
  level: Level;
}

export function LevelBadge({ level }: Props) {
  const s = styles[level] ?? styles.info;
  return (
    <span
      style={{
        display: "inline-block",
        padding: "1px 7px",
        borderRadius: "4px",
        fontSize: "var(--text-xs)",
        fontWeight: 600,
        letterSpacing: "0.06em",
        fontFamily: "var(--font-mono)",
        color: s.color,
        background: s.bg,
        whiteSpace: "nowrap",
      }}
    >
      {s.label}
    </span>
  );
}
