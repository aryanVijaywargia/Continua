import type { Span, TimelineEvent, TimelineTraceStatus } from '../api/client';
import { getDecisionDetails } from './eventSemantics';
import { compareTimelineEvents } from './timeline';

export interface DecisionTraceEntry {
  event: TimelineEvent;
  spanId: string | null;
  spanName: string;
  question: string;
  chosen: unknown;
  alternatives?: unknown[];
  reasoning?: string;
}

export interface TraceCostPoint {
  anchorMs: number;
  cumulativeCostUsd: number;
  incrementalCostUsd: number;
}

export interface TraceCostSeries {
  partial: boolean;
  points: TraceCostPoint[];
  totalCostUsd: number;
}

export function buildReasoningEntries(
  events: TimelineEvent[],
  spans: Span[]
): DecisionTraceEntry[] {
  const spanNamesById = new Map(spans.map((span) => [span.span_id, span.name]));

  return events
    .map((event) => {
      const decision = getDecisionDetails(event);
      if (!decision) {
        return null;
      }

      return {
        event,
        spanId: event.span_id ?? null,
        spanName:
          getNonEmptyString(event.span_name) ??
          (event.span_id ? spanNamesById.get(event.span_id) : null) ??
          event.span_id ??
          'Unknown span',
        question: decision.question,
        chosen: decision.chosen,
        alternatives: decision.alternatives,
        reasoning: decision.reasoning,
      };
    })
    .filter((entry): entry is DecisionTraceEntry => entry !== null)
    .sort((left, right) => compareTimelineEvents(left.event, right.event));
}

export function buildTraceCostSeries(
  spans: Span[],
  traceStatus: TimelineTraceStatus | null
): TraceCostSeries | null {
  const incrementByAnchorMs = new Map<number, number>();

  for (const span of spans) {
    if (!isTerminalSpan(span)) {
      continue;
    }

    const costUsd =
      typeof span.cost_usd === 'number' && Number.isFinite(span.cost_usd)
        ? span.cost_usd
        : 0;
    if (costUsd === 0) {
      continue;
    }

    const anchorMs = parseTimestamp(span.ended_at ?? span.started_at);
    if (anchorMs === null) {
      continue;
    }

    incrementByAnchorMs.set(
      anchorMs,
      (incrementByAnchorMs.get(anchorMs) ?? 0) + costUsd
    );
  }

  const sortedAnchors = Array.from(incrementByAnchorMs.entries()).sort(
    ([leftAnchorMs], [rightAnchorMs]) => leftAnchorMs - rightAnchorMs
  );
  if (sortedAnchors.length === 0) {
    return null;
  }

  let cumulativeCostUsd = 0;
  const points = sortedAnchors.map(([anchorMs, incrementalCostUsd]) => {
    cumulativeCostUsd += incrementalCostUsd;

    return {
      anchorMs,
      cumulativeCostUsd,
      incrementalCostUsd,
    };
  });

  return {
    partial: traceStatus === 'RUNNING',
    points,
    totalCostUsd: cumulativeCostUsd,
  };
}

function isTerminalSpan(span: Span): boolean {
  return span.status === 'COMPLETED' || span.status === 'FAILED';
}

function parseTimestamp(value: string | undefined | null): number | null {
  if (!value) {
    return null;
  }

  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? null : parsed;
}

function getNonEmptyString(value: string | undefined): string | null {
  return typeof value === 'string' && value.length > 0 ? value : null;
}
