import type { ReactNode } from 'react';
import type { TraceDetail } from '../../api/client';
import { describeEngineWaitState } from '../engineWaitState';

export type TraceDetailSectionId =
  | 'overview'
  | 'timeline'
  | 'logs'
  | 'metrics'
  | 'engine'
  | 'replay';

export function TraceDetailEmptyState({ children }: { children: ReactNode }) {
  return (
    <div className="app-page">
      <div className="app-empty-state mt-0 flex min-h-[24rem] items-center justify-center px-6">
        {children}
      </div>
    </div>
  );
}

export function TraceDetailErrorState({ children }: { children: ReactNode }) {
  return (
    <div className="app-page">
      <div className="app-alert-error">{children}</div>
    </div>
  );
}

export function TraceHeaderMetric({
  label,
  value,
  danger = false,
  mono = false,
}: {
  label: string;
  value: string;
  danger?: boolean;
  mono?: boolean;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <div className="text-[10.5px] font-medium uppercase tracking-[0.04em] text-[var(--c-text-muted)]">
        {label}
      </div>
      <div
        className={`text-[13px] font-medium tabular-nums ${mono ? 'font-mono' : ''} ${
          danger ? 'text-[var(--c-red-text)]' : 'text-[var(--c-text-primary)]'
        }`}
      >
        {value}
      </div>
    </div>
  );
}

export function TraceDetailTabs({
  activeSection,
  eventCount,
  hasEngine,
  onChange,
  replayPreviewEnabled,
  spanCount,
}: {
  activeSection: TraceDetailSectionId;
  eventCount: number;
  hasEngine: boolean;
  onChange: (section: TraceDetailSectionId) => void;
  replayPreviewEnabled: boolean;
  spanCount: number;
}) {
  const tabs: Array<{ id: TraceDetailSectionId; label: string; count?: number }> = [
    { id: 'overview', label: 'Overview' },
    { id: 'timeline', label: 'Timeline', count: spanCount },
    { id: 'logs', label: 'Logs', count: eventCount },
    { id: 'metrics', label: 'Metrics' },
    ...(hasEngine ? [{ id: 'engine' as const, label: 'Engine state' }] : []),
    ...(replayPreviewEnabled ? [{ id: 'replay' as const, label: 'Replay' }] : []),
  ];

  return (
    <nav
      aria-label="Trace detail sections"
      className="flex border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-6"
    >
      {tabs.map((tab) => (
        <button
          key={tab.id}
          type="button"
          aria-pressed={activeSection === tab.id}
          onClick={() => onChange(tab.id)}
          className={`-mb-px border-b-2 px-3.5 py-2 text-[13px] font-medium ${
            activeSection === tab.id
              ? 'border-[var(--c-accent)] text-[var(--c-text-primary)]'
              : 'border-transparent text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
          }`}
        >
          {tab.label}
          {tab.count != null ? (
            <span className="ml-1.5 font-mono text-[11px] text-[var(--c-text-muted)]">
              {tab.count}
            </span>
          ) : null}
        </button>
      ))}
    </nav>
  );
}

export function TraceSectionSurface({
  children,
  description,
  flush = false,
  title,
}: {
  children: ReactNode;
  description: string;
  flush?: boolean;
  title: string;
}) {
  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-[var(--c-app-bg)]">
      <div className="border-b border-[var(--c-border)] px-6 py-3">
        <h2 className="text-[11px] font-semibold uppercase tracking-[0.16em] text-[var(--c-text-muted)]">
          {title}
        </h2>
        <p className="mt-1 text-sm text-[var(--c-text-secondary)]">{description}</p>
      </div>
      <div className={flush ? 'flex min-h-0 flex-1 flex-col overflow-hidden' : 'min-h-0 flex-1 overflow-auto p-4'}>
        {children}
      </div>
    </section>
  );
}

export function EngineWaitStateSummary({
  engine,
}: {
  engine: NonNullable<TraceDetail['engine']>;
}) {
  const summary = describeEngineWaitState(engine.wait_state);
  if (!summary) {
    return null;
  }

  return (
    <section className="mt-3 rounded-md border border-[var(--c-blue-border)] bg-[var(--c-blue-faint)] px-3 py-2 text-[var(--c-blue-text)]">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="text-sm font-semibold">{summary.heading}</h2>
        <span className="text-[11px] font-medium uppercase tracking-[0.08em] opacity-75">
          {engine.pending_work.pending_activity_tasks} tasks · {engine.pending_work.pending_inbox_items} inbox
        </span>
      </div>
      <p className="mt-1 text-sm leading-6">{summary.detail}</p>
    </section>
  );
}

export function DefinitionVersionMismatchBanner() {
  return (
    <section className="rounded-[1rem] border border-amber-300/50 bg-amber-100/70 px-4 py-3 text-sm text-amber-950 shadow-[var(--continua-shadow-soft)] dark:border-amber-300/20 dark:bg-amber-400/10 dark:text-amber-100">
      <p className="font-semibold">Definition version mismatch</p>
      <p className="mt-1">
        This run failed because the engine definition version could not be
        matched during activation.
      </p>
    </section>
  );
}
