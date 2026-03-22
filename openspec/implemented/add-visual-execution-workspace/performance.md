# Visual Workspace Profiling

## Method

- Command: `pnpm --filter web profile:workspace`
- Harness: `web/scripts/profile-visual-workspace.mjs`
- Environment: `jsdom` via Vite SSR module loading
- Machine: Apple M2, 8 GB RAM, Node `v22.16.0`, macOS `darwin 25.3.0`
- Iterations: 3 warmup + 15 measured samples per scenario

This harness measures local React render/commit work in the benchmark harness. The current implementation uses windowed tree and waterfall row rendering for large traces, so the measurements reflect the shipped Phase 9 design rather than a fully-mounted off-screen DOM.

## Results

| Spans | Expand All p95 | Collapse All p95 | Waterfall Render p95 | Search Input Commit p95 | Search Deferred Settle p95 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 200 | 7.93 ms | 0.13 ms | 4.43 ms | 0.02 ms | 2.74 ms |
| 400 | 4.08 ms | 1.40 ms | 7.50 ms | 0.01 ms | 1.46 ms |
| 800 | 2.34 ms | 0.12 ms | 4.11 ms | 0.02 ms | 1.60 ms |
| 1200 | 2.44 ms | 0.17 ms | 4.02 ms | 0.01 ms | 3.61 ms |

Raw artifact: `openspec/changes/add-visual-execution-workspace/artifacts/visual-workspace-profile.json`

## Assessment

- Expand-all and collapse-all stay within one frame for the profiled `<500` span tiers.
- Waterfall render stays below `100 ms` p95 at all profiled sizes in this harness.
- Search responsiveness remains well below one frame for input commit at all profiled sizes.
- The passing numbers depend on the approved windowed tree/waterfall render path for off-screen rows on large traces.
- The current `700` reveal-row constant is now validated and can be treated as finalized for Phase 9.

## Next Step

Keep this artifact as the checked-in benchmark note for the Phase 9 acceptance record. If later UI changes materially alter the tree or waterfall render path, rerun `pnpm --filter web profile:workspace` and refresh the artifact before changing the guard constant.
