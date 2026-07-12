"""Live end-to-end tests for Go engine workflows calling Python remote activities.

Local runbook:
- Start Postgres and set DATABASE_URL for the target database.
- Run `continua serve` with ENGINE_PUBLIC_API_ENABLED=true.
- Run `continua-engine migrate up` and `continua-engine serve` against the same DATABASE_URL.
- Seed a project API key using the same scheme as `scripts/seed-demo.sh`.
- Set CONTINUA_ENDPOINT and CONTINUA_API_KEY for that server and project.
"""

from __future__ import annotations

import os
import threading
import time
import uuid
from collections.abc import Callable, Iterator
from contextlib import contextmanager
from typing import Any

import httpx
import pytest

from continua import (
    ActivityWorker,
    EngineControlClient,
    EngineRunNotFoundError,
    NonRetryableError,
    ValidationError,
)
from continua.types import EngineRunResponse, EngineRunStatus, EngineStartRunResponse

CONTINUA_ENDPOINT = os.environ.get("CONTINUA_ENDPOINT", "http://localhost:8081")
CONTINUA_API_KEY = os.environ.get("CONTINUA_API_KEY", "test-api-key-12345")
REMOTE_DEMO_DEFINITION_NAME = "darklaunch.remote-demo"
REMOTE_DEMO_DEFINITION_VERSION = "v1"
REMOTE_ACTIVITY_TYPE = "darklaunch.remote-compose-greeting"


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


def _start_remote_run(
    client: EngineControlClient,
    *,
    input: dict[str, object],
) -> EngineStartRunResponse:
    try:
        return client.start(
            {
                "definition_name": REMOTE_DEMO_DEFINITION_NAME,
                "definition_version": REMOTE_DEMO_DEFINITION_VERSION,
                "instance_key": f"py-worker-e2e-{uuid.uuid4()}",
                "request_key": f"py-worker-e2e-req-{uuid.uuid4()}",
                "input": input,
            }
        )
    except (EngineRunNotFoundError, ValidationError) as exc:
        pytest.fail(
            "darklaunch.remote-demo/v1 is not published; a current continua-engine serve "
            f"must be running with that definition registered: {exc}"
        )


@contextmanager
def running_worker(
    handler: Callable[[dict[str, Any]], dict[str, Any]],
    *,
    worker_id: str | None = None,
    lease_duration: str = "10s",
    heartbeat_interval: float | None = None,
    poll_interval: float = 0.2,
) -> Iterator[ActivityWorker]:
    worker = ActivityWorker(
        api_key=CONTINUA_API_KEY,
        endpoint=CONTINUA_ENDPOINT,
        worker_id=worker_id,
        lease_duration=lease_duration,
        heartbeat_interval=heartbeat_interval,
        poll_interval=poll_interval,
    )
    worker.register(REMOTE_ACTIVITY_TYPE, handler)
    stop = threading.Event()

    def poll_loop() -> None:
        while not stop.is_set():
            worker.poll_once()
            time.sleep(poll_interval)

    thread = threading.Thread(target=poll_loop, name=f"{worker.worker_id}-poll", daemon=True)
    thread.start()
    try:
        yield worker
    finally:
        stop.set()
        thread.join(timeout=2.0)
        worker.wait_for_idle(timeout=worker.shutdown_timeout)
        worker.close()


def _terminate_if_needed(client: EngineControlClient, run_id: object | None) -> None:
    if run_id is None:
        return
    try:
        run = client.get_run(run_id)
    except Exception:
        return
    if run.status not in _TERMINAL_STATUSES:
        client.terminate(run_id)


_TERMINAL_STATUSES = {
    EngineRunStatus.COMPLETED,
    EngineRunStatus.FAILED,
    EngineRunStatus.CANCELLED,
    EngineRunStatus.TERMINATED,
    EngineRunStatus.CONTINUED_AS_NEW,
}


def test_remote_activity_happy_path_completes_workflow() -> None:
    nonce = uuid.uuid4().hex
    seen_inputs: list[dict[str, Any]] = []
    seen_lock = threading.Lock()
    client = _client()
    run_id = None

    def handler(inp: dict[str, Any]) -> dict[str, str]:
        with seen_lock:
            seen_inputs.append(dict(inp))
        return {"greeting": f"hello, {inp['name']}"}

    try:
        started = _start_remote_run(client, input={"name": nonce})
        run_id = started.run_id
        with running_worker(handler):
            terminal = client.wait_for_terminal(run_id, timeout=60.0, poll_interval=0.2)

        assert terminal.status == EngineRunStatus.COMPLETED
        result = client.get_result(run_id)
        assert result.status == EngineRunStatus.COMPLETED
        assert result.result == {"greeting": f"hello, {nonce}"}
        assert {"name": nonce} in seen_inputs
    finally:
        _terminate_if_needed(client, run_id)
        client.close()


def test_remote_activity_retryable_failure_then_success() -> None:
    nonce = uuid.uuid4().hex
    invocation_counts: dict[str, int] = {}
    counts_lock = threading.Lock()
    client = _client()
    run_id = None

    def handler(inp: dict[str, Any]) -> dict[str, str]:
        name = str(inp["name"])
        with counts_lock:
            invocation_counts[name] = invocation_counts.get(name, 0) + 1
            count = invocation_counts[name]
        if name == nonce and count == 1:
            raise RuntimeError("transient boom")
        return {"greeting": f"retry:{inp['name']}"}

    try:
        started = _start_remote_run(client, input={"name": nonce})
        run_id = started.run_id
        with running_worker(handler):
            terminal = client.wait_for_terminal(run_id, timeout=60.0, poll_interval=0.2)

        assert terminal.status == EngineRunStatus.COMPLETED
        result = client.get_result(run_id)
        assert result.status == EngineRunStatus.COMPLETED
        assert result.result == {"greeting": f"retry:{nonce}"}
        assert invocation_counts.get(nonce, 0) >= 2
    finally:
        _terminate_if_needed(client, run_id)
        client.close()


def test_remote_activity_lease_loss_second_worker_completes() -> None:
    nonce = uuid.uuid4().hex
    worker_a_claimed = threading.Event()
    release_worker_a = threading.Event()
    client = _client()
    run_id = None

    def worker_a_handler(inp: dict[str, Any]) -> dict[str, str]:
        if inp["name"] == nonce:
            worker_a_claimed.set()
            release_worker_a.wait(timeout=120.0)
        return {"greeting": f"a:{inp['name']}"}

    def worker_b_handler(inp: dict[str, Any]) -> dict[str, str]:
        return {"greeting": f"b:{inp['name']}"}

    try:
        started = _start_remote_run(client, input={"name": nonce})
        run_id = started.run_id
        with running_worker(
            worker_a_handler,
            worker_id=f"py-worker-e2e-a-{nonce}",
            lease_duration="10s",
            heartbeat_interval=3600.0,
        ):
            assert worker_a_claimed.wait(timeout=30.0), "worker A did not claim the activity"
            with running_worker(
                worker_b_handler,
                worker_id=f"py-worker-e2e-b-{nonce}",
            ):
                terminal = client.wait_for_terminal(run_id, timeout=90.0, poll_interval=0.2)

            assert terminal.status == EngineRunStatus.COMPLETED
            result = client.get_result(run_id)
            assert result.status == EngineRunStatus.COMPLETED
            assert result.result == {"greeting": f"b:{nonce}"}

            release_worker_a.set()

        result_after_late_completion = client.get_result(run_id)
        assert result_after_late_completion.status == EngineRunStatus.COMPLETED
        assert result_after_late_completion.result == {"greeting": f"b:{nonce}"}
    finally:
        release_worker_a.set()
        _terminate_if_needed(client, run_id)
        client.close()


def test_remote_activity_non_retryable_failure_fails_workflow() -> None:
    nonce = uuid.uuid4().hex
    invocation_counts: dict[str, int] = {}
    counts_lock = threading.Lock()
    client = _client()
    run_id = None

    def handler(inp: dict[str, Any]) -> dict[str, str]:
        name = str(inp["name"])
        with counts_lock:
            invocation_counts[name] = invocation_counts.get(name, 0) + 1
        raise NonRetryableError("permanent boom")

    try:
        started = _start_remote_run(client, input={"name": nonce})
        run_id = started.run_id
        with running_worker(handler):
            terminal: EngineRunResponse = client.wait_for_terminal(
                run_id,
                timeout=60.0,
                poll_interval=0.2,
            )

        assert terminal.status == EngineRunStatus.FAILED
        result = client.get_result(run_id)
        assert result.status == EngineRunStatus.FAILED
        assert result.failure is not None
        assert "permanent boom" in result.failure.error_message
        assert invocation_counts.get(nonce, 0) == 1
    finally:
        _terminate_if_needed(client, run_id)
        client.close()
