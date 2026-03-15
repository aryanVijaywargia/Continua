import { describe, expect, it } from 'vitest';
import {
  buildCanonicalQueryString,
  clearChip,
  deriveActiveChips,
  isoToLocalDateInputValue,
  localDateToISOEnd,
  localDateToISOStart,
  parseTracesParams,
  serializeTracesParams,
} from './tracesSearchParams';

describe('tracesSearchParams', () => {
  it('normalizes parsed params and round-trips through serialization', () => {
    const params = new URLSearchParams({
      offset: '20',
      q: '  checkout flow  ',
      status: 'ERROR',
      start_time_from: '2026-03-14T00:00:00+05:30',
      start_time_to: '2026-03-14T23:59:59+05:30',
      user_id: '  user-123  ',
      has_errors: 'true',
      min_duration_ms: '2500',
      session_id: 'ABCDEF12-3456-7890-ABCD-EF1234567890',
    });

    const parsed = parseTracesParams(params);

    expect(parsed).toMatchObject({
      limit: 20,
      offset: 20,
      q: 'checkout flow',
      status: 'failed',
      start_time_from: '2026-03-13T18:30:00.000Z',
      start_time_to: '2026-03-14T18:29:59.000Z',
      user_id: 'user-123',
      has_errors: true,
      min_duration_ms: 2500,
      session_id: 'abcdef12-3456-7890-abcd-ef1234567890',
    });

    expect(serializeTracesParams(parsed).toString()).toBe(
      [
        'offset=20',
        'session_id=abcdef12-3456-7890-abcd-ef1234567890',
        'q=checkout+flow',
        'status=failed',
        'start_time_from=2026-03-13T18%3A30%3A00.000Z',
        'start_time_to=2026-03-14T18%3A29%3A59.000Z',
        'user_id=user-123',
        'has_errors=true',
        'min_duration_ms=2500',
      ].join('&')
    );
  });

  it('drops malformed values during parsing', () => {
    const parsed = parseTracesParams(
      new URLSearchParams({
        offset: '-5',
        q: '   ',
        status: 'pending',
        session_id: 'not-a-uuid',
        start_time_from: '2026-03-14',
        start_time_to: 'still-not-a-date',
        has_errors: 'TRUE',
        min_duration_ms: '1.9',
      })
    );

    expect(parsed).toMatchObject({ limit: 20, offset: 0 });
    expect(deriveActiveChips(parsed)).toEqual([]);
  });

  it('treats zero, negative, and non-integer min duration values as unset', () => {
    expect(
      parseTracesParams(new URLSearchParams({ min_duration_ms: '0' })).min_duration_ms
    ).toBeUndefined();
    expect(
      parseTracesParams(new URLSearchParams({ min_duration_ms: '-10' })).min_duration_ms
    ).toBeUndefined();
    expect(
      parseTracesParams(new URLSearchParams({ min_duration_ms: '1.9' })).min_duration_ms
    ).toBeUndefined();
  });

  it('converts local date boundaries to ISO timestamps', () => {
    const start = new Date(localDateToISOStart('2026-03-14'));
    const end = new Date(localDateToISOEnd('2026-03-14'));

    expect(start.getFullYear()).toBe(2026);
    expect(start.getMonth()).toBe(2);
    expect(start.getDate()).toBe(14);
    expect(start.getHours()).toBe(0);
    expect(start.getMinutes()).toBe(0);
    expect(start.getSeconds()).toBe(0);
    expect(start.getMilliseconds()).toBe(0);

    expect(end.getFullYear()).toBe(2026);
    expect(end.getMonth()).toBe(2);
    expect(end.getDate()).toBe(14);
    expect(end.getHours()).toBe(23);
    expect(end.getMinutes()).toBe(59);
    expect(end.getSeconds()).toBe(59);
    expect(end.getMilliseconds()).toBe(999);
  });

  it('derives active chips and clears individual filters while resetting pagination', () => {
    const state = parseTracesParams(
      new URLSearchParams({
        offset: '40',
        q: 'trace search',
        has_errors: 'true',
        session_id: '123e4567-e89b-12d3-a456-426614174000',
      })
    );

    expect(deriveActiveChips(state)).toEqual([
      { key: 'q', label: 'Search', value: 'trace search' },
      { key: 'has_errors', label: 'Errors', value: 'Only traces with errors' },
      {
        key: 'session_id',
        label: 'Session',
        value: '123e4567-e89b-12d3-a456-426614174000',
      },
    ]);

    expect(clearChip(state, 'q')).toMatchObject({
      limit: 20,
      offset: 0,
      has_errors: true,
      session_id: '123e4567-e89b-12d3-a456-426614174000',
    });
  });

  it('builds identical canonical strings for equivalent params', () => {
    const first = buildCanonicalQueryString(
      parseTracesParams(
        new URLSearchParams({
          q: '  flow  ',
          status: 'FAILED',
          has_errors: 'true',
          offset: '20',
          session_id: '123E4567-E89B-12D3-A456-426614174000',
        })
      )
    );

    const second = buildCanonicalQueryString(
      parseTracesParams(
        new URLSearchParams({
          session_id: '123e4567-e89b-12d3-a456-426614174000',
          offset: '20',
          has_errors: 'true',
          status: 'error',
          q: 'flow',
        })
      )
    );

    expect(first).toBe(
      'limit=20&offset=20&session_id=123e4567-e89b-12d3-a456-426614174000&q=flow&status=failed&has_errors=true'
    );
    expect(second).toBe(first);
  });

  it('preserves valid session ids and formats ISO dates for date inputs', () => {
    const parsed = parseTracesParams(
      new URLSearchParams({
        session_id: '123e4567-e89b-12d3-a456-426614174000',
        start_time_from: '2026-03-14T00:00:00.000Z',
      })
    );

    expect(parsed.session_id).toBe('123e4567-e89b-12d3-a456-426614174000');
    expect(isoToLocalDateInputValue(parsed.start_time_from)).toBe('2026-03-14');
  });
});
