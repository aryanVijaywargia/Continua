import { useEffect, useMemo, useState } from 'react';
import { Span } from '../api/client';
import { StatusBadge } from './StatusBadge';
import { formatDuration } from '../utils/format';

interface SpanTreeProps {
  spans: Span[];
  selectedSpanId: string | null;
  onSelectSpan: (spanId: string) => void;
  failedSpanIds: Set<string>;
  primaryAncestorPath: Set<string>;
  revealPath: Set<string>;
  revealKey: number;
  inlineErrorPreviews: Map<string, string>;
}

interface SpanNode {
  span: Span;
  children: SpanNode[];
}

/**
 * Build a tree structure from flat span list.
 * Uses span_id (external) for parent-child relationships since
 * parent_span_id references the external span_id, not the internal UUID.
 */
function buildSpanTree(spans: Span[]): SpanNode[] {
  const spanMap = new Map<string, SpanNode>();
  const spanIndex = new Map<string, Span>();
  const roots: SpanNode[] = [];
  const rootIds = new Set<string>();

  // Create nodes for all spans, keyed by external span_id
  for (const span of spans) {
    spanMap.set(span.span_id, { span, children: [] });
    spanIndex.set(span.span_id, span);
  }

  // Build tree structure using external IDs
  for (const span of spans) {
    const node = spanMap.get(span.span_id)!;
    if (span.parent_span_id) {
      const parent = spanMap.get(span.parent_span_id);
      if (
        parent &&
        !wouldCreateTreeCycle(span.span_id, span.parent_span_id, spanIndex)
      ) {
        parent.children.push(node);
      } else {
        addRoot(node, roots, rootIds);
      }
    } else {
      addRoot(node, roots, rootIds);
    }
  }

  // Sort children by started_at
  const sortChildren = (nodes: SpanNode[], visited = new Set<string>()) => {
    nodes.sort((a, b) =>
      new Date(a.span.started_at).getTime() - new Date(b.span.started_at).getTime()
    );
    for (const node of nodes) {
      if (visited.has(node.span.span_id)) {
        continue;
      }
      visited.add(node.span.span_id);
      sortChildren(node.children, visited);
    }
  };
  sortChildren(roots);

  return roots;
}

function addRoot(node: SpanNode, roots: SpanNode[], rootIds: Set<string>) {
  if (rootIds.has(node.span.span_id)) {
    return;
  }

  roots.push(node);
  rootIds.add(node.span.span_id);
}

function wouldCreateTreeCycle(
  childSpanId: string,
  parentSpanId: string,
  spanIndex: Map<string, Span>
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

/**
 * Recursive span tree component.
 */
export function SpanTree({
  spans,
  selectedSpanId,
  onSelectSpan,
  failedSpanIds,
  primaryAncestorPath,
  revealPath,
  revealKey,
  inlineErrorPreviews,
}: SpanTreeProps) {
  const tree = useMemo(() => buildSpanTree(spans), [spans]);

  if (tree.length === 0) {
    return (
      <div className="p-4 text-gray-500 text-center">
        No spans found for this trace.
      </div>
    );
  }

  return (
    <div className="py-2">
      {tree.map((node) => (
        <SpanTreeNode
          key={node.span.id}
          node={node}
          depth={0}
          selectedSpanId={selectedSpanId}
          onSelectSpan={onSelectSpan}
          failedSpanIds={failedSpanIds}
          primaryAncestorPath={primaryAncestorPath}
          revealPath={revealPath}
          revealKey={revealKey}
          inlineErrorPreviews={inlineErrorPreviews}
        />
      ))}
    </div>
  );
}

interface SpanTreeNodeProps {
  node: SpanNode;
  depth: number;
  selectedSpanId: string | null;
  onSelectSpan: (spanId: string) => void;
  failedSpanIds: Set<string>;
  primaryAncestorPath: Set<string>;
  revealPath: Set<string>;
  revealKey: number;
  inlineErrorPreviews: Map<string, string>;
}

const kindColors: Record<string, string> = {
  LLM: 'bg-purple-100 text-purple-800',
  TOOL: 'bg-yellow-100 text-yellow-800',
  CHAIN: 'bg-blue-100 text-blue-800',
  AGENT: 'bg-green-100 text-green-800',
  CUSTOM: 'bg-gray-100 text-gray-800',
};

function SpanTreeNode({
  node,
  depth,
  selectedSpanId,
  onSelectSpan,
  failedSpanIds,
  primaryAncestorPath,
  revealPath,
  revealKey,
  inlineErrorPreviews,
}: SpanTreeNodeProps) {
  const [isExpanded, setIsExpanded] = useState(true);
  const hasChildren = node.children.length > 0;
  const isSelected = node.span.span_id === selectedSpanId;
  const isFailed = failedSpanIds.has(node.span.span_id);
  const isOnPrimaryPath = primaryAncestorPath.has(node.span.span_id);
  const errorPreview = inlineErrorPreviews.get(node.span.span_id);
  const rowStateId = `span-row-state-${node.span.id}`;

  useEffect(() => {
    if (hasChildren && revealPath.has(node.span.span_id)) {
      setIsExpanded(true);
    }
  }, [hasChildren, node.span.span_id, revealKey, revealPath]);

  const rowClasses = isSelected
    ? 'border-blue-200 bg-blue-50 shadow-sm ring-1 ring-blue-200'
    : isOnPrimaryPath
      ? 'border-amber-300 bg-amber-50'
      : isFailed
        ? 'border-red-200 bg-red-50/80'
        : 'border-transparent bg-white hover:bg-gray-50';

  return (
    <div>
      <div
        className="flex items-start gap-1 px-2 py-1"
        style={{ paddingLeft: `${depth * 20 + 8}px` }}
      >
        {hasChildren ? (
          <button
            type="button"
            className="mt-3 flex h-6 w-6 shrink-0 items-center justify-center rounded text-gray-400 transition hover:bg-gray-100 hover:text-gray-600 focus:outline-none focus:ring-2 focus:ring-blue-200"
            aria-label={`${isExpanded ? 'Collapse' : 'Expand'} span ${node.span.name}`}
            onClick={() => setIsExpanded((expanded) => !expanded)}
          >
            {isExpanded ? '▼' : '▶'}
          </button>
        ) : (
          <span
            className="mt-3 flex h-6 w-6 shrink-0 items-center justify-center text-gray-300"
            aria-hidden="true"
          >
            ·
          </span>
        )}

        <button
          type="button"
          className={`flex min-w-0 flex-1 items-start justify-between rounded-xl border px-3 py-2 text-left transition ${rowClasses}`}
          aria-label={`Select span ${node.span.name}`}
          aria-describedby={rowStateId}
          aria-pressed={isSelected}
          onClick={() => onSelectSpan(node.span.span_id)}
        >
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span
                className={`rounded px-1.5 py-0.5 text-xs font-medium ${
                  kindColors[node.span.kind] || kindColors.CUSTOM
                }`}
              >
                {node.span.kind}
              </span>
              <span className="truncate text-sm font-medium text-gray-900">
                {node.span.name}
              </span>
              {isSelected && (
                <span className="rounded-full border border-blue-200 bg-white px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.16em] text-blue-700">
                  Selected
                </span>
              )}
              {isOnPrimaryPath && (
                <span className="rounded-full border border-amber-200 bg-white px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.16em] text-amber-700">
                  Failure path
                </span>
              )}
              {isFailed && (
                <span className="rounded-full border border-red-200 bg-white px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.16em] text-red-700">
                  Failed
                </span>
              )}
            </div>

            <span id={rowStateId} className="sr-only">
              {describeRowState(isSelected, isOnPrimaryPath, isFailed)}
            </span>

            {errorPreview && (
              <p className="mt-1 truncate text-xs text-red-700">{errorPreview}</p>
            )}
          </div>

          <div className="ml-3 flex shrink-0 items-center gap-2">
            <span className="text-xs text-gray-500">
              {formatDuration(node.span.latency_ms)}
            </span>
            <StatusBadge status={node.span.status} />
          </div>
        </button>
      </div>

      {/* Children */}
      {hasChildren && isExpanded && (
        <div>
          {node.children.map((child) => (
            <SpanTreeNode
              key={child.span.id}
              node={child}
              depth={depth + 1}
              selectedSpanId={selectedSpanId}
              onSelectSpan={onSelectSpan}
              failedSpanIds={failedSpanIds}
              primaryAncestorPath={primaryAncestorPath}
              revealPath={revealPath}
              revealKey={revealKey}
              inlineErrorPreviews={inlineErrorPreviews}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function describeRowState(
  isSelected: boolean,
  isOnPrimaryPath: boolean,
  isFailed: boolean
): string {
  const states = ['Selectable span row'];

  if (isSelected) {
    states.push('Currently selected');
  }
  if (isOnPrimaryPath) {
    states.push('On the primary failure path');
  }
  if (isFailed) {
    states.push('Failed span');
  }

  return states.join('. ');
}
