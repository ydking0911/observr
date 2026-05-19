export type Level = "error" | "warn" | "info" | "debug";
export type EventType = "http_request" | "log" | "span";

export interface ObservrEvent {
  id: string;
  trace_id?: string;
  span_id?: string;
  parent_span_id?: string;
  service: string;
  timestamp: string;
  type: EventType;
  level: Level;
  method?: string;
  path?: string;
  status_code?: number;
  duration_ms?: number;
  message: string;
  attributes?: Record<string, unknown>;
}

export interface Stats {
  total: number;
  errors: number;
  warnings: number;
  p50_ms: number;
  p99_ms: number;
  rps: number;
}

export interface Pattern {
  fingerprint: string;
  group_by?: "tool" | "intent" | "model" | "";
  group_value?: string;
  count: number;
  first_seen: string;
  last_seen: string;
  level: Level;
  services: string[];
  sample_event_id: string;
  trend: "rising" | "stable" | "falling";
  anomaly_score: number;
  anomaly: boolean;
  buckets?: PatternBucket[];
  tools?: string[];
  intents?: string[];
  models?: string[];
}

export interface PatternBucket {
  t: string;
  count: number;
}

export interface CausalCorrelation {
  root_intent: string;
  error_fingerprint: string;
  count: number;
  probability: number;
  services: string[];
  sample_event_id: string;
}
