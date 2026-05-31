// Pure span-expansion model for the trace-detail workspace.
//
// This is the one deep module extracted from useWorkspaceState/useTreeRailState:
// it owns every way the tree's open set changes, so the raw setExpandedSpanIds
// setter no longer has to be threaded through TraceWorkspace -> TreeRail.
// See CONTEXT.md ("Span expansion"). The reducer is the test surface; the React
// hook around it is thin wiring.

/**
 * expanded: span ids currently open in the tree rail.
 * known: expandable span ids seen so far. Lets syncExpandable tell a
 *   newly-arrived branch (auto-expand) from one the operator collapsed (preserve).
 */
export interface ExpansionState {
  expanded: ReadonlySet<string>;
  known: ReadonlySet<string>;
}

export type ExpansionEvent =
  | { type: 'toggle'; spanId: string }
  | { type: 'syncExpandable'; expandable: ReadonlySet<string> }
  | { type: 'revealAncestors'; spanIds: readonly string[] }
  | { type: 'expandAll'; expandable: ReadonlySet<string> }
  | { type: 'collapseAll' }
  | { type: 'setExact'; expanded: ReadonlySet<string> };

export function initExpansionState(
  expandable: ReadonlySet<string>
): ExpansionState {
  return { expanded: new Set(expandable), known: new Set(expandable) };
}

export function expansionReducer(
  state: ExpansionState,
  event: ExpansionEvent
): ExpansionState {
  switch (event.type) {
    case 'toggle': {
      const expanded = new Set(state.expanded);
      if (!expanded.delete(event.spanId)) {
        expanded.add(event.spanId);
      }
      return { ...state, expanded };
    }

    case 'syncExpandable': {
      // Preserve operator intent for known spans; auto-expand newly-arrived
      // branches. Drop ids that are no longer expandable.
      const expanded = new Set<string>();
      for (const id of event.expandable) {
        if (state.expanded.has(id) || !state.known.has(id)) {
          expanded.add(id);
        }
      }
      return commit(state, expanded, event.expandable);
    }

    case 'revealAncestors': {
      const expanded = new Set(state.expanded);
      for (const id of event.spanIds) {
        if (state.known.has(id)) {
          expanded.add(id);
        }
      }
      return commit(state, expanded, state.known);
    }

    case 'expandAll':
      return commit(state, event.expandable, event.expandable);

    case 'collapseAll':
      return commit(state, EMPTY, state.known);

    case 'setExact':
      return commit(state, event.expanded, state.known);

    default: {
      const _exhaustive: never = event;
      return _exhaustive;
    }
  }
}

const EMPTY: ReadonlySet<string> = new Set();

// Return the SAME state reference when neither set changed, so useReducer bails
// out of re-rendering. Without this, a syncExpandable effect keyed on a
// fresh-but-equal expandable set spins into an infinite render loop.
function commit(
  state: ExpansionState,
  expanded: ReadonlySet<string>,
  known: ReadonlySet<string>
): ExpansionState {
  const expandedSame = setsEqual(state.expanded, expanded);
  const knownSame = setsEqual(state.known, known);
  if (expandedSame && knownSame) {
    return state;
  }
  return {
    expanded: expandedSame ? state.expanded : expanded,
    known: knownSame ? state.known : known,
  };
}

function setsEqual(a: ReadonlySet<string>, b: ReadonlySet<string>): boolean {
  if (a === b) {
    return true;
  }
  if (a.size !== b.size) {
    return false;
  }
  for (const value of a) {
    if (!b.has(value)) {
      return false;
    }
  }
  return true;
}
