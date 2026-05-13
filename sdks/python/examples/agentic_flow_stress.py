#!/usr/bin/env python3
# ruff: noqa: E402,I001
"""High-difficulty SDK trace/session stress runner.

This script is intended for release-gating validation against a local Continua
server. It runs complex agent-like flows through the Python SDK, calls OpenAI
through raw REST, then verifies traces and sessions through Continua read APIs.

Required for live execution:
    CONTINUA_API_URL=http://localhost:8080
    CONTINUA_API_KEY=default
    OPENAI_API_KEY=...

Optional:
    OPENAI_MODEL=gpt-5.4-nano
    CONTINUA_INGEST_MODE=sync|async_v2|server_default
    CONTINUA_STRESS_RUN_ID=...
    CONTINUA_STRESS_REPORT_PATH=/tmp/continua-agentic-stress.json
"""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import logging
import os
import sys
import time
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Protocol

import httpx

# Keep this script runnable from a source checkout without installing the SDK.
SDK_SRC = Path(__file__).resolve().parents[1] / "src"
if str(SDK_SRC) not in sys.path:
    sys.path.insert(0, str(SDK_SRC))

from continua import Continua, session, span  # noqa: E402
from continua.trace import TraceContext  # noqa: E402


LOGGER = logging.getLogger("continua.agentic_flow_stress")

DEFAULT_CONTINUA_API_URL = "http://localhost:8080"
DEFAULT_CONTINUA_APP_URL = "http://localhost:3000"
DEFAULT_CONTINUA_API_KEY = "default"
DEFAULT_OPENAI_BASE_URL = "https://api.openai.com/v1"
DEFAULT_OPENAI_MODEL = "gpt-5.4-nano"
DEFAULT_INGEST_MODE = "sync"
DEFAULT_READBACK_TIMEOUT = 45.0
DEFAULT_POLL_INTERVAL = 0.5

FLOW_BRANCHING = "branching_support_triage"
FLOW_PARALLEL = "parallel_research_synthesis"
FLOW_FAILURE = "failure_retry_malformed_output"
ALL_FLOWS = (FLOW_BRANCHING, FLOW_PARALLEL, FLOW_FAILURE)


class LLMClient(Protocol):
    """Minimal LLM interface used by the stress flows."""

    def complete(
        self,
        *,
        prompt: str,
        instructions: str,
        max_output_tokens: int = 500,
        metadata: dict[str, str] | None = None,
    ) -> LLMResult:
        """Return a single text completion plus usage metadata."""


@dataclass(frozen=True)
class StressConfig:
    """Runtime configuration resolved from env and CLI."""

    continua_api_url: str
    continua_app_url: str
    continua_api_key: str
    continua_project_id: str | None
    openai_api_key: str
    openai_base_url: str
    openai_model: str
    continua_ingest_mode: str
    run_id: str
    report_path: str | None
    readback_timeout: float
    poll_interval: float

    @classmethod
    def from_env(
        cls,
        *,
        report_path: str | None = None,
        readback_timeout: float | None = None,
    ) -> StressConfig:
        """Resolve runner configuration from environment variables."""
        run_id = os.environ.get("CONTINUA_STRESS_RUN_ID")
        if not run_id:
            run_id = datetime.now(timezone.utc).strftime("%Y%m%d%H%M%S")

        return cls(
            continua_api_url=(
                os.environ.get("CONTINUA_API_URL")
                or os.environ.get("CONTINUA_ENDPOINT")
                or DEFAULT_CONTINUA_API_URL
            ).rstrip("/"),
            continua_app_url=os.environ.get(
                "CONTINUA_APP_URL",
                DEFAULT_CONTINUA_APP_URL,
            ).rstrip("/"),
            continua_api_key=os.environ.get(
                "CONTINUA_API_KEY",
                DEFAULT_CONTINUA_API_KEY,
            ),
            continua_project_id=os.environ.get("CONTINUA_PROJECT_ID"),
            openai_api_key=os.environ.get("OPENAI_API_KEY", ""),
            openai_base_url=os.environ.get(
                "OPENAI_BASE_URL",
                DEFAULT_OPENAI_BASE_URL,
            ).rstrip("/"),
            openai_model=os.environ.get("OPENAI_MODEL", DEFAULT_OPENAI_MODEL),
            continua_ingest_mode=os.environ.get(
                "CONTINUA_INGEST_MODE",
                DEFAULT_INGEST_MODE,
            ),
            run_id=run_id,
            report_path=report_path or os.environ.get("CONTINUA_STRESS_REPORT_PATH"),
            readback_timeout=readback_timeout or DEFAULT_READBACK_TIMEOUT,
            poll_interval=DEFAULT_POLL_INTERVAL,
        )

    def redacted(self) -> dict[str, Any]:
        """Return config safe for reports."""
        data = asdict(self)
        data["continua_api_key"] = "<redacted>" if self.continua_api_key else "<unset>"
        data["openai_api_key"] = "<redacted>" if self.openai_api_key else "<unset>"
        return data


@dataclass(frozen=True)
class LLMResult:
    """LLM response text plus enough metadata to validate SDK capture."""

    text: str
    model: str
    response_id: str | None
    prompt_tokens: int | None
    completion_tokens: int | None
    total_tokens: int | None
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass(frozen=True)
class ExpectedTrace:
    """Trace-level readback expectations."""

    name: str
    status: str
    min_spans: int
    required_spans: tuple[str, ...] = ()
    required_event_types: tuple[str, ...] = ()
    required_llm_spans: tuple[str, ...] = ()
    required_payload_spans: tuple[str, ...] = ()
    required_parent_links: tuple[tuple[str, str], ...] = ()


@dataclass(frozen=True)
class FlowExpectation:
    """Session-level readback expectations for one flow."""

    flow_name: str
    session_external_id: str
    user_id: str
    traces: tuple[ExpectedTrace, ...]
    compare_pair: tuple[str, str] | None = None
    require_narrative_events: bool = True


@dataclass
class FlowReadbackSummary:
    """Readback references discovered while validating one flow."""

    session_id: str | None = None
    trace_urls: list[str] = field(default_factory=list)
    trace_ids_by_name: dict[str, str] = field(default_factory=dict)
    assertion_stage: str | None = None


class FlowReadbackError(AssertionError):
    """Assertion failure with partial readback context attached."""

    def __init__(self, message: str, summary: FlowReadbackSummary) -> None:
        super().__init__(message)
        self.summary = summary


@dataclass
class FlowResult:
    """Serializable outcome for one flow."""

    flow_name: str
    session_external_id: str
    passed: bool
    session_id: str | None = None
    trace_urls: list[str] = field(default_factory=list)
    trace_ids_by_name: dict[str, str] = field(default_factory=dict)
    assertion_stage: str | None = None
    error: str | None = None
    expected_trace_names: list[str] = field(default_factory=list)


class OpenAIResponsesClient:
    """Small raw-REST client for OpenAI Responses API."""

    def __init__(
        self,
        *,
        api_key: str,
        model: str,
        base_url: str = DEFAULT_OPENAI_BASE_URL,
        timeout: float = 60.0,
    ) -> None:
        self.model = model
        self._client = httpx.Client(
            base_url=base_url.rstrip("/"),
            headers={
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
            },
            timeout=timeout,
        )

    def complete(
        self,
        *,
        prompt: str,
        instructions: str,
        max_output_tokens: int = 500,
        metadata: dict[str, str] | None = None,
    ) -> LLMResult:
        """Create a text response and normalize usage metadata."""
        payload: dict[str, Any] = {
            "model": self.model,
            "instructions": instructions,
            "input": prompt,
            "max_output_tokens": max_output_tokens,
            "store": False,
        }
        if metadata:
            payload["metadata"] = metadata

        response = self._client.post("/responses", json=payload)
        if response.status_code >= 400:
            request_id = response.headers.get("x-request-id", "<missing>")
            body = response.text[:1000]
            msg = f"OpenAI request failed ({response.status_code}, {request_id=}): {body}"
            raise RuntimeError(msg)

        data = response.json()
        usage = data.get("usage") or {}
        return LLMResult(
            text=extract_response_text(data),
            model=str(data.get("model") or self.model),
            response_id=data.get("id"),
            prompt_tokens=usage.get("input_tokens"),
            completion_tokens=usage.get("output_tokens"),
            total_tokens=usage.get("total_tokens"),
            raw=data,
        )

    def close(self) -> None:
        """Close the underlying HTTP connection pool."""
        self._client.close()


class ContinuaReadClient:
    """Readback client for debugger REST APIs."""

    def __init__(
        self,
        *,
        api_url: str,
        api_key: str,
        project_id: str | None = None,
        timeout: float = 10.0,
    ) -> None:
        self.api_url = api_url.rstrip("/")
        self.project_id = project_id
        self._client = httpx.Client(
            base_url=self.api_url,
            headers={"X-API-Key": api_key},
            timeout=timeout,
        )

    def close(self) -> None:
        """Close the underlying HTTP connection pool."""
        self._client.close()

    def get_json(
        self,
        path: str,
        *,
        params: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """GET a JSON object from the Continua API."""
        request_params = dict(params or {})
        if self.project_id:
            request_params.setdefault("project_id", self.project_id)
        response = self._client.get(path, params=request_params)
        response.raise_for_status()
        payload = response.json()
        if not isinstance(payload, dict):
            raise AssertionError(f"Expected object response from {path}, got {type(payload)}")
        return payload

    def wait_for_session(
        self,
        external_id: str,
        *,
        timeout: float,
        poll_interval: float,
    ) -> dict[str, Any]:
        """Poll until a session with the exact external ID is visible."""
        deadline = time.monotonic() + timeout
        while True:
            payload = self.get_json(
                "/api/sessions",
                params={"q": external_id, "limit": 100, "offset": 0},
            )
            for session_obj in payload.get("sessions", []):
                if session_obj.get("external_id") == external_id:
                    return session_obj

            if time.monotonic() >= deadline:
                raise AssertionError(f"Timed out waiting for session {external_id!r}")
            time.sleep(poll_interval)

    def wait_for_session_traces(
        self,
        session_id: str,
        expected_names: set[str],
        *,
        timeout: float,
        poll_interval: float,
    ) -> dict[str, dict[str, Any]]:
        """Poll until all expected trace names are present for the session."""
        deadline = time.monotonic() + timeout
        while True:
            traces = self.get_json(
                "/api/traces",
                params={
                    "session_id": session_id,
                    "limit": 100,
                    "offset": 0,
                    "sort_by": "started_at",
                    "sort_dir": "asc",
                },
            ).get("traces", [])
            by_name = {
                str(trace_obj.get("name")): trace_obj
                for trace_obj in traces
                if trace_obj.get("name") in expected_names
            }
            if expected_names.issubset(by_name):
                return by_name

            if time.monotonic() >= deadline:
                missing = sorted(expected_names.difference(by_name))
                raise AssertionError(f"Timed out waiting for traces: {missing}")
            time.sleep(poll_interval)

    def get_trace(self, trace_id: str) -> dict[str, Any]:
        """Fetch trace detail by internal trace UUID."""
        return self.get_json(f"/api/traces/{trace_id}")

    def list_spans(self, trace_id: str) -> list[dict[str, Any]]:
        """List spans for an internal trace UUID."""
        payload = self.get_json(f"/api/traces/{trace_id}/spans")
        spans = payload.get("spans", [])
        if not isinstance(spans, list):
            raise AssertionError(f"Expected span list for trace {trace_id}")
        return spans

    def timeline(self, trace_id: str) -> dict[str, Any]:
        """Fetch a single large timeline page for an internal trace UUID."""
        return self.get_json(f"/api/traces/{trace_id}/events", params={"limit": 500})

    def narrative(self, session_id: str) -> dict[str, Any]:
        """Fetch session narrative for an internal session UUID."""
        return self.get_json(f"/api/sessions/{session_id}/narrative")

    def compare(
        self,
        session_id: str,
        *,
        baseline_trace_id: str,
        candidate_trace_id: str,
    ) -> dict[str, Any]:
        """Fetch session comparison for two internal trace UUIDs."""
        return self.get_json(
            f"/api/sessions/{session_id}/compare",
            params={
                "baseline_trace_id": baseline_trace_id,
                "candidate_trace_id": candidate_trace_id,
            },
        )


class StressRunner:
    """Runs the agentic flows and validates readback."""

    def __init__(
        self,
        *,
        config: StressConfig,
        llm: LLMClient,
        read_client: ContinuaReadClient,
        sdk_client: Any,
    ) -> None:
        self.config = config
        self.llm = llm
        self.read_client = read_client
        self.sdk_client = sdk_client

    def run_all(self, flows: tuple[str, ...] = ALL_FLOWS) -> list[FlowResult]:
        """Run and verify each requested flow."""
        results: list[FlowResult] = []
        for flow_name in flows:
            results.append(self.run_flow(flow_name))
        return results

    def run_flow(self, flow_name: str) -> FlowResult:
        """Run one named flow, flush SDK data, then verify API readback."""
        LOGGER.info("Running flow %s", flow_name)
        expectation: FlowExpectation | None = None

        try:
            if flow_name == FLOW_BRANCHING:
                expectation = self.branching_support_triage()
            elif flow_name == FLOW_PARALLEL:
                expectation = self.parallel_research_synthesis()
            elif flow_name == FLOW_FAILURE:
                expectation = self.failure_retry_malformed_output()
            else:
                raise ValueError(f"Unknown flow: {flow_name}")

            self.sdk_client.flush()
            readback = self.assert_flow_readback(expectation)
            return FlowResult(
                flow_name=flow_name,
                session_external_id=expectation.session_external_id,
                passed=True,
                session_id=readback.session_id,
                trace_urls=readback.trace_urls,
                trace_ids_by_name=readback.trace_ids_by_name,
                assertion_stage=readback.assertion_stage,
                expected_trace_names=[trace.name for trace in expectation.traces],
            )
        except FlowReadbackError as exc:  # pragma: no cover - exercised by live failures
            LOGGER.exception("Flow %s failed", flow_name)
            return FlowResult(
                flow_name=flow_name,
                session_external_id=(
                    expectation.session_external_id
                    if expectation is not None
                    else self.session_id_for(flow_name)
                ),
                passed=False,
                session_id=exc.summary.session_id,
                trace_urls=exc.summary.trace_urls,
                trace_ids_by_name=exc.summary.trace_ids_by_name,
                assertion_stage=exc.summary.assertion_stage,
                error=str(exc),
                expected_trace_names=(
                    [trace.name for trace in expectation.traces]
                    if expectation is not None
                    else []
                ),
            )
        except Exception as exc:  # pragma: no cover - exercised by live failures
            LOGGER.exception("Flow %s failed", flow_name)
            return FlowResult(
                flow_name=flow_name,
                session_external_id=(
                    expectation.session_external_id
                    if expectation is not None
                    else self.session_id_for(flow_name)
                ),
                passed=False,
                error=str(exc),
                expected_trace_names=(
                    [trace.name for trace in expectation.traces]
                    if expectation is not None
                    else []
                ),
            )

    def branching_support_triage(self) -> FlowExpectation:
        """Multi-agent support triage with branching and escalation."""
        flow_name = FLOW_BRANCHING
        session_id = self.session_id_for(flow_name)
        user_id = f"release-gate-user-{self.config.run_id}"
        trace_prefix = self.trace_prefix(flow_name)
        ticket = {
            "ticket_id": f"case-{self.config.run_id}-8472",
            "severity": "high",
            "product_area": "billing",
            "symptom": "customer charged twice after retrying checkout",
        }

        with session(
            session_id,
            user_id=user_id,
            metadata={"flow": flow_name, "run_id": self.config.run_id},
        ):
            with TraceContext(name=f"{trace_prefix}.planner") as trace_ctx:
                trace_ctx.set_metadata("agent_role", "router")
                with span("triage.intake", kind="agent") as intake:
                    intake.set_input({"ticket": ticket})
                    intake.log("Received high-severity support ticket", payload=ticket)
                    intake.state_change(
                        "ticket_state",
                        "new",
                        "triaging",
                        namespace="support",
                        message="Ticket entered triage",
                    )
                    intake.snapshot_marker(
                        "Triage started",
                        marker_kind="phase",
                        payload={"ticket_id": ticket["ticket_id"]},
                    )
                    intake.set_output({"normalized": True, "risk": "billing_duplicate"})

                with span("triage.plan", kind="llm") as planner:
                    planner.set_input({"ticket": ticket})
                    result = self.llm.complete(
                        instructions=(
                            "You are a support-routing planner. Return a concise "
                            "routing recommendation with risk and next action."
                        ),
                        prompt=json.dumps({"task": "route ticket", "ticket": ticket}),
                        max_output_tokens=450,
                        metadata={"flow": flow_name, "stage": "planner"},
                    )
                    planner.set_llm_response(
                        self.config.openai_model,
                        {"ticket": ticket},
                        {"text": result.text, "response_id": result.response_id},
                        tokens_in=result.prompt_tokens,
                        tokens_out=result.completion_tokens,
                        provider="openai",
                    )
                    planner.decision(
                        "Which specialist branch should handle the ticket?",
                        "billing_escalation",
                        alternatives=["self_serve", "technical_support", "billing_escalation"],
                        reasoning="Duplicate-charge reports require billing review.",
                        message="Escalated to billing specialist",
                    )

                with span("crm_lookup", kind="tool") as crm:
                    lookup_result = {
                        "customer_tier": "enterprise",
                        "open_invoices": 2,
                        "recent_payment_retries": 3,
                    }
                    crm.set_tool_call(
                        "crm_lookup",
                        {"customer_id": "cust_release_gate"},
                        lookup_result,
                        has_external_side_effect=False,
                    )
                    crm.state_change(
                        "customer_context",
                        "unknown",
                        "loaded",
                        namespace="crm",
                    )

                trace_ctx.set_output({"branch": "billing_escalation", "ticket": ticket})

            with TraceContext(name=f"{trace_prefix}.billing_specialist") as trace_ctx:
                trace_ctx.set_metadata("agent_role", "specialist")
                with span("specialist.root", kind="agent") as root:
                    root.set_input({"ticket_id": ticket["ticket_id"]})
                    with span("policy.search", kind="retrieval") as search:
                        search.set_input({"query": "duplicate charge refund policy"})
                        search.set_output(
                            {
                                "documents": [
                                    "refund-policy-v3",
                                    "payment-retry-runbook",
                                ],
                            }
                        )
                    with span("draft.response", kind="llm") as draft:
                        result = self.llm.complete(
                            instructions=(
                                "You are a billing support specialist. Draft a "
                                "short customer-safe response."
                            ),
                            prompt=json.dumps({"ticket": ticket, "policy": "refund-policy-v3"}),
                            max_output_tokens=500,
                            metadata={"flow": flow_name, "stage": "billing_specialist"},
                        )
                        draft.set_llm_response(
                            self.config.openai_model,
                            {"ticket": ticket, "policy": "refund-policy-v3"},
                            {"text": result.text, "response_id": result.response_id},
                            tokens_in=result.prompt_tokens,
                            tokens_out=result.completion_tokens,
                            provider="openai",
                        )
                    root.effect(
                        "internal_note",
                        has_external_side_effect=True,
                        idempotent=True,
                        idempotency_key=f"{ticket['ticket_id']}:note",
                        payload={"ticket_id": ticket["ticket_id"]},
                    )
                    root.set_output({"drafted": True, "next": "manager_approval"})
                trace_ctx.set_output({"specialist_status": "drafted"})

            with TraceContext(name=f"{trace_prefix}.escalation") as trace_ctx:
                trace_ctx.set_metadata("agent_role", "escalation")
                with span("manager.review", kind="agent") as review:
                    review.wait(
                        "manager_approval",
                        phase="entered",
                        wait_id=f"approval-{self.config.run_id}",
                    )
                    review.wait(
                        "manager_approval",
                        phase="resolved",
                        resolution="approved",
                        wait_id=f"approval-{self.config.run_id}",
                    )
                    review.decision(
                        "Refund now or wait for invoice reconciliation?",
                        "refund_now",
                        alternatives=["refund_now", "wait_for_reconciliation"],
                        reasoning="Enterprise duplicate charge with retry evidence.",
                    )
                    review.set_output({"approved": True})
                with span("case_update", kind="tool") as case_update:
                    case_update.set_tool_call(
                        "case_update",
                        {"ticket_id": ticket["ticket_id"], "status": "refund_approved"},
                        {"status": "updated", "refund_id": f"refund-{self.config.run_id}"},
                        has_external_side_effect=True,
                    )
                trace_ctx.set_output({"resolution": "refund_approved"})

        return FlowExpectation(
            flow_name=flow_name,
            session_external_id=session_id,
            user_id=user_id,
            traces=(
                ExpectedTrace(
                    name=f"{trace_prefix}.planner",
                    status="COMPLETED",
                    min_spans=3,
                    required_spans=("triage.intake", "triage.plan", "crm_lookup"),
                    required_event_types=("decision", "state_change", "snapshot_marker"),
                    required_llm_spans=("triage.plan",),
                    required_payload_spans=("triage.intake", "crm_lookup"),
                ),
                ExpectedTrace(
                    name=f"{trace_prefix}.billing_specialist",
                    status="COMPLETED",
                    min_spans=3,
                    required_spans=("specialist.root", "policy.search", "draft.response"),
                    required_event_types=("effect",),
                    required_llm_spans=("draft.response",),
                    required_payload_spans=("policy.search", "draft.response"),
                    required_parent_links=(("policy.search", "specialist.root"),),
                ),
                ExpectedTrace(
                    name=f"{trace_prefix}.escalation",
                    status="COMPLETED",
                    min_spans=2,
                    required_spans=("manager.review", "case_update"),
                    required_event_types=("wait", "decision", "effect"),
                    required_payload_spans=("case_update",),
                ),
            ),
            compare_pair=(f"{trace_prefix}.planner", f"{trace_prefix}.billing_specialist"),
        )

    def parallel_research_synthesis(self) -> FlowExpectation:
        """Parallel workers with explicit session IDs and synthesis readback."""
        flow_name = FLOW_PARALLEL
        session_id = self.session_id_for(flow_name)
        user_id = f"parallel-user-{self.config.run_id}"
        trace_prefix = self.trace_prefix(flow_name)
        topics = (
            "SDK trace context behavior under concurrency",
            "session grouping semantics for parallel branches",
            "timeline event ordering for nested spans",
        )

        def run_worker(index: int, topic: str) -> dict[str, Any]:
            trace_name = f"{trace_prefix}.worker.{index}"
            with TraceContext(
                name=trace_name,
                session_id=session_id,
                user_id=user_id,
                metadata={"flow": flow_name, "worker": index, "run_id": self.config.run_id},
            ) as trace_ctx:
                with span(f"worker.{index}.root", kind="agent") as root:
                    root.set_input({"topic": topic})
                    root.state_change(
                        "worker_state",
                        "queued",
                        "running",
                        namespace=f"worker-{index}",
                    )
                    with span(f"retrieve.shard.{index}", kind="retrieval") as retrieval:
                        retrieval.set_input({"topic": topic, "limit": 3})
                        retrieval.set_output(
                            {
                                "documents": [
                                    f"doc-{index}-a",
                                    f"doc-{index}-b",
                                    f"doc-{index}-c",
                                ],
                            }
                        )
                    with span(f"summarize.shard.{index}", kind="llm") as summarizer:
                        result = self.llm.complete(
                            instructions=(
                                "Summarize this research shard in two precise "
                                "sentences for a release validation report."
                            ),
                            prompt=json.dumps({"topic": topic, "worker": index}),
                            max_output_tokens=400,
                            metadata={"flow": flow_name, "worker": str(index)},
                        )
                        summarizer.set_llm_response(
                            self.config.openai_model,
                            {"topic": topic, "worker": index},
                            {"text": result.text, "response_id": result.response_id},
                            tokens_in=result.prompt_tokens,
                            tokens_out=result.completion_tokens,
                            provider="openai",
                        )
                    root.snapshot_marker(
                        f"Worker {index} complete",
                        marker_kind="worker",
                        payload={"topic": topic},
                    )
                    root.set_output({"topic": topic, "summary": "captured"})
                trace_ctx.set_output({"worker": index, "status": "completed"})
            return {"worker": index, "topic": topic}

        with concurrent.futures.ThreadPoolExecutor(max_workers=len(topics)) as executor:
            futures = [
                executor.submit(run_worker, index, topic)
                for index, topic in enumerate(topics, start=1)
            ]
            worker_results = [future.result() for future in futures]

        with TraceContext(
            name=f"{trace_prefix}.synthesis",
            session_id=session_id,
            user_id=user_id,
            metadata={"flow": flow_name, "run_id": self.config.run_id},
        ) as trace_ctx:
            with span("synthesis.merge", kind="chain") as merge:
                merge.wait(
                    "parallel_workers",
                    phase="entered",
                    wait_id=f"workers-{self.config.run_id}",
                )
                merge.wait(
                    "parallel_workers",
                    phase="resolved",
                    resolution="all_completed",
                    wait_id=f"workers-{self.config.run_id}",
                )
                with span("synthesis.final_answer", kind="llm") as final_answer:
                    result = self.llm.complete(
                        instructions=(
                            "Synthesize worker notes into a compact validation finding."
                        ),
                        prompt=json.dumps({"worker_results": worker_results}),
                        max_output_tokens=500,
                        metadata={"flow": flow_name, "stage": "synthesis"},
                    )
                    final_answer.set_llm_response(
                        self.config.openai_model,
                        {"worker_results": worker_results},
                        {"text": result.text, "response_id": result.response_id},
                        tokens_in=result.prompt_tokens,
                        tokens_out=result.completion_tokens,
                        provider="openai",
                    )
                merge.snapshot_marker(
                    "Parallel synthesis complete",
                    marker_kind="phase",
                    payload={"workers": len(worker_results)},
                )
                merge.set_output({"worker_count": len(worker_results)})
            trace_ctx.set_output({"status": "synthesized", "workers": worker_results})

        worker_expectations = tuple(
            ExpectedTrace(
                name=f"{trace_prefix}.worker.{index}",
                status="COMPLETED",
                min_spans=3,
                required_spans=(
                    f"worker.{index}.root",
                    f"retrieve.shard.{index}",
                    f"summarize.shard.{index}",
                ),
                required_event_types=("state_change", "snapshot_marker", "effect"),
                required_llm_spans=(f"summarize.shard.{index}",),
                required_payload_spans=(f"retrieve.shard.{index}",),
                required_parent_links=((f"retrieve.shard.{index}", f"worker.{index}.root"),),
            )
            for index in range(1, len(topics) + 1)
        )

        return FlowExpectation(
            flow_name=flow_name,
            session_external_id=session_id,
            user_id=user_id,
            traces=(
                *worker_expectations,
                ExpectedTrace(
                    name=f"{trace_prefix}.synthesis",
                    status="COMPLETED",
                    min_spans=2,
                    required_spans=("synthesis.merge", "synthesis.final_answer"),
                    required_event_types=("wait", "snapshot_marker", "effect"),
                    required_llm_spans=("synthesis.final_answer",),
                    required_payload_spans=("synthesis.final_answer",),
                    required_parent_links=(("synthesis.final_answer", "synthesis.merge"),),
                ),
            ),
            compare_pair=(f"{trace_prefix}.worker.1", f"{trace_prefix}.synthesis"),
        )

    def failure_retry_malformed_output(self) -> FlowExpectation:
        """Failure and retry flow with malformed output and recovery trace."""
        flow_name = FLOW_FAILURE
        session_id = self.session_id_for(flow_name)
        user_id = f"retry-user-{self.config.run_id}"
        trace_prefix = self.trace_prefix(flow_name)

        try:
            with TraceContext(
                name=f"{trace_prefix}.attempt.1",
                session_id=session_id,
                user_id=user_id,
                metadata={"flow": flow_name, "attempt": 1, "run_id": self.config.run_id},
            ) as trace_ctx:
                with span("attempt.1.model_call", kind="llm") as model_call:
                    result = self.llm.complete(
                        instructions=(
                            "Return a JSON object with keys risk, action, and confidence."
                        ),
                        prompt="Classify a checkout retry failure for release validation.",
                        max_output_tokens=350,
                        metadata={"flow": flow_name, "attempt": "1"},
                    )
                    model_call.set_llm_response(
                        self.config.openai_model,
                        {"task": "classify checkout retry failure"},
                        {"text": result.text, "response_id": result.response_id},
                        tokens_in=result.prompt_tokens,
                        tokens_out=result.completion_tokens,
                        provider="openai",
                    )

                with span("parse_model_output", kind="tool") as parser:
                    malformed_payload = result.text[:80] + " {"
                    parser.set_input({"raw": malformed_payload})
                    try:
                        json.loads(malformed_payload)
                    except json.JSONDecodeError as exc:
                        parser.error(
                            "Malformed model output",
                            payload={"raw_length": len(malformed_payload)},
                        )
                        parser.exception(exc, payload={"stage": "json_parse"})
                        parser.set_error("Could not parse model JSON")
                        parser.set_output({"parsed": False, "error": str(exc)})
                        raise ValueError("Malformed model output after model call") from exc
                trace_ctx.set_output({"unexpected": "parser succeeded"})
        except ValueError:
            LOGGER.info("Captured expected malformed-output failure")

        with TraceContext(
            name=f"{trace_prefix}.attempt.2_recovery",
            session_id=session_id,
            user_id=user_id,
            metadata={"flow": flow_name, "attempt": 2, "run_id": self.config.run_id},
        ) as trace_ctx:
            with span("retry.backoff", kind="agent") as retry:
                retry.wait("retry_backoff", phase="entered", wait_id=f"retry-{self.config.run_id}")
                retry.wait(
                    "retry_backoff",
                    phase="resolved",
                    resolution="elapsed",
                    wait_id=f"retry-{self.config.run_id}",
                )
                retry.decision(
                    "Retry with stricter output contract?",
                    "retry_with_repair_prompt",
                    alternatives=["abort", "retry_with_repair_prompt"],
                    reasoning="Initial output was malformed but request is recoverable.",
                )
                retry.set_output({"retry": True})

            with span("attempt.2.model_repair", kind="llm") as repair:
                result = self.llm.complete(
                    instructions=(
                        "Return strict JSON only. Use keys risk, action, confidence."
                    ),
                    prompt="Return JSON for duplicate checkout retry risk.",
                    max_output_tokens=350,
                    metadata={"flow": flow_name, "attempt": "2"},
                )
                repaired = {
                    "risk": "duplicate_charge",
                    "action": "escalate_to_billing",
                    "confidence": 0.91,
                    "model_text": result.text,
                }
                repair.set_llm_response(
                    self.config.openai_model,
                    {"task": "repair malformed output"},
                    repaired,
                    tokens_in=result.prompt_tokens,
                    tokens_out=result.completion_tokens,
                    provider="openai",
                )

            with span("schema.validate", kind="tool") as validator:
                validator.set_tool_call(
                    "schema.validate",
                    {"required": ["risk", "action", "confidence"]},
                    {"valid": True, "missing": []},
                    has_external_side_effect=False,
                )
                validator.state_change(
                    "parse_state",
                    "malformed",
                    "valid",
                    namespace="model_output",
                    message="Recovered model output",
                )
            trace_ctx.set_output({"status": "recovered", "risk": "duplicate_charge"})

        return FlowExpectation(
            flow_name=flow_name,
            session_external_id=session_id,
            user_id=user_id,
            traces=(
                ExpectedTrace(
                    name=f"{trace_prefix}.attempt.1",
                    status="FAILED",
                    min_spans=2,
                    required_spans=("attempt.1.model_call", "parse_model_output"),
                    required_event_types=("effect", "error", "exception"),
                    required_llm_spans=("attempt.1.model_call",),
                    required_payload_spans=("parse_model_output",),
                ),
                ExpectedTrace(
                    name=f"{trace_prefix}.attempt.2_recovery",
                    status="COMPLETED",
                    min_spans=3,
                    required_spans=(
                        "retry.backoff",
                        "attempt.2.model_repair",
                        "schema.validate",
                    ),
                    required_event_types=("wait", "decision", "effect", "state_change"),
                    required_llm_spans=("attempt.2.model_repair",),
                    required_payload_spans=("schema.validate",),
                ),
            ),
            compare_pair=(f"{trace_prefix}.attempt.1", f"{trace_prefix}.attempt.2_recovery"),
        )

    def assert_flow_readback(self, expectation: FlowExpectation) -> FlowReadbackSummary:
        """Assert session, trace, span, timeline, narrative, and compare readback."""
        summary = FlowReadbackSummary(assertion_stage="session")
        try:
            session_obj = self.read_client.wait_for_session(
                expectation.session_external_id,
                timeout=self.config.readback_timeout,
                poll_interval=self.config.poll_interval,
            )
            summary.session_id = str(session_obj["id"])
            assert_equal(session_obj.get("external_id"), expectation.session_external_id)
            assert_at_least(int(session_obj.get("trace_count") or 0), len(expectation.traces))

            expected_names = {trace.name for trace in expectation.traces}
            summary.assertion_stage = "session_traces"
            trace_records = self.read_client.wait_for_session_traces(
                summary.session_id,
                expected_names,
                timeout=self.config.readback_timeout,
                poll_interval=self.config.poll_interval,
            )
            summary.trace_ids_by_name = {
                name: str(trace_record["id"])
                for name, trace_record in sorted(trace_records.items())
            }

            for expected in expectation.traces:
                trace_record = trace_records[expected.name]
                summary.assertion_stage = f"trace:{expected.name}"
                trace_url = f"{self.config.continua_app_url}/traces/{trace_record['id']}"
                summary.trace_urls.append(trace_url)
                self.assert_trace_readback(expected, trace_record)

            if expectation.require_narrative_events:
                summary.assertion_stage = "narrative"
                narrative = self.read_client.narrative(summary.session_id)
                narrative_summary = narrative.get("summary") or {}
                assert_at_least(
                    int(narrative_summary.get("total_trace_count") or 0),
                    len(expectation.traces),
                )
                narrative_traces = narrative.get("traces", [])
                if not any((trace.get("semantic_events") or []) for trace in narrative_traces):
                    raise AssertionError(
                        f"Session {expectation.session_external_id} has no narrative events"
                    )

            if expectation.compare_pair:
                summary.assertion_stage = "compare"
                baseline, candidate = expectation.compare_pair
                comparison = self.read_client.compare(
                    summary.session_id,
                    baseline_trace_id=str(trace_records[baseline]["id"]),
                    candidate_trace_id=str(trace_records[candidate]["id"]),
                )
                comparison_summary = comparison.get("summary") or {}
                assert_at_least(int(comparison_summary.get("total_spans_baseline") or 0), 1)
                assert_at_least(int(comparison_summary.get("total_spans_candidate") or 0), 1)

            summary.assertion_stage = "complete"
            return summary
        except FlowReadbackError:
            raise
        except Exception as exc:
            raise FlowReadbackError(str(exc), summary) from exc

    def assert_trace_readback(
        self,
        expected: ExpectedTrace,
        trace_record: dict[str, Any],
    ) -> None:
        """Assert one trace detail, span list, and timeline."""
        detail = self.read_client.get_trace(str(trace_record["id"]))
        assert_equal(detail.get("name"), expected.name)
        assert_equal(detail.get("status"), expected.status)

        spans = self.read_client.list_spans(str(trace_record["id"]))
        assert_at_least(len(spans), expected.min_spans)
        spans_by_name = {str(span_obj.get("name")): span_obj for span_obj in spans}

        for span_name in expected.required_spans:
            if span_name not in spans_by_name:
                raise AssertionError(f"Missing span {span_name!r} in trace {expected.name!r}")

        for child_name, parent_name in expected.required_parent_links:
            child = spans_by_name.get(child_name)
            parent = spans_by_name.get(parent_name)
            if child is None or parent is None:
                raise AssertionError(f"Cannot verify parent link {child_name}->{parent_name}")
            assert_equal(
                child.get("parent_span_id"),
                parent.get("span_id"),
                label=f"parent link for {child_name}",
            )

        for span_name in expected.required_llm_spans:
            span_obj = spans_by_name[span_name]
            assert_equal(span_obj.get("model"), self.config.openai_model)
            assert_at_least(int(span_obj.get("tokens_in") or 0), 1)
            assert_at_least(int(span_obj.get("tokens_out") or 0), 1)

        for span_name in expected.required_payload_spans:
            span_obj = spans_by_name[span_name]
            if span_obj.get("input") is None and span_obj.get("output") is None:
                raise AssertionError(f"Span {span_name!r} did not preserve input/output payload")

        timeline = self.read_client.timeline(str(trace_record["id"]))
        assert_equal(timeline.get("trace_status"), expected.status)
        events = timeline.get("events") or []
        assert_at_least(len(events), expected.min_spans)

        event_types = {event.get("event_type") for event in events}
        for event_type in expected.required_event_types:
            if event_type not in event_types:
                raise AssertionError(
                    f"Missing timeline event {event_type!r} in trace {expected.name!r}"
                )

        explicit_events = [event for event in events if event.get("source") == "explicit"]
        sequences_by_span: dict[str, list[int]] = {}
        for event in explicit_events:
            sequence = event.get("sequence")
            span_id = event.get("span_id")
            if sequence is None or not span_id:
                continue
            sequences_by_span.setdefault(str(span_id), []).append(int(sequence))

        for span_id, sequences in sequences_by_span.items():
            sorted_sequences = sorted(sequences)
            if sorted_sequences != list(range(1, len(sorted_sequences) + 1)):
                raise AssertionError(f"Non-contiguous event sequence for span {span_id}")

    def session_id_for(self, flow_name: str) -> str:
        """Stable external session ID for a flow/run pair."""
        return f"stress-sdk-{self.config.run_id}-{flow_name}"

    def trace_prefix(self, flow_name: str) -> str:
        """Stable trace-name prefix for a flow/run pair."""
        return f"stress.{self.config.run_id}.{flow_name}"


def extract_response_text(data: dict[str, Any]) -> str:
    """Extract visible text from an OpenAI Responses API payload."""
    direct = data.get("output_text")
    if isinstance(direct, str) and direct:
        return direct

    fragments: list[str] = []
    for item in data.get("output", []) or []:
        if not isinstance(item, dict):
            continue
        if item.get("type") == "output_text" and isinstance(item.get("text"), str):
            fragments.append(item["text"])
        for content in item.get("content", []) or []:
            if not isinstance(content, dict):
                continue
            if content.get("type") == "output_text" and isinstance(content.get("text"), str):
                fragments.append(content["text"])

    text = "\n".join(fragment for fragment in fragments if fragment)
    if not text:
        raise RuntimeError("OpenAI response did not include output text")
    return text


def assert_equal(actual: Any, expected: Any, *, label: str = "value") -> None:
    """Assert equality with a compact error message."""
    if actual != expected:
        raise AssertionError(f"Expected {label} {expected!r}, got {actual!r}")


def assert_at_least(actual: int, minimum: int, *, label: str = "value") -> None:
    """Assert an integer lower bound."""
    if actual < minimum:
        raise AssertionError(f"Expected {label} >= {minimum}, got {actual}")


def build_report(config: StressConfig, results: list[FlowResult]) -> dict[str, Any]:
    """Build the structured JSON report emitted by the runner."""
    return {
        "run_id": config.run_id,
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "config": config.redacted(),
        "overall_passed": all(result.passed for result in results),
        "flows": [asdict(result) for result in results],
        "needed_from_user": [
            "OPENAI_API_KEY with access to the configured OPENAI_MODEL",
            "Optional OPENAI_MODEL=gpt-5.4-mini override if gpt-5.4-nano is unavailable",
        ],
    }


def write_report(path: str, report: dict[str, Any]) -> None:
    """Write a JSON report to disk."""
    report_path = Path(path).expanduser().resolve()
    report_path.parent.mkdir(parents=True, exist_ok=True)
    report_path.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    LOGGER.info("Wrote report to %s", report_path)


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    """Parse command-line options."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--flow",
        action="append",
        choices=ALL_FLOWS,
        help="Run only this flow. May be provided multiple times.",
    )
    parser.add_argument(
        "--report",
        help="Optional JSON report path. Defaults to CONTINUA_STRESS_REPORT_PATH.",
    )
    parser.add_argument(
        "--readback-timeout",
        type=float,
        help=f"Readback polling timeout in seconds. Default: {DEFAULT_READBACK_TIMEOUT}.",
    )
    parser.add_argument("--verbose", action="store_true", help="Enable debug logging.")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    """Run configured flows and return a process exit code."""
    args = parse_args(argv)
    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
    )

    config = StressConfig.from_env(
        report_path=args.report,
        readback_timeout=args.readback_timeout,
    )
    if not config.openai_api_key:
        print("OPENAI_API_KEY is required to run live agentic flow stress tests.", file=sys.stderr)
        return 2

    selected_flows = tuple(args.flow) if args.flow else ALL_FLOWS
    LOGGER.info("Starting Continua SDK stress run %s", config.run_id)
    LOGGER.info("Continua API: %s", config.continua_api_url)
    LOGGER.info("OpenAI model: %s", config.openai_model)
    LOGGER.info("Ingest mode: %s", config.continua_ingest_mode)

    sdk_client = Continua.init(
        api_key=config.continua_api_key,
        endpoint=config.continua_api_url,
        batch_size=1000,
        flush_interval=60.0,
        ingest_mode=config.continua_ingest_mode,
    )
    read_client = ContinuaReadClient(
        api_url=config.continua_api_url,
        api_key=config.continua_api_key,
        project_id=config.continua_project_id,
    )
    llm = OpenAIResponsesClient(
        api_key=config.openai_api_key,
        model=config.openai_model,
        base_url=config.openai_base_url,
    )

    try:
        runner = StressRunner(
            config=config,
            llm=llm,
            read_client=read_client,
            sdk_client=sdk_client,
        )
        results = runner.run_all(selected_flows)
        report = build_report(config, results)
        if config.report_path:
            write_report(config.report_path, report)
        print(json.dumps(report, indent=2, sort_keys=True))
        return 0 if report["overall_passed"] else 1
    finally:
        llm.close()
        read_client.close()
        sdk_client.shutdown()
        Continua._instance = None


if __name__ == "__main__":
    raise SystemExit(main())
