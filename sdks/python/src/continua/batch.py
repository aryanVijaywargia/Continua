"""Batch queue for accumulating and sending traces, spans, and events."""

from __future__ import annotations

import threading
from typing import Any, Callable


class BatchQueue:
    """Thread-safe queue for batching observability data.

    Accumulates traces, spans, and events and flushes them either when
    the batch size is reached or on a timer interval.
    """

    def __init__(
        self,
        flush_callback: Callable[
            [list[dict[str, Any]], list[dict[str, Any]], list[dict[str, Any]]], None
        ],
        batch_size: int = 100,
        flush_interval: float = 5.0,
    ) -> None:
        """Create a new BatchQueue.

        Args:
            flush_callback: Function to call when flushing (traces, spans, events)
            batch_size: Maximum items before auto-flush
            flush_interval: Seconds between auto-flushes
        """
        self._flush_callback = flush_callback
        self._batch_size = batch_size
        self._flush_interval = flush_interval

        self._lock = threading.Lock()
        self._traces: list[dict[str, Any]] = []
        self._spans: list[dict[str, Any]] = []
        self._events: list[dict[str, Any]] = []

        self._shutdown = threading.Event()
        self._flush_thread: threading.Thread | None = None

    def start(self) -> None:
        """Start the background flush thread."""
        if self._flush_thread is not None:
            return

        self._flush_thread = threading.Thread(
            target=self._flush_loop,
            daemon=True,
            name="continua-batch-flush",
        )
        self._flush_thread.start()

    def add_trace(self, trace: dict[str, Any]) -> None:
        """Add a trace to the queue."""
        should_flush = False
        with self._lock:
            self._traces.append(trace)
            if self._should_flush():
                should_flush = True
        if should_flush:
            self.flush()

    def add_span(self, span: dict[str, Any]) -> None:
        """Add a span to the queue."""
        should_flush = False
        with self._lock:
            self._spans.append(span)
            if self._should_flush():
                should_flush = True
        if should_flush:
            self.flush()

    def add_event(self, event: dict[str, Any]) -> None:
        """Add an event to the queue."""
        should_flush = False
        with self._lock:
            self._events.append(event)
            if self._should_flush():
                should_flush = True
        if should_flush:
            self.flush()

    def flush(self) -> None:
        """Flush all pending items immediately."""
        # Copy queues under lock, then send outside lock
        with self._lock:
            if not self._traces and not self._spans and not self._events:
                return

            traces = self._traces
            spans = self._spans
            events = self._events

            self._traces = []
            self._spans = []
            self._events = []

        # Send outside the lock to avoid blocking add_* methods
        try:
            self._flush_callback(traces, spans, events)
        except Exception as e:
            print(f"Continua: Flush failed: {e}")

    def shutdown(self) -> None:
        """Shutdown the queue and flush remaining items."""
        self._shutdown.set()
        if self._flush_thread is not None:
            self._flush_thread.join(timeout=5.0)
        self.flush()

    def _should_flush(self) -> bool:
        """Check if we should flush based on batch size."""
        total = len(self._traces) + len(self._spans) + len(self._events)
        return total >= self._batch_size

    def _flush_loop(self) -> None:
        """Background thread that periodically flushes."""
        while not self._shutdown.wait(self._flush_interval):
            self.flush()
