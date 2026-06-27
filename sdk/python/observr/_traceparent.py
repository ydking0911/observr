"""W3C traceparent header parsing and formatting (RFC 0002 version 00)."""
from __future__ import annotations

_LOWER_HEX = frozenset("0123456789abcdef")


def parse_traceparent(header: str) -> tuple[str, str] | None:
    """Parse a W3C traceparent header.

    Returns (trace_id, parent_id) on success, None for any invalid input.
    The returned parent_id becomes parent_span_id for the new span so the
    causal chain links correctly across service boundaries.
    """
    parts = header.split("-")
    if len(parts) != 4 or parts[0] != "00":
        return None
    trace_id, parent_id, flags = parts[1], parts[2], parts[3]
    if len(trace_id) != 32 or len(parent_id) != 16 or len(flags) != 2:
        return None
    if not all(c in _LOWER_HEX for c in trace_id + parent_id + flags):
        return None
    if all(c == "0" for c in trace_id) or all(c == "0" for c in parent_id):
        return None
    return trace_id, parent_id


def format_traceparent(trace_id: str, span_id: str) -> str:
    """Format a W3C traceparent header value."""
    return f"00-{trace_id}-{span_id}-01"
