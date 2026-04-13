import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import {
  fetchSessions,
  isAuthError,
  type Session,
} from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { PaginationControls } from '../components/PaginationControls';
import { SortableHeader } from '../components/SortableHeader';
import { useRequireApiKey } from '../hooks/useRequireApiKey';
import { useSessionsSearchParams } from '../hooks/useSessionsSearchParams';
import { DEFAULT_PAGE_SIZE, getLastValidOffset } from '../utils/pagination';
import { buildSessionsQueryString } from '../utils/sessionsSearchParams';
import { formatRelativeTime } from '../utils/format';

const DEBOUNCE_MS = 300;
const EMPTY_SESSIONS: Session[] = [];

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

  const sessions = sessionsQuery.data?.sessions ?? EMPTY_SESSIONS;
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
    <div className="app-page">
      <section className="app-surface p-6 sm:p-7">
        <div className="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
          <div className="max-w-3xl">
            <div className="app-overline">Session workflows</div>
            <h1 className="mt-3 text-3xl font-black tight-headline text-[var(--continua-text-primary)] sm:text-4xl">
              Follow a user journey across multiple traces without losing narrative context.
            </h1>
            <p className="mt-3 text-sm leading-7 text-[var(--continua-text-secondary)] sm:text-base">
              Search sessions by external ID, filter by user, and sort for scale.
            </p>
          </div>

          <div className="grid gap-3 sm:grid-cols-3">
            <SessionStat label="Sessions" value={String(total)} />
            <SessionStat
              label="Loaded"
              value={String(sessions.length)}
            />
            <SessionStat
              label="Scoped user"
              value={filters.user_id ? '1' : 'All'}
            />
          </div>
        </div>
      </section>

      <section className="app-surface sticky top-[4.9rem] z-20 p-4 sm:p-5">
          <div className="mb-4 flex flex-wrap items-center gap-2">
            <QuickFilterButton
              active={!filters.user_id}
              label="All users"
              onClick={() => setFilters({ user_id: undefined }, 'push')}
            />
            <QuickFilterButton
              active={Boolean(filters.q)}
              label="Search active"
              onClick={() => {
                if (filters.q) {
                  setFilters({ q: undefined }, 'push');
                }
              }}
            />
            <div className="ml-auto text-sm text-[var(--continua-text-muted)]">
              <span>{total} total</span>
              {sessionsQuery.isFetching && !sessionsQuery.isPending ? <span className="ml-2">Refreshing…</span> : null}
            </div>
          </div>

          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <label
                htmlFor="session-search"
                className="mb-1 block text-sm font-medium text-[var(--continua-text-secondary)]"
              >
                Search
              </label>
              <input
                id="session-search"
                type="text"
                value={searchDraft}
                onChange={(event) => setSearchDraft(event.target.value)}
                placeholder="Search external ID or name"
                className="app-input"
              />
            </div>

            <div>
              <label
                htmlFor="session-user-id"
                className="mb-1 block text-sm font-medium text-[var(--continua-text-secondary)]"
              >
                User ID
              </label>
              <input
                id="session-user-id"
                type="text"
                value={userIdDraft}
                onChange={(event) => setUserIdDraft(event.target.value)}
                placeholder="Filter by exact user"
                className="app-input"
              />
            </div>
          </div>

          {hasFilters && (
            <div className="mt-4 flex justify-end">
              <button
                type="button"
                onClick={clearAll}
                className="app-button-secondary"
              >
                Clear filters
              </button>
            </div>
          )}
      </section>

      {sessionsQuery.error && (
        isAuthError(sessionsQuery.error) ? (
          <AuthErrorBanner
            message={
              sessionsQuery.error instanceof Error
                ? sessionsQuery.error.message
                : 'Invalid or missing API key'
            }
          />
        ) : (
          <div className="app-alert-error">
            Error loading sessions:{' '}
            {sessionsQuery.error instanceof Error
              ? sessionsQuery.error.message
              : 'Unknown error'}
          </div>
        )
      )}

      {sessionsQuery.isPending && !sessionsQuery.data ? (
        <div className="app-empty-state">Loading sessions...</div>
      ) : sessions.length === 0 ? (
        <div className="app-empty-state">
          <h2 className="text-lg font-black tight-headline text-[var(--continua-text-primary)]">
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
          <section className="app-surface overflow-hidden">
            <div className="flex items-center justify-between border-b border-[var(--continua-border-soft)] px-4 py-3 sm:px-5">
              <div className="flex items-center gap-4 app-overline">
                <span>Session</span>
                <span>User</span>
                <span>Name</span>
              </div>
              <div className="flex items-center gap-3 app-overline">
                <SortableHeader
                  label="Traces"
                  isActive={filters.sort_by === 'trace_count'}
                  isAscending={filters.sort_dir === 'asc'}
                  isDisabled={isSearchActive}
                  onClick={handleTraceCountSortToggle}
                />
                <SortableHeader
                  label="Created"
                  isActive={filters.sort_by === 'created_at'}
                  isAscending={filters.sort_dir === 'asc'}
                  isDisabled={isSearchActive}
                  onClick={handleCreatedSortToggle}
                />
              </div>
            </div>

            <div className="space-y-3 p-4 sm:p-5">
              {sessions.map((session) => (
                <SessionRow
                  key={session.id}
                  returnTo={currentListUrl}
                  session={session}
                />
              ))}
            </div>
          </section>

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
    <article className="app-list-row">
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-3">
          <Link
            to={`/sessions/${session.id}`}
            state={{ returnTo }}
            className="truncate text-sm font-semibold text-[var(--continua-text-primary)] transition hover:text-[var(--continua-accent)] focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)]"
          >
            {session.external_id}
          </Link>
          <span className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-2.5 py-1 text-[11px] font-medium text-[var(--continua-text-secondary)]">
            {session.trace_count ?? 0} traces
          </span>
        </div>
        <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--continua-text-muted)]">
          <span>{session.user_id || 'No user ID'}</span>
          <span>{session.name || 'Unnamed session'}</span>
          <span>Created {formatRelativeTime(session.created_at)}</span>
          <span className="font-mono">{session.id}</span>
        </div>
      </div>

      <div className="grid shrink-0 gap-3 text-right text-xs sm:grid-cols-2 sm:text-sm">
        <MetricColumn label="Traces" value={String(session.trace_count ?? 0)} />
        <MetricColumn label="Created" value={formatRelativeTime(session.created_at)} />
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

function SessionStat({ label, value }: { label: string; value: string }) {
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
