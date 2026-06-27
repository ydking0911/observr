"""Tests for traceparent support in the FastAPI/Starlette integration."""
from __future__ import annotations

import json
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest


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


def _make_starlette_app(collector, service_name: str):
    pytest.importorskip("starlette")
    from starlette.applications import Starlette
    from starlette.responses import JSONResponse
    from starlette.routing import Route

    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]
    port = collector.server_address[1]
    import observr
    observr.init(service=service_name, collector_url=f"http://127.0.0.1:{port}", auto_instrument=False)
    import observr as _observr
    from observr.integrations.fastapi import ObservrMiddleware

    def homepage(request):
        return JSONResponse({"ok": True})

    app = Starlette(routes=[Route("/api", homepage)])
    app.add_middleware(ObservrMiddleware, transport=_observr._client._transport)
    return app


def test_fastapi_reads_traceparent_header(collector):
    """Starlette/FastAPI middleware extracts context from W3C traceparent."""
    from starlette.testclient import TestClient

    app = _make_starlette_app(collector, "fastapi-tp")
    with TestClient(app) as client:
        resp = client.get(
            "/api",
            headers={"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
        )
        assert resp.status_code == 200

    assert wait_for(lambda: any(e.get("path") == "/api" for e in _CollectorHandler.events))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/api")
    assert event["trace_id"] == "4bf92f3577b34da6a3ce929d0e0e4736"
    assert event["parent_span_id"] == "00f067aa0ba902b7"


def test_fastapi_sets_traceparent_response_header(collector):
    """Response includes a traceparent header continuing the trace."""
    from starlette.testclient import TestClient

    app = _make_starlette_app(collector, "fastapi-tp-resp")
    with TestClient(app) as client:
        resp = client.get(
            "/api",
            headers={"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"},
        )
    assert resp.headers.get("traceparent", "").startswith("00-4bf92f3577b34da6a3ce929d0e0e4736-")


def test_fastapi_generates_trace_id_when_no_header(collector):
    """Fresh trace_id is generated when traceparent header is absent."""
    from starlette.testclient import TestClient

    app = _make_starlette_app(collector, "fastapi-no-tp")
    with TestClient(app) as client:
        client.get("/api")

    assert wait_for(lambda: any(e.get("path") == "/api" for e in _CollectorHandler.events))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/api")
    assert event["trace_id"]
    assert "parent_span_id" not in event
