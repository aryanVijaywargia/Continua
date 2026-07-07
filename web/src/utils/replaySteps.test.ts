import { describe, expect, it } from 'vitest';
import { createSpan } from '../test/traceFixtures';
import { buildReplaySteps } from './replaySteps';

describe('buildReplaySteps', () => {
  it('returns no steps without spans', () => {
    expect(buildReplaySteps([], undefined, false)).toEqual([]);
  });

  it('splits steps around the failed span', () => {
    const before = createSpan({ span_id: 'before' });
    const failed = createSpan({ span_id: 'failed', status: 'FAILED' });
    const after = createSpan({ span_id: 'after', status: 'SCHEDULED' });

    const steps = buildReplaySteps([before, failed, after], failed, false);
    expect(steps.map((step) => step.status)).toEqual(['replayed', 'current', 'pending']);
    expect(steps.map((step) => step.mock)).toEqual([false, false, false]);
    expect(steps[1]).toMatchObject({ id: failed.id, spanId: 'failed' });
  });

  it('marks the failed span onward as mocked when mocking failures', () => {
    const before = createSpan({ span_id: 'before' });
    const failed = createSpan({ span_id: 'failed', status: 'FAILED' });
    const after = createSpan({ span_id: 'after', status: 'SCHEDULED' });

    const steps = buildReplaySteps([before, failed, after], failed, true);
    expect(steps.map((step) => step.mock)).toEqual([false, true, true]);
  });

  it('marks completed spans as replayed when nothing failed', () => {
    const completed = createSpan({ span_id: 'done', status: 'COMPLETED' });
    const running = createSpan({ span_id: 'running', status: 'STARTED' });

    const steps = buildReplaySteps([completed, running], undefined, true);
    expect(steps.map((step) => step.status)).toEqual(['replayed', 'pending']);
    // no failure -> nothing to mock even with the toggle on
    expect(steps.map((step) => step.mock)).toEqual([false, false]);
  });
});
