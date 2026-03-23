"""Unit tests for the Transport layer."""

import json
import queue
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from unittest.mock import patch

import pytest

from observr._transport import Transport


class _MockCollector(BaseHTTPRequestHandler):
    received: list[dict] = []

    def do_POST(self):  # noqa: N802
        length = int(self.headers["Content-Length"])
        body = json.loads(self.rfile.read(length))
        _MockCollector.received.extend(body.get("events", []))
        self.send_response(200)
        self.end_headers()

    def log_message(self, *args):
        pass  # silence request logs in test output


@pytest.fixture()
def mock_server():
    _MockCollector.received = []
    server = HTTPServer(("127.0.0.1", 0), _MockCollector)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    yield server
    server.shutdown()


def test_send_event_reaches_collector(mock_server):
    port = mock_server.server_address[1]
    transport = Transport(collector_url=f"http://127.0.0.1:{port}", service="test")

    transport.send({"type": "log", "message": "hello"})
    # shutdown() flushes the queue AND waits for in-flight HTTP posts to complete,
    # unlike flush() which only waits for the queue to drain.
    transport.shutdown()

    assert len(_MockCollector.received) == 1
    assert _MockCollector.received[0]["message"] == "hello"
    assert _MockCollector.received[0]["service"] == "test"


def test_send_does_not_raise_when_collector_down():
    transport = Transport(collector_url="http://127.0.0.1:1", service="test")
    # Should not raise even if collector is unreachable
    transport.send({"type": "log", "message": "hello"})
    transport.flush(timeout=0.5)


def test_queue_drops_events_when_full():
    transport = Transport(collector_url="http://127.0.0.1:1", service="test")
    # Overfill the queue (maxsize=10_000)
    for i in range(10_001):
        transport.send({"i": i})
    # Should not raise — extra events are dropped silently
