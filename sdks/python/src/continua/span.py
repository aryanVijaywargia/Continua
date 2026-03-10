"""Span context and helper for tracing individual operations."""

from __future__ import annotations

import traceback
import uuid
from contextvars import ContextVar
from datetime import datetime, timezone
from typing import Any, Literal

from .client import _get_client_if_initialized
from .trace import get_current_trace

# Context variable for the current span
_current_span: ContextVar[SpanContext | None] = ContextVar(
    "current_span", default=None
)

SpanKind = Literal["llm", "tool", "agent", "chain", "retrieval", "embedding", "generation", "default"]


def get_current_span() -> SpanContext | None:
    """Get the current span context, if any."""
    return _current_span.get()


class SpanContext:
    """Context manager for a span within a trace.

    Tracks an individual operation (LLM call, tool use, etc.) within a trace.

    Example:
        with span("openai_call", kind="llm") as s:
            s.set_input({"messages": [...]})
            response = openai.chat.completions.create(...)
            s.set_output(response.model_dump())
            s.set_tokens(prompt=100, completion=50)
    """

    def __init__(
        self,
        name: str,
        *,
        kind: SpanKind = "default",
        metadata: dict[str, Any] | None = None,
    ) -> None:
        """Create a new span context.

        Args:
            name: Name of the span
            kind: Type of operation (llm, tool, agent, etc.)
            metadata: Optional metadata dictionary
        """
        self.span_id = str(uuid.uuid4())
        self.name = name
        self.kind = kind
        self.metadata = metadata or {}
        self.start_time = datetime.now(timezone.utc)
        self.end_time: datetime | None = None
        self.status = "running"
        self.status_message: str | None = None

        # Get trace and parent span from context
        trace = get_current_trace()
        self.trace_id = trace.trace_id if trace else None

        parent = _current_span.get()
        self.parent_span_id = parent.span_id if parent else None

        # Metrics
        self._input: Any | None = None
        self._output: Any | None = None
        self._model: str | None = None
        self._provider: str | None = None
        self._prompt_tokens: int | None = None
        self._completion_tokens: int | None = None
        self._total_tokens: int | None = None
        self._total_cost: float | None = None

        self._token: Any = None

    def set_input(self, value: Any) -> None:
        """Set the span input."""
        self._input = value

    def set_output(self, value: Any) -> None:
        """Set the span output."""
        self._output = value

    def set_model(self, model: str, provider: str | None = None) -> None:
        """Set the model used in this span."""
        self._model = model
        self._provider = provider

    def set_tokens(
        self,
        *,
        prompt: int | None = None,
        completion: int | None = None,
        total: int | None = None,
    ) -> None:
        """Set token counts for this span.

        At least one directional field (`prompt` or `completion`) must be provided.
        `total` is retained as a local helper value and is not sent to the server.
        """
        if total is not None and prompt is None and completion is None:
            raise ValueError(
                "set_tokens(total=...) without prompt/completion is unsupported; "
                "provide prompt and/or completion tokens",
            )
        self._prompt_tokens = prompt
        self._completion_tokens = completion
        self._total_tokens = total or ((prompt or 0) + (completion or 0))

    def set_cost(self, cost: float) -> None:
        """Set the cost for this span."""
        self._total_cost = cost

    def set_metadata(self, key: str, value: Any) -> None:
        """Set a metadata value."""
        self.metadata[key] = value

    def set_error(self, message: str) -> None:
        """Mark this span as failed with an error message."""
        self.status = "failed"
        self.status_message = message

    def set_llm_response(
        self,
        model: str,
        messages: Any,
        response: Any,
        tokens_in: int | None = None,
        tokens_out: int | None = None,
        *,
        provider: str | None = None,
        cost: float | None = None,
    ) -> None:
        """Set LLM call details on this span.

        Args:
            model: The model name (e.g., "gpt-4", "claude-3-opus")
            messages: The input messages/prompt
            response: The LLM response
            tokens_in: Number of input/prompt tokens
            tokens_out: Number of output/completion tokens
            provider: Optional provider name (e.g., "openai", "anthropic")
            cost: Optional cost in USD
        """
        self._model = model
        self._provider = provider
        self._input = messages
        self._output = response
        self._prompt_tokens = tokens_in
        self._completion_tokens = tokens_out
        if tokens_in is not None or tokens_out is not None:
            self._total_tokens = (tokens_in or 0) + (tokens_out or 0)
        if cost is not None:
            self._total_cost = cost

    def set_tool_call(
        self,
        tool_name: str,
        arguments: Any,
        result: Any,
    ) -> None:
        """Set tool call details on this span.

        Args:
            tool_name: Name of the tool being called
            arguments: The tool arguments/input
            result: The tool execution result
        """
        self.name = tool_name  # Override span name with tool name
        self._input = arguments
        self._output = result

    def log(
        self,
        message: str,
        level: str = "info",
        payload: dict[str, Any] | None = None,
    ) -> None:
        """Log a message as an event on this span.

        Args:
            message: The log message
            level: Log level ("debug", "info", "warning", "error")
            payload: Optional additional data
        """
        self._record_event(
            event_type="log",
            level=level,
            message=message,
            payload=payload,
        )

    def error(
        self,
        message: str,
        payload: dict[str, Any] | None = None,
    ) -> None:
        """Emit an explicit error event for this span."""
        self._record_event(
            event_type="error",
            level="error",
            message=message,
            payload=payload,
        )

    def exception(
        self,
        exc: BaseException,
        payload: dict[str, Any] | None = None,
    ) -> None:
        """Capture an exception and attach structured exception details."""
        exception_payload = dict(payload or {})
        exception_payload.update(
            {
                "exception_type": type(exc).__name__,
                "exception_message": str(exc),
                "traceback": "".join(
                    traceback.format_exception(type(exc), exc, exc.__traceback__)
                ),
            }
        )

        self._record_event(
            event_type="exception",
            level="error",
            message=str(exc),
            payload=exception_payload,
        )

    def metric(
        self,
        name: str,
        value: int | float,
        unit: str | None = None,
        payload: dict[str, Any] | None = None,
    ) -> None:
        """Emit a structured metric event for this span."""
        metric_payload = dict(payload or {})
        metric_payload.update(
            {
                "metric_name": name,
                "metric_value": value,
            }
        )
        if unit is not None:
            metric_payload["metric_unit"] = unit

        self._record_event(
            event_type="metric",
            level="info",
            payload=metric_payload,
        )

    def __enter__(self) -> SpanContext:
        """Enter the span context."""
        self._token = _current_span.set(self)
        self._send_span_start()
        return self

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc_val: BaseException | None,
        exc_tb: Any,
    ) -> None:
        """Exit the span context."""
        self.end_time = datetime.now(timezone.utc)
        if exc_type is not None:
            self.status = "failed"
            self.status_message = str(exc_val)
        elif self.status == "running":
            self.status = "completed"
        self._send_span_end()
        _current_span.reset(self._token)

    def _build_span_data(self) -> dict[str, Any]:
        """Build the span data dictionary."""
        if self.trace_id is None:
            return {}

        data: dict[str, Any] = {
            "trace_id": self.trace_id,
            "span_id": self.span_id,
            "name": self.name,
            "type": self.kind,
            "status": self.status,
            "start_time": self.start_time.isoformat(),
        }

        if self.parent_span_id:
            data["parent_span_id"] = self.parent_span_id
        if self.end_time:
            data["end_time"] = self.end_time.isoformat()
        if self.status_message:
            data["status_message"] = self.status_message
        if self._input is not None:
            data["input"] = self._input
        if self._output is not None:
            data["output"] = self._output
        if self._model:
            data["model"] = self._model
        if self._provider:
            data["provider"] = self._provider
        if self._prompt_tokens is not None:
            data["prompt_tokens"] = self._prompt_tokens
        if self._completion_tokens is not None:
            data["completion_tokens"] = self._completion_tokens
        if self._total_cost is not None:
            data["total_cost"] = self._total_cost
        if self.metadata:
            data["metadata"] = self.metadata

        return data

    def _send_span_start(self) -> None:
        """Send the span start event."""
        if self.trace_id is None:
            return
        client = _get_client_if_initialized()
        if client is None:
            return
        client.add_span(self._build_span_data())

    def _send_span_end(self) -> None:
        """Send the span end event."""
        if self.trace_id is None:
            return
        client = _get_client_if_initialized()
        if client is None:
            return
        client.add_span(self._build_span_data())

    def _record_event(
        self,
        *,
        event_type: str,
        level: str,
        message: str | None = None,
        payload: dict[str, Any] | None = None,
    ) -> None:
        """Queue a structured event for this span."""
        if self.trace_id is None:
            return

        client = _get_client_if_initialized()
        if client is None:
            return

        event_data: dict[str, Any] = {
            "trace_id": self.trace_id,
            "span_id": self.span_id,
            "event_type": event_type,
            "level": level,
        }
        if message is not None:
            event_data["message"] = message
        if payload is not None:
            event_data["payload"] = payload
        client.add_event(event_data)


def span(
    name: str,
    *,
    kind: SpanKind = "default",
    metadata: dict[str, Any] | None = None,
) -> SpanContext:
    """Create a span context manager.

    Args:
        name: Name of the span
        kind: Type of operation (llm, tool, agent, etc.)
        metadata: Optional metadata dictionary

    Returns:
        A SpanContext that can be used as a context manager

    Example:
        with span("openai_call", kind="llm") as s:
            s.set_input({"prompt": "..."})
            result = call_openai()
            s.set_output(result)
    """
    return SpanContext(name, kind=kind, metadata=metadata)
