import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { fetchSessions, type Session } from '../api/client';
import { PaginationControls } from '../components/PaginationControls';
import { SortableHeader } from '../components/SortableHeader';
import { useRequireApiKey } from '../hooks/useRequireApiKey';
import { useSessionsSearchParams } from '../hooks/useSessionsSearchParams';
import { DEFAULT_PAGE_SIZE, getLastValidOffset } from '../utils/pagination';
import { buildSessionsQueryString } from '../utils/sessionsSearchParams';
import { formatRelativeTime } from '../utils/format';

const DEBOUNCE_MS = 300;

function normalizeDraft(value: string): string {
  return value.trim();
}

export function SessionsPage() {
  const { hasApiKey, prompt } = useRequireApiKey();

  if (!hasApiKey) {
    return prompt;
  }

  return <SessionsContent />;
}

function SessionsContent() {
  const location = useLocation();
  const { filters, setFilters, clearAll } = useSessionsSearchParams();
  const [searchDraft, setSearchDraft] = useState(filters.q ?? '');
  const [userIdDraft, setUserIdDraft] = useState(filters.user_id ?? '');
  const isSearchActive = Boolean(filters.q);
  const currentListUrl = `${location.pathname}${location.search}`;

  useEffect(() => {
    setSearchDraft(filters.q ?? '');
  }, [filters.q]);

  useEffect(() => {
    setUserIdDraft(filters.user_id ?? '');
  }, [filters.user_id]);

  const commitSearch = useCallback(
    (value: string) => {
      setFilters({ q: normalizeDraft(value) || undefined }, 'replace');
    },
    [setFilters]
  );

  const commitUserId = useCallback(
    (value: string) => {
      setFilters({ user_id: normalizeDraft(value) || undefined }, 'replace');
    },
    [setFilters]
  );

  useEffect(() => {
    if (normalizeDraft(searchDraft) === (filters.q ?? '')) {
      return;
    }

    const timeoutId = window.setTimeout(() => {
      commitSearch(searchDraft);
    }, DEBOUNCE_MS);

    return () => window.clearTimeout(timeoutId);
  }, [commitSearch, filters.q, searchDraft]);

  useEffect(() => {
    if (normalizeDraft(userIdDraft) === (filters.user_id ?? '')) {
      return;
    }

    const timeoutId = window.setTimeout(() => {
      commitUserId(userIdDraft);
    }, DEBOUNCE_MS);

    return () => window.clearTimeout(timeoutId);
  }, [commitUserId, filters.user_id, userIdDraft]);

  const handleCreatedSortToggle = useCallback(() => {
    if (isSearchActive) {
      return;
    }

    setFilters(
      {
        sort_by: 'created_at',
        sort_dir:
          filters.sort_by === 'created_at' && filters.sort_dir === 'asc'
            ? 'desc'
            : 'asc',
      },
      'push'
    );
  }, [filters.sort_by, filters.sort_dir, isSearchActive, setFilters]);

  const handleTraceCountSortToggle = useCallback(() => {
    if (isSearchActive) {
      return;
    }

    setFilters(
      {
        sort_by: 'trace_count',
        sort_dir:
          filters.sort_by === 'trace_count' && filters.sort_dir === 'asc'
            ? 'desc'
            : 'asc',
      },
      'push'
    );
  }, [filters.sort_by, filters.sort_dir, isSearchActive, setFilters]);

  const queryParams = { ...filters };
  const canonicalQueryString = buildSessionsQueryString(queryParams);
  const sessionsQuery = useQuery({
    queryKey: ['sessions', canonicalQueryString],
    queryFn: () => fetchSessions(queryParams),
    placeholderData: keepPreviousData,
  });

  const sessions = sessionsQuery.data?.sessions ?? [];
  const total = sessionsQuery.data?.total ?? 0;
  const hasFilters = Boolean(filters.q || filters.user_id);

  useEffect(() => {
    if (sessions.length !== 0 || total === 0 || filters.offset === 0) {
      return;
    }

    const lastValidOffset = getLastValidOffset(total, filters.limit ?? DEFAULT_PAGE_SIZE);
    if (lastValidOffset !== filters.offset) {
      setFilters({ offset: lastValidOffset }, 'replace');
    }
  }, [filters.limit, filters.offset, sessions.length, setFilters, total]);

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
        <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h1 className="text-2xl font-bold text-gray-900">Sessions</h1>
            <p className="mt-1 text-sm text-gray-500">
              Search sessions by external ID, filter by user, and sort for scale.
            </p>
          </div>
          <div className="text-sm text-gray-500">
            <span>{total} total</span>
            {sessionsQuery.isFetching && !sessionsQuery.isPending && (
              <span className="ml-2 text-blue-600">Updating...</span>
            )}
          </div>
        </div>

        <section className="mb-4 rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <label
                htmlFor="session-search"
                className="mb-1 block text-sm font-medium text-gray-700"
              >
                Search
              </label>
              <input
                id="session-search"
                type="text"
                value={searchDraft}
                onChange={(event) => setSearchDraft(event.target.value)}
                placeholder="Search external ID or name"
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 shadow-sm transition focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-200"
              />
            </div>

            <div>
              <label
                htmlFor="session-user-id"
                className="mb-1 block text-sm font-medium text-gray-700"
              >
                User ID
              </label>
              <input
                id="session-user-id"
                type="text"
                value={userIdDraft}
                onChange={(event) => setUserIdDraft(event.target.value)}
                placeholder="Filter by exact user"
                className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-900 shadow-sm transition focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-200"
              />
            </div>
          </div>

          {hasFilters && (
            <div className="mt-4 flex justify-end">
              <button
                type="button"
                onClick={clearAll}
                className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm font-medium text-gray-700 transition hover:border-gray-400 hover:text-gray-900 focus:outline-none focus:ring-2 focus:ring-blue-200"
              >
                Clear filters
              </button>
            </div>
          )}
        </section>

        {sessionsQuery.error && (
          <div className="mb-4 rounded-xl border border-red-200 bg-red-50 p-4 text-red-700">
            Error loading sessions:{' '}
            {sessionsQuery.error instanceof Error
              ? sessionsQuery.error.message
              : 'Unknown error'}
          </div>
        )}

        {sessionsQuery.isPending && !sessionsQuery.data ? (
          <div className="rounded-xl border border-gray-200 bg-white p-8 text-center text-gray-500 shadow-sm">
            Loading sessions...
          </div>
        ) : sessions.length === 0 ? (
          <div className="rounded-xl border border-gray-200 bg-white p-8 text-center shadow-sm">
            <h2 className="text-lg font-semibold text-gray-900">
              {hasFilters ? 'No matching sessions' : 'No sessions yet'}
            </h2>
            <p className="mt-2 text-sm text-gray-500">
              {hasFilters
                ? 'Try broadening the filters or clearing them.'
                : 'Sessions are created when traces include a session identifier.'}
            </p>
          </div>
        ) : (
          <>
            <div className="overflow-x-auto rounded-xl border border-gray-200 bg-white shadow-sm">
              <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                  <tr>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-gray-500">
                      Session
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-gray-500">
                      Name
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-gray-500">
                      User ID
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-gray-500">
                      <SortableHeader
                        label="Traces"
                        isActive={filters.sort_by === 'trace_count'}
                        isAscending={filters.sort_dir === 'asc'}
                        isDisabled={isSearchActive}
                        onClick={handleTraceCountSortToggle}
                      />
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-gray-500">
                      <SortableHeader
                        label="Created"
                        isActive={filters.sort_by === 'created_at'}
                        isAscending={filters.sort_dir === 'asc'}
                        isDisabled={isSearchActive}
                        onClick={handleCreatedSortToggle}
                      />
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-200 bg-white">
                  {sessions.map((session) => (
                    <SessionRow
                      key={session.id}
                      returnTo={currentListUrl}
                      session={session}
                    />
                  ))}
                </tbody>
              </table>
            </div>

            <PaginationControls
              offset={filters.offset}
              pageSize={filters.limit ?? DEFAULT_PAGE_SIZE}
              total={total}
              currentItemCount={sessions.length}
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

function SessionRow({
  session,
  returnTo,
}: {
  session: Session;
  returnTo: string;
}) {
  return (
    <tr className="transition hover:bg-gray-50">
      <td className="px-6 py-4 align-top">
        <div className="min-w-[14rem]">
          <Link
            to={`/sessions/${session.id}`}
            state={{ returnTo }}
            className="text-sm font-semibold text-blue-700 transition hover:text-blue-900 focus:outline-none focus:ring-2 focus:ring-blue-200"
          >
            {session.external_id}
          </Link>
          <div className="mt-1 font-mono text-xs text-gray-400">{session.id}</div>
        </div>
      </td>
      <td className="px-6 py-4 text-sm text-gray-900 align-top">
        {session.name || '-'}
      </td>
      <td className="px-6 py-4 text-sm text-gray-500 align-top">
        {session.user_id || '-'}
      </td>
      <td className="px-6 py-4 text-sm text-gray-900 align-top">
        {session.trace_count ?? 0}
      </td>
      <td className="px-6 py-4 text-sm text-gray-500 align-top">
        {formatRelativeTime(session.created_at)}
      </td>
    </tr>
  );
}
