"""Session context manager for grouping traces."""

from __future__ import annotations

import uuid
from contextvars import ContextVar
from typing import Any

# Context variable for the current session
_current_session: ContextVar[SessionContext | None] = ContextVar(
    "current_session", default=None
)


def get_current_session() -> SessionContext | None:
    """Get the current session context, if any."""
    return _current_session.get()


def get_current_session_id() -> str | None:
    """Get the current session ID, if any."""
    session = _current_session.get()
    return session.session_id if session else None


class SessionContext:
    """Context manager for session scoping.

    All traces created within the session context will automatically
    inherit the session_id.

    Example:
        with session("user_123_session") as sess:
            @trace()
            def my_agent(query):
                # This trace will have session_id = "user_123_session"
                return "response"
            my_agent("hello")

        # Or with auto-generated session ID:
        with session() as sess:
            print(f"Session ID: {sess.session_id}")
            # All traces here inherit the session ID
    """

    def __init__(
        self,
        session_id: str | None = None,
        *,
        user_id: str | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> None:
        """Create a new session context.

        Args:
            session_id: Optional session identifier. If not provided, a UUID is generated.
            user_id: Optional user identifier for the session
            metadata: Optional metadata dictionary
        """
        self.session_id = session_id or str(uuid.uuid4())
        self.user_id = user_id
        self.metadata = metadata or {}
        self._token: Any = None

    def __enter__(self) -> SessionContext:
        """Enter the session context."""
        self._token = _current_session.set(self)
        return self

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc_val: BaseException | None,
        exc_tb: Any,
    ) -> None:
        """Exit the session context."""
        _current_session.reset(self._token)


def session(
    session_id: str | None = None,
    *,
    user_id: str | None = None,
    metadata: dict[str, Any] | None = None,
) -> SessionContext:
    """Create a session context manager.

    Args:
        session_id: Optional session identifier. If not provided, a UUID is generated.
        user_id: Optional user identifier for the session
        metadata: Optional metadata dictionary

    Returns:
        A SessionContext that can be used as a context manager

    Example:
        with session("user_session_123") as sess:
            # All traces created here inherit session_id
            pass

        with session() as sess:
            # Auto-generated session ID
            print(f"Session: {sess.session_id}")
    """
    return SessionContext(session_id, user_id=user_id, metadata=metadata)
