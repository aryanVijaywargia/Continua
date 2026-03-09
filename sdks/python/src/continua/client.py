"""Continua client for sending traces to the server."""

from __future__ import annotations

import atexit
import random
import time
import uuid
from typing import Any

import httpx

from .batch import BatchQueue
from .exceptions import (
    AuthenticationError,
    NetworkError,
    RateLimitError,
    ValidationError,
)

# Module-level reference for shutdown handler
_current_client: Continua | None = None
_atexit_registered: bool = False

# Retry configuration
DEFAULT_MAX_RETRIES = 3
DEFAULT_BASE_DELAY = 1.0  # seconds
DEFAULT_MAX_DELAY = 30.0  # seconds
DEFAULT_BATCH_POLL_INTERVAL = 1.0  # seconds

INGEST_MODE_SYNC = "sync"
INGEST_MODE_ASYNC_V2 = "async_v2"
INGEST_MODE_SERVER_DEFAULT = "server_default"
VALID_INGEST_MODES = {
    INGEST_MODE_SYNC,
    INGEST_MODE_ASYNC_V2,
    INGEST_MODE_SERVER_DEFAULT,
}


def _module_shutdown() -> None:
    """Module-level shutdown handler called once at exit."""
    global _current_client
    if _current_client is not None:
        _current_client._do_shutdown()
        _current_client = None


class Continua:
    """Main client for the Continua observability platform.

    Provides a singleton pattern for easy access across the application.

    Example:
        # Initialize once at application startup
        Continua.init(api_key="your_api_key")

        # Use decorators and context managers throughout your code
        @trace
        def my_agent():
            with span("llm_call", kind="llm") as s:
                # do work
                s.set_output({"result": "..."})
    """

    _instance: Continua | None = None

    def __init__(
        self,
        api_key: str,
        endpoint: str = "http://localhost:8080",
        *,
        batch_size: int = 100,
        flush_interval: float = 5.0,
        max_retries: int = DEFAULT_MAX_RETRIES,
        base_delay: float = DEFAULT_BASE_DELAY,
        max_delay: float = DEFAULT_MAX_DELAY,
        ingest_mode: str = INGEST_MODE_SERVER_DEFAULT,
    ) -> None:
        """Create a new Continua client.

        Args:
            api_key: API key for authentication
            endpoint: Server endpoint URL
            batch_size: Maximum items before auto-flush
            flush_interval: Seconds between auto-flushes
            max_retries: Maximum retry attempts for transient errors
            base_delay: Base delay for exponential backoff (seconds)
            max_delay: Maximum delay between retries (seconds)
            ingest_mode: Ingest behavior. One of "sync", "async_v2", or
                "server_default"
        """
        global _current_client, _atexit_registered

        if ingest_mode not in VALID_INGEST_MODES:
            msg = (
                f"Invalid ingest_mode {ingest_mode!r}. "
                f"Expected one of {sorted(VALID_INGEST_MODES)!r}"
            )
            raise ValueError(msg)

        self.api_key = api_key
        self.endpoint = endpoint.rstrip("/")
        self.max_retries = max_retries
        self.base_delay = base_delay
        self.max_delay = max_delay
        self.ingest_mode = ingest_mode

        self._client = httpx.Client(
            base_url=self.endpoint,
            headers={"X-API-Key": api_key},
            timeout=30.0,
        )
        self._batch = BatchQueue(
            flush_callback=self._send_batch,
            batch_size=batch_size,
            flush_interval=flush_interval,
        )
        self._batch.start()

        # Register module-level shutdown handler once
        _current_client = self
        if not _atexit_registered:
            atexit.register(_module_shutdown)
            _atexit_registered = True

    @classmethod
    def init(
        cls,
        api_key: str,
        endpoint: str = "http://localhost:8080",
        **kwargs: Any,
    ) -> Continua:
        """Initialize the global Continua client.

        Args:
            api_key: API key for authentication
            endpoint: Server endpoint URL
            **kwargs: Additional arguments passed to __init__

        Returns:
            The global Continua instance
        """
        if cls._instance is not None:
            cls._instance.shutdown()
        cls._instance = cls(api_key, endpoint, **kwargs)
        return cls._instance

    @classmethod
    def get_instance(cls) -> Continua:
        """Get the global Continua instance.

        Raises:
            RuntimeError: If Continua.init() hasn't been called
        """
        if cls._instance is None:
            raise RuntimeError("Continua not initialized. Call Continua.init() first.")
        return cls._instance

    def add_trace(self, trace: dict[str, Any]) -> None:
        """Add a trace to the batch queue."""
        self._batch.add_trace(trace)

    def add_span(self, span: dict[str, Any]) -> None:
        """Add a span to the batch queue."""
        self._batch.add_span(span)

    def add_event(self, event: dict[str, Any]) -> None:
        """Add an event to the batch queue."""
        self._batch.add_event(event)

    def flush(self) -> None:
        """Flush all pending items immediately."""
        self._batch.flush()

    def wait_for_batch(
        self,
        batch_id: str,
        timeout: float = 30.0,
        poll_interval: float = DEFAULT_BATCH_POLL_INTERVAL,
    ) -> dict[str, Any]:
        """Poll batch status until it reaches a terminal state or times out."""
        deadline = time.monotonic() + timeout

        while True:
            response = self._request_with_retry(
                "GET",
                f"/v1/ingest/batches/{batch_id}",
            )
            data = response.json()
            if data.get("status") in {"completed", "failed"}:
                return data

            if time.monotonic() >= deadline:
                raise TimeoutError(f"Timed out waiting for batch {batch_id}")

            time.sleep(poll_interval)

    def ingest(
        self,
        *,
        traces: list[dict[str, Any]] | None = None,
        spans: list[dict[str, Any]] | None = None,
        events: list[dict[str, Any]] | None = None,
        flush: bool = False,
    ) -> None:
        """Add traces, spans, and events to the batch queue for ingestion.

        This method adds items to the batch queue, which will be flushed
        automatically based on batch_size and flush_interval settings.
        Use flush=True for immediate send (useful for dev scripts).

        Args:
            traces: List of trace dictionaries to ingest
            spans: List of span dictionaries to ingest
            events: List of event dictionaries to ingest
            flush: If True, flush immediately after adding items

        Example:
            # Add to batch queue (will be flushed automatically)
            client.ingest(
                traces=[{"trace_id": "t1", "name": "my_trace"}],
                spans=[{"trace_id": "t1", "span_id": "s1", "name": "my_span"}],
            )

            # Add and flush immediately
            client.ingest(traces=[...], flush=True)
        """
        for trace in traces or []:
            self._batch.add_trace(trace)
        for span in spans or []:
            self._batch.add_span(span)
        for event in events or []:
            self._batch.add_event(event)

        if flush:
            self.flush()

    def shutdown(self) -> None:
        """Shutdown the client gracefully."""
        global _current_client
        self._do_shutdown()
        if _current_client is self:
            _current_client = None

    def _do_shutdown(self) -> None:
        """Internal shutdown logic."""
        self._batch.shutdown()
        self._client.close()

    def _send_batch(
        self,
        traces: list[dict[str, Any]],
        spans: list[dict[str, Any]],
        events: list[dict[str, Any]],
    ) -> None:
        """Send a batch of traces, spans, and events to the server."""
        if not traces and not spans and not events:
            return

        batch_key = str(uuid.uuid4())
        payload: dict[str, Any] = {"batch_key": batch_key}

        if traces:
            payload["traces"] = traces
        if spans:
            payload["spans"] = spans
        if events:
            payload["events"] = events

        try:
            params, headers = self._ingest_request_options()
            self._send_with_retry(
                "/v1/ingest",
                payload,
                params=params,
                headers=headers,
            )
        except (AuthenticationError, ValidationError, RateLimitError, NetworkError) as e:
            # Log but don't raise - we don't want to crash the application
            print(f"Continua: Failed to send batch: {e}")

    def _send_with_retry(
        self,
        path: str,
        payload: dict[str, Any],
        *,
        params: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> httpx.Response:
        """Send a POST request with retry behavior."""
        return self._request_with_retry(
            "POST",
            path,
            json_payload=payload,
            params=params,
            headers=headers,
        )

    def _request_with_retry(
        self,
        method: str,
        path: str,
        *,
        json_payload: dict[str, Any] | None = None,
        params: dict[str, Any] | None = None,
        headers: dict[str, str] | None = None,
    ) -> httpx.Response:
        """Send a request with exponential backoff retry.

        Args:
            method: HTTP method to call
            path: API path to call
            json_payload: JSON payload to send for POST requests
            params: Optional query parameters
            headers: Optional per-request headers

        Returns:
            The successful response

        Raises:
            AuthenticationError: On 401 (no retry)
            RateLimitError: On 429 (no retry in this version)
            ValidationError: On 400 (no retry)
            NetworkError: After all retries exhausted
        """
        last_exception: Exception | None = None

        for attempt in range(self.max_retries + 1):
            try:
                if method == "POST":
                    response = self._client.post(
                        path,
                        json=json_payload,
                        params=params,
                        headers=headers,
                    )
                elif method == "GET":
                    response = self._client.get(
                        path,
                        params=params,
                        headers=headers,
                    )
                else:
                    response = self._client.request(
                        method,
                        path,
                        json=json_payload,
                        params=params,
                        headers=headers,
                    )

                # Handle specific HTTP error codes
                if response.status_code == 401:
                    raise AuthenticationError("Invalid or missing API key")

                if response.status_code == 429:
                    retry_after = response.headers.get("Retry-After")
                    retry_seconds = int(retry_after) if retry_after else None
                    raise RateLimitError(
                        "Rate limit exceeded", retry_after=retry_seconds
                    )

                if response.status_code == 400:
                    try:
                        error_body = response.json()
                        details = error_body.get("message", str(error_body))
                    except Exception:
                        details = response.text
                    raise ValidationError("Request validation failed", details=details)

                if (
                    response.status_code == 404
                    and method == "GET"
                    and path.startswith("/v1/ingest/batches/")
                ):
                    raise NetworkError("Batch not found", retry_count=attempt)

                response.raise_for_status()
                return response

            except (AuthenticationError, RateLimitError, ValidationError):
                # Don't retry these errors
                raise

            except (httpx.ConnectError, httpx.TimeoutException) as e:
                # Transient errors - retry with backoff
                last_exception = e
                if attempt < self.max_retries:
                    delay = self._calculate_backoff(attempt)
                    time.sleep(delay)
                continue

            except httpx.HTTPStatusError as e:
                # 5xx errors - retry with backoff
                if e.response.status_code >= 500:
                    last_exception = e
                    if attempt < self.max_retries:
                        delay = self._calculate_backoff(attempt)
                        time.sleep(delay)
                    continue
                # Other HTTP errors - don't retry
                raise NetworkError(
                    f"HTTP error {e.response.status_code}",
                    retry_count=attempt,
                    cause=e,
                )

        # All retries exhausted
        raise NetworkError(
            "Request failed after retries",
            retry_count=self.max_retries,
            cause=last_exception,
        )

    def _ingest_request_options(self) -> tuple[dict[str, Any] | None, dict[str, str] | None]:
        if self.ingest_mode == INGEST_MODE_SYNC:
            return {"sync": True}, None
        if self.ingest_mode == INGEST_MODE_ASYNC_V2:
            return None, {"X-Continua-Async-Version": "2"}
        return None, None

    def _calculate_backoff(self, attempt: int) -> float:
        """Calculate exponential backoff delay with jitter.

        Args:
            attempt: The attempt number (0-indexed)

        Returns:
            Delay in seconds
        """
        # Exponential backoff: base_delay * 2^attempt
        delay = self.base_delay * (2**attempt)
        # Add random jitter (0-1 second)
        jitter = random.random()
        delay += jitter
        # Cap at max_delay
        return min(delay, self.max_delay)
