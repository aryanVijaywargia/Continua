import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useState, type KeyboardEvent } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { Clock3, Download, Link2, SlidersHorizontal, Zap } from 'lucide-react';
import { fetchTraces, isAuthError, type Trace } from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import {
  Btn,
  Chip,
  DataTable,
  FilterBar,
  SearchInput,
  StatusDot,
  Td,
  Th,
  Tr,
} from '../components/DebuggerKit';
import { HonestyNote } from '../components/DataState';
import { PaginationControls } from '../components/PaginationControls';
import { STATUS_TONE } from '../components/statusTone';
import { useTracesSearchParams } from '../hooks/useTracesSearchParams';
import { DEFAULT_PAGE_SIZE, getLastValidOffset } from '../utils/pagination';
import {
  buildCanonicalQueryString,
  deriveActiveChips,
  isoToLocalDateInputValue,
  localDateToISOEnd,
  localDateToISOStart,
} from '../utils/tracesSearchParams';
import { calculateDuration, formatDerivedDuration } from '../utils/format';
import { appendProjectToPath } from '../utils/projectSearchParams';
import { downloadJsonFile } from '../utils/downloadJson';
import { TracesAdvancedPanel } from './traces/TracesAdvancedPanel';
import {
  countAdvancedFilters,
  formatExactTimeWithSeconds,
  shortTraceId,
} from './traces/traceListFormat';

const DEBOUNCE_MS = 300;
const EMPTY_TRACES: Trace[] = [];
const ADVANCED_PANEL_ID = 'traces-advanced-filters';
const TOOLBAR_CONTROL_CLASS =
  'h-7 rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-2 text-xs font-medium text-[var(--c-text-primary)] outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]';
const DATE_INPUT_CLASS =
  'h-6 w-[118px] min-w-0 border-0 bg-transparent p-0 text-[11.5px] text-[var(--c-text-primary)] outline-none focus:ring-0';

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
  const engineDefinitionItems = Array.from(
    new Set(
      [
        ...traces.map((trace) => trace.engine?.definition_name),
        filters.engine_definition_name,
      ].filter((name): name is string => Boolean(name))
    )
  ).sort();

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

  const advancedFilterCount = countAdvancedFilters(filters);
  const [advancedOpen, setAdvancedOpen] = useState(advancedFilterCount > 0);
  useEffect(() => {
    if (advancedFilterCount > 0) {
      setAdvancedOpen(true);
    }
  }, [advancedFilterCount]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <header className="flex items-center justify-between gap-4 border-b border-[var(--c-border)] px-6 py-3">
        <div className="flex min-w-0 items-baseline gap-2.5">
          <h1 className="text-lg font-bold tracking-[-0.015em] text-[var(--c-text-primary)]">
            Traces
          </h1>
          <span className="truncate text-[12px] text-[var(--c-text-muted)]">
            auto-refreshes every 5s
          </span>
        </div>
        <Btn kind="secondary" leadingIcon={Download} size="sm" onClick={handleExport}>
          Export
        </Btn>
      </header>

      <FilterBar className="px-6 py-2.5">
        <SearchInput
          aria-label="Search"
          value={searchDraft}
          onChange={(event) => setSearchDraft(event.target.value)}
          onKeyDown={(event) => commitOnEnter(event, commitSearch)}
          onClear={() => {
            setSearchDraft('');
            setFilters({ q: undefined }, 'push');
          }}
          placeholder="Search trace, step, session, or user…"
          widthClass="w-full sm:w-[300px]"
        />
        <span className="sr-only">Search names, user IDs, and matching span names.</span>
        <select
          aria-label="Status"
          value={filters.status ?? ''}
          onChange={(event) =>
            setFilters(
              {
                status: event.target.value
                  ? (event.target.value as 'running' | 'completed' | 'failed')
                  : undefined,
              },
              'push'
            )
          }
          className={TOOLBAR_CONTROL_CLASS}
        >
          <option value="">Any status</option>
          {(['RUNNING', 'COMPLETED', 'FAILED'] as const).map((status) => (
            <option key={status} value={status.toLowerCase()}>
              {STATUS_TONE[status].label}
            </option>
          ))}
        </select>
        <div className="inline-flex h-7 items-center gap-1.5 rounded-md border border-[var(--c-border)] bg-[var(--c-app-bg)] px-2">
          <Clock3 aria-hidden="true" className="h-3.5 w-3.5 text-[var(--c-text-muted)]" />
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
            className={DATE_INPUT_CLASS}
          />
          <span className="text-[11px] text-[var(--c-text-muted)]">–</span>
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
            className={DATE_INPUT_CLASS}
          />
        </div>
        <button
          type="button"
          aria-expanded={advancedOpen}
          aria-controls={ADVANCED_PANEL_ID}
          onClick={() => setAdvancedOpen((open) => !open)}
          className={`inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)] ${
            advancedOpen || advancedFilterCount > 0
              ? 'border-[var(--c-accent-border)] bg-[var(--c-accent-faint)] text-[var(--c-accent-text)]'
              : 'border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-primary)]'
          }`}
        >
          <SlidersHorizontal aria-hidden="true" className="h-3.5 w-3.5" />
          Advanced
          <span className="rounded-[3px] border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-1 font-mono text-[10px] text-[var(--c-text-muted)]">
            {advancedFilterCount > 0 ? `${advancedFilterCount} active` : 'engine · projection'}
          </span>
        </button>
      </FilterBar>

      {advancedOpen ? (
        <TracesAdvancedPanel
          engineDefinitions={engineDefinitionItems}
          engineInstanceKey={{
            value: engineInstanceKeyDraft,
            onChange: setEngineInstanceKeyDraft,
            onKeyDown: (event) => commitOnEnter(event, commitEngineInstanceKey),
          }}
          filters={filters}
          id={ADVANCED_PANEL_ID}
          minDuration={{
            value: minDurationDraft,
            onChange: setMinDurationDraft,
            onKeyDown: (event) => commitOnEnter(event, commitMinDuration),
          }}
          setFilters={setFilters}
          userId={{
            value: userIdDraft,
            onChange: setUserIdDraft,
            onKeyDown: (event) => commitOnEnter(event, commitUserId),
          }}
        />
      ) : null}

      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-[var(--c-border)] px-6 py-2 text-[12.5px] text-[var(--c-text-secondary)]">
        <span className="font-medium text-[var(--c-text-primary)]">
          {tracesQuery.data
            ? hasActiveFilters
              ? `${total} ${total === 1 ? 'trace matches' : 'traces match'}`
              : `${total} ${total === 1 ? 'trace' : 'traces'}`
            : '—'}
        </span>
        {tracesQuery.data && traces.length > 0 && traces.length < total ? (
          <span className="text-[var(--c-text-muted)]">{traces.length} on this page</span>
        ) : null}
        {filters.q ? (
          <span className="border-l border-[var(--c-border)] pl-3 text-[var(--c-text-muted)]">
            search checks trace names and IDs, user IDs, session IDs, and step names
          </span>
        ) : null}
        {activeChips.map((chip) => (
          <Chip
            key={chip.key}
            closeLabel={`Clear ${chip.label} filter`}
            onClose={() => clearChip(chip.key)}
          >
            <span>{chip.label}:</span> <span>{chip.value}</span>
          </Chip>
        ))}
        {hasActiveFilters ? (
          <button
            type="button"
            className="text-xs font-medium text-[var(--c-text-muted)] hover:text-[var(--c-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]"
            onClick={clearAll}
          >
            Clear ({activeChips.length})
          </button>
        ) : null}
        <span className="ml-auto flex items-center gap-3 text-[11.5px] text-[var(--c-text-muted)]">
          {tracesQuery.isFetching && !tracesQuery.isPending ? <span>Refreshing…</span> : null}
          {location.search ? (
            <span
              className="hidden max-w-[320px] items-center gap-1 truncate font-mono md:inline-flex"
              title="These filters are saved in the page URL."
            >
              <Link2 aria-hidden="true" className="h-3 w-3 shrink-0" />
              <span className="truncate">{location.search}</span>
            </span>
          ) : null}
        </span>
      </div>

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
              <col />
              <col className="w-[120px]" />
              <col className="w-[104px]" />
              <col className="w-[104px]" />
              <col className="w-[180px]" />
            </colgroup>
            <thead>
              <tr>
                <Th>Trace</Th>
                <Th>Status</Th>
                <Th align="right">Failed steps</Th>
                <Th align="right">Duration</Th>
                <Th
                  align="right"
                  sortable={!filters.q}
                  sortActive={filters.sort_by === 'started_at'}
                  sortDir={filters.sort_dir}
                  onSort={handleStartedSortToggle}
                >
                  Started
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
            <HonestyNote className="mb-1.5" kind="derived">
              Start times are exact so runs on the same day stay distinguishable. Durations are
              derived from recorded start and end times.
            </HonestyNote>
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
  const tracePath = appendProjectToPath(`/traces/${trace.id}`, projectId);
  const failedSteps = trace.error_count ?? 0;

  return (
    <Tr className="hover:bg-[var(--c-row-hover-bg)]">
      <Td>
        <article className="flex min-w-0 items-center gap-2">
          <Link
            aria-label={trace.name}
            to={tracePath}
            state={{ returnTo }}
            className="min-w-0 truncate font-mono text-[12.5px] font-medium text-[var(--c-text-primary)] hover:text-[var(--c-accent-text)]"
          >
            {trace.name}
          </Link>
          <span
            className="shrink-0 font-mono text-[11px] text-[var(--c-text-muted)]"
            title={trace.id}
          >
            {shortTraceId(trace.id)}
          </span>
          {trace.engine ? (
            <Chip icon={Zap}>{trace.engine.definition_name}</Chip>
          ) : null}
        </article>
      </Td>
      <Td>
        <StatusDot status={trace.status} />
      </Td>
      <Td
        align="right"
        className={failedSteps > 0 ? 'text-[var(--c-red-text)]' : 'text-[var(--c-text-muted)]'}
        mono
      >
        {failedSteps}
      </Td>
      <Td align="right" mono>
        {trace.ended_at ? (
          <span title="Derived from recorded start and end times.">
            {formatDerivedDuration(calculateDuration(trace.started_at, trace.ended_at))}
          </span>
        ) : (
          <span className="font-sans text-[12px] text-[var(--c-text-muted)]">running</span>
        )}
      </Td>
      <Td align="right" dim mono>
        <span title={trace.started_at}>{formatExactTimeWithSeconds(trace.started_at)}</span>
      </Td>
    </Tr>
  );
}
