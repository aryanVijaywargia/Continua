"""Integration tests for health endpoint."""

import pytest

from continua import ContinuaClient


@pytest.mark.asyncio
async def test_health(client: ContinuaClient) -> None:
    """Test health endpoint returns OK."""
    health = await client.health()
    assert health["status"] == "ok"
    assert "version" in health
    assert "commit" in health
    assert "build_time" in health
