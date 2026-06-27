"""
Flask integration — patches Flask's request/response cycle to emit
HTTP trace events for every request.

Called automatically by ObservrClient when `flask` is detected in sys.modules.
"""

from __future__ import annotations

import secrets
import time
from datetime import datetime, timezone
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from observr._transport import Transport


def instrument_flask(transport: "Transport") -> None:
    """Patch Flask.__init__ so every new Flask app automatically gets observr hooks."""
    try:
        import flask
    except ImportError:
        return

    _original_init = flask.Flask.__init__

    def _patched_init(self, *args, **kwargs):
        _original_init(self, *args, **kwargs)
        _register_hooks(self, transport)

    flask.Flask.__init__ = _patched_init  # type: ignore[method-assign]

    # Also register hooks on any app that already exists in the current context
    _patch_existing_apps(transport)


def _patch_existing_apps(transport: "Transport") -> None:
    """
    If a Flask app was already created before init(), register hooks on it.
    """
    try:
        import flask
        app = flask._app_ctx_stack.top  # type: ignore[attr-defined]
        if app is not None and hasattr(app, "app"):
            _register_hooks(app.app, transport)
    except AttributeError:
        pass


def _register_hooks(app, transport: "Transport") -> None:
    from flask import g, request
    from observr._traceparent import format_traceparent, parse_traceparent

    @app.before_request
    def before():
        g._observr_start = time.monotonic()
        tp = request.headers.get("traceparent")
        if tp:
            parsed = parse_traceparent(tp)
            if parsed:
                g._observr_trace_id, g._observr_parent_span_id = parsed
                g._observr_span_id = secrets.token_hex(8)
                return
        g._observr_trace_id = secrets.token_hex(16)
        g._observr_span_id = secrets.token_hex(8)
        g._observr_parent_span_id = None

    @app.after_request
    def after(response):
        duration_ms = (time.monotonic() - getattr(g, "_observr_start", time.monotonic())) * 1000
        trace_id = getattr(g, "_observr_trace_id", None)
        span_id = getattr(g, "_observr_span_id", None)
        parent_span_id = getattr(g, "_observr_parent_span_id", None)
        event: dict = {
            "timestamp": datetime.now(tz=timezone.utc).isoformat(),
            "type": "http_request",
            "level": "error" if response.status_code >= 500 else "warn" if response.status_code >= 400 else "info",
            "trace_id": trace_id,
            "span_id": span_id,
            "message": f"{request.method} {request.path}",
            "method": request.method,
            "path": request.path,
            "status_code": response.status_code,
            "duration_ms": round(duration_ms, 2),
            "attributes": {
                "query_string": request.query_string.decode("utf-8", errors="replace"),
                "remote_addr": request.remote_addr,
            },
        }
        if parent_span_id:
            event["parent_span_id"] = parent_span_id
        transport.send(event)
        if trace_id and span_id:
            response.headers["traceparent"] = format_traceparent(trace_id, span_id)
        return response
