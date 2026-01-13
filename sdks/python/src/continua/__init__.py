"""Continua SDK for Python.

A simple SDK for tracing AI agent executions.

Example:
    from continua import Continua, trace, span

    # Initialize the client
    Continua.init(api_key="your_api_key")

    @trace()
    def my_agent(query: str) -> str:
        with span("openai_call", kind="llm") as s:
            s.set_input({"query": query})
            result = call_openai(query)
            s.set_output(result)
            s.set_tokens(prompt=100, completion=50)
        return result
"""

__version__ = "0.0.1"

from .client import Continua
from .span import SpanContext, get_current_span, span
from .trace import TraceContext, get_current_trace, trace

__all__ = [
    "Continua",
    "trace",
    "span",
    "TraceContext",
    "SpanContext",
    "get_current_trace",
    "get_current_span",
]
