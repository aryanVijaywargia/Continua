import { useRef } from 'react';
import type { Span, TimelineEvent, TimelineTraceStatus } from '../api/client';
import { evaluateStaleTraceSignal } from '../utils/failureAnalysis';
import {
  classifyRunningTrace,
  type WaitStallAssessment,
} from '../utils/waitStallAnalysis';

interface UseWaitStallAnalysisOptions {
  traceStatus: TimelineTraceStatus | null;
  traceStartedAt: string | undefined;
  spans: Span[];
  events: TimelineEvent[];
  hasTimelineSnapshot: boolean;
}

interface WaitStallAnalysisCache {
  signature: string;
  staleShouldDisplay: boolean;
  assessmentCore: Omit<
    WaitStallAssessment,
    'latestActivityAt' | 'runtimeMs' | 'inactivityMs'
  >;
}

export function useWaitStallAnalysis({
  traceStatus,
  traceStartedAt,
  spans,
  events,
  hasTimelineSnapshot,
}: UseWaitStallAnalysisOptions): WaitStallAssessment | null {
  const cacheRef = useRef<WaitStallAnalysisCache | null>(null);

  if (!hasTimelineSnapshot || traceStatus !== 'RUNNING') {
    return null;
  }

  const now = new Date();
  const staleTraceSignal = evaluateStaleTraceSignal({
    traceStatus,
    traceStartedAt,
    spans,
    events,
    now,
  });
  const signature = buildWaitStallSignature(traceStatus, spans, events);

  if (
    !cacheRef.current ||
    cacheRef.current.signature !== signature ||
    cacheRef.current.staleShouldDisplay !== staleTraceSignal.shouldDisplay
  ) {
    const assessment = classifyRunningTrace({
      traceStatus,
      traceStartedAt,
      spans,
      events,
      now,
    });

    if (!assessment) {
      return null;
    }

    cacheRef.current = {
      signature,
      staleShouldDisplay: staleTraceSignal.shouldDisplay,
      assessmentCore: {
        classification: assessment.classification,
        basis: assessment.basis,
        reason: assessment.reason,
        decisiveEventId: assessment.decisiveEventId,
        decisiveSpanId: assessment.decisiveSpanId,
        decisiveSpanName: assessment.decisiveSpanName,
      },
    };
  }

  return {
    ...cacheRef.current.assessmentCore,
    latestActivityAt: staleTraceSignal.latestActivityAt,
    runtimeMs: staleTraceSignal.runtimeMs,
    inactivityMs: staleTraceSignal.inactivityMs,
  };
}

function buildWaitStallSignature(
  traceStatus: TimelineTraceStatus,
  spans: Span[],
  events: TimelineEvent[]
): string {
  const spanSignature = spans
    .map((span) =>
      [
        span.span_id,
        span.name,
        span.kind,
        span.status,
        span.started_at,
        span.ended_at ?? '',
      ].join(':')
    )
    .join('|');
  const eventSignature = events
    .filter((event) => event.source === 'explicit' || event.event_type === 'wait')
    .map((event) =>
      [
        event.id,
        event.event_type,
        event.timestamp,
        event.source,
        event.sequence ?? '',
        event.span_id ?? '',
        event.span_name ?? '',
        JSON.stringify(event.payload ?? null),
      ].join(':')
    )
    .join('|');

  return `${traceStatus}::${spanSignature}::${eventSignature}`;
}
