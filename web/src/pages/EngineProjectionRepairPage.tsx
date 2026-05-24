import { useState, type ChangeEvent, type FormEvent, type ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { AlertTriangle, RefreshCw, Wrench, X } from 'lucide-react';
import {
  ApiError,
  backfillEngineProjections,
  isAuthError,
  type EngineProjectionBackfillRequest,
  type EngineProjectionBackfillResponse,
  type EngineProjectionState,
  type EngineRunStatus,
} from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { Btn, Chip, DataTable, PageHeader, Td, Th, Tr } from '../components/DebuggerKit';
import { formatProjectionStateLabel } from '../components/engineProjectionState';
import {
  buildProjectPath,
  getProjectIdFromSearchParams,
} from '../utils/projectSearchParams';

const RUN_STATUSES: EngineRunStatus[] = [
  'QUEUED',
  'RUNNING',
  'WAITING',
  'SUSPENDED',
  'COMPLETED',
  'FAILED',
  'CANCELLED',
  'TERMINATED',
  'CONTINUED_AS_NEW',
];

const PROJECTION_STATES: EngineProjectionState[] = [
  'summary_only',
  'up_to_date',
  'catching_up',
  'journal_expired',
];

type FormState = {
  engine_instance_key: string;
  engine_definition_name: string;
  engine_run_status: '' | EngineRunStatus;
  engine_projection_state: EngineProjectionState;
  older_than: string;
  limit: string;
};

type RequestMode = 'idle' | 'dry-run' | 'apply';

function defaultFormState(): FormState {
  return {
    engine_instance_key: '',
    engine_definition_name: '',
    engine_run_status: '',
    engine_projection_state: 'summary_only',
    older_than: '',
    limit: '50',
  };
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unknown error';
}

function localDateTimeToUTC(value: string): string | undefined {
  if (!value.trim()) {
    return undefined;
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return undefined;
  }
  return parsed.toISOString();
}

function buildRequest(form: FormState, dryRun: boolean): EngineProjectionBackfillRequest {
  return {
    dry_run: dryRun,
    limit: Number(form.limit || '50'),
    ...(form.older_than ? { older_than: localDateTimeToUTC(form.older_than) } : {}),
    ...(form.engine_instance_key.trim()
      ? { engine_instance_key: form.engine_instance_key.trim() }
      : {}),
    ...(form.engine_definition_name.trim()
      ? { engine_definition_name: form.engine_definition_name.trim() }
      : {}),
    ...(form.engine_run_status ? { engine_run_status: form.engine_run_status } : {}),
    engine_projection_state: form.engine_projection_state,
  };
}

function classifyBackfillError(error: unknown): { title: string; message: string; auth: boolean } {
  if (isAuthError(error)) {
    return {
      auth: true,
      title: 'Authentication required',
      message: errorMessage(error),
    };
  }
  if (error instanceof ApiError) {
    if (error.status === 400) {
      return {
        auth: false,
        title: 'Request needs correction',
        message: error.message,
      };
    }
    if (error.status === 404) {
      return {
        auth: false,
        title: 'Engine API disabled',
        message: 'The projection repair endpoint is not available on this server.',
      };
    }
    if (error.status >= 500) {
      return {
        auth: false,
        title: 'Backend failure',
        message: error.message,
      };
    }
  }
  return {
    auth: false,
    title: 'Backend failure',
    message: errorMessage(error),
  };
}

export function EngineProjectionRepairPage() {
  const location = useLocation();
  const projectId = getProjectIdFromSearchParams(new URLSearchParams(location.search));
  const [form, setForm] = useState<FormState>(defaultFormState);
  const [result, setResult] = useState<EngineProjectionBackfillResponse | null>(null);
  const [lastEligibleCount, setLastEligibleCount] = useState<number | null>(null);
  const [hasDryRun, setHasDryRun] = useState(false);
  const [stale, setStale] = useState(false);
  const [requestMode, setRequestMode] = useState<RequestMode>('idle');
  const [limitError, setLimitError] = useState<string | null>(null);
  const [requestError, setRequestError] = useState<ReturnType<typeof classifyBackfillError> | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const isApplying = requestMode === 'apply';
  const isDryRunning = requestMode === 'dry-run';
  const formDisabled = isApplying;
  const applyDisabled =
    isApplying ||
    isDryRunning ||
    stale ||
    lastEligibleCount === null ||
    lastEligibleCount === 0;
  const applyDisabledCopy = stale
    ? 'Run a fresh dry run to apply.'
    : lastEligibleCount === 0
      ? 'No eligible runs to repair.'
      : null;

  const updateField = (field: keyof FormState) => (
    event: ChangeEvent<HTMLInputElement | HTMLSelectElement>
  ) => {
    setForm((current) => ({ ...current, [field]: event.target.value }));
    setRequestError(null);
    if (result || hasDryRun) {
      setResult(null);
      setStale(true);
    }
    if (field === 'limit') {
      const value = Number(event.target.value);
      setLimitError(value > 100 ? 'Limit must be 100 or less.' : null);
    }
  };

  const runBackfill = async (dryRun: boolean) => {
    if (requestMode !== 'idle') {
      return;
    }
    const limit = Number(form.limit || '50');
    if (limit > 100) {
      setLimitError('Limit must be 100 or less.');
      return;
    }
    setLimitError(null);
    setRequestError(null);
    setRequestMode(dryRun ? 'dry-run' : 'apply');
    try {
      const response = await backfillEngineProjections(buildRequest(form, dryRun));
      setResult(response);
      if (dryRun) {
        setHasDryRun(true);
        setStale(false);
        setLastEligibleCount(response.eligible_count);
      } else {
        setStale(false);
        setLastEligibleCount(response.eligible_count);
      }
    } catch (error) {
      setRequestError(classifyBackfillError(error));
    } finally {
      setRequestMode('idle');
      setConfirmOpen(false);
    }
  };

  const handleDryRun = (event: FormEvent) => {
    event.preventDefault();
    void runBackfill(true);
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader
        description="Preview and repair stale engine trace projections."
        title="Engine projection repair"
      />

      <form
        className="grid gap-4 border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-6 py-4"
        onSubmit={handleDryRun}
      >
        <fieldset disabled={formDisabled} className="grid gap-4 lg:grid-cols-3">
          <RepairField label="Instance key">
            <input
              className="app-input h-9 w-full rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 text-sm"
              value={form.engine_instance_key}
              onChange={updateField('engine_instance_key')}
            />
          </RepairField>
          <RepairField label="Definition name">
            <input
              className="app-input h-9 w-full rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 text-sm"
              value={form.engine_definition_name}
              onChange={updateField('engine_definition_name')}
            />
          </RepairField>
          <RepairField label="Run status">
            <select
              className="app-input h-9 w-full rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 text-sm"
              value={form.engine_run_status}
              onChange={updateField('engine_run_status')}
            >
              <option value="">Any status</option>
              {RUN_STATUSES.map((status) => (
                <option key={status} value={status}>
                  {status}
                </option>
              ))}
            </select>
          </RepairField>
          <RepairField
            label="Projection state"
            helper="Default repair target is summary_only. up_to_date, catching_up, and journal_expired return zero eligible rows."
          >
            <select
              className="app-input h-9 w-full rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 text-sm"
              value={form.engine_projection_state}
              onChange={updateField('engine_projection_state')}
            >
              {PROJECTION_STATES.map((state) => (
                <option key={state} value={state}>
                  {formatProjectionStateLabel(state)}
                </option>
              ))}
            </select>
          </RepairField>
          <RepairField label="Older than" helper="Times entered are converted to UTC before submission.">
            <input
              className="app-input h-9 w-full rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 text-sm"
              type="datetime-local"
              value={form.older_than}
              onChange={updateField('older_than')}
            />
          </RepairField>
          <RepairField label="Limit" helper={limitError ?? 'Maximum 100 runs per request.'}>
            <input
              className="app-input h-9 w-full rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 text-sm"
              min="1"
              max="100"
              step="1"
              type="number"
              value={form.limit}
              onChange={updateField('limit')}
            />
          </RepairField>
        </fieldset>

        <div className="flex flex-wrap items-center gap-2">
          <Btn kind="primary" leadingIcon={RefreshCw} type="submit" disabled={isDryRunning || isApplying}>
            {isDryRunning ? 'Running…' : 'Run dry run'}
          </Btn>
          {hasDryRun ? (
            <Btn
              kind="secondary"
              leadingIcon={Wrench}
              type="button"
              disabled={applyDisabled}
              onClick={() => setConfirmOpen(true)}
            >
              {isApplying ? 'Applying…' : 'Apply repair'}
            </Btn>
          ) : null}
          {applyDisabledCopy ? (
            <span className="text-xs text-[var(--c-text-muted)]">{applyDisabledCopy}</span>
          ) : null}
        </div>
      </form>

      {requestError ? (
        requestError.auth ? (
          <AuthErrorBanner message={requestError.message} />
        ) : (
          <div className="border-b border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-6 py-3 text-sm text-[var(--c-red-text)]">
            <span className="font-semibold">{requestError.title}</span>: {requestError.message}
            <button
              type="button"
              className="ml-3 font-semibold underline underline-offset-2"
              onClick={() => void runBackfill(true)}
            >
              Retry dry run
            </button>
          </div>
        )
      ) : null}

      <BackfillResultTable result={result} projectId={projectId} />

      {confirmOpen && lastEligibleCount !== null ? (
        <ApplyConfirmation
          count={lastEligibleCount}
          isApplying={isApplying}
          onCancel={() => setConfirmOpen(false)}
          onConfirm={() => void runBackfill(false)}
        />
      ) : null}
    </div>
  );
}

function RepairField({
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

function BackfillResultTable({
  projectId,
  result,
}: {
  projectId?: string;
  result: EngineProjectionBackfillResponse | null;
}) {
  if (!result) {
    return (
      <div className="app-empty-state">
        Run a dry run to preview eligible projection repairs.
      </div>
    );
  }

  return (
    <>
      <div className="grid gap-3 border-b border-[var(--c-border)] px-6 py-4 sm:grid-cols-4">
        <ResultStat label="Eligible" value={result.eligible_count} />
        <ResultStat label="Repair requested" value={result.repair_requested_count} />
        <ResultStat label="Skipped" value={result.skipped_count} />
        <ResultStat label="Mode" value={result.dry_run ? 'Dry run' : 'Apply'} />
      </div>
      {result.eligible_count === 0 && result.results.length === 0 ? (
        <div className="app-empty-state">
          <h2 className="text-base font-semibold text-[var(--c-text-primary)]">
            No eligible runs
          </h2>
          <p className="mt-2">The dry run completed successfully and found no repair targets.</p>
        </div>
      ) : (
        <DataTable>
          <thead>
            <tr>
              <Th>Run</Th>
              <Th>Trace</Th>
              <Th>Projection</Th>
              <Th>Action</Th>
              <Th>Reason</Th>
            </tr>
          </thead>
          <tbody>
            {result.results.map((row) => (
              <Tr key={row.run_id}>
                <Td mono>{row.run_id}</Td>
                <Td>
                  <Link
                    className="font-mono text-[var(--c-accent-text)] hover:underline"
                    to={buildProjectPath(`/traces/${row.trace_id}`, projectId)}
                  >
                    {row.trace_id}
                  </Link>
                </Td>
                <Td>
                  <Chip tone={row.projection_state === 'summary_only' ? 'amber' : 'muted'}>
                    {formatProjectionStateLabel(row.projection_state)}
                  </Chip>
                </Td>
                <Td>{row.action}</Td>
                <Td>{row.reason ?? '—'}</Td>
              </Tr>
            ))}
          </tbody>
        </DataTable>
      )}
    </>
  );
}

function ResultStat({ label, value }: { label: string; value: number | string }) {
  return (
    <div className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-2">
      <div className="text-[11px] font-semibold uppercase tracking-[0.08em] text-[var(--c-text-muted)]">
        {label}
      </div>
      <div className="mt-1 font-mono text-lg font-semibold text-[var(--c-text-primary)]">
        {value}
      </div>
    </div>
  );
}

function ApplyConfirmation({
  count,
  isApplying,
  onCancel,
  onConfirm,
}: {
  count: number;
  isApplying: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div className="fixed inset-0 z-[90] flex items-center justify-center bg-slate-950/45 p-4">
      <div className="w-full max-w-md rounded-lg border border-[var(--c-red-border)] bg-[var(--c-app-bg)] shadow-2xl">
        <div className="flex items-start justify-between gap-3 border-b border-[var(--c-red-border)] px-5 py-4">
          <div className="flex gap-3">
            <AlertTriangle className="mt-0.5 h-5 w-5 text-[var(--c-red-text)]" />
            <div>
              <h2 className="text-base font-semibold text-[var(--c-text-primary)]">
                Apply projection repair
              </h2>
              <p className="mt-2 text-sm text-[var(--c-text-secondary)]">
                Last dry run found {count} eligible runs. Backend state may have changed since the dry-run.
              </p>
            </div>
          </div>
          <button
            type="button"
            aria-label="Close confirmation"
            className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-[var(--c-border)]"
            onClick={onCancel}
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="flex justify-end gap-2 px-5 py-4">
          <Btn kind="secondary" type="button" disabled={isApplying} onClick={onCancel}>
            Cancel
          </Btn>
          <button
            type="button"
            disabled={isApplying}
            className="inline-flex h-8 items-center justify-center rounded-md border border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-3 text-[13px] font-semibold text-[var(--c-red-text)] disabled:cursor-not-allowed disabled:opacity-50"
            onClick={onConfirm}
          >
            {isApplying ? 'Applying…' : 'Apply repair'}
          </button>
        </div>
      </div>
    </div>
  );
}
