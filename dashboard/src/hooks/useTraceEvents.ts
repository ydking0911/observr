import { useEffect, useState } from "react";
import type { ObservrEvent } from "../types";

export function useTraceEvents(traceId: string | null) {
  const [events, setEvents] = useState<ObservrEvent[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!traceId) {
      setEvents([]);
      return;
    }
    setLoading(true);
    const params = new URLSearchParams({ trace_id: traceId, last: "1000", format: "json" });
    fetch(`/query?${params}`)
      .then((r) => (r.ok ? r.json() : []))
      .then((data: ObservrEvent[]) => setEvents(Array.isArray(data) ? data : []))
      .catch(() => setEvents([]))
      .finally(() => setLoading(false));
  }, [traceId]);

  return { events, loading };
}
