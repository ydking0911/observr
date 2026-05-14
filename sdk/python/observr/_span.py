"""Manual span context manager for custom tracing."""

from __future__ import annotations

import secrets
import time
from datetime import datetime, timezone
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from observr._transport import Transport


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
        self.trace_id = secrets.token_hex(16)
        self.parent_span_id = parent_span_id
        self._transport = transport
        self._attributes: dict[str, Any] = dict(attributes)
        self._start: float = 0.0
        self._error: Exception | None = None

    def set_attribute(self, key: str, value: Any) -> None:
        self._attributes[key] = value

    def __enter__(self) -> "Span":
        self._start = time.monotonic()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb) -> bool:
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
        return False  # don't suppress exceptions
