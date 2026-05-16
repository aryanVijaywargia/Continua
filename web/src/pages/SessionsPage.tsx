import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { Download } from 'lucide-react';
import { fetchSessions, isAuthError, type Session } from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import {
  Btn,
  Chip,
  DataTable,
  FilterBar,
  PageHeader,
  SearchInput,
  Td,
  Th,
  Tr,
} from '../components/DebuggerKit';
import { PaginationControls } from '../components/PaginationControls';
import { useSessionsSearchParams } from '../hooks/useSessionsSearchParams';
import { DEFAULT_PAGE_SIZE, getLastValidOffset } from '../utils/pagination';
import { buildSessionsQueryString } from '../utils/sessionsSearchParams';
import { formatRelativeTime } from '../utils/format';
import { appendProjectToPath } from '../utils/projectSearchParams';
import { downloadJsonFile } from '../utils/downloadJson';

const DEBOUNCE_MS = 300;
const EMPTY_SESSIONS: Session[] = [];
const USER_SEARCH_PREFIX = 'user:';

function normalizeDraft(value: string): string {
  return value.trim();
}

export function SessionsPage() {
  return <SessionsContent />;
}

function SessionsContent() {
  const location = useLocation();
  const { filters, setFilters, clearAll } = useSessionsSearchParams();
  const [searchDraft, setSearchDraft] = useState(filters.q ?? '');
  const isSearchActive = Boolean(filters.q);
  const currentListUrl = `${location.pathname}${location.search}`;

  useEffect(() => {
    if (filters.q) {
      setSearchDraft(filters.q);
      return;
    }

    setSearchDraft(filters.user_id ? `${USER_SEARCH_PREFIX}${filters.user_id}` : '');
  }, [filters.q, filters.user_id]);

  const commitSearch = useCallback(
    (value: string) => {
      const normalizedValue = normalizeDraft(value);
      if (normalizedValue.toLowerCase().startsWith(USER_SEARCH_PREFIX)) {
        const userId = normalizeDraft(normalizedValue.slice(USER_SEARCH_PREFIX.length));
        setFilters({ q: undefined, user_id: userId || undefined }, 'replace');
        return;
      }

      setFilters({ q: normalizedValue || undefined }, 'replace');
    },
    [setFilters]
  );

  useEffect(() => {
    const committedSearchDraft = filters.q
      ? filters.q
      : filters.user_id
        ? `${USER_SEARCH_PREFIX}${filters.user_id}`
        : '';

    if (normalizeDraft(searchDraft) === committedSearchDraft) {
      return;
    }

    const timeoutId = window.setTimeout(() => {
      commitSearch(searchDraft);
    }, DEBOUNCE_MS);

    return () => window.clearTimeout(timeoutId);
  }, [commitSearch, filters.q, filters.user_id, searchDraft]);

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

  const sessions = sessionsQuery.data?.sessions ?? EMPTY_SESSIONS;
  const total = sessionsQuery.data?.total ?? 0;
  const hasFilters = Boolean(filters.q || filters.user_id);
  const filterCount = Number(Boolean(filters.q)) + Number(Boolean(filters.user_id));
  const handleExport = useCallback(() => {
    downloadJsonFile('continua-sessions.json', {
      exported_at: new Date().toISOString(),
      source: currentListUrl,
      filters,
      total,
      count: sessions.length,
      sessions,
    });
  }, [currentListUrl, filters, sessions, total]);

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
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader
        actions={
          <Btn kind="secondary" leadingIcon={Download} size="sm" onClick={handleExport}>
            Export
          </Btn>
        }
        description="Group multi-trace user journeys to follow narrative context across runs."
        title="Sessions"
      />

      <FilterBar
        count={filterCount}
        onClear={clearAll}
        right={
          <span className="text-[11.5px] text-[var(--c-text-muted)]">
            {total} sessions
            {sessionsQuery.isFetching && !sessionsQuery.isPending ? ' · refreshing' : ''}
          </span>
        }
      >
        <SearchInput
          aria-label="Search"
          value={searchDraft}
          onChange={(event) => setSearchDraft(event.target.value)}
          onClear={() => {
            setSearchDraft('');
            setFilters({ q: undefined, user_id: undefined }, 'push');
          }}
          placeholder="Search external ID, user, or name…"
        />
        {filters.q && filters.user_id ? (
          <Chip
            closeLabel="Clear User filter"
            onClose={() => setFilters({ user_id: undefined }, 'push')}
          >
            <span>user:</span> <span>{filters.user_id}</span>
          </Chip>
        ) : null}
      </FilterBar>

      {sessionsQuery.error ? (
        isAuthError(sessionsQuery.error) ? (
          <AuthErrorBanner
            message={
              sessionsQuery.error instanceof Error
                ? sessionsQuery.error.message
                : 'Invalid or missing API key'
            }
          />
        ) : (
          <div className="border-b border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-6 py-3 text-sm text-[var(--c-red-text)]">
            Error loading sessions:{' '}
            {sessionsQuery.error instanceof Error
              ? sessionsQuery.error.message
              : 'Unknown error'}
          </div>
        )
      ) : null}

      {sessionsQuery.isPending && !sessionsQuery.data ? (
        <div className="app-empty-state">Loading sessions...</div>
      ) : sessions.length === 0 ? (
        <div className="app-empty-state">
          <h2 className="text-base font-semibold text-[var(--c-text-primary)]">
            {hasFilters ? 'No matching sessions' : 'No sessions yet'}
          </h2>
          <p className="mt-2">
            {hasFilters
              ? 'Try broadening the filters or clearing them.'
              : 'Sessions are created when traces include a session identifier.'}
          </p>
        </div>
      ) : (
        <>
          <DataTable>
            <colgroup>
              <col className="w-[26%]" />
              <col className="w-[22%]" />
              <col className="w-[26%]" />
              <col className="w-[90px]" />
              <col className="w-[130px]" />
              <col className="w-[130px]" />
            </colgroup>
            <thead>
              <tr>
                <Th>Session</Th>
                <Th>User</Th>
                <Th>Name</Th>
                <Th
                  align="right"
                  sortable={!isSearchActive}
                  sortActive={filters.sort_by === 'trace_count'}
                  sortDir={filters.sort_dir}
                  onSort={handleTraceCountSortToggle}
                >
                  Traces
                </Th>
                <Th align="right">Last active</Th>
                <Th
                  align="right"
                  sortable={!isSearchActive}
                  sortActive={filters.sort_by === 'created_at'}
                  sortDir={filters.sort_dir}
                  onSort={handleCreatedSortToggle}
                >
                  Created
                </Th>
              </tr>
            </thead>
            <tbody>
              {sessions.map((session) => (
                <SessionRow
                  key={session.id}
                  projectId={filters.project_id}
                  returnTo={currentListUrl}
                  session={session}
                />
              ))}
            </tbody>
          </DataTable>
          <div className="border-t border-[var(--c-border)] px-6 py-2">
            <PaginationControls
              offset={filters.offset}
              pageSize={filters.limit ?? DEFAULT_PAGE_SIZE}
              total={total}
              currentItemCount={sessions.length}
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

function SessionRow({
  projectId,
  returnTo,
  session,
}: {
  projectId?: string;
  returnTo: string;
  session: Session;
}) {
  const sessionPath = appendProjectToPath(`/sessions/${session.id}`, projectId);

  return (
    <Tr>
      <Td>
        <Link
          to={sessionPath}
          state={{ returnTo }}
          className="flex min-w-0 flex-col gap-0.5 hover:text-[var(--c-accent-text)]"
        >
          <span className="truncate font-mono text-[12.5px] font-medium text-[var(--c-text-primary)]">
            {session.external_id}
          </span>
          <span className="truncate font-mono text-[10.5px] text-[var(--c-text-muted)]">
            {session.id}
          </span>
        </Link>
      </Td>
      <Td mono>{session.user_id || 'No user ID'}</Td>
      <Td>{session.name || 'Unnamed session'}</Td>
      <Td align="right" mono>
        {session.trace_count ?? 0}
      </Td>
      <Td align="right" dim>
        {formatRelativeTime(session.created_at)}
      </Td>
      <Td align="right" dim>
        {formatRelativeTime(session.created_at)}
      </Td>
    </Tr>
  );
}
