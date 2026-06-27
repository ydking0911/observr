// sdk/node/tests/express.test.ts
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { expressMiddleware } from "../src/integrations/express.js";
import { Transport } from "../src/transport.js";

type FakeReq = {
  method: string;
  path: string;
  url: string;
  ip?: string;
  headers: Record<string, string | string[] | undefined>;
};

type FakeRes = {
  statusCode: number;
  on: (event: string, fn: () => void) => void;
  setHeader: (name: string, value: string) => void;
};

function makeRes(
  statusCode = 200,
  onFinish?: () => void
): FakeRes & { capturedHeaders: Record<string, string> } {
  const capturedHeaders: Record<string, string> = {};
  return {
    statusCode,
    capturedHeaders,
    on(event, fn) {
      if (event === "finish") fn();
    },
    setHeader(name, value) {
      capturedHeaders[name.toLowerCase()] = value;
    },
  };
}

describe("expressMiddleware traceparent", () => {
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

  it("reads traceparent header and uses its trace_id and parent_id", () => {
    const mw = expressMiddleware(transport);
    const req: FakeReq = {
      method: "GET",
      path: "/api",
      url: "/api",
      headers: {
        traceparent:
          "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
      },
    };
    const res = makeRes();
    mw(req as never, res as never, () => {});

    const event = sendSpy.mock.calls[0][0];
    expect(event.trace_id).toBe("4bf92f3577b34da6a3ce929d0e0e4736");
    expect(event.parent_span_id).toBe("00f067aa0ba902b7");
  });

  it("sets traceparent response header continuing the same trace", () => {
    const mw = expressMiddleware(transport);
    const req: FakeReq = {
      method: "GET",
      path: "/api",
      url: "/api",
      headers: {
        traceparent:
          "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
      },
    };
    const res = makeRes();
    mw(req as never, res as never, () => {});

    expect(res.capturedHeaders["traceparent"]).toMatch(
      /^00-4bf92f3577b34da6a3ce929d0e0e4736-/
    );
  });

  it("generates new trace_id when traceparent header is absent", () => {
    const mw = expressMiddleware(transport);
    const req: FakeReq = {
      method: "GET",
      path: "/",
      url: "/",
      headers: {},
    };
    const res = makeRes();
    mw(req as never, res as never, () => {});

    const event = sendSpy.mock.calls[0][0];
    expect(event.trace_id).toBeTruthy();
    expect(event.parent_span_id).toBeUndefined();
  });

  it("ignores invalid traceparent and falls back to new trace_id", () => {
    const mw = expressMiddleware(transport);
    const req: FakeReq = {
      method: "GET",
      path: "/",
      url: "/",
      headers: { traceparent: "not-valid" },
    };
    const res = makeRes();
    mw(req as never, res as never, () => {});

    const event = sendSpy.mock.calls[0][0];
    expect(event.trace_id).toBeTruthy();
    expect(event.parent_span_id).toBeUndefined();
  });
});
