import { describe, expect, it } from 'vitest';
import { describeEngineWaitState } from './engineWaitState';

describe('describeEngineWaitState', () => {
  it('describes replay mismatch waits with expected and actual event detail', () => {
    const summary = describeEngineWaitState({
      kind: 'replay_mismatch',
      expected_type: 'activity_scheduled',
      expected_key: 'charge-card',
      actual_type: 'timer_started',
      actual_key: 'timeout',
      detail: 'replay produced a different next event',
    });

    expect(summary?.heading).toBe('Replay mismatch');
    expect(summary?.detail).toContain('activity_scheduled');
    expect(summary?.detail).toContain('timer_started');
    expect(summary?.heading).not.toBe('Waiting on engine state');
  });

  it('describes engine invariant waits', () => {
    const summary = describeEngineWaitState({
      kind: 'engine_invariant',
      detail: 'sequence gap detected',
    });

    expect(summary?.heading).toBe('Engine invariant');
    expect(summary?.detail).toContain('sequence gap detected');
  });

  it('keeps the generic engine-state fallback for unknown kinds', () => {
    const summary = describeEngineWaitState({ kind: 'mystery' });

    expect(summary).toEqual({
      heading: 'Waiting on engine state',
      detail: 'mystery',
    });
  });
});
