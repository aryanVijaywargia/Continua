import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useState, type KeyboardEvent } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { Download, MoreHorizontal, RefreshCw, Zap } from 'lucide-react';
import { fetchTraces, isAuthError, type Trace } from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import {
  Btn,
  Chip,
  DataTable,
  FacetGroup,
  FacetItem,
  FilterBar,
  PageHeader,
  SearchInput,
  StatusDot,
  Td,
  Th,
  Tr,
} from '../components/DebuggerKit';
import { PaginationControls } from '../components/PaginationControls';
import { STATUS_TONE } from '../components/statusTone';
import { useTracesSearchParams } from '../hooks/useTracesSearchParams';
import { DEFAULT_PAGE_SIZE, getLastValidOffset } from '../utils/pagination';
import {
  buildCanonicalQueryString,
  deriveActiveChips,
  ENGINE_PROJECTION_STATE_FILTER_VALUES,
  ENGINE_RUN_STATUS_FILTER_VALUES,
  formatEngineProjectionStateLabel,
  formatEngineRunStatusLabel,
  isoToLocalDateInputValue,
  localDateToISOEnd,
  localDateToISOStart,
} from '../utils/tracesSearchParams';
import {
  calculateDuration,
  formatCost,
  formatDuration,
  formatRelativeTime,
  formatTokens,
} from '../utils/format';
import { appendProjectToPath } from '../utils/projectSearchParams';
import { downloadJsonFile } from '../utils/downloadJson';

const DEBOUNCE_MS = 300;
const EMPTY_TRACES: Trace[] = [];

function normalizeTrimmedDraft(value: string): string {
  return value.trim();
}

function normalizeDurationDraft(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    return '';
  }

  const parsed = Number(trimmed);
  return Number.isInteger(parsed) && parsed > 0 ? String(parsed) : '';
}

function getDateRangeError(startDate: string, endDate: string): string | null {
  if (!startDate || !endDate) {
    return null;
  }

  return startDate > endDate
    ? 'Start date must be on or before the end date.'
    : null;
}

function getErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unknown error';
}

function useDebouncedDraftCommit({
  committedValue,
  draftValue,
  normalizeForComparison,
  onCommit,
}: {
  committedValue: string;
  draftValue: string;
  normalizeForComparison: (value: string) => string;
  onCommit: (value: string) => void;
}) {
  useEffect(() => {
    if (normalizeForComparison(draftValue) === committedValue) {
      return;
    }

    const timeoutId = window.setTimeout(() => {
      onCommit(draftValue);
    }, DEBOUNCE_MS);

    return () => {
      window.clearTimeout(timeoutId);
    };
  }, [committedValue, draftValue, normalizeForComparison, onCommit]);
}

export function TracesPage() {
  return <TracesContent />;
}

function TracesContent() {
  const location = useLocation();
  const { filters, setFilters, clearAll, clearChip } = useTracesSearchParams();
  const [searchDraft, setSearchDraft] = useState(filters.q ?? '');
  const [userIdDraft, setUserIdDraft] = useState(filters.user_id ?? '');
  const [engineInstanceKeyDraft, setEngineInstanceKeyDraft] = useState(
    filters.engine_instance_key ?? ''
  );
  const [minDurationDraft, setMinDurationDraft] = useState(
    filters.min_duration_ms?.toString() ?? ''
  );

  useEffect(() => setSearchDraft(filters.q ?? ''), [filters.q]);
  useEffect(() => setUserIdDraft(filters.user_id ?? ''), [filters.user_id]);
  useEffect(
    () => setEngineInstanceKeyDraft(filters.engine_instance_key ?? ''),
    [filters.engine_instance_key]
  );
  useEffect(
    () => setMinDurationDraft(filters.min_duration_ms?.toString() ?? ''),
    [filters.min_duration_ms]
  );

  const commitSearch = useCallback(
    (value: string) => {
      const normalizedValue = normalizeTrimmedDraft(value);
      setSearchDraft(normalizedValue);
      setFilters({ q: normalizedValue || undefined }, 'replace');
    },
    [setFilters]
  );
  const commitUserId = useCallback(
    (value: string) => {
      const normalizedValue = normalizeTrimmedDraft(value);
      setUserIdDraft(normalizedValue);
      setFilters({ user_id: normalizedValue || undefined }, 'replace');
    },
    [setFilters]
  );
  const commitEngineInstanceKey = useCallback(
    (value: string) => {
      const normalizedValue = normalizeTrimmedDraft(value);
      setEngineInstanceKeyDraft(normalizedValue);
      setFilters({ engine_instance_key: normalizedValue || undefined }, 'replace');
    },
    [setFilters]
  );
  const commitMinDuration = useCallback(
    (value: string) => {
      const normalizedValue = normalizeDurationDraft(value);
      setMinDurationDraft(normalizedValue);
      setFilters(
        {
          min_duration_ms: normalizedValue ? Number(normalizedValue) : undefined,
        },
        'replace'
      );
    },
    [setFilters]
  );
  const commitOnEnter = useCallback(
    (event: KeyboardEvent<HTMLInputElement>, commit: (value: string) => void) => {
      if (event.key === 'Enter') {
        commit(event.currentTarget.value);
      }
    },
    []
  );

  useDebouncedDraftCommit({
    committedValue: filters.q ?? '',
    draftValue: searchDraft,
    normalizeForComparison: normalizeTrimmedDraft,
    onCommit: commitSearch,
  });
  useDebouncedDraftCommit({
    committedValue: filters.user_id ?? '',
    draftValue: userIdDraft,
    normalizeForComparison: normalizeTrimmedDraft,
    onCommit: commitUserId,
  });
  useDebouncedDraftCommit({
    committedValue: filters.engine_instance_key ?? '',
    draftValue: engineInstanceKeyDraft,
    normalizeForComparison: normalizeTrimmedDraft,
    onCommit: commitEngineInstanceKey,
  });
  useDebouncedDraftCommit({
    committedValue: filters.min_duration_ms?.toString() ?? '',
    draftValue: minDurationDraft,
    normalizeForComparison: normalizeDurationDraft,
    onCommit: commitMinDuration,
  });

  const startDate = isoToLocalDateInputValue(filters.start_time_from);
  const endDate = isoToLocalDateInputValue(filters.start_time_to);
  const dateRangeError = getDateRangeError(startDate, endDate);
  const activeChips = deriveActiveChips(filters);
  const hasActiveFilters = activeChips.length > 0;
  const queryParams = { ...filters };
  const canonicalQueryString = buildCanonicalQueryString(queryParams);
  const tracesQuery = useQuery({
    queryKey: ['traces', canonicalQueryString],
    queryFn: () => fetchTraces(queryParams),
    enabled: !dateRangeError,
    placeholderData: keepPreviousData,
    refetchInterval: 5000,
  });
  const traces = tracesQuery.data?.traces ?? EMPTY_TRACES;
  const total = tracesQuery.data?.total ?? 0;
  const currentListUrl = `${location.pathname}${location.search}`;
  const handleExport = useCallback(() => {
    downloadJsonFile('continua-traces.json', {
      exported_at: new Date().toISOString(),
      source: currentListUrl,
      filters,
      total,
      count: traces.length,
      traces,
    });
  }, [currentListUrl, filters, total, traces]);
  const statusCounts = {
    RUNNING: traces.filter((trace) => trace.status === 'RUNNING').length,
    COMPLETED: traces.filter((trace) => trace.status === 'COMPLETED').length,
    FAILED: traces.filter((trace) => trace.status === 'FAILED').length,
  };
  const engineDefinitionCounts = traces.reduce<Map<string, number>>((counts, trace) => {
    const definitionName = trace.engine?.definition_name;
    if (definitionName) {
      counts.set(definitionName, (counts.get(definitionName) ?? 0) + 1);
    }
    return counts;
  }, new Map<string, number>());
  if (
    filters.engine_definition_name &&
    !engineDefinitionCounts.has(filters.engine_definition_name)
  ) {
    engineDefinitionCounts.set(filters.engine_definition_name, 0);
  }
  const engineDefinitionItems = Array.from(engineDefinitionCounts.entries());

  useEffect(() => {
    if (traces.length !== 0 || total === 0 || filters.offset === 0) {
      return;
    }

    const lastValidOffset = getLastValidOffset(total, filters.limit ?? DEFAULT_PAGE_SIZE);
    if (lastValidOffset !== filters.offset) {
      setFilters({ offset: lastValidOffset }, 'replace');
    }
  }, [filters.limit, filters.offset, setFilters, total, traces.length]);

  const handleStartedSortToggle = useCallback(() => {
    if (filters.q) {
      return;
    }

    setFilters(
      {
        sort_by: 'started_at',
        sort_dir:
          filters.sort_by === 'started_at' && filters.sort_dir === 'asc'
            ? 'desc'
            : 'asc',
      },
      'push'
    );
  }, [filters.q, filters.sort_by, filters.sort_dir, setFilters]);

  return (
    <div className="flex min-h-0 flex-1">
      <aside className="hidden w-60 shrink-0 overflow-y-auto border-r border-[var(--c-border)] bg-[var(--c-app-bg)] lg:block">
        <div className="flex items-center justify-between px-3.5 py-3">
          <span className="text-xs font-semibold text-[var(--c-text-primary)]">
            Filters
          </span>
          {hasActiveFilters ? (
            <button
              type="button"
              onClick={clearAll}
              className="text-[11.5px] text-[var(--c-text-muted)] hover:text-[var(--c-text-primary)]"
            >
              Reset
            </button>
          ) : null}
        </div>

        <FacetGroup label="Status" count={filters.status ? 1 : 0}>
          {(['RUNNING', 'COMPLETED', 'FAILED'] as const).map((status) => (
            <FacetItem
              key={status}
              ariaLabel={`Filter ${STATUS_TONE[status].label} traces`}
              checked={filters.status === status.toLowerCase()}
              count={statusCounts[status]}
              dot={STATUS_TONE[status].dot}
              label={STATUS_TONE[status].label}
              onChange={() =>
                setFilters(
                  {
                    status:
                      filters.status === status.toLowerCase()
                        ? undefined
                        : (status.toLowerCase() as 'running' | 'completed' | 'failed'),
                  },
                  'push'
                )
              }
            />
          ))}
        </FacetGroup>

        <FacetGroup
          label="Engine definition"
          count={filters.engine_definition_name ? 1 : 0}
        >
          {engineDefinitionItems.length > 0 ? (
            engineDefinitionItems.map(([definitionName, count]) => (
              <FacetItem
                key={definitionName}
                ariaLabel={`Filter ${definitionName} engine definition`}
                checked={filters.engine_definition_name === definitionName}
                count={count}
                label={definitionName}
                onChange={() =>
                  setFilters(
                    {
                      engine_definition_name:
                        filters.engine_definition_name === definitionName
                          ? undefined
                          : definitionName,
                    },
                    'push'
                  )
                }
              />
            ))
          ) : (
            <p className="py-1 text-[12.5px] text-[var(--c-text-muted)]">
              No engine definitions
            </p>
          )}
        </FacetGroup>

        <FacetGroup label="Engine instance" defaultOpen={false} count={filters.engine_instance_key ? 1 : 0}>
          <input
            aria-label="Engine Instance Key"
            value={engineInstanceKeyDraft}
            onChange={(event) => setEngineInstanceKeyDraft(event.target.value)}
            onKeyDown={(event) => commitOnEnter(event, commitEngineInstanceKey)}
            placeholder="Instance key"
            className="h-7 w-full rounded border border-[var(--c-border)] bg-[var(--c-app-bg)] px-2 text-xs text-[var(--c-text-primary)] outline-none"
          />
        </FacetGroup>

        <FacetGroup label="Engine status" defaultOpen={false} count={filters.engine_run_status ? 1 : 0}>
          <select
            aria-label="Engine Status"
            value={filters.engine_run_status ?? ''}
            onChange={(event) =>
              setFilters(
                {
                  engine_run_status: event.target.value
                    ? (event.target.value as (typeof ENGINE_RUN_STATUS_FILTER_VALUES)[number])
                    : undefined,
                },
                'push'
              )
            }
            className="h-7 w-full rounded border border-[var(--c-border)] bg-[var(--c-app-bg)] px-2 text-xs text-[var(--c-text-primary)] outline-none"
          >
            <option value="">All engine statuses</option>
            {ENGINE_RUN_STATUS_FILTER_VALUES.map((value) => (
              <option key={value} value={value}>
                {formatEngineRunStatusLabel(value)}
              </option>
            ))}
          </select>
        </FacetGroup>

        <FacetGroup label="Projection" defaultOpen={false} count={filters.engine_projection_state ? 1 : 0}>
          <select
            aria-label="Projection State"
            value={filters.engine_projection_state ?? ''}
            onChange={(event) =>
              setFilters(
                {
                  engine_projection_state: event.target.value
                    ? (event.target.value as (typeof ENGINE_PROJECTION_STATE_FILTER_VALUES)[number])
                    : undefined,
                },
                'push'
              )
            }
            className="h-7 w-full rounded border border-[var(--c-border)] bg-[var(--c-app-bg)] px-2 text-xs text-[var(--c-text-primary)] outline-none"
          >
            <option value="">All projection states</option>
            {ENGINE_PROJECTION_STATE_FILTER_VALUES.map((value) => (
              <option key={value} value={value}>
                {formatEngineProjectionStateLabel(value)}
              </option>
            ))}
          </select>
          <p className="mt-2 text-[11px] leading-4 text-[var(--c-text-muted)]">
            Advanced operator filter for inspecting projection health across engine traces.
          </p>
        </FacetGroup>

        <FacetGroup label="Errors" count={filters.has_errors ? 1 : 0}>
          <FacetItem
            ariaLabel="Only show traces with errors"
            checked={Boolean(filters.has_errors)}
            count={traces.filter((trace) => (trace.error_count ?? 0) > 0).length}
            label="Has errors"
            onChange={() =>
              setFilters({ has_errors: filters.has_errors ? undefined : true }, 'push')
            }
          />
        </FacetGroup>

        <FacetGroup label="Time range" defaultOpen={false}>
          <div className="grid grid-cols-2 gap-2">
            <input
              aria-label="Start Date"
              type="date"
              value={startDate}
              onChange={(event) =>
                setFilters(
                  {
                    start_time_from: event.target.value
                      ? localDateToISOStart(event.target.value)
                      : undefined,
                  },
                  'push'
                )
              }
              className="h-7 min-w-0 rounded border border-[var(--c-border)] bg-[var(--c-app-bg)] px-1 text-[11px] text-[var(--c-text-primary)] outline-none"
            />
            <input
              aria-label="End Date"
              type="date"
              value={endDate}
              onChange={(event) =>
                setFilters(
                  {
                    start_time_to: event.target.value
                      ? localDateToISOEnd(event.target.value)
                      : undefined,
                  },
                  'push'
                )
              }
              className="h-7 min-w-0 rounded border border-[var(--c-border)] bg-[var(--c-app-bg)] px-1 text-[11px] text-[var(--c-text-primary)] outline-none"
            />
          </div>
        </FacetGroup>

        <FacetGroup label="Duration" defaultOpen={false} count={filters.min_duration_ms ? 1 : 0}>
          <div className="flex items-center gap-1.5 text-[11.5px] text-[var(--c-text-muted)]">
            <input
              aria-label="Min Duration (ms)"
              type="number"
              min="1"
              step="1"
              value={minDurationDraft}
              onChange={(event) => setMinDurationDraft(event.target.value)}
              onKeyDown={(event) => commitOnEnter(event, commitMinDuration)}
              placeholder="Min"
              className="h-7 min-w-0 flex-1 rounded border border-[var(--c-border)] bg-[var(--c-app-bg)] px-2 text-xs text-[var(--c-text-primary)] outline-none"
            />
            <span>—</span>
            <input
              aria-label="Max Duration (ms)"
              disabled
              placeholder="Max ms"
              className="h-7 min-w-0 flex-1 rounded border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-2 text-xs text-[var(--c-text-muted)] outline-none"
            />
          </div>
        </FacetGroup>

        <FacetGroup label="User" defaultOpen={false} count={filters.user_id ? 1 : 0}>
          <input
            aria-label="User ID"
            value={userIdDraft}
            onChange={(event) => setUserIdDraft(event.target.value)}
            onKeyDown={(event) => commitOnEnter(event, commitUserId)}
            placeholder="usr_…"
            className="h-7 w-full rounded border border-[var(--c-border)] bg-[var(--c-app-bg)] px-2 text-xs text-[var(--c-text-primary)] outline-none"
          />
        </FacetGroup>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <PageHeader
          actions={
            <>
              <Btn kind="secondary" leadingIcon={Download} size="sm" onClick={handleExport}>
                Export
              </Btn>
            </>
          }
          description={`${traces.length} of ${total} traces · auto-refreshing every 5s`}
          title="Traces"
        />

        <FilterBar
          count={activeChips.length}
          onClear={clearAll}
          right={
            <div className="flex items-center gap-2">
              <span className="text-[11.5px] text-[var(--c-text-muted)]">
                {tracesQuery.isFetching && !tracesQuery.isPending
                  ? 'Refreshing…'
                  : `${traces.length} results`}
              </span>
              <Btn
                kind="secondary"
                leadingIcon={RefreshCw}
                size="sm"
                onClick={() => void tracesQuery.refetch()}
              >
                Auto
              </Btn>
            </div>
          }
        >
          <SearchInput
            aria-label="Search"
            value={searchDraft}
            onChange={(event) => setSearchDraft(event.target.value)}
            onKeyDown={(event) => commitOnEnter(event, commitSearch)}
            onClear={() => {
              setSearchDraft('');
              setFilters({ q: undefined }, 'push');
            }}
            placeholder="Search trace, span, session, or user…"
          />
          <span className="sr-only">Search names, user IDs, and matching span names.</span>
          {activeChips.map((chip) => (
            <Chip
              key={chip.key}
              closeLabel={`Clear ${chip.label} filter`}
              onClose={() => clearChip(chip.key)}
            >
              <span>{chip.label}:</span> <span>{chip.value}</span>
            </Chip>
          ))}
        </FilterBar>

        {tracesQuery.error ? (
          isAuthError(tracesQuery.error) ? (
            <AuthErrorBanner message={getErrorMessage(tracesQuery.error)} />
          ) : (
            <div className="border-b border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-6 py-3 text-sm text-[var(--c-red-text)]">
              <span>Could not load traces</span>: {getErrorMessage(tracesQuery.error)}
              <button
                type="button"
                className="ml-3 font-semibold underline underline-offset-2"
                onClick={() => void tracesQuery.refetch()}
              >
                Retry
              </button>
            </div>
          )
        ) : null}
        {dateRangeError ? (
          <div className="border-b border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-6 py-3 text-sm text-[var(--c-red-text)]">
            {dateRangeError}
          </div>
        ) : null}

        {dateRangeError ? (
          <div className="app-empty-state">Fix the date range to load traces.</div>
        ) : tracesQuery.isPending && !tracesQuery.data ? (
          <div className="app-empty-state">Loading traces...</div>
        ) : tracesQuery.error && !tracesQuery.data ? (
          <div className="app-empty-state">Retry the request or adjust your filters to continue.</div>
        ) : traces.length === 0 ? (
          <div className="app-empty-state">
            <h2 className="text-base font-semibold text-[var(--c-text-primary)]">
              {hasActiveFilters ? 'No matching traces' : 'No traces yet'}
            </h2>
            <p className="mt-2">
              {hasActiveFilters
                ? 'Try broadening the filters or clearing them entirely.'
                : 'Start sending traces from your application to see them here.'}
            </p>
          </div>
        ) : (
          <>
            <DataTable>
              <colgroup>
                <col className="w-[30%]" />
                <col className="w-[110px]" />
                <col className="w-[140px]" />
                <col className="w-[90px]" />
                <col className="w-[90px]" />
                <col className="w-[90px]" />
                <col className="w-[60px]" />
                <col className="w-[130px]" />
                <col className="w-8" />
              </colgroup>
              <thead>
                <tr>
                  <Th>Trace</Th>
                  <Th>Status</Th>
                  <Th>Engine</Th>
                  <Th align="right">Duration</Th>
                  <Th align="right">Tokens</Th>
                  <Th align="right">Cost</Th>
                  <Th align="right">Errors</Th>
                  <Th
                    align="right"
                    sortable={!filters.q}
                    sortActive={filters.sort_by === 'started_at'}
                    sortDir={filters.sort_dir}
                    onSort={handleStartedSortToggle}
                  >
                    Started
                  </Th>
                  <Th>
                    <span className="sr-only">Actions</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {traces.map((trace) => (
                  <TraceRow
                    key={trace.id}
                    projectId={filters.project_id}
                    returnTo={currentListUrl}
                    trace={trace}
                  />
                ))}
              </tbody>
            </DataTable>
            <div className="border-t border-[var(--c-border)] px-6 py-2">
              <PaginationControls
                offset={filters.offset}
                pageSize={filters.limit ?? DEFAULT_PAGE_SIZE}
                total={total}
                currentItemCount={traces.length}
                onOffsetChange={(offset) => setFilters({ offset }, 'push')}
                onPageSizeChange={(limit) => setFilters({ limit }, 'push')}
                onRepairOffset={(offset) => setFilters({ offset }, 'replace')}
              />
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function TraceRow({
  projectId,
  returnTo,
  trace,
}: {
  projectId?: string;
  returnTo: string;
  trace: Trace;
}) {
  const duration = calculateDuration(trace.started_at, trace.ended_at);
  const totalTokens = (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);
  const tracePath = appendProjectToPath(`/traces/${trace.id}`, projectId);

  return (
    <Tr>
      <Td>
        <article className="min-w-0">
          <Link
            aria-label={trace.name}
            to={tracePath}
            state={{ returnTo }}
            className="flex min-w-0 flex-col gap-0.5 hover:text-[var(--c-accent-text)]"
          >
            <span className="truncate font-mono text-[12.5px] font-medium text-[var(--c-text-primary)]">
              {trace.name}
            </span>
          </Link>
          <span className="block truncate font-mono text-[10.5px] text-[var(--c-text-muted)]">
            {trace.id}
          </span>
        </article>
      </Td>
      <Td>
        <StatusDot status={trace.status} />
      </Td>
      <Td>
        {trace.engine ? (
          <Chip icon={Zap}>{trace.engine.definition_name}</Chip>
        ) : (
          <span className="text-[var(--c-text-muted)]">—</span>
        )}
      </Td>
      <Td align="right" mono>
        {formatDuration(duration)}
      </Td>
      <Td align="right" mono>
        {formatTokens(totalTokens)}
      </Td>
      <Td align="right" mono>
        {formatCost(trace.total_cost_usd)}
      </Td>
      <Td
        align="right"
        className={
          trace.error_count && trace.error_count > 0
            ? 'text-[var(--c-red-text)]'
            : 'text-[var(--c-text-muted)]'
        }
        mono
      >
        {trace.error_count ?? 0}
      </Td>
      <Td align="right" dim>
        {formatRelativeTime(trace.started_at)}
      </Td>
      <Td align="right">
        <button
          type="button"
          aria-label={`Open actions for ${trace.name}`}
          className="inline-flex text-[var(--c-text-muted)] hover:text-[var(--c-text-primary)]"
          onClick={(event) => event.stopPropagation()}
        >
          <MoreHorizontal className="h-3.5 w-3.5" />
        </button>
      </Td>
    </Tr>
  );
}
