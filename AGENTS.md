# AGENTS.md

Guidance for AI coding agents (Codex, Claude, Cursor, Devin, etc.) working in the observr repository.

---

## Project at a Glance

observr is a **zero-config local observability stack** composed of:

- **`observrd`** — Go 1.22 daemon. Receives events via HTTP, stores in SQLite, streams via WebSocket (dashboard) and SSE (`tail` CLI).
- **Python SDK** — `pip install observr`. Auto-instruments Flask, FastAPI, Django. Lazy import hook via `builtins.__import__` override.
- **Node.js SDK** — `npm install observr`. Auto-instruments Express. Console patch. Manual spans via async `.run()`.
- **React dashboard** — Vite SPA embedded in the `observrd` binary.

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

---

## File Ownership Map

| Path | Owner | Notes |
|------|-------|-------|
| `server/cmd/observrd/main.go` | Go | Subcommand dispatch, SSE+WS broadcaster wiring |
| `server/internal/storage/store.go` | Go | Single source of truth for DB schema and `Broadcaster` interface |
| `server/internal/tail/tail.go` | Go | SSE hub; filters on level/service/type |
| `server/internal/dashboard/hub.go` | Go | WebSocket hub; `all:dist` embed |
| `sdk/python/observr/_client.py` | Python | Lazy import hook, lifecycle, framework dispatch |
| `sdk/python/observr/_transport.py` | Python | Background thread, queue, HTTP POST |
| `sdk/python/observr/integrations/fastapi.py` | Python | Patches `fastapi.FastAPI.__init__` |
| `sdk/python/observr/integrations/django.py` | Python | WSGI middleware; `_get_transport()` fallback |
| `sdk/node/src/transport.ts` | TypeScript | `fetch` + `AbortSignal.timeout`, `unref()` timer |
| `sdk/node/src/span.ts` | TypeScript | Async span, error capture |
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
