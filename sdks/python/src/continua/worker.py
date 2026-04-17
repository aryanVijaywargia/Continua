"""Remote activity worker for Continua engine activity tasks."""

from __future__ import annotations

import inspect
import logging
import signal
import threading
import time
import uuid
from collections.abc import Callable
from concurrent.futures import Future, ThreadPoolExecutor
from dataclasses import dataclass
from typing import Any

import httpx

from .exceptions import AuthenticationError, NetworkError, ValidationError

_PREVIEW_HEADERS = {"X-Continua-Engine-Preview": "1"}
_DEFAULT_ENDPOINT = "http://localhost:8080"
_DEFAULT_POLL_INTERVAL = 1.0
_DEFAULT_CONCURRENCY_LIMIT = 5
_DEFAULT_LEASE_DURATION = "60s"
_DEFAULT_SHUTDOWN_TIMEOUT = 30.0
_MAX_ERROR_MESSAGE_LENGTH = 4096

ActivityHandler = Callable[..., Any]


class NonRetryableError(Exception):
    """Raised by handlers to fail an activity without retrying it."""


@dataclass(frozen=True)
class _HandlerRegistration:
    handler: ActivityHandler
    accepts_context: bool


@dataclass(frozen=True)
class _TaskOutcome:
    output: Any | None = None
    error_code: str | None = None
    error_message: str | None = None
    non_retryable: bool = False

    @property
    def succeeded(self) -> bool:
        return self.error_code is None


class ActivityTaskContext:
    """Context passed to remote activity handlers that accept a second argument."""

    def __init__(self, *, task_id: str, activity_key: str, activity_type: str) -> None:
        self.task_id = task_id
        self.activity_key = activity_key
        self.activity_type = activity_type
        self._lost = threading.Event()

    @property
    def lost_lease(self) -> bool:
        return self._lost.is_set()

    @property
    def cancelled(self) -> bool:
        return self._lost.is_set()

    def is_cancelled(self) -> bool:
        return self._lost.is_set()

    def _mark_lost(self) -> None:
        self._lost.set()


@dataclass
class _InFlightTask:
    task: dict[str, Any]
    context: ActivityTaskContext
    future: Future[_TaskOutcome]
    heartbeat_interval: float
    next_heartbeat_at: float


class ActivityWorker:
    """Short-polling remote activity worker for synchronous Python handlers."""

    def __init__(
        self,
        *,
        api_key: str,
        endpoint: str = _DEFAULT_ENDPOINT,
        worker_id: str | None = None,
        poll_interval: float = _DEFAULT_POLL_INTERVAL,
        concurrency_limit: int = _DEFAULT_CONCURRENCY_LIMIT,
        lease_duration: str = _DEFAULT_LEASE_DURATION,
        heartbeat_interval: float | None = None,
        shutdown_timeout: float = _DEFAULT_SHUTDOWN_TIMEOUT,
        client: httpx.Client | None = None,
        logger: logging.Logger | None = None,
    ) -> None:
        if concurrency_limit < 1:
            raise ValueError("concurrency_limit must be at least 1")
        if poll_interval <= 0:
            raise ValueError("poll_interval must be positive")
        if heartbeat_interval is not None and heartbeat_interval <= 0:
            raise ValueError("heartbeat_interval must be positive when provided")
        if shutdown_timeout < 0:
            raise ValueError("shutdown_timeout must be non-negative")

        generated_worker_id = worker_id or f"py-{uuid.uuid4()}"
        if not generated_worker_id.strip() or len(generated_worker_id) > 128:
            raise ValueError("worker_id must be non-empty and at most 128 characters")

        self.api_key = api_key
        self.endpoint = endpoint.rstrip("/")
        self.worker_id = generated_worker_id
        self.poll_interval = poll_interval
        self.concurrency_limit = concurrency_limit
        self.lease_duration = lease_duration
        self.heartbeat_interval = heartbeat_interval
        self.shutdown_timeout = shutdown_timeout
        self._logger = logger or logging.getLogger(__name__)
        self._handlers: dict[str, _HandlerRegistration] = {}
        self._in_flight: dict[str, _InFlightTask] = {}
        self._lock = threading.Lock()
        self._stop_event = threading.Event()
        self._executor = ThreadPoolExecutor(max_workers=concurrency_limit)
        self._owns_client = client is None
        self._client = client or httpx.Client(
            base_url=self.endpoint,
            headers={"X-API-Key": api_key},
            timeout=30.0,
        )

    def register(self, activity_type: str, handler_fn: ActivityHandler) -> ActivityHandler:
        """Register a synchronous handler for an activity type."""
        activity_type = activity_type.strip()
        if not activity_type:
            raise ValueError("activity_type must be non-empty")
        if inspect.iscoroutinefunction(handler_fn):
            raise TypeError("remote activity worker v1 supports sync handlers only")
        accepts_context = _handler_accepts_context(handler_fn)
        self._handlers[activity_type] = _HandlerRegistration(
            handler=handler_fn,
            accepts_context=accepts_context,
        )
        return handler_fn

    def activity(self, activity_type: str) -> Callable[[ActivityHandler], ActivityHandler]:
        """Decorator form for registering an activity handler."""

        def decorator(handler_fn: ActivityHandler) -> ActivityHandler:
            return self.register(activity_type, handler_fn)

        return decorator

    def stop(self) -> None:
        """Request a graceful shutdown."""
        self._stop_event.set()

    def close(self) -> None:
        """Close worker resources."""
        self._executor.shutdown(wait=False, cancel_futures=True)
        if self._owns_client:
            self._client.close()

    def run(self) -> None:
        """Run the blocking polling loop until stopped or signalled."""
        previous_handlers = self._install_signal_handlers()
        try:
            while not self._stop_event.is_set():
                self.poll_once()
                time.sleep(self.poll_interval)
            self.wait_for_idle(timeout=self.shutdown_timeout)
        finally:
            self._restore_signal_handlers(previous_handlers)
            self.close()

    def poll_once(self) -> int:
        """Run one poll/dispatch/heartbeat/drain iteration.

        Returns the number of tasks claimed in this iteration.
        """
        self._tick_heartbeats()
        self._drain_finished()
        if self._stop_event.is_set():
            return 0

        activity_types = sorted(self._handlers.keys())
        if not activity_types:
            return 0

        available_slots = self._available_slots()
        if available_slots <= 0:
            return 0

        response = self._request(
            "POST",
            "/v1/engine/activities/claim",
            json={
                "worker_id": self.worker_id,
                "activity_types": activity_types,
                "lease_duration": self.lease_duration,
                "max_tasks": available_slots,
            },
        )
        payload = response.json()
        tasks = payload.get("tasks", [])
        for task in tasks[:available_slots]:
            self._dispatch_task(task)
        return len(tasks[:available_slots])

    def wait_for_idle(self, *, timeout: float | None = None) -> bool:
        """Wait for in-flight handlers to finish while continuing heartbeats."""
        deadline = None if timeout is None else time.monotonic() + timeout
        while True:
            self._tick_heartbeats()
            self._drain_finished()
            if self._in_flight_count() == 0:
                return True
            if deadline is not None and time.monotonic() >= deadline:
                self._mark_all_in_flight_lost()
                return False
            time.sleep(min(self.poll_interval, 0.05))

    def _dispatch_task(self, task: dict[str, Any]) -> None:
        task_id = str(task["task_id"])
        context = ActivityTaskContext(
            task_id=task_id,
            activity_key=str(task["activity_key"]),
            activity_type=str(task["activity_type"]),
        )
        heartbeat_interval = self._heartbeat_interval_for_task(task)
        future = self._executor.submit(self._execute_task, task, context)
        with self._lock:
            self._in_flight[task_id] = _InFlightTask(
                task=task,
                context=context,
                future=future,
                heartbeat_interval=heartbeat_interval,
                next_heartbeat_at=time.monotonic() + heartbeat_interval,
            )

    def _execute_task(
        self,
        task: dict[str, Any],
        context: ActivityTaskContext,
    ) -> _TaskOutcome:
        registration = self._handlers.get(str(task["activity_type"]))
        if registration is None:
            return _TaskOutcome(
                error_code="activity_not_registered",
                error_message=f"no handler registered for activity type {task['activity_type']}",
                non_retryable=True,
            )

        try:
            if registration.accepts_context:
                output = registration.handler(task.get("input"), context)
            else:
                output = registration.handler(task.get("input"))
            return _TaskOutcome(output=output)
        except NonRetryableError as exc:
            return _TaskOutcome(
                error_code="non_retryable_error",
                error_message=_truncate(str(exc), _MAX_ERROR_MESSAGE_LENGTH),
                non_retryable=True,
            )
        except Exception as exc:  # noqa: BLE001 - handler exceptions are reported to the engine.
            self._logger.exception("Remote activity handler failed")
            return _TaskOutcome(
                error_code="activity_error",
                error_message=_truncate(f"{type(exc).__name__}: {exc}", _MAX_ERROR_MESSAGE_LENGTH),
                non_retryable=False,
            )

    def _tick_heartbeats(self) -> None:
        now = time.monotonic()
        with self._lock:
            due = [
                item
                for item in self._in_flight.values()
                if not item.future.done()
                and not item.context.lost_lease
                and now >= item.next_heartbeat_at
            ]

        for item in due:
            response = self._request(
                "POST",
                f"/v1/engine/activities/{item.context.task_id}/heartbeat",
                json={"worker_id": self.worker_id},
                raise_for_status=False,
            )
            if response.status_code in {404, 409}:
                item.context._mark_lost()
                continue
            self._raise_for_response(response)
            payload = response.json()
            item.heartbeat_interval = self._heartbeat_interval_from_ms(
                payload.get("effective_lease_duration_ms")
            )
            item.next_heartbeat_at = time.monotonic() + item.heartbeat_interval

    def _drain_finished(self) -> None:
        with self._lock:
            finished = [
                (task_id, item)
                for task_id, item in self._in_flight.items()
                if item.future.done()
            ]
            for task_id, _ in finished:
                self._in_flight.pop(task_id, None)

        for _, item in finished:
            if item.context.lost_lease:
                continue
            outcome = item.future.result()
            if outcome.succeeded:
                self._complete_task(item, outcome.output)
            else:
                self._fail_task(item, outcome)

    def _complete_task(self, item: _InFlightTask, output: Any) -> None:
        response = self._request(
            "POST",
            f"/v1/engine/activities/{item.context.task_id}/complete",
            json={"worker_id": self.worker_id, "output": output},
            raise_for_status=False,
        )
        if response.status_code in {404, 409}:
            item.context._mark_lost()
            return
        self._raise_for_response(response)

    def _fail_task(self, item: _InFlightTask, outcome: _TaskOutcome) -> None:
        response = self._request(
            "POST",
            f"/v1/engine/activities/{item.context.task_id}/fail",
            json={
                "worker_id": self.worker_id,
                "error_code": outcome.error_code,
                "error_message": outcome.error_message,
                "non_retryable": outcome.non_retryable,
            },
            raise_for_status=False,
        )
        if response.status_code in {404, 409}:
            item.context._mark_lost()
            return
        self._raise_for_response(response)

    def _request(
        self,
        method: str,
        path: str,
        *,
        json: dict[str, Any] | None = None,
        raise_for_status: bool = True,
    ) -> httpx.Response:
        try:
            response = self._client.request(
                method,
                path,
                json=json,
                headers=_PREVIEW_HEADERS,
            )
        except httpx.RequestError as exc:
            raise NetworkError("Network request failed", cause=exc) from exc

        if raise_for_status:
            self._raise_for_response(response)
        return response

    def _raise_for_response(self, response: httpx.Response) -> None:
        if 200 <= response.status_code < 300:
            return

        payload: dict[str, Any] = {}
        try:
            payload = response.json()
        except ValueError:
            payload = {}

        message = str(payload.get("message") or response.text or "Request failed")
        if response.status_code == 401:
            raise AuthenticationError(message)
        if response.status_code == 400:
            raise ValidationError("Validation error", message)
        raise NetworkError(
            f"Remote activity worker request failed with status {response.status_code}"
        )

    def _available_slots(self) -> int:
        return self.concurrency_limit - self._in_flight_count()

    def _in_flight_count(self) -> int:
        with self._lock:
            return len(self._in_flight)

    def _mark_all_in_flight_lost(self) -> None:
        with self._lock:
            for item in self._in_flight.values():
                item.context._mark_lost()

    def _heartbeat_interval_for_task(self, task: dict[str, Any]) -> float:
        if self.heartbeat_interval is not None:
            return self.heartbeat_interval
        return self._heartbeat_interval_from_ms(task.get("effective_lease_duration_ms"))

    @staticmethod
    def _heartbeat_interval_from_ms(effective_lease_duration_ms: object) -> float:
        if not isinstance(effective_lease_duration_ms, int) or effective_lease_duration_ms <= 0:
            return _DEFAULT_POLL_INTERVAL
        return max(effective_lease_duration_ms / 2000.0, 0.001)

    def _install_signal_handlers(
        self,
    ) -> dict[signal.Signals, signal.Handlers] | None:
        if threading.current_thread() is not threading.main_thread():
            return None

        previous: dict[signal.Signals, signal.Handlers] = {}
        for sig in (signal.SIGINT, signal.SIGTERM):
            previous[sig] = signal.getsignal(sig)

            def handler(_signum: int, _frame: object) -> None:
                self.stop()

            signal.signal(sig, handler)
        return previous

    @staticmethod
    def _restore_signal_handlers(
        previous_handlers: dict[signal.Signals, signal.Handlers] | None,
    ) -> None:
        if previous_handlers is None:
            return
        for sig, handler in previous_handlers.items():
            signal.signal(sig, handler)


def _handler_accepts_context(handler_fn: ActivityHandler) -> bool:
    signature = inspect.signature(handler_fn)
    parameters = list(signature.parameters.values())
    if any(param.kind == inspect.Parameter.VAR_POSITIONAL for param in parameters):
        return True

    positional = [
        param
        for param in parameters
        if param.kind
        in {
            inspect.Parameter.POSITIONAL_ONLY,
            inspect.Parameter.POSITIONAL_OR_KEYWORD,
        }
    ]
    if len(positional) < 1:
        raise TypeError("activity handlers must accept at least the input argument")
    return len(positional) >= 2


def _truncate(value: str, max_length: int) -> str:
    if len(value) <= max_length:
        return value
    return value[:max_length]
