import type {
  EngineHealthResponse,
  JsonValue,
  Span,
  Trace,
  TraceDetail,
} from '../../api/client';
import type { DataHonesty } from '../../components/DataState';

export type OverviewRange = '24h' | '7d' | '30d' | 'all';

export const OVERVIEW_RANGES: ReadonlyArray<{
  value: OverviewRange;
  label: string;
  ms?: number;
}> = [
  { value: '24h', label: 'Last 24 hours', ms: 24 * 60 * 60 * 1000 },
  { value: '7d', label: 'Last 7 days', ms: 7 * 24 * 60 * 60 * 1000 },
  { value: '30d', label: 'Last 30 days', ms: 30 * 24 * 60 * 60 * 1000 },
  { value: 'all', label: 'All time' },
];

export function parseOverviewRange(value: string | null): OverviewRange {
  return OVERVIEW_RANGES.some((option) => option.value === value)
    ? (value as OverviewRange)
    : 'all';
}

export function overviewRangeLabel(range: OverviewRange): string {
  return OVERVIEW_RANGES.find((option) => option.value === range)?.label ?? 'All time';
}

/** ISO start of the range, or undefined for "all time". */
export function overviewRangeStart(range: OverviewRange, now: number): string | undefined {
  const ms = OVERVIEW_RANGES.find((option) => option.value === range)?.ms;
  return ms ? new Date(now - ms).toISOString() : undefined;
}

/** Builds a traces-list link that carries the exact overview range start. */
export function buildTracesLink(
  rangeStart: string | undefined,
  extra: Record<string, string> = {}
): string {
  const params = new URLSearchParams(extra);
  if (rangeStart) {
    params.set('start_time_from', rangeStart);
  }
  const query = params.toString();
  return query ? `/traces?${query}` : '/traces';
}

const REQUEST_KEYS = [
  'request',
  'prompt',
  'query',
  'question',
  'message',
  'input',
  'content',
  'text',
];

/**
 * Picks a one-line summary of a recorded trace input. Returns null when the
 * trace has no input, so callers can say "not recorded" instead of guessing.
 */
export function summarizeRequest(input: JsonValue | undefined): string | null {
  if (input === undefined || input === null) {
    return null;
  }
  if (typeof input === 'string') {
    return input.trim() || null;
  }
  if (typeof input === 'number' || typeof input === 'boolean') {
    return String(input);
  }
  if (Array.isArray(input)) {
    const userMessage = input.find(
      (item) =>
        item !== null &&
        typeof item === 'object' &&
        !Array.isArray(item) &&
        item.role === 'user' &&
        typeof item.content === 'string'
    );
    if (userMessage && typeof userMessage === 'object' && !Array.isArray(userMessage)) {
      return summarizeRequest(userMessage.content);
    }
    return input.length > 0 ? JSON.stringify(input) : null;
  }
  for (const key of REQUEST_KEYS) {
    const value = input[key];
    if (typeof value === 'string' && value.trim()) {
      return value.trim();
    }
  }
  if (Array.isArray(input.messages)) {
    const fromMessages = summarizeRequest(input.messages);
    if (fromMessages) {
      return fromMessages;
    }
  }
  const serialized = JSON.stringify(input);
  return serialized === '{}' ? null : serialized;
}

export interface RecordedDuration {
  trace: Trace;
  durationMs: number;
}

/** Durations derived only from traces that recorded both start and end times. */
export function recordedDurations(traces: Trace[]): RecordedDuration[] {
  return traces.flatMap((trace) => {
    if (!trace.ended_at) {
      return [];
    }
    const durationMs =
      new Date(trace.ended_at).getTime() - new Date(trace.started_at).getTime();
    return Number.isFinite(durationMs) && durationMs >= 0 ? [{ trace, durationMs }] : [];
  });
}

export type EngineProjectionSummary =
  | { kind: 'checking' }
  | { kind: 'unavailable' }
  | { kind: 'up-to-date' }
  | { kind: 'catching-up'; runs: number };

export function summarizeEngineProjections(
  health: EngineHealthResponse | undefined,
  isError: boolean
): EngineProjectionSummary {
  if (isError) {
    return { kind: 'unavailable' };
  }
  if (!health) {
    return { kind: 'checking' };
  }
  const { lag_rows: lagRows, runs_catching_up: runsCatchingUp } = health.projector;
  return lagRows === 0 && runsCatchingUp === 0
    ? { kind: 'up-to-date' }
    : { kind: 'catching-up', runs: runsCatchingUp };
}

export interface CoverageRow {
  key: string;
  label: string;
  kind: DataHonesty;
  value?: string;
}

function hasPayload(value: JsonValue | undefined): boolean {
  if (value === undefined || value === null) {
    return false;
  }
  if (typeof value === 'string') {
    return value.trim().length > 0;
  }
  if (Array.isArray(value)) {
    return value.length > 0;
  }
  if (typeof value === 'object') {
    return Object.keys(value).length > 0;
  }
  return true;
}

/**
 * Derives the coverage card from what the page actually loaded. Sampled
 * details and spans are `undefined` when a request is pending or failed.
 */
export function deriveCoverage({
  engineHealthLoaded,
  sampledDetails,
  sampledSpans,
  traces,
}: {
  engineHealthLoaded: boolean;
  sampledDetails: Array<TraceDetail | undefined>;
  sampledSpans: Array<Span[] | undefined>;
  traces: Trace[];
}): CoverageRow[] {
  const loadedSpans = sampledSpans.filter((spans): spans is Span[] => spans !== undefined);
  const loadedDetails = sampledDetails.filter(
    (detail): detail is TraceDetail => detail !== undefined
  );
  const spanCount = loadedSpans.reduce((sum, spans) => sum + spans.length, 0);

  const hasUsage = traces.some(
    (trace) =>
      (trace.total_tokens_in ?? 0) > 0 ||
      (trace.total_tokens_out ?? 0) > 0 ||
      (trace.total_cost_usd ?? 0) > 0
  );
  const hasCapturedPayloads =
    loadedDetails.some((detail) => hasPayload(detail.input) || hasPayload(detail.output)) ||
    loadedSpans.some((spans) =>
      spans.some((span) => hasPayload(span.input) || hasPayload(span.output))
    );
  const payloadsChecked = loadedDetails.length > 0 || loadedSpans.length > 0;

  return [
    {
      key: 'status',
      label: 'Trace status and start times',
      kind: traces.length > 0 ? 'recorded' : 'unavailable',
      value: traces.length > 0 ? undefined : 'no traces yet',
    },
    {
      key: 'spans',
      label: 'Steps (spans)',
      kind:
        loadedSpans.length === 0 ? 'unavailable' : spanCount > 0 ? 'recorded' : 'not-captured',
      value: loadedSpans.length === 0 ? 'not loaded' : undefined,
    },
    {
      key: 'duration',
      label: 'Trace and step duration',
      kind: 'derived',
    },
    {
      key: 'usage',
      label: 'Token and cost usage',
      kind: hasUsage ? 'recorded' : 'unverified',
    },
    {
      key: 'payloads',
      label: 'Captured prompts and outputs',
      kind: !payloadsChecked ? 'unavailable' : hasCapturedPayloads ? 'recorded' : 'not-captured',
      value: payloadsChecked ? undefined : 'not loaded',
    },
    {
      key: 'engine',
      label: 'Engine diagnostics',
      kind: engineHealthLoaded ? 'recorded' : 'unavailable',
    },
  ];
}
