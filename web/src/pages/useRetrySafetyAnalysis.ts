import { useRef } from 'react';
import type { Span, TimelineEvent } from '../api/client';
import { buildTimelineEventsBySpanId } from '../utils/failureAnalysis';
import {
  assessSpanRetrySafety,
  assessTraceRetrySafety,
  classifyEffectEvent,
  type RetrySafetyAssessment,
} from '../utils/retrySafety';

export interface RetrySafetyAnalysis {
  spanAssessments: Map<string, RetrySafetyAssessment>;
  traceAssessment: RetrySafetyAssessment;
}

interface RetrySafetyAnalysisCache {
  signature: string;
  result: RetrySafetyAnalysis;
}

export function useRetrySafetyAnalysis(
  spans: Span[],
  events: TimelineEvent[]
): RetrySafetyAnalysis {
  const signature = buildRetrySafetySignature(spans, events);
  const cacheRef = useRef<RetrySafetyAnalysisCache | null>(null);

  if (!cacheRef.current || cacheRef.current.signature !== signature) {
    cacheRef.current = {
      signature,
      result: computeRetrySafetyAnalysis(spans, events),
    };
  }

  return cacheRef.current.result;
}

function computeRetrySafetyAnalysis(
  spans: Span[],
  events: TimelineEvent[]
): RetrySafetyAnalysis {
  const failedSpans = spans.filter((span) => span.status === 'FAILED');
  const eventsBySpanId = buildTimelineEventsBySpanId(events);
  const spanAssessments = new Map<string, RetrySafetyAssessment>();

  for (const span of failedSpans) {
    const assessment = assessSpanRetrySafety(
      span,
      eventsBySpanId.get(span.span_id) ?? []
    );
    if (!assessment) {
      continue;
    }

    spanAssessments.set(span.span_id, assessment);
  }

  return {
    spanAssessments,
    traceAssessment: assessTraceRetrySafety(spanAssessments.values()),
  };
}

function buildRetrySafetySignature(spans: Span[], events: TimelineEvent[]): string {
  const failedSpans = spans
    .filter((span) => span.status === 'FAILED')
    .map((span) => `${span.span_id}:${span.name}:${span.status}`)
    .join('|');

  const failedSpanIds = new Set(
    spans
      .filter((span) => span.status === 'FAILED')
      .map((span) => span.span_id)
  );

  const effectInputs = events
    .filter(
      (event) => event.event_type === 'effect' && Boolean(event.span_id) && failedSpanIds.has(event.span_id!)
    )
    .map((event) => {
      const assessment = classifyEffectEvent(event);
      return [
        event.id,
        event.span_id,
        event.timestamp,
        event.sequence ?? '',
        assessment?.classification ?? 'non-effect',
        assessment?.reason ?? 'non-effect',
        assessment?.effectKind ?? '',
        assessment?.hasExternalSideEffect ?? '',
        assessment?.idempotent ?? '',
        assessment?.idempotencyKey ?? '',
        JSON.stringify(event.payload ?? null),
      ].join(':');
    })
    .join('|');

  return `${failedSpans}::${effectInputs}`;
}
