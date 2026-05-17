import { useEffect, useState } from "react";
import type { ObservrEvent } from "../types";

export function useTraceEvents(traceId: string | null) {
  const [events, setEvents] = useState<ObservrEvent[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!traceId) {
      setEvents([]);
      setLoading(false);
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    const params = new URLSearchParams({ trace_id: traceId, last: "1000", format: "json" });
    fetch(`/query?${params}`, { signal: controller.signal })
      .then((r) => (r.ok ? r.json() : []))
      .then((data: ObservrEvent[]) => setEvents(Array.isArray(data) ? data : []))
      .catch((err) => { if (err.name !== "AbortError") setEvents([]); })
      .finally(() => setLoading(false));
    return () => controller.abort();
  }, [traceId]);

  return { events, loading };
}
