"""Tests for span context."""

from continua.client import Continua
from continua.span import SpanContext, get_current_span, span
from continua.trace import TraceContext


class StubClient:
    def __init__(self) -> None:
        self.traces: list[dict] = []
        self.spans: list[dict] = []
        self.events: list[dict] = []

    def add_trace(self, trace: dict) -> None:
        self.traces.append(trace)

    def add_span(self, span: dict) -> None:
        self.spans.append(span)

    def add_event(self, event: dict) -> None:
        self.events.append(event)


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
            try:
                s.set_tokens(total=150)
                raised = False
            except ValueError:
                raised = True
    assert raised is True


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


def test_state_change_helper_records_semantic_event():
    """Test state_change helper payload construction."""
    stub_client = StubClient()
    previous_client = Continua._instance
    Continua._instance = stub_client

    try:
        with TraceContext(name="test_trace"):
            with span("stateful_span") as s:
                s.state_change(
                    "status",
                    "pending",
                    "approved",
                    namespace="order",
                    message="Order approved",
                )
    finally:
        Continua._instance = previous_client

    assert len(stub_client.events) == 1
    assert stub_client.events[0] == {
        "trace_id": stub_client.traces[0]["trace_id"],
        "span_id": stub_client.spans[0]["span_id"],
        "event_type": "state_change",
        "level": "info",
        "message": "Order approved",
        "payload": {
            "key": "status",
            "old_value": "pending",
            "new_value": "approved",
            "namespace": "order",
        },
    }


def test_decision_helper_records_semantic_event():
    """Test decision helper payload construction."""
    stub_client = StubClient()
    previous_client = Continua._instance
    Continua._instance = stub_client

    try:
        with TraceContext(name="test_trace"):
            with span("decision_span") as s:
                s.decision(
                    "Which model?",
                    "gpt-4.1",
                    alternatives=["gpt-4o-mini", "gpt-4.1"],
                    reasoning="Need better reasoning quality",
                    message="Escalated to stronger model",
                )
    finally:
        Continua._instance = previous_client

    assert len(stub_client.events) == 1
    assert stub_client.events[0] == {
        "trace_id": stub_client.traces[0]["trace_id"],
        "span_id": stub_client.spans[0]["span_id"],
        "event_type": "decision",
        "level": "info",
        "message": "Escalated to stronger model",
        "payload": {
            "question": "Which model?",
            "chosen": "gpt-4.1",
            "alternatives": ["gpt-4o-mini", "gpt-4.1"],
            "reasoning": "Need better reasoning quality",
        },
    }
