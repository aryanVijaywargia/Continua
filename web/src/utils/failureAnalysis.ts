import type {
  Span,
  TimelineEvent,
  TimelineTraceStatus,
} from '../api/client';
import { isTimelineErrorEvent } from './timeline';

const ERROR_PREVIEW_MAX_LENGTH = 120;
export const STALE_RUNTIME_THRESHOLD_MS = 15 * 60 * 1000;
export const STALE_INACTIVITY_THRESHOLD_MS = 5 * 60 * 1000;

export type SpanIndex = Map<string, Span>;
export type TimelineEventsBySpanId = Map<string, TimelineEvent[]>;

export interface BreadcrumbSegment {
  spanId: string;
  name: string;
}

export interface FailureSummary {
  primaryFailedSpan: Span | null;
  failedSpanCount: number;
  errorEventCount: number;
  errorPreview: string | null;
  failureTimestamp: string | null;
  breadcrumbPath: BreadcrumbSegment[];
}

export interface FailureAnalysis {
  spanIndex: SpanIndex;
  failedSpanIds: Set<string>;
  inlineErrorPreviews: Map<string, string>;
  primaryAncestorPath: Set<string>;
  summary: FailureSummary;
}

export interface StaleTraceSignal {
  shouldDisplay: boolean;
  latestActivityAt: string | null;
  runtimeMs: number | null;
  inactivityMs: number | null;
}

interface BuildFailureSummaryOptions {
  spans: Span[];
  events: TimelineEvent[];
  spanIndex?: SpanIndex;
  eventsBySpanId?: TimelineEventsBySpanId;
}

interface EvaluateStaleTraceSignalOptions {
  traceStatus: TimelineTraceStatus | null;
  traceStartedAt: string | undefined;
  spans: Span[];
  events: TimelineEvent[];
  now?: Date | number;
}

export function buildSpanIndex(spans: Span[]): SpanIndex {
  const spanIndex: SpanIndex = new Map();

  for (const span of spans) {
    spanIndex.set(span.span_id, span);
  }

  return spanIndex;
}

export function buildTimelineEventsBySpanId(
  events: TimelineEvent[]
): TimelineEventsBySpanId {
  const eventsBySpanId: TimelineEventsBySpanId = new Map();

  for (const event of events) {
    if (!event.span_id) {
      continue;
    }

    const spanEvents = eventsBySpanId.get(event.span_id);
    if (spanEvents) {
      spanEvents.push(event);
      continue;
    }

    eventsBySpanId.set(event.span_id, [event]);
  }

  return eventsBySpanId;
}

export function findPrimaryFailedSpan(spans: Span[]): Span | null {
  const failedSpans = spans
    .map((span, index) => ({ span, index }))
    .filter(({ span }) => span.status === 'FAILED');

  if (failedSpans.length === 0) {
    return null;
  }

  failedSpans.sort((left, right) => {
    const endedAtDelta = compareTimestamps(
      left.span.ended_at,
      right.span.ended_at,
      'last'
    );
    if (endedAtDelta !== 0) {
      return endedAtDelta;
    }

    const startedAtDelta = compareTimestamps(
      left.span.started_at,
      right.span.started_at,
      'last'
    );
    if (startedAtDelta !== 0) {
      return startedAtDelta;
    }

    return left.index - right.index;
  });

  return failedSpans[0]?.span ?? null;
}

export function buildBreadcrumbPath(
  span: Span | null | undefined,
  spanIndex: SpanIndex
): BreadcrumbSegment[] {
  if (!span) {
    return [];
  }

  const path: BreadcrumbSegment[] = [];
  const visited = new Set<string>();
  let current: Span | undefined = span;

  while (current && !visited.has(current.span_id)) {
    path.push({
      spanId: current.span_id,
      name: current.name,
    });
    visited.add(current.span_id);

    if (!current.parent_span_id) {
      break;
    }

    current = spanIndex.get(current.parent_span_id);
  }

  return path.reverse();
}

export function getInlineErrorPreview(
  span: Span | null | undefined,
  eventsOrIndex: TimelineEvent[] | TimelineEventsBySpanId
): string | null {
  if (!span) {
    return null;
  }

  const spanErrorPreview = normalizeErrorPreview(span.error_message);
  if (spanErrorPreview) {
    return spanErrorPreview;
  }

  const spanEvents =
    eventsOrIndex instanceof Map
      ? eventsOrIndex.get(span.span_id) ?? []
      : eventsOrIndex;

  for (const event of spanEvents) {
    if (
      event.span_id !== span.span_id ||
      (event.event_type !== 'error' && event.event_type !== 'exception')
    ) {
      continue;
    }

    const eventPreview = normalizeErrorPreview(event.message);
    if (eventPreview) {
      return eventPreview;
    }
  }

  return null;
}

export function buildFailureSummary({
  spans,
  events,
  spanIndex = buildSpanIndex(spans),
  eventsBySpanId = buildTimelineEventsBySpanId(events),
}: BuildFailureSummaryOptions): FailureSummary {
  const primaryFailedSpan = findPrimaryFailedSpan(spans);
  const breadcrumbPath = buildBreadcrumbPath(primaryFailedSpan, spanIndex);

  return {
    primaryFailedSpan,
    failedSpanCount: spans.filter((span) => span.status === 'FAILED').length,
    errorEventCount: events.filter(isTimelineErrorEvent).length,
    errorPreview: getInlineErrorPreview(primaryFailedSpan, eventsBySpanId),
    failureTimestamp: primaryFailedSpan?.ended_at ?? null,
    breadcrumbPath,
  };
}

export function buildFailureAnalysis(
  spans: Span[],
  events: TimelineEvent[],
  spanIndex: SpanIndex = buildSpanIndex(spans)
): FailureAnalysis {
  const eventsBySpanId = buildTimelineEventsBySpanId(events);
  const summary = buildFailureSummary({
    spans,
    events,
    spanIndex,
    eventsBySpanId,
  });

  const failedSpanIds = new Set<string>();
  const inlineErrorPreviews = new Map<string, string>();

  for (const span of spans) {
    if (span.status !== 'FAILED') {
      continue;
    }

    failedSpanIds.add(span.span_id);
    const preview = getInlineErrorPreview(span, eventsBySpanId);
    if (preview) {
      inlineErrorPreviews.set(span.span_id, preview);
    }
  }

  return {
    spanIndex,
    failedSpanIds,
    inlineErrorPreviews,
    primaryAncestorPath: new Set(
      summary.breadcrumbPath.map((segment) => segment.spanId)
    ),
    summary,
  };
}

export function evaluateStaleTraceSignal({
  traceStatus,
  traceStartedAt,
  spans,
  events,
  now = new Date(),
}: EvaluateStaleTraceSignalOptions): StaleTraceSignal {
  const traceStartedAtMs = parseTimestamp(traceStartedAt);
  const latestActivityAt =
    getLatestTimestamp(events.map((event) => event.timestamp)) ??
    getLatestTimestamp(spans.map((span) => span.ended_at)) ??
    getLatestTimestamp(spans.map((span) => span.started_at)) ??
    traceStartedAt ??
    null;
  const latestActivityAtMs = parseTimestamp(latestActivityAt);
  const nowMs = now instanceof Date ? now.getTime() : now;
  const runtimeMs =
    traceStartedAtMs === null ? null : Math.max(0, nowMs - traceStartedAtMs);
  const inactivityMs =
    latestActivityAtMs === null ? null : Math.max(0, nowMs - latestActivityAtMs);

  return {
    shouldDisplay:
      traceStatus === 'RUNNING' &&
      runtimeMs !== null &&
      inactivityMs !== null &&
      runtimeMs >= STALE_RUNTIME_THRESHOLD_MS &&
      inactivityMs >= STALE_INACTIVITY_THRESHOLD_MS,
    latestActivityAt,
    runtimeMs,
    inactivityMs,
  };
}

function normalizeErrorPreview(value: string | undefined): string | null {
  if (!value) {
    return null;
  }

  const firstNonEmptyLine = value
    .trim()
    .split(/\r?\n/)
    .map((line) => line.trim())
    .find((line) => line.length > 0);

  if (!firstNonEmptyLine) {
    return null;
  }

  if (firstNonEmptyLine.length <= ERROR_PREVIEW_MAX_LENGTH) {
    return firstNonEmptyLine;
  }

  return `${firstNonEmptyLine.slice(0, ERROR_PREVIEW_MAX_LENGTH - 3)}...`;
}

function compareTimestamps(
  left: string | undefined,
  right: string | undefined,
  missingPlacement: 'first' | 'last'
): number {
  const leftTimestamp = parseTimestamp(left);
  const rightTimestamp = parseTimestamp(right);

  if (leftTimestamp === null && rightTimestamp === null) {
    return 0;
  }
  if (leftTimestamp === null) {
    return missingPlacement === 'first' ? -1 : 1;
  }
  if (rightTimestamp === null) {
    return missingPlacement === 'first' ? 1 : -1;
  }

  return leftTimestamp - rightTimestamp;
}

function parseTimestamp(timestamp: string | undefined | null): number | null {
  if (!timestamp) {
    return null;
  }

  const parsed = Date.parse(timestamp);
  return Number.isNaN(parsed) ? null : parsed;
}

function getLatestTimestamp(timestamps: Array<string | undefined>): string | null {
  let latestTimestamp: string | null = null;
  let latestTimestampMs: number | null = null;

  for (const timestamp of timestamps) {
    const parsedTimestamp = parseTimestamp(timestamp);
    if (parsedTimestamp === null) {
      continue;
    }

    if (latestTimestampMs === null || parsedTimestamp > latestTimestampMs) {
      latestTimestamp = timestamp ?? null;
      latestTimestampMs = parsedTimestamp;
    }
  }

  return latestTimestamp;
}
