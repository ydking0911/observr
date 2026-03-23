"""
HTTP transport — batches events and POSTs them to the observrd collector.
Non-blocking: events are queued and sent in a background thread.
"""

from __future__ import annotations

import json
import logging
import queue
import threading
import time
import urllib.error
import urllib.request
from typing import Any

logger = logging.getLogger(__name__)

_BATCH_SIZE = 50
_FLUSH_INTERVAL = 1.0  # seconds


class Transport:
    """Thread-safe, non-blocking event transport."""

    def __init__(self, collector_url: str, service: str) -> None:
        self.collector_url = collector_url.rstrip("/")
        self.service = service
        self._queue: queue.Queue[dict[str, Any]] = queue.Queue(maxsize=10_000)
        self._stop_event = threading.Event()
        self._thread = threading.Thread(target=self._worker, daemon=True, name="observr-transport")
        self._thread.start()

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def send(self, event: dict[str, Any]) -> None:
        """Enqueue a single event. Never blocks, never raises."""
        event.setdefault("service", self.service)
        try:
            self._queue.put_nowait(event)
        except queue.Full:
            pass  # drop silently — observability must not affect app stability

    def flush(self, timeout: float = 5.0) -> None:
        """Block until queue is empty or timeout expires."""
        deadline = time.monotonic() + timeout
        while not self._queue.empty() and time.monotonic() < deadline:
            time.sleep(0.05)

    def shutdown(self) -> None:
        self.flush()
        self._stop_event.set()
        self._thread.join(timeout=3.0)

    # ------------------------------------------------------------------
    # Background worker
    # ------------------------------------------------------------------

    def _worker(self) -> None:
        while not self._stop_event.is_set():
            batch: list[dict[str, Any]] = []
            deadline = time.monotonic() + _FLUSH_INTERVAL

            while time.monotonic() < deadline and len(batch) < _BATCH_SIZE:
                try:
                    event = self._queue.get(timeout=0.1)
                    batch.append(event)
                except queue.Empty:
                    break

            if batch:
                self._post_batch(batch)

    def _post_batch(self, batch: list[dict[str, Any]]) -> None:
        url = f"{self.collector_url}/events"
        payload = json.dumps({"events": batch}).encode()
        req = urllib.request.Request(
            url,
            data=payload,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, timeout=3):
                pass
        except (urllib.error.URLError, OSError):
            # Collector not running — silently discard
            pass
