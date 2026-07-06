import type { Trace, TraceDetail } from '../api/client';

/**
 * Ordered lineage chain (root ancestor → … → current trace) for an
 * engine-backed trace, deduplicated by run id. Non-engine traces have no
 * lineage chain.
 */
export function buildTraceLineageChain(
  trace: TraceDetail | null,
  lineageAncestors: Trace[]
): Trace[] {
  if (!trace?.engine?.run_id) {
    return [];
  }

  const visitedRunIds = new Set<string>();
  return [...lineageAncestors, trace].filter((lineageTrace) => {
    const runId = lineageTrace.engine?.run_id;
    if (!runId || visitedRunIds.has(runId)) {
      return false;
    }
    visitedRunIds.add(runId);
    return true;
  });
}

/**
 * Validates a `location.state.returnTo` value against the destinations the
 * trace-detail page is allowed to link back to; anything else falls back to
 * the traces list.
 */
export function getReturnToDestination(state: unknown): string {
  if (
    typeof state !== 'object' ||
    state === null ||
    !('returnTo' in state) ||
    typeof state.returnTo !== 'string'
  ) {
    return '/traces';
  }

  const { returnTo } = state;
  return returnTo === '/traces' ||
    returnTo.startsWith('/traces?') ||
    returnTo === '/engine/runs' ||
    returnTo.startsWith('/engine/runs?') ||
    returnTo.startsWith('/sessions/')
    ? returnTo
    : '/traces';
}
