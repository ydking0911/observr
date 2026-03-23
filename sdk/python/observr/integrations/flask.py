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
    """Attach before/after request hooks to the Flask app."""
    try:
        import flask
    except ImportError:
        return

    # Use Flask's global signal hooks so we don't need the app instance
    from flask import g, request

    @flask.Flask.before_request_funcs.setdefault(None, []).append  # type: ignore[attr-defined]
    def _before() -> None:
        g._observr_start = time.monotonic()
        g._observr_trace_id = secrets.token_hex(16)
        g._observr_span_id = secrets.token_hex(8)

    @flask.Flask.after_request_funcs.setdefault(None, []).append  # type: ignore[attr-defined]
    def _after(response):
        duration_ms = (time.monotonic() - getattr(g, "_observr_start", time.monotonic())) * 1000
        transport.send({
            "timestamp": datetime.now(tz=timezone.utc).isoformat(),
            "type": "http_request",
            "level": "error" if response.status_code >= 500 else "warn" if response.status_code >= 400 else "info",
            "trace_id": getattr(g, "_observr_trace_id", None),
            "span_id": getattr(g, "_observr_span_id", None),
            "message": f"{request.method} {request.path}",
            "method": request.method,
            "path": request.path,
            "status_code": response.status_code,
            "duration_ms": round(duration_ms, 2),
            "attributes": {
                "query_string": request.query_string.decode(),
                "remote_addr": request.remote_addr,
                "user_agent": request.user_agent.string,
            },
        })
        return response

    # Monkey-patch at module level so existing app instances pick it up
    _patch_existing_apps(transport)


def _patch_existing_apps(transport: "Transport") -> None:
    """
    If a Flask app was already created before init(), register hooks on it.
    """
    import flask
    app = flask._app_ctx_stack.top  # type: ignore[attr-defined]
    if app is not None and hasattr(app, "app"):
        _register_hooks(app.app, transport)


def _register_hooks(app, transport: "Transport") -> None:
    import secrets
    import time
    from datetime import datetime, timezone
    from flask import g, request

    @app.before_request
    def before():
        g._observr_start = time.monotonic()
        g._observr_trace_id = secrets.token_hex(16)

    @app.after_request
    def after(response):
        duration_ms = (time.monotonic() - getattr(g, "_observr_start", time.monotonic())) * 1000
        transport.send({
            "timestamp": datetime.now(tz=timezone.utc).isoformat(),
            "type": "http_request",
            "level": "error" if response.status_code >= 500 else "info",
            "trace_id": getattr(g, "_observr_trace_id", None),
            "message": f"{request.method} {request.path}",
            "method": request.method,
            "path": request.path,
            "status_code": response.status_code,
            "duration_ms": round(duration_ms, 2),
            "attributes": {},
        })
        return response
