import type { Span } from '../api/client';

export interface SpanTreeNode {
  span: Span;
  children: SpanTreeNode[];
}

export interface SpanTreeRow {
  span: Span;
  depth: number;
  hasChildren: boolean;
}

export function buildSpanTree(spans: Span[]): SpanTreeNode[] {
  const spanMap = new Map<string, SpanTreeNode>();
  const spanIndex = new Map<string, Span>();
  const inputOrder = new Map<string, number>();
  const roots: SpanTreeNode[] = [];
  const rootIds = new Set<string>();

  for (const [index, span] of spans.entries()) {
    spanMap.set(span.span_id, { span, children: [] });
    spanIndex.set(span.span_id, span);
    inputOrder.set(span.span_id, index);
  }

  for (const span of spans) {
    const node = spanMap.get(span.span_id);
    if (!node) {
      continue;
    }

    if (span.parent_span_id) {
      const parent = spanMap.get(span.parent_span_id);
      if (
        parent &&
        !wouldCreateTreeCycle(span.span_id, span.parent_span_id, spanIndex)
      ) {
        parent.children.push(node);
        continue;
      }
    }

    addRoot(node, roots, rootIds);
  }

  sortTreeNodes(roots, inputOrder);
  return roots;
}

export function flattenPreorder(nodes: SpanTreeNode[]): SpanTreeRow[] {
  const rows: SpanTreeRow[] = [];

  const visit = (treeNodes: SpanTreeNode[], depth: number) => {
    for (const node of treeNodes) {
      rows.push({
        span: node.span,
        depth,
        hasChildren: node.children.length > 0,
      });
      visit(node.children, depth + 1);
    }
  };

  visit(nodes, 0);
  return rows;
}

export function deriveVisibleRows(
  nodes: SpanTreeNode[],
  expandedSpanIds: ReadonlySet<string>
): SpanTreeRow[] {
  const rows: SpanTreeRow[] = [];

  const visit = (treeNodes: SpanTreeNode[], depth: number) => {
    for (const node of treeNodes) {
      const hasChildren = node.children.length > 0;
      rows.push({
        span: node.span,
        depth,
        hasChildren,
      });

      if (hasChildren && expandedSpanIds.has(node.span.span_id)) {
        visit(node.children, depth + 1);
      }
    }
  };

  visit(nodes, 0);
  return rows;
}

export function getAncestorIds(
  spanId: string,
  spanIndex: ReadonlyMap<string, Span>
): string[] {
  const ancestors: string[] = [];
  const visited = new Set<string>([spanId]);
  let current = spanIndex.get(spanId);

  while (current?.parent_span_id) {
    const parent = spanIndex.get(current.parent_span_id);
    if (!parent || visited.has(parent.span_id)) {
      break;
    }

    ancestors.push(parent.span_id);
    visited.add(parent.span_id);
    current = parent;
  }

  return ancestors.reverse();
}

export function collectExpandableSpanIds(nodes: SpanTreeNode[]): Set<string> {
  const expandableSpanIds = new Set<string>();

  const visit = (treeNodes: SpanTreeNode[]) => {
    for (const node of treeNodes) {
      if (node.children.length > 0) {
        expandableSpanIds.add(node.span.span_id);
        visit(node.children);
      }
    }
  };

  visit(nodes);
  return expandableSpanIds;
}

function addRoot(node: SpanTreeNode, roots: SpanTreeNode[], rootIds: Set<string>) {
  if (rootIds.has(node.span.span_id)) {
    return;
  }

  roots.push(node);
  rootIds.add(node.span.span_id);
}

function wouldCreateTreeCycle(
  childSpanId: string,
  parentSpanId: string,
  spanIndex: ReadonlyMap<string, Span>
): boolean {
  const visited = new Set<string>([childSpanId]);
  let currentParent = spanIndex.get(parentSpanId);

  while (currentParent) {
    if (visited.has(currentParent.span_id)) {
      return true;
    }

    visited.add(currentParent.span_id);
    if (!currentParent.parent_span_id) {
      return false;
    }

    currentParent = spanIndex.get(currentParent.parent_span_id);
  }

  return false;
}

function sortTreeNodes(
  nodes: SpanTreeNode[],
  inputOrder: ReadonlyMap<string, number>,
  visited = new Set<string>()
) {
  nodes.sort((left, right) => {
    const delta =
      new Date(left.span.started_at).getTime() -
      new Date(right.span.started_at).getTime();

    if (delta !== 0) {
      return delta;
    }

    return (
      (inputOrder.get(left.span.span_id) ?? Number.MAX_SAFE_INTEGER) -
      (inputOrder.get(right.span.span_id) ?? Number.MAX_SAFE_INTEGER)
    );
  });

  for (const node of nodes) {
    if (visited.has(node.span.span_id)) {
      continue;
    }

    visited.add(node.span.span_id);
    sortTreeNodes(node.children, inputOrder, visited);
  }
}
