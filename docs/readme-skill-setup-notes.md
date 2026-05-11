# README Skill Setup Notes

Status: Current

## Local Skill Layout

The README refresh skills were installed locally for both Claude Code and Codex-style discovery:

- `.claude/skills/readme-generator-glincker`
- `.claude/skills/readme-generator-visual`
- `.claude/skills/mermaid-tools`
- `.claude/skills/cli-demo-generator`
- `.claude/skills/walkthrough`

The matching `.agents/skills/<skill-name>` entries are relative symlinks to `../../.claude/skills/<skill-name>`.

`.agents/skills` itself is a real directory, not a symlink.

## Notes

- Symlinks are supported in this environment and were used.
- No fallback copies were required.
- No Playwright or Chromium install was needed for Phase 1 or the README refresh.
- `readme-generator-visual` includes a `package.json` dependency on Playwright, but the refresh used static SVG assets and Mermaid source instead of running screenshot generation.
- The `walkthrough` source used lowercase `skill.md`; it was normalized to `SKILL.md` for case-sensitive checkout compatibility.
