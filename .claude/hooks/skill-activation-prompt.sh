#!/bin/bash
# skill-activation-prompt.sh
# UserPromptSubmit hook: Scans prompt for keywords, injects matching skill context
# ONLY injects each skill ONCE per session to avoid duplicate context

# Read JSON input from stdin
INPUT=$(cat 2>/dev/null) || INPUT="{}"

# Check if jq is available
if ! command -v jq &> /dev/null; then
  exit 0
fi

# Parse input
PROMPT=$(echo "$INPUT" | jq -r '.prompt // empty' 2>/dev/null) || PROMPT=""
SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // "default"' 2>/dev/null) || SESSION_ID="default"

# Exit silently if no prompt
if [ -z "$PROMPT" ]; then
  exit 0
fi

# Determine paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd)" || exit 0
SKILLS_DIR="$SCRIPT_DIR/../skills"
RULES_FILE="$SKILLS_DIR/skill-rules.json"
STATE_DIR="$SCRIPT_DIR/state"
SESSION_FILE="$STATE_DIR/skills-loaded-${SESSION_ID}.txt"

# Exit if no rules file
if [ ! -f "$RULES_FILE" ]; then
  exit 0
fi

# Create state directory if needed
mkdir -p "$STATE_DIR" 2>/dev/null || true

# Convert prompt to lowercase for matching
PROMPT_LOWER=$(echo "$PROMPT" | tr '[:upper:]' '[:lower:]')

# Load all rules once into variables (optimization)
SKILLS_JSON=$(cat "$RULES_FILE" 2>/dev/null) || exit 0
SKILL_NAMES=$(echo "$SKILLS_JSON" | jq -r '.skills | keys[]' 2>/dev/null) || exit 0

# Function to check if skill was already loaded this session
is_skill_loaded() {
  local skill="$1"
  if [ -f "$SESSION_FILE" ]; then
    grep -q "^${skill}$" "$SESSION_FILE" 2>/dev/null && return 0
  fi
  return 1
}

# Function to mark skill as loaded
mark_skill_loaded() {
  local skill="$1"
  echo "$skill" >> "$SESSION_FILE" 2>/dev/null || true
}

# Function to check if prompt matches any keyword
check_keywords() {
  local skill_name="$1"
  local keywords
  keywords=$(echo "$SKILLS_JSON" | jq -r ".skills[\"$skill_name\"].promptTriggers.keywords // [] | .[]" 2>/dev/null) || return 1

  while IFS= read -r keyword; do
    [ -z "$keyword" ] && continue
    keyword_lower=$(echo "$keyword" | tr '[:upper:]' '[:lower:]')
    if echo "$PROMPT_LOWER" | grep -qF "$keyword_lower" 2>/dev/null; then
      return 0
    fi
  done <<< "$keywords"
  return 1
}

# Function to check if prompt matches any intent pattern
check_intent() {
  local skill_name="$1"
  local patterns
  patterns=$(echo "$SKILLS_JSON" | jq -r ".skills[\"$skill_name\"].promptTriggers.intentPatterns // [] | .[]" 2>/dev/null) || return 1

  while IFS= read -r pattern; do
    [ -z "$pattern" ] && continue
    if echo "$PROMPT_LOWER" | grep -qiE "$pattern" 2>/dev/null; then
      return 0
    fi
  done <<< "$patterns"
  return 1
}

# Find matching skill (priority: high first)
MATCHED_SKILL=""
MATCHED_PRIORITY="low"

while IFS= read -r skill; do
  [ -z "$skill" ] && continue

  # Skip if already loaded this session
  if is_skill_loaded "$skill"; then
    continue
  fi

  if check_keywords "$skill" || check_intent "$skill"; then
    priority=$(echo "$SKILLS_JSON" | jq -r ".skills[\"$skill\"].priority // \"medium\"" 2>/dev/null) || priority="medium"

    if [ -z "$MATCHED_SKILL" ]; then
      MATCHED_SKILL="$skill"
      MATCHED_PRIORITY="$priority"
    elif [ "$priority" = "high" ] && [ "$MATCHED_PRIORITY" != "high" ]; then
      MATCHED_SKILL="$skill"
      MATCHED_PRIORITY="$priority"
    fi
  fi
done <<< "$SKILL_NAMES"

# If a skill matched, inject its content
if [ -n "$MATCHED_SKILL" ]; then
  SKILL_PATH=$(echo "$SKILLS_JSON" | jq -r ".skills[\"$MATCHED_SKILL\"].skillPath // empty" 2>/dev/null) || SKILL_PATH=""

  if [ -n "$SKILL_PATH" ] && [ -n "$CLAUDE_PROJECT_DIR" ]; then
    FULL_PATH="$CLAUDE_PROJECT_DIR/$SKILL_PATH"

    if [ -f "$FULL_PATH" ]; then
      # Mark as loaded BEFORE outputting (prevents race conditions)
      mark_skill_loaded "$MATCHED_SKILL"

      echo ""
      echo "📚 Auto-loaded skill: $MATCHED_SKILL (loaded once per session)"
      echo "---"
      cat "$FULL_PATH" 2>/dev/null || true
      echo ""
      echo "---"
    fi
  fi
fi

exit 0
