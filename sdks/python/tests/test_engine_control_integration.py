"""Live integration tests for the Python engine control client.

These tests require a running Continua server with:
- ENGINE_PUBLIC_API_ENABLED=true
- a project using CONTINUA_API_KEY
- a published engine definition checkout/v1

They reuse the same opt-in local-server style as the other Python integration tests.
"""

from __future__ import annotations

import os
import uuid

import httpx
import pytest

from continua import EngineControlClient, EngineRunNotFoundError
from continua.types import (
    EngineProjectionState,
    EnginePurgeMode,
    EngineRepairReason,
    EngineRunStatus,
)

CONTINUA_ENDPOINT = os.environ.get("CONTINUA_ENDPOINT", "http://localhost:8081")
CONTINUA_API_KEY = os.environ.get("CONTINUA_API_KEY", "test-api-key-12345")


def server_available() -> bool:
    try:
        response = httpx.get(f"{CONTINUA_ENDPOINT}/api/health", timeout=2.0)
        return response.status_code == 200
    except httpx.RequestError:
        return False


pytestmark = pytest.mark.skipif(
    not server_available(),
    reason=f"Continua server not available at {CONTINUA_ENDPOINT}",
)


def _client() -> EngineControlClient:
    return EngineControlClient(
        api_key=CONTINUA_API_KEY,
        endpoint=CONTINUA_ENDPOINT,
        timeout=10.0,
    )


def _start_checkout_run(client: EngineControlClient):
    try:
        return client.start(
            {
                "definition_name": "checkout",
                "definition_version": "v1",
                "instance_key": f"py-engine-instance-{uuid.uuid4()}",
                "request_key": f"py-engine-request-{uuid.uuid4()}",
            }
        )
    except EngineRunNotFoundError as exc:
        pytest.skip(
            "engine control integration requires ENGINE_PUBLIC_API_ENABLED=true "
            f"and checkout/v1 to be published: {exc}"
        )


def test_engine_control_round_trip_terminate_purge_and_repair():
    client = _client()
    try:
        started = _start_checkout_run(client)

        run = client.get_run(started.run_id)
        assert run.run_id == started.run_id
        assert run.instance_key == started.instance_key

        instance = client.get_instance(started.instance_key)
        assert instance.instance_key == started.instance_key
        assert instance.run.run_id == started.run_id

        terminated = client.terminate(started.run_id)
        assert terminated.status == EngineRunStatus.TERMINATED

        waited = client.wait_for_terminal(started.run_id, timeout=5.0, poll_interval=0.1)
        assert waited.status == EngineRunStatus.TERMINATED

        projection_only = client.purge(started.run_id, mode=EnginePurgeMode.projection_only)
        assert projection_only.projection_state == EngineProjectionState.summary_only

        summary_only_repair = client.repair(started.run_id)
        assert summary_only_repair.reason == EngineRepairReason.no_events_to_project
        assert summary_only_repair.projection_state == EngineProjectionState.summary_only

        full = client.purge(started.run_id, mode=EnginePurgeMode.full)
        assert full.projection_state == EngineProjectionState.journal_expired

        history = client.get_history(started.run_id)
        assert history.expired is True
        assert history.events == []

        expired_repair = client.repair(started.run_id)
        assert expired_repair.reason == EngineRepairReason.history_expired
        assert expired_repair.projection_state == EngineProjectionState.journal_expired
    finally:
        client.close()
