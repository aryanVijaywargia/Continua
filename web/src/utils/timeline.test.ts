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

  it('summarizes well-formed snapshot markers from semantic payload fields', () => {
    const event = createTimelineEvent({
      event_type: 'snapshot_marker',
      message: 'fallback marker message',
      payload: {
        marker_kind: 'milestone',
        label: 'Data ingestion complete',
      },
    });

    expect(summarizeTimelineEvent(event)).toBe('Data ingestion complete');
  });

  it('falls back to the message for malformed snapshot markers', () => {
    const event = createTimelineEvent({
      event_type: 'snapshot_marker',
      message: 'fallback marker message',
      payload: {
        marker_kind: 'milestone',
      },
    });

    expect(summarizeTimelineEvent(event)).toBe('fallback marker message');
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
