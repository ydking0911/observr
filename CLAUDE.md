# CLAUDE.md

This file provides guidance to Claude Code when working with the observr repository.

---

## Project Overview

**observr** is an open-source audit trail and accountability layer for AI agents. It captures every agent action, tool call, and log event with full causal attribution (`parent_span_id` links child spans to their causal parent), stores them in an immutable local audit log, and exposes them for querying and alerting.

The strategic direction is **developer-first open source**: make it easy for developers to naturally adopt audit features, contribute back, and grow the project through community. Target users are developers building AI agents who want to understand what their agent did and why — not enterprise sales.

| Component | Language | Path | Purpose |
|-----------|----------|------|---------|
| `observrd` | Go 1.22 | `server/` | Collector daemon — HTTP intake, SQLite, WebSocket, SSE |
| Python SDK | Python 3.10+ | `sdk/python/` | Auto-instruments Flask, FastAPI, Django; lazy import hook |
| Node.js SDK | TypeScript / Node 18+ | `sdk/node/` | Auto-instruments Express; console patch; manual spans |
| Dashboard | React + Vite | `dashboard/` | Real-time audit event browser, embedded in `observrd` binary |
| CI/CD | GitHub Actions | `.github/workflows/` | Test, release, PyPI publish, npm publish |

---

## Running Tests

```bash
# Python SDK (from sdk/python/)
python -m pytest tests/ -v

# Node.js SDK (from sdk/node/)
npm test

# Go server (from server/)
go test ./... -race -timeout 60s

# Lint
cd sdk/python && ruff check observr/ tests/
cd sdk/node && npx tsc --noEmit
cd server && go vet ./...
```

---

## Key Architecture Notes

### Go Server (`server/`)

- **Entry point**: `cmd/observrd/main.go` — three modes: daemon, `query` subcommand, `tail` subcommand.
- **Storage**: SQLite via `mattn/go-sqlite3` (CGO required). WAL mode. All writes go through `storage.Store.Insert()`.
- **Broadcaster**: `storage.Store` holds a `Broadcaster` interface. In daemon mode, a `multiBroadcaster` fans out to the WebSocket hub (`dashboard.Hub`), the SSE tail hub (`tail.Hub`), and the webhook alerter (`webhook.Alerter`). **New audit sinks (e.g., on-chain anchoring) implement this interface and plug in here with zero changes to existing code.**
- **Patterns** (`internal/patterns/`): normalises event messages into fingerprints (UUID/IP/hex/number → placeholders) and groups them by frequency. This is the behavioral analysis layer — the foundation for audit reports and compliance exports.
- **Webhook alerter** (`internal/webhook/`): implements `Broadcaster`. Fires Slack/Discord alerts when error thresholds are exceeded. Acts as a policy enforcement hook.
- **Dashboard embed**: `//go:embed all:dist` in `hub.go`. The `all:` prefix is intentional — `dist/*` would miss hidden files like `.gitkeep`. The file `server/internal/dashboard/dist/.gitkeep` must stay committed so `go vet` passes without a built dashboard.
- **SSE endpoint** (`GET /tail`): streams new events as `data: <json>\n\n`. Filters: `?level=`, `?service=`, `?type=`. Used by `observrd tail` CLI.
- **WriteTimeout**: set to `0` (no timeout) for the HTTP server because SSE connections are long-lived.

### Python SDK (`sdk/python/`)

- **Lazy instrumentation**: `_client.py` overrides `builtins.__import__` to detect when Flask/FastAPI/Django is imported after `observr.init()`. Key subtleties:
  - Only triggers on **top-level** package imports (`name == top`) to avoid firing during internal circular imports (e.g., `fastapi.applications` imports `from fastapi import routing`).
  - Uses `was_in_modules` guard: if the package was already (even partially) in `sys.modules` before the import call, skip patching. This prevents patching a half-initialised module during FastAPI's own `__init__.py` execution.
  - `shutdown()` restores `builtins.__import__` to its original value.
- **FastAPI instrumentation**: patches `fastapi.FastAPI.__init__` to call `app.add_middleware(ObservrMiddleware, ...)` on every new app instance.
- **Transport**: background thread, 10k event queue, silent drop, 1s flush interval, POSTs JSON to `/events`.
- **Span API**: `client.span(name, parent_span_id=None, **attrs)` — context manager, propagates via `ContextVar`. `client.agent_span(name, *, intent, trigger, model, tool, **extra)` — thin wrapper that pre-populates standard agent attribute keys; standard keys take priority over `**extra`.
- **Zero dependencies**: `sdk/python/pyproject.toml` has `dependencies = []`. Flask, FastAPI, Django are optional extras.

### Node.js SDK (`sdk/node/`)

- **Transport**: uses `fetch()` (Node 18+ built-in), `AbortSignal.timeout(3000)`, background `setInterval`. Timer is `.unref()`'d so the process can exit normally.
- **Span API**: `client.span(name, attrs).run(async (s) => { ... })` — async, emits on completion. `client.agentSpan(name, { intent?, trigger?, model?, tool?, ...extra })` — same contract as Python's `agent_span()`; destructures standard keys, passes remainder as arbitrary attributes.
- **Console patch**: replaces `console.log/debug/warn/error` with wrapped versions that send log events. Restored by `unpatchConsole()`.
- **Build**: `tsup` outputs both ESM and CJS with `.d.ts` declarations.

---

## Important `.gitignore` Rules

- `sdk/python/dist/` — Python wheel build output (scoped, not global `dist/`).
- `sdk/node/dist/` — TypeScript compiled output.
- `server/internal/dashboard/dist/*` with `!server/internal/dashboard/dist/.gitkeep` — keeps the placeholder so `go:embed` works.
- `.claude/` — local Claude plans/logs (not committed).

---

## CI Workflows

| Workflow | Trigger | What it does |
|----------|---------|-------------|
| `ci.yml` | push/PR to `develop` | Python tests (3.10–3.12), Node.js tests, Go tests + build, dashboard build, E2E |
| `release.yml` | tag `v*` | Cross-compile Go binaries for 4 platforms, create GitHub Release |
| `publish-py.yml` | GitHub Release published | Build Python wheel, publish to PyPI via OIDC trusted publishing |
| `publish-node.yml` | GitHub Release published | npm test + build, publish to npm |

---

## Conventions

- **Commit style**: `type: short description` (e.g., `feat: add Django support`, `fix: lazy instrumentation circular import`).
- **Branch naming**: `feat/#<issue>/<short-name>` or `fix/#<issue>/<short-name>`, always branched from `develop`.
- **Branch model**: `develop` is the integration branch (all PRs land here); `main` is stable release-only (maintainer merges from `develop` at release time). Never PR directly to `main`.
- **No force-push to `main` or `develop`**.
- **Go module path**: `github.com/ydking0911/observr/server`.
- **PyPI package**: `observr` (published at pypi.org/project/observr).
- **npm package**: `observr` (published at npmjs.com/package/observr).

---

## Common Pitfalls

1. **`go:embed all:dist` vs `dist/*`**: always use `all:dist`. The `dist/*` glob silently fails when only `.gitkeep` exists.
2. **FastAPI import order**: in production code, import FastAPI BEFORE `observr.init()` OR use lazy instrumentation (init first). Both work. In tests that re-import observr, be careful about `sys.modules` state.
3. **Django middleware settings**: when added via `settings.py` string (`"observr.integrations.django.ObservrMiddleware"`), Django's middleware loader calls `__init__(get_response)`. When used programmatically, call `ObservrMiddleware(transport, get_response)`.
4. **CGO for SQLite**: Go builds require `CGO_ENABLED=1` and `gcc`/`libc-dev`. CI installs `gcc libc-dev` before building.
5. **Node.js fetch + AbortSignal.timeout**: requires Node 18+. Node 16 does not have built-in `fetch`.
