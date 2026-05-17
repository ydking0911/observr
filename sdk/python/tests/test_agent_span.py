"""Tests for agent_span() — standard agent observability attribute helper."""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from observr._span import _active_span


@pytest.fixture(autouse=True)
def reset_context():
    token = _active_span.set(None)
    yield
    _active_span.reset(token)


@pytest.fixture()
def client():
    from observr._client import ObservrClient
    c = ObservrClient(
        service="test",
        collector_url="http://localhost:7676",
        auto_instrument=False,
        log_level="debug",
    )
    c._transport = MagicMock()
    c._transport.send = MagicMock()
    return c


def emitted(client) -> dict:
    return client._transport.send.call_args[0][0]


# ── Standard attributes ───────────────────────────────────────────────────────

def test_agent_span_sets_intent(client):
    with client.agent_span("decide", intent="summarize document"):
        pass
    assert emitted(client)["attributes"]["agent.intent"] == "summarize document"


def test_agent_span_sets_trigger(client):
    with client.agent_span("decide", trigger="user_message"):
        pass
    assert emitted(client)["attributes"]["agent.trigger"] == "user_message"


def test_agent_span_sets_model(client):
    with client.agent_span("decide", model="claude-sonnet-4-6"):
        pass
    assert emitted(client)["attributes"]["agent.model"] == "claude-sonnet-4-6"


def test_agent_span_sets_tool(client):
    with client.agent_span("tool.call", tool="web_search"):
        pass
    assert emitted(client)["attributes"]["agent.tool"] == "web_search"


def test_agent_span_all_attributes(client):
    with client.agent_span(
        "tool.call",
        intent="find papers",
        trigger="user_message",
        model="claude-sonnet-4-6",
        tool="web_search",
    ):
        pass
    attrs = emitted(client)["attributes"]
    assert attrs["agent.intent"] == "find papers"
    assert attrs["agent.trigger"] == "user_message"
    assert attrs["agent.model"] == "claude-sonnet-4-6"
    assert attrs["agent.tool"] == "web_search"


def test_agent_span_omits_none_attributes(client):
    with client.agent_span("decide", intent="summarize"):
        pass
    attrs = emitted(client)["attributes"]
    assert "agent.trigger" not in attrs
    assert "agent.model" not in attrs
    assert "agent.tool" not in attrs


# ── Extra attributes pass through ────────────────────────────────────────────

def test_agent_span_forwards_extra_kwargs(client):
    with client.agent_span("decide", intent="summarize", result_count=5):
        pass
    attrs = emitted(client)["attributes"]
    assert attrs["result_count"] == 5
    assert attrs["agent.intent"] == "summarize"


# ── Context propagation works the same as span() ─────────────────────────────

def test_agent_span_inherits_parent_context(client):
    with client.span("outer") as outer:
        with client.agent_span("inner", intent="sub-task") as inner:
            assert inner.parent_span_id == outer.span_id
            assert inner.trace_id == outer.trace_id


def test_agent_span_explicit_parent_id(client):
    with client.agent_span("child", intent="sub-task", parent_span_id="explicit-id") as span:
        assert span.parent_span_id == "explicit-id"


def test_agent_span_emits_span_type(client):
    with client.agent_span("decide", intent="summarize"):
        pass
    assert emitted(client)["type"] == "span"
    assert emitted(client)["message"] == "decide"
