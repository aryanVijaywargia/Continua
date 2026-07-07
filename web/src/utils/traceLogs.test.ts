import { describe, expect, it } from 'vitest';
import { createTimelineEvent } from '../test/traceFixtures';
import {
  buildLogLines,
  deriveLogLevel,
  deriveLogSource,
  formatLogLinesAsText,
  formatLogTimestamp,
} from './traceLogs';

describe('deriveLogLevel', () => {
  it('maps failures and error levels to error', () => {
    expect(deriveLogLevel(createTimelineEvent({ event_type: 'error' }))).toBe('error');
    expect(deriveLogLevel(createTimelineEvent({ event_type: 'exception' }))).toBe('error');
    expect(deriveLogLevel(createTimelineEvent({ event_type: 'span_failed' }))).toBe('error');
    expect(
      deriveLogLevel(createTimelineEvent({ event_type: 'log', level: 'error' }))
    ).toBe('error');
  });

  it('maps warnings, waits, and debug levels', () => {
    expect(
      deriveLogLevel(createTimelineEvent({ event_type: 'log', level: 'warning' }))
    ).toBe('warn');
    expect(deriveLogLevel(createTimelineEvent({ event_type: 'wait' }))).toBe('warn');
    expect(
      deriveLogLevel(createTimelineEvent({ event_type: 'log', level: 'debug' }))
    ).toBe('debug');
    expect(deriveLogLevel(createTimelineEvent({ event_type: 'message' }))).toBe('info');
  });
});

describe('deriveLogSource', () => {
  it('prefers the span name, then engine sources, then trace', () => {
    expect(
      deriveLogSource(createTimelineEvent({ span_name: 'Charge card' }))
    ).toBe('Charge card');
    expect(
      deriveLogSource(createTimelineEvent({ event_type: 'snapshot_marker' }))
    ).toBe('engine.checkpoint');
    expect(
      deriveLogSource(createTimelineEvent({ event_type: 'state_change' }))
    ).toBe('engine.state');
    expect(
      deriveLogSource(createTimelineEvent({ event_type: 'decision' }))
    ).toBe('engine.decision');
    expect(deriveLogSource(createTimelineEvent({ event_type: 'log' }))).toBe('trace');
  });
});

describe('formatLogTimestamp', () => {
  it('formats to hh:mm:ss.mmm and passes through invalid input', () => {
    const formatted = formatLogTimestamp('2026-03-14T10:02:03.456Z');
    expect(formatted).toMatch(/^\d{2}:\d{2}:\d{2}\.456$/);
    expect(formatLogTimestamp('not-a-date')).toBe('not-a-date');
  });
});

describe('buildLogLines', () => {
  it('keeps explicit events only and falls back to humanized event types', () => {
    const lines = buildLogLines([
      createTimelineEvent({
        id: 'explicit',
        source: 'explicit',
        span_id: 'span-1',
        span_name: 'Checkout',
        message: 'called tool',
      }),
      createTimelineEvent({
        id: 'synthetic',
        source: 'synthetic',
        event_type: 'span_started',
      }),
      createTimelineEvent({
        id: 'no-message',
        source: 'explicit',
        event_type: 'state_change',
      }),
    ]);

    expect(lines.map((line) => line.id)).toEqual(['explicit', 'no-message']);
    expect(lines[0]).toMatchObject({
      source: 'Checkout',
      message: 'called tool',
      spanId: 'span-1',
    });
    expect(lines[1]?.message).toBe('state change');
  });
});

describe('formatLogLinesAsText', () => {
  it('renders one padded line per log entry', () => {
    const [line] = buildLogLines([
      createTimelineEvent({
        source: 'explicit',
        level: 'warning',
        span_name: 'Checkout',
        message: 'slow tool',
        timestamp: '2026-03-14T10:02:03.456Z',
      }),
    ]);

    expect(formatLogLinesAsText([line])).toBe(
      `${line.hms} WARN  Checkout — slow tool`
    );
  });
});
