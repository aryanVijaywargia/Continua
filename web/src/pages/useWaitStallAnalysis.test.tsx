import { renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  createSpan,
  createTimelineEvent,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import { useWaitStallAnalysis } from './useWaitStallAnalysis';

const TRACE_STARTED_AT = '2026-03-14T10:00:00.000Z';

beforeEach(() => {
  resetTestEntityCounter();
  vi.useFakeTimers();
  vi.setSystemTime(new Date('2026-03-14T10:02:00.000Z'));
});

afterEach(() => {
  vi.useRealTimers();
});

describe('useWaitStallAnalysis', () => {
  it.each(['COMPLETED', 'FAILED'] as const)(
    'returns null when trace status is %s',
    (traceStatus) => {
      const { result } = renderHook(() =>
        useWaitStallAnalysis({
          traceStatus,
          traceStartedAt: TRACE_STARTED_AT,
          spans: [],
          events: [],
          hasTimelineSnapshot: true,
        })
      );

      expect(result.current).toBeNull();
    }
  );

  it('returns null before the initial timeline snapshot loads', () => {
    const { result } = renderHook(() =>
      useWaitStallAnalysis({
        traceStatus: 'RUNNING',
        traceStartedAt: TRACE_STARTED_AT,
        spans: [],
        events: [],
        hasTimelineSnapshot: false,
      })
    );

    expect(result.current).toBeNull();
  });

  it('keeps classification stable when only time advances and stale does not flip', () => {
    const completedSpan = createSpan({
      span_id: 'completed-work',
      status: 'COMPLETED',
      started_at: '2026-03-14T10:00:30.000Z',
      ended_at: '2026-03-14T10:01:00.000Z',
    });

    const { result, rerender } = renderHook(
      ({ spans, events }) =>
        useWaitStallAnalysis({
          traceStatus: 'RUNNING',
          traceStartedAt: TRACE_STARTED_AT,
          spans,
          events,
          hasTimelineSnapshot: true,
        }),
      {
        initialProps: {
          spans: [completedSpan],
          events: [] as ReturnType<typeof createTimelineEvent>[],
        },
      }
    );

    const firstAssessment = result.current;
    vi.setSystemTime(new Date('2026-03-14T10:04:00.000Z'));
    rerender({
      spans: [completedSpan],
      events: [],
    });

    expect(firstAssessment?.classification).toBe('actively_executing');
    expect(result.current?.classification).toBe('actively_executing');
    expect(result.current?.reason).toBe(firstAssessment?.reason);
  });

  it('updates timing fields on every poll cycle even when evidence is unchanged', () => {
    const completedSpan = createSpan({
      span_id: 'completed-work',
      status: 'COMPLETED',
      started_at: '2026-03-14T10:00:30.000Z',
      ended_at: '2026-03-14T10:01:00.000Z',
    });

    const { result, rerender } = renderHook(
      () =>
        useWaitStallAnalysis({
          traceStatus: 'RUNNING',
          traceStartedAt: TRACE_STARTED_AT,
          spans: [completedSpan],
          events: [],
          hasTimelineSnapshot: true,
        })
    );

    const firstRuntimeMs = result.current?.runtimeMs ?? 0;
    const firstInactivityMs = result.current?.inactivityMs ?? 0;

    vi.setSystemTime(new Date('2026-03-14T10:04:00.000Z'));
    rerender();

    expect((result.current?.runtimeMs ?? 0) - firstRuntimeMs).toBe(2 * 60 * 1000);
    expect((result.current?.inactivityMs ?? 0) - firstInactivityMs).toBe(
      2 * 60 * 1000
    );
  });

  it('transitions to possibly stalled when time advance flips the stale heuristic', () => {
    const { result, rerender } = renderHook(
      () =>
        useWaitStallAnalysis({
          traceStatus: 'RUNNING',
          traceStartedAt: TRACE_STARTED_AT,
          spans: [],
          events: [],
          hasTimelineSnapshot: true,
        })
    );

    expect(result.current?.classification).toBe('unknown');

    vi.setSystemTime(new Date('2026-03-14T10:20:00.000Z'));
    rerender();

    expect(result.current?.classification).toBe('possibly_stalled');
    expect(result.current?.reason).toBe('stale_without_stronger_signal');
  });

  it('recomputes when a new explicit event arrives', () => {
    const { result, rerender } = renderHook(
      ({ events }) =>
        useWaitStallAnalysis({
          traceStatus: 'RUNNING',
          traceStartedAt: TRACE_STARTED_AT,
          spans: [],
          events,
          hasTimelineSnapshot: true,
        }),
      {
        initialProps: {
          events: [] as ReturnType<typeof createTimelineEvent>[],
        },
      }
    );

    expect(result.current?.classification).toBe('unknown');

    rerender({
      events: [
        createTimelineEvent({
          id: 'log-1',
          event_type: 'log',
          timestamp: '2026-03-14T10:01:30.000Z',
          message: 'Fresh activity',
        }),
      ],
    });

    expect(result.current?.classification).toBe('actively_executing');
    expect(result.current?.reason).toBe('recent_activity_without_open_span');
  });

  it('recomputes when a new wait event arrives', () => {
    const { result, rerender } = renderHook(
      ({ events }) =>
        useWaitStallAnalysis({
          traceStatus: 'RUNNING',
          traceStartedAt: TRACE_STARTED_AT,
          spans: [],
          events,
          hasTimelineSnapshot: true,
        }),
      {
        initialProps: {
          events: [] as ReturnType<typeof createTimelineEvent>[],
        },
      }
    );

    expect(result.current?.classification).toBe('unknown');

    rerender({
      events: [
        createTimelineEvent({
          id: 'wait-1',
          event_type: 'wait',
          timestamp: '2026-03-14T10:01:30.000Z',
          payload: {
            wait_kind: 'model_response',
            phase: 'entered',
            wait_id: 'wait-1',
          },
        }),
      ],
    });

    expect(result.current?.classification).toBe('declared_wait');
    expect(result.current?.decisiveEventId).toBe('wait-1');
  });

  it('recomputes when a span status changes', () => {
    const toolSpan = createSpan({
      span_id: 'tool-1',
      name: 'Tool span',
      kind: 'TOOL',
      status: 'SCHEDULED',
      started_at: '2026-03-14T10:01:00.000Z',
    });

    const { result, rerender } = renderHook(
      ({ spans }) =>
        useWaitStallAnalysis({
          traceStatus: 'RUNNING',
          traceStartedAt: TRACE_STARTED_AT,
          spans,
          events: [],
          hasTimelineSnapshot: true,
        }),
      {
        initialProps: {
          spans: [toolSpan],
        },
      }
    );

    expect(result.current?.classification).toBe('unknown');

    rerender({
      spans: [
        {
          ...toolSpan,
          status: 'STARTED',
        },
      ],
    });

    expect(result.current?.classification).toBe('waiting_on_tool');
    expect(result.current?.decisiveSpanId).toBe('tool-1');
  });
});
