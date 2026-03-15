import { useState } from 'react';
import { renderHook, act } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { buildSpanIndex } from '../utils/failureAnalysis';
import { buildSpanTree, collectExpandableSpanIds } from '../utils/spanTree';
import {
  createSpan,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import { useWorkspaceState } from './useWorkspaceState';

type WorkspaceStateOptions = Parameters<typeof useWorkspaceState>[0];

beforeEach(() => {
  resetTestEntityCounter();
});

describe('useWorkspaceState', () => {
  it('initializes expanded span ids from every span that has children', () => {
    const spans = [
      createSpan({ span_id: 'root' }),
      createSpan({ span_id: 'child', parent_span_id: 'root' }),
      createSpan({ span_id: 'leaf', parent_span_id: 'child' }),
    ];

    const { result } = renderWorkspaceState(spans);

    expect(Array.from(result.current.expandedSpanIds).sort()).toEqual(['child', 'root']);
  });

  it('adds newly-expandable spans after rerender while preserving collapsed state', () => {
    const initialSpans = [
      createSpan({ span_id: 'root' }),
      createSpan({ span_id: 'child', parent_span_id: 'root' }),
    ];
    const nextSpans = [
      ...initialSpans,
      createSpan({ span_id: 'leaf', parent_span_id: 'child' }),
    ];

    const { result, rerender } = renderWorkspaceState(initialSpans);

    act(() => {
      result.current.toggleExpandedSpan('root');
    });

    rerender(buildOptions(nextSpans));

    expect(Array.from(result.current.expandedSpanIds).sort()).toEqual(['child']);
  });

  it('increments the reveal version and re-expands ancestors when the same span is reselected', () => {
    const spans = [
      createSpan({ span_id: 'root' }),
      createSpan({ span_id: 'child', parent_span_id: 'root' }),
    ];
    const setSpanParamSpy = vi.fn();
    const { result } = renderHook(() => {
      const [spanParam, setSpanParam] = useState<string | null>(null);

      return useWorkspaceState(
        buildOptions(spans, {
          spanParam,
          setSpanParam: (nextSpanParam) => {
            setSpanParam(nextSpanParam);
            setSpanParamSpy(nextSpanParam);
          },
        })
      );
    });

    act(() => {
      result.current.selectSpan('child');
    });
    const firstRevealVersion = result.current.revealVersion;

    act(() => {
      result.current.toggleExpandedSpan('root');
    });
    expect(result.current.expandedSpanIds.has('root')).toBe(false);

    act(() => {
      result.current.selectSpan('child');
    });

    expect(result.current.revealVersion).toBeGreaterThan(firstRevealVersion);
    expect(result.current.expandedSpanIds.has('root')).toBe(true);
    expect(result.current.userHasSelected).toBe(true);
    expect(setSpanParamSpy).toHaveBeenLastCalledWith('child');
  });

  it('preserves a manual selection while the URL param update is still pending', () => {
    const spans = [
      createSpan({ span_id: 'root' }),
      createSpan({ span_id: 'child', parent_span_id: 'root' }),
    ];
    const { result } = renderWorkspaceState(spans);

    act(() => {
      result.current.selectSpan('child');
    });

    expect(result.current.selectedSpanExternalId).toBe('child');
    expect(result.current.userHasSelected).toBe(true);
  });
});

function renderWorkspaceState(spans: Array<ReturnType<typeof createSpan>>) {
  return renderHook((options: WorkspaceStateOptions) => useWorkspaceState(options), {
    initialProps: buildOptions(spans),
  });
}

function buildOptions(
  spans: Array<ReturnType<typeof createSpan>>,
  overrides: Partial<WorkspaceStateOptions> = {}
): WorkspaceStateOptions {
  const spanIndex = buildSpanIndex(spans);
  const spanTree = buildSpanTree(spans);

  return {
    isSpanDataReady: true,
    spanIndex,
    spanParam: null,
    setSpanParam: vi.fn(),
    timelineStatus: 'COMPLETED' as const,
    primaryFailedSpanId: null,
    expandableSpanIds: collectExpandableSpanIds(spanTree),
    ...overrides,
  };
}
