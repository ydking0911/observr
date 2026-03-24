import type { Transport } from "./transport.js";

type LogLevel = "debug" | "info" | "warn" | "error";

const LEVEL_RANK: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
};

const CONSOLE_MAP: Record<LogLevel, keyof Console> = {
  debug: "debug",
  info: "log",
  warn: "warn",
  error: "error",
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type OriginalConsoleFn = (...args: any[]) => void;

const originals = new Map<LogLevel, OriginalConsoleFn>();

export function patchConsole(transport: Transport, minLevel: LogLevel): void {
  const levels: LogLevel[] = ["debug", "info", "warn", "error"];
  for (const level of levels) {
    const consoleKey = CONSOLE_MAP[level] as keyof typeof console;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const original = (console as any)[consoleKey] as OriginalConsoleFn;
    originals.set(level, original);

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (console as any)[consoleKey] = (...args: any[]) => {
      original.apply(console, args);
      if (LEVEL_RANK[level] >= LEVEL_RANK[minLevel]) {
        const message = args
          .map((a) =>
            typeof a === "string" ? a : JSON.stringify(a)
          )
          .join(" ");
        transport.send({
          timestamp: new Date().toISOString(),
          type: "log",
          level,
          message,
          attributes: {},
        });
      }
    };
  }
}

export function unpatchConsole(): void {
  for (const [level, original] of originals) {
    const consoleKey = CONSOLE_MAP[level] as keyof typeof console;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (console as any)[consoleKey] = original;
  }
  originals.clear();
}
