"""Pytest configuration for integration tests."""

import os

import pytest
import pytest_asyncio

from continua import ContinuaClient
from continua.client import ContinuaClientConfig


@pytest.fixture
def api_url() -> str:
    return os.environ.get("CONTINUA_API_URL", "http://localhost:8243")


@pytest_asyncio.fixture
async def client(api_url: str) -> ContinuaClient:
    config = ContinuaClientConfig(base_url=api_url)
    async with ContinuaClient(config) as client:
        yield client
