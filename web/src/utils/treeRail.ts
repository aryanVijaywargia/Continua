import type { SpanTreeNode } from './spanTree';

export const EXPAND_ALL_REVEAL_COUNT_THRESHOLD = 700;

export function estimateExpandAllRevealCount(
  nodes: SpanTreeNode[],
  expandedSpanIds: ReadonlySet<string>
): number {
  let revealCount = 0;

  const visit = (treeNodes: SpanTreeNode[]) => {
    for (const node of treeNodes) {
      if (node.children.length === 0) {
        continue;
      }

      if (expandedSpanIds.has(node.span.span_id)) {
        visit(node.children);
        continue;
      }

      revealCount += countTreeRows(node.children);
    }
  };

  visit(nodes);
  return revealCount;
}

function countTreeRows(nodes: SpanTreeNode[]): number {
  let rowCount = 0;

  const visit = (treeNodes: SpanTreeNode[]) => {
    for (const node of treeNodes) {
      rowCount += 1;
      visit(node.children);
    }
  };

  visit(nodes);
  return rowCount;
}
