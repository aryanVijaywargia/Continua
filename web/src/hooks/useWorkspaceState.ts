import { useCallback, useEffect, useRef, useState } from 'react';
import type { Span, TimelineTraceStatus } from '../api/client';
import { getAncestorIds } from '../utils/spanTree';

interface UseWorkspaceStateOptions {
  isSpanDataReady: boolean;
  spanIndex: ReadonlyMap<string, Span>;
  spanParam: string | null;
  setSpanParam: (spanId: string | null) => void;
  timelineStatus: TimelineTraceStatus | null;
  primaryFailedSpanId: string | null;
  expandableSpanIds: ReadonlySet<string>;
}

export function useWorkspaceState({
  isSpanDataReady,
  spanIndex,
  spanParam,
  setSpanParam,
  timelineStatus,
  primaryFailedSpanId,
  expandableSpanIds,
}: UseWorkspaceStateOptions) {
  const [selectedSpanExternalId, setSelectedSpanExternalId] = useState<string | null>(
    null
  );
  const [expandedSpanIds, setExpandedSpanIds] = useState(
    () => new Set(expandableSpanIds)
  );
  const [revealPath, setRevealPath] = useState<Set<string>>(new Set());
  const [revealVersion, setRevealVersion] = useState(0);
  const [waterfallRevealTarget, setWaterfallRevealTarget] = useState<string | null>(
    null
  );
  const [userHasSelected, setUserHasSelected] = useState(false);
  const knownExpandableSpanIdsRef = useRef(new Set(expandableSpanIds));
  const pendingUrlSyncSpanIdRef = useRef<string | null>(null);

  const updateSelectedSpan = useCallback(
    (spanId: string | null) => {
      setSelectedSpanExternalId(spanId);
      setWaterfallRevealTarget(spanId);

      if (spanId) {
        const nextRevealPath = new Set([
          ...getAncestorIds(spanId, spanIndex),
          spanId,
        ]);
        setRevealPath(nextRevealPath);
        setExpandedSpanIds((currentExpandedSpanIds) => {
          const nextExpandedSpanIds = new Set(currentExpandedSpanIds);

          for (const revealSpanId of nextRevealPath) {
            if (expandableSpanIds.has(revealSpanId)) {
              nextExpandedSpanIds.add(revealSpanId);
            }
          }

          return nextExpandedSpanIds;
        });
        setRevealVersion((currentVersion) => currentVersion + 1);
        return;
      }

      setRevealPath(new Set());
    },
    [expandableSpanIds, spanIndex]
  );

  const selectSpan = useCallback(
    (spanId: string) => {
      pendingUrlSyncSpanIdRef.current = spanId;
      updateSelectedSpan(spanId);
      setUserHasSelected(true);
      setSpanParam(spanId);
    },
    [setSpanParam, updateSelectedSpan]
  );

  const toggleExpandedSpan = useCallback((spanId: string) => {
    setExpandedSpanIds((currentExpandedSpanIds) => {
      const nextExpandedSpanIds = new Set(currentExpandedSpanIds);
      if (nextExpandedSpanIds.has(spanId)) {
        nextExpandedSpanIds.delete(spanId);
      } else {
        nextExpandedSpanIds.add(spanId);
      }
      return nextExpandedSpanIds;
    });
  }, []);

  useEffect(() => {
    setExpandedSpanIds((currentExpandedSpanIds) => {
      const knownExpandableSpanIds = knownExpandableSpanIdsRef.current;
      const nextExpandedSpanIds = new Set<string>();

      for (const spanId of expandableSpanIds) {
        if (
          currentExpandedSpanIds.has(spanId) ||
          !knownExpandableSpanIds.has(spanId)
        ) {
          nextExpandedSpanIds.add(spanId);
        }
      }

      knownExpandableSpanIdsRef.current = new Set(expandableSpanIds);

      if (setsAreEqual(currentExpandedSpanIds, nextExpandedSpanIds)) {
        return currentExpandedSpanIds;
      }

      return nextExpandedSpanIds;
    });
  }, [expandableSpanIds]);

  useEffect(() => {
    if (!isSpanDataReady) {
      return;
    }

    if (spanParam !== null) {
      if (pendingUrlSyncSpanIdRef.current === spanParam) {
        pendingUrlSyncSpanIdRef.current = null;
      }

      if (spanIndex.has(spanParam)) {
        if (selectedSpanExternalId !== spanParam) {
          updateSelectedSpan(spanParam);
        }
        if (!userHasSelected) {
          setUserHasSelected(true);
        }
        return;
      }

      pendingUrlSyncSpanIdRef.current = null;
      updateSelectedSpan(null);
      setUserHasSelected(false);
      setSpanParam(null);
      return;
    }

    if (!userHasSelected) {
      return;
    }

    if (
      pendingUrlSyncSpanIdRef.current !== null &&
      pendingUrlSyncSpanIdRef.current === selectedSpanExternalId
    ) {
      return;
    }

    updateSelectedSpan(null);
    setUserHasSelected(false);
  }, [
    selectedSpanExternalId,
    isSpanDataReady,
    setSpanParam,
    spanIndex,
    spanParam,
    updateSelectedSpan,
    userHasSelected,
  ]);

  useEffect(() => {
    if (!selectedSpanExternalId || spanIndex.has(selectedSpanExternalId)) {
      return;
    }

    pendingUrlSyncSpanIdRef.current = null;
    updateSelectedSpan(null);
    setUserHasSelected(false);

    if (spanParam !== null) {
      setSpanParam(null);
    }
  }, [
    selectedSpanExternalId,
    setSpanParam,
    spanIndex,
    spanParam,
    updateSelectedSpan,
  ]);

  useEffect(() => {
    if (timelineStatus !== 'FAILED' || userHasSelected || spanParam !== null) {
      return;
    }

    const nextSelectedSpanId = primaryFailedSpanId ?? null;
    if (selectedSpanExternalId === nextSelectedSpanId) {
      return;
    }

    updateSelectedSpan(nextSelectedSpanId);
  }, [
    primaryFailedSpanId,
    selectedSpanExternalId,
    spanParam,
    timelineStatus,
    updateSelectedSpan,
    userHasSelected,
  ]);

  return {
    expandedSpanIds,
    revealPath,
    revealVersion,
    selectSpan,
    selectedSpanExternalId,
    setExpandedSpanIds,
    toggleExpandedSpan,
    updateSelectedSpan,
    userHasSelected,
    waterfallRevealTarget,
  };
}

function setsAreEqual<T>(left: ReadonlySet<T>, right: ReadonlySet<T>) {
  if (left.size !== right.size) {
    return false;
  }

  for (const value of left) {
    if (!right.has(value)) {
      return false;
    }
  }

  return true;
}
