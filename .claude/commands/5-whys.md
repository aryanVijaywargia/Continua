<!--
  Source: taches-cc-resources
  Original Path: commands/consider/5-whys.md
  Adaptations: None needed - generic debugging framework
-->
---
description: Drill to root cause by asking why repeatedly
argument-hint: [problem or leave blank for current context]
---

# 5 Whys Root Cause Analysis

## Objective

Apply the 5 Whys technique to $ARGUMENTS (or the current discussion if no arguments provided).

Keep asking "why" until you hit the root cause, not just symptoms.

## Process

1. State the problem clearly
2. Ask "Why does this happen?" - Answer 1
3. Ask "Why?" about Answer 1 - Answer 2
4. Ask "Why?" about Answer 2 - Answer 3
5. Continue until you hit a root cause (usually 5 iterations, sometimes fewer)
6. Identify actionable intervention at the root

## Output Format

**Problem:** [clear statement]

**Why 1:** [surface cause]
**Why 2:** [deeper cause]
**Why 3:** [even deeper]
**Why 4:** [approaching root]
**Why 5:** [root cause]

**Root Cause:** [the actual thing to fix]

**Intervention:** [specific action at the root level]

## Success Criteria

- Moves past symptoms to actual cause
- Each "why" digs genuinely deeper
- Stops when hitting actionable root (not infinite regress)
- Intervention addresses root, not surface
- Prevents same problem from recurring

## Example (Database Performance)

**Problem:** API response times are slow (>2s)

**Why 1:** The database query is taking too long
**Why 2:** The query is scanning millions of rows
**Why 3:** There's no index on the filtered column
**Why 4:** The column was added in a migration without an index
**Why 5:** Our migration review process doesn't check for missing indexes

**Root Cause:** Migration review process lacks index verification

**Intervention:** Add index check to migration review checklist and CI
