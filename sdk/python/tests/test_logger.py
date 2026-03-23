"""Unit tests for the log interceptor."""

import logging
from unittest.mock import MagicMock

from observr._logger import ObservrLogHandler


def test_log_record_forwarded_as_event():
    transport = MagicMock()
    handler = ObservrLogHandler(transport=transport)

    logger = logging.getLogger("test.observr")
    logger.addHandler(handler)
    logger.setLevel(logging.DEBUG)

    logger.error("Payment failed", extra={"user_id": "u_123"})

    transport.send.assert_called_once()
    event = transport.send.call_args[0][0]
    assert event["type"] == "log"
    assert event["level"] == "error"
    assert event["message"] == "Payment failed"
    assert event["attributes"]["user_id"] == "u_123"


def test_exception_is_captured():
    transport = MagicMock()
    handler = ObservrLogHandler(transport=transport)

    logger = logging.getLogger("test.observr.exc")
    logger.addHandler(handler)
    logger.setLevel(logging.DEBUG)

    try:
        raise ValueError("boom")
    except ValueError:
        logger.exception("Something went wrong")

    event = transport.send.call_args[0][0]
    assert "exception" in event["attributes"]
    assert "ValueError" in event["attributes"]["exception"]
