/**
 * observr — Zero-config observability for AI agents and developers.
 *
 * Usage:
 *   import observr from "observr";
 *   observr.init({ service: "my-api" });
 *
 *   // Manual span
 *   await observr.span("db.query").run(async (span) => {
 *     const rows = await db.query("SELECT ...");
 *     span.setAttribute("row_count", rows.length);
 *     return rows;
 *   });
 */

export { ObservrClient } from "./client.js";
export { Transport } from "./transport.js";
export { Span } from "./span.js";
export { expressMiddleware } from "./integrations/express.js";
export { fastifyPlugin } from "./integrations/fastify.js";
export type { ObservrConfig, ObservrEvent } from "./types.js";

import { ObservrClient } from "./client.js";
import type { ObservrConfig } from "./types.js";

let _client: ObservrClient | null = null;

/**
 * Initialize observr. Call once at application startup.
 */
export function init(config: ObservrConfig = {}): ObservrClient {
  _client = new ObservrClient({
    service: config.service ?? "app",
    collectorUrl: config.collectorUrl ?? "http://localhost:7676",
    autoInstrument: config.autoInstrument ?? true,
    logLevel: config.logLevel ?? "debug",
  });
  _client.start();
  return _client;
}

/**
 * Return the active client. Throws if init() has not been called.
 */
export function getClient(): ObservrClient {
  if (!_client) {
    throw new Error("observr.init() must be called before getClient()");
  }
  return _client;
}

const observr = { init, getClient };
export default observr;
