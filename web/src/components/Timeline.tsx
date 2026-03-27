import { useRef, useState, type KeyboardEvent } from 'react';
import { Span, TimelineEvent, TimelineTraceStatus } from '../api/client';
import { JsonViewer } from './JsonViewer';
import {
  formatInlineSemanticValue,
  getDecisionDetails,
  getEffectDetails,
  getStateChangeDetails,
  getWaitDetails,
} from '../utils/eventSemantics';
import {
  isTimelineErrorEvent,
  summarizeTimelineEvent,
} from '../utils/timeline';

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
    <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-950/70">
        <div>
          <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-slate-600 dark:text-slate-300">
            Timeline
          </h2>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            Chronological trace events with lifecycle markers and payload inspection.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <SegmentedFilter filterMode={filterMode} onChange={setFilterMode} />
          <button
            type="button"
            className={`rounded-full border px-3 py-1 text-xs font-medium transition focus:outline-none focus:ring-2 focus:ring-blue-200 ${
              showErrorsOnly
                ? 'border-red-200 bg-red-50 text-red-700'
                : 'border-slate-200 bg-white text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200 dark:hover:bg-slate-800'
            }`}
            aria-label="Show error events only"
            aria-pressed={showErrorsOnly}
            onClick={() => setShowErrorsOnly((active) => !active)}
          >
            Errors only
          </button>
          <div className="flex items-center gap-2 rounded-full border border-slate-200 bg-white px-3 py-1 text-xs font-medium text-slate-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200">
            <span
              className={`h-2.5 w-2.5 rounded-full ${
                isLive
                  ? 'animate-pulse bg-emerald-500'
                  : traceStatus === 'FAILED'
                    ? 'bg-red-500'
                    : 'bg-slate-400 dark:bg-slate-500'
              }`}
            />
            <span>{timelineStatusLabel(traceStatus, isLive)}</span>
          </div>
        </div>
      </div>

      {isLoading && events.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-slate-500 dark:text-slate-400">
          Loading timeline...
        </div>
      ) : error && events.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-red-600">{error}</div>
      ) : visibleEvents.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-slate-500 dark:text-slate-400">
          {getTimelineEmptyStateMessage(filterMode, showErrorsOnly)}
        </div>
      ) : (
        <div className="divide-y divide-slate-100 dark:divide-slate-800">
          {visibleEvents.map((event) => (
            <TimelineRow
              key={event.id}
              event={event}
              isSelectedSpan={selectedSpanId === event.span_id}
              onSelectSpan={onSelectSpan}
              spanIndex={spanIndex}
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
}

function TimelineRow({
  event,
  isSelectedSpan,
  onSelectSpan,
  spanIndex,
}: TimelineRowProps) {
  const [isExpanded, setIsExpanded] = useState(false);
  const hasDetails = Boolean(event.message || event.payload);
  const hasNavigableSpan = Boolean(event.span_id && spanIndex.has(event.span_id));
  const isError = isTimelineErrorEvent(event);
  const stateChange = getStateChangeDetails(event);
  const decision = getDecisionDetails(event);
  const effectDetails = getEffectDetails(event);
  const waitDetails = getWaitDetails(event);
  const rowAccent = isError
    ? 'border-red-200 bg-red-50/70 dark:border-red-500/40 dark:bg-red-500/10'
    : event.source === 'synthetic'
      ? 'border-amber-200 bg-amber-50/70 dark:border-amber-500/40 dark:bg-amber-500/10'
      : 'border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-900';

  return (
    <div className="p-4">
      <div className={`rounded-2xl border ${rowAccent} p-4 transition-colors`}>
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 gap-4">
            <div className="min-w-28 rounded-xl bg-slate-900 px-3 py-2 text-xs font-medium uppercase tracking-[0.18em] text-white dark:bg-slate-100 dark:text-slate-950">
              <div>{formatTimelineTime(event.timestamp)}</div>
              <div className="mt-1 text-[10px] tracking-[0.25em] text-slate-300 dark:text-slate-600">
                {event.source}
              </div>
            </div>

            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <span
                  className={`rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.16em] ${
                    isError
                      ? 'bg-red-100 text-red-700'
                      : event.source === 'synthetic'
                        ? 'bg-amber-100 text-amber-700'
                        : 'bg-blue-100 text-blue-700 dark:bg-sky-500/15 dark:text-sky-200'
                  }`}
                >
                  {formatEventType(event.event_type)}
                </span>
                {event.level && (
                  <span className="rounded-full border border-slate-200 bg-white px-2.5 py-1 text-[11px] font-medium uppercase tracking-[0.16em] text-slate-600 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-300">
                    {event.level}
                  </span>
                )}
              </div>

              {stateChange ? (
                <StateChangePreview stateChange={stateChange} />
              ) : decision ? (
                <DecisionPreview decision={decision} />
              ) : effectDetails ? (
                <EffectPreview effect={effectDetails} />
              ) : waitDetails ? (
                <WaitPreview wait={waitDetails} />
              ) : (
                <p className="mt-3 text-sm font-medium leading-6 text-slate-900 dark:text-slate-100">
                  {summarizeTimelineEvent(event)}
                </p>
              )}

              {(event.span_name || event.span_id) && (
                <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
                  <span className="uppercase tracking-[0.18em] text-slate-400 dark:text-slate-500">
                    span
                  </span>
                  {hasNavigableSpan && event.span_id ? (
                    <button
                      type="button"
                      className={`rounded-full px-3 py-1 font-medium transition ${
                        isSelectedSpan
                          ? 'bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-950'
                          : 'bg-white text-slate-700 ring-1 ring-slate-200 hover:bg-slate-100 dark:bg-slate-950 dark:text-slate-200 dark:ring-slate-700 dark:hover:bg-slate-800'
                      }`}
                      onClick={() => onSelectSpan(event.span_id!)}
                    >
                      {event.span_name ?? event.span_id}
                    </button>
                  ) : (
                    <span className="rounded-full bg-white px-3 py-1 font-mono text-slate-600 ring-1 ring-slate-200 dark:bg-slate-950 dark:text-slate-300 dark:ring-slate-700">
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
              className="rounded-full border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-600 transition hover:bg-slate-100 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200 dark:hover:bg-slate-800"
              onClick={() => setIsExpanded((expanded) => !expanded)}
            >
              {isExpanded ? 'Hide details' : 'Show details'}
            </button>
          )}
        </div>

        {isExpanded && hasDetails && (
          <div className="mt-4 space-y-3 border-t border-slate-200 pt-4 dark:border-slate-700">
            {event.message && (
              <div>
                <div className="mb-1 text-xs font-semibold uppercase tracking-[0.18em] text-slate-500 dark:text-slate-400">
                  Message
                </div>
                <div className="rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm text-slate-700 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200">
                  {event.message}
                </div>
              </div>
            )}
            {event.payload && (
              <div>
                <div className="mb-1 text-xs font-semibold uppercase tracking-[0.18em] text-slate-500 dark:text-slate-400">
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
      className="flex items-center gap-1 rounded-full border border-slate-200 bg-white p-1 text-xs font-medium text-slate-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200"
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
                ? 'bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-950'
                : 'text-slate-600 hover:bg-slate-100 dark:text-slate-200 dark:hover:bg-slate-800'
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
        <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
          {stateChange.namespace}
        </div>
      ) : null}
      <div className="flex flex-wrap items-center gap-2 text-sm font-medium text-slate-900 dark:text-slate-100">
        <span>{stateChange.key}</span>
        <SemanticValuePill>
          {formatInlineSemanticValue(stateChange.oldValue)}
        </SemanticValuePill>
        <span className="text-slate-400 dark:text-slate-500">→</span>
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
      <p className="text-sm font-medium leading-6 text-slate-900 dark:text-slate-100">
        {decision.question}
      </p>
      <div className="flex flex-wrap items-center gap-2 text-sm text-slate-600 dark:text-slate-300">
        <span>Chosen</span>
        <SemanticValuePill tone="accent">
          {formatInlineSemanticValue(decision.chosen)}
        </SemanticValuePill>
      </div>
      {decision.alternatives && decision.alternatives.length > 0 ? (
        <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
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
}: {
  effect: NonNullable<ReturnType<typeof getEffectDetails>>;
}) {
  return (
    <div className="mt-3 space-y-2">
      <p className="text-sm font-medium leading-6 text-slate-900 dark:text-slate-100">
        {effect.effectKind}
      </p>
      <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
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
      <p className="text-sm font-medium leading-6 text-slate-900 dark:text-slate-100">
        {wait.waitKind}
      </p>
      <div className="flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
        <SemanticValuePill>{wait.phase}</SemanticValuePill>
        {wait.resolution ? (
          <SemanticValuePill tone="accent">{wait.resolution}</SemanticValuePill>
        ) : null}
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
          ? 'border-blue-200 bg-blue-100 text-blue-800 dark:border-sky-500/40 dark:bg-sky-500/15 dark:text-sky-200'
          : 'border-slate-200 bg-white text-slate-700 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200'
      }`}
    >
      {children}
    </span>
  );
}

function formatTimelineTime(timestamp: string): string {
  return new Date(timestamp).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
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
