import { useEffect, useState } from "react";
import type { Level, Pattern } from "../types";

interface Options {
  since?: string; // e.g. "15m", "1h"
  level?: Level | "";
  minCount?: number;
}

export function usePatterns(opts: Options = {}) {
  const [patterns, setPatterns] = useState<Pattern[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const params = new URLSearchParams();
    if (opts.since) params.set("since", opts.since);
    if (opts.level) params.set("level", opts.level);
    if (opts.minCount && opts.minCount > 1) params.set("min_count", String(opts.minCount));

    const load = async () => {
      try {
        const res = await fetch(`/patterns?${params}`);
        if (!res.ok) return;
        const data: Pattern[] = await res.json();
        setPatterns(data ?? []);
      } catch {
        // swallow network errors silently
      } finally {
        setLoading(false);
      }
    };

    load();
    const id = setInterval(load, 10_000);
    return () => clearInterval(id);
  }, [opts.since, opts.level, opts.minCount]);

  return { patterns, loading };
}
