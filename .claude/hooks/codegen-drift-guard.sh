#!/bin/bash
set -e

if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

input=$(cat)

tool_name=$(echo "$input" | jq -r '.tool_name // empty')
file_path=$(echo "$input" | jq -r '.tool_input.file_path // empty')

if [[ ! "$tool_name" =~ ^(Write|Edit|MultiEdit)$ ]] || [[ -z "$file_path" ]]; then
  exit 0
fi

project_dir=${CLAUDE_PROJECT_DIR:-$(pwd)}
if [[ "$file_path" == "$project_dir"/* ]]; then
  rel_path=${file_path#"$project_dir"/}
else
  rel_path=$file_path
fi

case "$rel_path" in
  contracts/openapi/*|contracts/websocket/*|db/platform/queries/*|db/platform/migrations/*|db/platform/sqlc.yaml|engine/db/queries/*|engine/db/migrations/*|engine/db/sqlc.yaml)
    echo ""
    echo "[codegen] Detected change in: $rel_path" >&2
    echo "[codegen] Run: make generate" >&2
    echo "[codegen] Verify: ./scripts/check-generated.sh" >&2
    echo ""
    ;;
  *)
    ;;
esac

exit 0
