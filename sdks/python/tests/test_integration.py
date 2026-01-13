"""Integration tests for the Continua Python SDK.

These tests require a running Continua server at http://localhost:8081.
They are skipped when the server is not available.

To run these tests:
1. Start PostgreSQL: make dev
2. Run migrations: make migrate
3. Start the server: DATABASE_URL="postgres://continua:continua@localhost:5432/continua?sslmode=disable" PORT=8081 go run ./cmd/continua serve
4. Run tests: cd sdks/python && uv run pytest tests/test_integration.py -v
"""

from __future__ import annotations

import os
import time
import uuid

import httpx
import pytest

from continua import Continua, span, trace

# Server configuration
CONTINUA_ENDPOINT = os.environ.get("CONTINUA_ENDPOINT", "http://localhost:8081")
CONTINUA_API_KEY = os.environ.get("CONTINUA_API_KEY", "test-api-key-12345")


def server_available() -> bool:
    """Check if the Continua server is running."""
    try:
        response = httpx.get(f"{CONTINUA_ENDPOINT}/api/health", timeout=2.0)
        return response.status_code == 200
    except httpx.RequestError:
        return False


# Skip all tests in this module if server is not available
pytestmark = pytest.mark.skipif(
    not server_available(),
    reason=f"Continua server not available at {CONTINUA_ENDPOINT}",
)


@pytest.fixture(autouse=True)
def reset_client():
    """Reset the global Continua client before each test."""
    # Reset singleton if it exists
    if Continua._instance is not None:
        try:
            Continua._instance.shutdown()
        except Exception:
            pass
        Continua._instance = None
    yield
    # Cleanup after test
    if Continua._instance is not None:
        try:
            Continua._instance.shutdown()
        except Exception:
            pass
        Continua._instance = None


class TestIntegration:
    """Integration tests that require a running server."""

    def test_init_and_shutdown(self):
        """Test basic client initialization and shutdown."""
        client = Continua.init(
            api_key=CONTINUA_API_KEY,
            endpoint=CONTINUA_ENDPOINT,
        )

        assert client is not None
        assert Continua.get_instance() is client

        client.shutdown()

    def test_trace_decorator_creates_trace(self):
        """Test that @trace decorator creates and sends a trace."""
        Continua.init(
            api_key=CONTINUA_API_KEY,
            endpoint=CONTINUA_ENDPOINT,
            batch_size=1,  # Flush immediately
            flush_interval=0.1,
        )

        trace_name = f"test-trace-{uuid.uuid4()}"

        @trace(name=trace_name)
        def my_function():
            return "result"

        result = my_function()
        assert result == "result"

        # Wait for flush
        time.sleep(0.5)
        Continua.get_instance().flush()

    def test_span_context_manager(self):
        """Test span context manager creates spans."""
        Continua.init(
            api_key=CONTINUA_API_KEY,
            endpoint=CONTINUA_ENDPOINT,
            batch_size=1,
            flush_interval=0.1,
        )

        @trace
        def my_agent():
            with span("llm_call", kind="llm") as s:
                s.set_model("gpt-4")
                s.set_tokens(prompt=100, completion=50)
                s.set_output({"response": "Hello!"})

            with span("tool_call", kind="tool") as s:
                s.set_input({"query": "search term"})
                s.set_output({"results": ["a", "b", "c"]})

        my_agent()

        # Wait for flush
        time.sleep(0.5)
        Continua.get_instance().flush()

    def test_nested_spans(self):
        """Test nested span hierarchy."""
        Continua.init(
            api_key=CONTINUA_API_KEY,
            endpoint=CONTINUA_ENDPOINT,
            batch_size=10,
            flush_interval=0.1,
        )

        @trace
        def nested_agent():
            with span("outer", kind="chain") as outer:
                outer.set_input({"step": "outer"})

                with span("inner", kind="llm") as inner:
                    inner.set_model("gpt-4")
                    inner.set_output({"result": "done"})

                outer.set_output({"completed": True})

        nested_agent()

        # Explicit flush
        Continua.get_instance().flush()

    def test_manual_trace_and_span(self):
        """Test manual trace and span creation via add_trace/add_span."""
        client = Continua.init(
            api_key=CONTINUA_API_KEY,
            endpoint=CONTINUA_ENDPOINT,
        )

        trace_id = f"manual-trace-{uuid.uuid4()}"

        client.add_trace({
            "trace_id": trace_id,
            "name": "manual_test_trace",
            "status": "running",
        })

        client.add_span({
            "trace_id": trace_id,
            "span_id": f"span-{uuid.uuid4()}",
            "name": "manual_span",
            "type": "custom",
            "status": "completed",
        })

        client.flush()

    def test_ingest_method(self):
        """Test the ingest() method with flush=True for immediate send."""
        client = Continua.init(
            api_key=CONTINUA_API_KEY,
            endpoint=CONTINUA_ENDPOINT,
        )

        trace_id = f"ingest-method-trace-{uuid.uuid4()}"

        # Use ingest with flush=True for immediate send
        client.ingest(
            traces=[{
                "trace_id": trace_id,
                "name": "ingest_method_test",
                "status": "completed",
            }],
            spans=[{
                "trace_id": trace_id,
                "span_id": f"span-{uuid.uuid4()}",
                "name": "ingest_method_span",
                "type": "llm",
                "status": "completed",
                "total_tokens": 100,
            }],
            flush=True,
        )

        # Give server time to process
        time.sleep(0.3)

    def test_ingest_endpoint_directly(self):
        """Test direct HTTP call to ingest endpoint."""
        batch_key = f"integration-test-{uuid.uuid4()}"
        trace_id = f"direct-trace-{uuid.uuid4()}"

        payload = {
            "batch_key": batch_key,
            "traces": [{
                "trace_id": trace_id,
                "name": "direct_ingest_test",
                "status": "completed",
            }],
            "spans": [{
                "trace_id": trace_id,
                "span_id": f"span-{uuid.uuid4()}",
                "name": "direct_span",
                "type": "llm",
                "status": "completed",
                "total_tokens": 150,
                "total_cost": 0.01,
            }],
        }

        response = httpx.post(
            f"{CONTINUA_ENDPOINT}/v1/ingest",
            json=payload,
            headers={"X-API-Key": CONTINUA_API_KEY},
        )

        assert response.status_code in (200, 202)
        data = response.json()
        assert data["status"] in ("ok", "accepted")
        assert data["batch_key"] == batch_key

    def test_list_traces_endpoint(self):
        """Test that we can list traces after ingesting."""
        response = httpx.get(
            f"{CONTINUA_ENDPOINT}/api/traces",
            params={"limit": 10, "offset": 0},
            headers={"X-API-Key": CONTINUA_API_KEY},
        )

        assert response.status_code == 200
        data = response.json()
        assert "traces" in data
        assert "total" in data
        assert isinstance(data["traces"], list)

    def test_duplicate_batch_returns_duplicate_status(self):
        """Test that duplicate batch keys are handled correctly."""
        batch_key = f"duplicate-test-{uuid.uuid4()}"
        trace_id = f"dup-trace-{uuid.uuid4()}"

        payload = {
            "batch_key": batch_key,
            "traces": [{
                "trace_id": trace_id,
                "name": "duplicate_test",
                "status": "completed",
            }],
        }

        # First request
        response1 = httpx.post(
            f"{CONTINUA_ENDPOINT}/v1/ingest",
            json=payload,
            headers={"X-API-Key": CONTINUA_API_KEY},
        )
        assert response1.status_code in (200, 202)

        # Second request with same batch_key
        response2 = httpx.post(
            f"{CONTINUA_ENDPOINT}/v1/ingest",
            json=payload,
            headers={"X-API-Key": CONTINUA_API_KEY},
        )
        assert response2.status_code in (200, 202)
        data2 = response2.json()
        assert data2["status"] == "duplicate"

    def test_auth_required(self):
        """Test that requests without API key are rejected."""
        response = httpx.get(f"{CONTINUA_ENDPOINT}/api/traces")
        assert response.status_code == 401

    def test_invalid_api_key_rejected(self):
        """Test that invalid API keys are rejected."""
        response = httpx.get(
            f"{CONTINUA_ENDPOINT}/api/traces",
            headers={"X-API-Key": "invalid-key"},
        )
        assert response.status_code == 401

    def test_health_endpoint_public(self):
        """Test that health endpoint is publicly accessible."""
        response = httpx.get(f"{CONTINUA_ENDPOINT}/api/health")
        assert response.status_code == 200
        data = response.json()
        assert data["status"] == "ok"
