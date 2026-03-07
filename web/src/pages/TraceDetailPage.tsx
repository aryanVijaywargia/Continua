import { startTransition, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  fetchSpans,
  fetchTimelineEvents,
  fetchTrace,
  getApiKey,
  Span,
  TimelineEvent,
  TimelineTraceStatus,
} from '../api/client';
import { ApiKeyPrompt } from '../components/ApiKeyPrompt';
import { SpanDetail } from '../components/SpanDetail';
import { SpanTree } from '../components/SpanTree';
import { StatusBadge } from '../components/StatusBadge';
import { Timeline } from '../components/Timeline';
import {
  calculateDuration,
  formatCost,
  formatDuration,
  formatTokens,
} from '../utils/format';
import { mergeTimelineEvents } from '../utils/timeline';

const TIMELINE_PAGE_LIMIT = 100;
const TIMELINE_POLL_INTERVAL_MS = 3000;

interface TimelineSnapshot {
  events: TimelineEvent[];
  traceStatus: TimelineTraceStatus;
  pollCursor: string | null;
}

/**
 * Trace detail page with span tree, detail panel, and merged event timeline.
 */
export function TraceDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [hasApiKey, setHasApiKey] = useState(() => !!getApiKey());
  const [selectedSpan, setSelectedSpan] = useState<Span | null>(null);

  if (!hasApiKey) {
    return <ApiKeyPrompt onSubmit={() => setHasApiKey(true)} />;
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
      traceId={id}
      selectedSpan={selectedSpan}
      onSelectSpan={setSelectedSpan}
    />
  );
}

interface TraceDetailContentProps {
  traceId: string;
  selectedSpan: Span | null;
  onSelectSpan: (span: Span | null) => void;
}

function TraceDetailContent({
  traceId,
  selectedSpan,
  onSelectSpan,
}: TraceDetailContentProps) {
  const [timelineSnapshot, setTimelineSnapshot] = useState<TimelineSnapshot | null>(null);
  const [needsTerminalRefresh, setNeedsTerminalRefresh] = useState(false);

  const traceQuery = useQuery({
    queryKey: ['trace', traceId],
    queryFn: () => fetchTrace(traceId),
  });

  const spansQuery = useQuery({
    queryKey: ['spans', traceId],
    queryFn: () => fetchSpans(traceId),
  });

  const timelineBootstrapQuery = useQuery({
    queryKey: ['timeline', traceId, 'bootstrap'],
    queryFn: () => fetchFullTimelineSnapshot(traceId),
    refetchOnWindowFocus: false,
  });

  useEffect(() => {
    if (!timelineBootstrapQuery.data) {
      return;
    }

    startTransition(() => {
      setTimelineSnapshot(timelineBootstrapQuery.data);
      setNeedsTerminalRefresh(false);
    });
  }, [timelineBootstrapQuery.data]);

  const pollingEnabled =
    timelineSnapshot?.traceStatus === 'RUNNING' &&
    !needsTerminalRefresh;

  const timelinePollQuery = useQuery({
    queryKey: ['timeline', traceId, 'poll', timelineSnapshot?.pollCursor ?? 'head'],
    queryFn: () =>
      fetchTimelineEvents(traceId, {
        after: timelineSnapshot?.pollCursor ?? undefined,
        limit: TIMELINE_PAGE_LIMIT,
      }),
    enabled: pollingEnabled,
    refetchInterval: pollingEnabled ? TIMELINE_POLL_INTERVAL_MS : false,
    refetchOnWindowFocus: false,
  });

  useEffect(() => {
    if (!timelinePollQuery.data) {
      return;
    }

    const pollResult = timelinePollQuery.data;

    if (pollResult.trace_status !== 'RUNNING') {
      setNeedsTerminalRefresh(true);
    }

    startTransition(() => {
      setTimelineSnapshot((current) => {
        if (!current) {
          return current;
        }

        return {
          events: mergeTimelineEvents(current.events, pollResult.events),
          traceStatus: pollResult.trace_status,
          pollCursor: pollResult.poll_cursor ?? current.pollCursor,
        };
      });
    });
  }, [timelinePollQuery.data]);

  const timelineTerminalRefreshQuery = useQuery({
    queryKey: ['timeline', traceId, 'terminal-refresh', needsTerminalRefresh],
    queryFn: () => fetchFullTimelineSnapshot(traceId),
    enabled: needsTerminalRefresh,
    refetchOnWindowFocus: false,
    retry: false,
  });

  useEffect(() => {
    if (!timelineTerminalRefreshQuery.data) {
      return;
    }

    startTransition(() => {
      setTimelineSnapshot(timelineTerminalRefreshQuery.data);
      setNeedsTerminalRefresh(false);
    });
  }, [timelineTerminalRefreshQuery.data]);

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

  const trace = traceQuery.data;
  const spans = spansQuery.data?.spans ?? [];

  if (!trace) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-red-600">Trace not found</div>
      </div>
    );
  }

  const timelineStatus = timelineSnapshot?.traceStatus ?? trace.status;
  const timelineEvents = timelineSnapshot?.events ?? [];
  const duration = calculateDuration(trace.started_at, trace.ended_at);
  const totalTokens =
    (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);

  return (
    <div className="flex min-h-screen flex-col bg-gray-50">
      <header className="border-b bg-white px-6 py-4">
        <div className="flex items-center gap-4">
          <Link to="/traces" className="text-gray-500 hover:text-gray-700">
            ← Traces
          </Link>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="truncate text-xl font-semibold text-gray-900">
                {trace.name}
              </h1>
              <StatusBadge status={timelineStatus} />
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
                  selectedSpanId={selectedSpan?.id ?? null}
                  onSelectSpan={onSelectSpan}
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
                <SpanDetail span={selectedSpan} />
              </div>
            </section>
          </div>

          <Timeline
            events={timelineEvents}
            traceStatus={timelineStatus}
            isLive={pollingEnabled}
            isLoading={!timelineSnapshot && timelineBootstrapQuery.isLoading}
            error={resolveTimelineError(
              timelineSnapshot,
              timelineBootstrapQuery.error,
              timelinePollQuery.error,
              timelineTerminalRefreshQuery.error
            )}
            selectedSpanId={selectedSpan?.span_id ?? null}
            onSelectSpan={(spanId) => {
              const span = spans.find((candidate) => candidate.span_id === spanId) ?? null;
              if (span) {
                onSelectSpan(span);
              }
            }}
          />
        </div>
      </div>
    </div>
  );
}

async function fetchFullTimelineSnapshot(traceId: string): Promise<TimelineSnapshot> {
  let after: string | undefined;
  let hasMore = true;
  let pollCursor: string | null = null;
  let traceStatus: TimelineTraceStatus = 'RUNNING';
  let events: TimelineEvent[] = [];

  while (hasMore) {
    const page = await fetchTimelineEvents(traceId, {
      after,
      limit: TIMELINE_PAGE_LIMIT,
    });

    events = mergeTimelineEvents(events, page.events);
    traceStatus = page.trace_status;

    if (page.poll_cursor) {
      pollCursor = page.poll_cursor;
    }
    hasMore = page.has_more && Boolean(page.next_cursor);
    if (hasMore) {
      after = page.next_cursor ?? undefined;
    }
  }

  return {
    events,
    traceStatus,
    pollCursor,
  };
}

function resolveTimelineError(
  timelineSnapshot: TimelineSnapshot | null,
  bootstrapError: unknown,
  pollError: unknown,
  terminalRefreshError: unknown
): string | null {
  if (!timelineSnapshot) {
    return queryErrorMessage(bootstrapError ?? terminalRefreshError);
  }

  return queryErrorMessage(pollError ?? terminalRefreshError);
}

function queryErrorMessage(error: unknown): string | null {
  if (!error) {
    return null;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return 'Unknown timeline error';
}
