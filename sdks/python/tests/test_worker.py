"""Tests for the remote activity worker."""

from __future__ import annotations

import threading
import time
from typing import Any

import pytest

from continua import ActivityTaskContext, ActivityWorker, NonRetryableError


class StubResponse:
    def __init__(self, status_code: int, payload: dict[str, Any] | None = None) -> None:
        self.status_code = status_code
        self._payload = payload or {}
        self.text = ""

    def json(self) -> dict[str, Any]:
        return self._payload


class RecordingClient:
    def __init__(
        self,
        *,
        claims: list[list[dict[str, Any]]] | None = None,
        heartbeat_statuses: list[int] | None = None,
    ) -> None:
        self.claims = claims or []
        self.heartbeat_statuses = heartbeat_statuses or []
        self.requests: list[dict[str, Any]] = []
        self.closed = False
        self._lock = threading.Lock()

    def request(
        self,
        method: str,
        path: str,
        *,
        json: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> StubResponse:
        with self._lock:
            self.requests.append(
                {
                    "method": method,
                    "path": path,
                    "json": json,
                    "headers": headers,
                }
            )

            if path == "/v1/engine/activities/claim":
                tasks = self.claims.pop(0) if self.claims else []
                return StubResponse(200, {"tasks": tasks})

            if path.endswith("/heartbeat"):
                status = self.heartbeat_statuses.pop(0) if self.heartbeat_statuses else 200
                return StubResponse(
                    status,
                    {
                        "lease_expires_at": "2026-04-13T12:00:00Z",
                        "effective_lease_duration_ms": 20,
                    },
                )

            if path.endswith("/complete") or path.endswith("/fail"):
                return StubResponse(204)

        return StubResponse(500, {"message": "unexpected request"})

    def close(self) -> None:
        self.closed = True

    def paths(self) -> list[str]:
        return [request["path"] for request in self.requests]

    def payloads_for(self, suffix: str) -> list[dict[str, Any]]:
        return [
            request["json"]
            for request in self.requests
            if request["path"].endswith(suffix)
        ]


def task(
    task_id: str,
    activity_type: str,
    *,
    input_value: dict[str, Any] | None = None,
    effective_lease_duration_ms: int = 1000,
) -> dict[str, Any]:
    return {
        "task_id": task_id,
        "activity_key": task_id + "-key",
        "activity_type": activity_type,
        "input": input_value or {"value": task_id},
        "lease_expires_at": "2026-04-13T12:00:00Z",
        "effective_lease_duration_ms": effective_lease_duration_ms,
    }


def test_handler_registration_decorator_dispatch_and_context() -> None:
    client = RecordingClient(
        claims=[[task("task-1", "process_image", input_value={"value": 41})]]
    )
    worker = ActivityWorker(api_key="test-key", worker_id="worker-1", client=client)
    seen_contexts: list[ActivityTaskContext] = []

    @worker.activity("process_image")
    def process_image(payload: dict[str, Any], context: ActivityTaskContext) -> dict[str, Any]:
        seen_contexts.append(context)
        return {"value": payload["value"] + 1}

    try:
        assert worker.poll_once() == 1
        assert worker.wait_for_idle(timeout=1.0)
    finally:
        worker.close()

    assert seen_contexts[0].task_id == "task-1"
    complete_payload = client.payloads_for("/complete")[0]
    assert complete_payload == {"worker_id": "worker-1", "output": {"value": 42}}


def test_concurrency_limit_sets_slot_based_max_tasks_and_avoids_backlog() -> None:
    started = threading.Event()
    release = threading.Event()
    client = RecordingClient(
        claims=[
            [
                task("task-1", "slow"),
                task("task-2", "slow"),
            ]
        ]
    )
    worker = ActivityWorker(
        api_key="test-key",
        worker_id="worker-1",
        client=client,
        concurrency_limit=1,
        poll_interval=0.01,
    )

    def slow(payload: dict[str, Any]) -> dict[str, Any]:
        started.set()
        release.wait(timeout=1.0)
        return payload

    worker.register("slow", slow)
    try:
        assert worker.poll_once() == 1
        assert started.wait(timeout=1.0)
        assert worker.poll_once() == 0
        claim_payloads = client.payloads_for("/claim")
        assert len(claim_payloads) == 1
        assert claim_payloads[0]["max_tasks"] == 1

        release.set()
        assert worker.wait_for_idle(timeout=1.0)
    finally:
        worker.close()

    assert len(client.payloads_for("/complete")) == 1


def test_heartbeat_uses_half_effective_lease_and_successfully_renews() -> None:
    release = threading.Event()
    client = RecordingClient(
        claims=[[task("task-1", "slow", effective_lease_duration_ms=20)]],
        heartbeat_statuses=[200],
    )
    worker = ActivityWorker(
        api_key="test-key",
        worker_id="worker-1",
        client=client,
        concurrency_limit=1,
        poll_interval=0.01,
    )

    def slow(payload: dict[str, Any]) -> dict[str, Any]:
        release.wait(timeout=1.0)
        return payload

    worker.register("slow", slow)
    try:
        assert worker.poll_once() == 1
        time.sleep(0.02)
        assert worker.poll_once() == 0
        release.set()
        assert worker.wait_for_idle(timeout=1.0)
    finally:
        worker.close()

    heartbeat_payload = client.payloads_for("/heartbeat")[0]
    assert heartbeat_payload == {"worker_id": "worker-1"}


@pytest.mark.parametrize("heartbeat_status", [404, 409])
def test_heartbeat_lost_lease_status_marks_context_lost_and_suppresses_terminal_call(
    heartbeat_status: int,
) -> None:
    client = RecordingClient(
        claims=[[task("task-1", "lost", effective_lease_duration_ms=20)]],
        heartbeat_statuses=[heartbeat_status],
    )
    worker = ActivityWorker(
        api_key="test-key",
        worker_id="worker-1",
        client=client,
        concurrency_limit=1,
        poll_interval=0.01,
    )
    observed_lost = threading.Event()

    def lost(payload: dict[str, Any], context: ActivityTaskContext) -> dict[str, Any]:
        deadline = time.monotonic() + 1.0
        while time.monotonic() < deadline:
            if context.lost_lease:
                observed_lost.set()
                return payload
            time.sleep(0.005)
        return payload

    worker.register("lost", lost)
    try:
        assert worker.poll_once() == 1
        time.sleep(0.02)
        assert worker.poll_once() == 0
        assert observed_lost.wait(timeout=1.0)
        assert worker.wait_for_idle(timeout=1.0)
    finally:
        worker.close()

    assert client.payloads_for("/heartbeat")
    assert not client.payloads_for("/complete")
    assert not client.payloads_for("/fail")


def test_error_mapping_for_non_retryable_and_unhandled_exceptions() -> None:
    client = RecordingClient(
        claims=[
            [
                task("task-1", "bad_input"),
                task("task-2", "flaky"),
                task("task-3", "unknown"),
            ]
        ]
    )
    worker = ActivityWorker(
        api_key="test-key",
        worker_id="stable-worker",
        client=client,
        concurrency_limit=3,
    )

    def bad_input(_payload: dict[str, Any]) -> None:
        raise NonRetryableError("invalid input")

    def flaky(_payload: dict[str, Any]) -> None:
        raise ConnectionError("timeout")

    worker.register("bad_input", bad_input)
    worker.register("flaky", flaky)
    try:
        assert worker.poll_once() == 3
        assert worker.wait_for_idle(timeout=1.0)
    finally:
        worker.close()

    fail_payloads = sorted(client.payloads_for("/fail"), key=lambda payload: payload["error_code"])
    assert {
        (payload["error_code"], payload["error_message"], payload["non_retryable"])
        for payload in fail_payloads
    } == {
        ("activity_error", "ConnectionError: timeout", False),
        (
            "activity_not_registered",
            "no handler registered for activity type unknown",
            True,
        ),
        ("non_retryable_error", "invalid input", True),
    }
    worker_ids = {request["json"]["worker_id"] for request in client.requests if request["json"]}
    assert worker_ids == {"stable-worker"}


def test_graceful_shutdown_waits_or_marks_unfinished_tasks_lost() -> None:
    started = threading.Event()
    release = threading.Event()
    client = RecordingClient(claims=[[task("task-1", "slow")]])
    worker = ActivityWorker(
        api_key="test-key",
        worker_id="worker-1",
        client=client,
        concurrency_limit=1,
        poll_interval=0.01,
    )

    def slow(payload: dict[str, Any], context: ActivityTaskContext) -> dict[str, Any]:
        started.set()
        release.wait(timeout=1.0)
        return {"lost": context.lost_lease, "payload": payload}

    worker.register("slow", slow)
    try:
        assert worker.poll_once() == 1
        assert started.wait(timeout=1.0)
        assert not worker.wait_for_idle(timeout=0.01)
        release.set()
        time.sleep(0.02)
        assert worker.wait_for_idle(timeout=1.0)
    finally:
        worker.close()

    assert not client.payloads_for("/complete")
    assert not client.payloads_for("/fail")


def test_coroutine_handlers_are_rejected() -> None:
    worker = ActivityWorker(api_key="test-key", worker_id="worker-1", client=RecordingClient())

    async def async_handler(_payload: dict[str, Any]) -> None:
        return None

    try:
        with pytest.raises(TypeError, match="sync handlers only"):
            worker.register("async", async_handler)
    finally:
        worker.close()


def test_worker_id_generated_once_and_reused() -> None:
    client = RecordingClient(claims=[[task("task-1", "echo")]])
    worker = ActivityWorker(api_key="test-key", client=client)

    def echo(payload: dict[str, Any]) -> dict[str, Any]:
        return payload

    worker.register("echo", echo)
    try:
        generated_worker_id = worker.worker_id
        assert generated_worker_id.startswith("py-")
        assert len(generated_worker_id) <= 128
        assert worker.poll_once() == 1
        assert worker.wait_for_idle(timeout=1.0)
    finally:
        worker.close()

    worker_ids = {request["json"]["worker_id"] for request in client.requests if request["json"]}
    assert worker_ids == {generated_worker_id}
