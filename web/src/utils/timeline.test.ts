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

  it('summarizes mutating effects from semantic payload fields', () => {
    const event = createTimelineEvent({
      event_type: 'effect',
      message: 'fallback effect',
      payload: {
        effect_kind: 'api_call',
        has_external_side_effect: true,
      },
    });

    expect(summarizeTimelineEvent(event)).toBe('api_call (mutating)');
  });

  it('summarizes read-only effects from semantic payload fields', () => {
    const event = createTimelineEvent({
      event_type: 'effect',
      message: 'fallback effect',
      payload: {
        effect_kind: 'model_call',
        has_external_side_effect: false,
      },
    });

    expect(summarizeTimelineEvent(event)).toBe('model_call (read-only)');
  });

  it('falls back to the message for malformed effects', () => {
    const event = createTimelineEvent({
      event_type: 'effect',
      message: 'Custom effect note',
      payload: {
        effect_kind: 'api_call',
      },
    });

    expect(summarizeTimelineEvent(event)).toBe('Custom effect note');
  });

  it('falls back to the event type for malformed effects without a message', () => {
    const event = createTimelineEvent({
      event_type: 'effect',
      payload: {
        effect_kind: 'api_call',
      },
    });

    expect(summarizeTimelineEvent(event)).toBe('effect');
  });

  it('summarizes waits with a resolution from semantic payload fields', () => {
    const event = createTimelineEvent({
      event_type: 'wait',
      message: 'fallback wait',
      payload: {
        wait_kind: 'human_approval',
        phase: 'resolved',
        resolution: 'approved',
      },
    });

    expect(summarizeTimelineEvent(event)).toBe(
      'human_approval (resolved) → approved'
    );
  });

  it('summarizes waits without a resolution from semantic payload fields', () => {
    const event = createTimelineEvent({
      event_type: 'wait',
      message: 'fallback wait',
      payload: {
        wait_kind: 'human_approval',
        phase: 'entered',
      },
    });

    expect(summarizeTimelineEvent(event)).toBe('human_approval (entered)');
  });

  it('falls back to the message for malformed waits', () => {
    const event = createTimelineEvent({
      event_type: 'wait',
      message: 'Custom wait note',
      payload: {
        wait_kind: 'human_approval',
      },
    });

    expect(summarizeTimelineEvent(event)).toBe('Custom wait note');
  });

  it('falls back to the event type for malformed waits without a message', () => {
    const event = createTimelineEvent({
      event_type: 'wait',
      payload: {
        wait_kind: 'human_approval',
      },
    });

    expect(summarizeTimelineEvent(event)).toBe('wait');
  });
});
