"""Tests for the Continua client."""

import pytest

from continua import ContinuaClient
from continua.client import ContinuaClientConfig


@pytest.fixture
def client() -> ContinuaClient:
    config = ContinuaClientConfig(base_url="http://localhost:8243")
    return ContinuaClient(config)


@pytest.mark.asyncio
async def test_client_creation(client: ContinuaClient) -> None:
    """Test client can be created."""
    assert client is not None


@pytest.mark.asyncio
async def test_client_context_manager() -> None:
    """Test client works as context manager."""
    config = ContinuaClientConfig(base_url="http://localhost:8243")
    async with ContinuaClient(config) as client:
        assert client is not None
