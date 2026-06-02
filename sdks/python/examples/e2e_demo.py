#!/usr/bin/env python3
# ruff: noqa: E402,I001
"""
End-to-End Demo for Continua Python SDK

This script demonstrates:
1. SDK initialization
2. Creating traces with nested spans
3. Recording real provider-backed LLM, tool, and agent spans when requested
4. Verifying data via API

Usage:
    cd sdks/python
    uv run python examples/e2e_demo.py
"""

import json
import os
import re
import sys
import time
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Any

import httpx

# Keep the demo runnable from a source checkout even if the SDK package has not
# been installed into the active environment.
SDK_SRC = Path(__file__).resolve().parents[1] / "src"
if str(SDK_SRC) not in sys.path:
    sys.path.insert(0, str(SDK_SRC))

# Import the SDK
from continua import Continua, session, span, trace
from continua.engine_control import EngineControlClient


# Configuration
API_URL = (
    os.environ.get("CONTINUA_API_URL")
    or os.environ.get("CONTINUA_ENDPOINT")
    or "http://localhost:8080"
).rstrip("/")
APP_URL = os.environ.get("CONTINUA_APP_URL", "http://localhost:3000").rstrip("/")
API_KEY = os.environ.get("CONTINUA_API_KEY", "")
PRINT_API_KEY = os.environ.get("CONTINUA_PRINT_API_KEY", "").lower() in {
    "1",
    "true",
    "yes",
}
DEMO_RUN_ID = os.environ.get(
    "CONTINUA_DEMO_RUN_ID",
    datetime.now().strftime("%Y%m%d%H%M%S"),
)
AGENT_MODE = os.environ.get("CONTINUA_DEMO_AGENT_MODE", "offline").strip().lower()
OPENAI_API_KEY = os.environ.get("OPENAI_API_KEY", "")
OPENAI_MODEL = os.environ.get("CONTINUA_DEMO_OPENAI_MODEL", "gpt-4.1-mini")
OPENAI_API_URL = os.environ.get(
    "CONTINUA_DEMO_OPENAI_API_URL",
    "https://api.openai.com/v1/chat/completions",
)
SEED_ENGINE_RUNS = os.environ.get("CONTINUA_DEMO_SEED_ENGINE_RUNS", "").lower() in {
    "1",
    "true",
    "yes",
}
SESSION_IDS = [
    f"demo-sdk-{DEMO_RUN_ID}-research",
    f"demo-sdk-{DEMO_RUN_ID}-incident",
]
REPO_ROOT = Path(__file__).resolve().parents[3]
CODE_REVIEW_TARGET = REPO_ROOT / "sdks" / "python" / "src" / "continua" / "trace.py"
DOC_SEARCH_ROOT = REPO_ROOT / "docs-site" / "concepts"


@dataclass(frozen=True)
class ModelCallResult:
    content: str
    model: str
    provider: str
    prompt_tokens: int | None
    completion_tokens: int | None
    response_id: str | None = None


def require_real_agent_backend() -> None:
    if AGENT_MODE != "openai":
        return
    if not OPENAI_API_KEY:
        raise RuntimeError(
            "OPENAI_API_KEY is required when CONTINUA_DEMO_AGENT_MODE=openai. "
            "Public demo seeding requires real provider-backed agent runs."
        )


def call_openai_chat(
    *,
    purpose: str,
    messages: list[dict[str, str]],
    response_format: dict[str, str] | None = None,
) -> ModelCallResult:
    """Call OpenAI directly through HTTP so the demo has no SDK dependency."""
    require_real_agent_backend()
    request_body: dict[str, Any] = {
        "model": OPENAI_MODEL,
        "messages": messages,
        "temperature": 0.2,
    }
    if response_format is not None:
        request_body["response_format"] = response_format

    with httpx.Client(timeout=45.0) as client:
        response = client.post(
            OPENAI_API_URL,
            headers={
                "Authorization": f"Bearer {OPENAI_API_KEY}",
                "Content-Type": "application/json",
            },
            json=request_body,
        )
        response.raise_for_status()
        payload = response.json()

    choice = payload.get("choices", [{}])[0]
    content = str(choice.get("message", {}).get("content", "")).strip()
    usage = payload.get("usage") or {}
    if not content:
        raise RuntimeError(f"OpenAI returned an empty response for {purpose}")
    return ModelCallResult(
        content=content,
        model=str(payload.get("model") or OPENAI_MODEL),
        provider="openai",
        prompt_tokens=usage.get("prompt_tokens"),
        completion_tokens=usage.get("completion_tokens"),
        response_id=payload.get("id"),
    )


def call_agent_model(
    *,
    purpose: str,
    messages: list[dict[str, str]],
    response_format: dict[str, str] | None = None,
) -> ModelCallResult:
    """Run an agent model call, using OpenAI for seeded demos and offline only by opt-in."""
    if AGENT_MODE == "openai":
        return call_openai_chat(
            purpose=purpose,
            messages=messages,
            response_format=response_format,
        )
    if AGENT_MODE != "offline":
        raise RuntimeError("CONTINUA_DEMO_AGENT_MODE must be 'openai' or 'offline'")

    prompt_text = "\n".join(message["content"] for message in messages)
    return ModelCallResult(
        content=(
            f"Offline demo response for {purpose}. "
            f"Input length: {len(prompt_text)} characters."
        ),
        model="offline-demo-model",
        provider="offline",
        prompt_tokens=max(1, len(prompt_text) // 4),
        completion_tokens=16,
        response_id=None,
    )


def parse_json_object(value: str) -> dict[str, Any]:
    try:
        decoded = json.loads(value)
    except json.JSONDecodeError:
        return {"raw_response": value}
    return decoded if isinstance(decoded, dict) else {"raw_response": decoded}


def search_docs(query: str, *, limit: int = 4) -> list[dict[str, Any]]:
    terms = {term.lower() for term in re.findall(r"[a-zA-Z][a-zA-Z0-9_-]{2,}", query)}
    results: list[dict[str, Any]] = []
    for path in sorted(DOC_SEARCH_ROOT.glob("*.mdx")):
        text = path.read_text(encoding="utf-8")
        score = sum(text.lower().count(term) for term in terms)
        if score == 0:
            continue
        snippet = " ".join(text.split())[:700]
        results.append(
            {"path": str(path.relative_to(REPO_ROOT)), "score": score, "snippet": snippet}
        )
    return sorted(results, key=lambda item: item["score"], reverse=True)[:limit]


def analyze_python_source(path: Path) -> dict[str, Any]:
    text = path.read_text(encoding="utf-8")
    functions = re.findall(r"^\s*def\s+([a-zA-Z_][a-zA-Z0-9_]*)\(", text, flags=re.MULTILINE)
    classes = re.findall(r"^\s*class\s+([a-zA-Z_][a-zA-Z0-9_]*)\(", text, flags=re.MULTILINE)
    return {
        "path": str(path.relative_to(REPO_ROOT)),
        "line_count": len(text.splitlines()),
        "function_count": len(functions),
        "class_count": len(classes),
        "functions": functions[:12],
        "classes": classes[:8],
        "sample": text[:1800],
    }


def probe_unavailable_dependency() -> dict[str, Any]:
    target = "http://127.0.0.1:9/continua-demo-health"
    started = time.monotonic()
    try:
        httpx.get(target, timeout=0.5)
    except httpx.HTTPError as exc:
        return {
            "target": target,
            "status": "failed",
            "error_type": type(exc).__name__,
            "error": str(exc),
            "duration_ms": round((time.monotonic() - started) * 1000, 2),
        }
    return {
        "target": target,
        "status": "unexpectedly_available",
        "duration_ms": round((time.monotonic() - started) * 1000, 2),
    }


@trace(name="research_agent")
def run_research_agent(query: str):
    """
    Run a research agent that:
    1. Plans the research
    2. Searches for information
    3. Synthesizes results
    """
    print(f"  Starting research agent for: {query}")
    plan_data: dict[str, Any]
    docs_results: list[dict[str, Any]]

    # Step 1: Planning span
    with span("plan_research", kind="llm") as s:
        messages = [
            {
                "role": "system",
                "content": (
                    "You are a concise research agent. Return a JSON object with "
                    "keys plan, chosen_focus, and alternatives."
                ),
            },
            {"role": "user", "content": f"Create a research plan for: {query}"},
        ]
        s.set_input({"messages": messages})
        s.log("Planning research steps", payload={"query": query})
        plan = call_agent_model(
            purpose="research_plan",
            messages=messages,
            response_format={"type": "json_object"} if AGENT_MODE == "openai" else None,
        )
        plan_data = parse_json_object(plan.content)
        s.set_llm_response(
            plan.model,
            messages,
            plan_data,
            tokens_in=plan.prompt_tokens,
            tokens_out=plan.completion_tokens,
            provider=plan.provider,
        )
        s.decision(
            "Which research focus should the agent pursue?",
            plan_data.get("chosen_focus", "observability value and failure debugging"),
            alternatives=plan_data.get("alternatives")
            if isinstance(plan_data.get("alternatives"), list)
            else None,
            reasoning=str(plan_data.get("plan", plan.content))[:600],
            message="Selected research focus",
        )
        print("    - Created research plan")

    # Step 2: Real local documentation search
    with span("docs_search", kind="tool") as s:
        args = {"query": query, "root": str(DOC_SEARCH_ROOT.relative_to(REPO_ROOT)), "limit": 4}
        docs_results = search_docs(query)
        s.set_tool_call(
            "docs_search",
            args,
            {"matches": docs_results},
            has_external_side_effect=False,
        )
        s.metric("match_count", len(docs_results), payload={"stage": "retrieval"})
        print("    - Searched current docs-site concepts")

    # Step 3: Synthesis
    with span("synthesize_results", kind="llm") as s:
        messages = [
            {
                "role": "system",
                "content": (
                    "You synthesize retrieved project docs for a demo trace. "
                    "Return concise JSON with keys synthesis, risks, and confidence."
                ),
            },
            {
                "role": "user",
                "content": json.dumps(
                    {"query": query, "plan": plan_data, "retrieved_docs": docs_results},
                    ensure_ascii=False,
                ),
            },
        ]
        s.set_input({"messages": messages})
        synthesis = call_agent_model(
            purpose="research_synthesis",
            messages=messages,
            response_format={"type": "json_object"} if AGENT_MODE == "openai" else None,
        )
        synthesis_data = parse_json_object(synthesis.content)
        s.set_llm_response(
            synthesis.model,
            messages,
            synthesis_data,
            tokens_in=synthesis.prompt_tokens,
            tokens_out=synthesis.completion_tokens,
            provider=synthesis.provider,
        )
        confidence = synthesis_data.get("confidence", 0.78)
        if isinstance(confidence, (int, float)):
            s.metric("confidence_score", confidence, payload={"stage": "synthesis"})
        s.snapshot_marker(
            "Research synthesis complete",
            marker_kind="agent_step",
            payload={"source_count": len(docs_results)},
        )
        print("    - Synthesized results")

    return {"status": "completed", "query": query, "retrieved_sources": len(docs_results)}


@trace(name="code_review_agent")
def run_code_review_agent(target_path: Path = CODE_REVIEW_TARGET):
    """
    Run a code review agent that:
    1. Analyzes code structure
    2. Checks for issues
    3. Provides recommendations
    """
    print("  Starting code review agent")
    analysis: dict[str, Any]
    issue_scan: dict[str, Any]

    # Analysis chain
    with span("code_analysis_chain", kind="chain"):

        with span("parse_code", kind="tool") as s:
            analysis = analyze_python_source(target_path)
            s.set_tool_call(
                "parse_python_source",
                {"path": str(target_path.relative_to(REPO_ROOT))},
                analysis,
                has_external_side_effect=False,
            )
            print("    - Parsed code structure")

        with span("local_quality_scan", kind="tool") as s:
            sample = str(analysis.get("sample", ""))
            issue_scan = {
                "long_lines": sum(1 for line in sample.splitlines() if len(line) > 100),
                "uses_contextvars": "ContextVar" in sample,
                "sends_trace_start_before_user_input": "_send_trace_start" in sample,
            }
            s.set_tool_call(
                "local_quality_scan",
                {"path": analysis["path"]},
                issue_scan,
                has_external_side_effect=False,
            )
            print("    - Ran local quality scan")

    # Generate review
    with span("generate_review", kind="llm") as s:
        messages = [
            {
                "role": "system",
                "content": (
                    "You are reviewing a Python SDK tracing module. Return JSON with "
                    "keys summary, findings, and score."
                ),
            },
            {
                "role": "user",
                "content": json.dumps(
                    {"static_analysis": analysis, "quality_scan": issue_scan},
                    ensure_ascii=False,
                ),
            },
        ]
        s.set_input({"messages": messages})
        review = call_agent_model(
            purpose="code_review",
            messages=messages,
            response_format={"type": "json_object"} if AGENT_MODE == "openai" else None,
        )
        review_data = parse_json_object(review.content)
        s.set_llm_response(
            review.model,
            messages,
            review_data,
            tokens_in=review.prompt_tokens,
            tokens_out=review.completion_tokens,
            provider=review.provider,
        )
        s.decision(
            "Should this module pass the demo review?",
            "pass_with_notes",
            alternatives=["fail", "pass_with_notes", "pass"],
            reasoning=str(review_data.get("summary", review.content))[:600],
            message="Completed code review decision",
        )
        print("    - Generated review")

    return {"status": "completed", "target": str(target_path.relative_to(REPO_ROOT))}


@trace(name="failing_agent")
def run_failing_agent():
    """Run an incident agent against an unavailable dependency."""
    print("  Starting failing agent (real dependency probe)")
    error_message = "Dependency health check failed"

    with span("initial_step", kind="llm") as s:
        messages = [
            {
                "role": "system",
                "content": (
                    "You are an incident triage agent. Return JSON with first_check "
                    "and rationale."
                ),
            },
            {
                "role": "user",
                "content": "Plan the first dependency check for a stalled agent workflow.",
            },
        ]
        s.set_input({"messages": messages})
        triage = call_agent_model(
            purpose="incident_triage_plan",
            messages=messages,
            response_format={"type": "json_object"} if AGENT_MODE == "openai" else None,
        )
        triage_data = parse_json_object(triage.content)
        s.set_llm_response(
            triage.model,
            messages,
            triage_data,
            tokens_in=triage.prompt_tokens,
            tokens_out=triage.completion_tokens,
            provider=triage.provider,
        )
        s.decision(
            "Which dependency check should run first?",
            triage_data.get("first_check", "health_probe"),
            reasoning=str(triage_data.get("rationale", triage.content))[:500],
            message="Planned dependency probe",
        )

    with span("error_step", kind="tool") as s:
        probe = probe_unavailable_dependency()
        s.set_tool_call(
            "dependency_health_probe",
            {"target": probe["target"]},
            probe,
            has_external_side_effect=False,
        )
        s.wait(
            "dependency_health",
            phase="resolved",
            resolution="failed",
            wait_id=f"dependency-probe-{DEMO_RUN_ID}",
            payload=probe,
        )
        if probe["status"] == "failed":
            exc = TimeoutError(error_message)
            s.error(error_message, payload=probe)
            s.exception(exc, payload=probe)
            s.set_error(error_message)
            print("    - Dependency probe failed (expected)")
        else:
            print("    - Dependency probe unexpectedly succeeded")

    raise TimeoutError(error_message)


def api_headers() -> dict[str, str]:
    """Build API headers for verification requests."""
    return {"X-API-Key": API_KEY}


def resolve_api_key() -> str:
    """Resolve a working local demo API key.

    If CONTINUA_API_KEY is unset, bootstrap a local project against an
    Auth0-disabled development server. Hosted or authenticated deployments should
    pass CONTINUA_API_KEY explicitly.
    """
    candidates = [API_KEY]

    seen = set()
    deduped_candidates = []
    for candidate in candidates:
        if candidate and candidate not in seen:
            deduped_candidates.append(candidate)
            seen.add(candidate)

    with httpx.Client(timeout=5.0) as client:
        if not deduped_candidates:
            response = client.post(
                f"{API_URL}/api/projects",
                json={"name": f"e2e-demo-{DEMO_RUN_ID}"},
            )
            if response.status_code != 201:
                raise RuntimeError(
                    "CONTINUA_API_KEY is required. Create a project in the debugger UI "
                    "and use the generated project API key."
                )
            try:
                payload = response.json()
            except ValueError as exc:
                raise RuntimeError("Project creation response did not contain JSON") from exc
            created_key = str(payload.get("api_key", "")).strip()
            if not created_key:
                raise RuntimeError("Project creation response did not include api_key")
            return created_key

        for candidate in deduped_candidates:
            try:
                response = client.get(
                    f"{API_URL}/api/projects",
                    headers={"X-API-Key": candidate},
                )
            except httpx.RequestError:
                continue

            if response.status_code != 200:
                continue

            expected_project_id = (
                os.environ.get("CONTINUA_DEMO_PROJECT_ID")
                or os.environ.get("PUBLIC_DEMO_PROJECT_ID")
                or ""
            ).strip()
            if expected_project_id:
                try:
                    payload = response.json()
                except ValueError:
                    continue
                project_ids = {
                    str(project.get("id", ""))
                    for project in payload.get("projects", [])
                    if isinstance(project, dict)
                }
                if expected_project_id not in project_ids:
                    continue

            return candidate

    raise RuntimeError("CONTINUA_API_KEY was provided but did not validate against the API server")


def printable_api_key(api_key: str) -> str:
    if PRINT_API_KEY:
        return api_key
    if not api_key:
        return "<unset>"
    return "<redacted>"


def fetch_json(
    client: httpx.Client,
    path: str,
    *,
    params: dict[str, str | int] | None = None,
) -> dict:
    """Fetch and decode a JSON document from the Continua API."""
    response = client.get(f"{API_URL}{path}", headers=api_headers(), params=params)
    response.raise_for_status()
    return response.json()


def verify_demo_data() -> bool:
    """Verify the demo sessions, traces, and timeline events exist."""
    print("\n" + "=" * 60)
    print("VERIFICATION: Querying API for demo sessions and traces")
    print("=" * 60)

    try:
        with httpx.Client(timeout=10.0) as client:
            sessions_payload = fetch_json(
                client,
                "/api/sessions",
                params={"limit": 20, "offset": 0},
            )
            sessions = {
                session_obj["external_id"]: session_obj
                for session_obj in sessions_payload.get("sessions", [])
                if session_obj.get("external_id") in SESSION_IDS
            }

            missing_sessions = [
                session_id for session_id in SESSION_IDS if session_id not in sessions
            ]
            if missing_sessions:
                print(f"ERROR: Missing demo sessions: {missing_sessions}")
                return False

            print("\nCreated sessions:")
            print("-" * 60)

            first_trace_id: str | None = None
            for session_id in SESSION_IDS:
                session_obj = sessions[session_id]
                print(
                    f"  • {session_obj['external_id']} "
                    f"(trace_count={session_obj.get('trace_count', 0)})"
                )
                traces_payload = fetch_json(
                    client,
                    "/api/traces",
                    params={
                        "session_id": session_obj["id"],
                        "limit": 10,
                        "offset": 0,
                    },
                )
                traces = traces_payload.get("traces", [])
                if not traces:
                    print("    ERROR: Session exists but no traces were returned")
                    return False

                for trace_obj in traces:
                    status = trace_obj.get("status", "UNKNOWN")
                    status_icon = "✓" if status == "COMPLETED" else "✗"
                    print(f"    {status_icon} {trace_obj['name']} ({status})")
                    if first_trace_id is None:
                        first_trace_id = trace_obj["id"]

            if first_trace_id is None:
                print("ERROR: No trace ID available to verify timeline events")
                return False

            timeline_payload = fetch_json(
                client,
                f"/api/traces/{first_trace_id}/events",
                params={"limit": 20},
            )
            events = timeline_payload.get("events", [])
            print("-" * 60)
            print(f"Timeline check: trace {first_trace_id[:8]}... returned {len(events)} event(s)")

            return len(events) > 0

    except httpx.HTTPError as e:
        print(f"ERROR: Failed to verify demo data: {e}")
        return False


def seed_engine_demo_run(
    *,
    definition_name: str,
    instance_key: str,
    request_key: str,
    session_key: str,
    trace_name: str,
    input_payload: dict[str, Any],
) -> str:
    """Create an engine-projected trace through the public preview API."""
    engine = EngineControlClient(endpoint=API_URL, api_key=API_KEY)
    try:
        response = engine.start(
            {
                "instance_key": instance_key,
                "definition_name": definition_name,
                "definition_version": "v1",
                "request_key": request_key,
                "input": input_payload,
                "session": {
                    "key": session_key,
                    "name": f"Real agent demo: {session_key}",
                    "metadata": {
                        "demo": True,
                        "run_id": DEMO_RUN_ID,
                        "agent_mode": AGENT_MODE,
                    },
                },
                "trace": {
                    "name": trace_name,
                    "user_id": "demo-user",
                    "tags": ["engine", "demo", "real-agent"],
                    "environment": "demo",
                    "release": "public-demo",
                    "metadata": {
                        "demo": True,
                        "seeded_by": "sdks/python/examples/e2e_demo.py",
                        "agent_mode": AGENT_MODE,
                    },
                },
            }
        )
    finally:
        engine.close()
    return response.trace_id


def main():
    """Run the E2E demo."""
    global API_KEY

    require_real_agent_backend()
    API_KEY = resolve_api_key()

    print("=" * 60)
    print("Continua SDK End-to-End Demo")
    print("=" * 60)
    print(f"API URL: {API_URL}")
    print(f"API key: {printable_api_key(API_KEY)}")
    print(f"Demo run ID: {DEMO_RUN_ID}")
    print(f"Agent mode: {AGENT_MODE}")
    print(f"Time: {datetime.now().isoformat()}")
    print()

    # Initialize SDK
    print("1. Initializing SDK...")
    client = Continua.init(
        api_key=API_KEY,
        endpoint=API_URL,
        flush_interval=1.0,  # Flush every second for demo
        batch_size=10,
    )
    print("   SDK initialized successfully")
    print()

    # Run demo agents
    print("2. Running demo agents...")
    print()

    print(f"  [Session 1: {SESSION_IDS[0]}]")
    with session(SESSION_IDS[0]):
        print("  [Agent 1: Research Agent]")
        result1 = run_research_agent("What are the benefits of AI observability?")
        print(f"  Result: {result1}")
        if SEED_ENGINE_RUNS:
            trace_id = seed_engine_demo_run(
                definition_name="agent.research",
                instance_key=f"demo-{DEMO_RUN_ID}-research-agent",
                request_key=f"demo-{DEMO_RUN_ID}-research-agent-start",
                session_key=SESSION_IDS[0],
                trace_name="engine: research agent orchestration",
                input_payload={
                    "query": "What are the benefits of AI observability?",
                    "agent_result": result1,
                },
            )
            print(f"    - Created engine run trace {trace_id}")
        print()

        print("  [Agent 2: Code Review Agent]")
        result2 = run_code_review_agent(CODE_REVIEW_TARGET)
        print(f"  Result: {result2}")
        if SEED_ENGINE_RUNS:
            trace_id = seed_engine_demo_run(
                definition_name="agent.code_review",
                instance_key=f"demo-{DEMO_RUN_ID}-code-review-agent",
                request_key=f"demo-{DEMO_RUN_ID}-code-review-agent-start",
                session_key=SESSION_IDS[0],
                trace_name="engine: code review agent orchestration",
                input_payload={"target": result2.get("target"), "agent_result": result2},
            )
            print(f"    - Created engine run trace {trace_id}")
        print()

    print(f"  [Session 2: {SESSION_IDS[1]}]")
    with session(SESSION_IDS[1]):
        print("  [Agent 3: Failing Agent]")
        try:
            run_failing_agent()
        except TimeoutError as exc:
            result3 = {"status": "failed", "error": str(exc)}
        print(f"  Result: {result3}")
        if SEED_ENGINE_RUNS:
            trace_id = seed_engine_demo_run(
                definition_name="agent.incident_response",
                instance_key=f"demo-{DEMO_RUN_ID}-incident-agent",
                request_key=f"demo-{DEMO_RUN_ID}-incident-agent-start",
                session_key=SESSION_IDS[1],
                trace_name="engine: incident response agent orchestration",
                input_payload={"agent_result": result3},
            )
            print(f"    - Created engine run trace {trace_id}")
        print()

    # Shutdown SDK (flushes remaining data)
    print("3. Shutting down SDK (flushing data)...")
    client.shutdown()
    print("   SDK shutdown complete")

    # Wait for data to be processed
    print("\n4. Waiting for server to process data...")
    time.sleep(2)

    # Verify
    print("\n5. Verifying traces, sessions, and timeline...")
    success = verify_demo_data()

    print("\n" + "=" * 60)
    if success:
        print("SUCCESS: E2E demo completed!")
        print("\nNext steps:")
        print(f"  1. Open {APP_URL}/traces in your browser")
        print("  2. Enter your local project API key")
        print("  3. Open /sessions to see the new session groups")
        print(f"  4. Look for session IDs starting with demo-sdk-{DEMO_RUN_ID}")
    else:
        print("WARNING: Demo completed but verification failed")
        print(f"Check that the server is running on {API_URL}")
    print("=" * 60)
    if not success:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
