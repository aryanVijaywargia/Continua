"""Decorators for agent tools."""

from functools import wraps
from typing import Any, Callable, TypeVar

F = TypeVar("F", bound=Callable[..., Any])


def tool(
    name: str | None = None,
    description: str | None = None,
) -> Callable[[F], F]:
    """
    Decorator to mark a method as a tool.

    Args:
        name: Tool name (defaults to function name)
        description: Tool description (defaults to docstring)

    Returns:
        Decorated function
    """

    def decorator(func: F) -> F:
        tool_name = name or func.__name__
        tool_description = description or func.__doc__ or ""

        @wraps(func)
        async def wrapper(*args: Any, **kwargs: Any) -> Any:
            # TODO: Add recording/replay logic
            return await func(*args, **kwargs)

        # Store metadata on the wrapper
        wrapper._tool_name = tool_name  # type: ignore
        wrapper._tool_description = tool_description  # type: ignore
        wrapper._is_tool = True  # type: ignore

        return wrapper  # type: ignore

    return decorator
