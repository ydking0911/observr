"""Tests for automatic span context propagation (ContextVar-based)."""

from __future__ import annotations

import asyncio
from unittest.mock import MagicMock

import pytest

from observr._span import Span, _active_span


@pytest.fixture(autouse=True)
def reset_context():
    """Ensure _active_span is None before and after every test."""
    token = _active_span.set(None)
    yield
    _active_span.reset(token)


@pytest.fixture()
def transport():
    t = MagicMock()
    t.send = MagicMock()
    return t


# ── Root span ─────────────────────────────────────────────────────────────────

def test_root_span_has_no_parent(transport):
    with Span("root", transport, {}) as s:
        assert s.parent_span_id is None
        assert s.trace_id is not None
    event = transport.send.call_args[0][0]
    assert "parent_span_id" not in event


# ── 2-level nesting ───────────────────────────────────────────────────────────

def test_two_level_nesting(transport):
    with Span("outer", transport, {}) as outer:
        with Span("inner", transport, {}) as inner:
            assert inner.parent_span_id == outer.span_id
            assert inner.trace_id == outer.trace_id


# ── 3-level nesting ───────────────────────────────────────────────────────────

def test_three_level_nesting(transport):
    with Span("a", transport, {}) as a:
        with Span("b", transport, {}) as b:
            with Span("c", transport, {}) as c:
                assert b.parent_span_id == a.span_id
                assert c.parent_span_id == b.span_id
                assert a.trace_id == b.trace_id == c.trace_id


# ── Context restoration ───────────────────────────────────────────────────────

def test_context_restored_after_exit(transport):
    with Span("outer", transport, {}) as outer:
        with Span("inner", transport, {}):
            pass
        # After inner exits, active span should be back to outer
        sibling = Span("sibling", transport, {})
        assert sibling.parent_span_id == outer.span_id
        assert sibling.trace_id == outer.trace_id


def test_context_none_after_root_exits(transport):
    with Span("root", transport, {}):
        pass
    # After root exits, no active span
    standalone = Span("standalone", transport, {})
    assert standalone.parent_span_id is None


# ── Explicit override ──────────────────────────────────────────────────────────

def test_explicit_override_uses_parent_id_and_inherits_trace(transport):
    with Span("outer", transport, {}) as outer:
        child = Span("child", transport, {}, parent_span_id="explicit-parent-id")
        assert child.parent_span_id == "explicit-parent-id"
        # Explicit override inside an active context inherits that context's trace_id
        assert child.trace_id == outer.trace_id


def test_explicit_override_without_context_generates_new_trace(transport):
    child = Span("child", transport, {}, parent_span_id="explicit-parent-id")
    assert child.parent_span_id == "explicit-parent-id"
    assert child.trace_id is not None


# ── Async context manager ─────────────────────────────────────────────────────

async def test_async_two_level_nesting(transport):
    async with Span("async-outer", transport, {}) as outer:
        async with Span("async-inner", transport, {}) as inner:
            assert inner.parent_span_id == outer.span_id
            assert inner.trace_id == outer.trace_id


async def test_async_context_restored_after_exit(transport):
    async with Span("outer", transport, {}) as outer:
        async with Span("inner", transport, {}):
            pass
        sibling = Span("sibling", transport, {})
        assert sibling.parent_span_id == outer.span_id


# ── Parallel async isolation ──────────────────────────────────────────────────

async def test_async_parallel_isolation(transport):
    """Two concurrent asyncio Tasks must not pollute each other's context."""
    results: dict[str, str | None] = {}

    async def task_a():
        async with Span("task-a-outer", transport, {}) as outer:
            await asyncio.sleep(0)  # yield to event loop
            inner = Span("task-a-inner", transport, {})
            results["a_inner_parent"] = inner.parent_span_id
            results["a_outer_id"] = outer.span_id

    async def task_b():
        # task_b runs with no active span context of its own
        await asyncio.sleep(0)
        standalone = Span("task-b-span", transport, {})
        results["b_parent"] = standalone.parent_span_id

    await asyncio.gather(task_a(), task_b())

    assert results["a_inner_parent"] == results["a_outer_id"]
    assert results["b_parent"] is None
