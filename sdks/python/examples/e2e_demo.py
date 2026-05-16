#!/usr/bin/env python3
"""
End-to-End Demo for Continua Python SDK

This script demonstrates:
1. SDK initialization
2. Creating traces with nested spans
3. Simulating LLM, tool, and agent spans
4. Verifying data via API

Usage:
    cd sdks/python
    uv run python examples/e2e_demo.py
"""

import os
import random
import sys
import time
from datetime import datetime
from pathlib import Path

import httpx

# Keep the demo runnable from a source checkout even if the SDK package has not
# been installed into the active environment.
SDK_SRC = Path(__file__).resolve().parents[1] / "src"
if str(SDK_SRC) not in sys.path:
    sys.path.insert(0, str(SDK_SRC))

# Import the SDK
from continua import Continua, session, span, trace


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
SESSION_IDS = [
    f"demo-sdk-{DEMO_RUN_ID}-research",
    f"demo-sdk-{DEMO_RUN_ID}-incident",
]
SAMPLE_CODE = """
def hello_world():
    print("Hello, World!")
    return True
"""


def simulate_llm_call(prompt: str) -> str:
    """Simulate an LLM API call with realistic latency."""
    time.sleep(random.uniform(0.1, 0.3))  # Simulate network latency
    responses = [
        "I'll help you with that task.",
        "Based on my analysis, here's what I found...",
        "Let me process that information for you.",
        "Here's the result of the computation.",
    ]
    return random.choice(responses)


def simulate_tool_call(tool_name: str, args: dict) -> dict:
    """Simulate a tool execution."""
    time.sleep(random.uniform(0.05, 0.15))
    return {"status": "success", "tool": tool_name, "result": f"Executed {tool_name}"}


@trace(name="research_agent")
def run_research_agent(query: str):
    """
    Simulate a research agent that:
    1. Plans the research
    2. Searches for information
    3. Synthesizes results
    """
    print(f"  Starting research agent for: {query}")

    # Step 1: Planning span
    with span("plan_research", kind="llm") as s:
        s.set_input({"query": query, "instruction": "Create a research plan"})
        s.log("Planning research steps", payload={"query": query})
        plan = simulate_llm_call(f"Plan research for: {query}")
        s.set_output({"plan": plan})
        s.set_tokens(prompt=50, completion=100)
        s.set_model("gpt-4", provider="openai")
        print("    - Created research plan")

    # Step 2: Search tool calls
    with span("web_search", kind="tool") as s:
        s.set_input({"query": query, "num_results": 5})
        results = simulate_tool_call("web_search", {"query": query})
        s.set_output(results)
        print("    - Executed web search")

    with span("database_lookup", kind="tool") as s:
        s.set_input({"query": query, "database": "knowledge_base"})
        results = simulate_tool_call("database_lookup", {"query": query})
        s.set_output(results)
        print("    - Executed database lookup")

    # Step 3: Synthesis
    with span("synthesize_results", kind="llm") as s:
        s.set_input({"sources": ["web", "database"], "query": query})
        synthesis = simulate_llm_call("Synthesize all gathered information")
        s.set_output({"synthesis": synthesis, "confidence": 0.85})
        s.set_tokens(prompt=200, completion=300)
        s.set_model("gpt-4", provider="openai")
        s.metric("confidence_score", 0.85, payload={"stage": "synthesis"})
        print("    - Synthesized results")

    return {"status": "completed", "query": query}


@trace(name="code_review_agent")
def run_code_review_agent(code: str):
    """
    Simulate a code review agent that:
    1. Analyzes code structure
    2. Checks for issues
    3. Provides recommendations
    """
    print("  Starting code review agent")

    # Analysis chain
    with span("code_analysis_chain", kind="chain"):

        with span("parse_code", kind="tool") as s:
            s.set_input({"code": code[:100] + "..."})
            time.sleep(0.05)
            s.set_output({"parsed": True, "lines": 50, "functions": 3})
            print("    - Parsed code structure")

        with span("security_scan", kind="tool") as s:
            s.set_input({"scan_type": "security"})
            time.sleep(0.1)
            s.set_output({"vulnerabilities": 0, "warnings": 2})
            print("    - Completed security scan")

        with span("style_check", kind="tool") as s:
            s.set_input({"style_guide": "PEP8"})
            time.sleep(0.05)
            s.set_output({"issues": 5, "auto_fixable": 3})
            print("    - Checked code style")

    # Generate review
    with span("generate_review", kind="llm") as s:
        s.set_input({"analysis_results": "combined"})
        review = simulate_llm_call("Generate code review")
        s.set_output({"review": review, "score": 8.5})
        s.set_tokens(prompt=150, completion=250)
        s.set_model("claude-3-opus", provider="anthropic")
        print("    - Generated review")

    return {"status": "completed", "score": 8.5}


@trace(name="failing_agent")
def run_failing_agent():
    """Simulate an agent that encounters an error."""
    print("  Starting failing agent (intentional error)")
    error_message = "Failed to reach external service"

    with span("initial_step", kind="llm") as s:
        s.set_input({"task": "process data"})
        simulate_llm_call("Start processing")
        s.set_output({"status": "ok"})
        s.set_tokens(prompt=20, completion=30)

    with span("error_step", kind="tool") as s:
        s.set_input({"operation": "risky_operation"})
        time.sleep(0.1)
        # Simulate an error
        try:
            raise TimeoutError(error_message)
        except TimeoutError as exc:
            s.error("Connection timeout", payload={"operation": "risky_operation"})
            s.exception(exc, payload={"operation": "risky_operation"})
            s.set_error("Connection timeout: Failed to reach external service")
        s.set_output({"status": "failed"})
        print("    - Encountered error (expected)")

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

            missing_sessions = [session_id for session_id in SESSION_IDS if session_id not in sessions]
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


def main():
    """Run the E2E demo."""
    global API_KEY

    random.seed(42)
    API_KEY = resolve_api_key()

    print("=" * 60)
    print("Continua SDK End-to-End Demo")
    print("=" * 60)
    print(f"API URL: {API_URL}")
    print(f"API key: {printable_api_key(API_KEY)}")
    print(f"Demo run ID: {DEMO_RUN_ID}")
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
        print()

        print("  [Agent 2: Code Review Agent]")
        result2 = run_code_review_agent(SAMPLE_CODE)
        print(f"  Result: {result2}")
        print()

    print(f"  [Session 2: {SESSION_IDS[1]}]")
    with session(SESSION_IDS[1]):
        print("  [Agent 3: Failing Agent]")
        try:
            run_failing_agent()
        except TimeoutError as exc:
            result3 = {"status": "failed", "error": str(exc)}
        print(f"  Result: {result3}")
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
