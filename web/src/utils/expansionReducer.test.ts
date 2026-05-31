import { describe, expect, it } from 'vitest';
import {
  expansionReducer,
  initExpansionState,
  type ExpansionState,
} from './expansionReducer';

const state = (expanded: string[], known: string[]): ExpansionState => ({
  expanded: new Set(expanded),
  known: new Set(known),
});
const expanded = (s: ExpansionState) => Array.from(s.expanded).sort();

describe('expansionReducer', () => {
  it('initializes with every expandable span open', () => {
    const next = initExpansionState(new Set(['root', 'child']));
    expect(expanded(next)).toEqual(['child', 'root']);
    expect(Array.from(next.known).sort()).toEqual(['child', 'root']);
  });

  it('toggle opens then closes a span', () => {
    let s = state([], ['root']);
    s = expansionReducer(s, { type: 'toggle', spanId: 'root' });
    expect(expanded(s)).toEqual(['root']);
    s = expansionReducer(s, { type: 'toggle', spanId: 'root' });
    expect(expanded(s)).toEqual([]);
  });

  it('syncExpandable preserves a manual collapse for a known span', () => {
    // operator collapsed 'root'; a later poll re-reports the same expandable set
    const s = expansionReducer(state([], ['root']), {
      type: 'syncExpandable',
      expandable: new Set(['root']),
    });
    expect(expanded(s)).toEqual([]);
  });

  it('syncExpandable auto-expands a newly-arrived branch while preserving choices', () => {
    // 'root' was manually collapsed and is known; 'child' is new this poll
    const s = expansionReducer(state([], ['root']), {
      type: 'syncExpandable',
      expandable: new Set(['root', 'child']),
    });
    expect(expanded(s)).toEqual(['child']); // new branch open, manual collapse kept
    expect(Array.from(s.known).sort()).toEqual(['child', 'root']);
  });

  it('syncExpandable drops spans that are no longer expandable', () => {
    const s = expansionReducer(state(['gone', 'root'], ['gone', 'root']), {
      type: 'syncExpandable',
      expandable: new Set(['root']),
    });
    expect(expanded(s)).toEqual(['root']);
    expect(s.known.has('gone')).toBe(false);
  });

  it('revealAncestors expands only ancestors that are expandable (known)', () => {
    const s = expansionReducer(state([], ['root']), {
      type: 'revealAncestors',
      spanIds: ['root', 'leaf'], // 'leaf' has no children -> not known -> ignored
    });
    expect(expanded(s)).toEqual(['root']);
  });

  it('expandAll opens every expandable span and refreshes known', () => {
    const s = expansionReducer(state([], ['root']), {
      type: 'expandAll',
      expandable: new Set(['root', 'child']),
    });
    expect(expanded(s)).toEqual(['child', 'root']);
    expect(Array.from(s.known).sort()).toEqual(['child', 'root']);
  });

  it('collapseAll closes everything', () => {
    const s = expansionReducer(state(['root', 'child'], ['root', 'child']), {
      type: 'collapseAll',
    });
    expect(expanded(s)).toEqual([]);
  });

  it('setExact replaces the open set (used for search save/restore)', () => {
    const s = expansionReducer(state(['root'], ['root', 'child']), {
      type: 'setExact',
      expanded: new Set(['child']),
    });
    expect(expanded(s)).toEqual(['child']);
  });

  it('keeps input immutable when toggling', () => {
    const before = state(['root'], ['root']);
    const after = expansionReducer(before, { type: 'toggle', spanId: 'root' });
    expect(before.expanded.has('root')).toBe(true); // input untouched
    expect(after).not.toBe(before);
  });

  // Referential stability: a no-op dispatch must return the SAME state object so
  // useReducer bails out and a syncExpandable effect cannot spin into a loop when
  // a caller passes a fresh-but-equal expandable set each render.
  it('syncExpandable returns the same reference when content is unchanged', () => {
    const before = initExpansionState(new Set(['root', 'child']));
    const after = expansionReducer(before, {
      type: 'syncExpandable',
      expandable: new Set(['root', 'child']), // new Set, identical content
    });
    expect(after).toBe(before);
  });

  it('syncExpandable returns a new reference when content changes', () => {
    const before = initExpansionState(new Set(['root']));
    const after = expansionReducer(before, {
      type: 'syncExpandable',
      expandable: new Set(['root', 'child']),
    });
    expect(after).not.toBe(before);
  });

  it('revealAncestors returns the same reference when nothing new expands', () => {
    const before = initExpansionState(new Set(['root'])); // root already open
    const after = expansionReducer(before, {
      type: 'revealAncestors',
      spanIds: ['root'],
    });
    expect(after).toBe(before);
  });

  it('collapseAll returns the same reference when already fully collapsed', () => {
    const before = state([], ['root', 'child']);
    const after = expansionReducer(before, { type: 'collapseAll' });
    expect(after).toBe(before);
  });
});
