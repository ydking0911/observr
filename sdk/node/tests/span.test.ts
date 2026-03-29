import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { Transport } from "../src/transport.js";
import { Span } from "../src/span.js";

describe("Span", () => {
  let transport: Transport;
  let sendSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true }));
    transport = new Transport("http://localhost:7676", "test");
    sendSpy = vi.spyOn(transport, "send");
  });

  afterEach(async () => {
    await transport.shutdown();
    vi.unstubAllGlobals();
  });

  it("emits a span event on completion", async () => {
    const span = new Span("db.query", transport, { table: "users" });
    await span.run(async () => "result");

    expect(sendSpy).toHaveBeenCalledOnce();
    const event = sendSpy.mock.calls[0][0];
    expect(event.type).toBe("span");
    expect(event.level).toBe("info");
    expect(event.message).toBe("db.query");
    expect(event.duration_ms).toBeGreaterThanOrEqual(0);
    expect(event.attributes?.table).toBe("users");
  });

  it("marks span as error when function throws", async () => {
    const span = new Span("failing.op", transport);
    await expect(
      span.run(async () => {
        throw new Error("boom");
      })
    ).rejects.toThrow("boom");

    const event = sendSpy.mock.calls[0][0];
    expect(event.level).toBe("error");
    expect(String(event.attributes?.exception)).toContain("boom");
  });

  it("setAttribute accumulates attributes", async () => {
    const span = new Span("op", transport);
    await span.run(async (s) => {
      s.setAttribute("rows", 42);
      s.setAttribute("cached", true);
    });

    const event = sendSpy.mock.calls[0][0];
    expect(event.attributes?.rows).toBe(42);
    expect(event.attributes?.cached).toBe(true);
  });

  it("generates unique trace_id and span_id", async () => {
    const ids = new Set<string>();
    for (let i = 0; i < 5; i++) {
      const span = new Span("op", transport);
      ids.add(span.traceId);
      ids.add(span.spanId);
      await span.run(async () => undefined);
    }
    expect(ids.size).toBe(10);
  });
});
