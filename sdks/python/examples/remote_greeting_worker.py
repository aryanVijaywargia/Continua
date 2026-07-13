#!/usr/bin/env python3
"""Run the Python activity worker for the remote greeter example.

Runbook: /guides/go-workflows-python-activities
"""

from __future__ import annotations

import os

from continua import ActivityWorker


def compose_greeting(input: dict[str, object]) -> dict[str, str]:
    """Compose the greeting returned to the Go workflow."""
    name = input.get("name") or "world"
    return {"greeting": f"hello, {name}"}


def build_worker(*, api_key: str, endpoint: str) -> ActivityWorker:
    """Build a worker registered for the remote greeter activity."""
    worker = ActivityWorker(api_key=api_key, endpoint=endpoint)
    worker.register("examples.compose-greeting", compose_greeting)
    return worker


def main() -> None:
    """Run the blocking worker poll loop from environment configuration."""
    api_key = os.environ.get("CONTINUA_API_KEY")
    if not api_key:
        raise RuntimeError("CONTINUA_API_KEY is required")
    endpoint = os.environ.get("CONTINUA_ENDPOINT", "http://localhost:8080")
    build_worker(api_key=api_key, endpoint=endpoint).run()


if __name__ == "__main__":
    main()
