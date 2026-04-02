import {
  useEffect,
  useMemo,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react';
import type { Span } from '../api/client';
import { useTreeRailState } from '../hooks/useTreeRailState';
import type { RetrySafetyAssessment } from '../utils/retrySafety';
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
  spanAssessments?: ReadonlyMap<string, RetrySafetyAssessment>;
}

const TREE_ROW_HEIGHT = 72;
const TREE_ROW_HEIGHT_WITH_METRICS = 94;
const EMPTY_SPAN_ASSESSMENTS = new Map<string, RetrySafetyAssessment>();

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
  spanAssessments = EMPTY_SPAN_ASSESSMENTS,
}: TreeRailProps) {
  const [failedOnly, setFailedOnly] = useState(false);
  const [kindFilter, setKindFilter] = useState<'ALL' | Span['kind']>('ALL');
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
  const availableKinds = useMemo(
    () => Array.from(new Set(spans.map((span) => span.kind))),
    [spans]
  );
  const filteredRows = useMemo(
    () =>
      visibleRows.filter((row) => {
        if (failedOnly && !failedSpanIds.has(row.span.span_id)) {
          return false;
        }

        if (kindFilter !== 'ALL' && row.span.kind !== kindFilter) {
          return false;
        }

        return true;
      }),
    [failedOnly, failedSpanIds, kindFilter, visibleRows]
  );
  const visibleRowIndexBySpanId = useMemo(
    () =>
      new Map(
        filteredRows.map((row, index) => [row.span.span_id, index] as const)
      ),
    [filteredRows]
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
    rows: filteredRows,
  });

  useEffect(() => {
    onVisibleRowsChange(filteredRows);
  }, [filteredRows, onVisibleRowsChange]);

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
    <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-[1.5rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface)] shadow-[var(--continua-shadow-soft)]">
      <div className="border-b border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-[var(--continua-text-secondary)]">
              Spans ({spans.length})
            </h2>
            <p className="mt-1 text-sm text-[var(--continua-text-muted)]">
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
              className="app-input"
            />
          </label>

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              className="app-button-ghost"
              onClick={handleExpandAll}
            >
              Expand all
            </button>
            <button
              type="button"
              className="app-button-ghost"
              onClick={handleCollapseAll}
            >
              Collapse all
            </button>
            <button
              type="button"
              className={`rounded-full px-3 py-1.5 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)] ${
                showMetrics
                  ? 'border border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] text-[var(--continua-accent)]'
                  : 'border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)]'
              }`}
              aria-pressed={showMetrics}
              onClick={() => setShowMetrics((current) => !current)}
            >
              Show metrics
            </button>
            {matchedSpanIds !== null ? (
              <span className="text-xs font-medium uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
                {matchedSpanIds.size} match{matchedSpanIds.size === 1 ? '' : 'es'}
              </span>
            ) : null}
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              aria-pressed={failedOnly}
              onClick={() => setFailedOnly((current) => !current)}
              className={`rounded-full px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.14em] transition focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)] ${
                failedOnly
                  ? 'border border-red-400/50 bg-red-100/80 text-red-900 dark:border-red-400/25 dark:bg-red-400/10 dark:text-red-100'
                  : 'border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)]'
              }`}
            >
              Failed only
            </button>
            <button
              type="button"
              aria-pressed={kindFilter === 'ALL'}
              onClick={() => setKindFilter('ALL')}
              className={`rounded-full px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.14em] transition ${
                kindFilter === 'ALL'
                  ? 'border border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] text-[var(--continua-accent)]'
                  : 'border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)]'
              }`}
            >
              All kinds
            </button>
            {availableKinds.map((kind) => (
              <button
                key={kind}
                type="button"
                aria-pressed={kindFilter === kind}
                onClick={() => setKindFilter((current) => (current === kind ? 'ALL' : kind))}
                className={`rounded-full px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.14em] transition ${
                  kindFilter === kind
                    ? 'border border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] text-[var(--continua-accent)]'
                    : 'border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)]'
                }`}
              >
                {kind}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div
        ref={containerRef}
        className="min-h-0 flex-1 overflow-y-auto"
        data-visible-row-count={filteredRows.length}
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
            spanAssessments={spanAssessments}
          />
        </div>
      </div>
    </section>
  );
}
