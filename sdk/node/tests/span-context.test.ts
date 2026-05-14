import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { Transport } from "../src/transport.js";
import { Span } from "../src/span.js";

describe("Span context propagation", () => {
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

  // ── Root span ──────────────────────────────────────────────────────────────

  it("root span has no parent and generates its own traceId", async () => {
    const span = new Span("root", transport);
    await span.run(async () => undefined);

    expect(span.parentSpanId).toBeUndefined();
    expect(span.traceId).toBeTruthy();
    const event = sendSpy.mock.calls[0][0];
    expect(event.parent_span_id).toBeUndefined();
  });

  // ── 2-level nesting ────────────────────────────────────────────────────────

  it("child span auto-inherits parent spanId and traceId", async () => {
    let inner!: Span;

    const outer = new Span("outer", transport);
    await outer.run(async () => {
      inner = new Span("inner", transport);
      await inner.run(async () => undefined);
    });

    expect(inner.parentSpanId).toBe(outer.spanId);
    expect(inner.traceId).toBe(outer.traceId);
  });

  // ── 3-level nesting ────────────────────────────────────────────────────────

  it("three-level nesting shares traceId and has correct parent chain", async () => {
    let b!: Span;
    let c!: Span;

    const a = new Span("a", transport);
    await a.run(async () => {
      b = new Span("b", transport);
      await b.run(async () => {
        c = new Span("c", transport);
        await c.run(async () => undefined);
      });
    });

    expect(b.parentSpanId).toBe(a.spanId);
    expect(c.parentSpanId).toBe(b.spanId);
    expect(a.traceId).toBe(b.traceId);
    expect(b.traceId).toBe(c.traceId);
  });

  // ── Context restoration ────────────────────────────────────────────────────

  it("context restores to outer span after inner run completes", async () => {
    let sibling!: Span;

    const outer = new Span("outer", transport);
    await outer.run(async () => {
      const inner = new Span("inner", transport);
      await inner.run(async () => undefined);

      // After inner completes, new spans should link back to outer
      sibling = new Span("sibling", transport);
      await sibling.run(async () => undefined);
    });

    expect(sibling.parentSpanId).toBe(outer.spanId);
    expect(sibling.traceId).toBe(outer.traceId);
  });

  it("no active span after root run completes", async () => {
    const root = new Span("root", transport);
    await root.run(async () => undefined);

    const standalone = new Span("standalone", transport);
    await standalone.run(async () => undefined);

    expect(standalone.parentSpanId).toBeUndefined();
  });

  // ── Explicit override ──────────────────────────────────────────────────────

  it("explicit parentSpanId overrides parent link but inherits traceId from active context", async () => {
    const outer = new Span("outer", transport);
    await outer.run(async () => {
      const override = new Span("override", transport, {}, "explicit-parent-id");
      await override.run(async () => undefined);

      expect(override.parentSpanId).toBe("explicit-parent-id");
      expect(override.traceId).toBe(outer.traceId);
    });
  });

  it("explicit parentSpanId with no active context generates new traceId", async () => {
    const standalone = new Span("standalone", transport, {}, "explicit-parent-id");
    await standalone.run(async () => undefined);

    expect(standalone.parentSpanId).toBe("explicit-parent-id");
    expect(standalone.traceId).toBeTruthy();
  });

  // ── Parallel isolation ─────────────────────────────────────────────────────

  it("parallel run() calls do not cross-contaminate context", async () => {
    let innerA!: Span;
    let standaloneB!: Span;

    const spanA = new Span("a", transport);
    const spanB = new Span("b-outer", transport);

    await Promise.all([
      spanA.run(async () => {
        // Yield to let B start, then check context
        await new Promise((r) => setImmediate(r));
        innerA = new Span("a-inner", transport);
        await innerA.run(async () => undefined);
      }),
      spanB.run(async () => {
        await new Promise((r) => setImmediate(r));
        standaloneB = new Span("b-inner", transport);
        await standaloneB.run(async () => undefined);
      }),
    ]);

    expect(innerA.parentSpanId).toBe(spanA.spanId);
    expect(standaloneB.parentSpanId).toBe(spanB.spanId);
  });
});
