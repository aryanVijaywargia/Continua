import { describe, expect, it } from 'vitest';
import { createSpan, createTimelineEvent } from '../test/traceFixtures';
import {
  buildTimelinePhases,
  buildTimelineRows,
  classifyTimelineLevel,
  classifyTimelineRow,
  findRootSpanId,
  formatOffset,
  timelineRowMessage,
  timelineRowMeta,
} from './timelineRows';

const TRACE_START_MS = new Date('2026-03-14T10:00:00.000Z').getTime();

describe('classifyTimelineRow', () => {
  it('classifies span lifecycle events as span rows', () => {
    for (const eventType of ['span_started', 'span_completed', 'span_failed'] as const) {
      expect(
        classifyTimelineRow(createTimelineEvent({ event_type: eventType }), undefined)
      ).toBe('span');
    }
  });

  it('classifies engine journal events as engine rows', () => {
    for (const eventType of ['snapshot_marker', 'state_change', 'decision'] as const) {
      expect(
        classifyTimelineRow(createTimelineEvent({ event_type: eventType }), undefined)
      ).toBe('engine');
    }
  });

  it('classifies synthetic root-span events as engine rows', () => {
    const event = createTimelineEvent({
      event_type: 'message',
      source: 'synthetic',
      span_id: 'root-uuid',
    });
    expect(classifyTimelineRow(event, 'root-uuid')).toBe('engine');
    expect(classifyTimelineRow(event, 'other-root')).toBe('log');
  });

  it('classifies everything else as log rows', () => {
    expect(
      classifyTimelineRow(createTimelineEvent({ event_type: 'log' }), undefined)
    ).toBe('log');
  });
});

describe('classifyTimelineLevel', () => {
  it('marks failures and error-level events as errors', () => {
    expect(classifyTimelineLevel(createTimelineEvent({ event_type: 'error' }))).toBe('error');
    expect(classifyTimelineLevel(createTimelineEvent({ event_type: 'exception' }))).toBe('error');
    expect(classifyTimelineLevel(createTimelineEvent({ event_type: 'span_failed' }))).toBe('error');
    expect(
      classifyTimelineLevel(createTimelineEvent({ event_type: 'log', level: 'error' }))
    ).toBe('error');
  });

  it('marks warnings and waits as warn', () => {
    expect(
      classifyTimelineLevel(createTimelineEvent({ event_type: 'log', level: 'warning' }))
    ).toBe('warn');
    expect(classifyTimelineLevel(createTimelineEvent({ event_type: 'wait' }))).toBe('warn');
  });

  it('defaults to info', () => {
    expect(classifyTimelineLevel(createTimelineEvent({ event_type: 'message' }))).toBe('info');
  });
});

describe('timelineRowMessage', () => {
  it('prefers the explicit message', () => {
    expect(
      timelineRowMessage(
        createTimelineEvent({ event_type: 'span_started', message: 'custom' })
      )
    ).toBe('custom');
  });

  it('describes span lifecycle events with the span name', () => {
    expect(
      timelineRowMessage(
        createTimelineEvent({ event_type: 'span_failed', span_name: 'Charge card' })
      )
    ).toBe('Span failed · Charge card');
  });

  it('humanizes unknown event types', () => {
    expect(timelineRowMessage(createTimelineEvent({ event_type: 'state_change' }))).toBe(
      'State change'
    );
    expect(timelineRowMessage(createTimelineEvent({ event_type: 'custom' }))).toBe('custom');
  });
});

describe('timelineRowMeta', () => {
  it('compacts up to three scalar payload entries', () => {
    const meta = timelineRowMeta(
      createTimelineEvent({
        payload: {
          message: 'skipped',
          kind: 'skipped',
          level: 'skipped',
          attempt: 2,
          tool: 'checkout',
          nested: { skipped: true },
          missing: null,
          fourth: 'dropped',
        },
      })
    );
    expect(meta).toBe('attempt=2 · tool=checkout');
  });

  it('returns undefined when nothing compacts', () => {
    expect(timelineRowMeta(createTimelineEvent({}))).toBeUndefined();
    expect(
      timelineRowMeta(createTimelineEvent({ payload: { message: 'only-reserved' } }))
    ).toBeUndefined();
  });
});

describe('formatOffset', () => {
  it('formats sub-second offsets in ms and larger offsets in seconds', () => {
    expect(formatOffset(999)).toBe('+999ms');
    expect(formatOffset(1500)).toBe('+1.50s');
  });
});

describe('buildTimelineRows', () => {
  it('sorts rows by offset and clamps pre-trace timestamps to zero', () => {
    const rows = buildTimelineRows(
      [
        createTimelineEvent({
          id: 'late',
          timestamp: '2026-03-14T10:00:02.000Z',
          event_type: 'log',
        }),
        createTimelineEvent({
          id: 'early',
          timestamp: '2026-03-14T09:59:59.000Z',
          event_type: 'log',
        }),
      ],
      undefined,
      TRACE_START_MS
    );

    expect(rows.map((row) => row.id)).toEqual(['early', 'late']);
    expect(rows[0]?.offsetMs).toBe(0);
    expect(rows[1]?.offsetMs).toBe(2000);
  });
});

describe('findRootSpanId / buildTimelinePhases', () => {
  it('builds phases for top-level spans only', () => {
    const root = createSpan({
      span_id: 'root',
      started_at: '2026-03-14T10:00:00.000Z',
      ended_at: '2026-03-14T10:00:04.000Z',
    });
    const child = createSpan({
      span_id: 'child',
      parent_span_id: 'root',
      started_at: '2026-03-14T10:00:01.000Z',
      ended_at: '2026-03-14T10:00:02.000Z',
    });
    const grandchild = createSpan({
      span_id: 'grandchild',
      parent_span_id: 'child',
      started_at: '2026-03-14T10:00:01.000Z',
      ended_at: '2026-03-14T10:00:02.000Z',
    });

    const rootSpanId = findRootSpanId([root, child, grandchild]);
    expect(rootSpanId).toBe(root.id);

    const phases = buildTimelinePhases(
      [root, child, grandchild],
      rootSpanId,
      TRACE_START_MS,
      4000
    );
    expect(phases.map((phase) => phase.id)).toEqual([root.id]);
    expect(phases[0]).toMatchObject({ startMs: 0, durationMs: 4000 });
  });

  it('extends open spans to the end of the window and drops zero-duration phases', () => {
    const open = createSpan({
      span_id: 'open',
      started_at: '2026-03-14T10:00:01.000Z',
      ended_at: undefined,
    });
    const zero = createSpan({
      span_id: 'zero',
      started_at: '2026-03-14T10:00:02.000Z',
      ended_at: '2026-03-14T10:00:02.000Z',
    });

    const phases = buildTimelinePhases([open, zero], undefined, TRACE_START_MS, 5000);
    expect(phases.map((phase) => phase.id)).toEqual([open.id]);
    expect(phases[0]).toMatchObject({ startMs: 1000, durationMs: 4000 });
  });
});
