interface Props {
  connected: boolean;
}

export function StatusDot({ connected }: Props) {
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: "var(--space-2)",
        fontSize: "var(--text-sm)",
        color: connected ? "var(--status-ok)" : "var(--text-tertiary)",
        fontWeight: 500,
      }}
    >
      <span
        style={{
          width: 7,
          height: 7,
          borderRadius: "50%",
          background: connected ? "var(--status-ok)" : "var(--border-strong)",
          boxShadow: connected
            ? "0 0 0 2px oklch(56% 0.16 145 / 0.25)"
            : "none",
          transition: "all var(--duration-base) var(--ease-out-quart)",
        }}
      />
      {connected ? "Live" : "Disconnected"}
    </span>
  );
}
