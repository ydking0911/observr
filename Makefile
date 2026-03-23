# observr Makefile
# Usage: make <target>

.PHONY: help build build-server build-dashboard install-py dev-server dev-dashboard \
        test test-go test-py test-e2e lint lint-go lint-py lint-dashboard clean

# ── Config ────────────────────────────────────────────────────────────────
BIN_DIR   := server/bin
BINARY    := $(BIN_DIR)/observrd
DB_PATH   := ./observr.db
PORT      := 7676

# ── Default ───────────────────────────────────────────────────────────────
help:
	@echo ""
	@echo "  observr — development commands"
	@echo ""
	@echo "  Build"
	@echo "    make build            Build everything (dashboard + server binary)"
	@echo "    make build-server     Build Go server binary only"
	@echo "    make build-dashboard  Build React dashboard only"
	@echo ""
	@echo "  Dev"
	@echo "    make dev-server       Run observrd in hot-reload mode (requires Air)"
	@echo "    make dev-dashboard    Run Vite dev server (proxies to :$(PORT))"
	@echo "    make install-py       Install Python SDK in editable mode"
	@echo ""
	@echo "  Test"
	@echo "    make test             Run all tests (Go + Python)"
	@echo "    make test-go          Go unit tests"
	@echo "    make test-py          Python unit + integration tests"
	@echo "    make test-e2e         Full end-to-end test (requires built binary)"
	@echo ""
	@echo "  Lint"
	@echo "    make lint             Lint everything"
	@echo "    make lint-go          go vet + staticcheck"
	@echo "    make lint-py          ruff check"
	@echo "    make lint-dashboard   eslint"
	@echo ""
	@echo "  Misc"
	@echo "    make clean            Remove build artifacts and test DB"
	@echo ""

# ── Build ─────────────────────────────────────────────────────────────────
build: build-dashboard build-server

build-server:
	@echo "→ Building observrd..."
	@mkdir -p $(BIN_DIR)
	cd server && go mod tidy && CGO_ENABLED=1 go build -ldflags="-s -w" -o bin/observrd ./cmd/observrd
	@echo "  ✓ $(BINARY)"

build-dashboard:
	@echo "→ Building dashboard..."
	cd dashboard && npm install --silent && npm run build
	@echo "  ✓ server/internal/dashboard/dist/"

# ── Dev ───────────────────────────────────────────────────────────────────
dev-server:
	@echo "→ Starting observrd (port $(PORT))..."
	cd server && go run ./cmd/observrd --port=$(PORT) --db=../$(DB_PATH)

dev-dashboard:
	@echo "→ Starting Vite dev server..."
	cd dashboard && npm run dev

install-py:
	@echo "→ Installing Python SDK (editable)..."
	python -m pip install -e "sdk/python[dev]"
	@echo "  ✓ observr installed"

# ── Test ──────────────────────────────────────────────────────────────────
test: test-go test-py

test-go:
	@echo "→ Running Go tests..."
	cd server && go test ./... -v -count=1 -race

test-py:
	@echo "→ Running Python tests..."
	cd sdk/python && python -m pytest tests/ -v

test-e2e: build-server
	@echo "→ Running e2e test..."
	python scripts/test_e2e.py

# ── Lint ──────────────────────────────────────────────────────────────────
lint: lint-go lint-py lint-dashboard

lint-go:
	@echo "→ Linting Go..."
	cd server && go vet ./...
	@command -v staticcheck >/dev/null 2>&1 && cd server && staticcheck ./... || \
		echo "  ⚠ staticcheck not found (go install honnef.co/go/tools/cmd/staticcheck@latest)"

lint-py:
	@echo "→ Linting Python..."
	@command -v ruff >/dev/null 2>&1 && ruff check sdk/python || \
		echo "  ⚠ ruff not found (pip install ruff)"

lint-dashboard:
	@echo "→ Linting dashboard..."
	cd dashboard && npm run lint 2>/dev/null || echo "  ⚠ eslint not configured"

# ── Clean ─────────────────────────────────────────────────────────────────
clean:
	@echo "→ Cleaning..."
	rm -rf $(BIN_DIR)
	rm -rf server/internal/dashboard/dist/*
	rm -f server/internal/dashboard/dist/.gitkeep
	touch server/internal/dashboard/dist/.gitkeep
	rm -f $(DB_PATH) /tmp/observr-e2e-test.db /tmp/observr_e2e_app.py
	find sdk/python -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
	find sdk/python -name "*.egg-info" -exec rm -rf {} + 2>/dev/null || true
	@echo "  ✓ done"
