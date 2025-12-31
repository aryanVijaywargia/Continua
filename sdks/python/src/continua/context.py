"""Agent execution context."""

from dataclasses import dataclass, field
from typing import Any


@dataclass
class Message:
    """A message in the conversation."""

    role: str  # system, user, assistant, tool
    content: str
    tool_call_id: str | None = None
    name: str | None = None


@dataclass
class AgentContext:
    """Context for agent execution."""

    execution_id: str
    tenant_id: str = "default"
    _messages: list[Message] = field(default_factory=list)
    _memory: dict[str, Any] = field(default_factory=dict)
    _is_replaying: bool = False

    def add_message(self, message: Message) -> None:
        """Add a message to the conversation."""
        self._messages.append(message)

    def get_messages(self) -> list[Message]:
        """Get all messages."""
        return list(self._messages)

    def set_memory(self, key: str, value: Any) -> None:
        """Set a memory value."""
        self._memory[key] = value

    def get_memory(self, key: str, default: Any = None) -> Any:
        """Get a memory value."""
        return self._memory.get(key, default)

    def is_replaying(self) -> bool:
        """Check if the context is in replay mode."""
        return self._is_replaying
