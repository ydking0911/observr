"""Manual span context manager for custom tracing."""

from __future__ import annotations

import secrets
import time
from contextvars import ContextVar, Token
from datetime import datetime, timezone
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from observr._transport import Transport

_active_span: ContextVar["Span | None"] = ContextVar("observr_active_span", default=None)


class Span:
    """
    Context manager for manual instrumentation.

    Usage:
        with observr.get_client().span("database.query", table="users") as span:
            rows = db.execute("SELECT ...")
            span.set_attribute("row_count", len(rows))
    """

    def __init__(
        self,
        name: str,
        transport: "Transport",
        attributes: dict[str, Any],
        parent_span_id: str | None = None,
    ) -> None:
        self.name = name
        self.span_id = secrets.token_hex(8)
        self._transport = transport
        self._attributes: dict[str, Any] = dict(attributes)
        self._start: float = 0.0
        self._token: Token | None = None

        active = _active_span.get()
        if parent_span_id is not None:
            # Explicit override: starts a new independent trace
            self.parent_span_id: str | None = parent_span_id
            self.trace_id: str = secrets.token_hex(16)
        elif active is not None:
            # Auto-inherit from enclosing span
            self.parent_span_id = active.span_id
            self.trace_id = active.trace_id
        else:
            # Root span
            self.parent_span_id = None
            self.trace_id = secrets.token_hex(16)

    def set_attribute(self, key: str, value: Any) -> None:
        self._attributes[key] = value

    def __enter__(self) -> "Span":
        self._start = time.monotonic()
        self._token = _active_span.set(self)
        return self

    def __exit__(self, exc_type, exc_val, exc_tb) -> bool:
        _active_span.reset(self._token)
        self._emit(exc_type)
        return False

    async def __aenter__(self) -> "Span":
        return self.__enter__()

    async def __aexit__(self, exc_type, exc_val, exc_tb) -> bool:
        return self.__exit__(exc_type, exc_val, exc_tb)

    def _emit(self, exc_type: type | None) -> None:
        duration_ms = (time.monotonic() - self._start) * 1000

        event: dict[str, Any] = {
            "timestamp": datetime.now(tz=timezone.utc).isoformat(),
            "type": "span",
            "level": "error" if exc_type else "info",
            "trace_id": self.trace_id,
            "span_id": self.span_id,
            "message": self.name,
            "duration_ms": round(duration_ms, 2),
            "attributes": self._attributes,
        }

        if self.parent_span_id is not None:
            event["parent_span_id"] = self.parent_span_id

        if exc_type is not None:
            import traceback
            event["attributes"]["exception"] = traceback.format_exc()

        self._transport.send(event)
