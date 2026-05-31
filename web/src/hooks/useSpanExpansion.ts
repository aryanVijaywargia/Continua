import { useCallback, useEffect, useReducer } from 'react';
import {
  expansionReducer,
  initExpansionState,
} from '../utils/expansionReducer';

/**
 * Thin React binding around the pure {@link expansionReducer}. It owns the tree
 * rail's open set so the raw `setExpandedSpanIds` setter no longer has to be
 * threaded through the workspace. The reducer is the test surface; this hook is
 * wiring. See CONTEXT.md ("Span expansion").
 *
 * Dispatches `syncExpandable` whenever the set of expandable spans changes
 * (e.g. live polling brings new branches), preserving manual collapse while
 * auto-expanding newly-arrived branches.
 */
export function useSpanExpansion(expandableSpanIds: ReadonlySet<string>) {
  const [state, dispatch] = useReducer(
    expansionReducer,
    expandableSpanIds,
    initExpansionState
  );

  useEffect(() => {
    dispatch({ type: 'syncExpandable', expandable: expandableSpanIds });
  }, [expandableSpanIds]);

  const toggleExpandedSpan = useCallback((spanId: string) => {
    dispatch({ type: 'toggle', spanId });
  }, []);

  const revealAncestors = useCallback((spanIds: readonly string[]) => {
    dispatch({ type: 'revealAncestors', spanIds });
  }, []);

  const expandAll = useCallback(() => {
    dispatch({ type: 'expandAll', expandable: expandableSpanIds });
  }, [expandableSpanIds]);

  const collapseAll = useCallback(() => {
    dispatch({ type: 'collapseAll' });
  }, []);

  const setExact = useCallback((expanded: ReadonlySet<string>) => {
    dispatch({ type: 'setExact', expanded });
  }, []);

  return {
    expandedSpanIds: state.expanded,
    toggleExpandedSpan,
    revealAncestors,
    expandAll,
    collapseAll,
    setExact,
  };
}
