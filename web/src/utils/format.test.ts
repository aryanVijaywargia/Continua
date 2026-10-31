import { describe, expect, it } from 'vitest';
import { formatBytes, getTraceTiming } from './format';

describe('formatBytes', () => {
  it('formats byte counts across supported units', () => {
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(2048)).toBe('2.0 KB');
    expect(formatBytes(1048576)).toBe('1.0 MB');
    expect(formatBytes(1073741824)).toBe('1.0 GB');
  });
});

describe('getTraceTiming', () => {
  const startedAt = '2026-03-14T10:00:00.000Z';

  it.each([
    ['COMPLETED', '2026-03-14T10:00:02.000Z', { running: false, durationMs: 2000 }],
    ['COMPLETED', undefined, { running: false, durationMs: null }],
    ['FAILED', undefined, { running: false, durationMs: null }],
  ] as const)('reads a %s trace ending at %s', (status, endedAt, expected) => {
    expect(getTraceTiming({ status, started_at: startedAt, ended_at: endedAt })).toEqual(expected);
  });

  it('runs the duration to now only for a running trace without an end', () => {
    const timing = getTraceTiming({ status: 'RUNNING', started_at: startedAt });
    expect(timing.running).toBe(true);
    expect(timing.durationMs).toBeGreaterThan(0);
  });
});
