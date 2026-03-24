import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { Transport } from "../src/transport.js";

describe("Transport", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchSpy);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("sends events to collector", async () => {
    const t = new Transport("http://localhost:7676", "test-service");
    t.send({
      timestamp: new Date().toISOString(),
      type: "log",
      level: "info",
      message: "hello",
    });
    await t.flush();
    await t.shutdown();

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, opts] = fetchSpy.mock.calls[0];
    expect(url).toBe("http://localhost:7676/events");
    const body = JSON.parse(opts.body as string);
    expect(body.events).toHaveLength(1);
    expect(body.events[0].message).toBe("hello");
    expect(body.events[0].service).toBe("test-service");
  });

  it("silently drops events when collector is unreachable", async () => {
    fetchSpy.mockRejectedValue(new Error("ECONNREFUSED"));
    const t = new Transport("http://localhost:9999", "test-service");
    t.send({
      timestamp: new Date().toISOString(),
      type: "log",
      level: "info",
      message: "dropped",
    });
    await expect(t.flush()).resolves.not.toThrow();
    await t.shutdown();
  });

  it("enforces queue limit of 10,000", async () => {
    fetchSpy.mockRejectedValue(new Error("ECONNREFUSED"));
    const t = new Transport("http://localhost:9999", "test-service");
    for (let i = 0; i < 11_000; i++) {
      t.send({
        timestamp: new Date().toISOString(),
        type: "log",
        level: "info",
        message: `event-${i}`,
      });
    }
    // Should not throw
    await t.shutdown();
  });

  it("strips trailing slash from collectorUrl", async () => {
    const t = new Transport("http://localhost:7676/", "svc");
    t.send({
      timestamp: new Date().toISOString(),
      type: "log",
      level: "info",
      message: "x",
    });
    await t.flush();
    await t.shutdown();
    const [url] = fetchSpy.mock.calls[0];
    expect(url).toBe("http://localhost:7676/events");
  });
});
