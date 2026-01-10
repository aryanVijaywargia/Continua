#!/usr/bin/env python3
"""PreToolUse hook: block edits to protected files and generated outputs."""
import json
import os
import re
import sys
import time
from pathlib import Path

WARNED_CACHE_FILE = Path("/tmp/.continua_protected_warned.json")
OVERRIDE_TIMEOUT = 60

PROTECTED_PATTERNS = [
    r"/\.env(\.|$)",
    r"/go\.(mod|sum)$",
    r"/pnpm-lock\.yaml$",
    r"/package-lock\.json$",
    r"/yarn\.lock$",
    r"/contracts/generated/",
    r"/internal/api/server_gen\.go$",
    r"/db/platform/gen/",
    r"/engine/db/gen/",
    r"/web/dist/",
]

ASK_PATTERNS = [
    r"/db/platform/sqlc\.yaml$",
    r"/engine/db/sqlc\.yaml$",
    r"/contracts/openapi/openapi\.yaml$",
]


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


def is_protected(file_path: str) -> tuple[str, str | None]:
    normalized = file_path.replace("\\", "/")

    for pattern in PROTECTED_PATTERNS:
        if re.search(pattern, normalized):
            if check_override(file_path):
                return ("allow", None)

            record_warning(file_path)

            if "/contracts/generated/" in normalized or normalized.endswith("server_gen.go"):
                reason = (
                    "BLOCKED: Generated code should not be edited directly.\n\n"
                    "Edit the source schema and run 'make generate'.\n"
                    "(Retry to proceed anyway)"
                )
            elif normalized.endswith(".env") or "/.env." in normalized:
                reason = (
                    "BLOCKED: .env files contain sensitive data.\n\n"
                    "Edit them manually outside Claude Code.\n"
                    "(Retry to proceed anyway)"
                )
            elif normalized.endswith("go.mod") or normalized.endswith("go.sum"):
                reason = (
                    "BLOCKED: Go module files require managed updates.\n\n"
                    "Use 'go get' or 'go mod tidy'.\n"
                    "(Retry to proceed anyway)"
                )
            else:
                reason = (
                    "BLOCKED: Protected file.\n\n"
                    "Proceed only if you understand the impact.\n"
                    "(Retry to proceed anyway)"
                )
            return ("deny", reason)

    for pattern in ASK_PATTERNS:
        if re.search(pattern, normalized):
            return ("ask", f"Editing {os.path.basename(file_path)} - please confirm")

    return ("allow", None)


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
    if not file_path:
        print(json.dumps({"decision": "approve"}))
        sys.exit(0)

    decision, reason = is_protected(file_path)
    if decision == "deny":
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
    elif decision == "ask":
        print(
            json.dumps(
                {
                    "hookSpecificOutput": {
                        "hookEventName": "PreToolUse",
                        "permissionDecision": "ask",
                        "permissionDecisionReason": reason,
                    }
                }
            )
        )
    else:
        print(json.dumps({"decision": "approve"}))


if __name__ == "__main__":
    main()
