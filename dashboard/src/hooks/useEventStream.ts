import { useEffect, useRef, useState, useCallback } from "react";
import type { ObservrEvent } from "../types";

const MAX_EVENTS = 500;

export function useEventStream() {
  const [events, setEvents] = useState<ObservrEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);

  const connect = useCallback(() => {
    const protocol = location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${protocol}://${location.host}/ws`);
    wsRef.current = ws;

    ws.onopen = () => setConnected(true);
    ws.onclose = () => {
      setConnected(false);
      // Reconnect after 2s
      setTimeout(connect, 2000);
    };
    ws.onerror = () => ws.close();

    ws.onmessage = (e) => {
      try {
        const event: ObservrEvent = JSON.parse(e.data);
        setEvents((prev) => [event, ...prev].slice(0, MAX_EVENTS));
      } catch {
        // malformed message — ignore
      }
    };
  }, []);

  useEffect(() => {
    // Load initial events via HTTP
    fetch("/query?last=100&format=json")
      .then((r) => r.json())
      .then((data: ObservrEvent[]) => {
        if (Array.isArray(data)) setEvents(data);
      })
      .catch(() => {});

    connect();
    return () => wsRef.current?.close();
  }, [connect]);

  const clear = useCallback(() => setEvents([]), []);

  return { events, connected, clear };
}
