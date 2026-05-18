"""
Django integration — middleware that emits HTTP trace events for every request.

Works for both WSGI and ASGI deployments. When the inner ``get_response``
callable is a coroutine function (ASGI mode), the middleware is automatically
marked as async so Django's ASGI handler calls it natively without a thread.

Called automatically by ObservrClient when ``django`` is detected in
sys.modules, OR can be added manually to settings.py::

    MIDDLEWARE = [
        "observr.integrations.django.ObservrMiddleware",
        ...
    ]

Incoming trace context
----------------------
If the request carries ``X-Trace-Id`` / ``X-Span-Id`` headers, the middleware
reuses them so cross-service traces stay linked:

    trace_id      ← X-Trace-Id   (generated if absent)
    parent_span_id ← X-Span-Id   (omitted if absent)
"""

from __future__ import annotations

import asyncio
import secrets
import time
from datetime import datetime, timezone
from typing import TYPE_CHECKING, Callable

if TYPE_CHECKING:
    from observr._transport import Transport

# Use asgiref helpers when available (always true for Django 3.1+).
try:
    from asgiref.sync import iscoroutinefunction, markcoroutinefunction
except ImportError:
    iscoroutinefunction = asyncio.iscoroutinefunction  # type: ignore[assignment]

    def markcoroutinefunction(fn):  # type: ignore[misc]
        fn._is_coroutine = asyncio.coroutines._is_coroutine


def instrument_django(transport: "Transport") -> None:
    """
    Monkey-patch Django's base handler so ObservrMiddleware is prepended
    to every application's middleware stack.
    """
    try:
        import django.core.handlers.base as base_handler
    except ImportError:
        return

    existing = base_handler.BaseHandler.load_middleware
    if getattr(existing, "_observr_patched", False):
        return

    original_load = existing

    def _patched_load(self, *args, **kwargs):
        original_load(self, *args, **kwargs)
        _prepend_middleware(self, transport)

    _patched_load._observr_patched = True  # type: ignore[attr-defined]
    base_handler.BaseHandler.load_middleware = _patched_load  # type: ignore[method-assign]


def _prepend_middleware(handler, transport: "Transport") -> None:
    if isinstance(getattr(handler, "_middleware_chain", None), ObservrMiddleware):
        return
    handler._middleware_chain = ObservrMiddleware(transport, handler._middleware_chain)


class ObservrMiddleware:
    """
    Django middleware — WSGI and ASGI compatible.

    Supports two calling conventions:
      1. Django settings.py string path — Django calls ``__init__(get_response)``
      2. Direct instantiation — ``ObservrMiddleware(transport, get_response)``
    """

    def __init__(self, get_response_or_transport, get_response: Callable | None = None) -> None:
        if get_response is None:
            self._transport = _get_transport()
            self._get_response: Callable = get_response_or_transport
        else:
            self._transport = get_response_or_transport
            self._get_response = get_response

        # Mark this middleware as async when the inner chain is async (ASGI).
        # Django's ASGIHandler then awaits __call__ instead of using a thread.
        if iscoroutinefunction(self._get_response):
            markcoroutinefunction(self)

    def __call__(self, request):
        if iscoroutinefunction(self._get_response):
            return self._acall(request)
        return self._call(request)

    # ── Synchronous (WSGI) ────────────────────────────────────────────────

    def _call(self, request):
        trace_id, span_id, parent_span_id = _extract_ids(request)
        start = time.monotonic()
        try:
            response = self._get_response(request)
        except Exception as exc:
            _emit(self._transport, request, None, trace_id, span_id, parent_span_id, start, exc=exc)
            raise
        _emit(self._transport, request, response, trace_id, span_id, parent_span_id, start)
        return response

    # ── Asynchronous (ASGI) ───────────────────────────────────────────────

    async def _acall(self, request):
        trace_id, span_id, parent_span_id = _extract_ids(request)
        start = time.monotonic()
        try:
            response = await self._get_response(request)
        except Exception as exc:
            _emit(self._transport, request, None, trace_id, span_id, parent_span_id, start, exc=exc)
            raise
        _emit(self._transport, request, response, trace_id, span_id, parent_span_id, start)
        return response


# ── Helpers ───────────────────────────────────────────────────────────────

def _extract_ids(request) -> tuple[str, str, str | None]:
    """Read or generate trace / span IDs from incoming request headers."""
    trace_id = request.META.get("HTTP_X_TRACE_ID") or secrets.token_hex(16)
    parent_span_id = request.META.get("HTTP_X_SPAN_ID") or None
    span_id = secrets.token_hex(8)
    return trace_id, span_id, parent_span_id


def _emit(
    transport: "Transport",
    request,
    response,
    trace_id: str,
    span_id: str,
    parent_span_id: str | None,
    start: float,
    *,
    exc: BaseException | None = None,
) -> None:
    duration_ms = round((time.monotonic() - start) * 1000, 2)
    if response is not None:
        status_code = response.status_code
        level = "error" if status_code >= 500 else "warn" if status_code >= 400 else "info"
    else:
        status_code = 500
        level = "error"
    path = getattr(request, "path", "/")
    method = getattr(request, "method", "")

    attrs: dict = {
        "remote_addr": _get_ip(request),
        "user_agent": request.META.get("HTTP_USER_AGENT", ""),
    }
    if exc is not None:
        attrs["error"] = str(exc)
        attrs["error_type"] = type(exc).__name__

    event: dict = {
        "timestamp": datetime.now(tz=timezone.utc).isoformat(),
        "type": "http_request",
        "level": level,
        "trace_id": trace_id,
        "span_id": span_id,
        "message": f"{method} {path}",
        "method": method,
        "path": path,
        "status_code": status_code,
        "duration_ms": duration_ms,
        "attributes": attrs,
    }
    if parent_span_id:
        event["parent_span_id"] = parent_span_id
    transport.send(event)


def _get_ip(request) -> str:
    x_forwarded = request.META.get("HTTP_X_FORWARDED_FOR", "")
    if x_forwarded:
        return x_forwarded.split(",")[0].strip()
    return request.META.get("REMOTE_ADDR", "")


def _get_transport() -> "Transport":
    import observr
    client = observr._client
    if client is None:
        raise RuntimeError("observr.init() must be called before Django middleware loads")
    return client._transport
