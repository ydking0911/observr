import type { CausalCorrelation } from "../types";

function fmtProbability(value: number) {
  return `${Math.round(value * 100)}%`;
}

interface Props {
  correlations: CausalCorrelation[];
  loading: boolean;
}

export function CausalTable({ correlations, loading }: Props) {
  return (
    <section style={{ borderTop: "1px solid var(--border)", paddingTop: "var(--space-5)" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: "var(--space-3)" }}>
        <h2
          style={{
            fontSize: "var(--text-xs)",
            fontWeight: 600,
            letterSpacing: "0.05em",
            textTransform: "uppercase",
            color: "var(--text-tertiary)",
          }}
        >
          Causal correlations
        </h2>
        <span style={{ fontSize: "var(--text-sm)", color: "var(--text-tertiary)" }}>
          {loading ? "loading..." : `${correlations.length} pairs`}
        </span>
      </div>

      {!loading && correlations.length === 0 && (
        <div style={{ color: "var(--text-tertiary)", fontSize: "var(--text-sm)", padding: "var(--space-4) 0" }}>
          No intent-to-error correlations in this window.
        </div>
      )}

      {correlations.length > 0 && (
        <div style={{ overflowX: "auto" }}>
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "var(--text-sm)" }}>
            <thead>
              <tr style={{ borderBottom: "2px solid var(--border)", textAlign: "left" }}>
                {["Root intent", "", "Error fingerprint", "Probability", "Count", "Services"].map((h) => (
                  <th
                    key={h || "arrow"}
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
              {correlations.map((row) => (
                <tr key={`${row.root_intent}-${row.error_fingerprint}`} style={{ borderBottom: "1px solid var(--border)" }}>
                  <td style={{ padding: "var(--space-3) var(--space-4)", fontFamily: "var(--font-mono)", color: "var(--status-ok)", whiteSpace: "nowrap" }}>
                    {row.root_intent}
                  </td>
                  <td style={{ padding: "var(--space-3) var(--space-2)", color: "var(--text-tertiary)", textAlign: "center" }}>→</td>
                  <td style={{ padding: "var(--space-3) var(--space-4)", maxWidth: 460 }}>
                    <span
                      title={row.error_fingerprint}
                      style={{
                        display: "block",
                        fontFamily: "var(--font-mono)",
                        color: "var(--status-error)",
                        fontSize: "var(--text-xs)",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                      }}
                    >
                      {row.error_fingerprint}
                    </span>
                  </td>
                  <td style={{ padding: "var(--space-3) var(--space-4)" }}>
                    <span
                      style={{
                        display: "inline-block",
                        minWidth: 44,
                        textAlign: "center",
                        fontFamily: "var(--font-mono)",
                        fontWeight: 700,
                        color: row.probability >= 0.5 ? "var(--status-error)" : "var(--accent)",
                        background: row.probability >= 0.5 ? "var(--status-error-bg)" : "var(--accent-subtle)",
                        borderRadius: 4,
                        padding: "1px 6px",
                      }}
                    >
                      {fmtProbability(row.probability)}
                    </span>
                  </td>
                  <td style={{ padding: "var(--space-3) var(--space-4)", fontFamily: "var(--font-mono)", fontWeight: 700 }}>{row.count}</td>
                  <td style={{ padding: "var(--space-3) var(--space-4)" }}>
                    <div style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
                      {row.services.map((svc) => (
                        <span
                          key={svc}
                          style={{
                            fontSize: "var(--text-xs)",
                            fontFamily: "var(--font-mono)",
                            color: "var(--text-secondary)",
                            background: "var(--bg-subtle)",
                            border: "1px solid var(--border)",
                            borderRadius: 4,
                            padding: "1px 5px",
                          }}
                        >
                          {svc}
                        </span>
                      ))}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
