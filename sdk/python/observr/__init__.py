"""
observr — Zero-config observability for AI-agent-friendly local tracing.

Usage:
    import observr
    observr.init(service="my-api")
"""

from observr._client import ObservrClient
from observr._version import __version__

_client: ObservrClient | None = None


def init(
    service: str = "app",
    collector_url: str = "http://localhost:7676",
    *,
    auto_instrument: bool = True,
    log_level: str = "DEBUG",
) -> ObservrClient:
    """
    Initialize observr. Call this once at application startup.

    Args:
        service:         Name of your service (shown in dashboard).
        collector_url:   URL of the observrd collector. Defaults to localhost.
        auto_instrument: Automatically patch Flask / FastAPI / logging.
        log_level:       Minimum log level to capture.

    Returns:
        The active ObservrClient instance.
    """
    global _client
    _client = ObservrClient(
        service=service,
        collector_url=collector_url,
        auto_instrument=auto_instrument,
        log_level=log_level,
    )
    _client.start()
    return _client


def get_client() -> ObservrClient:
    """Return the active client. Raises if init() has not been called."""
    if _client is None:
        raise RuntimeError("observr.init() must be called before get_client()")
    return _client


__all__ = ["init", "get_client", "__version__", "ObservrClient"]
