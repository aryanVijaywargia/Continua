import { useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { ArrowLeft, Braces, ChevronDown, ChevronRight, Info, X } from 'lucide-react';
import type { Span } from '../../api/client';
import { CopyButton } from '../../components/CopyButton';
import { DerivedTag } from '../../components/DataState';
import { Chip, StatusDot } from '../../components/DebuggerKit';
import { PayloadInspector } from '../../components/PayloadInspector';
import { SpanDetail } from '../../components/SpanDetail';
import { TruncationBanner } from '../../components/TruncationBanner';
import { ReadablePayloadView } from '../../components/trace/ReadablePayloadView';
import { formatPreciseTime } from '../../components/trace/traceSteps';
import type { StepDuration } from '../../components/trace/traceSteps';
import type { StepDurations } from '../../components/trace/useStepDurations';
import { formatCost, formatDerivedDuration, formatTokens } from '../../utils/format';
import { appendProjectToPath } from '../../utils/projectSearchParams';
import { CompactPayloadInspector, InspectorEmptyState } from './CompactPayloadInspector';
import { TraceStatePanels } from './TraceStatePanels';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

export const SELECT_STEP_HINT = 'Select a step to inspect its input, output, and details.';
const PROMPTS_ABSENT_NOTE =
  'Recorded event data, summarised. The captured prompts are not in this dataset.';
const UNKNOWN_DURATION: StepDuration = { ms: null, derived: false, running: false };

/**
 * The right-hand inspector. With no step selected it summarises the trace;
 * with a step selected it shows that step's input, output, and details.
 */
export function TraceInspector({
  durations,
  onBack,
}: {
  durations: StepDurations;
  /** Narrow layouts pass this to return to the step list. */
  onBack?: () => void;
}) {
  const { selectedSpan } = useTraceDetailWorkspace();

  return (
    <aside
      aria-label="Inspector"
      className="flex h-full min-h-0 flex-col overflow-hidden bg-[var(--c-app-bg)]"
    >
      {selectedSpan ? (
        <StepInspector
          key={selectedSpan.span_id}
          duration={durations.byId.get(selectedSpan.span_id) ?? UNKNOWN_DURATION}
          onBack={onBack}
          span={selectedSpan}
        />
      ) : (
        <TraceSummaryInspector />
      )}
    </aside>
  );
}

function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <h3 className="mb-2 text-[11px] font-semibold uppercase tracking-[0.1em] text-[var(--c-text-muted)]">
      {children}
    </h3>
  );
}

function KeyValueRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[6.5rem_minmax(0,1fr)] items-center gap-3 py-1.5 text-xs">
      <dt className="text-[var(--c-text-muted)]">{label}</dt>
      <dd className="min-w-0 break-words text-[var(--c-text-primary)]">{children}</dd>
    </div>
  );
}

function TraceSummaryInspector() {
  const { projectId, returnTo, trace } = useTraceDetailWorkspace();
  const [detailsOpen, setDetailsOpen] = useState(true);
  const hasInput = trace.input !== undefined && trace.input !== null;
  const hasOutput = trace.output !== undefined && trace.output !== null;
  const externalTraceId = trace.trace_id ?? trace.id;

  return (
    <div className="min-h-0 flex-1 space-y-5 overflow-y-auto p-4">
      <TraceStatePanels />

      {!hasInput && !hasOutput ? (
        <p className="flex items-start gap-2 rounded-md border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-3 py-2.5 text-xs leading-5 text-[var(--c-text-secondary)]">
          <Info aria-hidden="true" className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--c-text-muted)]" />
          {PROMPTS_ABSENT_NOTE}
        </p>
      ) : null}

      <section>
        <SectionLabel>Trace request</SectionLabel>
        {hasInput ? (
          <ReadablePayloadView value={trace.input} />
        ) : (
          <InspectorEmptyState>No request payload was recorded for this trace.</InspectorEmptyState>
        )}
      </section>

      <section>
        <SectionLabel>Result</SectionLabel>
        {hasOutput ? (
          <ReadablePayloadView value={trace.output} />
        ) : (
          <InspectorEmptyState>No result payload was recorded for this trace.</InspectorEmptyState>
        )}
      </section>

      <section className="border-t border-[var(--c-border)] pt-3">
        <button
          type="button"
          aria-expanded={detailsOpen}
          onClick={() => setDetailsOpen((open) => !open)}
          className="flex w-full items-center gap-1.5 text-[11px] font-semibold uppercase tracking-[0.1em] text-[var(--c-text-muted)] hover:text-[var(--c-text-primary)]"
        >
          {detailsOpen ? (
            <ChevronDown aria-hidden="true" className="h-3.5 w-3.5" />
          ) : (
            <ChevronRight aria-hidden="true" className="h-3.5 w-3.5" />
          )}
          Details
        </button>
        {detailsOpen ? (
          <dl className="mt-2">
            <KeyValueRow label="Trace ID">
              <span className="inline-flex max-w-full items-center gap-1">
                <span className="truncate font-mono">{externalTraceId}</span>
                <CopyButton
                  aria-label="Copy trace ID"
                  value={externalTraceId}
                  idleLabel=""
                  successLabel=""
                  className="h-5 min-w-5 border-0 bg-transparent px-1 text-[var(--c-text-muted)] shadow-none hover:text-[var(--c-text-primary)]"
                />
              </span>
            </KeyValueRow>
            <KeyValueRow label="Session">
              {trace.session_id ? (
                <Link
                  to={appendProjectToPath(`/sessions/${trace.session_id}`, projectId)}
                  state={{ returnTo }}
                  className="font-mono text-[var(--c-accent-text)] hover:underline"
                >
                  {trace.session_external_id ?? trace.session_id}
                </Link>
              ) : (
                <span className="text-[var(--c-text-muted)]">No session</span>
              )}
            </KeyValueRow>
            <KeyValueRow label="Started">
              <span className="font-mono">{formatPreciseTime(trace.started_at)}</span>
            </KeyValueRow>
            <KeyValueRow label="Ended">
              <span className="font-mono">
                {trace.ended_at ? formatPreciseTime(trace.ended_at) : 'Still running'}
              </span>
            </KeyValueRow>
            {trace.user_id ? (
              <KeyValueRow label="User">
                <span className="font-mono">{trace.user_id}</span>
              </KeyValueRow>
            ) : null}
            {trace.environment ? (
              <KeyValueRow label="Environment">{trace.environment}</KeyValueRow>
            ) : null}
            {trace.release ? <KeyValueRow label="Release">{trace.release}</KeyValueRow> : null}
            {trace.tags && trace.tags.length > 0 ? (
              <KeyValueRow label="Tags">
                <span className="flex flex-wrap gap-1">
                  {trace.tags.map((tag) => (
                    <Chip key={tag}>{tag}</Chip>
                  ))}
                </span>
              </KeyValueRow>
            ) : null}
          </dl>
        ) : null}
      </section>

      <p className="text-xs text-[var(--c-text-muted)]">{SELECT_STEP_HINT}</p>
    </div>
  );
}

type StepTab = 'input' | 'output' | 'details';

function StepInspector({
  duration,
  onBack,
  span,
}: {
  duration: StepDuration;
  onBack?: () => void;
  span: Span;
}) {
  const {
    clearSelection,
    events,
    retrySafetyAnalysis,
    selectSpanAndShowDetails,
    selectedBreadcrumbPath,
    spanIndex,
  } = useTraceDetailWorkspace();
  const [activeTab, setActiveTab] = useState<StepTab>('input');
  const [showRaw, setShowRaw] = useState(false);
  const spanEvents = events.filter(
    (event) => event.span_id === span.span_id && event.source === 'explicit'
  );
  const totalTokens = (span.tokens_in ?? 0) + (span.tokens_out ?? 0);
  const hasCost = (span.cost_usd ?? 0) > 0;
  const tabs: Array<{ id: StepTab; label: string; count?: number }> = [
    { id: 'input', label: 'Input' },
    { id: 'output', label: 'Output' },
    { id: 'details', label: 'Details', count: spanEvents.length || undefined },
  ];
  const payload = activeTab === 'input' ? span.input : span.output;

  return (
    <>
      <div className="border-b border-[var(--c-border)] px-4 py-3">
        {onBack ? (
          <button
            type="button"
            onClick={onBack}
            className="mb-2 inline-flex items-center gap-1 text-xs font-medium text-[var(--c-text-secondary)] hover:text-[var(--c-accent-text)]"
          >
            <ArrowLeft aria-hidden="true" className="h-3.5 w-3.5" />
            Back to steps
          </button>
        ) : null}
        <div className="flex items-center gap-2">
          <StatusDot status={span.status} withLabel={false} />
          <h2 className="min-w-0 flex-1 truncate font-mono text-[13px] font-semibold text-[var(--c-text-primary)]">
            {span.name}
          </h2>
          <span className="shrink-0 font-mono text-xs tabular-nums text-[var(--c-text-secondary)]">
            {formatDerivedDuration(duration.ms)}
          </span>
          {onBack ? null : (
            <button
              type="button"
              aria-label="Close step"
              title="Show the trace summary"
              onClick={clearSelection}
              className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-[var(--c-text-muted)] hover:bg-[var(--c-row-hover-bg)] hover:text-[var(--c-text-primary)]"
            >
              <X aria-hidden="true" className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          <Chip className="font-mono uppercase">{span.kind}</Chip>
          <StatusDot status={span.status} />
          {duration.derived ? <DerivedTag label="duration derived" /> : null}
          {span.model ? <Chip>{span.model}</Chip> : null}
          {totalTokens > 0 ? <Chip>{formatTokens(totalTokens)} tokens</Chip> : null}
          {hasCost ? <Chip>{formatCost(span.cost_usd)}</Chip> : null}
        </div>
        {span.error_message ? (
          <pre className="mt-3 max-h-40 overflow-auto whitespace-pre-wrap rounded border border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-3 py-2 font-mono text-xs leading-5 text-[var(--c-red-text)]">
            {span.error_message}
          </pre>
        ) : null}
      </div>

      <div className="flex items-center border-b border-[var(--c-border)] px-3">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            aria-pressed={activeTab === tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`-mb-px border-b-2 px-3 py-2 text-xs font-medium ${
              activeTab === tab.id
                ? 'border-[var(--c-accent)] text-[var(--c-text-primary)]'
                : 'border-transparent text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
            }`}
          >
            {tab.label}
            {tab.count != null ? (
              <span className="ml-1 font-mono text-[10.5px] text-[var(--c-text-muted)]">{tab.count}</span>
            ) : null}
          </button>
        ))}
        {activeTab !== 'details' && payload !== undefined ? (
          <button
            type="button"
            aria-pressed={showRaw}
            onClick={() => setShowRaw((value) => !value)}
            className={`ml-auto inline-flex h-6 items-center gap-1 rounded border px-2 text-[11px] font-medium transition ${
              showRaw
                ? 'border-[var(--c-accent-border)] bg-[var(--c-accent-faint)] text-[var(--c-accent-text)]'
                : 'border-[var(--c-border)] text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
            }`}
          >
            <Braces aria-hidden="true" className="h-3 w-3" />
            Raw JSON
          </button>
        ) : null}
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {activeTab === 'input' ? (
          <PayloadPane
            emptyLabel="No input payload was recorded for this step."
            showRaw={showRaw}
            truncation={
              <TruncationBanner
                title="Input payload"
                truncated={span.input_truncated}
                originalSizeBytes={span.input_original_size_bytes}
                reason={span.input_truncation_reason}
              />
            }
            value={span.input}
          />
        ) : null}

        {activeTab === 'output' ? (
          <PayloadPane
            emptyLabel={
              span.error_message
                ? 'No output was recorded. The step failed with the error above.'
                : 'No output payload was recorded for this step.'
            }
            showRaw={showRaw}
            truncation={
              <TruncationBanner
                title="Output payload"
                truncated={span.output_truncated}
                originalSizeBytes={span.output_original_size_bytes}
                reason={span.output_truncation_reason}
              />
            }
            value={span.output}
          />
        ) : null}

        {activeTab === 'details' ? (
          <div className="space-y-5">
            <SpanDetail
              variant="details"
              span={span}
              breadcrumbPath={selectedBreadcrumbPath}
              onSelectSpan={selectSpanAndShowDetails}
              spanIndex={spanIndex}
              events={events}
              retrySafety={
                span.status === 'FAILED' && retrySafetyAnalysis
                  ? retrySafetyAnalysis.spanAssessments.get(span.span_id) ?? null
                  : null
              }
            />
            <section>
              <SectionLabel>Step events</SectionLabel>
              {spanEvents.length > 0 ? (
                <ul className="divide-y divide-[var(--c-border-subtle)] rounded-md border border-[var(--c-border)] bg-[var(--c-surface)]">
                  {spanEvents.map((event) => (
                    <li key={event.id} className="px-3 py-2 text-xs">
                      <div className="flex items-center justify-between gap-3">
                        <span className="font-mono text-[var(--c-text-primary)]">{event.event_type}</span>
                        <span className="font-mono text-[var(--c-text-muted)]">
                          {formatPreciseTime(event.timestamp)}
                        </span>
                      </div>
                      {event.message ? (
                        <p className="mt-1 text-[var(--c-text-secondary)]">{event.message}</p>
                      ) : null}
                      {event.payload !== undefined ? (
                        <div className="mt-2">
                          <CompactPayloadInspector value={event.payload} />
                        </div>
                      ) : null}
                    </li>
                  ))}
                </ul>
              ) : (
                <InspectorEmptyState>No events were recorded for this step.</InspectorEmptyState>
              )}
            </section>
          </div>
        ) : null}
      </div>
    </>
  );
}

function PayloadPane({
  emptyLabel,
  showRaw,
  truncation,
  value,
}: {
  emptyLabel: string;
  showRaw: boolean;
  truncation: ReactNode;
  value: unknown;
}) {
  if (value === undefined) {
    return <InspectorEmptyState>{emptyLabel}</InspectorEmptyState>;
  }

  return (
    <>
      {truncation}
      {showRaw ? <PayloadInspector data={value} /> : <ReadablePayloadView value={value} />}
    </>
  );
}
