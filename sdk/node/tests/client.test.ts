import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { init, getClient } from "../src/index.js";

describe("ObservrClient / init()", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true }));
  });

  afterEach(async () => {
    // shutdown to release timer
    try {
      await getClient().shutdown();
    } catch {
      // client may not be initialized
    }
    vi.unstubAllGlobals();
  });

  it("init() returns a client", () => {
    const client = init({ service: "my-service", autoInstrument: false });
    expect(client).toBeDefined();
    expect(client.service).toBe("my-service");
  });

  it("getClient() returns same instance", () => {
    const c1 = init({ autoInstrument: false });
    const c2 = getClient();
    expect(c1).toBe(c2);
  });

  it("getClient() throws before init()", async () => {
    // shutdown any existing client
    try {
      await getClient().shutdown();
    } catch {
      // ignore
    }
    // Reimport with a fresh module to get null _client
    const mod = await import("../src/index.js?fresh=" + Date.now());
    expect(() => mod.getClient()).toThrow("observr.init()");
  });

  it("span() returns a Span with correct name", () => {
    const client = init({ service: "svc", autoInstrument: false });
    const span = client.span("my.operation", { env: "test" });
    expect(span.name).toBe("my.operation");
  });
});
