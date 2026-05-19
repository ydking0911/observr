import { useEffect, useState } from "react";
import type { CausalCorrelation } from "../types";

interface Options {
  since?: string;
  minCount?: number;
  enabled?: boolean;
}

export function useCausalCorrelations(opts: Options = {}) {
  const [correlations, setCorrelations] = useState<CausalCorrelation[]>([]);
  const [loading, setLoading] = useState(true);
  const enabled = opts.enabled ?? true;

  useEffect(() => {
    if (!enabled) return;

    const params = new URLSearchParams();
    if (opts.since) params.set("since", opts.since);
    if (opts.minCount && opts.minCount > 1) params.set("min_count", String(opts.minCount));

    setLoading(true);

    const controller = new AbortController();

    const load = async () => {
      try {
        const res = await fetch(`/patterns/causal?${params}`, { signal: controller.signal });
        if (!res.ok) return;
        const data: CausalCorrelation[] = await res.json();
        setCorrelations(data ?? []);
      } catch (err) {
        if ((err as Error).name !== "AbortError") {
          // keep the current view if the daemon is unavailable
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
  }, [enabled, opts.since, opts.minCount]);

  return { correlations, loading };
}
