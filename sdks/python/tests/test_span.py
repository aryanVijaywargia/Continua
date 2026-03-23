"""Tests for span context and semantic event helpers."""

from __future__ import annotations

import threading
import time
from contextlib import contextmanager
from typing import Callable, Iterator

import pytest

from tests.support import assert_event_metadata

from continua.client import Continua
from continua.span import (
    SpanContext,
    _MAX_EVENT_SEQUENCE,
    get_current_span,
    span,
)
from continua.trace import TraceContext


class StubClient:
    def __init__(self) -> None:
        self._lock = threading.Lock()
        self.traces: list[dict] = []
        self.spans: list[dict] = []
        self.events: list[dict] = []

    def add_trace(self, trace: dict) -> None:
        with self._lock:
            self.traces.append(trace)

    def add_span(self, span_data: dict) -> None:
        with self._lock:
            self.spans.append(span_data)

    def add_event(self, event: dict) -> None:
        with self._lock:
            self.events.append(event)


class SlowStubClient(StubClient):
    def __init__(self, *, delay_s: float = 0.01) -> None:
        super().__init__()
        self._delay_s = delay_s

    def add_event(self, event: dict) -> None:
        time.sleep(self._delay_s)
        super().add_event(event)


@contextmanager
def use_stub_client(stub_client: StubClient) -> Iterator[None]:
    previous_client = Continua._instance
    Continua._instance = stub_client
    try:
        yield
    finally:
        Continua._instance = previous_client


@contextmanager
def without_client() -> Iterator[None]:
    previous_client = Continua._instance
    Continua._instance = None
    try:
        yield
    finally:
        Continua._instance = previous_client


@contextmanager
def traced_span(
    stub_client: StubClient,
    *,
    trace_name: str = "test_trace",
    span_name: str = "test_span",
    kind: str = "default",
) -> Iterator[SpanContext]:
    with use_stub_client(stub_client):
        with TraceContext(name=trace_name):
            with span(span_name, kind=kind) as ctx:
                yield ctx


def run_concurrent_workers(
    worker_count: int,
    action: Callable[[int], None],
) -> list[BaseException]:
    barrier = threading.Barrier(worker_count)
    failures: list[BaseException] = []
    failures_lock = threading.Lock()

    def worker(index: int) -> None:
        try:
            barrier.wait()
            action(index)
        except BaseException as exc:  # pragma: no cover - only for debugging failures
            with failures_lock:
                failures.append(exc)

    threads = [
        threading.Thread(target=worker, args=(index,))
        for index in range(worker_count)
    ]
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join()

    return failures


def test_span_context_basic():
    """Test basic span context."""
    with TraceContext(name="test_trace"):
        with SpanContext(name="test_span", kind="llm") as s:
            assert s.name == "test_span"
            assert s.kind == "llm"
            assert s.status == "running"
            assert get_current_span() is s

        assert s.status == "completed"
        assert s.end_time is not None


def test_span_context_nesting():
    """Test nested span contexts."""
    with TraceContext(name="test_trace"):
        with SpanContext(name="parent") as parent:
            with SpanContext(name="child") as child:
                assert child.parent_span_id == parent.span_id
                assert get_current_span() is child
            assert get_current_span() is parent
        assert get_current_span() is None


def test_span_context_with_metrics():
    """Test span context with metrics."""
    with TraceContext(name="test_trace"):
        with span("llm_call", kind="llm") as s:
            s.set_input({"prompt": "test"})
            s.set_output({"response": "result"})
            s.set_model("gpt-4", provider="openai")
            s.set_tokens(prompt=100, completion=50)
            s.set_cost(0.01)
            s.set_metadata("custom", "value")

    assert s._input == {"prompt": "test"}
    assert s._output == {"response": "result"}
    assert s._model == "gpt-4"
    assert s._provider == "openai"
    assert s._prompt_tokens == 100
    assert s._completion_tokens == 50
    assert s._total_tokens == 150
    assert s._total_cost == 0.01
    assert s.metadata == {"custom": "value"}


def test_span_context_on_exception():
    """Test span context marks as failed on exception."""
    with TraceContext(name="test_trace"):
        try:
            with span("failing_span") as s:
                raise ValueError("Test error")
        except ValueError:
            pass

    assert s.status == "failed"
    assert "Test error" in s.status_message


def test_set_tokens_total_only_raises():
    """Test total-only token usage is rejected."""
    with TraceContext(name="test_trace"):
        with span("llm_call", kind="llm") as s:
            with pytest.raises(ValueError):
                s.set_tokens(total=150)


def test_span_without_trace():
    """Test span without active trace."""
    with span("orphan_span") as s:
        assert s.trace_id is None


def test_span_helper_function():
    """Test the span() helper function."""
    with TraceContext(name="test"):
        with span("test_span", kind="tool", metadata={"key": "value"}) as s:
            assert s.name == "test_span"
            assert s.kind == "tool"
            assert s.metadata == {"key": "value"}


def test_explicit_events_receive_monotonic_sequence_and_timestamps():
    """Explicit helpers share one per-span sequence and emit event metadata."""
    stub_client = StubClient()

    with traced_span(stub_client, span_name="eventful_span") as s:
        s.log("started")
        s.metric("latency_ms", 42.5, "ms")
        s.state_change("status", "pending", "approved")

    assert [event["event_type"] for event in stub_client.events] == [
        "log",
        "metric",
        "state_change",
    ]
    for sequence, event in enumerate(stub_client.events, start=1):
        assert_event_metadata(event, sequence=sequence)


def test_sequence_counter_is_per_span():
    """Each span maintains its own event sequence counter."""
    stub_client = StubClient()

    with use_stub_client(stub_client):
        with TraceContext(name="test_trace"):
            with span("first_span") as first:
                first.log("one")
                first.metric("count", 1)
            with span("second_span") as second:
                second.log("alpha")
                second.decision("route?", "fast")

    first_sequences = [
        event["sequence"]
        for event in stub_client.events
        if event["span_id"] == first.span_id
    ]
    second_sequences = [
        event["sequence"]
        for event in stub_client.events
        if event["span_id"] == second.span_id
    ]

    assert first_sequences == [1, 2]
    assert second_sequences == [1, 2]


def test_sequence_overflow_raises_at_int32_boundary():
    """Sequence assignment fails loudly when the int32 boundary is exceeded."""
    stub_client = StubClient()

    with traced_span(stub_client, span_name="overflow_span") as s:
        s._event_seq = _MAX_EVENT_SEQUENCE - 1

        s.log("last allowed event")
        assert_event_metadata(stub_client.events[0], sequence=_MAX_EVENT_SEQUENCE)

        with pytest.raises(OverflowError, match="int32 maximum"):
            s.log("overflow")


def test_early_return_paths_do_not_consume_sequence_numbers():
    """No trace or no client should not increment the per-span counter."""
    orphan = SpanContext("orphan")
    with without_client():
        orphan.log("ignored")
    assert orphan._event_seq == 0

    stub_client = StubClient()
    with use_stub_client(stub_client):
        orphan.trace_id = "trace-1"
        orphan.log("sent")

    assert orphan._event_seq == 1
    assert_event_metadata(stub_client.events[0], sequence=1)

    late_client = SpanContext("late_client")
    late_client.trace_id = "trace-2"
    with without_client():
        late_client.log("ignored without client")
    assert late_client._event_seq == 0

    with use_stub_client(stub_client):
        late_client.log("sent after client init")

    assert late_client._event_seq == 1
    assert_event_metadata(stub_client.events[1], sequence=1)


def test_concurrent_event_emission_produces_contiguous_sequences():
    """Concurrent emission should assign duplicate-free contiguous sequences."""
    stub_client = StubClient()
    worker_count = 24

    with traced_span(stub_client, span_name="parallel_span") as s:
        failures = run_concurrent_workers(
            worker_count,
            lambda index: s.log(f"worker-{index}"),
        )

    assert failures == []
    sequences = sorted(event["sequence"] for event in stub_client.events)
    assert sequences == list(range(1, worker_count + 1))


def test_state_change_helper_records_semantic_event():
    """Test state_change helper payload construction."""
    stub_client = StubClient()

    with traced_span(stub_client, span_name="stateful_span") as s:
        s.state_change(
            "status",
            "pending",
            "approved",
            namespace="order",
            message="Order approved",
        )

    assert len(stub_client.events) == 1
    event = stub_client.events[0]
    assert_event_metadata(event, sequence=1)
    assert event["trace_id"] == s.trace_id
    assert event["span_id"] == s.span_id
    assert event["event_type"] == "state_change"
    assert event["level"] == "info"
    assert event["message"] == "Order approved"
    assert event["payload"] == {
        "key": "status",
        "old_value": "pending",
        "new_value": "approved",
        "namespace": "order",
    }


def test_decision_helper_records_semantic_event():
    """Test decision helper payload construction."""
    stub_client = StubClient()

    with traced_span(stub_client, span_name="decision_span") as s:
        s.decision(
            "Which model?",
            "gpt-4.1",
            alternatives=["gpt-4o-mini", "gpt-4.1"],
            reasoning="Need better reasoning quality",
            message="Escalated to stronger model",
        )

    assert len(stub_client.events) == 1
    event = stub_client.events[0]
    assert_event_metadata(event, sequence=1)
    assert event["trace_id"] == s.trace_id
    assert event["span_id"] == s.span_id
    assert event["event_type"] == "decision"
    assert event["level"] == "info"
    assert event["message"] == "Escalated to stronger model"
    assert event["payload"] == {
        "question": "Which model?",
        "chosen": "gpt-4.1",
        "alternatives": ["gpt-4o-mini", "gpt-4.1"],
        "reasoning": "Need better reasoning quality",
    }


def test_effect_helper_records_reserved_merge_semantics():
    """Effect helper should merge payloads without mutating caller input."""
    stub_client = StubClient()
    caller_payload = {
        "url": "https://example.com",
        "effect_kind": "wrong",
        "has_external_side_effect": True,
        "effect_id": "caller-effect",
        "idempotent": True,
        "idempotency_key": "caller-key",
    }
    original_payload = dict(caller_payload)

    with traced_span(stub_client, span_name="effect_span") as s:
        s.effect(
            "api_call",
            has_external_side_effect=False,
            effect_id=None,
            idempotent=False,
            idempotency_key="operation-123",
            payload=caller_payload,
        )

    assert caller_payload == original_payload
    event = stub_client.events[0]
    assert_event_metadata(event, sequence=1)
    assert event["event_type"] == "effect"
    assert event["level"] == "info"
    assert event["message"] == "Effect: api call"
    assert "idempotency_key" not in event
    assert event["payload"] == {
        "url": "https://example.com",
        "effect_kind": "api_call",
        "has_external_side_effect": False,
        "idempotent": False,
        "idempotency_key": "operation-123",
    }


def test_effect_helper_omits_empty_string_identifiers():
    """Empty string effect identifiers should be treated as absent."""
    stub_client = StubClient()

    with traced_span(stub_client, span_name="effect_span") as s:
        s.effect(
            "api_call",
            has_external_side_effect=True,
            effect_id="",
            idempotency_key="",
        )

    event = stub_client.events[0]
    assert_event_metadata(event, sequence=1)
    assert event["payload"] == {
        "effect_kind": "api_call",
        "has_external_side_effect": True,
    }


def test_effect_helper_rejects_empty_kind():
    """Effect helper rejects empty effect kinds."""
    with pytest.raises(ValueError, match="kind must be a non-empty string"):
        SpanContext("effect_span").effect("", has_external_side_effect=True)


def test_wait_helper_records_reserved_merge_semantics():
    """Wait helper should merge payloads with helper-owned field precedence."""
    stub_client = StubClient()
    caller_payload = {
        "service": "stripe",
        "phase": "wrong",
        "wait_kind": "wrong",
        "wait_id": "caller-wait",
        "resolution": "wrong",
    }
    original_payload = dict(caller_payload)

    with traced_span(stub_client, span_name="wait_span") as s:
        s.wait(
            "human_approval",
            phase="resolved",
            resolution="approved",
            wait_id="wait-abc",
            payload=caller_payload,
        )

    assert caller_payload == original_payload
    event = stub_client.events[0]
    assert_event_metadata(event, sequence=1)
    assert event["event_type"] == "wait"
    assert event["level"] == "info"
    assert event["message"] == "Resolved wait: human approval"
    assert event["payload"] == {
        "service": "stripe",
        "wait_kind": "human_approval",
        "phase": "resolved",
        "wait_id": "wait-abc",
        "resolution": "approved",
    }


def test_wait_helper_omits_empty_wait_id():
    """Empty string wait identifiers should be treated as absent."""
    stub_client = StubClient()

    with traced_span(stub_client, span_name="wait_span") as s:
        s.wait("timer", phase="entered", wait_id="")

    event = stub_client.events[0]
    assert_event_metadata(event, sequence=1)
    assert event["message"] == "Entered wait: timer"
    assert event["payload"] == {
        "wait_kind": "timer",
        "phase": "entered",
    }


def test_wait_helper_rejects_empty_kind_or_phase():
    """Wait helper rejects empty semantic identifiers."""
    span_context = SpanContext("wait_span")

    with pytest.raises(ValueError, match="kind must be a non-empty string"):
        span_context.wait("", phase="entered")
    with pytest.raises(ValueError, match="phase must be a non-empty string"):
        span_context.wait("human_approval", phase="")


def test_set_llm_response_emits_one_implicit_effect():
    """First set_llm_response call emits an implicit effect exactly once."""
    stub_client = StubClient()
    messages = [{"role": "user", "content": "Hello"}]

    with traced_span(stub_client, span_name="llm_span", kind="llm") as s:
        s.set_llm_response("gpt-4", messages, {"content": "Hi"}, 10, 5)
        s.set_llm_response("gpt-4", messages, {"content": "Still hi"}, 11, 6)

    assert len(stub_client.events) == 1
    event = stub_client.events[0]
    assert_event_metadata(event, sequence=1)
    assert event["event_type"] == "effect"
    assert event["message"] == "Model call: gpt-4"
    assert event["payload"] == {
        "effect_kind": "model_call",
        "has_external_side_effect": False,
    }


def test_set_llm_response_emit_effect_false_does_not_consume_flag():
    """Suppressing implicit LLM emission should not block a later default call."""
    stub_client = StubClient()
    messages = [{"role": "user", "content": "Hello"}]

    with traced_span(stub_client, span_name="llm_span", kind="llm") as s:
        s.set_llm_response(
            "gpt-4",
            messages,
            {"content": "Hi"},
            10,
            5,
            emit_effect=False,
        )
        s.set_llm_response("gpt-4", messages, {"content": "Hi again"}, 10, 5)

    assert len(stub_client.events) == 1
    event = stub_client.events[0]
    assert_event_metadata(event, sequence=1)
    assert event["message"] == "Model call: gpt-4"


def test_concurrent_set_llm_response_emits_one_implicit_effect():
    """Concurrent LLM setter calls should still emit a single implicit effect."""
    stub_client = SlowStubClient()
    worker_count = 8
    messages = [{"role": "user", "content": "Hello"}]

    with traced_span(stub_client, span_name="llm_span", kind="llm") as s:
        failures = run_concurrent_workers(
            worker_count,
            lambda index: s.set_llm_response(
                "gpt-4",
                messages,
                {"content": f"Hi {index}"},
                10,
                5,
            ),
        )

    assert failures == []
    effect_events = [event for event in stub_client.events if event["event_type"] == "effect"]
    assert len(effect_events) == 1
    assert_event_metadata(effect_events[0], sequence=1)
    assert effect_events[0]["message"] == "Model call: gpt-4"
    assert effect_events[0]["payload"] == {
        "effect_kind": "model_call",
        "has_external_side_effect": False,
    }


def test_set_tool_call_emits_one_implicit_effect_with_override():
    """Tool helper should emit a single implicit effect with override support."""
    stub_client = StubClient()

    with traced_span(stub_client, span_name="tool_span", kind="tool") as s:
        s.set_tool_call(
            "cache_lookup",
            {"key": "weather"},
            {"hit": True},
            has_external_side_effect=False,
        )
        s.set_tool_call(
            "cache_lookup",
            {"key": "weather"},
            {"hit": True},
            has_external_side_effect=True,
        )

    assert len(stub_client.events) == 1
    event = stub_client.events[0]
    assert_event_metadata(event, sequence=1)
    assert event["message"] == "Tool call: cache_lookup"
    assert event["payload"] == {
        "effect_kind": "tool_call",
        "has_external_side_effect": False,
    }


def test_set_tool_call_emit_effect_false_does_not_consume_flag():
    """Suppressing implicit tool emission should not block a later default call."""
    stub_client = StubClient()

    with traced_span(stub_client, span_name="tool_span", kind="tool") as s:
        s.set_tool_call(
            "search",
            {"q": "test"},
            {"results": []},
            emit_effect=False,
        )
        s.set_tool_call(
            "search",
            {"q": "test"},
            {"results": []},
            has_external_side_effect=True,
        )

    assert len(stub_client.events) == 1
    event = stub_client.events[0]
    assert_event_metadata(event, sequence=1)
    assert event["message"] == "Tool call: search"
    assert event["payload"] == {
        "effect_kind": "tool_call",
        "has_external_side_effect": True,
    }


def test_concurrent_set_tool_call_emits_one_implicit_effect():
    """Concurrent tool setter calls should still emit a single implicit effect."""
    stub_client = SlowStubClient()
    worker_count = 8

    with traced_span(stub_client, span_name="tool_span", kind="tool") as s:
        failures = run_concurrent_workers(
            worker_count,
            lambda index: s.set_tool_call(
                "search",
                {"q": f"test-{index}"},
                {"results": []},
            ),
        )

    assert failures == []
    effect_events = [event for event in stub_client.events if event["event_type"] == "effect"]
    assert len(effect_events) == 1
    assert_event_metadata(effect_events[0], sequence=1)
    assert effect_events[0]["message"] == "Tool call: search"
    assert effect_events[0]["payload"] == {
        "effect_kind": "tool_call",
        "has_external_side_effect": True,
    }


def test_explicit_helpers_do_not_interfere_with_implicit_effect_flags():
    """Explicit helpers should neither consume nor block implicit effect emission."""
    stub_client = StubClient()
    messages = [{"role": "user", "content": "Hello"}]

    with traced_span(stub_client, span_name="llm_span", kind="llm") as s:
        s.set_llm_response(
            "gpt-4",
            messages,
            {"content": "Hi"},
            10,
            5,
            emit_effect=False,
        )
        s.effect("model_call", has_external_side_effect=False)
        s.wait("external", phase="entered")
        s.set_llm_response("gpt-4", messages, {"content": "Hi again"}, 10, 5)

    assert [event["event_type"] for event in stub_client.events] == [
        "effect",
        "wait",
        "effect",
    ]
    assert [event["message"] for event in stub_client.events] == [
        "Effect: model call",
        "Entered wait: external",
        "Model call: gpt-4",
    ]
    for sequence, event in enumerate(stub_client.events, start=1):
        assert_event_metadata(event, sequence=sequence)


def test_effect_and_wait_are_quiet_no_ops_without_trace_context():
    """Effect and wait helpers should do nothing when no trace is active."""
    stub_client = StubClient()
    orphan = SpanContext("orphan")

    with use_stub_client(stub_client):
        orphan.effect("api_call", has_external_side_effect=True)
        orphan.wait("human_approval", phase="entered")

    assert stub_client.events == []
    assert orphan._event_seq == 0


def test_effect_helpers_are_quiet_no_ops_without_initialized_client():
    """Tracing helpers should stay quiet when no client is initialized."""
    with without_client():
        with TraceContext(name="inactive_trace"):
            with span("inactive_span", kind="llm") as s:
                s.set_llm_response("gpt-4", [{"role": "user", "content": "Hello"}], {"ok": True}, 3, 2)
                s.set_tool_call("search", {"q": "test"}, {"results": []})
                s.effect("api_call", has_external_side_effect=True)
                s.wait("human_approval", phase="entered")

    assert s._model == "gpt-4"
    assert s.name == "search"
    assert s._input == {"q": "test"}
    assert s._output == {"results": []}
    assert s._event_seq == 0
    assert s._implicit_llm_effect_emitted is False
    assert s._implicit_tool_effect_emitted is False
