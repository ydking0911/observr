"""
FastAPI / Starlette integration — ASGI middleware that emits HTTP trace events.

Called automatically by ObservrClient when `fastapi` or `starlette` is
detected in sys.modules.
"""

from __future__ import annotations

import secrets
import time
from datetime import datetime, timezone
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from observr._transport import Transport


def instrument_fastapi(transport: "Transport") -> None:
    """
    Monkey-patch FastAPI.__init__ so any app created after observr.init()
    automatically gets the ObservrMiddleware injected.
    """
    try:
        import fastapi
    except ImportError:
        return

    _original_init = fastapi.FastAPI.__init__

    def _patched_init(self, *args, **kwargs):
        _original_init(self, *args, **kwargs)
        self.add_middleware(ObservrMiddleware, transport=transport)

    fastapi.FastAPI.__init__ = _patched_init  # type: ignore[method-assign]


class ObservrMiddleware:
    """
    Starlette/FastAPI ASGI middleware.
    Can also be added manually:

        app.add_middleware(ObservrMiddleware, transport=transport)
    """

    def __init__(self, app, transport: "Transport") -> None:
        self.app = app
        self._transport = transport

    async def __call__(self, scope, receive, send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        trace_id = secrets.token_hex(16)
        span_id = secrets.token_hex(8)
        start = time.monotonic()

        status_code = 500
        original_send = send

        async def _send(message):
            nonlocal status_code
            if message["type"] == "http.response.start":
                status_code = message["status"]
            await original_send(message)

        try:
            await self.app(scope, receive, _send)
        finally:
            duration_ms = (time.monotonic() - start) * 1000
            method = scope.get("method", "")
            path = scope.get("path", "")

            self._transport.send({
                "timestamp": datetime.now(tz=timezone.utc).isoformat(),
                "type": "http_request",
                "level": "error" if status_code >= 500 else "warn" if status_code >= 400 else "info",
                "trace_id": trace_id,
                "span_id": span_id,
                "message": f"{method} {path}",
                "method": method,
                "path": path,
                "status_code": status_code,
                "duration_ms": round(duration_ms, 2),
                "attributes": {
                    "query_string": scope.get("query_string", b"").decode(),
                    "client": scope.get("client"),
                },
            })
