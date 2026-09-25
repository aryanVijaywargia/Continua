import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo, useState, type FormEvent, type ReactNode } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import {
  ArrowRight,
  ArrowUpRight,
  FilterX,
  Link as LinkIcon,
  Play,
  RefreshCw,
  Workflow,
  X,
  type LucideIcon,
} from 'lucide-react';
import {
  ApiError,
  fetchEngineInstance,
  fetchTraces,
  isAuthError,
  listEngineDefinitions,
  startEngineRun,
  type EngineRunStatus,
  type JsonValue,
  type Trace,
} from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { ReadOnlyBadge } from '../components/DataState';
import {
  Btn,
  Chip,
  DataTable,
  FilterBar,
  PageHeader,
  StatusDot,
  Td,
  Th,
  Tr,
} from '../components/DebuggerKit';
import { formatProjectionStateLabel } from '../components/engineProjectionState';
import { PaginationControls } from '../components/PaginationControls';
import { describeEngineWaitState } from './engineWaitState';
import { DEFAULT_PAGE_SIZE } from '../utils/pagination';
import { formatExactTime, formatTimestamp } from '../utils/format';
import {
  buildProjectPath,
  getProjectIdFromSearchParams,
} from '../utils/projectSearchParams';
import {
  ENGINE_RUN_STATUS_FILTER_VALUES,
  formatEngineRunStatusLabel,
  type EngineRunStatusFilter,
} from '../utils/tracesSearchParams';
import { useRuntimeAuth } from '../auth/runtime';

const EMPTY_TRACES: Trace[] = [];
const ENGINE_GUIDE_URL = 'https://www.continua.in/docs/concepts/engine-foundation';
const SECONDARY_LINK_CLASS =
  'inline-flex h-7 items-center justify-center rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-2.5 text-xs font-medium text-[var(--c-text-primary)] transition hover:border-[var(--c-border-strong)]';

function getErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unknown error';
}

function describeErrorStatus(error: unknown): string {
  return error instanceof ApiError ? String(error.status) : 'no response';
}

/**
 * Names why a status filter returned nothing. The unfiltered total comes from
 * the unfiltered first page (same cache entry as the list with no filter), so
 * the page only claims the project has runs when it does.
 */
function describeFilterNoMatches(status: string, unfilteredTotal: number | undefined): string {
  if (unfilteredTotal === undefined) {
    return `No engine runs are ${status} right now.`;
  }
  if (unfilteredTotal === 0) {
    return `This project has no engine runs yet, so none are ${status}.`;
  }
  return `This project has engine runs, but none are ${status} right now.`;
}

function EmptyRunsState({
  body,
  children,
  icon: Icon,
  state,
  title,
}: {
  body: string;
  children?: ReactNode;
  icon: LucideIcon;
  state: 'empty-project' | 'filter-no-matches';
  title: string;
}) {
  return (
    <section
      data-state={state}
      className="flex flex-col items-center gap-2.5 px-6 py-14 text-center"
    >
      <span className="inline-flex h-[38px] w-[38px] items-center justify-center rounded-full bg-[var(--c-surface-muted)] text-[var(--c-text-secondary)]">
        <Icon aria-hidden="true" className="h-[19px] w-[19px]" />
      </span>
      <h2 className="text-sm font-bold text-[var(--c-text-primary)]">{title}</h2>
      <p className="max-w-[430px] text-[12.5px] leading-[1.6] text-[var(--c-text-secondary)]">
        {body}
      </p>
      {children ? (
        <div className="mt-0.5 flex flex-wrap items-center justify-center gap-3">{children}</div>
      ) : null}
    </section>
  );
}

function generateRequestKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID();
  }
  return `req-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function pendingBreakdown(trace: Trace): { label: string; title: string; total: number } {
  const activities = trace.engine?.pending_work?.pending_activity_tasks ?? 0;
  const inbox = trace.engine?.pending_work?.pending_inbox_items ?? 0;
  const total = activities + inbox;
  return {
    label: String(total),
    title: `${activities} activities, ${inbox} pending inbox items`,
    total,
  };
}

function updatedAt(trace: Trace): string {
  return trace.engine?.updated_at ?? trace.started_at;
}

function formatEngineDefinitionLabel(engine: Trace['engine']): string {
  if (!engine) {
    return '—';
  }
  return engine.definition_version
    ? `${engine.definition_name} · ${engine.definition_version}`
    : engine.definition_name;
}

export function EngineRunsPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const runtimeAuth = useRuntimeAuth();
  const isPublicDemo = runtimeAuth.public_demo_enabled === true;
  const searchParams = new URLSearchParams(location.search);
  const projectId = getProjectIdFromSearchParams(searchParams);
  const engineRunStatusCandidate = searchParams.get('engine_run_status');
  const engineRunStatus = ENGINE_RUN_STATUS_FILTER_VALUES.includes(
    engineRunStatusCandidate as EngineRunStatusFilter
  )
    ? (engineRunStatusCandidate as EngineRunStatusFilter)
    : undefined;
  const [offset, setOffset] = useState(0);
  const [pageSize, setPageSize] = useState<number>(DEFAULT_PAGE_SIZE);
  const [dialogOpen, setDialogOpen] = useState(false);
  const queryParams = {
    engine_only: true,
    limit: pageSize,
    offset,
    ...(engineRunStatus ? { engine_run_status: engineRunStatus } : {}),
  };
  const runsQuery = useQuery({
    queryKey: ['engine-runs', projectId ?? null, engineRunStatus ?? null, offset, pageSize],
    queryFn: () => fetchTraces(queryParams),
    placeholderData: keepPreviousData,
    refetchInterval: isPublicDemo ? false : 5000,
  });
  const traces = runsQuery.data?.traces ?? EMPTY_TRACES;
  const total = runsQuery.data?.total ?? 0;
  const returnTo = buildProjectPath('/engine/runs', projectId);
  const filteredToNothing =
    Boolean(engineRunStatus) &&
    runsQuery.isSuccess &&
    !runsQuery.isPlaceholderData &&
    total === 0;
  const unfilteredProbe = useQuery({
    queryKey: ['engine-runs', projectId ?? null, null, 0, pageSize],
    queryFn: () => fetchTraces({ engine_only: true, limit: pageSize, offset: 0 }),
    enabled: filteredToNothing,
  });
  const definitionVersions = useMemo(() => {
    const versions = new Map<string, Set<string>>();
    for (const trace of traces) {
      const name = trace.engine?.definition_name;
      const version = trace.engine?.definition_version;
      if (!name || !version) {
        continue;
      }
      const set = versions.get(name) ?? new Set<string>();
      set.add(version);
      versions.set(name, set);
    }
    return versions;
  }, [traces]);

  const setStatusFilter = (value: EngineRunStatusFilter | undefined) => {
    const nextSearchParams = new URLSearchParams(location.search);
    if (value) {
      nextSearchParams.set('engine_run_status', value);
    } else {
      nextSearchParams.delete('engine_run_status');
    }
    nextSearchParams.delete('offset');
    setOffset(0);
    navigate({
      pathname: location.pathname,
      search: nextSearchParams.toString(),
    });
  };
  const statusLabel = engineRunStatus ? formatEngineRunStatusLabel(engineRunStatus) : null;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader
        actions={
          <>
            {isPublicDemo ? <ReadOnlyBadge /> : null}
            <Link
              className={SECONDARY_LINK_CLASS}
              to={buildProjectPath('/tools/engine-health', projectId)}
            >
              Engine health
            </Link>
            <Btn kind="secondary" leadingIcon={RefreshCw} size="sm" onClick={() => void runsQuery.refetch()}>
              Refresh
            </Btn>
            {!isPublicDemo ? (
              <Btn kind="primary" leadingIcon={Play} size="sm" onClick={() => setDialogOpen(true)}>
                Start run
              </Btn>
            ) : null}
          </>
        }
        description={`${traces.length} of ${total} engine-backed traces · ${isPublicDemo ? 'read-only captured runs' : 'auto-refreshing every 5s'}`}
        title="Engine Runs"
      />

      <FilterBar
        right={
          engineRunStatus ? (
            <span
              className="inline-flex items-center gap-1.5 font-mono text-[10.5px] text-[var(--c-text-muted)]"
              title="The status filter is stored in the URL"
            >
              <LinkIcon aria-hidden="true" className="h-3 w-3" />
              ?engine_run_status={engineRunStatus}
            </span>
          ) : null
        }
      >
        {engineRunStatus && statusLabel ? (
          <Chip
            className="h-7 rounded-md px-2.5 text-xs font-semibold"
            closeLabel={`Clear ${statusLabel} status filter`}
            tone="accent"
            onClose={() => setStatusFilter(undefined)}
          >
            {statusLabel}
          </Chip>
        ) : null}
        <label className="sr-only" htmlFor="engine-run-status">
          Status
        </label>
        <select
          id="engine-run-status"
          aria-label="Engine run status"
          className="h-7 rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-2 text-xs font-medium text-[var(--c-text-secondary)] outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]"
          value={engineRunStatus ?? ''}
          onChange={(event) =>
            setStatusFilter((event.target.value || undefined) as EngineRunStatusFilter | undefined)
          }
        >
          <option value="">All statuses</option>
          {ENGINE_RUN_STATUS_FILTER_VALUES.map((value) => (
            <option
              key={value}
              aria-label={formatEngineRunStatusLabel(value)}
              label={formatEngineRunStatusLabel(value)}
              value={value}
            />
          ))}
        </select>
      </FilterBar>

      {runsQuery.error ? (
        isAuthError(runsQuery.error) ? (
          <AuthErrorBanner message={getErrorMessage(runsQuery.error)} />
        ) : (
          <div
            role="alert"
            className="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-6 py-2.5 text-[13px] text-[var(--c-red-text)]"
          >
            <span className="font-semibold">Could not load engine runs</span>
            <span className="font-mono text-[11px]">
              GET /api/traces · {describeErrorStatus(runsQuery.error)} · {getErrorMessage(runsQuery.error)}
            </span>
            <Btn
              className="ml-auto"
              kind="secondary"
              leadingIcon={RefreshCw}
              size="sm"
              onClick={() => void runsQuery.refetch()}
            >
              Retry
            </Btn>
          </div>
        )
      ) : null}

      {runsQuery.isPending && !runsQuery.data ? (
        <div className="app-empty-state">Loading engine runs...</div>
      ) : runsQuery.error && !runsQuery.data ? (
        <div className="app-empty-state">Retry the request to continue.</div>
      ) : traces.length === 0 ? (
        engineRunStatus && statusLabel ? (
          <EmptyRunsState
            state="filter-no-matches"
            icon={FilterX}
            title="No runs match this filter"
            body={describeFilterNoMatches(statusLabel.toLowerCase(), unfilteredProbe.data?.total)}
          >
            <Btn kind="secondary" size="sm" onClick={() => setStatusFilter(undefined)}>
              Clear filter
            </Btn>
          </EmptyRunsState>
        ) : (
          <EmptyRunsState
            state="empty-project"
            icon={Workflow}
            title="No engine runs in this project"
            body={
              isPublicDemo
                ? 'No captured engine runs are available in this demo.'
                : 'Nothing has been recorded yet. Durable workflows are authored in Go and appear here once a worker executes them. No filter is applied.'
            }
          >
            <a
              className="inline-flex items-center gap-1 text-xs font-semibold text-[var(--c-accent-text)] hover:underline"
              href={ENGINE_GUIDE_URL}
              rel="noreferrer"
              target="_blank"
            >
              Read the engine guide
              <ArrowUpRight aria-hidden="true" className="h-3 w-3" />
            </a>
            {!isPublicDemo ? (
              <Btn kind="primary" leadingIcon={Play} size="sm" onClick={() => setDialogOpen(true)}>
                Start run
              </Btn>
            ) : null}
          </EmptyRunsState>
        )
      ) : (
        <>
          <DataTable>
            <colgroup>
              <col className="w-[120px]" />
              <col className="w-[220px]" />
              <col className="w-[220px]" />
              <col className="w-[220px]" />
              <col className="w-[90px]" />
              <col className="w-[140px]" />
              <col className="w-[130px]" />
              <col className="w-12" />
            </colgroup>
            <thead>
              <tr>
                <Th>Status</Th>
                <Th>Definition</Th>
                <Th>Instance key</Th>
                <Th>Wait</Th>
                <Th align="right">Pending</Th>
                <Th>Projection</Th>
                <Th>Updated</Th>
                <Th align="center">→</Th>
              </tr>
            </thead>
            <tbody>
              {traces.map((trace) => {
                const waitSummary = describeEngineWaitState(trace.engine?.wait_state);
                const pending = pendingBreakdown(trace);
                const updated = updatedAt(trace);
                const to = buildProjectPath(`/traces/${trace.id}`, projectId);
                const definition = formatEngineDefinitionLabel(trace.engine);
                const instanceKey = trace.engine?.instance_key ?? '—';
                return (
                  <Tr key={trace.id} className="hover:bg-[var(--c-row-hover-bg)]">
                    <Td>
                      <StatusDot status={trace.engine?.status} />
                    </Td>
                    <Td>
                      <span title={definition}>{definition}</span>
                    </Td>
                    <Td mono>
                      <span className="text-xs" title={instanceKey}>{instanceKey}</span>
                    </Td>
                    <Td dim={!waitSummary}>
                      <span title={waitSummary ? `${waitSummary.heading}: ${waitSummary.detail}` : 'No wait state'}>
                        {waitSummary ? `${waitSummary.heading} · ${waitSummary.detail}` : '—'}
                      </span>
                    </Td>
                    <Td align="right" mono dim={pending.total === 0}>
                      <span className="text-xs" title={pending.title}>{pending.label}</span>
                    </Td>
                    <Td>
                      {trace.engine?.projection_state ? (
                        <Chip tone={trace.engine.projection_state === 'up_to_date' ? 'success' : 'amber'}>
                          {formatProjectionStateLabel(trace.engine.projection_state)}
                        </Chip>
                      ) : (
                        '—'
                      )}
                    </Td>
                    <Td mono dim>
                      <time className="text-xs" dateTime={updated} title={formatTimestamp(updated)}>
                        {formatExactTime(updated)}
                      </time>
                    </Td>
                    <Td align="center">
                      <Link
                        aria-label={`Open ${trace.id}`}
                        className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-[var(--c-border)] text-[var(--c-text-secondary)] hover:border-[var(--c-border-strong)] hover:text-[var(--c-text-primary)]"
                        state={{ returnTo }}
                        to={to}
                      >
                        <ArrowRight className="h-3.5 w-3.5" />
                      </Link>
                    </Td>
                  </Tr>
                );
              })}
            </tbody>
          </DataTable>
          <PaginationControls
            currentItemCount={traces.length}
            offset={offset}
            pageSize={pageSize}
            total={total}
            onOffsetChange={setOffset}
            onPageSizeChange={(nextPageSize) => {
              setPageSize(nextPageSize);
              setOffset(0);
            }}
          />
        </>
      )}

      {dialogOpen && !isPublicDemo ? (
        <StartEngineRunDialog
          definitionVersions={definitionVersions}
          projectId={projectId}
          onClose={() => setDialogOpen(false)}
          onSuccess={(traceId) => {
            setDialogOpen(false);
            navigate(buildProjectPath(`/traces/${traceId}`, projectId), {
              state: { returnTo },
            });
          }}
        />
      ) : null}
    </div>
  );
}

function StartEngineRunDialog({
  definitionVersions,
  onClose,
  onSuccess,
  projectId,
}: {
  definitionVersions: Map<string, Set<string>>;
  onClose: () => void;
  onSuccess: (traceId: string) => void;
  projectId?: string;
}) {
  const queryClient = useQueryClient();
  const [instanceKey, setInstanceKey] = useState('');
  const [definitionName, setDefinitionName] = useState('');
  const [definitionVersion, setDefinitionVersion] = useState('');
  const [definitionMode, setDefinitionMode] = useState<'picker' | 'manual'>('picker');
  const [requestKey, setRequestKey] = useState(generateRequestKey);
  const [inputText, setInputText] = useState('');
  const [fieldError, setFieldError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [definitionError, setDefinitionError] = useState(false);
  const [preflight, setPreflight] = useState<
    | { state: 'idle' }
    | { state: 'checking' }
    | { state: 'available' }
    | { state: 'exists'; definition?: string; status?: EngineRunStatus }
    | { state: 'unknown'; message: string }
  >({ state: 'idle' });
  const [submitting, setSubmitting] = useState(false);
  const definitionsQuery = useQuery({
    queryKey: ['engine-definitions', projectId ?? null],
    queryFn: listEngineDefinitions,
  });
  const definitions = definitionsQuery.data?.definitions ?? [];
  const definitionNames = Array.from(
    new Set(definitions.map((definition) => definition.definition_name))
  );
  const pickerVersions = definitions.filter(
    (definition) => definition.definition_name === definitionName
  );
  const versionSuggestions = definitionName
    ? Array.from(definitionVersions.get(definitionName) ?? [])
    : [];
  const definitionsLoading = definitionsQuery.isPending;
  const manualOnly =
    !definitionsLoading &&
    (definitionsQuery.isError || definitionsQuery.data?.definitions.length === 0);
  const manualMode = definitionMode === 'manual' || manualOnly;
  const staleDefinitions = definitions.filter((definition) => !definition.live);

  const runPreflight = async (): Promise<void> => {
    const key = instanceKey.trim();
    if (!key) {
      setPreflight({ state: 'idle' });
      return;
    }
    setPreflight({ state: 'checking' });
    try {
      const instance = await fetchEngineInstance(key);
      setPreflight({
        state: 'exists',
        definition: instance.current_run.definition_name,
        status: instance.current_run.status,
      });
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) {
        setPreflight({ state: 'available' });
        return;
      }
      setPreflight({
        state: 'unknown',
        message: getErrorMessage(error),
      });
    }
  };

  const handleDefinitionNameChange = (value: string) => {
    setDefinitionName(value);
    setDefinitionVersion('');
    setDefinitionError(false);
  };

  const parseInput = (): JsonValue | undefined => {
    const trimmed = inputText.trim();
    if (!trimmed) {
      return undefined;
    }
    const parsed = JSON.parse(trimmed) as JsonValue;
    return parsed;
  };

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (submitting) {
      return;
    }
    setFieldError(null);
    setSubmitError(null);
    setDefinitionError(false);

    if (
      !manualMode &&
      !definitions.some(
        (definition) =>
          definition.live &&
          definition.definition_name === definitionName &&
          definition.definition_version === definitionVersion
      )
    ) {
      setFieldError('Select a live definition and version.');
      return;
    }

    let input: JsonValue | undefined;
    try {
      input = parseInput();
    } catch {
      setFieldError('Input must be valid JSON.');
      return;
    }

    if (!instanceKey.trim() || !definitionName.trim() || !definitionVersion.trim() || !requestKey.trim()) {
      setFieldError('Instance key, definition, version, and idempotency key are required.');
      return;
    }

    setSubmitting(true);
    try {
      await runPreflight();
      const response = await startEngineRun({
        instance_key: instanceKey.trim(),
        definition_name: definitionName.trim(),
        definition_version: definitionVersion.trim(),
        request_key: requestKey.trim(),
        ...(input !== undefined ? { input } : {}),
      });
      const projectedTraces = await fetchTraces({
        engine_only: true,
        engine_run_id: response.run_id,
        limit: 1,
        ...(projectId ? { project_id: projectId } : {}),
      });
      const projectedTrace = projectedTraces.traces.find(
        (trace) => trace.engine?.run_id === response.run_id
      );
      if (!projectedTrace) {
        throw new Error('Run started, but its projected trace could not be resolved.');
      }
      await queryClient.invalidateQueries({ queryKey: ['engine-runs', projectId ?? null] });
      onSuccess(projectedTrace.id);
    } catch (error) {
      if (error instanceof ApiError && error.status === 409 && error.code === 'instance_conflict') {
        setSubmitError('Instance conflict. Recheck the durable workflow identity and run preflight again.');
      } else if (error instanceof ApiError && error.code === 'definition_not_registered') {
        setDefinitionError(true);
        setSubmitError('Definition is not registered. Check the engine definition configuration before retrying.');
      } else {
        setSubmitError(getErrorMessage(error));
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-[90] flex items-center justify-center bg-slate-950/45 p-4">
      <form
        className="max-h-[92vh] w-full max-w-2xl overflow-y-auto rounded-lg border border-[var(--c-border)] bg-[var(--c-app-bg)] shadow-2xl"
        onSubmit={(event) => void handleSubmit(event)}
      >
        <div className="flex items-center justify-between border-b border-[var(--c-border)] px-5 py-4">
          <div>
            <h2 className="text-base font-semibold text-[var(--c-text-primary)]">Start run</h2>
            <p className="mt-1 text-sm text-[var(--c-text-secondary)]">
              Launch an engine workflow and open its projected trace.
            </p>
          </div>
          <button
            type="button"
            aria-label="Close Start run"
            className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--c-border)]"
            onClick={onClose}
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="grid gap-4 px-5 py-4">
          <FormField label="Instance key" helper="The durable workflow identity, not a run id.">
            <div className="flex gap-2">
              <input
                className="app-input h-9 flex-1 rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 text-sm"
                value={instanceKey}
                onBlur={() => void runPreflight()}
                onChange={(event) => {
                  setInstanceKey(event.target.value);
                  setPreflight({ state: 'idle' });
                }}
              />
              <Btn kind="secondary" type="button" onClick={() => void runPreflight()}>
                Check
              </Btn>
            </div>
            <PreflightNotice state={preflight} />
          </FormField>

          {definitionsLoading ? (
            <div className="text-sm text-[var(--c-text-secondary)]">
              Loading registered definitions…
            </div>
          ) : manualOnly ? (
            <div className="app-alert-warning">
              Definitions unavailable. Enter the definition and version manually.
            </div>
          ) : (
            <div>
              <Btn
                kind="secondary"
                size="sm"
                type="button"
                onClick={() => setDefinitionMode(manualMode ? 'picker' : 'manual')}
              >
                {manualMode ? 'Use picker' : 'Enter manually'}
              </Btn>
            </div>
          )}

          <div className="grid gap-4 sm:grid-cols-2">
            <FormField label="Definition name">
              {manualMode ? (
                <>
                  <input
                    aria-invalid={definitionError}
                    className="app-input h-9 w-full rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 text-sm"
                    value={definitionName}
                    onChange={(event) => handleDefinitionNameChange(event.target.value)}
                    list="engine-definition-names"
                  />
                  <datalist id="engine-definition-names">
                    {Array.from(definitionVersions.keys()).map((name) => (
                      <option key={name} value={name} />
                    ))}
                  </datalist>
                </>
              ) : (
                <select
                  aria-invalid={definitionError}
                  className="app-input h-9 w-full rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 text-sm"
                  value={definitionName}
                  onChange={(event) => handleDefinitionNameChange(event.target.value)}
                >
                  <option value="">Select a definition</option>
                  {definitionNames.map((name) => {
                    const live = definitions.some(
                      (definition) =>
                        definition.definition_name === name && definition.live
                    );
                    return (
                      <option key={name} value={name} disabled={!live}>
                        {live ? name : `${name} — not live`}
                      </option>
                    );
                  })}
                </select>
              )}
            </FormField>
            <FormField label="Definition version">
              {manualMode ? (
                <>
                  <input
                    className="app-input h-9 w-full rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 text-sm"
                    value={definitionVersion}
                    onChange={(event) => setDefinitionVersion(event.target.value)}
                    list="engine-definition-versions"
                  />
                  <datalist id="engine-definition-versions">
                    {versionSuggestions.map((version) => (
                      <option key={version} value={version} />
                    ))}
                  </datalist>
                </>
              ) : (
                <select
                  className="app-input h-9 w-full rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 text-sm"
                  value={definitionVersion}
                  onChange={(event) => setDefinitionVersion(event.target.value)}
                >
                  <option value="">Select a version</option>
                  {pickerVersions.map((definition) => (
                    <option
                      key={definition.definition_version}
                      value={definition.definition_version}
                      disabled={!definition.live}
                    >
                      {definition.definition_version}
                    </option>
                  ))}
                </select>
              )}
            </FormField>
          </div>

          {!manualMode && staleDefinitions.length > 0 ? (
            <div className="grid gap-1">
              {staleDefinitions.map((definition) => (
                <p
                  key={`${definition.definition_name}:${definition.definition_version}`}
                  className="text-xs text-[var(--c-text-muted)]"
                >
                  {definition.definition_name} · {definition.definition_version} — last published{' '}
                  {formatTimestamp(definition.runtime_published_at)}
                </p>
              ))}
            </div>
          ) : null}

          <FormField label="Idempotency key" helper="Idempotency key — change only when retrying a prior start.">
            <input
              className="app-input h-9 w-full rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 font-mono text-sm"
              value={requestKey}
              onChange={(event) => setRequestKey(event.target.value)}
            />
          </FormField>

          <FormField label="Input JSON">
            <textarea
              className="app-input min-h-28 w-full rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-2 font-mono text-sm"
              placeholder='{"example": true}'
              value={inputText}
              onChange={(event) => setInputText(event.target.value)}
            />
          </FormField>

          {fieldError ? <div className="app-alert-error">{fieldError}</div> : null}
          {submitError ? <div className="app-alert-error">{submitError}</div> : null}
        </div>

        <div className="flex justify-end gap-2 border-t border-[var(--c-border)] px-5 py-4">
          <Btn kind="secondary" type="button" onClick={onClose}>
            Cancel
          </Btn>
          <Btn kind="primary" type="submit" disabled={submitting || definitionsLoading}>
            {submitting ? 'Starting…' : 'Start run'}
          </Btn>
        </div>
      </form>
    </div>
  );
}

function FormField({
  children,
  helper,
  label,
}: {
  children: ReactNode;
  helper?: string;
  label: string;
}) {
  return (
    <label className="block">
      <span className="text-xs font-semibold text-[var(--c-text-primary)]">{label}</span>
      <div className="mt-1.5">{children}</div>
      {helper ? <span className="mt-1.5 block text-xs text-[var(--c-text-muted)]">{helper}</span> : null}
    </label>
  );
}

function PreflightNotice({
  state,
}: {
  state:
    | { state: 'idle' }
    | { state: 'checking' }
    | { state: 'available' }
    | { state: 'exists'; definition?: string; status?: EngineRunStatus }
    | { state: 'unknown'; message: string };
}) {
  if (state.state === 'idle') {
    return null;
  }
  if (state.state === 'checking') {
    return <p className="mt-2 text-xs text-[var(--c-text-muted)]">Checking instance key…</p>;
  }
  if (state.state === 'available') {
    return <p className="mt-2 text-xs text-[var(--c-green-text)]">Instance key is available.</p>;
  }
  if (state.state === 'exists') {
    return (
      <p className="mt-2 text-xs text-[var(--c-amber-text)]">
        Active instance exists{state.definition ? ` for ${state.definition}` : ''}{state.status ? ` (${state.status})` : ''}.
      </p>
    );
  }
  return (
    <p className="mt-2 text-xs text-[var(--c-amber-text)]">
      Preflight unavailable. Submit remains enabled. {state.message}
    </p>
  );
}
