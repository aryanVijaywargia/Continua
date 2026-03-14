import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { Link, useLocation, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { fetchSpans, fetchTrace, type Span } from '../api/client';
import { CopyButton } from '../components/CopyButton';
import { FailureSummary } from '../components/FailureSummary';
import { JsonViewer } from '../components/JsonViewer';
import { SpanDetail } from '../components/SpanDetail';
import { SpanTree } from '../components/SpanTree';
import { StatusBadge } from '../components/StatusBadge';
import { Timeline } from '../components/Timeline';
import { useRequireApiKey } from '../hooks/useRequireApiKey';
import { useTraceDetailSearchParams } from '../hooks/useTraceDetailSearchParams';
import { useTraceTimeline } from './useTraceTimeline';
import {
  calculateDuration,
  formatCost,
  formatDuration,
  formatRelativeTime,
  formatTokens,
  formatTimestamp,
} from '../utils/format';
import {
  buildBreadcrumbPath,
  buildFailureAnalysis,
  buildSpanIndex,
  evaluateStaleTraceSignal,
  type StaleTraceSignal,
} from '../utils/failureAnalysis';
import { serializeSpanParam } from '../utils/traceDetailSearchParams';

/**
 * Trace detail page with span tree, detail panel, and merged event timeline.
 */
const EMPTY_SPANS: Span[] = [];
const EMPTY_STALE_TRACE_SIGNAL: StaleTraceSignal = {
  shouldDisplay: false,
  latestActivityAt: null,
  runtimeMs: null,
  inactivityMs: null,
};

export function TraceDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { hasApiKey, prompt } = useRequireApiKey();

  if (!hasApiKey) {
    return prompt;
  }

  if (!id) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-red-600">Trace ID is required</div>
      </div>
    );
  }

  return (
    <TraceDetailContent
      key={id}
      traceId={id}
    />
  );
}

interface TraceDetailContentProps {
  traceId: string;
}

function getReturnToDestination(state: unknown): string {
  if (
    typeof state !== 'object' ||
    state === null ||
    !('returnTo' in state) ||
    typeof state.returnTo !== 'string'
  ) {
    return '/traces';
  }

  const { returnTo } = state;
  return returnTo === '/traces' || returnTo.startsWith('/traces?')
    ? returnTo
    : '/traces';
}

function TraceDetailContent({
  traceId,
}: TraceDetailContentProps) {
  const location = useLocation();
  const { spanParam, setSpanParam } = useTraceDetailSearchParams();
  const [selectedSpanExternalId, setSelectedSpanExternalId] = useState<string | null>(
    null
  );
  const [revealPath, setRevealPath] = useState<Set<string>>(new Set());
  const [revealPathVersion, setRevealPathVersion] = useState(0);
  const [userHasSelected, setUserHasSelected] = useState(false);
  const traceQuery = useQuery({
    queryKey: ['trace', traceId],
    queryFn: () => fetchTrace(traceId),
  });

  const spansQuery = useQuery({
    queryKey: ['spans', traceId],
    queryFn: () => fetchSpans(traceId),
  });
  const timeline = useTraceTimeline(traceId);
  const trace = traceQuery.data ?? null;
  const spans = spansQuery.data?.spans ?? EMPTY_SPANS;
  const timelineStatus = trace ? timeline.traceStatus ?? trace.status : timeline.traceStatus;
  const duration = trace ? calculateDuration(trace.started_at, trace.ended_at) : null;
  const totalTokens = trace
    ? (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0)
    : 0;
  const returnTo = getReturnToDestination(location.state);
  const spanIndex = useMemo(() => buildSpanIndex(spans), [spans]);
  const failureAnalysis = useMemo(
    () => buildFailureAnalysis(spans, timeline.events, spanIndex),
    [spanIndex, spans, timeline.events]
  );
  const selectedSpan = selectedSpanExternalId
    ? spanIndex.get(selectedSpanExternalId) ?? null
    : null;
  const selectedBreadcrumbPath = useMemo(
    () => buildBreadcrumbPath(selectedSpan, spanIndex),
    [selectedSpan, spanIndex]
  );
  const staleTraceSignal = useMemo(
    () =>
      timeline.hasSnapshot
        ? evaluateStaleTraceSignal({
            traceStatus: timelineStatus,
            traceStartedAt: trace?.started_at,
            spans,
            events: timeline.events,
          })
        : EMPTY_STALE_TRACE_SIGNAL,
    [spans, timeline.events, timeline.hasSnapshot, timelineStatus, trace?.started_at]
  );

  const updateSelectedSpan = useCallback((spanId: string | null) => {
    setSelectedSpanExternalId(spanId);

    if (spanId) {
      const breadcrumbPath = buildBreadcrumbPath(spanIndex.get(spanId), spanIndex);
      setRevealPath(new Set(breadcrumbPath.map((segment) => segment.spanId)));
      setRevealPathVersion((version) => version + 1);
      return;
    }

    setRevealPath(new Set());
  }, [spanIndex]);

  const handleSelectSpan = useCallback((spanId: string) => {
    updateSelectedSpan(spanId);
    setUserHasSelected(true);
    setSpanParam(spanId);
  }, [setSpanParam, updateSelectedSpan]);

  const buildCopyTraceUrl = useCallback(() => {
    const searchParams = serializeSpanParam(
      new URLSearchParams(location.search),
      selectedSpanExternalId
    );
    const nextSearch = searchParams.toString();

    return new URL(
      `${location.pathname}${nextSearch ? `?${nextSearch}` : ''}`,
      window.location.origin
    ).toString();
  }, [location.pathname, location.search, selectedSpanExternalId]);

  useEffect(() => {
    if (!spansQuery.isSuccess) {
      return;
    }

    if (spanParam !== null) {
      if (spanIndex.has(spanParam)) {
        if (selectedSpanExternalId !== spanParam || !userHasSelected) {
          updateSelectedSpan(spanParam);
        }
        if (!userHasSelected) {
          setUserHasSelected(true);
        }
        return;
      }

      updateSelectedSpan(null);
      setUserHasSelected(false);
      setSpanParam(null);
      return;
    }

    if (!userHasSelected) {
      return;
    }

    updateSelectedSpan(null);
    setUserHasSelected(false);
  }, [
    selectedSpanExternalId,
    setSpanParam,
    spanIndex,
    spanParam,
    spansQuery.isSuccess,
    updateSelectedSpan,
    userHasSelected,
  ]);

  useEffect(() => {
    if (!selectedSpanExternalId || spanIndex.has(selectedSpanExternalId)) {
      return;
    }

    updateSelectedSpan(null);
    setUserHasSelected(false);

    if (spanParam !== null) {
      setSpanParam(null);
    }
  }, [
    selectedSpanExternalId,
    setSpanParam,
    spanIndex,
    spanParam,
    updateSelectedSpan,
  ]);

  useEffect(() => {
    if (timelineStatus !== 'FAILED' || userHasSelected || spanParam !== null) {
      return;
    }

    const nextSelectedSpanId =
      failureAnalysis.summary.primaryFailedSpan?.span_id ?? null;

    if (selectedSpanExternalId === nextSelectedSpanId) {
      return;
    }

    updateSelectedSpan(nextSelectedSpanId);
  }, [
    failureAnalysis.summary.primaryFailedSpan,
    selectedSpanExternalId,
    spanParam,
    timelineStatus,
    updateSelectedSpan,
    userHasSelected,
  ]);

  if (traceQuery.isLoading || spansQuery.isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-gray-500">Loading trace...</div>
      </div>
    );
  }

  if (traceQuery.error) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-red-600">
          Error loading trace:{' '}
          {traceQuery.error instanceof Error
            ? traceQuery.error.message
            : 'Unknown error'}
        </div>
      </div>
    );
  }

  if (spansQuery.error) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-red-600">
          Error loading spans:{' '}
          {spansQuery.error instanceof Error
            ? spansQuery.error.message
            : 'Unknown error'}
        </div>
      </div>
    );
  }

  if (!trace) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-red-600">Trace not found</div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen flex-col bg-gray-50">
      <header className="border-b bg-white px-6 py-4">
        <div className="flex items-center gap-4">
          <Link to={returnTo} className="text-gray-500 hover:text-gray-700">
            ← Traces
          </Link>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="truncate text-xl font-semibold text-gray-900">
                {trace.name}
              </h1>
              <StatusBadge status={timelineStatus!} />
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-4 text-sm text-gray-500">
              <span>{formatDuration(duration)}</span>
              <span>{formatTokens(totalTokens)} tokens</span>
              <span>{formatCost(trace.total_cost_usd)}</span>
              {trace.error_count && trace.error_count > 0 && (
                <span className="text-red-600">{trace.error_count} errors</span>
              )}
            </div>
          </div>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto p-4">
        <div className="mx-auto flex max-w-7xl flex-col gap-4">
          {timelineStatus === 'FAILED' && (
            <FailureSummary
              summary={failureAnalysis.summary}
              onJumpToPrimaryFailedSpan={handleSelectSpan}
            />
          )}

          {staleTraceSignal.shouldDisplay && (
            <section className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900 shadow-sm">
              <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-amber-800">
                Experimental stale trace signal
              </div>
              <p className="mt-2">
                Still marked running. Recent activity is sparse, so this trace may
                be stale or incomplete.
              </p>
              {staleTraceSignal.latestActivityAt && (
                <p className="mt-2 text-xs text-amber-800">
                  Latest activity: {formatTimestamp(staleTraceSignal.latestActivityAt)} (
                  {formatRelativeTime(staleTraceSignal.latestActivityAt)})
                </p>
              )}
            </section>
          )}

          <section className="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 bg-gray-50 px-4 py-3">
              <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-600">
                Trace Context
              </h2>
              <CopyButton
                aria-label="Copy Trace URL"
                className="shrink-0"
                getValue={buildCopyTraceUrl}
                idleLabel="Copy Trace URL"
                successLabel="Copied URL"
              />
            </div>
            <div className="space-y-6 p-4">
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
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
                  label="Session UUID"
                  value={trace.session_id ? (
                    <Link
                      to={`/sessions/${trace.session_id}`}
                      className="font-mono text-xs text-blue-600 hover:text-blue-800"
                    >
                      {trace.session_id}
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
                  className="md:col-span-2 xl:col-span-3"
                  value={trace.tags && trace.tags.length > 0 ? (
                    <div className="flex flex-wrap gap-2">
                      {trace.tags.map((tag) => (
                        <span
                          key={tag}
                          className="rounded-full border border-gray-200 bg-white px-3 py-1 font-mono text-xs text-gray-700"
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

              {(trace.input !== undefined || trace.output !== undefined) && (
                <div className="grid gap-4 xl:grid-cols-2">
                  {trace.input !== undefined && (
                    <TracePayloadPanel title="Input" data={trace.input} />
                  )}
                  {trace.output !== undefined && (
                    <TracePayloadPanel title="Output" data={trace.output} />
                  )}
                </div>
              )}
            </div>
          </section>

          <div className="grid gap-4 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
            <section className="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
              <div className="border-b border-gray-200 bg-gray-50 px-4 py-3">
                <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-600">
                  Spans ({spans.length})
                </h2>
              </div>
              <div className="h-[32rem] overflow-y-auto">
                <SpanTree
                  spans={spans}
                  selectedSpanId={selectedSpanExternalId}
                  onSelectSpan={handleSelectSpan}
                  failedSpanIds={failureAnalysis.failedSpanIds}
                  primaryAncestorPath={failureAnalysis.primaryAncestorPath}
                  revealPath={revealPath}
                  revealKey={revealPathVersion}
                  inlineErrorPreviews={failureAnalysis.inlineErrorPreviews}
                />
              </div>
            </section>

            <section className="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
              <div className="border-b border-gray-200 bg-gray-50 px-4 py-3">
                <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-600">
                  {selectedSpan ? selectedSpan.name : 'Span Details'}
                </h2>
              </div>
              <div className="h-[32rem]">
                <SpanDetail
                  span={selectedSpan}
                  breadcrumbPath={selectedBreadcrumbPath}
                  onSelectSpan={handleSelectSpan}
                  spanIndex={spanIndex}
                />
              </div>
            </section>
          </div>

          <Timeline
            events={timeline.events}
            traceStatus={timelineStatus}
            isLive={timeline.isLive}
            isLoading={timeline.isLoading}
            error={timeline.error}
            selectedSpanId={selectedSpanExternalId}
            onSelectSpan={handleSelectSpan}
            spanIndex={spanIndex}
          />
        </div>
      </div>
    </div>
  );
}

interface TraceContextFieldProps {
  label: string;
  value: ReactNode;
  className?: string;
  copyValue?: string;
  copyButtonLabel?: string;
}

function TraceContextField({
  label,
  value,
  className = '',
  copyValue,
  copyButtonLabel,
}: TraceContextFieldProps) {
  return (
    <div className={`rounded-lg border border-gray-200 bg-gray-50 p-4 ${className}`.trim()}>
      <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-gray-500">
        {label}
      </div>
      <div className="mt-2 flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1 text-sm text-gray-900">{value}</div>
        {copyValue && copyButtonLabel ? (
          <CopyButton
            aria-label={copyButtonLabel}
            className="shrink-0"
            value={copyValue}
          />
        ) : null}
      </div>
    </div>
  );
}

function TracePayloadPanel({ title, data }: { title: string; data: unknown }) {
  return (
    <div className="rounded-lg border border-gray-200 bg-gray-50 p-4">
      <h3 className="mb-2 text-sm font-medium text-gray-700">{title}</h3>
      <JsonViewer data={data} className="max-h-80 overflow-y-auto bg-white" />
    </div>
  );
}

function renderContextText(value: string | undefined, monospace = false) {
  if (value === undefined) {
    return <span className="text-sm text-gray-400">-</span>;
  }

  return (
    <span className={monospace ? 'font-mono text-xs text-gray-900' : 'text-sm text-gray-900'}>
      {value}
    </span>
  );
}
