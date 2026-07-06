import type { EngineRunStatus } from '../api/client';

export interface StateMachineStep {
  id: string;
  label: string;
  done: boolean;
  current: boolean;
  warn?: boolean;
  error?: boolean;
}

export function buildEngineStateMachine(status: EngineRunStatus): StateMachineStep[] {
  const isFailed = status === 'FAILED' || status === 'CANCELLED' || status === 'TERMINATED';
  const isClosed = status === 'COMPLETED' || isFailed || status === 'CONTINUED_AS_NEW';
  const isWaiting = status === 'WAITING' || status === 'SUSPENDED';
  return [
    { id: 'created', label: 'Created', done: true, current: false },
    {
      id: 'running',
      label: 'Running',
      done: status !== 'QUEUED',
      current: status === 'RUNNING',
    },
    {
      id: 'waiting',
      label: 'Waiting',
      done: isClosed || isWaiting,
      current: isWaiting,
      warn: isWaiting,
    },
    {
      id: 'closed',
      label: isFailed ? 'Failed' : isClosed ? 'Closed' : 'Closing',
      done: isClosed,
      current: isClosed,
      error: isFailed,
    },
  ];
}

export function isTerminalEngineStatus(status: EngineRunStatus): boolean {
  return (
    status === 'COMPLETED' ||
    status === 'FAILED' ||
    status === 'CANCELLED' ||
    status === 'TERMINATED' ||
    status === 'CONTINUED_AS_NEW'
  );
}
