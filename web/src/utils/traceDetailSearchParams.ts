const SPAN_QUERY_PARAM = 'span';

export function parseSpanParam(searchParams: URLSearchParams): string | null {
  const span = searchParams.get(SPAN_QUERY_PARAM);
  return span === null || span === '' ? null : span;
}

export function serializeSpanParam(
  searchParams: URLSearchParams,
  spanId: string | null
): URLSearchParams {
  const nextSearchParams = new URLSearchParams(searchParams);

  if (spanId === null || spanId === '') {
    nextSearchParams.delete(SPAN_QUERY_PARAM);
    return nextSearchParams;
  }

  nextSearchParams.set(SPAN_QUERY_PARAM, spanId);
  return nextSearchParams;
}
