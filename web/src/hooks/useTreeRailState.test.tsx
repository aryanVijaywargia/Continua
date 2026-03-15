import { useState } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { buildSpanIndex } from '../utils/failureAnalysis';
import { buildSpanTree, collectExpandableSpanIds } from '../utils/spanTree';
import {
  createSpan,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import { useTreeRailState } from './useTreeRailState';

beforeEach(() => {
  resetTestEntityCounter();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('useTreeRailState', () => {
  it('restores the prior expansion state after clearing search', async () => {
    const spans = [
      createSpan({ span_id: 'root' }),
      createSpan({ span_id: 'child', parent_span_id: 'root' }),
      createSpan({ span_id: 'leaf', parent_span_id: 'child', name: 'Needle leaf' }),
    ];
    const spanTree = buildSpanTree(spans);
    const expandableSpanIds = collectExpandableSpanIds(spanTree);
    const spanIndex = buildSpanIndex(spans);

    const { result } = renderHook(() => {
      const [expandedSpanIds, setExpandedSpanIds] = useState(new Set<string>());

      return useTreeRailState({
        expandableSpanIds,
        expandedSpanIds,
        inlineErrorPreviews: new Map(),
        setExpandedSpanIds,
        spanIndex,
        spanTree,
        spans,
      });
    });

    expect(result.current.visibleRows.map((row) => row.span.span_id)).toEqual(['root']);

    act(() => {
      result.current.setSearchQueryInput('needle');
    });

    await waitFor(() => {
      expect(result.current.visibleRows.map((row) => row.span.span_id)).toEqual([
        'root',
        'child',
        'leaf',
      ]);
    });

    act(() => {
      result.current.setSearchQueryInput('');
    });

    await waitFor(() => {
      expect(result.current.visibleRows.map((row) => row.span.span_id)).toEqual(['root']);
    });
  });

  it('confirms before expand all when the projected reveal cost exceeds the threshold', () => {
    const rootSpan = createSpan({ span_id: 'root' });
    const spans = [
      rootSpan,
      ...Array.from({ length: 705 }, (_, index) =>
        createSpan({
          span_id: `child-${index}`,
          parent_span_id: 'root',
        })
      ),
    ];
    const spanTree = buildSpanTree(spans);
    const expandableSpanIds = collectExpandableSpanIds(spanTree);
    const spanIndex = buildSpanIndex(spans);
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);

    const { result } = renderHook(() => {
      const [expandedSpanIds, setExpandedSpanIds] = useState(new Set<string>());

      return useTreeRailState({
        expandableSpanIds,
        expandedSpanIds,
        inlineErrorPreviews: new Map(),
        setExpandedSpanIds,
        spanIndex,
        spanTree,
        spans,
      });
    });

    act(() => {
      result.current.handleExpandAll();
    });

    expect(confirmSpy).toHaveBeenCalledOnce();
    expect(result.current.visibleRows.map((row) => row.span.span_id)).toEqual(['root']);
  });
});
