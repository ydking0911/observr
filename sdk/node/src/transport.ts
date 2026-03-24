import type { ObservrEvent } from "./types.js";

const BATCH_SIZE = 50;
const FLUSH_INTERVAL_MS = 1000;

export class Transport {
  private readonly collectorUrl: string;
  private readonly service: string;
  private queue: ObservrEvent[] = [];
  private timer: ReturnType<typeof setInterval> | null = null;
  private stopped = false;

  constructor(collectorUrl: string, service: string) {
    this.collectorUrl = collectorUrl.replace(/\/$/, "");
    this.service = service;
    this.timer = setInterval(() => this._flush(), FLUSH_INTERVAL_MS);
    // Allow process to exit without waiting for timer
    if (this.timer.unref) this.timer.unref();
  }

  send(event: Omit<ObservrEvent, "service"> & { service?: string }): void {
    if (this.stopped) return;
    const full: ObservrEvent = { service: this.service, ...event };
    if (this.queue.length >= 10_000) return; // drop silently
    this.queue.push(full);
    if (this.queue.length >= BATCH_SIZE) this._flush();
  }

  async flush(timeoutMs = 5000): Promise<void> {
    const deadline = Date.now() + timeoutMs;
    while (this.queue.length > 0 && Date.now() < deadline) {
      await this._flush();
      if (this.queue.length > 0) await sleep(50);
    }
  }

  async shutdown(): Promise<void> {
    this.stopped = true;
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = null;
    }
    await this.flush();
  }

  private _flush(): Promise<void> {
    if (this.queue.length === 0) return Promise.resolve();
    const batch = this.queue.splice(0, BATCH_SIZE);
    return this._postBatch(batch);
  }

  private async _postBatch(batch: ObservrEvent[]): Promise<void> {
    try {
      const url = `${this.collectorUrl}/events`;
      await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ events: batch }),
        signal: AbortSignal.timeout(3000),
      });
    } catch {
      // Collector not running — silently discard
    }
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}
