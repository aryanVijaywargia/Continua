"""Live integration tests for the Python engine control client.

These tests require a running Continua server with:
- ENGINE_PUBLIC_API_ENABLED=true
- a project using CONTINUA_API_KEY
- a published engine definition checkout/v1
- for continuation coverage, a published continue-as-new-capable definition
  (defaults: continue-demo/v1) that starts with `{"cursor": 1}`, continues
  twice, and then completes

They reuse the same opt-in local-server style as the other Python integration tests.
"""

from __future__ import annotations

import os
import uuid

import httpx
import pytest

from continua import (
    EngineControlClient,
    EngineRunContinuationDepthError,
    EngineRunNotFoundError,
)
from continua.types import (
    EngineProjectionState,
    EnginePurgeMode,
    EngineRepairReason,
    EngineRunStatus,
)

CONTINUA_ENDPOINT = os.environ.get("CONTINUA_ENDPOINT", "http://localhost:8081")
CONTINUA_API_KEY = os.environ.get("CONTINUA_API_KEY", "test-api-key-12345")
CONTINUA_CONTINUATION_DEFINITION_NAME = os.environ.get(
    "CONTINUA_CONTINUATION_DEFINITION_NAME",
    "continue-demo",
)
CONTINUA_CONTINUATION_DEFINITION_VERSION = os.environ.get(
    "CONTINUA_CONTINUATION_DEFINITION_VERSION",
    "v1",
)


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


def _start_run(
    client: EngineControlClient,
    *,
    definition_name: str,
    definition_version: str,
    input: dict[str, object] | None = None,
):
    try:
        return client.start(
            {
                "definition_name": definition_name,
                "definition_version": definition_version,
                "instance_key": f"py-engine-instance-{uuid.uuid4()}",
                "request_key": f"py-engine-request-{uuid.uuid4()}",
                "input": input,
            }
        )
    except EngineRunNotFoundError as exc:
        pytest.skip(
            "engine control integration requires ENGINE_PUBLIC_API_ENABLED=true "
            f"and {definition_name}/{definition_version} to be published: {exc}"
        )


def _start_checkout_run(client: EngineControlClient):
    return _start_run(
        client,
        definition_name="checkout",
        definition_version="v1",
    )


def _start_continuation_run(client: EngineControlClient):
    return _start_run(
        client,
        definition_name=CONTINUA_CONTINUATION_DEFINITION_NAME,
        definition_version=CONTINUA_CONTINUATION_DEFINITION_VERSION,
        input={"cursor": 1},
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
        assert instance.current_run.run_id == started.run_id

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


def test_engine_control_wait_for_terminal_follows_continuations():
    client = _client()
    try:
        started = _start_continuation_run(client)

        first = client.wait_for_terminal(started.run_id, timeout=10.0, poll_interval=0.1)
        assert first.status == EngineRunStatus.CONTINUED_AS_NEW
        assert first.continued_to_run_id is not None

        second = client.wait_for_terminal(
            first.continued_to_run_id,
            timeout=10.0,
            poll_interval=0.1,
        )
        assert second.status == EngineRunStatus.CONTINUED_AS_NEW
        assert second.continued_to_run_id is not None

        final = client.wait_for_terminal(
            started.run_id,
            timeout=10.0,
            poll_interval=0.1,
            follow_continuations=True,
            max_continuations=4,
        )
        assert final.status == EngineRunStatus.COMPLETED
        assert final.run_id == second.continued_to_run_id
    finally:
        client.close()


def test_engine_control_wait_for_terminal_continuation_depth_limit():
    client = _client()
    try:
        started = _start_continuation_run(client)

        with pytest.raises(EngineRunContinuationDepthError):
            client.wait_for_terminal(
                started.run_id,
                timeout=10.0,
                poll_interval=0.1,
                follow_continuations=True,
                max_continuations=1,
            )
    finally:
        client.close()
