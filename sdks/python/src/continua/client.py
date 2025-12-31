"""Continua API client."""

from dataclasses import dataclass
from typing import Any, TypedDict

import httpx


class HealthResponse(TypedDict):
    status: str
    version: str
    commit: str
    build_time: str


@dataclass
class ContinuaClientConfig:
    """Configuration for the Continua client."""

    base_url: str
    auth_token: str | None = None
    timeout: float = 30.0


class ContinuaClient:
    """Client for interacting with the Continua API."""

    def __init__(self, config: ContinuaClientConfig) -> None:
        self._config = config
        self._http = httpx.AsyncClient(
            base_url=config.base_url,
            timeout=config.timeout,
            headers={"Authorization": f"Bearer {config.auth_token}"} if config.auth_token else {},
        )

    async def health(self) -> HealthResponse:
        """Check server health."""
        response = await self._http.get("/health")
        response.raise_for_status()
        return response.json()

    async def start_execution(
        self,
        agent_type: str,
        input_data: Any,
        *,
        tenant_id: str = "default",
    ) -> str:
        """Start a new agent execution."""
        # TODO: Implement with gRPC client
        raise NotImplementedError

    async def get_execution(self, execution_id: str) -> dict[str, Any]:
        """Get execution details."""
        # TODO: Implement with gRPC client
        raise NotImplementedError

    async def list_events(self, execution_id: str) -> list[dict[str, Any]]:
        """List events for an execution."""
        # TODO: Implement with gRPC client
        raise NotImplementedError

    async def close(self) -> None:
        """Close the client."""
        await self._http.aclose()

    async def __aenter__(self) -> "ContinuaClient":
        return self

    async def __aexit__(self, *args: Any) -> None:
        await self.close()
