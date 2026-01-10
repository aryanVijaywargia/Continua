#!/usr/bin/env python3
"""PreToolUse hook: block edits to existing migrations."""
import json
import os
import sys
import time
from pathlib import Path

MIGRATION_DIRS = [
    "db/platform/migrations",
    "engine/db/migrations",
]

WARNED_CACHE_FILE = Path("/tmp/.continua_migration_guard_warned.json")
OVERRIDE_TIMEOUT = 60


def load_warned_cache() -> dict:
    if not WARNED_CACHE_FILE.exists():
        return {}
    try:
        with WARNED_CACHE_FILE.open() as f:
            return json.load(f)
    except (json.JSONDecodeError, OSError):
        return {}


def save_warned_cache(cache: dict) -> None:
    try:
        with WARNED_CACHE_FILE.open("w") as f:
            json.dump(cache, f)
    except OSError:
        pass


def check_override(file_path: str) -> bool:
    cache = load_warned_cache()
    now = time.time()
    cache = {k: v for k, v in cache.items() if now - v < OVERRIDE_TIMEOUT}
    if file_path in cache:
        del cache[file_path]
        save_warned_cache(cache)
        return True
    return False


def record_warning(file_path: str) -> None:
    cache = load_warned_cache()
    now = time.time()
    cache = {k: v for k, v in cache.items() if now - v < OVERRIDE_TIMEOUT}
    cache[file_path] = now
    save_warned_cache(cache)


def is_existing_migration(file_path: str, project_dir: str) -> bool:
    normalized = file_path.replace("\\", "/")
    for migration_dir in MIGRATION_DIRS:
        if f"/{migration_dir}/" in normalized or normalized.startswith(f"{project_dir}/{migration_dir}/"):
            if normalized.endswith(".sql") and os.path.exists(file_path):
                return True
    return False


def main() -> None:
    try:
        data = json.load(sys.stdin)
    except json.JSONDecodeError:
        sys.exit(0)

    tool_name = data.get("tool_name", "")
    if tool_name not in ("Edit", "MultiEdit", "Write"):
        print(json.dumps({"decision": "approve"}))
        sys.exit(0)

    file_path = data.get("tool_input", {}).get("file_path", "")
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR", "")
    if not file_path:
        print(json.dumps({"decision": "approve"}))
        sys.exit(0)

    if is_existing_migration(file_path, project_dir):
        if check_override(file_path):
            print(json.dumps({"decision": "approve"}))
            sys.exit(0)

        record_warning(file_path)
        filename = os.path.basename(file_path)
        reason = (
            f"BLOCKED: Cannot modify existing migration '{filename}'.\n\n"
            "Migrations are immutable once created.\n"
            "Create a new migration instead: make migrate-create name=<description>\n"
            "(Retry to proceed anyway)"
        )
        print(
            json.dumps(
                {
                    "hookSpecificOutput": {
                        "hookEventName": "PreToolUse",
                        "permissionDecision": "deny",
                        "permissionDecisionReason": reason,
                    }
                }
            )
        )
        sys.exit(0)

    print(json.dumps({"decision": "approve"}))


if __name__ == "__main__":
    main()
