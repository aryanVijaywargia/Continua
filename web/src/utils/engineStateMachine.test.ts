import { describe, expect, it } from 'vitest';
import {
  buildEngineStateMachine,
  isTerminalEngineStatus,
} from './engineStateMachine';
import type { EngineRunStatus } from '../api/client';

describe('buildEngineStateMachine', () => {
  it('marks a running workflow as current on the running step', () => {
    const steps = buildEngineStateMachine('RUNNING');
    expect(steps.map((step) => step.id)).toEqual([
      'created',
      'running',
      'waiting',
      'closed',
    ]);
    expect(steps[1]).toMatchObject({ done: true, current: true });
    expect(steps[3]).toMatchObject({ label: 'Closing', done: false, error: false });
  });

  it('keeps the running step pending while queued', () => {
    const steps = buildEngineStateMachine('QUEUED');
    expect(steps[1]).toMatchObject({ done: false, current: false });
  });

  it('flags waiting, suspended, and quarantined runs as a warned waiting step', () => {
    for (const status of ['WAITING', 'SUSPENDED', 'QUARANTINED'] as const) {
      const steps = buildEngineStateMachine(status as EngineRunStatus);
      expect(steps[2]).toMatchObject({
        label: status === 'QUARANTINED' ? 'Quarantined' : 'Waiting',
        current: true,
        warn: true,
        done: true,
      });
    }
  });

  it('closes completed and continued-as-new runs without an error', () => {
    for (const status of ['COMPLETED', 'CONTINUED_AS_NEW'] as const) {
      const steps = buildEngineStateMachine(status);
      expect(steps[3]).toMatchObject({ label: 'Closed', done: true, error: false });
    }
  });

  it('marks failed, cancelled, and terminated runs as failed', () => {
    for (const status of ['FAILED', 'CANCELLED', 'TERMINATED'] as const) {
      const steps = buildEngineStateMachine(status);
      expect(steps[3]).toMatchObject({ label: 'Failed', done: true, error: true });
    }
  });
});

describe('isTerminalEngineStatus', () => {
  it('treats closed shells as terminal and live states as not', () => {
    for (const status of [
      'COMPLETED',
      'FAILED',
      'CANCELLED',
      'TERMINATED',
      'CONTINUED_AS_NEW',
    ] as const) {
      expect(isTerminalEngineStatus(status)).toBe(true);
    }
    for (const status of [
      'QUEUED',
      'RUNNING',
      'WAITING',
      'SUSPENDED',
      'QUARANTINED',
    ] as const) {
      expect(isTerminalEngineStatus(status as EngineRunStatus)).toBe(false);
    }
  });
});
