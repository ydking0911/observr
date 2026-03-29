import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { Transport } from "../src/transport.js";
import { patchConsole, unpatchConsole } from "../src/logger.js";

describe("Logger (console patch)", () => {
  let transport: Transport;
  let sendSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true }));
    transport = new Transport("http://localhost:7676", "test");
    sendSpy = vi.spyOn(transport, "send");
  });

  afterEach(async () => {
    unpatchConsole();
    await transport.shutdown();
    vi.unstubAllGlobals();
  });

  it("intercepts console.log as info level", () => {
    patchConsole(transport, "debug");
    console.log("hello from log");
    expect(sendSpy).toHaveBeenCalledOnce();
    const event = sendSpy.mock.calls[0][0];
    expect(event.type).toBe("log");
    expect(event.level).toBe("info");
    expect(event.message).toContain("hello from log");
  });

  it("intercepts console.error", () => {
    patchConsole(transport, "debug");
    console.error("something failed");
    const event = sendSpy.mock.calls[0][0];
    expect(event.level).toBe("error");
  });

  it("respects minLevel filter", () => {
    patchConsole(transport, "warn");
    console.log("debug noise");
    console.debug("more noise");
    expect(sendSpy).not.toHaveBeenCalled();
    console.warn("this matters");
    expect(sendSpy).toHaveBeenCalledOnce();
  });

  it("serializes non-string args to JSON", () => {
    patchConsole(transport, "debug");
    console.log({ key: "value" });
    const event = sendSpy.mock.calls[0][0];
    expect(event.message).toContain('"key"');
  });
});
