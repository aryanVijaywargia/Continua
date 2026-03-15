import { beforeEach, describe, expect, it } from 'vitest';
import {
  createSpan,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import { buildSpanTree } from './spanTree';
import { estimateExpandAllRevealCount } from './treeRail';

beforeEach(() => {
  resetTestEntityCounter();
});

describe('treeRail utilities', () => {
  it('estimates only the rows that would be newly revealed by expand all', () => {
    const spans = [
      createSpan({ span_id: 'root' }),
      createSpan({ span_id: 'child-a', parent_span_id: 'root' }),
      createSpan({ span_id: 'child-b', parent_span_id: 'root' }),
      createSpan({ span_id: 'leaf', parent_span_id: 'child-b' }),
    ];

    const tree = buildSpanTree(spans);

    expect(estimateExpandAllRevealCount(tree, new Set())).toBe(3);
    expect(estimateExpandAllRevealCount(tree, new Set(['root']))).toBe(1);
    expect(estimateExpandAllRevealCount(tree, new Set(['root', 'child-b']))).toBe(0);
  });
});
