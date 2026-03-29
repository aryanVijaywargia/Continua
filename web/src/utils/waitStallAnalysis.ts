import type {
  Span,
  TimelineEvent,
  TimelineTraceStatus,
} from '../api/client';
import {
  evaluateStaleTraceSignal,
  type StaleTraceSignal,
} from './failureAnalysis';
import { getWaitDetails, type WaitDetails } from './eventSemantics';
import { compareTimelineEvents } from './timeline';

export type WaitStallClassification =
  | 'declared_wait'
  | 'waiting_on_model'
  | 'waiting_on_tool'
  | 'actively_executing'
  | 'possibly_stalled'
  | 'unknown';

export type WaitStallBasis = 'declared' | 'inferred' | 'heuristic';

export type WaitStallReason =
  | 'open_declared_wait'
  | 'open_model_span'
  | 'open_tool_span'
  | 'open_generic_span'
  | 'recent_activity_without_open_span'
  | 'stale_without_stronger_signal'
  | 'insufficient_running_evidence';

export interface WaitStallAssessment {
  classification: WaitStallClassification;
  basis: WaitStallBasis;
  reason: WaitStallReason;
  decisiveEventId?: string;
  decisiveSpanId?: string;
  decisiveSpanName?: string;
  latestActivityAt: string | null;
  runtimeMs: number | null;
  inactivityMs: number | null;
}

export interface OpenWait {
  event: TimelineEvent;
  details: WaitDetails;
}

interface ClassifyRunningTraceOptions {
  traceStatus: TimelineTraceStatus | null;
  traceStartedAt: string | undefined;
  spans: Span[];
  events: TimelineEvent[];
  now?: Date | number;
}

const OPEN_SPAN_STATUS = 'STARTED';
const EXECUTION_EVIDENCE_SPAN_STATUSES = new Set<Span['status']>([
  'STARTED',
  'COMPLETED',
  'FAILED',
]);
const GENERIC_ACTIVE_KINDS = new Set<Span['kind']>(['AGENT', 'CHAIN', 'CUSTOM']);

export function computeOpenWaits(events: TimelineEvent[]): OpenWait[] {
  const sortedEvents = [...events].sort(compareTimelineEvents);
  const unmatchedWaitsById = new Map<string, OpenWait[]>();
  const anonymousOpenWaits: OpenWait[] = [];

  for (const event of sortedEvents) {
    const details = getWaitDetails(event);
    if (!details) {
      continue;
    }

    if (details.phase === 'entered') {
      if (!details.waitId) {
        anonymousOpenWaits.push({ event, details });
        continue;
      }

      const openWaits = unmatchedWaitsById.get(details.waitId) ?? [];
      openWaits.push({ event, details });
      unmatchedWaitsById.set(details.waitId, openWaits);
      continue;
    }

    if (details.phase === 'resolved') {
      if (!details.waitId) {
        continue;
      }

      const openWaits = unmatchedWaitsById.get(details.waitId);
      if (!openWaits || openWaits.length === 0) {
        continue;
      }

      openWaits.shift();
      if (openWaits.length === 0) {
        unmatchedWaitsById.delete(details.waitId);
      }
    }
  }

  return [...anonymousOpenWaits, ...Array.from(unmatchedWaitsById.values()).flat()]
    .sort((left, right) => compareTimelineEvents(left.event, right.event));
}

export function classifyRunningTrace({
  traceStatus,
  traceStartedAt,
  spans,
  events,
  now = new Date(),
}: ClassifyRunningTraceOptions): WaitStallAssessment | null {
  if (traceStatus !== 'RUNNING') {
    return null;
  }

  const staleTraceSignal = evaluateStaleTraceSignal({
    traceStatus,
    traceStartedAt,
    spans,
    events,
    now,
  });
  const spanIndex = new Map(spans.map((span) => [span.span_id, span]));
  const openWait = computeOpenWaits(events).at(-1);

  if (openWait) {
    const decisiveSpan = openWait.event.span_id
      ? spanIndex.get(openWait.event.span_id)
      : undefined;

    return buildAssessment(
      staleTraceSignal,
      {
        classification: 'declared_wait',
        basis: 'declared',
        reason: 'open_declared_wait',
        decisiveEventId: openWait.event.id,
        decisiveSpanId: decisiveSpan?.span_id,
        decisiveSpanName: decisiveSpan?.name,
      }
    );
  }

  const openModelSpan = selectLatestStartedOpenSpan(spans, ['LLM']);
  if (openModelSpan) {
    return buildAssessment(
      staleTraceSignal,
      {
        classification: 'waiting_on_model',
        basis: 'inferred',
        reason: 'open_model_span',
        decisiveSpanId: openModelSpan.span_id,
        decisiveSpanName: openModelSpan.name,
      }
    );
  }

  const openToolSpan = selectLatestStartedOpenSpan(spans, ['TOOL']);
  if (openToolSpan) {
    return buildAssessment(
      staleTraceSignal,
      {
        classification: 'waiting_on_tool',
        basis: 'inferred',
        reason: 'open_tool_span',
        decisiveSpanId: openToolSpan.span_id,
        decisiveSpanName: openToolSpan.name,
      }
    );
  }

  const openGenericSpan = selectLatestStartedOpenSpan(
    spans,
    Array.from(GENERIC_ACTIVE_KINDS)
  );
  if (openGenericSpan) {
    return buildAssessment(
      staleTraceSignal,
      {
        classification: 'actively_executing',
        basis: 'heuristic',
        reason: 'open_generic_span',
        decisiveSpanId: openGenericSpan.span_id,
        decisiveSpanName: openGenericSpan.name,
      }
    );
  }

  if (
    hasExecutionEvidenceBeyondTraceStart(spans, events) &&
    !staleTraceSignal.shouldDisplay
  ) {
    return buildAssessment(
      staleTraceSignal,
      {
        classification: 'actively_executing',
        basis: 'heuristic',
        reason: 'recent_activity_without_open_span',
      }
    );
  }

  if (staleTraceSignal.shouldDisplay) {
    return buildAssessment(
      staleTraceSignal,
      {
        classification: 'possibly_stalled',
        basis: 'heuristic',
        reason: 'stale_without_stronger_signal',
      }
    );
  }

  return buildAssessment(
    staleTraceSignal,
    {
      classification: 'unknown',
      basis: 'heuristic',
      reason: 'insufficient_running_evidence',
    }
  );
}

function buildAssessment(
  staleTraceSignal: StaleTraceSignal,
  decision: Omit<
    WaitStallAssessment,
    'latestActivityAt' | 'runtimeMs' | 'inactivityMs'
  >
): WaitStallAssessment {
  return {
    ...decision,
    latestActivityAt: staleTraceSignal.latestActivityAt,
    runtimeMs: staleTraceSignal.runtimeMs,
    inactivityMs: staleTraceSignal.inactivityMs,
  };
}

function hasExecutionEvidenceBeyondTraceStart(
  spans: Span[],
  events: TimelineEvent[]
): boolean {
  if (events.some((event) => event.source === 'explicit')) {
    return true;
  }

  return spans.some((span) => EXECUTION_EVIDENCE_SPAN_STATUSES.has(span.status));
}

function selectLatestStartedOpenSpan(
  spans: Span[],
  kinds: Span['kind'][]
): Span | null {
  const allowedKinds = new Set(kinds);
  let selectedSpan: Span | null = null;
  let selectedStartedAt: number | null = null;

  for (const span of spans) {
    if (span.status !== OPEN_SPAN_STATUS || !allowedKinds.has(span.kind)) {
      continue;
    }

    const startedAt = parseTimestamp(span.started_at);
    if (selectedSpan === null) {
      selectedSpan = span;
      selectedStartedAt = startedAt;
      continue;
    }

    if (startedAt === null && selectedStartedAt !== null) {
      continue;
    }

    if (
      selectedStartedAt === null ||
      startedAt === null ||
      startedAt >= selectedStartedAt
    ) {
      selectedSpan = span;
      selectedStartedAt = startedAt;
    }
  }

  return selectedSpan;
}

function parseTimestamp(timestamp: string | undefined): number | null {
  if (!timestamp) {
    return null;
  }

  const parsed = Date.parse(timestamp);
  return Number.isNaN(parsed) ? null : parsed;
}
