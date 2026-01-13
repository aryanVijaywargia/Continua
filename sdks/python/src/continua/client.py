"""Continua client for sending traces to the server."""

from __future__ import annotations

import atexit
import uuid
from typing import Any

import httpx

from .batch import BatchQueue


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
    ) -> None:
        """Create a new Continua client.

        Args:
            api_key: API key for authentication
            endpoint: Server endpoint URL
            batch_size: Maximum items before auto-flush
            flush_interval: Seconds between auto-flushes
        """
        self.api_key = api_key
        self.endpoint = endpoint.rstrip("/")
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

        # Register shutdown handler
        atexit.register(self.shutdown)

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
            response = self._client.post("/v1/ingest", json=payload)
            response.raise_for_status()
        except httpx.HTTPError as e:
            # Log but don't raise - we don't want to crash the application
            print(f"Continua: Failed to send batch: {e}")
