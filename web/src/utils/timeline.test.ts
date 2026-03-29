import { describe, expect, it } from 'vitest';
import { createTimelineEvent } from '../test/traceFixtures';
import { summarizeTimelineEvent } from './timeline';

describe('summarizeTimelineEvent', () => {
  it('summarizes state changes from semantic payload fields', () => {
    const event = createTimelineEvent({
      event_type: 'state_change',
      message: 'fallback message',
      payload: {
        key: 'status',
        old_value: 'pending',
        new_value: 'approved',
      },
    });

    expect(summarizeTimelineEvent(event)).toBe('status: pending → approved');
  });

  it('summarizes decisions from semantic payload fields', () => {
    const event = createTimelineEvent({
      event_type: 'decision',
      message: 'fallback message',
      payload: {
        question: 'Which model?',
        chosen: 'gpt-4.1',
      },
    });

    expect(summarizeTimelineEvent(event)).toBe('Which model? → gpt-4.1');
  });

  it('falls back to the message when semantic fields are missing', () => {
    const stateEvent = createTimelineEvent({
      event_type: 'state_change',
      message: 'state fallback',
      payload: {
        old_value: 'pending',
        new_value: 'approved',
      },
    });
    const decisionEvent = createTimelineEvent({
      event_type: 'decision',
      message: 'decision fallback',
      payload: {
        question: 'Which model?',
      },
    });

    expect(summarizeTimelineEvent(stateEvent)).toBe('state fallback');
    expect(summarizeTimelineEvent(decisionEvent)).toBe('decision fallback');
  });

  it('summarizes well-formed wait events', () => {
    const enteredEvent = createTimelineEvent({
      event_type: 'wait',
      payload: {
        wait_kind: 'tool_call',
        phase: 'entered',
      },
    });
    const resolvedEvent = createTimelineEvent({
      event_type: 'wait',
      payload: {
        wait_kind: 'model_response',
        phase: 'resolved',
        resolution: 'success',
      },
    });

    expect(summarizeTimelineEvent(enteredEvent)).toBe('Entered wait: tool_call');
    expect(summarizeTimelineEvent(resolvedEvent)).toBe(
      'Resolved wait: model_response → success'
    );
  });

  it('falls back to the phase label for malformed wait events with a phase', () => {
    const event = createTimelineEvent({
      event_type: 'wait',
      payload: {
        phase: 'paused',
      },
    });

    expect(summarizeTimelineEvent(event)).toBe('Paused wait');
  });

  it('falls back to existing generic behavior for fully malformed wait events', () => {
    const withMessage = createTimelineEvent({
      event_type: 'wait',
      message: 'generic wait fallback',
      payload: {},
    });
    const withoutMessage = createTimelineEvent({
      event_type: 'wait',
      payload: {},
    });
    const nonWaitEvent = createTimelineEvent({
      event_type: 'log',
      message: 'plain log',
    });

    expect(summarizeTimelineEvent(withMessage)).toBe('generic wait fallback');
    expect(summarizeTimelineEvent(withoutMessage)).toBe('wait');
    expect(summarizeTimelineEvent(nonWaitEvent)).toBe('plain log');
  });
});
