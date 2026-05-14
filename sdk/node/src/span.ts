import { randomBytes } from "node:crypto";
import type { Transport } from "./transport.js";

export class Span {
  readonly name: string;
  readonly spanId: string;
  readonly traceId: string;
  readonly parentSpanId: string | undefined;
  private readonly transport: Transport;
  private readonly attributes: Record<string, unknown>;
  private startTime = 0;

  constructor(
    name: string,
    transport: Transport,
    attributes: Record<string, unknown> = {},
    parentSpanId?: string
  ) {
    this.name = name;
    this.spanId = randomBytes(8).toString("hex");
    this.traceId = randomBytes(16).toString("hex");
    this.parentSpanId = parentSpanId;
    this.transport = transport;
    this.attributes = { ...attributes };
  }

  setAttribute(key: string, value: unknown): this {
    this.attributes[key] = value;
    return this;
  }

  /** Run an async function inside this span, emit on completion. */
  async run<T>(fn: (span: this) => Promise<T>): Promise<T> {
    this.startTime = performance.now();
    let level: "info" | "error" = "info";
    let exceptionMsg: string | undefined;
    try {
      const result = await fn(this);
      return result;
    } catch (err) {
      level = "error";
      exceptionMsg = err instanceof Error ? err.stack ?? err.message : String(err);
      throw err;
    } finally {
      const durationMs = parseFloat(
        (performance.now() - this.startTime).toFixed(2)
      );
      if (exceptionMsg) this.attributes["exception"] = exceptionMsg;
      this.transport.send({
        timestamp: new Date().toISOString(),
        type: "span",
        level,
        trace_id: this.traceId,
        span_id: this.spanId,
        ...(this.parentSpanId !== undefined && { parent_span_id: this.parentSpanId }),
        message: this.name,
        duration_ms: durationMs,
        attributes: this.attributes,
      });
    }
  }
}
