"""Core ObservrClient — orchestrates transport, logging, and auto-instrumentation."""

from __future__ import annotations

import logging
import sys
from typing import TYPE_CHECKING

from observr._transport import Transport
from observr._logger import ObservrLogHandler

if TYPE_CHECKING:
    pass


class ObservrClient:
    """
    Central client that wires together transport, log capture,
    and framework auto-instrumentation.
    """

    def __init__(
        self,
        service: str,
        collector_url: str,
        auto_instrument: bool,
        log_level: str,
    ) -> None:
        self.service = service
        self.collector_url = collector_url
        self.auto_instrument = auto_instrument
        self.log_level = log_level

        self._transport = Transport(collector_url=collector_url, service=service)
        self._log_handler = ObservrLogHandler(transport=self._transport)
        self._started = False

    # ------------------------------------------------------------------
    # Lifecycle
    # ------------------------------------------------------------------

    def start(self) -> None:
        if self._started:
            return
        self._started = True

        self._setup_logging()

        if self.auto_instrument:
            self._auto_instrument()

    def shutdown(self) -> None:
        self._transport.flush()
        self._started = False

    # ------------------------------------------------------------------
    # Logging
    # ------------------------------------------------------------------

    def _setup_logging(self) -> None:
        level = getattr(logging, self.log_level.upper(), logging.DEBUG)
        self._log_handler.setLevel(level)

        root_logger = logging.getLogger()
        root_logger.addHandler(self._log_handler)
        if root_logger.level == logging.NOTSET:
            root_logger.setLevel(level)

    # ------------------------------------------------------------------
    # Auto-instrumentation
    # ------------------------------------------------------------------

    def _auto_instrument(self) -> None:
        """Detect and patch installed web frameworks."""
        if "flask" in sys.modules:
            self._instrument_flask()

        if "fastapi" in sys.modules or "starlette" in sys.modules:
            self._instrument_fastapi()

    def _instrument_flask(self) -> None:
        try:
            from observr.integrations.flask import instrument_flask
            instrument_flask(self._transport)
        except Exception as exc:  # noqa: BLE001
            logging.getLogger(__name__).debug("Flask instrumentation failed: %s", exc)

    def _instrument_fastapi(self) -> None:
        try:
            from observr.integrations.fastapi import instrument_fastapi
            instrument_fastapi(self._transport)
        except Exception as exc:  # noqa: BLE001
            logging.getLogger(__name__).debug("FastAPI instrumentation failed: %s", exc)

    # ------------------------------------------------------------------
    # Manual span API (for advanced use)
    # ------------------------------------------------------------------

    def span(self, name: str, **attributes: object):
        """Context manager for manual spans."""
        from observr._span import Span
        return Span(name=name, transport=self._transport, attributes=attributes)
