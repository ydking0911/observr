# AGENTS.md

Guidance for AI coding agents (Codex, Claude, Cursor, Devin, etc.) working in the observr repository.

---

## Project at a Glance

observr is an **open-source audit trail and accountability layer for AI agents**. It captures every agent action, tool call, and log event with full causal attribution, stores them in an immutable local audit log, and exposes them for querying, alerting, and compliance export.

**Strategic direction**: developer-first open source — make it easy for developers building AI agents to naturally adopt audit features, understand what their agent did and why, and contribute back to the project.

Components:

- **`observrd`** — Go 1.22 daemon. Receives events via HTTP, stores in SQLite (WAL), streams via WebSocket (dashboard) and SSE (`tail` CLI). Runs a `multiBroadcaster` that fans out to dashboard, SSE, and webhook alerter — new audit sinks plug in here.
- **Python SDK** — `pip install observr`. Auto-instruments Flask, FastAPI, Django. Lazy import hook via `builtins.__import__` override.
- **Node.js SDK** — `npm install @ydking0911/observr`. Auto-instruments Express. Console patch. Manual spans via async `.run()`.
- **React dashboard** — Vite SPA embedded in the `observrd` binary. Real-time audit event browser.
- **Patterns engine** (`server/internal/patterns/`) — fingerprints normalized event messages to group behavioral patterns across time.
- **Webhook alerter** (`server/internal/webhook/`) — fires Slack/Discord alerts on threshold violations. Acts as a policy enforcement hook.

---

## Verified Commands

Before running any command, `cd` to the correct directory.

### Python SDK
```bash
cd sdk/python
pip install -e ".[dev]"           # install editable + dev deps
python -m pytest tests/ -v        # run all tests
ruff check observr/ tests/        # lint
```

### Node.js SDK
```bash
cd sdk/node
npm install
npm test                          # vitest
npx tsc --noEmit                  # type-check
npm run build                     # compile to dist/
```

### Go Server
```bash
cd server
go mod tidy
go vet ./...
go test ./... -race -timeout 60s
CGO_ENABLED=1 go build -o bin/observrd ./cmd/observrd
```

### Dashboard
```bash
cd dashboard
npm install && npm run build      # output → server/internal/dashboard/dist/
```

### E2E (requires built binary + Python SDK installed)
```bash
python scripts/test_e2e.py
```

---

## Architecture Decisions Agents Must Respect

### Go embed directive
Always use `//go:embed all:dist` (not `dist/*`). The `all:` prefix includes hidden files like `.gitkeep`. Never change this.

### `.gitignore` scope
`sdk/python/dist/` is scoped (not global `dist/`) so that `server/internal/dashboard/dist/.gitkeep` can be committed. Never change `sdk/python/dist/` back to a global `dist/` rule.

### Python lazy instrumentation (`_client.py`)
The `builtins.__import__` hook has two critical guards:
1. `name == top` — only trigger on the top-level package (e.g., `"fastapi"`), not submodules (e.g., `"fastapi.routing"`).
2. `not was_in_modules` — skip if the package was already (even partially) in `sys.modules` before the call, to avoid patching a half-initialised module during circular imports inside FastAPI's own `__init__.py`.

Do **not** simplify or remove these guards. They fix a subtle bug where `fastapi/applications.py` calls `from fastapi import routing`, which re-triggers the hook with a partial module.

### Go `WriteTimeout = 0`
The HTTP server sets `WriteTimeout: 0` (no timeout). This is intentional because `GET /tail` SSE connections are long-lived. Do not set a finite `WriteTimeout` unless you also handle SSE separately.

### CGO requirement
The Go server uses `mattn/go-sqlite3` which requires CGO. Always build with `CGO_ENABLED=1`. CI installs `gcc libc-dev` on Ubuntu. Do not attempt a CGO-free build.

### `Broadcaster` interface is the audit sink extension point
New audit outputs (on-chain anchoring, compliance exporters, SIEM integrations) must implement `storage.Broadcaster` and be wired into the `multiBroadcaster` in `main.go`. Never bypass this interface by reading from SQLite directly in a goroutine.

### Patterns engine normalises before fingerprinting
`patterns.Normalize()` replaces UUIDs, IPs, hex strings, and numbers with placeholders before grouping. This is intentional — it makes behaviorally identical events group together even when IDs differ. Do not change the normalization order (UUID must precede hex to avoid partial replacement of UUID segments).

---

## File Ownership Map

| Path | Owner | Notes |
|------|-------|-------|
| `server/cmd/observrd/main.go` | Go | Subcommand dispatch, SSE+WS+webhook broadcaster wiring |
| `server/internal/storage/store.go` | Go | Single source of truth for DB schema and `Broadcaster` interface |
| `server/internal/tail/tail.go` | Go | SSE hub; filters on level/service/type |
| `server/internal/dashboard/hub.go` | Go | WebSocket hub; `all:dist` embed |
| `server/internal/patterns/patterns.go` | Go | Behavioral fingerprinting — normalise + group events |
| `server/internal/webhook/alerter.go` | Go | Policy enforcement broadcaster; Slack/Discord alerts |
| `sdk/python/observr/_client.py` | Python | Lazy import hook, lifecycle, framework dispatch |
| `sdk/python/observr/_transport.py` | Python | Background thread, queue, HTTP POST |
| `sdk/python/observr/integrations/fastapi.py` | Python | Patches `fastapi.FastAPI.__init__` |
| `sdk/python/observr/integrations/django.py` | Python | WSGI middleware; `_get_transport()` fallback |
| `sdk/node/src/transport.ts` | TypeScript | `fetch` + `AbortSignal.timeout`, `unref()` timer |
| `sdk/node/src/span.ts` | TypeScript | Async span, error capture; carries `parent_span_id` for causal chain |
| `.github/workflows/ci.yml` | CI | All language test matrix |
| `.github/workflows/publish-py.yml` | CI | PyPI OIDC trusted publishing |
| `.github/workflows/publish-node.yml` | CI | npm publish |

---

## What NOT to Do

- **Do not** mock the SQLite database in Go tests — use `":memory:"` or a temp file.
- **Do not** add `time.Sleep()` in tests except in tail/SSE tests where it's necessary for subscription registration (keep sleeps ≤ 50ms).
- **Do not** add mandatory runtime dependencies to the Python SDK (`dependencies` must stay `[]`).
- **Do not** use `any` types in public TypeScript APIs.
- **Do not** change `//go:embed all:dist` to `//go:embed dist/*`.
- **Do not** commit `.claude/` directory contents (gitignored).
- **Do not** commit `server/observrd` build artifacts (not gitignored by default — add to `.gitignore` if needed).

---

## Event Schema

All events share this JSON shape (sent to `POST /events` and stored in SQLite):

```json
{
  "timestamp": "2026-03-24T12:00:00.000Z",  // ISO 8601 UTC
  "type": "http_request" | "span" | "log",
  "level": "debug" | "info" | "warn" | "error",
  "service": "my-api",
  "trace_id": "hex32",         // optional
  "span_id": "hex16",          // optional
  "message": "GET /users",
  "method": "GET",             // http_request only
  "path": "/users",            // http_request only
  "status_code": 200,          // http_request only
  "duration_ms": 12.5,         // http_request + span
  "attributes": {}             // arbitrary key-value
}
```

The collector (`server/internal/collector/handler.go`) accepts a batch: `{ "events": [...] }`.

---

## Ports and Defaults

| Service | Default | Flag |
|---------|---------|------|
| observrd HTTP | 7676 | `--port` |
| Vite dev server | 5173 | — |
| SQLite file | `./observr.db` | `--db` |
