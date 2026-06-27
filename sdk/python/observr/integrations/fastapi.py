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

        from observr._traceparent import format_traceparent, parse_traceparent

        # Extract traceparent from ASGI headers (list of (bytes, bytes) tuples)
        headers_dict = {k.lower(): v for k, v in scope.get("headers", [])}
        parent_span_id: str | None = None
        trace_id: str | None = None

        # W3C traceparent takes priority
        tp_bytes = headers_dict.get(b"traceparent")
        if tp_bytes:
            parsed = parse_traceparent(tp_bytes.decode("utf-8", errors="replace"))
            if parsed:
                trace_id, parent_span_id = parsed

        # Fall back to legacy X-Trace-Id / X-Span-Id headers
        if trace_id is None:
            x_trace = headers_dict.get(b"x-trace-id")
            x_span = headers_dict.get(b"x-span-id")
            trace_id = x_trace.decode("utf-8", errors="replace") if x_trace else secrets.token_hex(16)
            if x_span:
                parent_span_id = x_span.decode("utf-8", errors="replace")

        span_id = secrets.token_hex(8)
        start = time.monotonic()
        status_code = 500
        original_send = send
        traceparent_bytes = format_traceparent(trace_id, span_id).encode()

        async def _send(message):
            nonlocal status_code
            if message["type"] == "http.response.start":
                status_code = message["status"]
                response_headers = list(message.get("headers", []))
                response_headers.append((b"traceparent", traceparent_bytes))
                message = {**message, "headers": response_headers}
            await original_send(message)

        try:
            await self.app(scope, receive, _send)
        finally:
            duration_ms = (time.monotonic() - start) * 1000
            method = scope.get("method", "")
            path = scope.get("path", "")
            event: dict = {
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
                    "query_string": scope.get("query_string", b"").decode("utf-8", errors="replace"),
                    "client": scope.get("client"),
                },
            }
            if parent_span_id:
                event["parent_span_id"] = parent_span_id
            self._transport.send(event)
