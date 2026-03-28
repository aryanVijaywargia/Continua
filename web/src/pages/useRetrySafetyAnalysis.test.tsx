import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import {
  createSpan,
  createTimelineEvent,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import { useRetrySafetyAnalysis } from './useRetrySafetyAnalysis';

beforeEach(() => {
  resetTestEntityCounter();
});

describe('useRetrySafetyAnalysis', () => {
  it('keeps analysis stable when polling adds non-effect events', () => {
    const failedSpan = createSpan({
      span_id: 'failed-span',
      status: 'FAILED',
      name: 'Failed span',
    });
    const effectEvent = createTimelineEvent({
      id: 'effect-event',
      span_id: failedSpan.span_id,
      event_type: 'effect',
      payload: {
        effect_kind: 'model_call',
        has_external_side_effect: false,
      },
    });

    const { result, rerender } = renderHook(
      ({ spans, events }) => useRetrySafetyAnalysis(spans, events),
      {
        initialProps: {
          spans: [failedSpan],
          events: [effectEvent],
        },
      }
    );

    const firstResult = result.current;

    rerender({
      spans: [failedSpan],
      events: [
        effectEvent,
        createTimelineEvent({
          id: 'later-log',
          span_id: failedSpan.span_id,
          event_type: 'message',
          message: 'poll update',
          timestamp: '2026-03-14T10:00:01.000Z',
        }),
      ],
    });

    expect(result.current).toBe(firstResult);
    expect(result.current.traceAssessment).toBe(firstResult.traceAssessment);
    expect(result.current.spanAssessments).toBe(firstResult.spanAssessments);
  });

  it('recomputes when a new effect event changes the failed-span assessment', () => {
    const failedSpan = createSpan({
      span_id: 'failed-span',
      status: 'FAILED',
      name: 'Failed span',
    });
    const effectEvent = createTimelineEvent({
      id: 'effect-event',
      span_id: failedSpan.span_id,
      event_type: 'effect',
      payload: {
        effect_kind: 'model_call',
        has_external_side_effect: false,
      },
    });

    const { result, rerender } = renderHook(
      ({ spans, events }) => useRetrySafetyAnalysis(spans, events),
      {
        initialProps: {
          spans: [failedSpan],
          events: [effectEvent],
        },
      }
    );

    const firstResult = result.current;

    rerender({
      spans: [failedSpan],
      events: [
        effectEvent,
        createTimelineEvent({
          id: 'unsafe-effect',
          span_id: failedSpan.span_id,
          event_type: 'effect',
          timestamp: '2026-03-14T10:00:01.000Z',
          payload: {
            effect_kind: 'api_call',
            has_external_side_effect: true,
            idempotent: false,
          },
        }),
      ],
    });

    expect(result.current).not.toBe(firstResult);
    expect(result.current.traceAssessment.classification).toBe('unsafe');
    expect(result.current.traceAssessment.reason).toBe('mutating_non_idempotent');
  });

  it('recomputes when a span becomes failed', () => {
    const span = createSpan({
      span_id: 'candidate-span',
      status: 'STARTED',
      name: 'Candidate span',
    });
    const effectEvent = createTimelineEvent({
      id: 'candidate-effect',
      span_id: span.span_id,
      event_type: 'effect',
      payload: {
        effect_kind: 'model_call',
        has_external_side_effect: false,
      },
    });

    const { result, rerender } = renderHook(
      ({ spans, events }) => useRetrySafetyAnalysis(spans, events),
      {
        initialProps: {
          spans: [span],
          events: [effectEvent],
        },
      }
    );

    const firstResult = result.current;

    rerender({
      spans: [
        {
          ...span,
          status: 'FAILED',
          ended_at: '2026-03-14T10:00:02.000Z',
        },
      ],
      events: [effectEvent],
    });

    expect(result.current).not.toBe(firstResult);
    expect(result.current.spanAssessments.get(span.span_id)?.classification).toBe(
      'retryable'
    );
  });
});
