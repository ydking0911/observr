import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { ObservrClient } from "../src/client.js";
import { Transport } from "../src/transport.js";

describe("agentSpan()", () => {
  let client: ObservrClient;
  let sendSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true }));
    client = new ObservrClient({
      service: "test",
      collectorUrl: "http://localhost:7676",
      autoInstrument: false,
      logLevel: "debug",
    });
    sendSpy = vi.spyOn(client.transport, "send");
  });

  afterEach(async () => {
    await client.shutdown();
    vi.unstubAllGlobals();
  });

  // ── Standard attributes ──────────────────────────────────────────────────

  it("sets agent.intent", async () => {
    await client.agentSpan("decide", { intent: "summarize document" }).run(async () => {});
    expect(sendSpy.mock.calls[0][0].attributes?.["agent.intent"]).toBe("summarize document");
  });

  it("sets agent.trigger", async () => {
    await client.agentSpan("decide", { trigger: "user_message" }).run(async () => {});
    expect(sendSpy.mock.calls[0][0].attributes?.["agent.trigger"]).toBe("user_message");
  });

  it("sets agent.model", async () => {
    await client.agentSpan("decide", { model: "claude-sonnet-4-6" }).run(async () => {});
    expect(sendSpy.mock.calls[0][0].attributes?.["agent.model"]).toBe("claude-sonnet-4-6");
  });

  it("sets agent.tool", async () => {
    await client.agentSpan("tool.call", { tool: "web_search" }).run(async () => {});
    expect(sendSpy.mock.calls[0][0].attributes?.["agent.tool"]).toBe("web_search");
  });

  it("sets all standard attributes at once", async () => {
    await client.agentSpan("tool.call", {
      intent: "find papers",
      trigger: "user_message",
      model: "claude-sonnet-4-6",
      tool: "web_search",
    }).run(async () => {});
    const attrs = sendSpy.mock.calls[0][0].attributes ?? {};
    expect(attrs["agent.intent"]).toBe("find papers");
    expect(attrs["agent.trigger"]).toBe("user_message");
    expect(attrs["agent.model"]).toBe("claude-sonnet-4-6");
    expect(attrs["agent.tool"]).toBe("web_search");
  });

  it("omits undefined standard attributes", async () => {
    await client.agentSpan("decide", { intent: "summarize" }).run(async () => {});
    const attrs = sendSpy.mock.calls[0][0].attributes ?? {};
    expect(attrs).not.toHaveProperty("agent.trigger");
    expect(attrs).not.toHaveProperty("agent.model");
    expect(attrs).not.toHaveProperty("agent.tool");
  });

  // ── Extra attributes pass through ────────────────────────────────────────

  it("forwards extra attributes", async () => {
    await client.agentSpan("decide", { intent: "summarize", result_count: 5 }).run(async () => {});
    const attrs = sendSpy.mock.calls[0][0].attributes ?? {};
    expect(attrs["result_count"]).toBe(5);
    expect(attrs["agent.intent"]).toBe("summarize");
  });

  // ── Context propagation ──────────────────────────────────────────────────

  it("inherits parent context automatically", async () => {
    let innerParentId: string | undefined;
    let outerSpanId: string | undefined;

    await client.span("outer").run(async (outer) => {
      outerSpanId = outer.spanId;
      const inner = client.agentSpan("inner", { intent: "sub-task" });
      innerParentId = inner.parentSpanId;
      await inner.run(async () => {});
    });

    expect(innerParentId).toBe(outerSpanId);
  });

  // ── Emitted event shape ──────────────────────────────────────────────────

  it("emits span type with correct message", async () => {
    await client.agentSpan("agent.decide", { intent: "summarize" }).run(async () => {});
    const event = sendSpy.mock.calls[0][0];
    expect(event.type).toBe("span");
    expect(event.message).toBe("agent.decide");
  });
});
