import { beforeEach, describe, expect, it } from 'vitest';
import {
  createSpan,
  createTimelineEvent,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import {
  deriveWaterfallWindow,
  getWaterfallBarLayout,
} from './waterfallTime';

beforeEach(() => {
  resetTestEntityCounter();
});

describe('waterfallTime', () => {
  it('uses the latest span or event timestamp as the right boundary for running traces', () => {
    const spans = [
      createSpan({
        span_id: 'root',
        started_at: '2026-03-14T10:00:00.000Z',
        ended_at: undefined,
      }),
    ];
    const events = [
      createTimelineEvent({
        timestamp: '2026-03-14T10:00:05.000Z',
      }),
    ];

    const window = deriveWaterfallWindow({
      traceStartedAt: '2026-03-14T10:00:00.000Z',
      traceEndedAt: undefined,
      spans,
      events,
    });

    expect(window).not.toBeNull();
    expect(window?.durationMs).toBe(5000);
  });

  it('keeps zero-duration spans anchored without producing negative layout values', () => {
    const window = deriveWaterfallWindow({
      traceStartedAt: '2026-03-14T10:00:00.000Z',
      traceEndedAt: '2026-03-14T10:00:10.000Z',
      spans: [
        createSpan({
          span_id: 'instant',
          started_at: '2026-03-14T10:00:03.000Z',
          ended_at: '2026-03-14T10:00:03.000Z',
        }),
      ],
      events: [],
    });

    if (!window) {
      throw new Error('expected waterfall window');
    }

    const layout = getWaterfallBarLayout(
      createSpan({
        span_id: 'instant',
        started_at: '2026-03-14T10:00:03.000Z',
        ended_at: '2026-03-14T10:00:03.000Z',
      }),
      window
    );

    expect(layout.leftPercent).toBe(30);
    expect(layout.widthPercent).toBe(0);
    expect(layout.durationMs).toBe(0);
  });

  it('extends running spans to the trace boundary', () => {
    const window = deriveWaterfallWindow({
      traceStartedAt: '2026-03-14T10:00:00.000Z',
      traceEndedAt: undefined,
      spans: [
        createSpan({
          span_id: 'running',
          started_at: '2026-03-14T10:00:02.000Z',
          ended_at: undefined,
        }),
      ],
      events: [
        createTimelineEvent({
          timestamp: '2026-03-14T10:00:05.000Z',
        }),
      ],
    });

    if (!window) {
      throw new Error('expected waterfall window');
    }

    const layout = getWaterfallBarLayout(
      createSpan({
        span_id: 'running',
        started_at: '2026-03-14T10:00:02.000Z',
        ended_at: undefined,
      }),
      window
    );

    expect(layout.isRunning).toBe(true);
    expect(layout.durationMs).toBe(3000);
    expect(layout.widthPercent).toBe(60);
  });
});
