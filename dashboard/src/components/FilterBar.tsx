import type { Level } from "../types";

export interface Filters {
  level: Level | "";
  search: string;
}

interface Props {
  filters: Filters;
  onChange: (f: Filters) => void;
  onClear: () => void;
  total: number;
}

const LEVELS: Array<{ value: Level | ""; label: string }> = [
  { value: "", label: "All" },
  { value: "error", label: "Error" },
  { value: "warn", label: "Warn" },
  { value: "info", label: "Info" },
  { value: "debug", label: "Debug" },
];

export function FilterBar({ filters, onChange, onClear, total }: Props) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: "var(--space-3)",
        flexWrap: "wrap",
      }}
    >
      {/* Level tabs */}
      <div
        style={{
          display: "flex",
          background: "var(--bg-subtle)",
          borderRadius: "var(--radius-sm)",
          padding: 3,
          gap: 2,
        }}
      >
        {LEVELS.map(({ value, label }) => {
          const active = filters.level === value;
          return (
            <button
              key={value}
              onClick={() => onChange({ ...filters, level: value })}
              style={{
                padding: "4px 12px",
                borderRadius: "4px",
                border: "none",
                cursor: "pointer",
                fontSize: "var(--text-sm)",
                fontWeight: active ? 600 : 400,
                fontFamily: "var(--font-sans)",
                background: active ? "var(--bg-surface)" : "transparent",
                color: active ? "var(--text-primary)" : "var(--text-secondary)",
                boxShadow: active ? "var(--shadow-sm)" : "none",
                transition: "all var(--duration-fast) var(--ease-out-quart)",
              }}
            >
              {label}
            </button>
          );
        })}
      </div>

      {/* Search */}
      <input
        type="search"
        placeholder="Filter by path, message, service…"
        value={filters.search}
        onChange={(e) => onChange({ ...filters, search: e.target.value })}
        style={{
          flex: 1,
          minWidth: 200,
          padding: "6px 12px",
          border: "1px solid var(--border)",
          borderRadius: "var(--radius-sm)",
          background: "var(--bg-surface)",
          color: "var(--text-primary)",
          fontSize: "var(--text-sm)",
          fontFamily: "var(--font-sans)",
          outline: "none",
          transition: "border-color var(--duration-fast)",
        }}
        onFocus={(e) => (e.currentTarget.style.borderColor = "var(--accent)")}
        onBlur={(e) => (e.currentTarget.style.borderColor = "var(--border)")}
      />

      {/* Count + clear */}
      <span style={{ fontSize: "var(--text-sm)", color: "var(--text-tertiary)", whiteSpace: "nowrap" }}>
        {total} events
      </span>
      <button
        onClick={onClear}
        style={{
          padding: "5px 12px",
          border: "1px solid var(--border)",
          borderRadius: "var(--radius-sm)",
          background: "transparent",
          color: "var(--text-secondary)",
          fontSize: "var(--text-sm)",
          fontFamily: "var(--font-sans)",
          cursor: "pointer",
          transition: "all var(--duration-fast)",
        }}
        onMouseEnter={(e) => {
          e.currentTarget.style.borderColor = "var(--status-error)";
          e.currentTarget.style.color = "var(--status-error)";
        }}
        onMouseLeave={(e) => {
          e.currentTarget.style.borderColor = "var(--border)";
          e.currentTarget.style.color = "var(--text-secondary)";
        }}
      >
        Clear
      </button>
    </div>
  );
}
