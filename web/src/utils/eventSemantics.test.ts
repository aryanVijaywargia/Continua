import { describe, expect, it } from 'vitest';
import { createTimelineEvent } from '../test/traceFixtures';
import { getEffectDetails, getWaitDetails } from './eventSemantics';

describe('getEffectDetails', () => {
  it('extracts a well-formed effect payload with all fields', () => {
    const event = createTimelineEvent({
      event_type: 'effect',
      payload: {
        effect_kind: 'api_call',
        has_external_side_effect: true,
        effect_id: 'effect_abc123',
        idempotent: false,
        idempotency_key: 'key-1',
      },
    });

    expect(getEffectDetails(event)).toEqual({
      effectKind: 'api_call',
      hasExternalSideEffect: true,
      effectId: 'effect_abc123',
      idempotent: false,
      idempotencyKey: 'key-1',
    });
  });

  it('extracts a minimal effect payload with required fields only', () => {
    const event = createTimelineEvent({
      event_type: 'effect',
      payload: {
        effect_kind: 'model_call',
        has_external_side_effect: false,
      },
    });

    expect(getEffectDetails(event)).toEqual({
      effectKind: 'model_call',
      hasExternalSideEffect: false,
      effectId: undefined,
      idempotent: undefined,
      idempotencyKey: undefined,
    });
  });

  it('returns null when required fields are missing', () => {
    const event = createTimelineEvent({
      event_type: 'effect',
      payload: {
        effect_kind: 'model_call',
      },
    });

    expect(getEffectDetails(event)).toBeNull();
  });

  it('returns null for the wrong event type', () => {
    const event = createTimelineEvent({
      event_type: 'log',
      payload: {
        effect_kind: 'api_call',
        has_external_side_effect: true,
      },
    });

    expect(getEffectDetails(event)).toBeNull();
  });

  it('returns null when the payload is missing', () => {
    const event = createTimelineEvent({
      event_type: 'effect',
      payload: undefined,
    });

    expect(getEffectDetails(event)).toBeNull();
  });

  it.each([
    {
      label: 'effect_kind is not a string',
      payload: {
        effect_kind: 123,
        has_external_side_effect: true,
      },
    },
    {
      label: 'effect_kind is an empty string',
      payload: {
        effect_kind: '',
        has_external_side_effect: true,
      },
    },
    {
      label: 'has_external_side_effect is not a boolean',
      payload: {
        effect_kind: 'api_call',
        has_external_side_effect: 'true',
      },
    },
    {
      label: 'effect_id is not a string',
      payload: {
        effect_kind: 'api_call',
        has_external_side_effect: true,
        effect_id: 42,
      },
    },
    {
      label: 'idempotent is not a boolean',
      payload: {
        effect_kind: 'api_call',
        has_external_side_effect: true,
        idempotent: 'yes',
      },
    },
    {
      label: 'idempotency_key is not a string',
      payload: {
        effect_kind: 'api_call',
        has_external_side_effect: true,
        idempotency_key: 42,
      },
    },
  ])('returns null when $label', ({ payload }) => {
    const event = createTimelineEvent({
      event_type: 'effect',
      payload,
    });

    expect(getEffectDetails(event)).toBeNull();
  });
});

describe('getWaitDetails', () => {
  it('extracts a well-formed wait payload with all fields', () => {
    const event = createTimelineEvent({
      event_type: 'wait',
      payload: {
        wait_kind: 'human_approval',
        phase: 'resolved',
        resolution: 'approved',
        wait_id: 'wait_xyz789',
      },
    });

    expect(getWaitDetails(event)).toEqual({
      waitKind: 'human_approval',
      phase: 'resolved',
      resolution: 'approved',
      waitId: 'wait_xyz789',
    });
  });

  it('extracts a minimal wait payload with required fields only', () => {
    const event = createTimelineEvent({
      event_type: 'wait',
      payload: {
        wait_kind: 'human_approval',
        phase: 'entered',
      },
    });

    expect(getWaitDetails(event)).toEqual({
      waitKind: 'human_approval',
      phase: 'entered',
      resolution: undefined,
      waitId: undefined,
    });
  });

  it('returns null when required fields are missing', () => {
    const event = createTimelineEvent({
      event_type: 'wait',
      payload: {
        wait_kind: 'external',
      },
    });

    expect(getWaitDetails(event)).toBeNull();
  });

  it('returns null for the wrong event type', () => {
    const event = createTimelineEvent({
      event_type: 'decision',
      payload: {
        wait_kind: 'human_approval',
        phase: 'entered',
      },
    });

    expect(getWaitDetails(event)).toBeNull();
  });

  it('returns null when the payload is missing', () => {
    const event = createTimelineEvent({
      event_type: 'wait',
      payload: undefined,
    });

    expect(getWaitDetails(event)).toBeNull();
  });

  it.each([
    {
      label: 'wait_kind is not a string',
      payload: {
        wait_kind: 123,
        phase: 'entered',
      },
    },
    {
      label: 'wait_kind is an empty string',
      payload: {
        wait_kind: '',
        phase: 'entered',
      },
    },
    {
      label: 'phase is not a string',
      payload: {
        wait_kind: 'human_approval',
        phase: false,
      },
    },
    {
      label: 'phase is an empty string',
      payload: {
        wait_kind: 'human_approval',
        phase: '',
      },
    },
    {
      label: 'resolution is not a string',
      payload: {
        wait_kind: 'human_approval',
        phase: 'resolved',
        resolution: 42,
      },
    },
    {
      label: 'wait_id is not a string',
      payload: {
        wait_kind: 'human_approval',
        phase: 'entered',
        wait_id: 42,
      },
    },
  ])('returns null when $label', ({ payload }) => {
    const event = createTimelineEvent({
      event_type: 'wait',
      payload,
    });

    expect(getWaitDetails(event)).toBeNull();
  });
});
