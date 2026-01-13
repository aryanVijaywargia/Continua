"""Tests for batch queue."""

from unittest.mock import MagicMock

from continua.batch import BatchQueue


def test_batch_queue_accumulates():
    """Test that batch queue accumulates items."""
    callback = MagicMock()
    queue = BatchQueue(flush_callback=callback, batch_size=10)

    queue.add_trace({"trace_id": "1"})
    queue.add_span({"span_id": "1"})
    queue.add_event({"event_id": "1"})

    # Not flushed yet (below batch size)
    callback.assert_not_called()


def test_batch_queue_flushes_on_size():
    """Test that batch queue flushes when size is reached."""
    callback = MagicMock()
    queue = BatchQueue(flush_callback=callback, batch_size=3)

    queue.add_trace({"trace_id": "1"})
    queue.add_span({"span_id": "1"})

    # Not yet at size
    callback.assert_not_called()

    # This should trigger flush
    queue.add_event({"event_id": "1"})
    callback.assert_called_once()


def test_batch_queue_manual_flush():
    """Test manual flush."""
    callback = MagicMock()
    queue = BatchQueue(flush_callback=callback, batch_size=100)

    queue.add_trace({"trace_id": "1"})
    queue.add_span({"span_id": "1"})

    queue.flush()

    callback.assert_called_once_with(
        [{"trace_id": "1"}],
        [{"span_id": "1"}],
        [],
    )


def test_batch_queue_empty_flush():
    """Test that empty flush doesn't call callback."""
    callback = MagicMock()
    queue = BatchQueue(flush_callback=callback, batch_size=100)

    queue.flush()
    callback.assert_not_called()


def test_batch_queue_shutdown():
    """Test shutdown flushes remaining items."""
    callback = MagicMock()
    queue = BatchQueue(flush_callback=callback, batch_size=100)

    queue.add_trace({"trace_id": "1"})

    queue.shutdown()

    callback.assert_called_once()
