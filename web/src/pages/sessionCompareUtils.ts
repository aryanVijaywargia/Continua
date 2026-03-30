const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function normalizeCompareTraceIdParam(
  value: string | null | undefined
): string | undefined {
  const trimmed = value?.trim();
  if (!trimmed || !UUID_PATTERN.test(trimmed)) {
    return undefined;
  }

  return trimmed.toLowerCase();
}

export function buildCompareSearchParams(
  baselineTraceId?: string,
  candidateTraceId?: string
): URLSearchParams {
  const params = new URLSearchParams();
  if (baselineTraceId) {
    params.set('baseline_trace_id', baselineTraceId);
  }
  if (candidateTraceId) {
    params.set('candidate_trace_id', candidateTraceId);
  }
  return params;
}

export function getCompareReturnToDestination(
  state: unknown,
  sessionId: string,
  searchParams: URLSearchParams
): string {
  if (
    typeof state === 'object' &&
    state !== null &&
    'returnTo' in state &&
    typeof state.returnTo === 'string' &&
    state.returnTo.startsWith('/sessions/')
  ) {
    return state.returnTo;
  }

  const baselineTraceId = normalizeCompareTraceIdParam(searchParams.get('baseline_trace_id'));
  const candidateTraceId = normalizeCompareTraceIdParam(searchParams.get('candidate_trace_id'));
  const params = buildCompareSearchParams(
    baselineTraceId,
    candidateTraceId && candidateTraceId !== baselineTraceId ? candidateTraceId : undefined
  );
  const query = params.toString();

  return query ? `/sessions/${sessionId}?${query}` : `/sessions/${sessionId}`;
}
