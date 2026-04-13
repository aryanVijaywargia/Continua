import { useRef, useState, type KeyboardEvent } from 'react';
import { Span, TimelineEvent, TimelineTraceStatus } from '../api/client';
import { JsonViewer } from './JsonViewer';
import {
  formatInlineSemanticValue,
  getDecisionDetails,
  getEffectDetails,
  getSnapshotMarkerDetails,
  getStateChangeDetails,
  getWaitDetails,
} from '../utils/eventSemantics';
import {
  formatTimelineTime,
  isTimelineErrorEvent,
  summarizeTimelineEvent,
} from '../utils/timeline';
import {
  classifyEffectEvent,
  getAccessibleSummary,
  type RetrySafetyAssessment,
} from '../utils/retrySafety';
import { RetrySafetyBadge } from './RetrySafetyBadge';

interface TimelineProps {
  events: TimelineEvent[];
  traceStatus: TimelineTraceStatus | null;
  isLive: boolean;
  isLoading?: boolean;
  error?: string | null;
  selectedSpanId?: string | null;
  onSelectSpan: (spanId: string) => void;
  spanIndex: Map<string, Span>;
}

type TimelineFilterMode = 'all' | 'semantic' | 'effects-waits';

const TIMELINE_FILTER_OPTIONS: Array<{
  label: string;
  mode: TimelineFilterMode;
}> = [
  { label: 'All', mode: 'all' },
  { label: 'Semantic', mode: 'semantic' },
  { label: 'Effects & waits', mode: 'effects-waits' },
];

const SEMANTIC_EVENT_TYPES = new Set<TimelineEvent['event_type']>([
  'state_change',
  'decision',
  'effect',
  'wait',
  'snapshot_marker',
]);
const EFFECT_WAIT_EVENT_TYPES = new Set<TimelineEvent['event_type']>([
  'effect',
  'wait',
]);

/**
 * Chronological timeline view for trace events and synthetic lifecycle markers.
 */
export function Timeline({
  events,
  traceStatus,
  isLive,
  isLoading = false,
  error = null,
  selectedSpanId = null,
  onSelectSpan,
  spanIndex,
}: TimelineProps) {
  const [showErrorsOnly, setShowErrorsOnly] = useState(false);
  const [filterMode, setFilterMode] = useState<TimelineFilterMode>('all');
  const visibleEvents = events.filter(
    (event) =>
      matchesTimelineFilterMode(event, filterMode) &&
      (!showErrorsOnly || isTimelineErrorEvent(event))
  );

  return (
    <section className="overflow-hidden rounded-[1rem] border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-4 py-3">
        <div>
          <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-[var(--continua-text-secondary)]">
            Timeline
          </h2>
          <p className="mt-1 text-sm text-[var(--continua-text-muted)]">
            Chronological trace events with lifecycle markers and payload inspection.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <SegmentedFilter filterMode={filterMode} onChange={setFilterMode} />
          <button
            type="button"
            className={`rounded-full border px-3 py-1 text-xs font-medium transition focus:outline-none focus:ring-2 focus:ring-blue-200 ${
              showErrorsOnly
                ? 'border-[var(--continua-error-border)] bg-[var(--continua-error-faint)] text-[var(--continua-error)]'
                : 'border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)] hover:bg-[var(--continua-surface-muted)]'
            }`}
            aria-label="Show error events only"
            aria-pressed={showErrorsOnly}
            onClick={() => setShowErrorsOnly((active) => !active)}
          >
            Errors only
          </button>
          <div className="flex items-center gap-2 rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-1 text-xs font-medium text-[var(--continua-text-secondary)]">
            <span
              className={`h-2.5 w-2.5 rounded-full ${
                isLive
                  ? 'animate-pulse bg-emerald-500'
                  : traceStatus === 'FAILED'
                    ? 'bg-red-500'
                    : 'bg-[var(--continua-text-muted)]'
              }`}
            />
            <span>{timelineStatusLabel(traceStatus, isLive)}</span>
          </div>
        </div>
      </div>

      {isLoading && events.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-[var(--continua-text-muted)]">
          Loading timeline...
        </div>
      ) : error && events.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-[var(--continua-error)]">{error}</div>
      ) : visibleEvents.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-[var(--continua-text-muted)]">
          {getTimelineEmptyStateMessage(filterMode, showErrorsOnly)}
        </div>
      ) : (
        <div className="divide-y divide-[var(--continua-border-soft)]">
          {visibleEvents.map((event) => (
            <TimelineRow
              key={event.id}
              event={event}
              isSelectedSpan={selectedSpanId === event.span_id}
              onSelectSpan={onSelectSpan}
              spanIndex={spanIndex}
              traceStatus={traceStatus}
            />
          ))}
        </div>
      )}
    </section>
  );
}

interface TimelineRowProps {
  event: TimelineEvent;
  isSelectedSpan: boolean;
  onSelectSpan: (spanId: string) => void;
  spanIndex: Map<string, Span>;
  traceStatus: TimelineTraceStatus | null;
}

function TimelineRow({
  event,
  isSelectedSpan,
  onSelectSpan,
  spanIndex,
  traceStatus,
}: TimelineRowProps) {
  const [isExpanded, setIsExpanded] = useState(false);
  const hasDetails = Boolean(event.message || event.payload);
  const hasNavigableSpan = Boolean(event.span_id && spanIndex.has(event.span_id));
  const isError = isTimelineErrorEvent(event);
  const stateChange = getStateChangeDetails(event);
  const decision = getDecisionDetails(event);
  const effectDetails = getEffectDetails(event);
  const retrySafety =
    traceStatus === 'FAILED' ? classifyEffectEvent(event) : null;
  const waitDetails = getWaitDetails(event);
  const snapshotMarker = getSnapshotMarkerDetails(event);
  const rowAccent = isError
    ? 'border-[var(--continua-error-border)] bg-[var(--continua-error-faint)]'
    : event.source === 'synthetic'
      ? 'border-amber-200 bg-amber-50/70 dark:border-amber-500/40 dark:bg-amber-500/10'
      : 'border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)]';

  return (
    <div className="p-4">
      <div className={`rounded-[1rem] border ${rowAccent} p-4 transition-colors`}>
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 gap-4">
            <div className="min-w-28 rounded-xl bg-[var(--continua-text-primary)] px-3 py-2 text-xs font-medium uppercase tracking-[0.18em] text-[var(--continua-surface-elevated)]">
              <div>{formatTimelineTime(event.timestamp)}</div>
              <div className="mt-1 text-[10px] tracking-[0.25em] text-[var(--continua-text-muted)]">
                {event.source}
              </div>
            </div>

            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <span
                  className={`rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.16em] ${
                    isError
                      ? 'bg-[var(--continua-error-faint)] text-[var(--continua-error)]'
                      : event.source === 'synthetic'
                        ? 'bg-amber-100 text-amber-700'
                        : 'bg-[var(--continua-accent-faint)] text-[var(--continua-accent)]'
                  }`}
                >
                  {formatEventType(event.event_type)}
                </span>
                {event.level && (
                  <span className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-2.5 py-1 text-[11px] font-medium uppercase tracking-[0.16em] text-[var(--continua-text-secondary)]">
                    {event.level}
                  </span>
                )}
              </div>

              {stateChange ? (
                <StateChangePreview stateChange={stateChange} />
              ) : decision ? (
                <DecisionPreview decision={decision} />
              ) : effectDetails ? (
                <EffectPreview
                  effect={effectDetails}
                  retrySafety={retrySafety}
                />
              ) : retrySafety ? (
                <MalformedEffectPreview
                  event={event}
                  retrySafety={retrySafety}
                />
              ) : waitDetails ? (
                <WaitPreview wait={waitDetails} />
              ) : snapshotMarker ? (
                <SnapshotMarkerPreview snapshotMarker={snapshotMarker} />
              ) : (
                <p className="mt-3 text-sm font-medium leading-6 text-[var(--continua-text-primary)]">
                  {summarizeTimelineEvent(event)}
                </p>
              )}

              {(event.span_name || event.span_id) && (
                <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-[var(--continua-text-muted)]">
                  <span className="uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
                    span
                  </span>
                  {hasNavigableSpan && event.span_id ? (
                    <button
                      type="button"
                      className={`rounded-full px-3 py-1 font-medium transition ${
                        isSelectedSpan
                          ? 'bg-[var(--continua-text-primary)] text-[var(--continua-surface-elevated)]'
                          : 'bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)] ring-1 ring-[var(--continua-border-soft)] hover:bg-[var(--continua-surface-muted)]'
                      }`}
                      onClick={() => onSelectSpan(event.span_id!)}
                    >
                      {event.span_name ?? event.span_id}
                    </button>
                  ) : (
                    <span className="rounded-full bg-[var(--continua-surface-elevated)] px-3 py-1 font-mono text-[var(--continua-text-secondary)] ring-1 ring-[var(--continua-border-soft)]">
                      {event.span_name ?? event.span_id}
                    </span>
                  )}
                </div>
              )}
            </div>
          </div>

          {hasDetails && (
            <button
              type="button"
              className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-1.5 text-xs font-medium text-[var(--continua-text-secondary)] transition hover:bg-[var(--continua-surface-muted)]"
              onClick={() => setIsExpanded((expanded) => !expanded)}
            >
              {isExpanded ? 'Hide details' : 'Show details'}
            </button>
          )}
        </div>

        {isExpanded && hasDetails && (
          <div className="mt-4 space-y-3 border-t border-[var(--continua-border-soft)] pt-4">
            {event.message && (
              <div>
                <div className="mb-1 text-xs font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
                  Message
                </div>
                <div className="rounded-xl border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-2 text-sm text-[var(--continua-text-secondary)]">
                  {event.message}
                </div>
              </div>
            )}
            {event.payload && (
              <div>
                <div className="mb-1 text-xs font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
                  Payload
                </div>
                <JsonViewer data={event.payload} className="max-h-80 overflow-y-auto" />
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function SegmentedFilter({
  filterMode,
  onChange,
}: {
  filterMode: TimelineFilterMode;
  onChange: (mode: TimelineFilterMode) => void;
}) {
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);

  const handleKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    index: number
  ) => {
    const nextIndex = getSegmentedFilterTargetIndex(event.key, index);
    if (nextIndex === null) {
      return;
    }

    event.preventDefault();
    onChange(TIMELINE_FILTER_OPTIONS[nextIndex].mode);
    optionRefs.current[nextIndex]?.focus();
  };

  return (
    <div
      role="radiogroup"
      aria-label="Timeline event filter"
      className="flex items-center gap-1 rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] p-1 text-xs font-medium text-[var(--continua-text-secondary)]"
    >
      {TIMELINE_FILTER_OPTIONS.map((option, index) => {
        const active = option.mode === filterMode;

        return (
          <button
            key={option.mode}
            ref={(element) => {
              optionRefs.current[index] = element;
            }}
            type="button"
            role="radio"
            aria-checked={active}
            tabIndex={active ? 0 : -1}
            className={`rounded-full px-3 py-1 transition focus:outline-none focus:ring-2 focus:ring-blue-200 ${
              active
                ? 'bg-[var(--continua-text-primary)] text-[var(--continua-surface-elevated)]'
                : 'text-[var(--continua-text-secondary)] hover:bg-[var(--continua-surface-muted)]'
            }`}
            onClick={() => onChange(option.mode)}
            onKeyDown={(event) => handleKeyDown(event, index)}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}

function StateChangePreview({
  stateChange,
}: {
  stateChange: NonNullable<ReturnType<typeof getStateChangeDetails>>;
}) {
  return (
    <div className="mt-3 space-y-2">
      {stateChange.namespace ? (
        <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
          {stateChange.namespace}
        </div>
      ) : null}
      <div className="flex flex-wrap items-center gap-2 text-sm font-medium text-[var(--continua-text-primary)]">
        <span>{stateChange.key}</span>
        <SemanticValuePill>
          {formatInlineSemanticValue(stateChange.oldValue)}
        </SemanticValuePill>
        <span className="text-[var(--continua-text-muted)]">→</span>
        <SemanticValuePill tone="accent">
          {formatInlineSemanticValue(stateChange.newValue)}
        </SemanticValuePill>
      </div>
    </div>
  );
}

function DecisionPreview({
  decision,
}: {
  decision: NonNullable<ReturnType<typeof getDecisionDetails>>;
}) {
  return (
    <div className="mt-3 space-y-2">
      <p className="text-sm font-medium leading-6 text-[var(--continua-text-primary)]">
        {decision.question}
      </p>
      <div className="flex flex-wrap items-center gap-2 text-sm text-[var(--continua-text-secondary)]">
        <span>Chosen</span>
        <SemanticValuePill tone="accent">
          {formatInlineSemanticValue(decision.chosen)}
        </SemanticValuePill>
      </div>
      {decision.alternatives && decision.alternatives.length > 0 ? (
        <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--continua-text-muted)]">
          <span>Alternatives</span>
          {decision.alternatives.map((alternative, index) => (
            <SemanticValuePill key={`${index}-${formatInlineSemanticValue(alternative)}`}>
              {formatInlineSemanticValue(alternative)}
            </SemanticValuePill>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function EffectPreview({
  effect,
  retrySafety,
}: {
  effect: NonNullable<ReturnType<typeof getEffectDetails>>;
  retrySafety: RetrySafetyAssessment | null;
}) {
  return (
    <div className="mt-3 space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-sm font-medium leading-6 text-[var(--continua-text-primary)]">
          {effect.effectKind}
        </p>
        {retrySafety ? (
          <RetrySafetyBadge
            classification={retrySafety.classification}
            variant="compact"
            aria-label={getAccessibleSummary(retrySafety.classification)}
          />
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--continua-text-muted)]">
        <SemanticValuePill tone={effect.hasExternalSideEffect ? 'accent' : 'neutral'}>
          {effect.hasExternalSideEffect ? 'mutating' : 'read-only'}
        </SemanticValuePill>
      </div>
    </div>
  );
}

function WaitPreview({
  wait,
}: {
  wait: NonNullable<ReturnType<typeof getWaitDetails>>;
}) {
  return (
    <div className="mt-3 space-y-2">
      <p className="text-sm font-medium leading-6 text-[var(--continua-text-primary)]">
        {wait.waitKind}
      </p>
      <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--continua-text-muted)]">
        <SemanticValuePill>{wait.phase}</SemanticValuePill>
        {wait.resolution ? (
          <SemanticValuePill tone="accent">{wait.resolution}</SemanticValuePill>
        ) : null}
      </div>
    </div>
  );
}

function SnapshotMarkerPreview({
  snapshotMarker,
}: {
  snapshotMarker: NonNullable<ReturnType<typeof getSnapshotMarkerDetails>>;
}) {
  return (
    <div className="mt-3 space-y-2">
      <p className="text-sm font-medium leading-6 text-[var(--continua-text-primary)]">
        {snapshotMarker.label}
      </p>
      <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--continua-text-muted)]">
        <SemanticValuePill tone="accent">{snapshotMarker.markerKind}</SemanticValuePill>
      </div>
    </div>
  );
}

function MalformedEffectPreview({
  event,
  retrySafety,
}: {
  event: TimelineEvent;
  retrySafety: RetrySafetyAssessment;
}) {
  return (
    <div className="mt-3 space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-sm font-medium leading-6 text-[var(--continua-text-primary)]">
          {summarizeTimelineEvent(event)}
        </p>
        <RetrySafetyBadge
          classification={retrySafety.classification}
          variant="compact"
          aria-label={getAccessibleSummary(retrySafety.classification)}
        />
      </div>
    </div>
  );
}

function SemanticValuePill({
  children,
  tone = 'neutral',
}: {
  children: string;
  tone?: 'neutral' | 'accent';
}) {
  return (
    <span
      className={`rounded-full border px-2.5 py-1 text-xs font-medium ${
        tone === 'accent'
          ? 'border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] text-[var(--continua-accent)]'
          : 'border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)]'
      }`}
    >
      {children}
    </span>
  );
}

function formatEventType(eventType: TimelineEvent['event_type']): string {
  return eventType.replace(/_/g, ' ');
}

function matchesTimelineFilterMode(
  event: TimelineEvent,
  filterMode: TimelineFilterMode
): boolean {
  if (filterMode === 'all') {
    return true;
  }

  if (event.source !== 'explicit') {
    return false;
  }

  if (filterMode === 'semantic') {
    return SEMANTIC_EVENT_TYPES.has(event.event_type);
  }

  return EFFECT_WAIT_EVENT_TYPES.has(event.event_type);
}

function getTimelineEmptyStateMessage(
  filterMode: TimelineFilterMode,
  showErrorsOnly: boolean
): string {
  if (showErrorsOnly) {
    if (filterMode === 'semantic') {
      return 'No error-level semantic events for this trace.';
    }
    if (filterMode === 'effects-waits') {
      return 'No error-level effect or wait events for this trace.';
    }
    return 'No error events for this trace.';
  }

  if (filterMode === 'semantic') {
    return 'No semantic events for this trace.';
  }
  if (filterMode === 'effects-waits') {
    return 'No effect or wait events for this trace.';
  }
  return 'No timeline events recorded for this trace yet.';
}

function getSegmentedFilterTargetIndex(
  key: string,
  currentIndex: number
): number | null {
  switch (key) {
    case 'ArrowRight':
    case 'ArrowDown':
      return (currentIndex + 1) % TIMELINE_FILTER_OPTIONS.length;
    case 'ArrowLeft':
    case 'ArrowUp':
      return (
        (currentIndex - 1 + TIMELINE_FILTER_OPTIONS.length) %
        TIMELINE_FILTER_OPTIONS.length
      );
    case 'Home':
      return 0;
    case 'End':
      return TIMELINE_FILTER_OPTIONS.length - 1;
    default:
      return null;
  }
}

function timelineStatusLabel(
  traceStatus: TimelineTraceStatus | null,
  isLive: boolean
): string {
  if (isLive) {
    return 'LIVE / polling every 3s';
  }
  if (traceStatus === 'FAILED') {
    return 'FAILED';
  }
  if (traceStatus === 'COMPLETED') {
    return 'COMPLETED';
  }
  return 'IDLE';
}
