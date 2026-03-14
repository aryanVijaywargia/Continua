import { describe, expect, it } from 'vitest';
import {
  parseSpanParam,
  serializeSpanParam,
} from './traceDetailSearchParams';

describe('traceDetailSearchParams', () => {
  it('parses a present span parameter', () => {
    expect(parseSpanParam(new URLSearchParams('span=abc'))).toBe('abc');
  });

  it('normalizes empty and missing span parameters to null', () => {
    expect(parseSpanParam(new URLSearchParams('span='))).toBeNull();
    expect(parseSpanParam(new URLSearchParams('debug=true'))).toBeNull();
  });

  it('preserves unrelated params when serializing span changes', () => {
    const next = serializeSpanParam(
      new URLSearchParams('debug=true&span=abc'),
      'def'
    );

    expect(next.toString()).toBe('debug=true&span=def');
  });

  it('removes span while preserving unrelated params', () => {
    const next = serializeSpanParam(
      new URLSearchParams('debug=true&span=abc'),
      null
    );

    expect(next.toString()).toBe('debug=true');
  });
});
