import { startTransition, useEffect, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  fetchTimelineEvents,
  TimelineEvent,
  TimelineTraceStatus,
} from '../api/client';
import { mergeTimelineEvents } from '../utils/timeline';

const TIMELINE_PAGE_LIMIT = 100;
const TIMELINE_POLL_INTERVAL_MS = 3000;

interface TimelineSnapshot {
  events: TimelineEvent[];
  traceStatus: TimelineTraceStatus;
  pollCursor: string | null;
}

export function useTraceTimeline(traceId: string) {
  const [timelineSnapshot, setTimelineSnapshot] = useState<TimelineSnapshot | null>(null);
  const [needsTerminalRefresh, setNeedsTerminalRefresh] = useState(false);
  const queryClient = useQueryClient();

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
      void Promise.all([
        queryClient.invalidateQueries({ queryKey: ['trace', traceId] }),
        queryClient.invalidateQueries({ queryKey: ['spans', traceId] }),
      ]);
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
  }, [queryClient, timelinePollQuery.data, traceId]);

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

  return {
    events: timelineSnapshot?.events ?? [],
    traceStatus: timelineSnapshot?.traceStatus ?? null,
    hasSnapshot: timelineSnapshot !== null,
    isLive: pollingEnabled,
    isLoading: !timelineSnapshot && timelineBootstrapQuery.isLoading,
    error: resolveTimelineError(
      timelineSnapshot,
      timelineBootstrapQuery.error,
      timelinePollQuery.error,
      timelineTerminalRefreshQuery.error
    ),
  };
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
