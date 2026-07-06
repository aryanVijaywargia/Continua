import { describe, expect, it } from 'vitest';
import { createTimelineEvent } from '../test/traceFixtures';
import type { WaitStallAssessment } from './waitStallAnalysis';
import {
  formatRunningStateBasis,
  getRunningStatePanelTone,
  getRunningStateSummary,
  resolveDeclaredWaitKind,
} from './runningStateSummary';

function createAssessment(
  overrides: Partial<WaitStallAssessment> = {}
): WaitStallAssessment {
  return {
    classification: 'declared_wait',
    basis: 'declared',
    reason: 'open_declared_wait',
    latestActivityAt: null,
    runtimeMs: null,
    inactivityMs: null,
    ...overrides,
  };
}

describe('getRunningStateSummary', () => {
  it('labels each classification', () => {
    expect(getRunningStateSummary(createAssessment()).label).toBe('Declared wait');
    expect(
      getRunningStateSummary(createAssessment({ classification: 'waiting_on_model' })).label
    ).toBe('Waiting on model');
    expect(
      getRunningStateSummary(createAssessment({ classification: 'waiting_on_tool' })).label
    ).toBe('Waiting on tool');
    expect(
      getRunningStateSummary(createAssessment({ classification: 'possibly_stalled' })).label
    ).toBe('Possibly stalled');
    expect(
      getRunningStateSummary(createAssessment({ classification: 'unknown' })).label
    ).toBe('Unknown');
  });

  it('distinguishes between-span activity from an open running span', () => {
    const betweenSpans = getRunningStateSummary(
      createAssessment({
        classification: 'actively_executing',
        reason: 'recent_activity_without_open_span',
      })
    );
    expect(betweenSpans.copy).toContain('between spans');

    const openSpan = getRunningStateSummary(
      createAssessment({
        classification: 'actively_executing',
        reason: 'open_generic_span',
      })
    );
    expect(openSpan.copy).toContain('running span');
  });
});

describe('formatRunningStateBasis', () => {
  it('maps each basis to display copy', () => {
    expect(formatRunningStateBasis('declared')).toBe('Declared');
    expect(formatRunningStateBasis('inferred')).toBe('Inferred');
    expect(formatRunningStateBasis('heuristic')).toBe('Heuristic');
  });
});

describe('getRunningStatePanelTone', () => {
  it('uses the waiting tone for all wait classifications', () => {
    const waitingTone = getRunningStatePanelTone('declared_wait');
    expect(getRunningStatePanelTone('waiting_on_model')).toBe(waitingTone);
    expect(getRunningStatePanelTone('waiting_on_tool')).toBe(waitingTone);
    expect(getRunningStatePanelTone('possibly_stalled')).not.toBe(waitingTone);
  });
});

describe('resolveDeclaredWaitKind', () => {
  it('resolves the wait kind from the decisive declared-wait event', () => {
    const waitEvent = createTimelineEvent({
      id: 'wait-event',
      event_type: 'wait',
      payload: { wait_kind: 'human_approval', phase: 'entered' },
    });

    expect(
      resolveDeclaredWaitKind(
        createAssessment({ decisiveEventId: 'wait-event' }),
        [waitEvent]
      )
    ).toBe('human_approval');
  });

  it('returns null for non-declared classifications or missing events', () => {
    expect(
      resolveDeclaredWaitKind(
        createAssessment({ classification: 'waiting_on_model', decisiveEventId: 'x' }),
        []
      )
    ).toBeNull();
    expect(
      resolveDeclaredWaitKind(createAssessment({ decisiveEventId: 'missing' }), [])
    ).toBeNull();
    expect(resolveDeclaredWaitKind(createAssessment(), [])).toBeNull();
  });
});
