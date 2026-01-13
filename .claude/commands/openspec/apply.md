---
name: OpenSpec: Apply
description: Implement an approved OpenSpec change and keep tasks in sync.
category: OpenSpec
tags: [openspec, apply]
---
<!-- OPENSPEC:START -->
**Guardrails**
- Favor straightforward, minimal implementations first and add complexity only when it is requested or clearly required.
- Keep changes tightly scoped to the requested outcome.
- Refer to `openspec/AGENTS.md` (located inside the `openspec/` directory—run `ls openspec` or `openspec update` if you don't see it) if you need additional OpenSpec conventions or clarifications.

**Steps**
Track these steps as TODOs and complete them one by one.

1. **Create feature branch (REQUIRED)**: Before any implementation:
   - Check current branch with `git branch --show-current`
   - If on `main` or `master`, create and switch to a new feature branch:
     ```bash
     git checkout -b <proposal-id>
     # Example: git checkout -b enable-e2e-usability
     ```
   - The branch name should match the proposal ID from `changes/<id>/`
   - If already on a feature branch, confirm it's the correct one for this proposal

2. Read `changes/<id>/proposal.md`, `design.md` (if present), and `tasks.md` to confirm scope and acceptance criteria.
3. Work through tasks sequentially, keeping edits minimal and focused on the requested change.
4. Confirm completion before updating statuses—make sure every item in `tasks.md` is finished.
5. Update the checklist after all work is done so each task is marked `- [x]` and reflects reality.
6. Reference `openspec list` or `openspec show <item>` when additional context is required.

**Important**: Never implement OpenSpec changes directly on `main`. Always use a feature branch and create a PR for review.

**Reference**
- Use `openspec show <id> --json --deltas-only` if you need additional context from the proposal while implementing.
<!-- OPENSPEC:END -->
