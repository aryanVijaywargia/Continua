import { describe, expect, it } from 'vitest';
import { createTimelineEvent } from '../test/traceFixtures';
import { getWaitDetails } from './eventSemantics';

describe('getWaitDetails', () => {
  it('returns structured details for a well-formed wait event', () => {
    const event = createTimelineEvent({
      event_type: 'wait',
      payload: {
        wait_kind: 'model_response',
        phase: 'entered',
        wait_id: 'wait-1',
        resolution: 'success',
      },
    });

    expect(getWaitDetails(event)).toEqual({
      waitKind: 'model_response',
      phase: 'entered',
      waitId: 'wait-1',
      resolution: 'success',
    });
  });

  it('accepts a minimal well-formed wait event', () => {
    const event = createTimelineEvent({
      event_type: 'wait',
      payload: {
        wait_kind: 'tool_call',
        phase: 'resolved',
      },
    });

    expect(getWaitDetails(event)).toEqual({
      waitKind: 'tool_call',
      phase: 'resolved',
      waitId: undefined,
      resolution: undefined,
    });
  });

  it('returns null for malformed wait payloads', () => {
    const missingWaitKind = createTimelineEvent({
      event_type: 'wait',
      payload: {
        phase: 'entered',
        wait_id: 'wait-1',
      },
    });
    const emptyPhase = createTimelineEvent({
      event_type: 'wait',
      payload: {
        wait_kind: 'model_response',
        phase: '',
      },
    });

    expect(getWaitDetails(missingWaitKind)).toBeNull();
    expect(getWaitDetails(emptyPhase)).toBeNull();
  });

  it('returns null for non-wait events', () => {
    const event = createTimelineEvent({
      event_type: 'effect',
      payload: {
        wait_kind: 'model_response',
        phase: 'entered',
      },
    });

    expect(getWaitDetails(event)).toBeNull();
  });
});
