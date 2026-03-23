"""
Integration tests for the Python SDK.

These tests spin up a real in-process HTTP collector mock and verify that
the full SDK pipeline (init → middleware → transport → collector) works end-to-end.
"""

import json
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


def wait_for(condition, timeout=2.0, interval=0.05):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if condition():
            return True
        time.sleep(interval)
    return False


# ── Tests ─────────────────────────────────────────────────────────────────

def test_init_returns_client(collector):
    import observr
    port = collector.server_address[1]
    client = observr.init(
        service="test-svc",
        collector_url=f"http://127.0.0.1:{port}",
        auto_instrument=False,
    )
    assert client is not None
    assert client.service == "test-svc"


def test_log_capture_reaches_collector(collector):
    import logging
    import observr

    port = collector.server_address[1]
    observr.init(
        service="log-test",
        collector_url=f"http://127.0.0.1:{port}",
        auto_instrument=False,
        log_level="DEBUG",
    )

    logger = logging.getLogger("test.integration.log")
    logger.error("payment failed", extra={"user_id": "u_123", "amount": 9900})

    assert wait_for(lambda: len(_CollectorHandler.events) >= 1)
    event = _CollectorHandler.events[-1]
    assert event["level"] == "error"
    assert event["message"] == "payment failed"
    assert event["attributes"]["user_id"] == "u_123"


def test_log_exception_captured(collector):
    import logging
    import observr

    port = collector.server_address[1]
    observr.init(
        service="exc-test",
        collector_url=f"http://127.0.0.1:{port}",
        auto_instrument=False,
    )

    logger = logging.getLogger("test.integration.exc")
    try:
        raise ValueError("boom")
    except ValueError:
        logger.exception("something went wrong")

    assert wait_for(lambda: any(
        "exception" in e.get("attributes", {})
        for e in _CollectorHandler.events
    ))
    event = next(
        e for e in _CollectorHandler.events
        if "exception" in e.get("attributes", {})
    )
    assert "ValueError" in event["attributes"]["exception"]


def test_fastapi_middleware_traces_request(collector):
    pytest.importorskip("fastapi")
    pytest.importorskip("httpx")

    import importlib
    import sys

    # Fresh import so the patched FastAPI.__init__ picks up our collector URL
    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    import observr
    port = collector.server_address[1]
    observr.init(service="fastapi-test", collector_url=f"http://127.0.0.1:{port}")

    from fastapi import FastAPI
    from fastapi.testclient import TestClient

    app = FastAPI()

    @app.get("/ping")
    async def ping():
        return {"ok": True}

    client = TestClient(app)
    response = client.get("/ping")
    assert response.status_code == 200

    assert wait_for(lambda: any(
        e.get("path") == "/ping" for e in _CollectorHandler.events
    ))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/ping")
    assert event["type"] == "http_request"
    assert event["method"] == "GET"
    assert event["status_code"] == 200
    assert event["duration_ms"] >= 0


def test_fastapi_middleware_captures_500(collector):
    pytest.importorskip("fastapi")
    pytest.importorskip("httpx")

    import sys
    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    import observr
    port = collector.server_address[1]
    observr.init(service="fastapi-500", collector_url=f"http://127.0.0.1:{port}")

    from fastapi import FastAPI
    from fastapi.testclient import TestClient

    app = FastAPI()

    @app.get("/error")
    async def error():
        raise RuntimeError("intentional error")

    client = TestClient(app, raise_server_exceptions=False)
    client.get("/error")

    assert wait_for(lambda: any(
        e.get("path") == "/error" and e.get("status_code", 0) >= 500
        for e in _CollectorHandler.events
    ))


def test_manual_span(collector):
    import sys
    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    import observr
    port = collector.server_address[1]
    client = observr.init(
        service="span-test",
        collector_url=f"http://127.0.0.1:{port}",
        auto_instrument=False,
    )

    with client.span("db.query", table="users") as span:
        time.sleep(0.01)
        span.set_attribute("row_count", 42)

    assert wait_for(lambda: any(
        e.get("message") == "db.query" for e in _CollectorHandler.events
    ))
    event = next(e for e in _CollectorHandler.events if e.get("message") == "db.query")
    assert event["type"] == "span"
    assert event["attributes"]["table"] == "users"
    assert event["attributes"]["row_count"] == 42
    assert event["duration_ms"] >= 0
