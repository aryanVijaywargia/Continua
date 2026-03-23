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
from datetime import datetime, timezone
from typing import Any

import httpx
import pytest

from continua import Continua, span, trace
from continua.trace import TraceContext
from tests.support import parse_rfc3339_timestamp

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
    if Continua._instance is not None:
        try:
            Continua._instance.shutdown()
        except Exception:
            pass
        Continua._instance = None


def api_headers() -> dict[str, str]:
    """Build auth headers for debugger API calls."""
    return {"X-API-Key": CONTINUA_API_KEY}


def wait_for_trace_record(trace_name: str, timeout: float = 5.0) -> dict[str, Any]:
    """Poll the traces list until a trace with the requested name appears."""
    deadline = time.monotonic() + timeout
    params = {
        "limit": 50,
        "offset": 0,
        "sort_by": "started_at",
        "sort_dir": "desc",
    }

    while True:
        response = httpx.get(
            f"{CONTINUA_ENDPOINT}/api/traces",
            params=params,
            headers=api_headers(),
            timeout=5.0,
        )
        response.raise_for_status()
        for trace_record in response.json()["traces"]:
            if trace_record["name"] == trace_name:
                return trace_record

        if time.monotonic() >= deadline:
            raise AssertionError(f"Timed out waiting for trace {trace_name!r}")

        time.sleep(0.1)


def fetch_timeline(trace_db_id: str) -> dict[str, Any]:
    """Fetch the timeline for a trace using a single large page."""
    response = httpx.get(
        f"{CONTINUA_ENDPOINT}/api/traces/{trace_db_id}/events",
        params={"limit": 100},
        headers=api_headers(),
        timeout=5.0,
    )
    response.raise_for_status()
    return response.json()


def explicit_events_for_span(
    timeline_events: list[dict[str, Any]],
    span_id: str,
    event_type: str,
) -> list[dict[str, Any]]:
    """Filter explicit timeline events for a specific span and type."""
    return [
        event
        for event in timeline_events
        if event.get("span_id") == span_id
        and event["source"] == "explicit"
        and event["event_type"] == event_type
    ]


def assert_timeline_sequence(event: dict[str, Any], expected_sequence: int) -> None:
    """Assert timeline metadata round-trips from the SDK."""
    assert event["sequence"] == expected_sequence
    assert event["timestamp"] is not None
    parse_rfc3339_timestamp(event["timestamp"])


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

        @trace()
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

        @trace()
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
                "prompt_tokens": 60,
                "completion_tokens": 40,
            }],
            flush=True,
        )

        # Give server time to process
        time.sleep(0.3)

    def test_ingest_endpoint_directly(self):
        """Test direct HTTP call to ingest endpoint."""
        batch_key = f"integration-test-{uuid.uuid4()}"
        trace_id = f"direct-trace-{uuid.uuid4()}"
        start_time = datetime.now(timezone.utc).isoformat()

        payload = {
            "batch_key": batch_key,
            "traces": [{
                "trace_id": trace_id,
                "name": "direct_ingest_test",
                "status": "completed",
                "start_time": start_time,
            }],
            "spans": [{
                "trace_id": trace_id,
                "span_id": f"span-{uuid.uuid4()}",
                "name": "direct_span",
                "type": "llm",
                "status": "completed",
                "start_time": start_time,
                "prompt_tokens": 90,
                "completion_tokens": 60,
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

    def test_sync_llm_effect_round_trip(self):
        """Implicit LLM effect events round-trip through the timeline API."""
        client = Continua.init(
            api_key=CONTINUA_API_KEY,
            endpoint=CONTINUA_ENDPOINT,
            batch_size=100,
            flush_interval=60.0,
            ingest_mode="sync",
        )
        trace_name = f"sdk-llm-effect-{uuid.uuid4()}"

        with TraceContext(name=trace_name):
            with span("llm_effect", kind="llm") as effect_span:
                effect_span.set_llm_response(
                    model="gpt-4.1-mini",
                    messages=[{"role": "user", "content": "Hello"}],
                    response={"role": "assistant", "content": "Hi"},
                    tokens_in=12,
                    tokens_out=7,
                )
                span_id = effect_span.span_id

        client.flush()
        trace_record = wait_for_trace_record(trace_name)
        timeline = fetch_timeline(trace_record["id"])
        span_events = [
            event for event in timeline["events"] if event.get("span_id") == span_id
        ]

        assert [event["event_type"] for event in span_events] == [
            "span_started",
            "effect",
            "span_completed",
        ]

        effect_events = explicit_events_for_span(timeline["events"], span_id, "effect")
        assert len(effect_events) == 1

        effect_event = effect_events[0]
        assert_timeline_sequence(effect_event, expected_sequence=1)
        assert effect_event["payload"]["effect_kind"] == "model_call"
        assert effect_event["payload"]["has_external_side_effect"] is False
        assert effect_event["payload"]["effect_id"]
        assert effect_event["message"] == "Model call: gpt-4.1-mini"

    def test_sync_tool_effect_round_trip(self):
        """Implicit tool effects round-trip with ordering preserved."""
        client = Continua.init(
            api_key=CONTINUA_API_KEY,
            endpoint=CONTINUA_ENDPOINT,
            batch_size=100,
            flush_interval=60.0,
            ingest_mode="sync",
        )
        trace_name = f"sdk-tool-effect-{uuid.uuid4()}"

        with TraceContext(name=trace_name):
            with span("tool_effect", kind="tool") as effect_span:
                effect_span.set_tool_call(
                    "get_weather",
                    {"city": "New York"},
                    {"temperature": 72, "conditions": "sunny"},
                    has_external_side_effect=True,
                )
                span_id = effect_span.span_id

        client.flush()
        trace_record = wait_for_trace_record(trace_name)
        timeline = fetch_timeline(trace_record["id"])
        span_events = [
            event for event in timeline["events"] if event.get("span_id") == span_id
        ]

        assert [event["event_type"] for event in span_events] == [
            "span_started",
            "effect",
            "span_completed",
        ]

        effect_events = explicit_events_for_span(timeline["events"], span_id, "effect")
        assert len(effect_events) == 1

        effect_event = effect_events[0]
        assert_timeline_sequence(effect_event, expected_sequence=1)
        assert effect_event["payload"]["effect_kind"] == "tool_call"
        assert effect_event["payload"]["has_external_side_effect"] is True
        assert effect_event["payload"]["effect_id"]
        assert effect_event["message"] == "Tool call: get_weather"

    def test_sync_explicit_wait_and_effect_round_trip(self):
        """Explicit wait/effect helpers should preserve IDs, ordering, and metadata."""
        client = Continua.init(
            api_key=CONTINUA_API_KEY,
            endpoint=CONTINUA_ENDPOINT,
            batch_size=100,
            flush_interval=60.0,
            ingest_mode="sync",
        )
        trace_name = f"sdk-wait-effect-{uuid.uuid4()}"
        explicit_effect_id = f"effect-{uuid.uuid4()}"
        explicit_wait_id = f"wait-{uuid.uuid4()}"

        with TraceContext(name=trace_name):
            with span("wait_effect", kind="default") as effect_span:
                effect_span.wait("human_approval", phase="entered")
                effect_span.wait(
                    "timer",
                    phase="resolved",
                    resolution="elapsed",
                    wait_id=explicit_wait_id,
                )
                effect_span.effect(
                    "api_call",
                    has_external_side_effect=True,
                    effect_id=explicit_effect_id,
                    payload={"target": "billing"},
                )
                span_id = effect_span.span_id

        client.flush()
        trace_record = wait_for_trace_record(trace_name)
        timeline = fetch_timeline(trace_record["id"])
        span_events = [
            event for event in timeline["events"] if event.get("span_id") == span_id
        ]

        assert [event["event_type"] for event in span_events] == [
            "span_started",
            "wait",
            "wait",
            "effect",
            "span_completed",
        ]

        wait_events = explicit_events_for_span(timeline["events"], span_id, "wait")
        effect_events = explicit_events_for_span(timeline["events"], span_id, "effect")

        assert len(wait_events) == 2
        assert len(effect_events) == 1

        entered_wait = next(
            event for event in wait_events if event["payload"]["phase"] == "entered"
        )
        resolved_wait = next(
            event for event in wait_events if event["payload"]["phase"] == "resolved"
        )
        effect_event = effect_events[0]

        assert_timeline_sequence(entered_wait, expected_sequence=1)
        assert_timeline_sequence(resolved_wait, expected_sequence=2)
        assert_timeline_sequence(effect_event, expected_sequence=3)

        assert entered_wait["payload"]["wait_kind"] == "human_approval"
        assert entered_wait["payload"]["wait_id"]
        assert resolved_wait["payload"]["wait_kind"] == "timer"
        assert resolved_wait["payload"]["resolution"] == "elapsed"
        assert resolved_wait["payload"]["wait_id"] == explicit_wait_id

        assert effect_event["payload"]["effect_kind"] == "api_call"
        assert effect_event["payload"]["effect_id"] == explicit_effect_id
        assert effect_event["payload"]["target"] == "billing"
