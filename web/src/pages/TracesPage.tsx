import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useState, type KeyboardEvent } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { fetchTraces, type Trace } from '../api/client';
import { PaginationControls } from '../components/PaginationControls';
import { StatusBadge } from '../components/StatusBadge';
import { useRequireApiKey } from '../hooks/useRequireApiKey';
import { useTracesSearchParams } from '../hooks/useTracesSearchParams';
import {
  buildCanonicalQueryString,
  deriveActiveChips,
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

const PAGE_SIZE = 20;
const DEBOUNCE_MS = 300;

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
  const { hasApiKey, prompt } = useRequireApiKey();

  if (!hasApiKey) {
    return prompt;
  }

  return <TracesContent />;
}

function TracesContent() {
  const location = useLocation();
  const { filters, setFilters, clearAll, clearChip } = useTracesSearchParams();
  const [searchDraft, setSearchDraft] = useState(filters.q ?? '');
  const [userIdDraft, setUserIdDraft] = useState(filters.user_id ?? '');
  const [minDurationDraft, setMinDurationDraft] = useState(
    filters.min_duration_ms?.toString() ?? ''
  );
  const searchFocus = useDraftFocus();
  const userIdFocus = useDraftFocus();
  const minDurationFocus = useDraftFocus();

  useEffect(() => {
    setSearchDraft(filters.q ?? '');
  }, [filters.q]);

  useEffect(() => {
    setUserIdDraft(filters.user_id ?? '');
  }, [filters.user_id]);

  useEffect(() => {
    setMinDurationDraft(filters.min_duration_ms?.toString() ?? '');
  }, [filters.min_duration_ms]);

  const commitSearch = useCallback(
    (value: string) => {
      const normalizedValue = normalizeTrimmedDraft(value);
      setSearchDraft(normalizedValue);
      setFilters({ q: normalizedValue || undefined }, 'push');
    },
    [setFilters]
  );

  const commitUserId = useCallback(
    (value: string) => {
      const normalizedValue = normalizeTrimmedDraft(value);
      setUserIdDraft(normalizedValue);
      setFilters({ user_id: normalizedValue || undefined }, 'push');
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
        'push'
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
  const queryParams = {
    ...filters,
    limit: PAGE_SIZE,
  };
  const canonicalQueryString = buildCanonicalQueryString(queryParams);
  const tracesQuery = useQuery({
    queryKey: ['traces', canonicalQueryString],
    queryFn: () => fetchTraces(queryParams),
    enabled: !dateRangeError,
    placeholderData: keepPreviousData,
  });

  const traces = tracesQuery.data?.traces ?? [];
  const total = tracesQuery.data?.total ?? 0;
  const currentListUrl = `${location.pathname}${location.search}`;

  useEffect(() => {
    if (traces.length !== 0 || total === 0 || filters.offset === 0) {
      return;
    }

    const lastValidOffset = Math.floor((total - 1) / PAGE_SIZE) * PAGE_SIZE;
    if (lastValidOffset !== filters.offset) {
      setFilters({ offset: lastValidOffset }, 'replace');
    }
  }, [filters.offset, setFilters, total, traces.length]);

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
        <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-2xl font-bold text-gray-900">Traces</h1>
            <p className="mt-1 text-sm text-gray-500">
              Find traces by name, owner, time window, or error signals.
            </p>
          </div>
          <div className="text-sm text-gray-500">
            <span>{total} total</span>
            {tracesQuery.isFetching && !tracesQuery.isPending && (
              <span className="ml-2 text-blue-600">Updating...</span>
            )}
          </div>
        </div>

        <section className="mb-4 rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-6">
            <div className="xl:col-span-2">
              <label
                htmlFor="trace-search"
                className="mb-1 block text-sm font-medium text-gray-700"
              >
                Search
              </label>
              <input
                id="trace-search"
                type="text"
                value={searchDraft}
                onChange={(event) => setSearchDraft(event.target.value)}
                onFocus={searchFocus.onFocus}
                onBlur={searchFocus.onBlur}
                onKeyDown={(event) =>
                  handleEnterCommit(event, commitSearch, searchDraft)
                }
                aria-describedby="trace-search-hint"
                placeholder="Search traces"
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 shadow-sm transition focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-200"
              />
              <p id="trace-search-hint" className="mt-1 text-xs text-gray-500">
                Search names, user IDs, and matching span names.
              </p>
            </div>

            <div>
              <label
                htmlFor="trace-status"
                className="mb-1 block text-sm font-medium text-gray-700"
              >
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
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 shadow-sm transition focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-200"
              >
                <option value="">All statuses</option>
                <option value="running">Running</option>
                <option value="completed">Completed</option>
                <option value="failed">Failed</option>
              </select>
            </div>

            <div>
              <label
                htmlFor="trace-start-date"
                className="mb-1 block text-sm font-medium text-gray-700"
              >
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
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 shadow-sm transition focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-200"
              />
            </div>

            <div>
              <label
                htmlFor="trace-end-date"
                className="mb-1 block text-sm font-medium text-gray-700"
              >
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
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 shadow-sm transition focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-200"
              />
            </div>

            <div>
              <label
                htmlFor="trace-user-id"
                className="mb-1 block text-sm font-medium text-gray-700"
              >
                User ID
              </label>
              <input
                id="trace-user-id"
                type="text"
                value={userIdDraft}
                onChange={(event) => setUserIdDraft(event.target.value)}
                onFocus={userIdFocus.onFocus}
                onBlur={userIdFocus.onBlur}
                onKeyDown={(event) =>
                  handleEnterCommit(event, commitUserId, userIdDraft)
                }
                placeholder="Filter by user"
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 shadow-sm transition focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-200"
              />
            </div>

            <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_auto] xl:grid-cols-1 xl:gap-2">
              <div>
                <label
                  htmlFor="trace-min-duration"
                  className="mb-1 block text-sm font-medium text-gray-700"
                >
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
                  onKeyDown={(event) =>
                    handleEnterCommit(event, commitMinDuration, minDurationDraft)
                  }
                  placeholder="e.g. 1500"
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 shadow-sm transition focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-200"
                />
              </div>

              <label className="flex items-end gap-2 text-sm font-medium text-gray-700 xl:pt-0">
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
                  className="mt-0 h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-2 focus:ring-blue-200"
                />
                <span>Has errors</span>
              </label>
            </div>
          </div>

          {dateRangeError && (
            <p className="mt-3 text-sm font-medium text-red-600">{dateRangeError}</p>
          )}
        </section>

        {hasActiveFilters && (
          <div className="mb-4 rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
            <div className="flex flex-wrap items-center gap-2">
              {activeChips.map((chip) => (
                <span
                  key={chip.key}
                  className="inline-flex items-center gap-2 rounded-full border border-gray-200 bg-gray-50 px-3 py-1 text-sm text-gray-700"
                >
                  <span className="font-medium">{chip.label}:</span>
                  <span>{chip.value}</span>
                  <button
                    type="button"
                    onClick={() => clearChip(chip.key)}
                    aria-label={`Clear ${chip.label} filter`}
                    className="rounded-full p-0.5 text-gray-500 transition hover:bg-gray-200 hover:text-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-200"
                  >
                    ×
                  </button>
                </span>
              ))}
              <button
                type="button"
                onClick={clearAll}
                className="ml-auto rounded-lg border border-gray-300 px-3 py-1.5 text-sm font-medium text-gray-700 transition hover:border-gray-400 hover:text-gray-900 focus:outline-none focus:ring-2 focus:ring-blue-200"
              >
                Clear all
              </button>
            </div>
          </div>
        )}

        {tracesQuery.error && (
          <div className="mb-4 flex flex-col gap-3 rounded-xl border border-red-200 bg-red-50 p-4 text-red-700 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p className="font-semibold">Could not load traces</p>
              <p className="text-sm">{getErrorMessage(tracesQuery.error)}</p>
            </div>
            <button
              type="button"
              onClick={() => void tracesQuery.refetch()}
              className="rounded-lg bg-red-600 px-3 py-2 text-sm font-medium text-white transition hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-200"
            >
              Retry
            </button>
          </div>
        )}

        {dateRangeError ? (
          <div className="rounded-xl border border-dashed border-red-200 bg-white p-8 text-center text-sm text-gray-600 shadow-sm">
            Fix the date range to load traces.
          </div>
        ) : tracesQuery.isPending && !tracesQuery.data ? (
          <div className="rounded-xl border border-gray-200 bg-white p-8 text-center text-gray-500 shadow-sm">
            Loading traces...
          </div>
        ) : tracesQuery.error && !tracesQuery.data ? (
          <div className="rounded-xl border border-gray-200 bg-white p-8 text-center text-gray-600 shadow-sm">
            Retry the request or adjust your filters to continue.
          </div>
        ) : traces.length === 0 ? (
          hasActiveFilters ? (
            <div className="rounded-xl border border-gray-200 bg-white p-8 text-center shadow-sm">
              <h2 className="text-lg font-semibold text-gray-900">No matching traces</h2>
              <p className="mt-2 text-sm text-gray-500">
                Try broadening the filters or clearing them entirely.
              </p>
            </div>
          ) : (
            <div className="rounded-xl border border-gray-200 bg-white p-8 text-center shadow-sm">
              <h2 className="text-lg font-semibold text-gray-900">No traces yet</h2>
              <p className="mt-2 text-sm text-gray-500">
                Start sending traces from your application to see them here.
              </p>
            </div>
          )
        ) : (
          <>
            <div className="overflow-x-auto rounded-xl border border-gray-200 bg-white shadow-sm">
              <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                  <tr>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-gray-500">
                      Name
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-gray-500">
                      Status
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-gray-500">
                      Duration
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-gray-500">
                      Tokens
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-gray-500">
                      Cost
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-gray-500">
                      Started
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 bg-white">
                  {traces.map((trace) => (
                    <TraceRow
                      key={trace.id}
                      trace={trace}
                      returnTo={currentListUrl}
                    />
                  ))}
                </tbody>
              </table>
            </div>
            <PaginationControls
              offset={filters.offset}
              pageSize={PAGE_SIZE}
              total={total}
              onOffsetChange={(offset) => setFilters({ offset }, 'push')}
            />
          </>
        )}
      </div>
    </div>
  );
}

interface TraceRowProps {
  trace: Trace;
  returnTo: string;
}

function TraceRow({ trace, returnTo }: TraceRowProps) {
  const duration = calculateDuration(trace.started_at, trace.ended_at);
  const totalTokens = (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);

  return (
    <tr className="transition hover:bg-gray-50">
      <td className="px-6 py-4 align-top">
        <div className="min-w-[16rem]">
          <div className="flex flex-wrap items-center gap-2">
            <Link
              to={`/traces/${trace.id}`}
              state={{ returnTo }}
              className="text-sm font-semibold text-blue-700 transition hover:text-blue-900 focus:outline-none focus:ring-2 focus:ring-blue-200"
            >
              {trace.name}
            </Link>
            {trace.error_count && trace.error_count > 0 && (
              <span className="inline-flex rounded-full bg-red-100 px-2 py-0.5 text-xs font-semibold text-red-700">
                {trace.error_count} error{trace.error_count === 1 ? '' : 's'}
              </span>
            )}
          </div>
          {trace.session_id && (
            <Link
              to={`/sessions/${trace.session_id}`}
              className="mt-1 inline-flex text-xs text-gray-500 transition hover:text-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-200"
            >
              Session {trace.session_id.slice(0, 8)}...
            </Link>
          )}
        </div>
      </td>
      <td className="px-6 py-4 whitespace-nowrap align-top">
        <StatusBadge status={trace.status} />
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900 align-top">
        {formatDuration(duration)}
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900 align-top">
        {formatTokens(totalTokens)}
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900 align-top">
        {formatCost(trace.total_cost_usd)}
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500 align-top">
        {formatRelativeTime(trace.started_at)}
      </td>
    </tr>
  );
}
