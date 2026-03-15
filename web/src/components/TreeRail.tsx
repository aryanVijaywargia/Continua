import {
  useEffect,
  useMemo,
  type Dispatch,
  type SetStateAction,
} from 'react';
import type { Span } from '../api/client';
import { useTreeRailState } from '../hooks/useTreeRailState';
import { useVirtualRows } from '../hooks/useVirtualRows';
import { deriveVisibleRows, type SpanTreeNode } from '../utils/spanTree';
import { SpanTree } from './SpanTree';

interface TreeRailProps {
  expandableSpanIds: ReadonlySet<string>;
  expandedSpanIds: ReadonlySet<string>;
  failedSpanIds: ReadonlySet<string>;
  inlineErrorPreviews: ReadonlyMap<string, string>;
  onSelectSpan: (spanId: string) => void;
  onToggleExpand: (spanId: string) => void;
  primaryAncestorPath: ReadonlySet<string>;
  revealKey: number;
  revealPath: ReadonlySet<string>;
  selectedSpanId: string | null;
  setExpandedSpanIds: Dispatch<SetStateAction<Set<string>>>;
  spanIndex: ReadonlyMap<string, Span>;
  spanTree: SpanTreeNode[];
  spans: Span[];
  onVisibleRowsChange: (rows: ReturnType<typeof deriveVisibleRows>) => void;
}

const TREE_ROW_HEIGHT = 72;
const TREE_ROW_HEIGHT_WITH_METRICS = 94;

export function TreeRail({
  expandableSpanIds,
  expandedSpanIds,
  failedSpanIds,
  inlineErrorPreviews,
  onSelectSpan,
  onToggleExpand,
  primaryAncestorPath,
  revealKey,
  revealPath,
  selectedSpanId,
  setExpandedSpanIds,
  spanIndex,
  spanTree,
  spans,
  onVisibleRowsChange,
}: TreeRailProps) {
  const {
    effectiveExpandedSpanIds,
    handleCollapseAll,
    handleExpandAll,
    matchedSpanIds,
    searchQueryInput,
    setSearchQueryInput,
    setShowMetrics,
    showMetrics,
    visibleRows,
  } = useTreeRailState({
    expandableSpanIds,
    expandedSpanIds,
    inlineErrorPreviews,
    setExpandedSpanIds,
    spanIndex,
    spanTree,
    spans,
  });
  const visibleRowIndexBySpanId = useMemo(
    () =>
      new Map(
        visibleRows.map((row, index) => [row.span.span_id, index] as const)
      ),
    [visibleRows]
  );
  const {
    containerRef,
    onScroll,
    paddingBottom,
    paddingTop,
    scrollToIndex,
    virtualRows,
  } = useVirtualRows({
    estimatedRowHeight: showMetrics ? TREE_ROW_HEIGHT_WITH_METRICS : TREE_ROW_HEIGHT,
    rows: visibleRows,
  });

  useEffect(() => {
    onVisibleRowsChange(visibleRows);
  }, [onVisibleRowsChange, visibleRows]);

  useEffect(() => {
    if (!selectedSpanId || !revealPath.has(selectedSpanId)) {
      return;
    }

    const selectedIndex = visibleRowIndexBySpanId.get(selectedSpanId);
    if (selectedIndex === undefined) {
      return;
    }

    scrollToIndex(selectedIndex);
  }, [
    revealKey,
    revealPath,
    scrollToIndex,
    selectedSpanId,
    visibleRowIndexBySpanId,
  ]);

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm">
      <div className="border-b border-gray-200 bg-gray-50 px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-600">
              Spans ({spans.length})
            </h2>
            <p className="mt-1 text-sm text-gray-500">
              Search, expand, and inspect the visible span hierarchy.
            </p>
          </div>
        </div>

        <div className="mt-3 space-y-3">
          <label className="block">
            <span className="sr-only">Search spans</span>
            <input
              type="search"
              value={searchQueryInput}
              onChange={(event) => setSearchQueryInput(event.target.value)}
              placeholder="Search name, kind, span ID, status, model, provider, errors"
              className="w-full rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-blue-300 focus:ring-2 focus:ring-blue-100"
            />
          </label>

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              className="rounded-full bg-white px-3 py-1.5 text-sm font-medium text-gray-600 ring-1 ring-gray-200 transition hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-200"
              onClick={handleExpandAll}
            >
              Expand all
            </button>
            <button
              type="button"
              className="rounded-full bg-white px-3 py-1.5 text-sm font-medium text-gray-600 ring-1 ring-gray-200 transition hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-blue-200"
              onClick={handleCollapseAll}
            >
              Collapse all
            </button>
            <button
              type="button"
              className={`rounded-full px-3 py-1.5 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-blue-200 ${
                showMetrics
                  ? 'bg-gray-900 text-white'
                  : 'bg-white text-gray-600 ring-1 ring-gray-200 hover:bg-gray-100'
              }`}
              aria-pressed={showMetrics}
              onClick={() => setShowMetrics((current) => !current)}
            >
              Show metrics
            </button>
            {matchedSpanIds !== null ? (
              <span className="text-xs font-medium uppercase tracking-[0.16em] text-gray-500">
                {matchedSpanIds.size} match{matchedSpanIds.size === 1 ? '' : 'es'}
              </span>
            ) : null}
          </div>
        </div>
      </div>

      <div
        ref={containerRef}
        className="min-h-0 flex-1 overflow-y-auto"
        data-visible-row-count={visibleRows.length}
        onScroll={onScroll}
      >
        <div
          style={{
            paddingBottom: `${paddingBottom}px`,
            paddingTop: `${paddingTop}px`,
          }}
        >
          <SpanTree
            rows={virtualRows.map(({ row }) => row)}
            expandedSpanIds={effectiveExpandedSpanIds}
            selectedSpanId={selectedSpanId}
            onSelectSpan={onSelectSpan}
            onToggleExpand={onToggleExpand}
            failedSpanIds={failedSpanIds}
            primaryAncestorPath={primaryAncestorPath}
            revealPath={revealPath}
            revealKey={revealKey}
            inlineErrorPreviews={inlineErrorPreviews}
            showMetrics={showMetrics}
            matchedSpanIds={matchedSpanIds}
          />
        </div>
      </div>
    </section>
  );
}
