#!/bin/bash
set -e

# Post-tool-use hook for Continua (Go monorepo)
# Tracks edited files and their modules for build/test validation
# Runs after Edit, MultiEdit, or Write tools

# Read tool information from stdin
tool_info=$(cat)

# Extract relevant data
tool_name=$(echo "$tool_info" | jq -r '.tool_name // empty')
file_path=$(echo "$tool_info" | jq -r '.tool_input.file_path // empty')
session_id=$(echo "$tool_info" | jq -r '.session_id // empty')

# Skip if not an edit tool or no file path
if [[ ! "$tool_name" =~ ^(Edit|MultiEdit|Write)$ ]] || [[ -z "$file_path" ]]; then
    exit 0
fi

# Skip non-code files
if [[ "$file_path" =~ \.(md|markdown|json|yaml|yml|txt|log)$ ]]; then
    exit 0
fi

# Create cache directory
cache_dir="$CLAUDE_PROJECT_DIR/.claude/hooks/state/${session_id:-default}"
mkdir -p "$cache_dir"

# Detect Continua module from file path
detect_module() {
    local file="$1"
    local project_root="$CLAUDE_PROJECT_DIR"
    local relative_path="${file#$project_root/}"
    local first_dir=$(echo "$relative_path" | cut -d'/' -f1)

    case "$first_dir" in
        # Go backend
        internal)
            local subdir=$(echo "$relative_path" | cut -d'/' -f2)
            echo "internal/$subdir"
            ;;
        cmd)
            echo "cmd"
            ;;
        pkg)
            echo "pkg"
            ;;
        engine)
            echo "engine"
            ;;
        # Database
        db)
            local subdir=$(echo "$relative_path" | cut -d'/' -f2)
            echo "db/$subdir"
            ;;
        # Contracts
        contracts)
            local subdir=$(echo "$relative_path" | cut -d'/' -f2)
            echo "contracts/$subdir"
            ;;
        # SDKs
        sdks)
            local sdk=$(echo "$relative_path" | cut -d'/' -f2)
            echo "sdks/$sdk"
            ;;
        # Web frontend
        web)
            echo "web"
            ;;
        # Root files
        *)
            if [[ ! "$relative_path" =~ / ]]; then
                echo "root"
            else
                echo "$first_dir"
            fi
            ;;
    esac
}

# Get validation command for module
get_validation_command() {
    local module="$1"
    local project_root="$CLAUDE_PROJECT_DIR"

    case "$module" in
        internal/*|cmd|pkg)
            echo "make lint-go && make test-go"
            ;;
        engine)
            echo "cd $project_root/engine && go test ./..."
            ;;
        db/platform)
            echo "make generate"
            ;;
        contracts/*)
            echo "make generate"
            ;;
        sdks/python)
            echo "cd $project_root/sdks/python && uv run pytest"
            ;;
        sdks/typescript)
            echo "cd $project_root/sdks/typescript && pnpm test"
            ;;
        web)
            echo "cd $project_root/web && pnpm type-check && pnpm lint"
            ;;
        *)
            echo ""
            ;;
    esac
}

# Detect module
module=$(detect_module "$file_path")

# Skip if unknown
if [[ -z "$module" ]]; then
    exit 0
fi

# Log edited file with timestamp
echo "$(date +%s)|$file_path|$module" >> "$cache_dir/edited-files.log"

# Track affected modules (unique)
if ! grep -q "^$module$" "$cache_dir/affected-modules.txt" 2>/dev/null; then
    echo "$module" >> "$cache_dir/affected-modules.txt"
fi

# Store validation command
validation_cmd=$(get_validation_command "$module")
if [[ -n "$validation_cmd" ]]; then
    # Store as module:command, avoiding duplicates
    if ! grep -q "^$module:" "$cache_dir/validation-commands.txt" 2>/dev/null; then
        echo "$module:$validation_cmd" >> "$cache_dir/validation-commands.txt"
    fi
fi

# Check if generated files were edited (warning)
if [[ "$file_path" =~ _gen\.go$ ]] || [[ "$file_path" =~ db/gen/ ]] || [[ "$file_path" =~ contracts/generated/ ]]; then
    echo "⚠️  Edited generated file: $file_path" >&2
    echo "   Run 'make generate' to regenerate from source" >&2
fi

exit 0
