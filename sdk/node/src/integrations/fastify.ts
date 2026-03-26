import { randomBytes } from "node:crypto";
import type { Transport } from "../transport.js";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type FastifyInstance = any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type FastifyRequest = any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type FastifyReply = any;

/**
 * Fastify plugin that records HTTP request events via observr.
 *
 * Usage (manual registration):
 *   import { fastifyPlugin } from "observr";
 *   await app.register(fastifyPlugin(transport));
 *
 * The plugin sets `Symbol.for('skip-override')` so it applies to the
 * root scope and captures all routes regardless of encapsulation.
 */
export function fastifyPlugin(transport: Transport) {
  async function plugin(app: FastifyInstance, _options: unknown) {
    app.addHook(
      "onResponse",
      async (request: FastifyRequest, reply: FastifyReply) => {
        const statusCode: number = reply.statusCode;
        const durationMs = parseFloat((reply.elapsedTime ?? 0).toFixed(2));
        const level =
          statusCode >= 500 ? "error" : statusCode >= 400 ? "warn" : "info";
        // Prefer route pattern (e.g. /users/:id) over raw URL to avoid
        // high-cardinality values polluting pattern detection.
        // Fastify 4+: routeOptions.url holds the pattern; routerPath was removed.
        const path: string =
          request.routeOptions?.url ??
          request.routerPath ??
          request.url?.split("?")[0] ??
          "/";

        transport.send({
          timestamp: new Date().toISOString(),
          type: "http_request",
          level,
          trace_id: randomBytes(16).toString("hex"),
          span_id: randomBytes(8).toString("hex"),
          message: `${request.method} ${path}`,
          method: request.method,
          path,
          status_code: statusCode,
          duration_ms: durationMs,
          attributes: {
            remote_addr: request.ip,
            user_agent: request.headers?.["user-agent"],
          },
        });
      }
    );
  }

  // Disable Fastify encapsulation so the hook applies to all routes.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (plugin as any)[Symbol.for("skip-override")] = true;
  return plugin;
}

/** Auto-instrument Fastify by patching its module-cache factory entry. */
export function instrumentFastify(transport: Transport): void {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const mod = require("fastify");
    if (!mod || mod.__observr_patched) return;

    const original: (...args: unknown[]) => FastifyInstance =
      mod.default ?? mod;
    const plugin = fastifyPlugin(transport);

    const wrapped = function (...args: unknown[]) {
      const app = original(...args) as FastifyInstance;
      app.register(plugin);
      return app;
    };
    Object.assign(wrapped, original);
    wrapped.__observr_patched = true;

    // Patch the CommonJS module cache so all subsequent require('fastify')
    // calls return the wrapped factory.
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const Module = require("module");
    const cached = Module._cache[require.resolve("fastify")];
    if (cached) {
      if (mod.default) {
        cached.exports.default = wrapped;
      } else {
        cached.exports = wrapped;
      }
    }
  } catch {
    // Fastify not installed — skip silently
  }
}
