export const INITIAL_COLLAPSE_DEPTH = 2;
export const INITIAL_COLLAPSE_CHILD_COUNT = 200;
export const MAX_EXPAND_ALL_NODE_COUNT = 5000;
const SIMPLE_OBJECT_KEY_PATTERN = /^[A-Za-z_$][A-Za-z0-9_$]*$/;

export type PayloadPathSegment = string | number;

interface PayloadTreeNodeBase {
  childCount: number;
  defaultExpanded: boolean;
  depth: number;
  key: PayloadPathSegment | null;
  path: string;
  subtreeNodeCount: number;
}

export interface ObjectNode extends PayloadTreeNodeBase {
  type: 'object';
  children: TreeNode[];
  value: Record<string, unknown>;
}

export interface ArrayNode extends PayloadTreeNodeBase {
  type: 'array';
  children: TreeNode[];
  value: unknown[];
}

export interface PrimitiveNode extends PayloadTreeNodeBase {
  type: 'primitive';
  value: string | number | boolean | null;
}

export type TreeNode = ObjectNode | ArrayNode | PrimitiveNode;

export interface PayloadMatch {
  id: string;
  path: string;
  target: 'key' | 'value';
}

export interface PayloadSearchResult {
  expandedPaths: Set<string>;
  matches: PayloadMatch[];
}

export function buildPayloadTree(data: unknown): TreeNode {
  return buildNode(data, null, '$', 0);
}

export function isCollectionNode(node: TreeNode): node is ObjectNode | ArrayNode {
  return node.type === 'object' || node.type === 'array';
}

export function collectInitiallyExpandedPaths(root: TreeNode): Set<string> {
  return collectPaths(root, (node) => node.defaultExpanded);
}

export function collectExpandablePaths(root: TreeNode): Set<string> {
  return collectPaths(root, () => true);
}

export function findPayloadMatches(
  root: TreeNode,
  query: string
): PayloadSearchResult {
  const normalizedQuery = query.trim().toLowerCase();

  if (!normalizedQuery) {
    return {
      expandedPaths: new Set<string>(),
      matches: [],
    };
  }

  const matches: PayloadMatch[] = [];
  const expandedPaths = new Set<string>();

  walkMatches(root, normalizedQuery, matches, expandedPaths);

  return {
    expandedPaths,
    matches,
  };
}

export function describeCollection(node: ObjectNode | ArrayNode): string {
  if (node.type === 'object') {
    return `{${node.childCount} ${pluralize(node.childCount, 'key')}}`;
  }

  return `[${node.childCount} ${pluralize(node.childCount, 'item')}]`;
}

export function formatLeafValueForDisplay(
  value: PrimitiveNode['value']
): string {
  if (typeof value === 'string') {
    return `"${value}"`;
  }

  return String(value);
}

export function formatLeafValueForClipboard(
  value: PrimitiveNode['value']
): string {
  if (typeof value === 'string') {
    return value;
  }

  return String(value);
}

export function serializeJsonForClipboard(value: unknown): string {
  try {
    const serialized = JSON.stringify(value, null, 2);
    return serialized ?? 'null';
  } catch {
    return String(value);
  }
}

function buildNode(
  value: unknown,
  key: PayloadPathSegment | null,
  path: string,
  depth: number
): TreeNode {
  if (Array.isArray(value)) {
    const children = value.map((item, index) =>
      buildNode(item, index, appendPathSegment(path, index), depth + 1)
    );

    return {
      type: 'array',
      childCount: children.length,
      children,
      defaultExpanded: shouldDefaultExpand(depth, children.length),
      depth,
      key,
      path,
      subtreeNodeCount:
        1 + children.reduce((total, child) => total + child.subtreeNodeCount, 0),
      value,
    };
  }

  if (value !== null && typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>);
    const children = entries.map(([childKey, childValue]) =>
      buildNode(childValue, childKey, appendPathSegment(path, childKey), depth + 1)
    );

    return {
      type: 'object',
      childCount: children.length,
      children,
      defaultExpanded: shouldDefaultExpand(depth, children.length),
      depth,
      key,
      path,
      subtreeNodeCount:
        1 + children.reduce((total, child) => total + child.subtreeNodeCount, 0),
      value: value as Record<string, unknown>,
    };
  }

  return {
    type: 'primitive',
    childCount: 0,
    defaultExpanded: false,
    depth,
    key,
    path,
    subtreeNodeCount: 1,
    value: normalizePrimitive(value),
  };
}

function normalizePrimitive(value: unknown): PrimitiveNode['value'] {
  if (
    value === null ||
    typeof value === 'string' ||
    typeof value === 'number' ||
    typeof value === 'boolean'
  ) {
    return value;
  }

  return String(value);
}

function shouldDefaultExpand(depth: number, childCount: number): boolean {
  if (depth >= INITIAL_COLLAPSE_DEPTH) {
    return false;
  }

  return childCount <= INITIAL_COLLAPSE_CHILD_COUNT;
}

function collectPaths(
  node: TreeNode,
  predicate: (node: ObjectNode | ArrayNode) => boolean
): Set<string> {
  const paths = new Set<string>();
  collectPathsRecursive(node, predicate, paths);
  return paths;
}

function collectPathsRecursive(
  node: TreeNode,
  predicate: (node: ObjectNode | ArrayNode) => boolean,
  paths: Set<string>
) {
  if (!isCollectionNode(node)) {
    return;
  }

  if (predicate(node)) {
    paths.add(node.path);
  }

  node.children.forEach((child) => {
    collectPathsRecursive(child, predicate, paths);
  });
}

function walkMatches(
  node: TreeNode,
  query: string,
  matches: PayloadMatch[],
  expandedPaths: Set<string>
): boolean {
  let hasMatch = false;

  if (node.key !== null && String(node.key).toLowerCase().includes(query)) {
    matches.push({
      id: buildMatchId(node.path, 'key'),
      path: node.path,
      target: 'key',
    });
    hasMatch = true;
  }

  if (
    node.type === 'primitive' &&
    formatLeafValueForClipboard(node.value).toLowerCase().includes(query)
  ) {
    matches.push({
      id: buildMatchId(node.path, 'value'),
      path: node.path,
      target: 'value',
    });
    hasMatch = true;
  }

  if (!isCollectionNode(node)) {
    return hasMatch;
  }

  let childHasMatch = false;

  for (const child of node.children) {
    if (walkMatches(child, query, matches, expandedPaths)) {
      childHasMatch = true;
    }
  }

  if (childHasMatch) {
    expandedPaths.add(node.path);
  }

  return hasMatch || childHasMatch;
}

function buildMatchId(path: string, target: PayloadMatch['target']): string {
  return `${path}:${target}`;
}

function appendPathSegment(
  path: string,
  segment: PayloadPathSegment
): string {
  if (typeof segment === 'number') {
    return `${path}[${segment}]`;
  }

  if (SIMPLE_OBJECT_KEY_PATTERN.test(segment)) {
    return `${path}.${segment}`;
  }

  return `${path}[${JSON.stringify(segment)}]`;
}

function pluralize(value: number, label: string): string {
  return `${label}${value === 1 ? '' : 's'}`;
}
