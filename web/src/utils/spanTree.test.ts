import { beforeEach, describe, expect, it } from 'vitest';
import {
  createSpan,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import { buildSpanIndex } from './failureAnalysis';
import {
  buildSpanTree,
  collectExpandableSpanIds,
  deriveVisibleRows,
  flattenPreorder,
  getAncestorIds,
} from './spanTree';

beforeEach(() => {
  resetTestEntityCounter();
});

describe('spanTree utilities', () => {
  it('builds a sorted tree and flattens it in preorder', () => {
    const spans = [
      createSpan({
        span_id: 'child-b',
        parent_span_id: 'root',
        name: 'Child B',
        started_at: '2026-03-14T10:00:02.000Z',
      }),
      createSpan({
        span_id: 'root',
        name: 'Root',
        started_at: '2026-03-14T10:00:00.000Z',
      }),
      createSpan({
        span_id: 'child-a',
        parent_span_id: 'root',
        name: 'Child A',
        started_at: '2026-03-14T10:00:01.000Z',
      }),
    ];

    const tree = buildSpanTree(spans);
    const rows = flattenPreorder(tree);

    expect(rows.map((row) => row.span.span_id)).toEqual(['root', 'child-a', 'child-b']);
    expect(rows.map((row) => row.depth)).toEqual([0, 1, 1]);
    expect(collectExpandableSpanIds(tree)).toEqual(new Set(['root']));
  });

  it('promotes orphaned spans to roots and breaks parent cycles deterministically', () => {
    const spans = [
      createSpan({
        span_id: 'cycle-a',
        parent_span_id: 'cycle-b',
        started_at: '2026-03-14T10:00:00.000Z',
      }),
      createSpan({
        span_id: 'cycle-b',
        parent_span_id: 'cycle-a',
        started_at: '2026-03-14T10:00:01.000Z',
      }),
      createSpan({
        span_id: 'orphan',
        parent_span_id: 'missing',
        started_at: '2026-03-14T10:00:02.000Z',
      }),
    ];

    const rows = flattenPreorder(buildSpanTree(spans));

    expect(rows.map((row) => row.span.span_id)).toEqual(['cycle-a', 'cycle-b', 'orphan']);
    expect(rows.map((row) => row.depth)).toEqual([0, 0, 0]);
  });

  it('derives visible rows from the shared expansion set and computes ancestor ids', () => {
    const spans = [
      createSpan({ span_id: 'root', name: 'Root' }),
      createSpan({
        span_id: 'child',
        name: 'Child',
        parent_span_id: 'root',
      }),
      createSpan({
        span_id: 'leaf',
        name: 'Leaf',
        parent_span_id: 'child',
      }),
    ];

    const tree = buildSpanTree(spans);
    const visibleRows = deriveVisibleRows(tree, new Set(['root']));

    expect(visibleRows.map((row) => row.span.span_id)).toEqual(['root', 'child']);
    expect(getAncestorIds('leaf', buildSpanIndex(spans))).toEqual(['root', 'child']);
  });
});
