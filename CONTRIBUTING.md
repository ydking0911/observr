# Contributing to observr

Thank you for your interest in contributing! observr is an open-source project and we welcome contributions of all kinds — bug reports, feature requests, documentation improvements, and code changes.

---

## Table of Contents

- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Project Structure](#project-structure)
- [Pull Request Guidelines](#pull-request-guidelines)
- [Coding Standards](#coding-standards)
- [Running Tests](#running-tests)
- [Opening Issues](#opening-issues)

---

## Branch Model

observr uses a two-branch workflow:

| Branch | Purpose |
|--------|---------|
| `develop` | Active development. All PRs target this branch. |
| `main` | Stable, released code only. Merged from `develop` by a maintainer at release time. |

Contributors never push directly to `main`.

---

## Getting Started

1. **Fork** the repository and clone your fork locally.
2. Sync your fork's `develop` branch with upstream before starting work.
3. Create a new branch **from `develop`** for your work:
   ```bash
   git checkout develop
   git pull upstream develop
   git checkout -b feat/your-feature-name
   # or
   git checkout -b fix/issue-number-short-description
   ```
4. Make your changes, write tests, and verify everything passes.
5. Open a Pull Request against the **`develop`** branch.

---

## Development Setup

### Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | ≥ 1.22 | Collector daemon (`observrd`) |
| Python | ≥ 3.10 | Python SDK |
| Node.js | ≥ 18 | Node.js/TypeScript SDK & dashboard |
| SQLite3 / gcc | system | CGO build for Go |

### Python SDK

```bash
cd sdk/python
pip install -e ".[dev]"   # installs observr in editable mode + test deps
```

### Node.js SDK

```bash
cd sdk/node
npm install
npm run build
```

### Go Server

```bash
cd server
go mod tidy
go build ./...
```

### Dashboard (optional)

```bash
cd dashboard
npm install
npm run build   # output goes to server/internal/dashboard/dist/
```

---

## Project Structure

```
observr/
├── server/                     # Go collector daemon (observrd)
│   ├── cmd/observrd/main.go    # Entry point + subcommands (query, tail)
│   └── internal/
│       ├── collector/          # POST /events handler
│       ├── storage/            # SQLite persistence
│       ├── query/              # GET /query handler + CLI
│       ├── tail/               # GET /tail SSE endpoint
│       └── dashboard/          # WebSocket hub + embedded React UI
├── sdk/
│   ├── python/                 # Python SDK (pip install observr)
│   │   ├── observr/
│   │   │   ├── _client.py      # ObservrClient, lazy import hook
│   │   │   ├── _transport.py   # Background HTTP transport
│   │   │   ├── _logger.py      # logging.Handler integration
│   │   │   ├── _span.py        # Manual span context manager
│   │   │   └── integrations/   # Flask, FastAPI, Django patches
│   │   └── tests/
│   └── node/                   # Node.js/TypeScript SDK (npm install observr)
│       └── src/
│           ├── index.ts        # Public API
│           ├── transport.ts    # Background fetch transport
│           ├── span.ts         # Manual span (async/await)
│           ├── logger.ts       # console patch
│           └── integrations/
│               └── express.ts  # Express middleware
├── dashboard/                  # React + Vite frontend
├── scripts/                    # install.sh, test_e2e.py
├── .github/workflows/          # CI + release + publish pipelines
└── Formula/                    # Homebrew formula
```

---

## Pull Request Guidelines

- **Target branch**: all PRs must target **`develop`**. PRs targeting `main` will be closed.
- **One thing per PR**: one feature or one bug fix per pull request. If you have multiple changes, open multiple PRs.
- **Link an issue**: every PR should reference an existing issue (`Closes #123`). If no issue exists, open one first.
- **Clear title and description**:
  - Title: short imperative summary (`Add Django middleware`, `Fix lazy instrumentation race`)
  - Description: what changed, why, and how to test it.
- **WIP label**: if your PR is not ready for review, add the `WIP` label and remove it when ready.
- **Keep it small**: large PRs are hard to review. Prefer multiple focused PRs over one giant one.
- **Tests required**: all non-trivial changes must include tests. See [Running Tests](#running-tests).

### PR Checklist

```
[ ] Targets develop branch (NOT main)
[ ] Branched from develop (not from main)
[ ] References an issue
[ ] Tests pass locally (Python, Go, Node.js)
[ ] Lint passes (ruff for Python, tsc --noEmit for TypeScript, go vet for Go)
[ ] No secrets or personal data committed
[ ] CHANGELOG or PR description updated
```

---

## Coding Standards

### Go

- Follow standard Go conventions (`gofmt`, `go vet`).
- All exported types and functions must have doc comments.
- Error handling: always check errors, never use `_` for important errors.
- Use `context.Context` for cancellable operations.

### Python

- Target Python 3.10+.
- Run `ruff check` before committing. All lint errors must be resolved.
- Type annotations required for all public functions.
- Zero mandatory dependencies for the SDK core — keep `dependencies = []` in `pyproject.toml`.
- Instrumentation patches must be idempotent (safe to call multiple times).

### TypeScript / Node.js

- Target Node.js 18+, ES2020.
- Run `tsc --noEmit` before committing.
- No `any` types in public APIs.
- Transport must be non-blocking and never throw (silent drop on failure).

---

## Running Tests

### All tests at once

```bash
# Python SDK
cd sdk/python && python -m pytest tests/ -v

# Node.js SDK
cd sdk/node && npm test

# Go server
cd server && go test ./... -race -timeout 60s

# Lint
cd sdk/python && ruff check observr/ tests/
cd sdk/node && npx tsc --noEmit
cd server && go vet ./...
```

### E2E test (requires a built binary)

```bash
cd server && go build -o bin/observrd ./cmd/observrd
python scripts/test_e2e.py
```

---

## Opening Issues

When opening a bug report, please include:

- observr version (`pip show observr` / `observrd --version`)
- OS and Python/Node/Go version
- Minimal reproduction steps
- Expected vs. actual behaviour
- Relevant logs (with sensitive data redacted)

For feature requests, describe the use-case and why existing functionality doesn't cover it.

---

## Questions?

Open a [GitHub Discussion](https://github.com/ydking0911/observr/discussions) or an issue — we're happy to help you get started.
