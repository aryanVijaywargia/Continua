import { describe, expect, it } from 'vitest';
import { createTimelineEvent } from '../test/traceFixtures';
import { extractStateChanges } from './stateChanges';

describe('extractStateChanges', () => {
  it('filters to state_change events with payload.key', () => {
    const changes = extractStateChanges([
      createTimelineEvent({
        id: 'state-valid',
        event_type: 'state_change',
        payload: {
          key: 'status',
          namespace: 'order',
          old_value: 'pending',
          new_value: 'approved',
        },
      }),
      createTimelineEvent({
        id: 'state-missing-key',
        event_type: 'state_change',
        payload: {
          old_value: 'pending',
          new_value: 'approved',
        },
      }),
      createTimelineEvent({
        id: 'decision',
        event_type: 'decision',
        payload: {
          question: 'Which model?',
          chosen: 'gpt-4.1',
        },
      }),
    ]);

    expect(changes).toEqual([
      {
        event: expect.objectContaining({ id: 'state-valid' }),
        key: 'status',
        namespace: 'order',
        oldValue: 'pending',
        newValue: 'approved',
      },
    ]);
  });

  it('returns an empty array when no valid state changes exist', () => {
    expect(
      extractStateChanges([
        createTimelineEvent({ event_type: 'message', message: 'hello' }),
      ])
    ).toEqual([]);
  });

  it('preserves interleaved namespaces and missing optional values', () => {
    const changes = extractStateChanges([
      createTimelineEvent({
        id: 'general-change',
        event_type: 'state_change',
        payload: {
          key: 'phase',
          new_value: 'running',
        },
      }),
      createTimelineEvent({
        id: 'order-change',
        event_type: 'state_change',
        payload: {
          key: 'status',
          namespace: 'order',
          old_value: 'pending',
          new_value: 'approved',
        },
      }),
      createTimelineEvent({
        id: 'network-change',
        event_type: 'state_change',
        payload: {
          key: 'retry_count',
          namespace: 'network',
          new_value: 2,
        },
      }),
    ]);

    expect(changes).toEqual([
      {
        event: expect.objectContaining({ id: 'general-change' }),
        key: 'phase',
        namespace: undefined,
        oldValue: undefined,
        newValue: 'running',
      },
      {
        event: expect.objectContaining({ id: 'order-change' }),
        key: 'status',
        namespace: 'order',
        oldValue: 'pending',
        newValue: 'approved',
      },
      {
        event: expect.objectContaining({ id: 'network-change' }),
        key: 'retry_count',
        namespace: 'network',
        oldValue: undefined,
        newValue: 2,
      },
    ]);
  });
});
