import { keepPreviousData, useQuery } from '@tanstack/react-query';
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useState,
  type KeyboardEvent,
} from 'react';
import { Link, useLocation } from 'react-router-dom';
import { PanelLeftClose, PanelLeftOpen } from 'lucide-react';
import { fetchTraces, isAuthError, type Trace } from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { EngineBadge } from '../components/EngineBadge';
import { PaginationControls } from '../components/PaginationControls';
import { SortableHeader } from '../components/SortableHeader';
import { StatusBadge } from '../components/StatusBadge';
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

function handleEnterCommit(
  event: KeyboardEvent<HTMLInputElement>,
  onCommit: (value: string) => void,
  value: string
) {
  if (event.key !== 'Enter') {
    return;
  }

  event.preventDefault();
  onCommit(value);
}

function useDebouncedDraftCommit({
  draftValue,
  committedValue,
  onCommit,
  isActive,
  normalizeForComparison,
}: {
  draftValue: string;
  committedValue: string;
  onCommit: (value: string) => void;
  isActive: boolean;
  normalizeForComparison: (value: string) => string;
}) {
  useEffect(() => {
    if (!isActive || normalizeForComparison(draftValue) === committedValue) {
      return;
    }

    const timeoutId = window.setTimeout(() => {
      onCommit(draftValue);
    }, DEBOUNCE_MS);

    return () => {
      window.clearTimeout(timeoutId);
    };
  }, [committedValue, draftValue, isActive, normalizeForComparison, onCommit]);
}

function useDraftFocus() {
  const [isFocused, setIsFocused] = useState(false);

  return {
    isFocused,
    onFocus: useCallback(() => setIsFocused(true), []),
    onBlur: useCallback(() => setIsFocused(false), []),
  };
}

/**
 * Traces list page with URL-driven filters and pagination.
 */
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
  const [engineDefinitionNameDraft, setEngineDefinitionNameDraft] = useState(
    filters.engine_definition_name ?? ''
  );
  const [minDurationDraft, setMinDurationDraft] = useState(
    filters.min_duration_ms?.toString() ?? ''
  );
  const searchFocus = useDraftFocus();
  const userIdFocus = useDraftFocus();
  const engineInstanceKeyFocus = useDraftFocus();
  const engineDefinitionNameFocus = useDraftFocus();
  const minDurationFocus = useDraftFocus();

  useEffect(() => {
    setSearchDraft(filters.q ?? '');
  }, [filters.q]);

  useEffect(() => {
    setUserIdDraft(filters.user_id ?? '');
  }, [filters.user_id]);

  useEffect(() => {
    setEngineInstanceKeyDraft(filters.engine_instance_key ?? '');
  }, [filters.engine_instance_key]);

  useEffect(() => {
    setEngineDefinitionNameDraft(filters.engine_definition_name ?? '');
  }, [filters.engine_definition_name]);

  useEffect(() => {
    setMinDurationDraft(filters.min_duration_ms?.toString() ?? '');
  }, [filters.min_duration_ms]);

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

  const commitEngineDefinitionName = useCallback(
    (value: string) => {
      const normalizedValue = normalizeTrimmedDraft(value);
      setEngineDefinitionNameDraft(normalizedValue);
      setFilters(
        { engine_definition_name: normalizedValue || undefined },
        'replace'
      );
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

  useDebouncedDraftCommit({
    draftValue: searchDraft,
    committedValue: filters.q ?? '',
    onCommit: commitSearch,
    isActive: searchFocus.isFocused,
    normalizeForComparison: normalizeTrimmedDraft,
  });

  useDebouncedDraftCommit({
    draftValue: userIdDraft,
    committedValue: filters.user_id ?? '',
    onCommit: commitUserId,
    isActive: userIdFocus.isFocused,
    normalizeForComparison: normalizeTrimmedDraft,
  });

  useDebouncedDraftCommit({
    draftValue: engineInstanceKeyDraft,
    committedValue: filters.engine_instance_key ?? '',
    onCommit: commitEngineInstanceKey,
    isActive: engineInstanceKeyFocus.isFocused,
    normalizeForComparison: normalizeTrimmedDraft,
  });

  useDebouncedDraftCommit({
    draftValue: engineDefinitionNameDraft,
    committedValue: filters.engine_definition_name ?? '',
    onCommit: commitEngineDefinitionName,
    isActive: engineDefinitionNameFocus.isFocused,
    normalizeForComparison: normalizeTrimmedDraft,
  });

  useDebouncedDraftCommit({
    draftValue: minDurationDraft,
    committedValue: filters.min_duration_ms?.toString() ?? '',
    onCommit: commitMinDuration,
    isActive: minDurationFocus.isFocused,
    normalizeForComparison: normalizeDurationDraft,
  });

  const startDate = isoToLocalDateInputValue(filters.start_time_from);
  const endDate = isoToLocalDateInputValue(filters.start_time_to);
  const dateRangeError = getDateRangeError(startDate, endDate);
  const activeChips = deriveActiveChips(filters);
  const hasActiveFilters = activeChips.length > 0;
  const hasActiveEngineFilters = Boolean(
    filters.engine_instance_key ||
      filters.engine_definition_name ||
      filters.engine_run_status ||
      filters.engine_projection_state
  );
  const [areEngineFiltersExpanded, setAreEngineFiltersExpanded] = useState(
    hasActiveEngineFilters
  );
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const isSearchActive = Boolean(filters.q);
  const queryParams = { ...filters };
  const canonicalQueryString = buildCanonicalQueryString(queryParams);
  const tracesQuery = useQuery({
    queryKey: ['traces', canonicalQueryString],
    queryFn: () => fetchTraces(queryParams),
    enabled: !dateRangeError,
    placeholderData: keepPreviousData,
  });

  const traces = tracesQuery.data?.traces ?? EMPTY_TRACES;
  const total = tracesQuery.data?.total ?? 0;
  const currentListUrl = `${location.pathname}${location.search}`;

  useLayoutEffect(() => {
    if (hasActiveEngineFilters) {
      setAreEngineFiltersExpanded(true);
    }
  }, [hasActiveEngineFilters]);

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
    if (isSearchActive) {
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
  }, [filters.sort_by, filters.sort_dir, isSearchActive, setFilters]);

  return (
    <div className="flex flex-col gap-6 lg:flex-row lg:items-start">
      <aside
        className={`w-full shrink-0 transition-[width] duration-200 lg:sticky lg:top-[5rem] lg:self-start lg:max-h-[calc(100vh-6rem)] lg:overflow-y-auto ${
          sidebarCollapsed ? 'lg:w-14' : 'lg:w-80'
        }`}
      >
        {sidebarCollapsed ? (
          <div className="app-surface hidden flex-col items-center gap-4 px-2 py-4 lg:flex">
            <button
              type="button"
              aria-label="Expand filters"
              onClick={() => setSidebarCollapsed(false)}
              className="rounded-full p-2 text-[var(--continua-text-secondary)] transition hover:bg-[var(--continua-surface-muted)] hover:text-[var(--continua-text-primary)]"
            >
              <PanelLeftOpen className="h-5 w-5" />
            </button>
            <span
              className="text-[10px] font-black uppercase tracking-widest text-[var(--continua-text-muted)]"
              style={{ writingMode: 'vertical-rl' }}
            >
              Filters{activeChips.length ? ` · ${activeChips.length}` : ''}
            </span>
          </div>
        ) : (
          <section className="app-surface p-4 sm:p-5">
            <div className="mb-4 flex items-center justify-between">
              <h2 className="app-overline">Filters</h2>
              <button
                type="button"
                aria-label="Collapse filters"
                onClick={() => setSidebarCollapsed(true)}
                className="hidden rounded-full p-1.5 text-[var(--continua-text-muted)] transition hover:bg-[var(--continua-surface-muted)] hover:text-[var(--continua-text-primary)] lg:inline-flex"
              >
                <PanelLeftClose className="h-4 w-4" />
              </button>
            </div>

            <div className="flex flex-col gap-4">
              <div>
                <label htmlFor="trace-search" className="mb-1 block text-sm font-medium text-[var(--continua-text-secondary)]">
                  Search
                </label>
                <input
                  id="trace-search"
                  type="text"
                  value={searchDraft}
                  onChange={(event) => setSearchDraft(event.target.value)}
                  onFocus={searchFocus.onFocus}
                  onBlur={searchFocus.onBlur}
                  onKeyDown={(event) => handleEnterCommit(event, commitSearch, searchDraft)}
                  aria-describedby="trace-search-hint"
                  placeholder="Search traces"
                  className="app-input"
                />
                <p id="trace-search-hint" className="mt-1 text-xs text-[var(--continua-text-muted)]">
                  Search names, user IDs, and matching span names.
                </p>
              </div>

              <div>
                <label htmlFor="trace-status" className="mb-1 block text-sm font-medium text-[var(--continua-text-secondary)]">
                  Status
                </label>
                <select
                  id="trace-status"
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
                  className="app-select"
                >
                  <option value="">All statuses</option>
                  <option value="running">Running</option>
                  <option value="completed">Completed</option>
                  <option value="failed">Failed</option>
                </select>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label htmlFor="trace-start-date" className="mb-1 block text-sm font-medium text-[var(--continua-text-secondary)]">
                    Start Date
                  </label>
                  <input
                    id="trace-start-date"
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
                    className="app-input"
                  />
                </div>

                <div>
                  <label htmlFor="trace-end-date" className="mb-1 block text-sm font-medium text-[var(--continua-text-secondary)]">
                    End Date
                  </label>
                  <input
                    id="trace-end-date"
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
                    className="app-input"
                  />
                </div>
              </div>

              <div>
                <label htmlFor="trace-user-id" className="mb-1 block text-sm font-medium text-[var(--continua-text-secondary)]">
                  User ID
                </label>
                <input
                  id="trace-user-id"
                  type="text"
                  value={userIdDraft}
                  onChange={(event) => setUserIdDraft(event.target.value)}
                  onFocus={userIdFocus.onFocus}
                  onBlur={userIdFocus.onBlur}
                  onKeyDown={(event) => handleEnterCommit(event, commitUserId, userIdDraft)}
                  placeholder="Filter by user"
                  className="app-input"
                />
              </div>

              <div>
                <label htmlFor="trace-min-duration" className="mb-1 block text-sm font-medium text-[var(--continua-text-secondary)]">
                  Min Duration (ms)
                </label>
                <input
                  id="trace-min-duration"
                  type="number"
                  min="1"
                  step="1"
                  value={minDurationDraft}
                  onChange={(event) => setMinDurationDraft(event.target.value)}
                  onFocus={minDurationFocus.onFocus}
                  onBlur={minDurationFocus.onBlur}
                  onKeyDown={(event) => handleEnterCommit(event, commitMinDuration, minDurationDraft)}
                  placeholder="e.g. 1500"
                  className="app-input"
                />
              </div>

              <label className="flex items-center gap-2 text-sm font-medium text-[var(--continua-text-secondary)]">
                <input
                  type="checkbox"
                  checked={Boolean(filters.has_errors)}
                  onChange={(event) =>
                    setFilters(
                      {
                        has_errors: event.target.checked ? true : undefined,
                      },
                      'push'
                    )
                  }
                  aria-label="Only show traces with errors"
                  className="h-4 w-4 rounded border-[var(--continua-border-strong)] text-[var(--continua-accent)] focus:ring-2 focus:ring-[var(--continua-accent-faint)]"
                />
                <span>Has errors only</span>
              </label>
            </div>

            <div className="mt-4 rounded-[1rem] border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] p-3">
              <div className="flex items-center justify-between gap-2">
                <h3 className="app-overline text-[var(--continua-text-secondary)]">Engine filters</h3>
                <button
                  type="button"
                  aria-controls="trace-engine-filters"
                  aria-expanded={areEngineFiltersExpanded}
                  onClick={() => setAreEngineFiltersExpanded((expanded) => !expanded)}
                  className="rounded-full px-2.5 py-1 text-xs font-semibold text-[var(--continua-text-secondary)] transition hover:bg-[var(--continua-surface-elevated)] hover:text-[var(--continua-text-primary)]"
                >
                  {areEngineFiltersExpanded ? 'Hide engine filters' : 'Show engine filters'}
                </button>
              </div>
              <p className="mt-2 text-xs leading-6 text-[var(--continua-text-muted)]">
                Advanced operator filter for inspecting projection health across
                engine traces.
              </p>

              {areEngineFiltersExpanded ? (
                <div id="trace-engine-filters" className="mt-3 flex flex-col gap-3">
                  <div>
                    <label htmlFor="trace-engine-instance-key" className="mb-1 block text-xs font-medium text-[var(--continua-text-secondary)]">
                      Engine Instance Key
                    </label>
                    <input
                      id="trace-engine-instance-key"
                      type="text"
                      value={engineInstanceKeyDraft}
                      onChange={(event) => setEngineInstanceKeyDraft(event.target.value)}
                      onFocus={engineInstanceKeyFocus.onFocus}
                      onBlur={engineInstanceKeyFocus.onBlur}
                      onKeyDown={(event) => handleEnterCommit(event, commitEngineInstanceKey, engineInstanceKeyDraft)}
                      placeholder="Filter by instance key"
                      className="app-input"
                    />
                  </div>

                  <div>
                    <label htmlFor="trace-engine-definition-name" className="mb-1 block text-xs font-medium text-[var(--continua-text-secondary)]">
                      Engine Definition Name
                    </label>
                    <input
                      id="trace-engine-definition-name"
                      type="text"
                      value={engineDefinitionNameDraft}
                      onChange={(event) => setEngineDefinitionNameDraft(event.target.value)}
                      onFocus={engineDefinitionNameFocus.onFocus}
                      onBlur={engineDefinitionNameFocus.onBlur}
                      onKeyDown={(event) => handleEnterCommit(event, commitEngineDefinitionName, engineDefinitionNameDraft)}
                      placeholder="Filter by definition name"
                      className="app-input"
                    />
                  </div>

                  <div>
                    <label htmlFor="trace-engine-run-status" className="mb-1 block text-xs font-medium text-[var(--continua-text-secondary)]">
                      Engine Status
                    </label>
                    <select
                      id="trace-engine-run-status"
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
                      className="app-select"
                    >
                      <option value="">All engine statuses</option>
                      {ENGINE_RUN_STATUS_FILTER_VALUES.map((value) => (
                        <option key={value} value={value}>
                          {formatEngineRunStatusLabel(value)}
                        </option>
                      ))}
                    </select>
                  </div>

                  <div>
                    <label htmlFor="trace-engine-projection-state" className="mb-1 block text-xs font-medium text-[var(--continua-text-secondary)]">
                      Projection State
                    </label>
                    <select
                      id="trace-engine-projection-state"
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
                      className="app-select"
                    >
                      <option value="">All projection states</option>
                      {ENGINE_PROJECTION_STATE_FILTER_VALUES.map((value) => (
                        <option key={value} value={value}>
                          {formatEngineProjectionStateLabel(value)}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>
              ) : null}
            </div>

            {dateRangeError && (
              <p className="mt-3 text-sm font-medium text-[var(--continua-error)]">{dateRangeError}</p>
            )}
          </section>
        )}
      </aside>

      <div className="flex min-w-0 flex-1 flex-col gap-6">
        <section className="app-surface p-6 sm:p-7">
          <div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
            <div className="max-w-3xl">
              <div className="app-overline">Trace triage</div>
              <h1 className="mt-3 text-3xl font-black tight-headline text-[var(--continua-text-primary)] sm:text-4xl">
                Find the run, isolate the failure, and jump straight into the workspace.
              </h1>
              <p className="mt-3 text-sm leading-7 text-[var(--continua-text-secondary)] sm:text-base">
                Search names, user IDs, and matching span names.
              </p>
            </div>

            <div className="grid gap-3 sm:grid-cols-3">
              <TraceStat label="Tracked" value={String(total)} />
              <TraceStat label="Live" value={String(runningTracesCount(filters.status, traces, tracesQuery.data?.total))} />
              <TraceStat
                label="Failures"
                value={
                  filters.status === 'failed'
                    ? String(total)
                    : String(traces.filter((trace) => (trace.error_count ?? 0) > 0).length)
                }
              />
            </div>
          </div>
        </section>

        <div className="flex flex-wrap items-center gap-2">
          <QuickFilterButton
            active={!filters.status}
            label="All traces"
            onClick={() => setFilters({ status: undefined }, 'push')}
          />
          <QuickFilterButton
            active={filters.status === 'failed'}
            label="Failed"
            onClick={() => setFilters({ status: filters.status === 'failed' ? undefined : 'failed' }, 'push')}
          />
          <QuickFilterButton
            active={filters.status === 'running'}
            label="Running"
            onClick={() => setFilters({ status: filters.status === 'running' ? undefined : 'running' }, 'push')}
          />
          <QuickFilterButton
            active={Boolean(filters.has_errors)}
            label="Has errors"
            onClick={() => setFilters({ has_errors: filters.has_errors ? undefined : true }, 'push')}
          />
          <div className="ml-auto text-sm text-[var(--continua-text-muted)]">
            <span>{total} total</span>
            {tracesQuery.isFetching && !tracesQuery.isPending ? (
              <span className="ml-2">Refreshing…</span>
            ) : null}
          </div>
        </div>

        {hasActiveFilters && (
          <div className="app-surface p-4">
            <div className="flex flex-wrap items-center gap-2">
              {activeChips.map((chip) => (
                <span
                  key={chip.key}
                  className="inline-flex items-center gap-2 rounded-full border border-[var(--continua-border-strong)] bg-[var(--continua-surface-muted)] px-3 py-1.5 text-sm text-[var(--continua-text-secondary)]"
                >
                  <span className="font-medium">{chip.label}:</span>
                  <span>{chip.value}</span>
                  <button
                    type="button"
                    onClick={() => clearChip(chip.key)}
                    aria-label={`Clear ${chip.label} filter`}
                    className="rounded-full p-0.5 text-[var(--continua-text-muted)] transition hover:bg-[var(--continua-surface-elevated)] hover:text-[var(--continua-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)]"
                  >
                    ×
                  </button>
                </span>
              ))}
              <button
                type="button"
                onClick={clearAll}
                className="app-button-secondary ml-auto"
              >
                Clear all
              </button>
            </div>
          </div>
        )}

        {tracesQuery.error && (
          isAuthError(tracesQuery.error) ? (
            <AuthErrorBanner message={getErrorMessage(tracesQuery.error)} />
          ) : (
            <div className="app-alert-error flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <p className="font-semibold">Could not load traces</p>
                <p className="text-sm">{getErrorMessage(tracesQuery.error)}</p>
              </div>
              <button
                type="button"
                onClick={() => void tracesQuery.refetch()}
                className="app-button-primary"
              >
                Retry
              </button>
            </div>
          )
        )}

        {dateRangeError ? (
          <div className="app-empty-state">Fix the date range to load traces.</div>
        ) : tracesQuery.isPending && !tracesQuery.data ? (
          <div className="app-empty-state">Loading traces...</div>
        ) : tracesQuery.error && !tracesQuery.data ? (
          <div className="app-empty-state">Retry the request or adjust your filters to continue.</div>
        ) : traces.length === 0 ? (
          hasActiveFilters ? (
            <div className="app-empty-state">
              <h2 className="text-lg font-black tight-headline text-[var(--continua-text-primary)]">No matching traces</h2>
              <p className="mt-2">Try broadening the filters or clearing them entirely.</p>
            </div>
          ) : (
            <div className="app-empty-state">
              <h2 className="text-lg font-black tight-headline text-[var(--continua-text-primary)]">No traces yet</h2>
              <p className="mt-2">Start sending traces from your application to see them here.</p>
            </div>
          )
        ) : (
          <>
            <section className="app-surface overflow-hidden">
              <div className="flex items-center justify-between border-b border-[var(--continua-border-soft)] px-4 py-3 sm:px-5">
                <div className="flex items-center gap-4 app-overline">
                  <span>Name</span>
                  <span>Status</span>
                  <span>Duration</span>
                  <span>Tokens</span>
                  <span>Cost</span>
                </div>
                <SortableHeader
                  label="Started"
                  isActive={filters.sort_by === 'started_at'}
                  isAscending={filters.sort_dir === 'asc'}
                  isDisabled={isSearchActive}
                  onClick={handleStartedSortToggle}
                />
              </div>

              <div className="space-y-3 p-4 sm:p-5">
                {traces.map((trace) => (
                  <TraceRow
                    key={trace.id}
                    projectId={filters.project_id}
                    trace={trace}
                    returnTo={currentListUrl}
                  />
                ))}
              </div>
            </section>

            <PaginationControls
              offset={filters.offset}
              pageSize={filters.limit ?? DEFAULT_PAGE_SIZE}
              total={total}
              currentItemCount={traces.length}
              onOffsetChange={(offset) => setFilters({ offset }, 'push')}
              onPageSizeChange={(limit) => setFilters({ limit }, 'push')}
              onRepairOffset={(offset) => setFilters({ offset }, 'replace')}
            />
          </>
        )}
      </div>
    </div>
  );
}

interface TraceRowProps {
  projectId?: string;
  trace: Trace;
  returnTo: string;
}

function TraceRow({ projectId, trace, returnTo }: TraceRowProps) {
  const duration = calculateDuration(trace.started_at, trace.ended_at);
  const totalTokens = (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);

  return (
    <article className="app-list-row">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <Link
            to={appendProjectToPath(`/traces/${trace.id}`, projectId)}
            state={{ returnTo }}
            className="truncate text-sm font-semibold text-[var(--continua-text-primary)] transition hover:text-[var(--continua-accent)] focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)]"
          >
            {trace.name}
          </Link>
          {trace.engine ? (
            <EngineBadge projectionState={trace.engine.projection_state} />
          ) : null}
          <StatusBadge status={trace.status} />
          {trace.error_count && trace.error_count > 0 ? (
            <span className="inline-flex rounded-full border border-[var(--continua-error-border)] bg-[var(--continua-error-faint)] px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--continua-error)]">
              {trace.error_count} error{trace.error_count === 1 ? '' : 's'}
            </span>
          ) : null}
        </div>

        <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--continua-text-muted)]">
          <span>{formatRelativeTime(trace.started_at)}</span>
          {trace.session_id ? (
            <Link
              to={appendProjectToPath(`/sessions/${trace.session_id}`, projectId)}
              className="inline-flex items-center gap-2 rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-2.5 py-1 text-[11px] font-medium text-[var(--continua-text-secondary)] transition hover:border-[var(--continua-border-strong)] hover:text-[var(--continua-accent)]"
            >
              <span>{trace.session_external_id ?? trace.session_id}</span>
            </Link>
          ) : null}
        </div>
      </div>

      <div className="grid shrink-0 gap-3 text-right text-xs sm:grid-cols-4 sm:text-sm">
        <MetricColumn label="Duration" value={formatDuration(duration)} />
        <MetricColumn label="Tokens" value={formatTokens(totalTokens)} />
        <MetricColumn label="Cost" value={formatCost(trace.total_cost_usd)} />
        <MetricColumn
          label="Errors"
          value={trace.error_count && trace.error_count > 0 ? String(trace.error_count) : '0'}
        />
      </div>
    </article>
  );
}

function MetricColumn({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-[5.25rem]">
      <div className="text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--continua-text-muted)]">
        {label}
      </div>
      <div className="mt-1 text-xs font-medium text-[var(--continua-text-primary)] sm:text-sm">
        {value}
      </div>
    </div>
  );
}

function TraceStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="app-surface-muted px-4 py-3">
      <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
        {label}
      </div>
      <div className="mt-2 text-2xl font-black tight-headline text-[var(--continua-text-primary)]">
        {value}
      </div>
    </div>
  );
}

function QuickFilterButton({
  active,
  label,
  onClick,
}: {
  active: boolean;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        active
          ? 'inline-flex items-center rounded-full border border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] px-3 py-1.5 text-sm font-medium text-[var(--continua-accent)]'
          : 'app-button-ghost'
      }
    >
      {label}
    </button>
  );
}

function runningTracesCount(
  status: 'running' | 'completed' | 'failed' | undefined,
  traces: Trace[],
  total?: number
) {
  if (status === 'running') {
    return total ?? traces.length;
  }

  return traces.filter((trace) => trace.status === 'RUNNING').length;
}
