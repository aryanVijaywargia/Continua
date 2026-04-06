"""Continua SDK for Python.

A simple SDK for tracing AI agent executions.

Example:
    from continua import Continua, trace, span, session

    # Initialize the client
    Continua.init(api_key="your_api_key")

    # Use session context to group traces
    with session("user_session_123"):
        @trace()
        def my_agent(query: str) -> str:
            with span("openai_call", kind="llm") as s:
                s.set_input({"query": query})
                result = call_openai(query)
                s.set_llm_response("gpt-4", query, result, tokens_in=100, tokens_out=50)
            return result
"""

__version__ = "0.0.1"

from .client import Continua
from .engine_control import EngineControlClient
from .exceptions import (
    AuthenticationError,
    ContinuaError,
    EngineRunNotFoundError,
    EngineRunNotTerminalError,
    EngineRunWaitTimeoutError,
    NetworkError,
    RateLimitError,
    ValidationError,
)
from .session import SessionContext, get_current_session, session
from .span import SpanContext, get_current_span, span
from .trace import TraceContext, get_current_trace, trace

__all__ = [
    # Core client
    "Continua",
    "EngineControlClient",
    # Decorators and context managers
    "trace",
    "span",
    "session",
    # Context classes
    "TraceContext",
    "SpanContext",
    "SessionContext",
    # Context getters
    "get_current_trace",
    "get_current_span",
    "get_current_session",
    # Exceptions
    "ContinuaError",
    "AuthenticationError",
    "EngineRunNotFoundError",
    "EngineRunNotTerminalError",
    "EngineRunWaitTimeoutError",
    "RateLimitError",
    "ValidationError",
    "NetworkError",
]
