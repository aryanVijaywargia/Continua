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
});
