import { useMemo, useState, type ReactNode } from 'react';
import {
  Download,
  ExternalLink,
  Info,
  Pause,
  Play,
  RotateCcw,
  SkipBack,
  SkipForward,
} from 'lucide-react';
import { Btn } from '../../components/DebuggerKit';
import { formatTimestamp } from '../../utils/format';
import {
  buildReplaySteps,
  type ReplayStep,
  type ReplayStepStatus,
} from '../../utils/replaySteps';
import { orderSpansByStart } from '../../utils/traceMetrics';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

type ReplayMode = 'from-start' | 'from-cp';

/** Preview-only replay workflow (no runtime replay is connected yet). */
export function TraceReplaySection() {
  const { events, exportTrace, selectedSpanId, spans, trace } =
    useTraceDetailWorkspace();

  const checkpoints = useMemo(
    () => events.filter((event) => event.event_type === 'snapshot_marker'),
    [events],
  );
  const failedSpan = useMemo(
    () => spans.find((span) => span.status === 'FAILED'),
    [spans],
  );
  const orderedSpans = useMemo(() => orderSpansByStart(spans), [spans]);

  const [mode, setMode] = useState<ReplayMode>(checkpoints.length > 0 ? 'from-cp' : 'from-start');
  const [selectedCheckpoint, setSelectedCheckpoint] = useState<string | undefined>(
    checkpoints[checkpoints.length - 1]?.id,
  );
  const [overrides, setOverrides] = useState({
    mockFailures: trace.status === 'FAILED',
    skipBackoff: false,
    frozenClock: true,
  });
  const [running, setRunning] = useState(false);

  const steps = useMemo<ReplayStep[]>(
    () => buildReplaySteps(orderedSpans, failedSpan, overrides.mockFailures),
    [failedSpan, orderedSpans, overrides.mockFailures]
  );

  const stepIdx = Math.max(
    steps.findIndex((step) => step.status === 'current'),
    0,
  );
  const progress = steps.length > 0 ? Math.round(((stepIdx + 1) / steps.length) * 100) : 0;

  const failureMessage = failedSpan?.error_message ?? 'Activity failed';
  const replayControlsDisabled = true;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <ReplayComingSoonBanner />
      <div className="flex min-h-0 flex-1">
      <aside className="hidden w-[320px] shrink-0 flex-col border-r border-[var(--c-border)] overflow-y-auto px-4 py-4 lg:flex">
        <ReplayGroup title="Replay source">
          <ReplayRadioRow
            checked={mode === 'from-start'}
            disabled={replayControlsDisabled}
            hint="Re-run full workflow history"
            label="From beginning"
            onChange={() => setMode('from-start')}
          />
          <ReplayRadioRow
            checked={mode === 'from-cp'}
            disabled={replayControlsDisabled || checkpoints.length === 0}
            hint={
              checkpoints.length > 0
                ? 'Resume from saved state'
                : 'No checkpoints recorded yet'
            }
            label="From checkpoint"
            onChange={() => checkpoints.length > 0 && setMode('from-cp')}
          />
          {mode === 'from-cp' && checkpoints.length > 0 ? (
            <select
              value={selectedCheckpoint ?? ''}
              disabled={replayControlsDisabled}
              onChange={(event) => setSelectedCheckpoint(event.target.value)}
              className="mt-1.5 w-full rounded border border-[var(--c-border)] bg-[var(--c-surface)] px-2.5 py-1.5 font-mono text-xs text-[var(--c-text-primary)] outline-none disabled:cursor-not-allowed disabled:opacity-60"
            >
              {checkpoints.map((checkpoint, index) => (
                <option key={checkpoint.id} value={checkpoint.id}>
                  cp-{index + 1} · {formatTimestamp(checkpoint.timestamp)}
                </option>
              ))}
            </select>
          ) : null}
        </ReplayGroup>

        <ReplayGroup title="Determinism overrides">
          <ReplayToggleRow
            checked={overrides.mockFailures}
            disabled={replayControlsDisabled}
            hint="Replace failing call with success response"
            label="Mock failed activities"
            onChange={(next) => setOverrides((prev) => ({ ...prev, mockFailures: next }))}
          />
          <ReplayToggleRow
            checked={overrides.skipBackoff}
            disabled={replayControlsDisabled}
            hint="Collapse retry backoff timers"
            label="Skip retry backoff"
            onChange={(next) => setOverrides((prev) => ({ ...prev, skipBackoff: next }))}
          />
          <ReplayToggleRow
            checked={overrides.frozenClock}
            disabled={replayControlsDisabled}
            hint="Pin time to original run"
            label="Frozen clock"
            onChange={(next) => setOverrides((prev) => ({ ...prev, frozenClock: next }))}
          />
        </ReplayGroup>

        <ReplayGroup title="Mocked response">
          <div className="rounded border border-[var(--c-border)] bg-[var(--c-surface)] p-2.5 font-mono text-[11.5px] leading-[1.55] text-[var(--c-text-primary)]">
            <div className="text-[var(--c-text-muted)]">
              {failedSpan ? `// ${failedSpan.name}` : '// no failure to mock'}
            </div>
            <div>{'{'}</div>
            <div className="pl-3">
              <span className="text-[var(--c-blue-text)]">"status"</span>:{' '}
              <span className="text-[var(--c-green-text)]">"succeeded"</span>,
            </div>
            <div className="pl-3">
              <span className="text-[var(--c-blue-text)]">"latency_ms"</span>:{' '}
              <span className="text-[var(--c-green-text)]">12</span>
            </div>
            <div>{'}'}</div>
          </div>
        </ReplayGroup>

        <div className="mt-2 flex flex-wrap gap-2">
          <Btn
            kind="primary"
            leadingIcon={running ? Pause : Play}
            size="sm"
            type="button"
            disabled={replayControlsDisabled}
            onClick={() => setRunning((prev) => !prev)}
          >
            {running ? 'Pause' : 'Run replay'}
          </Btn>
          <Btn
            kind="secondary"
            leadingIcon={RotateCcw}
            size="sm"
            type="button"
            disabled={replayControlsDisabled}
            onClick={() => setRunning(false)}
          >
            Reset
          </Btn>
          <Btn
            kind="secondary"
            leadingIcon={Download}
            size="sm"
            type="button"
            onClick={exportTrace}
          >
            Export
          </Btn>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <div
          className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--c-border-subtle)] px-5 py-3"
          style={{ background: running ? 'rgba(38,124,255,0.06)' : 'var(--c-surface-muted)' }}
        >
          <div className="flex items-center gap-2.5">
            <span
              className="inline-block h-2 w-2 rounded-full"
              style={{
                background: running ? 'var(--c-blue)' : 'var(--c-text-muted)',
                animation: running ? 'continua-pulse 1.6s ease-out infinite' : 'none',
              }}
            />
            <span className="text-[12.5px] font-medium text-[var(--c-text-primary)]">
              {running
                ? overrides.mockFailures
                  ? 'Replaying with mocked failures'
                  : 'Replaying original run'
                : 'Replay preview only'}
            </span>
            <span className="font-mono text-[11.5px] text-[var(--c-text-muted)]">
              · step {steps.length === 0 ? 0 : stepIdx + 1} of {steps.length}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <ReplayIconButton
              disabled={replayControlsDisabled}
              icon={SkipBack}
              label="Previous step"
              onClick={() => setRunning(false)}
            />
            <ReplayIconButton
              disabled={replayControlsDisabled}
              icon={running ? Pause : Play}
              label={running ? 'Pause' : 'Play'}
              onClick={() => setRunning((prev) => !prev)}
            />
            <ReplayIconButton
              disabled={replayControlsDisabled}
              icon={SkipForward}
              label="Next step"
              onClick={() => setRunning(false)}
            />
          </div>
        </div>

        <div className="border-b border-[var(--c-border-subtle)] px-5 py-3">
          <div className="relative h-1 bg-[var(--c-surface-muted)]">
            <div
              className="absolute inset-y-0 left-0 bg-[var(--c-accent)]"
              style={{ width: `${progress}%` }}
            />
            {checkpoints.map((checkpoint, index) => {
              const ts = new Date(checkpoint.timestamp).getTime();
              const start = new Date(trace.started_at).getTime();
              const end = trace.ended_at ? new Date(trace.ended_at).getTime() : ts;
              const total = Math.max(end - start, 1);
              const pct = Math.min(Math.max(((ts - start) / total) * 100, 0), 100);
              const active = checkpoint.id === selectedCheckpoint;
              return (
                <div
                  key={checkpoint.id}
                  className="absolute -top-1 -bottom-1 w-0.5"
                  style={{
                    left: `${pct}%`,
                    background: active ? 'var(--c-accent)' : 'var(--c-text-muted)',
                    opacity: active ? 1 : 0.4,
                  }}
                  title={`cp-${index + 1}`}
                />
              );
            })}
          </div>
          <div className="mt-1.5 flex justify-between font-mono text-[10.5px] text-[var(--c-text-muted)]">
            <span>start</span>
            {checkpoints.length > 0 ? (
              <span style={{ color: 'var(--c-accent)' }}>
                {checkpoints.length} checkpoint{checkpoints.length === 1 ? '' : 's'}
              </span>
            ) : null}
            <span>end</span>
          </div>
        </div>

        <div className="flex min-h-0 flex-1">
          <div className="hidden w-[320px] shrink-0 overflow-y-auto border-r border-[var(--c-border)] md:block">
            {steps.length === 0 ? (
              <div className="px-4 py-6 text-[12.5px] text-[var(--c-text-muted)]">
                No spans available to step through yet.
              </div>
            ) : (
              steps.map((step) => {
                const active = stepIdx === steps.findIndex((s) => s.id === step.id);
                return (
                  <div
                    key={step.id}
                    className="grid items-center gap-2 px-3.5 py-2 text-xs transition"
                    style={{
                      gridTemplateColumns: '24px minmax(0,1fr) auto',
                      borderBottom: '1px solid var(--c-border-subtle)',
                      borderLeft: `2px solid ${active ? 'var(--c-accent)' : 'transparent'}`,
                      background: active ? 'var(--c-row-selected-bg)' : 'transparent',
                    }}
                  >
                    <span className="text-right font-mono text-[10.5px] text-[var(--c-text-muted)]">
                      {steps.findIndex((s) => s.id === step.id) + 1}
                    </span>
                    <span
                      className="truncate font-mono"
                      style={{
                        color:
                          step.status === 'replayed'
                            ? 'var(--c-text-secondary)'
                            : step.status === 'current'
                              ? 'var(--c-text-primary)'
                              : 'var(--c-text-muted)',
                        fontWeight: step.status === 'current' ? 600 : 500,
                      }}
                    >
                      {step.name}
                    </span>
                    <span className="inline-flex items-center gap-1">
                      {step.mock ? (
                        <span className="rounded border border-[var(--c-amber-border)] bg-[var(--c-amber-faint)] px-1 font-mono text-[9px] uppercase text-[var(--c-amber-text)]">
                          MOCK
                        </span>
                      ) : null}
                      <ReplayStepDot status={step.status} />
                    </span>
                  </div>
                );
              })
            )}
          </div>

          <div className="min-w-0 flex-1 overflow-y-auto px-5 py-4">
            <div className="mb-3">
              <div className="text-[12.5px] font-semibold text-[var(--c-text-primary)]">
                Divergence from original run
              </div>
              <div className="mt-1 text-[11.5px] text-[var(--c-text-muted)]">
                {selectedSpanId
                  ? `Showing changes around span ${selectedSpanId}.`
                  : 'Showing changes if the failed step is mocked to succeed.'}
              </div>
            </div>

            {failedSpan ? (
              <ReplayDiffBlock
                after={[
                  '+ status: "succeeded"',
                  '+ duration_ms: 12 (mocked)',
                  '+ attempts: 1',
                ]}
                before={[
                  '- status: "failed"',
                  `- error: ${JSON.stringify(failureMessage)}`,
                  `- duration_ms: ${failedSpan.latency_ms ?? 0}`,
                ]}
                title={failedSpan.name}
              />
            ) : (
              <div className="rounded border border-dashed border-[var(--c-border)] px-4 py-6 text-center text-[12.5px] text-[var(--c-text-muted)]">
                No failure detected on this run — mocked replay would mirror the original outcome.
              </div>
            )}

            {failedSpan ? (
              <ReplayDiffBlock
                after={[
                  '+ trace.status: "completed"',
                  '+ projected_close_time: shortened',
                ]}
                before={[
                  `- trace.status: ${JSON.stringify(trace.status.toLowerCase())}`,
                  `- error_count: ${trace.error_count ?? 0}`,
                ]}
                title="Trace outcome"
              />
            ) : null}

            <div className="mt-4 flex items-start gap-2 rounded border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-3 py-2.5 text-[11.5px] text-[var(--c-text-secondary)]">
              <Info className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--c-text-muted)]" />
              <span>
                Planned replay will run in a sandboxed worker with buffered state writes
                before promotion as a new run.
                <Btn
                  kind="ghost"
                  leadingIcon={ExternalLink}
                  size="sm"
                  type="button"
                  className="ml-2"
                  onClick={exportTrace}
                >
                  Export trace
                </Btn>
              </span>
            </div>
          </div>
        </div>
      </div>
      </div>
    </div>
  );
}

function ReplayGroup({ children, title }: { children: ReactNode; title: string }) {
  return (
    <div className="mb-4">
      <div className="mb-1.5 text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
        {title}
      </div>
      {children}
    </div>
  );
}

function ReplayRadioRow({
  checked,
  disabled = false,
  hint,
  label,
  onChange,
}: {
  checked: boolean;
  disabled?: boolean;
  hint: string;
  label: string;
  onChange: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onChange}
      disabled={disabled}
      aria-pressed={checked}
      className={`mb-1.5 flex w-full items-start gap-2.5 rounded border px-2.5 py-2 text-left transition disabled:cursor-not-allowed disabled:opacity-60 ${
        checked
          ? 'border-[var(--c-accent)] bg-[var(--c-row-selected-bg)]'
          : 'border-[var(--c-border)] bg-transparent hover:border-[var(--c-border-strong)]'
      }`}
    >
      <span
        className="mt-0.5 inline-flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-full"
        style={{
          border: `1.5px solid ${checked ? 'var(--c-accent)' : 'var(--c-border-strong)'}`,
        }}
      >
        {checked ? (
          <span className="h-1.5 w-1.5 rounded-full bg-[var(--c-accent)]" />
        ) : null}
      </span>
      <span>
        <span className="block text-xs font-medium text-[var(--c-text-primary)]">{label}</span>
        <span className="mt-0.5 block text-[11px] text-[var(--c-text-muted)]">{hint}</span>
      </span>
    </button>
  );
}

function ReplayToggleRow({
  checked,
  disabled = false,
  hint,
  label,
  onChange,
}: {
  checked: boolean;
  disabled?: boolean;
  hint: string;
  label: string;
  onChange: (next: boolean) => void;
}) {
  return (
    <label
      className={`flex items-start gap-2.5 py-1.5 ${
        disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'
      }`}
    >
      <span
        className="relative mt-0.5 inline-block h-4 w-7 shrink-0 rounded-full transition-colors"
        style={{
          background: checked ? 'var(--c-accent)' : 'var(--c-border-strong)',
        }}
      >
        <span
          className="absolute top-0.5 h-3 w-3 rounded-full bg-white shadow-sm transition-[left]"
          style={{ left: checked ? '14px' : '2px' }}
        />
      </span>
      <input
        type="checkbox"
        className="sr-only"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span>
        <span className="block text-xs font-medium text-[var(--c-text-primary)]">{label}</span>
        <span className="mt-0.5 block text-[11px] text-[var(--c-text-muted)]">{hint}</span>
      </span>
    </label>
  );
}

function ReplayIconButton({
  disabled = false,
  icon: Icon,
  label,
  onClick,
}: {
  disabled?: boolean;
  icon: typeof Play;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      onClick={onClick}
      disabled={disabled}
      className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)] transition hover:text-[var(--c-text-primary)] disabled:cursor-not-allowed disabled:opacity-50"
    >
      <Icon className="h-3.5 w-3.5" />
    </button>
  );
}

function ReplayComingSoonBanner() {
  return (
    <div className="border-b border-[var(--c-amber-border)] bg-[var(--c-amber-faint)] px-5 py-3 text-[var(--c-amber-text)]">
      <div className="flex items-start gap-2.5">
        <Info className="mt-0.5 h-4 w-4 shrink-0" />
        <div>
          <div className="text-[12.5px] font-semibold text-[var(--c-text-primary)]">
            Replay is coming soon
          </div>
          <p className="mt-1 max-w-3xl text-[12px] leading-5 text-[var(--c-text-secondary)]">
            This preview shows the planned debugger workflow only. Runtime replay,
            sandboxed execution, checkpoints, mocked activities, and apply-as-new-run
            promotion are not connected in this checkout yet.
          </p>
        </div>
      </div>
    </div>
  );
}

function ReplayStepDot({ status }: { status: ReplayStepStatus }) {
  const color =
    status === 'replayed'
      ? 'var(--c-green)'
      : status === 'current'
        ? 'var(--c-blue)'
        : 'var(--c-border-strong)';
  return (
    <span
      className="inline-block h-2 w-2 rounded-full"
      style={{
        background: color,
        boxShadow: status === 'current' ? '0 0 0 3px rgba(38,124,255,0.18)' : 'none',
      }}
    />
  );
}

function ReplayDiffBlock({
  after,
  before,
  title,
}: {
  after: string[];
  before: string[];
  title: string;
}) {
  return (
    <div className="mb-3.5">
      <div className="mb-1.5 font-mono text-[11.5px] font-semibold text-[var(--c-text-secondary)]">
        {title}
      </div>
      <div className="overflow-hidden rounded border border-[var(--c-border)] font-mono text-[11.5px] leading-[1.65]">
        {before.map((line, index) => (
          <div
            key={`b-${index}`}
            className="px-3 py-0.5"
            style={{
              background: 'rgba(239,68,68,0.06)',
              color: 'var(--c-red-text)',
            }}
          >
            {line}
          </div>
        ))}
        {after.map((line, index) => (
          <div
            key={`a-${index}`}
            className="px-3 py-0.5"
            style={{
              background: 'rgba(16,185,129,0.07)',
              color: 'var(--c-green-text)',
            }}
          >
            {line}
          </div>
        ))}
      </div>
    </div>
  );
}
