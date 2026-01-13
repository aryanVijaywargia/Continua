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

import time
import random
import httpx
from datetime import datetime

# Import the SDK
from continua import Continua, trace, span


# Configuration
API_URL = "http://localhost:8081"
API_KEY = "test-api-key-12345"


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
    print(f"  Starting code review agent")

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

    with span("initial_step", kind="llm") as s:
        s.set_input({"task": "process data"})
        simulate_llm_call("Start processing")
        s.set_output({"status": "ok"})
        s.set_tokens(prompt=20, completion=30)

    with span("error_step", kind="tool") as s:
        s.set_input({"operation": "risky_operation"})
        time.sleep(0.1)
        # Simulate an error
        s.set_error("Connection timeout: Failed to reach external service")
        s.set_output({"status": "failed"})
        print("    - Encountered error (expected)")

    return {"status": "failed", "error": "Connection timeout"}


def verify_traces():
    """Verify traces were captured by querying the API."""
    print("\n" + "=" * 60)
    print("VERIFICATION: Querying API for traces")
    print("=" * 60)

    try:
        with httpx.Client() as client:
            # Fetch traces
            response = client.get(
                f"{API_URL}/api/traces",
                headers={"X-API-Key": API_KEY},
                params={"limit": 10}
            )
            response.raise_for_status()
            data = response.json()

            traces = data.get("traces", [])
            total = data.get("total", 0)

            print(f"\nFound {total} trace(s) in the system:")
            print("-" * 60)

            for t in traces:
                status_icon = "✓" if t.get("status") in ["COMPLETED", "ok"] else "✗"
                print(f"  {status_icon} {t['name']}")
                print(f"    ID: {t['id'][:8]}...")
                print(f"    Status: {t.get('status', 'unknown')}")
                print(f"    Tokens: {(t.get('total_tokens_in') or 0) + (t.get('total_tokens_out') or 0)}")

                # Fetch spans for this trace
                spans_resp = client.get(
                    f"{API_URL}/api/traces/{t['id']}/spans",
                    headers={"X-API-Key": API_KEY}
                )
                if spans_resp.status_code == 200:
                    spans_data = spans_resp.json()
                    span_count = len(spans_data.get("spans", []))
                    print(f"    Spans: {span_count}")
                print()

            return True

    except httpx.HTTPError as e:
        print(f"ERROR: Failed to verify traces: {e}")
        return False


def main():
    """Run the E2E demo."""
    print("=" * 60)
    print("Continua SDK End-to-End Demo")
    print("=" * 60)
    print(f"API URL: {API_URL}")
    print(f"Time: {datetime.now().isoformat()}")
    print()

    # Initialize SDK
    print("1. Initializing SDK...")
    Continua.init(
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

    print("  [Agent 1: Research Agent]")
    result1 = run_research_agent("What are the benefits of AI observability?")
    print(f"  Result: {result1}")
    print()

    print("  [Agent 2: Code Review Agent]")
    sample_code = """
def hello_world():
    print("Hello, World!")
    return True
"""
    result2 = run_code_review_agent(sample_code)
    print(f"  Result: {result2}")
    print()

    print("  [Agent 3: Failing Agent]")
    result3 = run_failing_agent()
    print(f"  Result: {result3}")
    print()

    # Shutdown SDK (flushes remaining data)
    print("3. Shutting down SDK (flushing data)...")
    Continua.get_instance().shutdown()
    print("   SDK shutdown complete")

    # Wait for data to be processed
    print("\n4. Waiting for server to process data...")
    time.sleep(2)

    # Verify
    print("\n5. Verifying traces...")
    success = verify_traces()

    print("\n" + "=" * 60)
    if success:
        print("SUCCESS: E2E demo completed!")
        print("\nNext steps:")
        print("  1. Open http://localhost:3000/traces in your browser")
        print("  2. Enter API key: test-api-key-12345")
        print("  3. View the traces and click to see span details")
    else:
        print("WARNING: Demo completed but verification failed")
        print("Check that the server is running on port 8081")
    print("=" * 60)


if __name__ == "__main__":
    main()
