import { describe, expect, it } from 'vitest';
import type { Trace, TraceDetail } from '../api/client';
import { TRACE_DETAIL, TRACE_ONE } from '../test/traceFixtures';
import { buildTraceLineageChain, getReturnToDestination } from './traceLineage';

function withEngine(trace: Trace, runId: string): Trace {
  return {
    ...trace,
    engine: {
      run_id: runId,
      instance_key: `instance-${runId}`,
      definition_name: 'checkout',
      definition_version: 'v1',
      projection_state: 'summary_only',
      status: 'RUNNING',
    },
  };
}

describe('buildTraceLineageChain', () => {
  it('returns an empty chain for traces without an engine run', () => {
    expect(buildTraceLineageChain(null, [])).toEqual([]);
    expect(buildTraceLineageChain(TRACE_DETAIL, [])).toEqual([]);
  });

  it('orders ancestors before the current trace', () => {
    const grandparent = withEngine({ ...TRACE_ONE, id: 'trace-gp' }, 'run-gp');
    const parent = withEngine({ ...TRACE_ONE, id: 'trace-p' }, 'run-p');
    const current = withEngine(
      { ...TRACE_DETAIL, id: 'trace-c' },
      'run-c'
    ) as TraceDetail;

    const chain = buildTraceLineageChain(current, [grandparent, parent]);
    expect(chain.map((trace) => trace.id)).toEqual(['trace-gp', 'trace-p', 'trace-c']);
  });

  it('drops duplicate run ids and traces without engine info', () => {
    const parent = withEngine({ ...TRACE_ONE, id: 'trace-p' }, 'run-p');
    const duplicate = withEngine({ ...TRACE_ONE, id: 'trace-dup' }, 'run-p');
    const engineless: Trace = { ...TRACE_ONE, id: 'trace-none', engine: undefined };
    const current = withEngine(
      { ...TRACE_DETAIL, id: 'trace-c' },
      'run-c'
    ) as TraceDetail;

    const chain = buildTraceLineageChain(current, [parent, duplicate, engineless]);
    expect(chain.map((trace) => trace.id)).toEqual(['trace-p', 'trace-c']);
  });
});

describe('getReturnToDestination', () => {
  it('accepts the known list destinations', () => {
    expect(getReturnToDestination({ returnTo: '/traces' })).toBe('/traces');
    expect(getReturnToDestination({ returnTo: '/traces?status=FAILED' })).toBe(
      '/traces?status=FAILED'
    );
    expect(getReturnToDestination({ returnTo: '/engine/runs' })).toBe('/engine/runs');
    expect(getReturnToDestination({ returnTo: '/engine/runs?page=2' })).toBe(
      '/engine/runs?page=2'
    );
    expect(getReturnToDestination({ returnTo: '/sessions/abc' })).toBe('/sessions/abc');
  });

  it('falls back to /traces for anything else', () => {
    expect(getReturnToDestination(undefined)).toBe('/traces');
    expect(getReturnToDestination(null)).toBe('/traces');
    expect(getReturnToDestination({})).toBe('/traces');
    expect(getReturnToDestination({ returnTo: 42 })).toBe('/traces');
    expect(getReturnToDestination({ returnTo: 'https://evil.example' })).toBe('/traces');
    expect(getReturnToDestination({ returnTo: '/settings' })).toBe('/traces');
  });
});
