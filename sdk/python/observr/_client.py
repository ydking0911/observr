"""Core ObservrClient — orchestrates transport, logging, and auto-instrumentation."""

from __future__ import annotations

import builtins
import logging
import sys
from typing import TYPE_CHECKING

from observr._transport import Transport
from observr._logger import ObservrLogHandler

if TYPE_CHECKING:
    pass

# Frameworks we watch for lazy instrumentation.
# Key: module top-level name that triggers instrumentation.
# Value: which patcher to call ("flask", "fastapi", "django").
# Note: "starlette" is intentionally omitted — FastAPI imports starlette as a
# dependency before fastapi.__init__ has finished loading.  Triggering on
# "starlette" would try to import fastapi while it is only partially initialised.
_FRAMEWORK_TRIGGERS: dict[str, str] = {
    "flask": "flask",
    "fastapi": "fastapi",
    "django": "django",
}

# Names to mark as patched when fastapi is instrumented (avoids double-patching
# if "starlette" later appears as a separate top-level import).
_FASTAPI_ALIASES = {"fastapi", "starlette"}


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
        self._original_import = None
        self._patched_frameworks: set[str] = set()

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
        self._remove_import_hook()
        logging.getLogger().removeHandler(self._log_handler)
        self._transport.shutdown()
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
        """
        1. Patch any frameworks already present in sys.modules.
        2. Install a builtins.__import__ hook to patch future imports.
        """
        # Patch already-imported frameworks
        if "flask" in sys.modules:
            self._instrument_flask()
            self._patched_frameworks.add("flask")

        if "fastapi" in sys.modules or "starlette" in sys.modules:
            self._instrument_fastapi()
            self._patched_frameworks.update({"fastapi", "starlette"})

        if "django" in sys.modules:
            self._instrument_django()
            self._patched_frameworks.add("django")

        # Install import hook for frameworks not yet imported
        self._install_import_hook()

    def _install_import_hook(self) -> None:
        """Override builtins.__import__ to intercept future framework imports."""
        client = self  # capture for closure
        original = builtins.__import__

        def _hooked_import(name, *args, **kwargs):
            top = name.split(".")[0]
            # Record whether the top-level package was already (even partially)
            # in sys.modules BEFORE this import call.  If it was, this is a
            # circular / internal import inside the package's own __init__ and
            # patching now would see a partially-initialised module.
            was_in_modules = top in sys.modules

            result = original(name, *args, **kwargs)

            # Only trigger on the top-level package import (name == top), not
            # on submodule imports like "fastapi.routing".
            if name != top:
                return result
            patcher = _FRAMEWORK_TRIGGERS.get(top)
            # Skip if: already patched, OR the package was already in sys.modules
            # before this call (internal circular import — module not fully loaded).
            if patcher and top not in client._patched_frameworks and not was_in_modules:
                client._patched_frameworks.add(top)
                if patcher == "flask":
                    client._instrument_flask()
                elif patcher == "fastapi":
                    # Mark starlette alias too so it never re-triggers
                    client._patched_frameworks.update(_FASTAPI_ALIASES)
                    client._instrument_fastapi()
                elif patcher == "django":
                    client._instrument_django()
            return result

        self._original_import = original
        builtins.__import__ = _hooked_import

    def _remove_import_hook(self) -> None:
        if self._original_import is not None:
            builtins.__import__ = self._original_import
            self._original_import = None

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

    def _instrument_django(self) -> None:
        try:
            from observr.integrations.django import instrument_django
            instrument_django(self._transport)
        except Exception as exc:  # noqa: BLE001
            logging.getLogger(__name__).debug("Django instrumentation failed: %s", exc)

    # ------------------------------------------------------------------
    # Manual span API (for advanced use)
    # ------------------------------------------------------------------

    def span(self, name: str, **attributes: object):
        """Context manager for manual spans."""
        from observr._span import Span
        return Span(name=name, transport=self._transport, attributes=attributes)
