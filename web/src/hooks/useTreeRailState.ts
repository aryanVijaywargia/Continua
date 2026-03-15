import {
  useCallback,
  useDeferredValue,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react';
import type { Span } from '../api/client';
import {
  deriveVisibleRows,
  getAncestorIds,
  type SpanTreeNode,
} from '../utils/spanTree';
import {
  estimateExpandAllRevealCount,
  EXPAND_ALL_REVEAL_COUNT_THRESHOLD,
} from '../utils/treeRail';

interface UseTreeRailStateOptions {
  expandableSpanIds: ReadonlySet<string>;
  expandedSpanIds: ReadonlySet<string>;
  inlineErrorPreviews: ReadonlyMap<string, string>;
  setExpandedSpanIds: Dispatch<SetStateAction<Set<string>>>;
  spanIndex: ReadonlyMap<string, Span>;
  spanTree: SpanTreeNode[];
  spans: Span[];
}

export function useTreeRailState({
  expandableSpanIds,
  expandedSpanIds,
  inlineErrorPreviews,
  setExpandedSpanIds,
  spanIndex,
  spanTree,
  spans,
}: UseTreeRailStateOptions) {
  const [searchQueryInput, setSearchQueryInput] = useState('');
  const deferredSearchQuery = useDeferredValue(searchQueryInput.trim());
  const [showMetrics, setShowMetrics] = useState(false);
  const savedExpandedSpanIdsBeforeSearchRef = useRef<Set<string> | null>(null);

  const normalizedSearchQuery = deferredSearchQuery.trim().toLowerCase();
  const matchedSpanIds = useMemo(() => {
    if (!normalizedSearchQuery) {
      return null;
    }

    const queryTokens = normalizedSearchQuery.split(/\s+/).filter(Boolean);
    const nextMatchedSpanIds = new Set<string>();

    for (const span of spans) {
      const searchableText = [
        span.name,
        span.kind,
        span.span_id,
        span.status,
        span.model,
        span.provider,
        inlineErrorPreviews.get(span.span_id),
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase();

      if (queryTokens.every((token) => searchableText.includes(token))) {
        nextMatchedSpanIds.add(span.span_id);
      }
    }

    return nextMatchedSpanIds;
  }, [inlineErrorPreviews, normalizedSearchQuery, spans]);

  const searchExpandedAncestorIds = useMemo(() => {
    if (matchedSpanIds === null || matchedSpanIds.size === 0) {
      return new Set<string>();
    }

    const nextExpandedAncestorIds = new Set<string>();

    for (const spanId of matchedSpanIds) {
      for (const ancestorId of getAncestorIds(spanId, spanIndex)) {
        if (expandableSpanIds.has(ancestorId)) {
          nextExpandedAncestorIds.add(ancestorId);
        }
      }
    }

    return nextExpandedAncestorIds;
  }, [expandableSpanIds, matchedSpanIds, spanIndex]);

  const effectiveExpandedSpanIds = useMemo(() => {
    if (!normalizedSearchQuery) {
      return expandedSpanIds;
    }

    const nextExpandedSpanIds = new Set(expandedSpanIds);
    for (const ancestorId of searchExpandedAncestorIds) {
      nextExpandedSpanIds.add(ancestorId);
    }
    return nextExpandedSpanIds;
  }, [expandedSpanIds, normalizedSearchQuery, searchExpandedAncestorIds]);

  const visibleRows = useMemo(
    () => deriveVisibleRows(spanTree, effectiveExpandedSpanIds),
    [effectiveExpandedSpanIds, spanTree]
  );

  useEffect(() => {
    if (normalizedSearchQuery) {
      if (savedExpandedSpanIdsBeforeSearchRef.current === null) {
        savedExpandedSpanIdsBeforeSearchRef.current = new Set(expandedSpanIds);
      }
      return;
    }

    if (savedExpandedSpanIdsBeforeSearchRef.current === null) {
      return;
    }

    setExpandedSpanIds(new Set(savedExpandedSpanIdsBeforeSearchRef.current));
    savedExpandedSpanIdsBeforeSearchRef.current = null;
  }, [expandedSpanIds, normalizedSearchQuery, setExpandedSpanIds]);

  const handleExpandAll = useCallback(() => {
    const revealCount = estimateExpandAllRevealCount(spanTree, effectiveExpandedSpanIds);
    const projectedVisibleRowCount = visibleRows.length + revealCount;

    if (
      revealCount > EXPAND_ALL_REVEAL_COUNT_THRESHOLD &&
      !window.confirm(
        `Expanding to ${projectedVisibleRowCount} visible rows may affect performance. Continue?`
      )
    ) {
      return;
    }

    setExpandedSpanIds(new Set(expandableSpanIds));
  }, [
    effectiveExpandedSpanIds,
    expandableSpanIds,
    setExpandedSpanIds,
    spanTree,
    visibleRows.length,
  ]);

  const handleCollapseAll = useCallback(() => {
    setExpandedSpanIds(new Set());
  }, [setExpandedSpanIds]);

  return {
    effectiveExpandedSpanIds,
    handleCollapseAll,
    handleExpandAll,
    matchedSpanIds,
    searchQueryInput,
    setSearchQueryInput,
    setShowMetrics,
    showMetrics,
    visibleRows,
  };
}
