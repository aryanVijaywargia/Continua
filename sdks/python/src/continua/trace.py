"""Trace context and decorator for tracing agent executions."""

from __future__ import annotations

import functools
import uuid
from contextvars import ContextVar
from datetime import datetime, timezone
from typing import Any, Callable, TypeVar

from .client import Continua

# Context variable for the current trace
_current_trace: ContextVar[TraceContext | None] = ContextVar(
    "current_trace", default=None
)

T = TypeVar("T")


def get_current_trace() -> TraceContext | None:
    """Get the current trace context, if any."""
    return _current_trace.get()


class TraceContext:
    """Context manager for a trace execution.

    Tracks the lifecycle of a trace and automatically sends it to the server.

    Example:
        with TraceContext(name="my_agent") as trace:
            trace.set_input({"query": "..."})
            # do work
            trace.set_output({"response": "..."})
    """

    def __init__(
        self,
        name: str,
        *,
        session_id: str | None = None,
        user_id: str | None = None,
        tags: list[str] | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> None:
        """Create a new trace context.

        Args:
            name: Name of the trace
            session_id: Optional session identifier
            user_id: Optional user identifier
            tags: Optional list of tags
            metadata: Optional metadata dictionary
        """
        self.trace_id = str(uuid.uuid4())
        self.name = name
        self.session_id = session_id
        self.user_id = user_id
        self.tags = tags or []
        self.metadata = metadata or {}
        self.start_time = datetime.now(timezone.utc)
        self.end_time: datetime | None = None
        self.status = "running"
        self._input: Any | None = None
        self._output: Any | None = None
        self._token: Any = None

    def set_input(self, value: Any) -> None:
        """Set the trace input."""
        self._input = value

    def set_output(self, value: Any) -> None:
        """Set the trace output."""
        self._output = value

    def set_metadata(self, key: str, value: Any) -> None:
        """Set a metadata value."""
        self.metadata[key] = value

    def __enter__(self) -> TraceContext:
        """Enter the trace context."""
        self._token = _current_trace.set(self)
        self._send_trace_start()
        return self

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc_val: BaseException | None,
        exc_tb: Any,
    ) -> None:
        """Exit the trace context."""
        self.end_time = datetime.now(timezone.utc)
        self.status = "failed" if exc_type is not None else "completed"
        self._send_trace_end()
        _current_trace.reset(self._token)

    def _send_trace_start(self) -> None:
        """Send the trace start event."""
        try:
            client = Continua.get_instance()
            trace_data: dict[str, Any] = {
                "trace_id": self.trace_id,
                "name": self.name,
                "status": "running",
                "start_time": self.start_time.isoformat(),
            }
            if self.session_id:
                trace_data["session_id"] = self.session_id
            if self.user_id:
                trace_data["user_id"] = self.user_id
            if self.tags:
                trace_data["tags"] = self.tags
            if self.metadata:
                trace_data["metadata"] = self.metadata
            if self._input is not None:
                trace_data["input"] = self._input
            client.add_trace(trace_data)
        except RuntimeError:
            # Client not initialized - skip
            pass

    def _send_trace_end(self) -> None:
        """Send the trace end event."""
        try:
            client = Continua.get_instance()
            trace_data: dict[str, Any] = {
                "trace_id": self.trace_id,
                "name": self.name,
                "status": self.status,
                "start_time": self.start_time.isoformat(),
            }
            if self.end_time:
                trace_data["end_time"] = self.end_time.isoformat()
            if self._output is not None:
                trace_data["output"] = self._output
            if self.metadata:
                trace_data["metadata"] = self.metadata
            client.add_trace(trace_data)
        except RuntimeError:
            # Client not initialized - skip
            pass


def trace(
    name: str | None = None,
    *,
    session_id: str | None = None,
    user_id: str | None = None,
    tags: list[str] | None = None,
    metadata: dict[str, Any] | None = None,
) -> Callable[[Callable[..., T]], Callable[..., T]]:
    """Decorator to trace a function execution.

    Args:
        name: Name of the trace (defaults to function name)
        session_id: Optional session identifier
        user_id: Optional user identifier
        tags: Optional list of tags
        metadata: Optional metadata dictionary

    Example:
        @trace()
        def my_agent(query: str) -> str:
            return "response"

        @trace(name="custom_name", tags=["production"])
        def another_agent():
            pass
    """

    def decorator(func: Callable[..., T]) -> Callable[..., T]:
        trace_name = name or func.__name__

        @functools.wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> T:
            with TraceContext(
                name=trace_name,
                session_id=session_id,
                user_id=user_id,
                tags=tags,
                metadata=metadata,
            ) as ctx:
                result = func(*args, **kwargs)
                # Auto-capture output if it's a simple return
                if result is not None:
                    ctx.set_output(result)
                return result

        return wrapper

    return decorator
