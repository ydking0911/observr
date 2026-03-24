"""
Django integration — middleware that emits HTTP trace events for every request.

Called automatically by ObservrClient when `django` is detected in sys.modules,
OR can be added manually to MIDDLEWARE in settings.py:

    MIDDLEWARE = [
        "observr.integrations.django.ObservrMiddleware",
        ...
    ]
"""

from __future__ import annotations

import secrets
import time
from datetime import datetime, timezone
from typing import TYPE_CHECKING, Callable

if TYPE_CHECKING:
    from observr._transport import Transport


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
        return  # already patched — avoid double-wrapping on re-init

    original_load = existing

    def _patched_load(self, *args, **kwargs):
        original_load(self, *args, **kwargs)
        _prepend_middleware(self, transport)

    _patched_load._observr_patched = True  # type: ignore[attr-defined]
    base_handler.BaseHandler.load_middleware = _patched_load  # type: ignore[method-assign]


def _prepend_middleware(handler, transport: "Transport") -> None:
    """Wrap the existing _middleware_chain with ObservrMiddleware."""
    if isinstance(getattr(handler, "_middleware_chain", None), ObservrMiddleware):
        return  # already wrapped
    handler._middleware_chain = ObservrMiddleware(transport, handler._middleware_chain)


class ObservrMiddleware:
    """
    Django WSGI middleware.
    Can also be added manually to settings.MIDDLEWARE as a string path.
    """

    def __init__(self, get_response_or_transport, get_response: Callable | None = None) -> None:
        # Support two calling conventions:
        #   1. Django settings.py string: Django calls __init__(get_response)
        #   2. Direct instantiation: ObservrMiddleware(transport, get_response)
        if get_response is None:
            # Called by Django's middleware loader: first arg is get_response
            self._transport = _get_transport()
            self._get_response: Callable = get_response_or_transport
        else:
            self._transport = get_response_or_transport
            self._get_response = get_response

    def __call__(self, request):
        trace_id = secrets.token_hex(16)
        span_id = secrets.token_hex(8)
        start = time.monotonic()

        response = self._get_response(request)

        duration_ms = round((time.monotonic() - start) * 1000, 2)
        status_code = response.status_code
        level = "error" if status_code >= 500 else "warn" if status_code >= 400 else "info"

        path = getattr(request, "path", "/")
        method = getattr(request, "method", "")

        self._transport.send({
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
            "attributes": {
                "remote_addr": _get_ip(request),
                "user_agent": request.META.get("HTTP_USER_AGENT", ""),
            },
        })
        return response


def _get_ip(request) -> str:
    x_forwarded = request.META.get("HTTP_X_FORWARDED_FOR", "")
    if x_forwarded:
        return x_forwarded.split(",")[0].strip()
    return request.META.get("REMOTE_ADDR", "")


def _get_transport() -> "Transport":
    """Fallback: retrieve transport from the global observr client."""
    import observr
    client = observr._client
    if client is None:
        raise RuntimeError("observr.init() must be called before Django middleware loads")
    return client._transport
