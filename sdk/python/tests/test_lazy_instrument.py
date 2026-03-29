"""
Tests for lazy instrumentation: observr.init() called BEFORE the framework
is imported — the import hook must detect the import and patch automatically.
"""

import builtins
import json
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest


# ── In-process mock collector ─────────────────────────────────────────────

class _CollectorHandler(BaseHTTPRequestHandler):
    events: list[dict] = []
    lock = threading.Lock()

    def do_POST(self):  # noqa: N802
        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length))
        with self.lock:
            self.events.extend(body.get("events", []))
        self.send_response(202)
        self.end_headers()

    def log_message(self, *args):
        pass


@pytest.fixture()
def collector():
    _CollectorHandler.events = []
    server = HTTPServer(("127.0.0.1", 0), _CollectorHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    yield server
    server.shutdown()


def wait_for(condition, timeout=2.0, interval=0.05) -> bool:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if condition():
            return True
        time.sleep(interval)
    return False


def _fresh_observr():
    """Remove all observr modules from sys.modules for a clean slate."""
    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]


# ── Tests ─────────────────────────────────────────────────────────────────

def test_lazy_fastapi_init_before_import(collector):
    """observr.init() is called BEFORE `import fastapi`. The import hook must patch it."""
    pytest.importorskip("fastapi")
    pytest.importorskip("httpx")

    # Remove fastapi and observr from modules to simulate fresh process
    _fresh_observr()
    for mod in list(sys.modules.keys()):
        if mod.startswith("fastapi") or mod.startswith("starlette"):
            del sys.modules[mod]

    port = collector.server_address[1]
    import observr
    observr.init(service="lazy-fastapi", collector_url=f"http://127.0.0.1:{port}")

    # NOW import FastAPI — the hook should kick in
    from fastapi import FastAPI
    from fastapi.testclient import TestClient

    app = FastAPI()

    @app.get("/lazy")
    async def lazy():
        return {"lazy": True}

    client = TestClient(app)
    resp = client.get("/lazy")
    assert resp.status_code == 200

    assert wait_for(lambda: any(e.get("path") == "/lazy" for e in _CollectorHandler.events))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/lazy")
    assert event["type"] == "http_request"
    assert event["service"] == "lazy-fastapi"


def test_lazy_no_double_patch(collector):
    """Importing the framework twice must not double-register middleware."""
    pytest.importorskip("fastapi")
    pytest.importorskip("httpx")

    _fresh_observr()
    for mod in list(sys.modules.keys()):
        if mod.startswith("fastapi") or mod.startswith("starlette"):
            del sys.modules[mod]

    port = collector.server_address[1]
    import observr
    observr.init(service="no-double", collector_url=f"http://127.0.0.1:{port}")

    from fastapi import FastAPI
    from fastapi.testclient import TestClient

    app = FastAPI()

    @app.get("/once")
    async def once():
        return {"ok": True}

    client = TestClient(app)
    client.get("/once")

    assert wait_for(lambda: any(e.get("path") == "/once" for e in _CollectorHandler.events))
    # Count events for this path — should be exactly 1
    events = [e for e in _CollectorHandler.events if e.get("path") == "/once"]
    assert len(events) == 1, f"expected 1 event, got {len(events)}"


def test_hook_removed_on_shutdown(collector):
    """After client.shutdown(), builtins.__import__ must be restored."""
    _fresh_observr()

    original_import = builtins.__import__
    port = collector.server_address[1]

    import observr
    client = observr.init(
        service="shutdown-test",
        collector_url=f"http://127.0.0.1:{port}",
        auto_instrument=True,
    )
    # Hook must have replaced builtins.__import__
    assert builtins.__import__ is not original_import

    client.shutdown()
    # Original must be restored
    assert builtins.__import__ is original_import
