import { Transport } from "./transport.js";
import { patchConsole, unpatchConsole } from "./logger.js";
import { Span } from "./span.js";
import type { ObservrConfig } from "./types.js";

export class ObservrClient {
  readonly service: string;
  readonly collectorUrl: string;
  readonly transport: Transport;
  private readonly config: Required<ObservrConfig>;
  private started = false;

  constructor(config: Required<ObservrConfig>) {
    this.config = config;
    this.service = config.service;
    this.collectorUrl = config.collectorUrl;
    this.transport = new Transport(config.collectorUrl, config.service);
  }

  start(): void {
    if (this.started) return;
    this.started = true;

    patchConsole(this.transport, this.config.logLevel);

    if (this.config.autoInstrument) {
      this._autoInstrument();
    }
  }

  /** Wrap an async function in a named span. */
  span(name: string, attributes: Record<string, unknown> = {}): Span {
    return new Span(name, this.transport, attributes);
  }

  /**
   * Span for agent actions with standard attribute keys.
   *
   * Standard keys (omitted when undefined):
   *   agent.intent   — goal the agent is working toward
   *   agent.trigger  — what caused this action (user_message | tool_result | <span_id>)
   *   agent.model    — LLM that made the decision
   *   agent.tool     — tool being invoked
   */
  agentSpan(
    name: string,
    options: {
      intent?: string;
      trigger?: string;
      model?: string;
      tool?: string;
      [key: string]: unknown;
    } = {}
  ): Span {
    const { intent, trigger, model, tool, ...extra } = options;
    const attributes: Record<string, unknown> = { ...extra };
    if (intent !== undefined) attributes["agent.intent"] = intent;
    if (trigger !== undefined) attributes["agent.trigger"] = trigger;
    if (model !== undefined) attributes["agent.model"] = model;
    if (tool !== undefined) attributes["agent.tool"] = tool;
    return new Span(name, this.transport, attributes);
  }

  async shutdown(): Promise<void> {
    unpatchConsole();
    await this.transport.shutdown();
    this.started = false;
  }

  private _autoInstrument(): void {
    if (typeof require !== "undefined") {
      try {
        require("express");
        this._instrumentExpress();
      } catch {
        // Express not installed
      }
      try {
        require("fastify");
        this._instrumentFastify();
      } catch {
        // Fastify not installed
      }
    }
  }

  private _instrumentExpress(): void {
    try {
      const { instrumentExpress } = require("./integrations/express.js");
      instrumentExpress(this.transport);
    } catch {
      // skip
    }
  }

  private _instrumentFastify(): void {
    try {
      const { instrumentFastify } = require("./integrations/fastify.js");
      instrumentFastify(this.transport);
    } catch {
      // skip
    }
  }
}
