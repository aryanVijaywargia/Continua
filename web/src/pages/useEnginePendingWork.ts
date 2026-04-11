import { useQuery } from '@tanstack/react-query';
import {
  fetchEnginePendingWork,
  type EnginePendingWorkResponse,
  type EngineRunStatus,
} from '../api/client';
import { TIMELINE_POLL_INTERVAL_MS } from './useTraceTimeline';

const ENGINE_PENDING_WORK_POLL_STATUSES: ReadonlySet<EngineRunStatus> = new Set([
  'QUEUED',
  'RUNNING',
  'WAITING',
  'SUSPENDED',
]);

export function shouldPollEnginePendingWork(
  engineStatus?: EngineRunStatus | null
): boolean {
  return Boolean(
    engineStatus && ENGINE_PENDING_WORK_POLL_STATUSES.has(engineStatus)
  );
}

export function useEnginePendingWork(
  runId: string | undefined,
  engineStatus: EngineRunStatus | undefined
) {
  const pollingEnabled = shouldPollEnginePendingWork(engineStatus);

  return useQuery<EnginePendingWorkResponse>({
    queryKey: ['enginePendingWork', runId],
    queryFn: () => {
      if (!runId) {
        throw new Error('Engine run ID is required to fetch pending work');
      }

      return fetchEnginePendingWork(runId);
    },
    enabled: Boolean(runId),
    refetchInterval: pollingEnabled ? TIMELINE_POLL_INTERVAL_MS : false,
    refetchOnWindowFocus: false,
  });
}
