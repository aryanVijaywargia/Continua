import { fetchTraces, type Trace, type TraceDetail } from '../../api/client';

const MAX_LINEAGE_ANCESTOR_DEPTH = 64;
const CHILD_TRACES_PAGE_SIZE = 20;

/**
 * Walks parent_run_id links upward, one trace per hop, and returns the
 * ancestor chain root-first. Bails out (empty result) on cycles, missing
 * parents, or chains deeper than {@link MAX_LINEAGE_ANCESTOR_DEPTH}.
 */
export async function fetchTraceLineageAncestors(
  trace: TraceDetail,
  projectId?: string
): Promise<Trace[]> {
  const engine = trace.engine;
  if (!engine?.run_id || !engine.parent_run_id) {
    return [];
  }

  const ancestors: Trace[] = [];
  const visitedRunIds = new Set<string>([engine.run_id]);
  let parentRunId: string | undefined = engine.parent_run_id;

  while (parentRunId) {
    if (ancestors.length >= MAX_LINEAGE_ANCESTOR_DEPTH) {
      return [];
    }
    if (visitedRunIds.has(parentRunId)) {
      return [];
    }
    visitedRunIds.add(parentRunId);

    const page = await fetchTraces({
      project_id: projectId,
      engine_run_id: parentRunId,
      limit: 1,
    });
    const parentTrace = page.traces[0];
    if (!parentTrace?.engine?.run_id) {
      return [];
    }

    ancestors.push(parentTrace);
    parentRunId = parentTrace.engine.parent_run_id;
  }

  return ancestors.reverse();
}

/** Pages through all direct child workflow traces of an engine run. */
export async function fetchDirectChildTraces(
  parentRunId: string,
  projectId?: string
): Promise<Trace[]> {
  const traces: Trace[] = [];
  let offset = 0;

  for (;;) {
    const page = await fetchTraces({
      project_id: projectId,
      engine_parent_run_id: parentRunId,
      limit: CHILD_TRACES_PAGE_SIZE,
      offset,
    });

    traces.push(...page.traces);
    if (
      page.traces.length === 0 ||
      traces.length >= page.total ||
      page.traces.length < CHILD_TRACES_PAGE_SIZE
    ) {
      return traces;
    }

    offset += page.traces.length;
  }
}
