import {
  useCallback,
  useDeferredValue,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type HTMLAttributes,
} from 'react';
import { CopyButton } from './CopyButton';
import {
  MAX_EXPAND_ALL_NODE_COUNT,
  buildPayloadTree,
  collectExpandablePaths,
  collectInitiallyExpandedPaths,
  describeCollection,
  findPayloadMatches,
  formatLeafValueForClipboard,
  formatLeafValueForDisplay,
  isCollectionNode,
  serializeJsonForClipboard,
  type PayloadMatch,
  type TreeNode,
} from '../utils/payloadTree';

interface PayloadInspectorProps {
  data: unknown;
  className?: string;
}

export function PayloadInspector({
  data,
  className = '',
}: PayloadInspectorProps) {
  const tree = useMemo(
    () => (data === undefined ? null : buildPayloadTree(data)),
    [data]
  );
  const initialExpandedPaths = useMemo(
    () => (tree ? collectInitiallyExpandedPaths(tree) : new Set<string>()),
    [tree]
  );
  const [searchQuery, setSearchQuery] = useState('');
  const deferredSearchQuery = useDeferredValue(searchQuery);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(
    () => new Set(initialExpandedPaths)
  );
  const [activeMatchIndex, setActiveMatchIndex] = useState(0);
  const searchInputId = useId();
  const preSearchExpandedPathsRef = useRef<Set<string> | null>(null);
  const previousDeferredSearchQueryRef = useRef('');
  const expandedPathsRef = useRef(expandedPaths);
  const matchElementsRef = useRef(new Map<string, HTMLElement>());

  useEffect(() => {
    expandedPathsRef.current = expandedPaths;
  }, [expandedPaths]);

  useEffect(() => {
    setExpandedPaths(new Set(initialExpandedPaths));
    setSearchQuery('');
    setActiveMatchIndex(0);
    preSearchExpandedPathsRef.current = null;
    matchElementsRef.current.clear();
  }, [initialExpandedPaths]);

  const searchResult = useMemo(
    () => (tree ? findPayloadMatches(tree, deferredSearchQuery) : emptySearchResult()),
    [deferredSearchQuery, tree]
  );
  const hasActiveSearch = deferredSearchQuery.trim().length > 0;
  const effectiveExpandedPaths = useMemo(() => {
    if (!hasActiveSearch) {
      return expandedPaths;
    }

    const mergedPaths = new Set(expandedPaths);
    searchResult.expandedPaths.forEach((path) => {
      mergedPaths.add(path);
    });
    return mergedPaths;
  }, [expandedPaths, hasActiveSearch, searchResult.expandedPaths]);
  const activeMatch =
    searchResult.matches.length > 0
      ? searchResult.matches[activeMatchIndex % searchResult.matches.length] ?? null
      : null;

  useEffect(() => {
    const previousDeferredSearchQuery = previousDeferredSearchQueryRef.current;
    previousDeferredSearchQueryRef.current = deferredSearchQuery;

    if (!hasActiveSearch) {
      if (preSearchExpandedPathsRef.current) {
        setExpandedPaths(new Set(preSearchExpandedPathsRef.current));
        preSearchExpandedPathsRef.current = null;
      }
      setActiveMatchIndex(0);
      return;
    }

    if (previousDeferredSearchQuery.trim().length === 0) {
      preSearchExpandedPathsRef.current = new Set(expandedPathsRef.current);
    }

    if (previousDeferredSearchQuery !== deferredSearchQuery) {
      setActiveMatchIndex(0);
    }
  }, [deferredSearchQuery, hasActiveSearch]);

  useEffect(() => {
    if (searchResult.matches.length === 0) {
      if (activeMatchIndex !== 0) {
        setActiveMatchIndex(0);
      }
      return;
    }

    if (activeMatchIndex >= searchResult.matches.length) {
      setActiveMatchIndex(0);
    }
  }, [activeMatchIndex, searchResult.matches.length]);

  useEffect(() => {
    if (!activeMatch) {
      return;
    }

    const activeMatchElement = matchElementsRef.current.get(activeMatch.id);
    activeMatchElement?.scrollIntoView({
      block: 'nearest',
      inline: 'nearest',
    });
  }, [activeMatch]);

  const toggleExpanded = useCallback((path: string) => {
    setExpandedPaths((current) => {
      const next = new Set(current);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  }, []);

  const handleExpandAll = useCallback(() => {
    if (!tree || tree.subtreeNodeCount > MAX_EXPAND_ALL_NODE_COUNT) {
      return;
    }

    setExpandedPaths(collectExpandablePaths(tree));
  }, [tree]);

  const handleCollapseAll = useCallback(() => {
    setExpandedPaths(new Set(initialExpandedPaths));
  }, [initialExpandedPaths]);

  const registerMatchElement = useCallback(
    (matchId: string, element: HTMLElement | null) => {
      if (element) {
        matchElementsRef.current.set(matchId, element);
        return;
      }

      matchElementsRef.current.delete(matchId);
    },
    []
  );

  if (tree === null) {
    return (
      <div
        className={`rounded-md border border-dashed border-[var(--c-border)] bg-[var(--c-surface-muted)] px-4 py-6 text-sm text-[var(--c-text-muted)] ${className}`.trim()}
      >
        No data
      </div>
    );
  }

  const expandAllDisabled = tree.subtreeNodeCount > MAX_EXPAND_ALL_NODE_COUNT;
  const matchCount = searchResult.matches.length;
  const matchSummary = hasActiveSearch
    ? `${matchCount} ${matchCount === 1 ? 'match' : 'matches'}`
    : 'Search keys and values';

  return (
    <section
      className={`flex min-h-0 flex-col overflow-hidden rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] ${className}`.trim()}
    >
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--c-border)] bg-[var(--c-surface-muted)] p-3">
        <label className="sr-only" htmlFor={searchInputId}>
          Search payload
        </label>
        <input
          id={searchInputId}
          type="search"
          value={searchQuery}
          onChange={(event) => setSearchQuery(event.target.value)}
          placeholder="Search payload"
          aria-label="Search payload"
          className="min-w-[11rem] flex-1 rounded-md border border-[var(--c-border)] bg-[var(--c-app-bg)] px-3 py-2 text-sm text-[var(--c-text-primary)] transition placeholder:text-[var(--c-text-muted)] focus:border-[var(--c-accent)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]"
        />
        <button
          type="button"
          className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-1.5 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)] disabled:cursor-not-allowed disabled:opacity-60"
          disabled={matchCount === 0}
          onClick={() =>
            setActiveMatchIndex((index) =>
              matchCount === 0 ? 0 : (index - 1 + matchCount) % matchCount
            )
          }
        >
          Prev
        </button>
        <button
          type="button"
          className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-1.5 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)] disabled:cursor-not-allowed disabled:opacity-60"
          disabled={matchCount === 0}
          onClick={() =>
            setActiveMatchIndex((index) =>
              matchCount === 0 ? 0 : (index + 1) % matchCount
            )
          }
        >
          Next
        </button>
        <button
          type="button"
          className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-1.5 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)] disabled:cursor-not-allowed disabled:opacity-60"
          title={
            expandAllDisabled
              ? 'Expand all is disabled for payloads with more than 5,000 nodes.'
              : undefined
          }
          disabled={expandAllDisabled}
          onClick={handleExpandAll}
        >
          Expand all
        </button>
        <button
          type="button"
          className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-1.5 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]"
          onClick={handleCollapseAll}
        >
          Collapse all
        </button>
        <CopyButton
          aria-label="Copy full JSON"
          getValue={() => serializeJsonForClipboard(data)}
          idleLabel="Copy full JSON"
          successLabel="Copied JSON"
        />
        <span className="ml-auto text-xs text-[var(--c-text-muted)]">{matchSummary}</span>
      </div>

      <div className="min-h-0 overflow-auto p-3">
        <div className="font-mono text-xs leading-6 text-[var(--c-text-primary)]">
          <PayloadNodeRow
            node={tree}
            activeMatchId={activeMatch?.id ?? null}
            expandedPaths={effectiveExpandedPaths}
            matches={searchResult.matches}
            onToggleExpanded={toggleExpanded}
            registerMatchElement={registerMatchElement}
          />
        </div>
      </div>
    </section>
  );
}

interface PayloadNodeRowProps {
  activeMatchId: string | null;
  expandedPaths: Set<string>;
  matches: PayloadMatch[];
  node: TreeNode;
  onToggleExpanded: (path: string) => void;
  registerMatchElement: (matchId: string, element: HTMLElement | null) => void;
}

function PayloadNodeRow({
  activeMatchId,
  expandedPaths,
  matches,
  node,
  onToggleExpanded,
  registerMatchElement,
}: PayloadNodeRowProps) {
  const isExpanded = isCollectionNode(node) ? expandedPaths.has(node.path) : false;
  const keyMatchId = `${node.path}:key`;
  const valueMatchId = `${node.path}:value`;
  const keyMatch = matches.some((match) => match.id === keyMatchId);
  const valueMatch = matches.some((match) => match.id === valueMatchId);

  return (
    <div>
      <div
        className="flex items-start gap-2 py-1"
        style={{ paddingLeft: `${node.depth * 16}px` }}
      >
        {isCollectionNode(node) ? (
          <button
            type="button"
            aria-label={`${isExpanded ? 'Collapse' : 'Expand'} ${describeNode(node)}`}
            className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded text-[var(--c-text-muted)] transition hover:bg-[var(--c-row-hover-bg)] hover:text-[var(--c-text-secondary)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]"
            onClick={() => onToggleExpanded(node.path)}
          >
            {isExpanded ? '-' : '+'}
          </button>
        ) : (
          <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center text-[var(--c-text-muted)]">
            -
          </span>
        )}

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-start gap-2">
            {node.key !== null && (
              <>
                <HighlightableText
                  activeMatchId={activeMatchId}
                  className="break-all text-[var(--c-accent-text)]"
                  isMatch={keyMatch}
                  matchId={keyMatchId}
                  registerMatchElement={registerMatchElement}
                >
                  {String(node.key)}
                </HighlightableText>
                <span className="text-[var(--c-text-muted)]">:</span>
              </>
            )}

            {isCollectionNode(node) ? (
              <CollectionSummary node={node} />
            ) : (
              <HighlightableText
                activeMatchId={activeMatchId}
                className={
                  typeof node.value === 'string'
                    ? 'max-h-36 overflow-auto whitespace-pre-wrap break-words text-[var(--c-green-text)]'
                    : 'break-all text-[var(--c-text-primary)]'
                }
                isMatch={valueMatch}
                matchId={valueMatchId}
                registerMatchElement={registerMatchElement}
              >
                {formatLeafValueForDisplay(node.value)}
              </HighlightableText>
            )}

            <CopyButton
              aria-label={
                isCollectionNode(node)
                  ? `Copy JSON for ${describeNode(node)}`
                  : `Copy value for ${describeNode(node)}`
              }
              className="ml-auto"
              getValue={() =>
                isCollectionNode(node)
                  ? serializeJsonForClipboard(node.value)
                  : formatLeafValueForClipboard(node.value)
              }
              idleLabel="Copy"
              successLabel="Copied"
            />
          </div>
        </div>
      </div>

      {isCollectionNode(node) && isExpanded && (
        <div>
          {node.children.map((child) => (
            <PayloadNodeRow
              key={child.path}
              node={child}
              activeMatchId={activeMatchId}
              expandedPaths={expandedPaths}
              matches={matches}
              onToggleExpanded={onToggleExpanded}
              registerMatchElement={registerMatchElement}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function CollectionSummary({ node }: { node: TreeNode }) {
  if (!isCollectionNode(node)) {
    return null;
  }

  return (
    <span className="inline-flex flex-wrap items-center gap-2">
      <span className="text-[var(--c-text-muted)]">{node.type === 'object' ? '{}' : '[]'}</span>
      <span className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-2 py-0.5 text-[11px] font-medium text-[var(--c-text-secondary)]">
        {describeCollection(node)}
      </span>
    </span>
  );
}

interface HighlightableTextProps extends HTMLAttributes<HTMLSpanElement> {
  activeMatchId: string | null;
  isMatch: boolean;
  matchId: string;
  registerMatchElement: (matchId: string, element: HTMLElement | null) => void;
}

function HighlightableText({
  activeMatchId,
  children,
  className = '',
  isMatch,
  matchId,
  registerMatchElement,
  ...spanProps
}: HighlightableTextProps) {
  return (
    <span
      {...spanProps}
      ref={(element) => registerMatchElement(matchId, element)}
      className={`${className} ${
        isMatch
          ? activeMatchId === matchId
            ? 'rounded bg-[var(--c-amber-faint)] px-1 text-[var(--c-amber-text)] ring-1 ring-[var(--c-amber-border)]'
            : 'rounded bg-[var(--c-amber-faint)] px-1 text-[var(--c-amber-text)]'
          : ''
      }`.trim()}
      data-match-state={
        isMatch ? (activeMatchId === matchId ? 'active' : 'match') : undefined
      }
    >
      {children}
    </span>
  );
}

function describeNode(node: TreeNode): string {
  if (node.key !== null) {
    return String(node.key);
  }

  return 'payload';
}

function emptySearchResult() {
  return {
    expandedPaths: new Set<string>(),
    matches: [],
  };
}
