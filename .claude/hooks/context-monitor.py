#!/usr/bin/env python3
"""PostToolUse hook: Monitor context usage and warn when high.

Adapted from: reference-repos/claude-codepro/.claude/hooks/context_monitor.py
Relevance Score: 4 (general utility - helps manage long sessions)

Purpose: Warns when context usage is high, prompting handoff before context runs out
Event: PostToolUse
Exit: 0 = normal, 2 = warning (stderr sent to Claude)
"""
import json
import sys
import time
from pathlib import Path

# Thresholds (percentage of max context)
THRESHOLD_WARN = 80
THRESHOLD_CRITICAL = 90

# Max context tokens (approximate for Claude)
MAX_CONTEXT_TOKENS = 200000

# Cache to avoid recalculating too frequently
CACHE_FILE = Path("/tmp/.claude_context_cache.json")
CACHE_TTL = 30  # seconds

YELLOW = "\033[0;33m"
RED = "\033[0;31m"
NC = "\033[0m"


def get_current_session_id() -> str:
    """Get current session ID from Claude history."""
    history = Path.home() / ".claude" / "history.jsonl"
    if not history.exists():
        return ""
    try:
        with history.open() as f:
            lines = f.readlines()
            if lines:
                return json.loads(lines[-1]).get("sessionId", "")
    except (json.JSONDecodeError, OSError):
        pass
    return ""


def find_session_file(session_id: str) -> Path | None:
    """Find session file for given session ID."""
    projects_dir = Path.home() / ".claude" / "projects"
    if not projects_dir.exists():
        return None
    for project_dir in projects_dir.iterdir():
        if project_dir.is_dir():
            session_file = project_dir / f"{session_id}.jsonl"
            if session_file.exists():
                return session_file
    return None


def get_token_count(session_file: Path) -> int | None:
    """Get token count from session file usage data."""
    last_usage = None

    try:
        with session_file.open() as f:
            for line in f:
                try:
                    msg = json.loads(line)
                    if msg.get("type") != "assistant":
                        continue

                    message = msg.get("message", {})
                    if not isinstance(message, dict):
                        continue

                    usage = message.get("usage")
                    if usage:
                        last_usage = usage
                except (json.JSONDecodeError, KeyError):
                    continue
    except OSError:
        return None

    if not last_usage:
        return None

    input_tokens = last_usage.get("input_tokens", 0)
    cache_creation = last_usage.get("cache_creation_input_tokens", 0)
    cache_read = last_usage.get("cache_read_input_tokens", 0)

    return input_tokens + cache_creation + cache_read


def get_cached_context(session_id: str) -> tuple:
    """Get cached context value if still valid."""
    if CACHE_FILE.exists():
        try:
            with CACHE_FILE.open() as f:
                cache = json.load(f)
                if (
                    cache.get("session_id") == session_id
                    and time.time() - cache.get("timestamp", 0) < CACHE_TTL
                ):
                    return cache.get("tokens", 0), True
        except (json.JSONDecodeError, OSError):
            pass
    return 0, False


def save_cache(tokens: int, session_id: str) -> None:
    """Save context calculation to cache."""
    try:
        with CACHE_FILE.open("w") as f:
            json.dump(
                {"tokens": tokens, "timestamp": time.time(), "session_id": session_id},
                f,
            )
    except OSError:
        pass


def main():
    session_id = get_current_session_id()
    if not session_id:
        sys.exit(0)

    # Check cache first
    cached_tokens, is_cached = get_cached_context(session_id)
    if is_cached:
        total_tokens = cached_tokens
    else:
        session_file = find_session_file(session_id)
        if not session_file:
            sys.exit(0)

        actual_tokens = get_token_count(session_file)
        if actual_tokens is None:
            sys.exit(0)

        total_tokens = actual_tokens
        save_cache(total_tokens, session_id)

    percentage = (total_tokens / MAX_CONTEXT_TOKENS) * 100

    if percentage >= THRESHOLD_CRITICAL:
        print("", file=sys.stderr)
        print(f"{RED}CONTEXT {percentage:.0f}% - Consider saving state{NC}", file=sys.stderr)
        print(f"{RED}Current work should be wrapped up soon.{NC}", file=sys.stderr)
        print(f"{RED}Consider summarizing progress before context limit.{NC}", file=sys.stderr)
        sys.exit(2)

    if percentage >= THRESHOLD_WARN:
        print("", file=sys.stderr)
        print(
            f"{YELLOW}Context: {percentage:.0f}% - Finish current task, then consider wrapping up{NC}",
            file=sys.stderr,
        )
        sys.exit(2)

    sys.exit(0)


if __name__ == "__main__":
    main()
