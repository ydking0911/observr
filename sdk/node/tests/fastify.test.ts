import { describe, it, expect, vi, beforeEach } from "vitest";
import { fastifyPlugin } from "../src/integrations/fastify.js";
import type { Transport } from "../src/transport.js";

// Minimal Transport stub
function makeTransport() {
  const sent: unknown[] = [];
  return {
    send: vi.fn((event: unknown) => sent.push(event)),
    sent,
  } as unknown as Transport & { sent: unknown[] };
}

// Minimal Fastify instance mock
function makeFastify() {
  const hooks: Record<string, ((...args: unknown[]) => Promise<void>)[]> = {};
  return {
    addHook(name: string, fn: (...args: unknown[]) => Promise<void>) {
      hooks[name] = hooks[name] ?? [];
      hooks[name].push(fn);
    },
    async trigger(name: string, ...args: unknown[]) {
      for (const fn of hooks[name] ?? []) {
        await fn(...args);
      }
    },
    hooks,
  };
}

function makeRequest(overrides: Partial<{
  method: string;
  routerPath: string;
  url: string;
  ip: string;
  headers: Record<string, string>;
}> = {}) {
  return {
    method: "GET",
    routerPath: "/users/:id",
    url: "/users/42",
    ip: "127.0.0.1",
    headers: { "user-agent": "test-agent" },
    ...overrides,
  };
}

function makeReply(statusCode: number, elapsedTime = 12.5) {
  return { statusCode, elapsedTime };
}

describe("fastifyPlugin", () => {
  let transport: Transport & { sent: unknown[] };

  beforeEach(() => {
    transport = makeTransport();
  });

  it("registers an onResponse hook", async () => {
    const app = makeFastify();
    const plugin = fastifyPlugin(transport);
    await plugin(app, {});
    expect(app.hooks["onResponse"]).toHaveLength(1);
  });

  it("sends http_request event on 200 response", async () => {
    const app = makeFastify();
    await fastifyPlugin(transport)(app, {});
    await app.trigger("onResponse", makeRequest(), makeReply(200, 20));

    expect(transport.send).toHaveBeenCalledOnce();
    const event = (transport.send as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(event.type).toBe("http_request");
    expect(event.level).toBe("info");
    expect(event.status_code).toBe(200);
    expect(event.method).toBe("GET");
  });

  it("sets level=error for 5xx responses", async () => {
    const app = makeFastify();
    await fastifyPlugin(transport)(app, {});
    await app.trigger("onResponse", makeRequest(), makeReply(500));

    const event = (transport.send as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(event.level).toBe("error");
    expect(event.status_code).toBe(500);
  });

  it("sets level=warn for 4xx responses", async () => {
    const app = makeFastify();
    await fastifyPlugin(transport)(app, {});
    await app.trigger("onResponse", makeRequest(), makeReply(404));

    const event = (transport.send as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(event.level).toBe("warn");
    expect(event.status_code).toBe(404);
  });

  it("uses routerPath over raw url as path", async () => {
    const app = makeFastify();
    await fastifyPlugin(transport)(app, {});
    await app.trigger(
      "onResponse",
      makeRequest({ routerPath: "/users/:id", url: "/users/42" }),
      makeReply(200)
    );

    const event = (transport.send as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(event.path).toBe("/users/:id");
  });

  it("falls back to url when routerPath is undefined", async () => {
    const app = makeFastify();
    await fastifyPlugin(transport)(app, {});
    const req = makeRequest({ url: "/health?check=1" });
    // @ts-expect-error intentionally removing routerPath
    delete req.routerPath;
    await app.trigger("onResponse", req, makeReply(200));

    const event = (transport.send as ReturnType<typeof vi.fn>).mock.calls[0][0];
    // query string should be stripped
    expect(event.path).toBe("/health");
  });

  it("records duration_ms from reply.elapsedTime", async () => {
    const app = makeFastify();
    await fastifyPlugin(transport)(app, {});
    await app.trigger("onResponse", makeRequest(), makeReply(200, 42.567));

    const event = (transport.send as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(event.duration_ms).toBeCloseTo(42.57, 1);
  });

  it("includes trace_id and span_id in event", async () => {
    const app = makeFastify();
    await fastifyPlugin(transport)(app, {});
    await app.trigger("onResponse", makeRequest(), makeReply(200));

    const event = (transport.send as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(event.trace_id).toMatch(/^[0-9a-f]{32}$/);
    expect(event.span_id).toMatch(/^[0-9a-f]{16}$/);
  });

  it("includes remote_addr and user_agent in attributes", async () => {
    const app = makeFastify();
    await fastifyPlugin(transport)(app, {});
    await app.trigger(
      "onResponse",
      makeRequest({ ip: "10.0.0.1", headers: { "user-agent": "curl/7.x" } }),
      makeReply(200)
    );

    const event = (transport.send as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(event.attributes.remote_addr).toBe("10.0.0.1");
    expect(event.attributes.user_agent).toBe("curl/7.x");
  });

  it("sets skip-override symbol for root-scope applicability", () => {
    const plugin = fastifyPlugin(transport);
    expect(plugin[Symbol.for("skip-override")]).toBe(true);
  });
});
