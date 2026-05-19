import { useEffect, useState } from "react";
import type { Level, Pattern } from "../types";

interface Options {
  since?: string;
  level?: Level | "";
  minCount?: number;
  groupBy?: "tool" | "intent" | "model" | "";
  buckets?: boolean;
  enabled?: boolean;
}

export function usePatterns(opts: Options = {}) {
  const [patterns, setPatterns] = useState<Pattern[]>([]);
  const [loading, setLoading] = useState(true);
  const enabled = opts.enabled ?? true;

  useEffect(() => {
    if (!enabled) return;

    const params = new URLSearchParams();
    if (opts.since) params.set("since", opts.since);
    if (opts.level) params.set("level", opts.level);
    if (opts.minCount && opts.minCount > 1) params.set("min_count", String(opts.minCount));
    if (opts.groupBy) params.set("group_by", opts.groupBy);
    if (opts.buckets) params.set("buckets", "true");

    setLoading(true);

    const controller = new AbortController();

    const load = async () => {
      try {
        const res = await fetch(`/patterns?${params}`, { signal: controller.signal });
        if (!res.ok) return;
        const data: Pattern[] = await res.json();
        setPatterns(data ?? []);
      } catch (err) {
        if ((err as Error).name !== "AbortError") {
          // swallow network errors silently, keep current view
        }
      } finally {
        setLoading(false);
      }
    };

    load();
    const id = setInterval(load, 10_000);
    return () => {
      clearInterval(id);
      controller.abort();
    };
  }, [enabled, opts.since, opts.level, opts.minCount, opts.groupBy, opts.buckets]);

  return { patterns, loading };
}
