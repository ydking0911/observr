"""
Tests for Django integration.

Django requires settings to be configured before any Django module is used,
so we use django.test.override_settings and a minimal settings dict.
"""

import json
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

import pytest


# ── Minimal Django setup ──────────────────────────────────────────────────

def _configure_django():
    django = pytest.importorskip("django")
    from django.conf import settings
    if not settings.configured:
        settings.configure(
            DEBUG=True,
            DATABASES={},
            INSTALLED_APPS=[
                "django.contrib.contenttypes",
                "django.contrib.auth",
            ],
            ROOT_URLCONF=__name__,  # use this module as urlconf
            MIDDLEWARE=[
                "observr.integrations.django.ObservrMiddleware",
            ],
        )
        django.setup()
    return django


# Minimal URL conf used by tests in this module
urlpatterns: list = []


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


# ── Tests ─────────────────────────────────────────────────────────────────

def test_django_middleware_traces_request(collector):
    """Django middleware emits an http_request event for each request."""
    pytest.importorskip("django")

    # Remove observr modules so we get a fresh client
    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    _configure_django()

    port = collector.server_address[1]
    import observr
    observr.init(service="django-test", collector_url=f"http://127.0.0.1:{port}", auto_instrument=False)

    from django.test import RequestFactory
    from observr.integrations.django import ObservrMiddleware
    import observr as _observr

    transport = _observr._client._transport

    def simple_view(request):
        from django.http import JsonResponse
        return JsonResponse({"ok": True})

    factory = RequestFactory()
    request = factory.get("/api/hello")
    middleware = ObservrMiddleware(transport, simple_view)
    response = middleware(request)

    assert response.status_code == 200
    assert wait_for(lambda: any(
        e.get("path") == "/api/hello" for e in _CollectorHandler.events
    ))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/api/hello")
    assert event["type"] == "http_request"
    assert event["method"] == "GET"
    assert event["status_code"] == 200
    assert event["service"] == "django-test"


def test_django_middleware_captures_4xx(collector):
    pytest.importorskip("django")

    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    _configure_django()

    port = collector.server_address[1]
    import observr
    observr.init(service="django-4xx", collector_url=f"http://127.0.0.1:{port}", auto_instrument=False)

    from django.test import RequestFactory
    from django.http import HttpResponse
    from observr.integrations.django import ObservrMiddleware
    import observr as _observr

    transport = _observr._client._transport

    def not_found_view(request):
        return HttpResponse("not found", status=404)

    factory = RequestFactory()
    request = factory.get("/missing")
    middleware = ObservrMiddleware(transport, not_found_view)
    middleware(request)

    assert wait_for(lambda: any(
        e.get("path") == "/missing" for e in _CollectorHandler.events
    ))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/missing")
    assert event["level"] == "warn"
    assert event["status_code"] == 404


def test_django_middleware_duration(collector):
    pytest.importorskip("django")

    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    _configure_django()

    port = collector.server_address[1]
    import observr
    observr.init(service="django-dur", collector_url=f"http://127.0.0.1:{port}", auto_instrument=False)

    from django.test import RequestFactory
    from django.http import JsonResponse
    from observr.integrations.django import ObservrMiddleware
    import observr as _observr

    transport = _observr._client._transport

    def slow_view(request):
        time.sleep(0.05)
        return JsonResponse({"slow": True})

    factory = RequestFactory()
    request = factory.get("/slow")
    middleware = ObservrMiddleware(transport, slow_view)
    middleware(request)

    assert wait_for(lambda: any(e.get("path") == "/slow" for e in _CollectorHandler.events))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/slow")
    assert event["duration_ms"] >= 40


# ── Incoming trace context propagation ───────────────────────────────────────

def test_django_middleware_uses_incoming_trace_id(collector):
    """X-Trace-Id header is used as trace_id instead of generating a new one."""
    pytest.importorskip("django")

    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    _configure_django()

    port = collector.server_address[1]
    import observr
    observr.init(service="django-trace", collector_url=f"http://127.0.0.1:{port}", auto_instrument=False)

    from django.test import RequestFactory
    from django.http import JsonResponse
    from observr.integrations.django import ObservrMiddleware
    import observr as _observr

    transport = _observr._client._transport

    def view(request):
        return JsonResponse({"ok": True})

    factory = RequestFactory()
    request = factory.get("/trace-ctx", HTTP_X_TRACE_ID="upstream-trace-abc123")
    middleware = ObservrMiddleware(transport, view)
    middleware(request)

    assert wait_for(lambda: any(e.get("path") == "/trace-ctx" for e in _CollectorHandler.events))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/trace-ctx")
    assert event["trace_id"] == "upstream-trace-abc123"


def test_django_middleware_uses_incoming_span_id_as_parent(collector):
    """X-Span-Id header is recorded as parent_span_id for causal attribution."""
    pytest.importorskip("django")

    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    _configure_django()

    port = collector.server_address[1]
    import observr
    observr.init(service="django-parent", collector_url=f"http://127.0.0.1:{port}", auto_instrument=False)

    from django.test import RequestFactory
    from django.http import JsonResponse
    from observr.integrations.django import ObservrMiddleware
    import observr as _observr

    transport = _observr._client._transport

    def view(request):
        return JsonResponse({"ok": True})

    factory = RequestFactory()
    request = factory.get(
        "/parent-ctx",
        HTTP_X_TRACE_ID="trace-xyz",
        HTTP_X_SPAN_ID="parent-span-99",
    )
    middleware = ObservrMiddleware(transport, view)
    middleware(request)

    assert wait_for(lambda: any(e.get("path") == "/parent-ctx" for e in _CollectorHandler.events))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/parent-ctx")
    assert event["trace_id"] == "trace-xyz"
    assert event["parent_span_id"] == "parent-span-99"


def test_django_middleware_generates_trace_id_when_header_absent(collector):
    """A fresh trace_id is generated when X-Trace-Id header is not present."""
    pytest.importorskip("django")

    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    _configure_django()

    port = collector.server_address[1]
    import observr
    observr.init(service="django-gen", collector_url=f"http://127.0.0.1:{port}", auto_instrument=False)

    from django.test import RequestFactory
    from django.http import JsonResponse
    from observr.integrations.django import ObservrMiddleware
    import observr as _observr

    transport = _observr._client._transport

    def view(request):
        return JsonResponse({"ok": True})

    factory = RequestFactory()
    request = factory.get("/no-headers")
    middleware = ObservrMiddleware(transport, view)
    middleware(request)

    assert wait_for(lambda: any(e.get("path") == "/no-headers" for e in _CollectorHandler.events))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/no-headers")
    assert event["trace_id"]  # auto-generated, non-empty
    assert "parent_span_id" not in event  # absent when header not sent


# ── Exception handling ────────────────────────────────────────────────────────

def test_django_middleware_emits_event_on_view_exception(collector):
    """When the view raises, an error event is emitted and the exception re-raised."""
    pytest.importorskip("django")

    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    _configure_django()

    port = collector.server_address[1]
    import observr
    observr.init(service="django-exc", collector_url=f"http://127.0.0.1:{port}", auto_instrument=False)

    from django.test import RequestFactory
    from observr.integrations.django import ObservrMiddleware
    import observr as _observr

    transport = _observr._client._transport

    def raising_view(request):
        raise ValueError("kaboom")

    factory = RequestFactory()
    request = factory.get("/exc-path")
    middleware = ObservrMiddleware(transport, raising_view)

    with pytest.raises(ValueError, match="kaboom"):
        middleware(request)

    assert wait_for(lambda: any(e.get("path") == "/exc-path" for e in _CollectorHandler.events))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/exc-path")
    assert event["level"] == "error"
    assert event["status_code"] == 500
    assert event["attributes"]["error_type"] == "ValueError"
    assert "kaboom" in event["attributes"]["error"]


async def test_django_asgi_middleware_emits_event_on_view_exception(collector):
    """ASGI path: exception in async view is recorded and re-raised."""
    pytest.importorskip("django")

    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    _configure_django()

    port = collector.server_address[1]
    import observr
    observr.init(service="django-asgi-exc", collector_url=f"http://127.0.0.1:{port}", auto_instrument=False)

    from django.test import RequestFactory
    from observr.integrations.django import ObservrMiddleware
    import observr as _observr

    transport = _observr._client._transport

    async def raising_async_view(request):
        raise RuntimeError("async boom")

    factory = RequestFactory()
    request = factory.get("/async-exc")
    middleware = ObservrMiddleware(transport, raising_async_view)

    with pytest.raises(RuntimeError, match="async boom"):
        await middleware(request)

    assert wait_for(lambda: any(e.get("path") == "/async-exc" for e in _CollectorHandler.events))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/async-exc")
    assert event["level"] == "error"
    assert event["status_code"] == 500
    assert event["attributes"]["error_type"] == "RuntimeError"


# ── ASGI (async) path ─────────────────────────────────────────────────────────

async def test_django_asgi_middleware_emits_event(collector):
    """Async get_response triggers the ASGI path; event is emitted correctly."""
    pytest.importorskip("django")

    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    _configure_django()

    port = collector.server_address[1]
    import observr
    observr.init(service="django-asgi", collector_url=f"http://127.0.0.1:{port}", auto_instrument=False)

    from django.test import RequestFactory
    from django.http import JsonResponse
    from observr.integrations.django import ObservrMiddleware
    import observr as _observr

    transport = _observr._client._transport

    async def async_view(request):
        return JsonResponse({"async": True})

    factory = RequestFactory()
    request = factory.get("/async-path")
    middleware = ObservrMiddleware(transport, async_view)

    # In ASGI mode __call__ returns a coroutine; await it directly.
    response = await middleware(request)

    assert response.status_code == 200
    assert wait_for(lambda: any(e.get("path") == "/async-path" for e in _CollectorHandler.events))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/async-path")
    assert event["type"] == "http_request"
    assert event["method"] == "GET"
    assert event["status_code"] == 200
    assert event["service"] == "django-asgi"


async def test_django_asgi_propagates_trace_headers(collector):
    """ASGI path also reads X-Trace-Id and X-Span-Id headers."""
    pytest.importorskip("django")

    for mod in list(sys.modules.keys()):
        if mod.startswith("observr"):
            del sys.modules[mod]

    _configure_django()

    port = collector.server_address[1]
    import observr
    observr.init(service="django-asgi-ctx", collector_url=f"http://127.0.0.1:{port}", auto_instrument=False)

    from django.test import RequestFactory
    from django.http import JsonResponse
    from observr.integrations.django import ObservrMiddleware
    import observr as _observr

    transport = _observr._client._transport

    async def async_view(request):
        return JsonResponse({"ok": True})

    factory = RequestFactory()
    request = factory.get(
        "/async-ctx",
        HTTP_X_TRACE_ID="async-trace-id",
        HTTP_X_SPAN_ID="async-parent-span",
    )
    middleware = ObservrMiddleware(transport, async_view)
    await middleware(request)

    assert wait_for(lambda: any(e.get("path") == "/async-ctx" for e in _CollectorHandler.events))
    event = next(e for e in _CollectorHandler.events if e.get("path") == "/async-ctx")
    assert event["trace_id"] == "async-trace-id"
    assert event["parent_span_id"] == "async-parent-span"
