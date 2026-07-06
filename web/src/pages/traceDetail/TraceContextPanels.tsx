import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import type { Trace } from '../../api/client';
import { CopyButton } from '../../components/CopyButton';
import { appendProjectToPath } from '../../utils/projectSearchParams';
import { CompactPayloadInspector } from './CompactPayloadInspector';
import { TraceLineageCard } from './TraceLineagePanels';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

const EMPTY_TRACES: Trace[] = [];

interface TraceContextOverlayProps {
  childTraces: Trace[];
  childTracesLoading: boolean;
  hasLineageError: boolean;
  onClose: () => void;
}

/** Trace-context overlay for md+ viewports. */
export function TraceContextDrawer({
  childTraces,
  childTracesLoading,
  hasLineageError,
  onClose,
}: TraceContextOverlayProps) {
  return (
    <div className="app-overlay-enter fixed inset-0 z-50 hidden bg-[#111318]/40 backdrop-blur-sm md:block">
      <button
        type="button"
        aria-label="Close trace context drawer"
        className="absolute inset-0"
        onClick={onClose}
      />
      <aside
        className="app-drawer-enter absolute inset-y-4 right-4 w-[36rem] max-w-[calc(100vw-2rem)] overflow-y-auto"
        role="dialog"
        aria-modal="true"
        aria-label="Trace context"
      >
        <TraceContextSection
          childTraces={childTraces}
          childTracesLoading={childTracesLoading}
          hasLineageError={hasLineageError}
          onToggle={onClose}
          open
          showLineage
        />
      </aside>
    </div>
  );
}

/** Bottom-sheet variant of the trace-context overlay for small viewports. */
export function TraceContextSheet({
  childTraces,
  childTracesLoading,
  hasLineageError,
  onClose,
}: TraceContextOverlayProps) {
  return (
    <div className="app-overlay-enter fixed inset-0 z-50 flex items-end bg-[#111318]/50 backdrop-blur-sm md:hidden">
      <button
        type="button"
        aria-label="Close trace context sheet"
        className="absolute inset-0"
        onClick={onClose}
      />
      <aside
        className="app-sheet-enter relative z-10 max-h-[82vh] w-full overflow-y-auto px-3 pb-3"
        role="dialog"
        aria-modal="true"
        aria-label="Trace context"
      >
        <TraceContextSection
          childTraces={childTraces}
          childTracesLoading={childTracesLoading}
          hasLineageError={hasLineageError}
          onToggle={onClose}
          open
          showLineage={false}
        />
      </aside>
    </div>
  );
}

function TraceContextSection({
  childTraces,
  childTracesLoading,
  hasLineageError,
  onToggle,
  open,
  showLineage,
}: {
  childTraces: Trace[];
  childTracesLoading: boolean;
  hasLineageError: boolean;
  onToggle: () => void;
  open: boolean;
  showLineage: boolean;
}) {
  const { buildCopyTraceUrl, projectId, returnTo, trace } =
    useTraceDetailWorkspace();

  return (
    <section className="overflow-hidden rounded-md border border-[var(--c-border)] bg-[var(--c-app-bg)] shadow-[0_14px_36px_rgba(15,23,42,0.14)]">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-4 py-3">
        <button
          type="button"
          className="flex items-center gap-3 text-left"
          aria-expanded={open}
          onClick={onToggle}
        >
          <span className="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--c-text-primary)]">
            Trace Context
          </span>
          <span className="text-xs font-medium text-[var(--c-text-muted)]">
            {open ? 'Hide' : 'Show'}
          </span>
        </button>
        <CopyButton
          aria-label="Copy Trace URL"
          className="shrink-0 rounded-md border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)]"
          getValue={buildCopyTraceUrl}
          idleLabel="Copy Trace URL"
          successLabel="Copied URL"
        />
      </div>

      {open ? (
        <div className="space-y-5 p-4">
          <div className="overflow-hidden rounded-md border border-[var(--c-border)]">
            <TraceContextField
              label="ID"
              value={renderContextText(trace.id, true)}
              copyValue={trace.id}
              copyButtonLabel="Copy trace UUID"
            />
            <TraceContextField
              label="External Trace ID"
              value={renderContextText(trace.trace_id, true)}
              copyValue={trace.trace_id}
              copyButtonLabel="Copy external trace ID"
            />
            <TraceContextField
              label="Session"
              value={trace.session_id ? (
                <Link
                  to={appendProjectToPath(`/sessions/${trace.session_id}`, projectId)}
                  className="inline-flex min-w-0 flex-col text-left text-[var(--c-accent-text)] hover:opacity-80"
                >
                  <span className="truncate text-xs font-medium text-[var(--c-text-primary)]">
                    {trace.session_external_id ?? trace.session_id}
                  </span>
                  <span className="truncate font-mono text-[11px] text-[var(--c-text-muted)]">
                    {trace.session_id}
                  </span>
                </Link>
              ) : (
                renderContextText(undefined)
              )}
              copyValue={trace.session_id}
              copyButtonLabel="Copy session UUID"
            />
            <TraceContextField
              label="User ID"
              value={renderContextText(trace.user_id)}
            />
            <TraceContextField
              label="Environment"
              value={renderContextText(trace.environment)}
            />
            <TraceContextField
              label="Release"
              value={renderContextText(trace.release)}
            />
            <TraceContextField
              label="Tags"
              value={trace.tags && trace.tags.length > 0 ? (
                <div className="flex flex-wrap gap-2">
                  {trace.tags.map((tag) => (
                    <span
                      key={tag}
                      className="rounded border border-[var(--c-border)] bg-[var(--c-surface)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--c-text-secondary)]"
                    >
                      {tag}
                    </span>
                  ))}
                </div>
              ) : (
                renderContextText(undefined)
              )}
            />
          </div>

          {showLineage && trace.engine ? (
            <TraceLineageCard
              childTraces={childTraces}
              childTracesLoading={childTracesLoading}
              framed={false}
              hasChildTracesError={hasLineageError}
              lineageChain={EMPTY_TRACES}
              lineageLoading={false}
              projectId={projectId}
              returnTo={returnTo}
              trace={trace}
            />
          ) : null}

          {(trace.input !== undefined || trace.output !== undefined) ? (
            <div className="space-y-4">
              {trace.input !== undefined ? (
                <TracePayloadPanel title="Input" data={trace.input} />
              ) : null}
              {trace.output !== undefined ? (
                <TracePayloadPanel title="Output" data={trace.output} />
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function TraceContextField({
  label,
  value,
  copyValue,
  copyButtonLabel,
}: {
  label: string;
  value: ReactNode;
  copyValue?: string;
  copyButtonLabel?: string;
}) {
  return (
    <div className="grid grid-cols-[9rem_minmax(0,1fr)] gap-4 border-b border-[var(--c-border-subtle)] px-3 py-2.5 last:border-b-0">
      <div className="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--c-text-muted)]">
        {label}
      </div>
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0 flex-1 text-xs text-[var(--c-text-primary)]">{value}</div>
        {copyValue && copyButtonLabel ? (
          <CopyButton
            aria-label={copyButtonLabel}
            className="h-6 shrink-0 rounded-md border-[var(--c-border)] bg-[var(--c-surface)] px-2 text-[11px] text-[var(--c-text-secondary)]"
            value={copyValue}
          />
        ) : null}
      </div>
    </div>
  );
}

function TracePayloadPanel({ title, data }: { title: string; data: unknown }) {
  return (
    <section className="overflow-hidden rounded-md border border-[var(--c-border)] bg-[var(--c-app-bg)]">
      <div className="border-b border-[var(--c-border)] bg-[var(--c-table-head-bg)] px-3 py-2">
        <h3 className="text-xs font-semibold text-[var(--c-text-primary)]">{title}</h3>
      </div>
      <div className="max-h-80 overflow-auto p-3">
        <CompactPayloadInspector value={data} />
      </div>
    </section>
  );
}

function renderContextText(value: string | undefined, monospace = false) {
  if (value === undefined) {
    return <span className="text-xs text-[var(--c-text-muted)]">-</span>;
  }

  return (
    <span
      className={
        monospace
          ? 'font-mono text-xs text-[var(--c-text-primary)]'
          : 'text-xs text-[var(--c-text-primary)]'
      }
    >
      {value}
    </span>
  );
}
