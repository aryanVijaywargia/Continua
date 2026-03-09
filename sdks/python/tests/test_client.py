"""Tests for the Continua client."""

from unittest.mock import MagicMock, patch

import pytest


def test_ingest_adds_to_batch_queue():
    """Test that ingest adds items to the batch queue."""
    with patch("continua.client.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client_class.return_value = mock_client

        from continua import Continua

        # Reset singleton
        Continua._instance = None

        client = Continua(api_key="test-key", endpoint="http://localhost:8080")

        # Add items via ingest (without flush)
        client.ingest(
            traces=[{"trace_id": "t1", "name": "test"}],
            spans=[{"trace_id": "t1", "span_id": "s1", "name": "span1"}],
        )

        # Verify items were added to batch queue (not sent yet)
        assert len(client._batch._traces) == 1
        assert len(client._batch._spans) == 1

        # Verify no HTTP calls were made yet
        mock_client.post.assert_not_called()

        # Cleanup
        client._batch.shutdown()


def test_ingest_with_flush_sends_immediately():
    """Test that ingest with flush=True sends items immediately."""
    with patch("continua.client.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_response = MagicMock()
        mock_response.json.return_value = {"status": "ok", "batch_key": "test"}
        mock_client.post.return_value = mock_response
        mock_client_class.return_value = mock_client

        from continua import Continua

        Continua._instance = None
        client = Continua(api_key="test-key", endpoint="http://localhost:8080")

        # Add items via ingest with flush=True
        client.ingest(
            traces=[{"trace_id": "t1", "name": "test"}],
            spans=[{"trace_id": "t1", "span_id": "s1", "name": "span1"}],
            flush=True,
        )

        # Verify HTTP call was made
        mock_client.post.assert_called_once()
        call_args = mock_client.post.call_args
        assert call_args[0][0] == "/v1/ingest"

        payload = call_args[1]["json"]
        assert len(payload["traces"]) == 1
        assert len(payload["spans"]) == 1
        assert "batch_key" in payload
        assert call_args[1]["params"] is None
        assert call_args[1]["headers"] is None

        # Cleanup
        client._batch.shutdown()


def test_ingest_empty_does_nothing():
    """Test that ingest with no data does nothing."""
    with patch("continua.client.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_client_class.return_value = mock_client

        from continua import Continua

        Continua._instance = None
        client = Continua(api_key="test-key", endpoint="http://localhost:8080")

        # Call ingest with no data
        client.ingest()

        # Should not call the server
        mock_client.post.assert_not_called()

        # Batch queue should be empty
        assert len(client._batch._traces) == 0
        assert len(client._batch._spans) == 0

        client._batch.shutdown()


def test_singleton_pattern():
    """Test that init creates a singleton."""
    with patch("continua.client.httpx.Client"):
        from continua import Continua

        Continua._instance = None

        client1 = Continua.init(api_key="key1", endpoint="http://localhost:8080")
        client2 = Continua.get_instance()

        assert client1 is client2

        client1.shutdown()
        Continua._instance = None


def test_get_instance_raises_without_init():
    """Test that get_instance raises if not initialized."""
    from continua import Continua

    Continua._instance = None

    with pytest.raises(RuntimeError, match="not initialized"):
        Continua.get_instance()


def test_sync_ingest_mode_sets_sync_query_param():
    with patch("continua.client.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_response = MagicMock()
        mock_response.json.return_value = {"status": "ok", "batch_key": "test"}
        mock_client.post.return_value = mock_response
        mock_client_class.return_value = mock_client

        from continua import Continua

        Continua._instance = None
        client = Continua(
            api_key="test-key",
            endpoint="http://localhost:8080",
            ingest_mode="sync",
        )

        client.ingest(traces=[{"trace_id": "t1", "name": "test"}], flush=True)

        call_args = mock_client.post.call_args
        assert call_args[1]["params"] == {"sync": True}
        assert call_args[1]["headers"] is None
        client._batch.shutdown()


def test_async_v2_ingest_mode_sets_header():
    with patch("continua.client.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_response = MagicMock()
        mock_response.json.return_value = {"status": "accepted", "batch_key": "test"}
        mock_client.post.return_value = mock_response
        mock_client_class.return_value = mock_client

        from continua import Continua

        Continua._instance = None
        client = Continua(
            api_key="test-key",
            endpoint="http://localhost:8080",
            ingest_mode="async_v2",
        )

        client.ingest(traces=[{"trace_id": "t1", "name": "test"}], flush=True)

        call_args = mock_client.post.call_args
        assert call_args[1]["params"] is None
        assert call_args[1]["headers"] == {"X-Continua-Async-Version": "2"}
        client._batch.shutdown()


def test_wait_for_batch_returns_terminal_response():
    with patch("continua.client.httpx.Client") as mock_client_class:
        with patch("continua.client.time.sleep"):
            mock_client = MagicMock()
            mock_client.get.side_effect = [
                MagicMock(json=MagicMock(return_value={"status": "accepted"}), status_code=200),
                MagicMock(json=MagicMock(return_value={"status": "processing"}), status_code=200),
                MagicMock(
                    json=MagicMock(return_value={"status": "completed", "batch_id": "b1"}),
                    status_code=200,
                ),
            ]
            mock_client_class.return_value = mock_client

            from continua import Continua

            Continua._instance = None
            client = Continua(api_key="test-key", endpoint="http://localhost:8080")

            result = client.wait_for_batch("batch-1", timeout=5, poll_interval=0.01)

            assert result["status"] == "completed"
            assert mock_client.get.call_count == 3
            client._batch.shutdown()


def test_wait_for_batch_times_out():
    with patch("continua.client.httpx.Client") as mock_client_class:
        with patch("continua.client.time.sleep"):
            with patch("continua.client.time.monotonic", side_effect=[0.0, 0.1, 0.2, 0.3]):
                mock_client = MagicMock()
                mock_client.get.return_value = MagicMock(
                    json=MagicMock(return_value={"status": "processing"}),
                    status_code=200,
                )
                mock_client_class.return_value = mock_client

                from continua import Continua

                Continua._instance = None
                client = Continua(api_key="test-key", endpoint="http://localhost:8080")

                with pytest.raises(TimeoutError, match="Timed out waiting for batch"):
                    client.wait_for_batch("batch-1", timeout=0.15, poll_interval=0.01)

                client._batch.shutdown()


def test_wait_for_batch_not_found_is_clear():
    with patch("continua.client.httpx.Client") as mock_client_class:
        mock_client = MagicMock()
        mock_response = MagicMock(status_code=404)
        mock_response.text = "not found"
        mock_client.get.return_value = mock_response
        mock_client_class.return_value = mock_client

        from continua import Continua
        from continua.exceptions import NetworkError

        Continua._instance = None
        client = Continua(api_key="test-key", endpoint="http://localhost:8080")

        with pytest.raises(NetworkError, match="Batch not found"):
            client.wait_for_batch("batch-1", timeout=1, poll_interval=0.01)

        client._batch.shutdown()
