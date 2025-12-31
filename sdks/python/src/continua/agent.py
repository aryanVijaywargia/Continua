"""Base agent class."""

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any

from continua.context import AgentContext


@dataclass
class AgentConfig:
    """Agent configuration."""

    name: str
    version: str = "1.0.0"
    max_iterations: int = 100
    timeout_seconds: float = 300.0
    metadata: dict[str, str] = field(default_factory=dict)


class Agent(ABC):
    """Base class for Continua agents."""

    def __init__(self, config: AgentConfig) -> None:
        self._config = config

    @property
    def name(self) -> str:
        return self._config.name

    @property
    def version(self) -> str:
        return self._config.version

    @abstractmethod
    async def run(self, context: AgentContext, input_data: Any) -> Any:
        """
        Execute the agent logic.

        Args:
            context: The agent execution context
            input_data: Input data for the agent

        Returns:
            The agent's output
        """
        ...
