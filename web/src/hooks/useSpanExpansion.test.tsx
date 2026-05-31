import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import { createSpan, resetTestEntityCounter } from '../test/traceFixtures';
import { buildSpanTree, collectExpandableSpanIds } from '../utils/spanTree';
import { useSpanExpansion } from './useSpanExpansion';

beforeEach(() => {
  resetTestEntityCounter();
});

function expandableOf(spans: ReturnType<typeof createSpan>[]) {
  return collectExpandableSpanIds(buildSpanTree(spans));
}

describe('useSpanExpansion', () => {
  it('starts with every expandable span open', () => {
    const spans = [
      createSpan({ span_id: 'root' }),
      createSpan({ span_id: 'child', parent_span_id: 'root' }),
      createSpan({ span_id: 'leaf', parent_span_id: 'child' }),
    ];
    const { result } = renderHook(() =>
      useSpanExpansion(expandableOf(spans))
    );
    expect(Array.from(result.current.expandedSpanIds).sort()).toEqual([
      'child',
      'root',
    ]);
  });

  it('toggle collapses and reopens a span', () => {
    const spans = [
      createSpan({ span_id: 'root' }),
      createSpan({ span_id: 'child', parent_span_id: 'root' }),
    ];
    const { result } = renderHook(() => useSpanExpansion(expandableOf(spans)));

    act(() => result.current.toggleExpandedSpan('root'));
    expect(result.current.expandedSpanIds.has('root')).toBe(false);

    act(() => result.current.toggleExpandedSpan('root'));
    expect(result.current.expandedSpanIds.has('root')).toBe(true);
  });

  it('auto-expands a newly-arrived branch while preserving a manual collapse', () => {
    const initial = [
      createSpan({ span_id: 'root' }),
      createSpan({ span_id: 'child', parent_span_id: 'root' }),
    ];
    const next = [
      ...initial,
      createSpan({ span_id: 'leaf', parent_span_id: 'child' }),
    ];
    const { result, rerender } = renderHook(
      ({ expandable }) => useSpanExpansion(expandable),
      { initialProps: { expandable: expandableOf(initial) } }
    );

    act(() => result.current.toggleExpandedSpan('root')); // manual collapse

    rerender({ expandable: expandableOf(next) }); // poll brings 'child' as a parent

    // root stays collapsed (operator intent), child auto-expands (new branch)
    expect(Array.from(result.current.expandedSpanIds).sort()).toEqual(['child']);
  });

  it('revealAncestors expands the ancestor path of a span', () => {
    const spans = [
      createSpan({ span_id: 'root' }),
      createSpan({ span_id: 'child', parent_span_id: 'root' }),
      createSpan({ span_id: 'leaf', parent_span_id: 'child' }),
    ];
    const { result } = renderHook(() => useSpanExpansion(expandableOf(spans)));

    act(() => result.current.toggleExpandedSpan('root'));
    act(() => result.current.toggleExpandedSpan('child'));
    expect(result.current.expandedSpanIds.size).toBe(0);

    act(() => result.current.revealAncestors(['root', 'child']));
    expect(Array.from(result.current.expandedSpanIds).sort()).toEqual([
      'child',
      'root',
    ]);
  });

  it('exposes expandAll, collapseAll, and setExact for the tree rail', () => {
    // root and child are both expandable (each has a descendant); leaf is not.
    const spans = [
      createSpan({ span_id: 'root' }),
      createSpan({ span_id: 'child', parent_span_id: 'root' }),
      createSpan({ span_id: 'leaf', parent_span_id: 'child' }),
    ];
    const { result } = renderHook(() => useSpanExpansion(expandableOf(spans)));

    act(() => result.current.collapseAll());
    expect(result.current.expandedSpanIds.size).toBe(0);

    act(() => result.current.expandAll());
    expect(Array.from(result.current.expandedSpanIds).sort()).toEqual([
      'child',
      'root',
    ]);

    act(() => result.current.setExact(new Set(['root'])));
    expect(Array.from(result.current.expandedSpanIds)).toEqual(['root']);
  });
});
