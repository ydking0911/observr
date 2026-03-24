"""
Patches Python's standard logging module so that all log records
are forwarded to the observrd collector as structured events.
"""

from __future__ import annotations

import logging
import traceback
from datetime import datetime, timezone
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from observr._transport import Transport


_STDLIB_ATTRS: frozenset[str] = frozenset({
    "name", "msg", "args", "created", "filename", "funcName",
    "levelname", "levelno", "lineno", "module", "msecs",
    "message", "pathname", "process", "processName",
    "relativeCreated", "stack_info", "thread", "threadName",
    "exc_info", "exc_text",
})


class ObservrLogHandler(logging.Handler):
    """Forwards log records to the Transport as structured JSON events."""

    def __init__(self, transport: "Transport") -> None:
        super().__init__()
        self._transport = transport

    def emit(self, record: logging.LogRecord) -> None:
        try:
            event = self._record_to_event(record)
            self._transport.send(event)
        except Exception:  # noqa: BLE001
            self.handleError(record)

    # ------------------------------------------------------------------

    @staticmethod
    def _record_to_event(record: logging.LogRecord) -> dict:
        event: dict = {
            "timestamp": datetime.fromtimestamp(record.created, tz=timezone.utc).isoformat(),
            "type": "log",
            "level": record.levelname.lower(),
            "message": record.getMessage(),
            "logger": record.name,
            "attributes": {
                "module": record.module,
                "funcName": record.funcName,
                "lineno": record.lineno,
                "filename": record.filename,
            },
        }

        # Capture extra fields set by the caller
        for key, value in record.__dict__.items():
            if key not in _STDLIB_ATTRS and not key.startswith("_"):
                event["attributes"][key] = value

        if record.exc_info:
            event["attributes"]["exception"] = "".join(
                traceback.format_exception(*record.exc_info)
            )

        return event
