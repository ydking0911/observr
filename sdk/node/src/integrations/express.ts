import { randomBytes } from "node:crypto";
import type { Transport } from "../transport.js";

type Req = {
  method: string;
  path: string;
  url: string;
  ip?: string;
  headers: Record<string, string | string[] | undefined>;
};
type Res = {
  statusCode: number;
  on: (event: string, fn: () => void) => void;
};
type Next = () => void;

/**
 * Express middleware factory.
 * Usage:
 *   import { expressMiddleware } from "observr/express";
 *   app.use(expressMiddleware(transport));
 */
export function expressMiddleware(transport: Transport) {
  return function observrMiddleware(req: Req, res: Res, next: Next): void {
    const traceId = randomBytes(16).toString("hex");
    const spanId = randomBytes(8).toString("hex");
    const start = performance.now();

    res.on("finish", () => {
      const durationMs = parseFloat((performance.now() - start).toFixed(2));
      const statusCode = res.statusCode;
      const level =
        statusCode >= 500 ? "error" : statusCode >= 400 ? "warn" : "info";
      const path = req.path || req.url || "/";
      transport.send({
        timestamp: new Date().toISOString(),
        type: "http_request",
        level,
        trace_id: traceId,
        span_id: spanId,
        message: `${req.method} ${path}`,
        method: req.method,
        path,
        status_code: statusCode,
        duration_ms: durationMs,
        attributes: {
          remote_addr: req.ip,
          user_agent: req.headers["user-agent"],
        },
      });
    });

    next();
  };
}

/** Auto-instrument Express by monkey-patching express() factory. */
export function instrumentExpress(transport: Transport): void {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const express = require("express");
    if (!express || express.__observr_patched) return;

    const originalExpress = express.default ?? express;
    const mw = expressMiddleware(transport);

    const wrapped = function (...args: unknown[]) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const app = originalExpress(...args) as any;
      app.use(mw);
      return app;
    };
    Object.assign(wrapped, originalExpress);
    wrapped.__observr_patched = true;

    if (express.default) {
      express.default = wrapped;
    } else {
      // CJS: mutate the module cache entry
      // eslint-disable-next-line @typescript-eslint/no-require-imports
      const Module = require("module");
      const cached = Module._cache[require.resolve("express")];
      if (cached) cached.exports = wrapped;
    }
  } catch {
    // Express not installed — skip
  }
}
