import { useState, useMemo } from 'react';
import { Span } from '../api/client';
import { StatusBadge } from './StatusBadge';
import { formatDuration } from '../utils/format';

interface SpanTreeProps {
  spans: Span[];
  selectedSpanId: string | null;
  onSelectSpan: (span: Span) => void;
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
  const roots: SpanNode[] = [];

  // Create nodes for all spans, keyed by external span_id
  for (const span of spans) {
    spanMap.set(span.span_id, { span, children: [] });
  }

  // Build tree structure using external IDs
  for (const span of spans) {
    const node = spanMap.get(span.span_id)!;
    if (span.parent_span_id) {
      const parent = spanMap.get(span.parent_span_id);
      if (parent) {
        parent.children.push(node);
      } else {
        // Parent not found, treat as root
        roots.push(node);
      }
    } else {
      roots.push(node);
    }
  }

  // Sort children by started_at
  const sortChildren = (nodes: SpanNode[]) => {
    nodes.sort((a, b) =>
      new Date(a.span.started_at).getTime() - new Date(b.span.started_at).getTime()
    );
    for (const node of nodes) {
      sortChildren(node.children);
    }
  };
  sortChildren(roots);

  return roots;
}

/**
 * Recursive span tree component.
 */
export function SpanTree({ spans, selectedSpanId, onSelectSpan }: SpanTreeProps) {
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
        />
      ))}
    </div>
  );
}

interface SpanTreeNodeProps {
  node: SpanNode;
  depth: number;
  selectedSpanId: string | null;
  onSelectSpan: (span: Span) => void;
}

const kindColors: Record<string, string> = {
  LLM: 'bg-purple-100 text-purple-800',
  TOOL: 'bg-yellow-100 text-yellow-800',
  CHAIN: 'bg-blue-100 text-blue-800',
  AGENT: 'bg-green-100 text-green-800',
  CUSTOM: 'bg-gray-100 text-gray-800',
};

function SpanTreeNode({ node, depth, selectedSpanId, onSelectSpan }: SpanTreeNodeProps) {
  const [isExpanded, setIsExpanded] = useState(true);
  const hasChildren = node.children.length > 0;
  const isSelected = node.span.id === selectedSpanId;

  return (
    <div>
      <div
        className={`flex items-center px-2 py-1 cursor-pointer hover:bg-gray-100 ${
          isSelected ? 'bg-blue-50 border-l-2 border-blue-500' : ''
        }`}
        style={{ paddingLeft: `${depth * 20 + 8}px` }}
        onClick={() => onSelectSpan(node.span)}
      >
        {/* Expand/collapse toggle */}
        <button
          className="w-5 h-5 flex items-center justify-center text-gray-400 mr-1"
          onClick={(e) => {
            e.stopPropagation();
            setIsExpanded(!isExpanded);
          }}
        >
          {hasChildren ? (isExpanded ? '▼' : '▶') : '·'}
        </button>

        {/* Kind badge */}
        <span
          className={`px-1.5 py-0.5 text-xs font-medium rounded mr-2 ${
            kindColors[node.span.kind] || kindColors.CUSTOM
          }`}
        >
          {node.span.kind}
        </span>

        {/* Span name */}
        <span className="flex-1 truncate text-sm font-medium text-gray-900">
          {node.span.name}
        </span>

        {/* Duration */}
        <span className="text-xs text-gray-500 mx-2">
          {formatDuration(node.span.latency_ms)}
        </span>

        {/* Status */}
        <StatusBadge status={node.span.status} />
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
            />
          ))}
        </div>
      )}
    </div>
  );
}
