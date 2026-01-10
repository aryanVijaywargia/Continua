#!/usr/bin/env python3
"""PreToolUse hook: validate dangerous bash commands for Continua."""
import json
import re
import sys


def check_dangerous_commands(command: str) -> tuple[str, str | None]:
    normalized = " ".join(command.strip().split())

    hard_blocks = [
        (r"rm\s+-[rf]*\s*[/~]$", "BLOCKED: rm -rf on root or home"),
        (r"rm\s+-[rf]*\s*/\*", "BLOCKED: rm with wildcard on root"),
        (r"rm\s+-[rf]*\s+\.\s*$", "BLOCKED: rm -rf on current directory"),
        (r"\bdd\s+if=", "BLOCKED: dd can overwrite disks"),
        (r"\bmkfs\b", "BLOCKED: mkfs formats disks"),
        (r">\s*/dev/", "BLOCKED: writing to device files"),
        (r"git\s+push\s+.*--force.*\s+(main|master)", "BLOCKED: force push to main/master"),
        (r"git\s+push\s+-f.*\s+(main|master)", "BLOCKED: force push to main/master"),
        (r"git\s+reset\s+--hard\s+origin/(main|master)", "BLOCKED: hard reset to main/master"),
    ]

    for pattern, reason in hard_blocks:
        if re.search(pattern, normalized, re.IGNORECASE):
            return ("deny", reason)

    ask_patterns = [
        (r"^sudo\s", "Command requires elevated privileges"),
        (r"^su\s", "Command switches user context"),
        (r"git\s+push\s+.*--force", "Force push may overwrite history"),
        (r"git\s+push\s+-f", "Force push may overwrite history"),
        (r"git\s+reset\s+--hard", "Hard reset discards changes"),
        (r"git\s+clean\s+-[fd]", "git clean removes untracked files"),
        (r"rm\s+-[rf]", "Recursive/force delete - verify target"),
        (r"\bcurl\s.*\|\s*sh", "Piping curl to shell - verify source"),
        (r"\bwget\s.*\|\s*sh", "Piping wget to shell - verify source"),
        (r"docker\s+system\s+prune", "Docker prune removes data"),
    ]

    for pattern, reason in ask_patterns:
        if re.search(pattern, normalized, re.IGNORECASE):
            return ("ask", reason)

    return ("allow", None)


def check_env_access(command: str) -> tuple[str, str | None]:
    normalized = " ".join(command.strip().split())
    env_patterns = [
        r"\bcat\s+.*\.env\b",
        r"\bless\s+.*\.env\b",
        r"\bhead\s+.*\.env\b",
        r"\btail\s+.*\.env\b",
        r"\bgrep\s+.*\.env\b",
        r">\s*\.env\b",
        r">>\s*\.env\b",
    ]
    for pattern in env_patterns:
        if re.search(pattern, normalized, re.IGNORECASE):
            return (
                "deny",
                "BLOCKED: Direct access to .env files is not allowed. Edit them manually outside Claude Code.",
            )
    return ("allow", None)


def check_git_add(command: str) -> tuple[str, str | None]:
    normalized = " ".join(command.strip().split())
    if not normalized.startswith("git add"):
        return ("allow", None)

    if "*" in normalized:
        return (
            "deny",
            "BLOCKED: Wildcards in git add. Use specific file paths.",
        )

    if re.search(r"git\s+add\s+(\.|--all|-[aA])", normalized):
        return (
            "deny",
            "BLOCKED: git add . / -A / --all. Use specific file paths instead.",
        )

    return ("allow", None)


def main() -> None:
    try:
        data = json.load(sys.stdin)
    except json.JSONDecodeError:
        sys.exit(0)

    tool_name = data.get("tool_name", "")
    if tool_name != "Bash":
        print(json.dumps({"decision": "approve"}))
        sys.exit(0)

    command = data.get("tool_input", {}).get("command", "")
    if not command:
        print(json.dumps({"decision": "approve"}))
        sys.exit(0)

    checks = [check_dangerous_commands, check_env_access, check_git_add]
    deny_reasons = []
    ask_reasons = []

    for check in checks:
        decision, reason = check(command)
        if decision == "deny":
            deny_reasons.append(reason)
        elif decision == "ask":
            ask_reasons.append(reason)

    if deny_reasons:
        reason = "\n\n".join([r for r in deny_reasons if r])
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
    elif ask_reasons:
        reason = "; ".join([r for r in ask_reasons if r])
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
