export interface ObservrEvent {
  timestamp: string;
  type: "http_request" | "span" | "log";
  level: "debug" | "info" | "warn" | "error";
  trace_id?: string;
  span_id?: string;
  message: string;
  service?: string;
  duration_ms?: number;
  method?: string;
  path?: string;
  status_code?: number;
  attributes?: Record<string, unknown>;
}

export interface ObservrConfig {
  service?: string;
  collectorUrl?: string;
  autoInstrument?: boolean;
  logLevel?: "debug" | "info" | "warn" | "error";
}
