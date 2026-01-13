---
name: smart-commit
description: Commit with automatic changelog + skill sync. Detects feat/fix commits, updates CHANGELOG.md, syncs skills, then commits and pushes.
arguments:
  - name: message
    description: Commit message (e.g., "feat: add trace filtering")
    required: true
  - name: skip-changelog
    description: Skip changelog prompt (default: false)
    required: false
---

# Smart Commit

Commits your changes with **automatic changelog generation** and skill synchronization.

## Workflow

1. **Detect changes** - Get list of changed files
2. **Changelog** - If feat/fix commit, generate changelog entry
3. **Skill sync** - Propose skill doc updates if relevant
4. **Commit & push** - Stage everything and push

---

## Step 1: Detect Changed Files

```bash
# Get staged + unstaged changes
STAGED=$(git diff --cached --name-only 2>/dev/null)
UNSTAGED=$(git diff --name-only 2>/dev/null)
CHANGED_FILES=$(echo -e "$STAGED\n$UNSTAGED" | sort -u | grep -v '^$')
```

If no changes detected:
> No changes detected. Stage your changes with `git add` first.

---

## Step 2: Changelog Generation

**Check if this is a notable commit:**

Parse the commit message `$message` for conventional commit prefixes:

| Prefix | Changelog Category |
|--------|-------------------|
| `feat:` | Added |
| `fix:` | Fixed |
| `perf:` | Changed |
| `refactor:` | Changed |
| `security:` | Security |
| `deprecate:` | Deprecated |
| `remove:` | Removed |

**If notable commit AND `$skip-changelog` is not true:**

### 2a. Generate Entry

Analyze the changes and commit message to create a **user-focused** changelog entry:

**Good entries:**
- "Add trace filtering by date range and status"
- "Fix WebSocket reconnection dropping buffered events"
- "Improve span tree query performance for large traces"

**Bad entries (too technical):**
- "Add GetTracesByFilter handler"
- "Fix nil pointer in ws.hub"

**Guidelines:**
- Start with verb (Add, Fix, Improve, Remove)
- Focus on user benefit, not implementation
- One line, concise

### 2b. Show Proposed Entry

```
════════════════════════════════════════════════════════════════
CHANGELOG ENTRY
════════════════════════════════════════════════════════════════

Category: {Added/Fixed/Changed/etc.}

Proposed entry:
  - {generated entry text}

════════════════════════════════════════════════════════════════
```

### 2c. Ask for Approval

```javascript
AskUserQuestion({
  questions: [{
    question: "Add this to CHANGELOG.md?",
    header: "Changelog",
    multiSelect: false,
    options: [
      { label: "Yes", description: "Add to CHANGELOG.md under [Unreleased]" },
      { label: "Edit", description: "Let me modify the entry first" },
      { label: "Skip", description: "Don't add to changelog" }
    ]
  }]
})
```

### 2d. Update CHANGELOG.md

If approved, read CHANGELOG.md and add entry under `[Unreleased]` → appropriate category:

```markdown
## [Unreleased]

### Added
- {new entry here}  ← INSERT AT TOP
- existing entries...
```

**Rules:**
- Add new entries at TOP of category
- Create category subsection if missing
- Keep categories in order: Added, Changed, Deprecated, Fixed, Removed, Security

---

## Step 3: Skill Sync (Optional)

Read the skill mapping configuration:
```
@commands/skill-sync-config.json
```

For each changed file, check which skills are affected. If skills are affected:

### 3a. Analyze with skill-analyzer

```javascript
Task(
  subagent_type: "skill-analyzer",
  prompt: `Analyze code changes and propose skill updates...`,
  description: "Analyze for skill updates"
)
```

### 3b. Show Proposed Updates

```
════════════════════════════════════════════════════════════════
PROPOSED SKILL UPDATES
════════════════════════════════════════════════════════════════

Skill: continua-backend-dev
Resource: api-patterns.md

Proposed addition:
  For batch operations, wrap errors with batch index:
  `return fmt.Errorf("batch item %d: %w", idx, err)`

════════════════════════════════════════════════════════════════
```

### 3c. Ask for Approval

```javascript
AskUserQuestion({
  questions: [{
    question: "Apply skill updates?",
    header: "Skills",
    options: [
      { label: "Yes", description: "Apply proposed updates" },
      { label: "Edit", description: "Review/modify first" },
      { label: "Skip", description: "No skill updates" }
    ]
  }]
})
```

If no skills affected, skip this step entirely.

---

## Step 4: Stage & Commit

```bash
# Stage all changes (code + changelog + skills)
git add -A

# Commit
git commit -m "$message"
```

---

## Step 5: Push

```bash
git push
```

If no upstream:
```bash
BRANCH=$(git branch --show-current)
git push -u origin $BRANCH
```

---

## Step 6: Completion Report

```
✅ Committed and pushed!

Commit: {hash}
Message: $message
Files: {count} changed

Changelog: {Added to [Unreleased] → {category} OR "skipped"}
Skills: {list of updated skills OR "none"}

Branch: {branch} → origin/{branch}
```

---

## Quick Examples

### Feature commit with changelog
```
/smart-commit "feat: add span tree visualization"

→ Detected: feat → Added
→ Proposed changelog: "Add interactive span tree visualization in trace view"
→ [Yes] [Edit] [Skip]
→ User: Yes
→ Updated CHANGELOG.md
→ Checking skills... no updates needed
→ Committed and pushed!
```

### Bug fix
```
/smart-commit "fix: WebSocket drops events on reconnect"

→ Detected: fix → Fixed
→ Proposed changelog: "Fix WebSocket reconnection dropping buffered events"
→ [Yes]
→ Committed and pushed!
```

### Minor change (no changelog)
```
/smart-commit "chore: update dependencies"

→ Not a notable commit type, skipping changelog
→ Committed and pushed!
```

### Skip changelog explicitly
```
/smart-commit "feat: minor tweak" skip-changelog:true

→ Skipping changelog (explicit)
→ Committed and pushed!
```

---

## Edge Cases

### Changelog category already has entries
Insert new entry at the **top** of the category, not bottom.

### Category doesn't exist
Create the subsection in correct order (Added before Changed before Fixed, etc.)

### User edits changelog entry
After "Edit" option:
> Please modify the CHANGELOG.md entry, then tell me when ready.

Wait for user confirmation before proceeding.

### Push fails
> Push failed. Please run `git push` manually after authenticating.

### No changes
> No changes to commit.
