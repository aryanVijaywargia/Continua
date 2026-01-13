"""Tests for trace context and decorators."""

from continua import Continua, trace
from continua.trace import get_current_trace, TraceContext


def test_trace_context_basic():
    """Test basic trace context creation and lifecycle."""
    with TraceContext(name="test_trace") as ctx:
        assert ctx.name == "test_trace"
        assert ctx.trace_id is not None
        assert ctx.status == "running"
        assert get_current_trace() is ctx

    assert ctx.status == "completed"
    assert ctx.end_time is not None
    assert get_current_trace() is None


def test_trace_context_with_metadata():
    """Test trace context with metadata."""
    with TraceContext(
        name="test_trace",
        session_id="session-123",
        user_id="user-456",
        tags=["test", "unit"],
        metadata={"key": "value"},
    ) as ctx:
        ctx.set_input({"query": "test"})
        ctx.set_output({"response": "result"})
        ctx.set_metadata("extra", "data")

    assert ctx.session_id == "session-123"
    assert ctx.user_id == "user-456"
    assert ctx.tags == ["test", "unit"]
    assert ctx.metadata == {"key": "value", "extra": "data"}
    assert ctx._input == {"query": "test"}
    assert ctx._output == {"response": "result"}


def test_trace_context_on_exception():
    """Test trace context marks as failed on exception."""
    try:
        with TraceContext(name="failing_trace") as ctx:
            raise ValueError("Test error")
    except ValueError:
        pass

    assert ctx.status == "failed"


def test_trace_decorator():
    """Test the @trace decorator."""
    @trace()
    def my_function():
        current = get_current_trace()
        assert current is not None
        assert current.name == "my_function"
        return "result"

    result = my_function()
    assert result == "result"


def test_trace_decorator_with_name():
    """Test the @trace decorator with custom name."""
    @trace(name="custom_name", tags=["test"])
    def my_function():
        current = get_current_trace()
        assert current.name == "custom_name"
        assert current.tags == ["test"]
        return 42

    result = my_function()
    assert result == 42
