#!/bin/bash
set -e

if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

input=$(cat)
file_path=$(echo "$input" | jq -r '.tool_input.file_path // empty')

if [[ -z "$file_path" ]] || [[ ! "$file_path" =~ \.go$ ]]; then
  exit 0
fi

if [[ "$file_path" =~ /generated/ ]] || [[ "$file_path" =~ _gen\.go$ ]] || [[ "$file_path" =~ \.pb\.go$ ]]; then
  exit 0
fi

if [[ "$file_path" =~ /vendor/ ]]; then
  exit 0
fi

if [[ ! -f "$file_path" ]]; then
  exit 0
fi

if command -v gofmt >/dev/null 2>&1; then
  gofmt -w "$file_path" 2>/dev/null || true
fi

if command -v goimports >/dev/null 2>&1; then
  goimports -w "$file_path" 2>/dev/null || true
fi

exit 0
