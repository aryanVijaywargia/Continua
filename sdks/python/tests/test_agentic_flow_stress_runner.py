"""Tests for the agentic SDK stress runner."""

from __future__ import annotations

import importlib.util
import sys
import uuid
from collections.abc import Iterator
from contextlib import contextmanager
from pathlib import Path
from typing import Any

import pytest


def load_stress_module():
    module_name = "continua_agentic_flow_stress"
    if module_name in sys.modules:
        return sys.modules[module_name]

    module_path = Path(__file__).resolve().parents[1] / "examples" / "agentic_flow_stress.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules[module_name] = module
    spec.loader.exec_module(module)
    return module


class FakeLLM:
    def __init__(self, stress) -> None:
        self.stress = stress
        self.calls: list[dict[str, Any]] = []

    def complete(self, **kwargs: Any):
        self.calls.append(kwargs)
        index = len(self.calls)
        return self.stress.LLMResult(
            text=f"fake llm result {index}",
            model="gpt-5.4-nano",
            response_id=f"resp-{index}",
            prompt_tokens=100 + index,
            completion_tokens=20 + index,
            total_tokens=120 + index,
            raw={"id": f"resp-{index}"},
        )


class RecordingSDKClient:
    def __init__(self) -> None:
        self.traces: list[dict[str, Any]] = []
        self.spans: list[dict[str, Any]] = []
        self.events: list[dict[str, Any]] = []
        self.flush_count = 0

    def add_trace(self, trace: dict[str, Any]) -> None:
        self.traces.append(dict(trace))

    def add_span(self, span: dict[str, Any]) -> None:
        self.spans.append(dict(span))

    def add_event(self, event: dict[str, Any]) -> None:
        self.events.append(dict(event))

    def flush(self) -> None:
        self.flush_count += 1

    def shutdown(self) -> None:
        return None


@contextmanager
def use_recording_client(stress, client: RecordingSDKClient) -> Iterator[None]:
    previous = stress.Continua._instance
    stress.Continua._instance = client
    try:
        yield
    finally:
        stress.Continua._instance = previous


class FakeReadClient:
    def __init__(self, *, missing_event: bool = False) -> None:
        self.session_id = str(uuid.uuid4())
        self.trace_ids = {
            "trace.a": str(uuid.uuid4()),
            "trace.b": str(uuid.uuid4()),
        }
        self.missing_event = missing_event

    def wait_for_session(self, external_id: str, **_kwargs: Any) -> dict[str, Any]:
        return {
            "id": self.session_id,
            "external_id": external_id,
            "trace_count": 2,
        }

    def wait_for_session_traces(
        self,
        _session_id: str,
        expected_names: set[str],
        **_kwargs: Any,
    ) -> dict[str, dict[str, Any]]:
        return {
            name: {
                "id": self.trace_ids["trace.a" if name.endswith(".a") else "trace.b"],
                "name": name,
                "status": "COMPLETED",
            }
            for name in expected_names
        }

    def get_trace(self, trace_id: str) -> dict[str, Any]:
        name = "readback.a" if trace_id == self.trace_ids["trace.a"] else "readback.b"
        return {"id": trace_id, "name": name, "status": "COMPLETED"}

    def list_spans(self, trace_id: str) -> list[dict[str, Any]]:
        suffix = "a" if trace_id == self.trace_ids["trace.a"] else "b"
        parent_span_id = f"parent-{suffix}"
        return [
            {
                "name": f"root.{suffix}",
                "span_id": parent_span_id,
                "status": "COMPLETED",
                "input": {"ok": True},
                "output": {"ok": True},
            },
            {
                "name": f"llm.{suffix}",
                "span_id": f"child-{suffix}",
                "parent_span_id": parent_span_id,
                "status": "COMPLETED",
                "model": "gpt-5.4-nano",
                "tokens_in": 12,
                "tokens_out": 5,
                "input": {"prompt": "x"},
                "output": {"text": "y"},
            },
        ]

    def timeline(self, trace_id: str) -> dict[str, Any]:
        suffix = "a" if trace_id == self.trace_ids["trace.a"] else "b"
        events = [
            {
                "event_type": "span_started",
                "source": "synthetic",
                "span_id": f"parent-{suffix}",
            },
            {
                "event_type": "decision",
                "source": "explicit",
                "span_id": f"parent-{suffix}",
                "sequence": 1,
            },
            {
                "event_type": "effect",
                "source": "explicit",
                "span_id": f"llm-{suffix}",
                "sequence": 1,
            },
        ]
        if self.missing_event:
            events = [event for event in events if event["event_type"] != "decision"]
        return {"trace_status": "COMPLETED", "events": events}

    def narrative(self, _session_id: str) -> dict[str, Any]:
        return {
            "summary": {"total_trace_count": 2},
            "traces": [{"semantic_events": [{"event_type": "decision"}]}],
        }

    def compare(self, _session_id: str, **_kwargs: Any) -> dict[str, Any]:
        return {
            "summary": {
                "total_spans_baseline": 2,
                "total_spans_candidate": 2,
            }
        }


def make_config(stress):
    return stress.StressConfig(
        continua_api_url="http://continua.test",
        continua_app_url="http://app.test",
        continua_api_key="pk_test_project_key",
        continua_project_id=None,
        openai_api_key="test-openai-api-key",
        openai_base_url="https://api.openai.test/v1",
        openai_model="gpt-5.4-nano",
        continua_ingest_mode="sync",
        run_id="unit",
        report_path=None,
        readback_timeout=0.1,
        poll_interval=0.01,
    )


def test_branching_flow_records_complex_sdk_payloads():
    stress = load_stress_module()
    config = make_config(stress)
    recorder = RecordingSDKClient()
    runner = stress.StressRunner(
        config=config,
        llm=FakeLLM(stress),
        read_client=FakeReadClient(),
        sdk_client=recorder,
    )

    with use_recording_client(stress, recorder):
        expectation = runner.branching_support_triage()

    trace_names = {trace_payload["name"] for trace_payload in recorder.traces}
    expected_trace_names = {trace.name for trace in expectation.traces}
    assert expected_trace_names.issubset(trace_names)

    event_types = {event["event_type"] for event in recorder.events}
    assert {"decision", "state_change", "effect", "wait", "snapshot_marker"}.issubset(
        event_types
    )
    assert any(span_payload.get("model") == "gpt-5.4-nano" for span_payload in recorder.spans)
    assert expectation.session_external_id == "stress-sdk-unit-branching_support_triage"
    billing = next(
        trace
        for trace in expectation.traces
        if trace.name.endswith(".billing_specialist")
    )
    assert billing.min_spans == 3
    assert billing.required_spans == (
        "specialist.root",
        "policy.search",
        "draft.response",
    )


def test_assert_flow_readback_validates_sessions_traces_and_timelines():
    stress = load_stress_module()
    config = make_config(stress)
    expectation = stress.FlowExpectation(
        flow_name="readback",
        session_external_id="session-readback",
        user_id="user",
        traces=(
            stress.ExpectedTrace(
                name="readback.a",
                status="COMPLETED",
                min_spans=2,
                required_spans=("root.a", "llm.a"),
                required_event_types=("decision", "effect"),
                required_llm_spans=("llm.a",),
                required_payload_spans=("llm.a",),
                required_parent_links=(("llm.a", "root.a"),),
            ),
            stress.ExpectedTrace(
                name="readback.b",
                status="COMPLETED",
                min_spans=2,
                required_spans=("root.b", "llm.b"),
                required_event_types=("decision", "effect"),
                required_llm_spans=("llm.b",),
                required_payload_spans=("llm.b",),
            ),
        ),
        compare_pair=("readback.a", "readback.b"),
    )
    runner = stress.StressRunner(
        config=config,
        llm=FakeLLM(stress),
        read_client=FakeReadClient(),
        sdk_client=RecordingSDKClient(),
    )

    summary = runner.assert_flow_readback(expectation)

    assert summary.session_id == runner.read_client.session_id
    assert summary.assertion_stage == "complete"
    assert summary.trace_ids_by_name == {
        "readback.a": runner.read_client.trace_ids["trace.a"],
        "readback.b": runner.read_client.trace_ids["trace.b"],
    }
    assert summary.trace_urls == [
        f"http://app.test/traces/{runner.read_client.trace_ids['trace.a']}",
        f"http://app.test/traces/{runner.read_client.trace_ids['trace.b']}",
    ]


def test_assert_flow_readback_fails_on_missing_required_event():
    stress = load_stress_module()
    config = make_config(stress)
    expectation = stress.FlowExpectation(
        flow_name="readback",
        session_external_id="session-readback",
        user_id="user",
        traces=(
            stress.ExpectedTrace(
                name="readback.a",
                status="COMPLETED",
                min_spans=1,
                required_spans=("root.a",),
                required_event_types=("decision",),
            ),
        ),
    )
    runner = stress.StressRunner(
        config=config,
        llm=FakeLLM(stress),
        read_client=FakeReadClient(missing_event=True),
        sdk_client=RecordingSDKClient(),
    )

    with pytest.raises(AssertionError, match="Missing timeline event"):
        runner.assert_flow_readback(expectation)


def test_run_flow_failure_retains_partial_readback_context():
    stress = load_stress_module()
    config = make_config(stress)
    expectation = stress.FlowExpectation(
        flow_name=stress.FLOW_BRANCHING,
        session_external_id="session-readback",
        user_id="user",
        traces=(
            stress.ExpectedTrace(
                name="readback.a",
                status="COMPLETED",
                min_spans=1,
                required_spans=("root.a",),
                required_event_types=("decision",),
            ),
        ),
    )
    read_client = FakeReadClient(missing_event=True)
    runner = stress.StressRunner(
        config=config,
        llm=FakeLLM(stress),
        read_client=read_client,
        sdk_client=RecordingSDKClient(),
    )
    runner.branching_support_triage = lambda: expectation

    result = runner.run_flow(stress.FLOW_BRANCHING)

    assert result.passed is False
    assert result.session_external_id == "session-readback"
    assert result.session_id == read_client.session_id
    assert result.expected_trace_names == ["readback.a"]
    assert result.trace_ids_by_name == {"readback.a": read_client.trace_ids["trace.a"]}
    assert result.trace_urls == [f"http://app.test/traces/{read_client.trace_ids['trace.a']}"]
    assert result.assertion_stage == "trace:readback.a"
    assert result.error is not None
    assert "Missing timeline event" in result.error


def test_extract_response_text_supports_nested_responses_payload():
    stress = load_stress_module()
    payload = {
        "output": [
            {
                "type": "message",
                "content": [
                    {"type": "output_text", "text": "hello"},
                    {"type": "output_text", "text": "world"},
                ],
            }
        ]
    }

    assert stress.extract_response_text(payload) == "hello\nworld"
