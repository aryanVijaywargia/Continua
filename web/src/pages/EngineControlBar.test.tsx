import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import type { EngineRunStatus, EngineRunSummary } from '../api/client';
import { STATUS_TONE } from '../components/statusTone';
import { EngineControlBar } from './EngineControlBar';

function renderEngineControlBar(status: EngineRunStatus) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
  const engine: EngineRunSummary = {
    run_id: '123e4567-e89b-12d3-a456-426614174100',
    instance_key: 'instance-quarantined',
    definition_name: 'demo',
    definition_version: 'v1',
    projection_state: 'up_to_date',
    status,
    created_at: '2026-03-14T10:00:00.000Z',
    updated_at: '2026-03-14T10:00:03.000Z',
    pending_work: {
      pending_activity_tasks: 0,
      pending_inbox_items: 0,
    },
    failure: {
      error_code: 'replay_mismatch',
      error_message: 'activity scheduling did not match recorded history',
      status: 'quarantined',
    },
    wait_state: {
      kind: 'replay_mismatch',
      detail: 'activity scheduling did not match recorded history',
    } as EngineRunSummary['wait_state'],
  };

  return render(
    <QueryClientProvider client={queryClient}>
      <EngineControlBar engine={engine} traceId="trace-quarantined" />
    </QueryClientProvider>
  );
}

describe('EngineControlBar', () => {
  it('enables resume and exposes a status tone for quarantined runs', () => {
    renderEngineControlBar('QUARANTINED' as EngineRunStatus);

    expect(screen.getByRole('button', { name: 'Resume' })).toBeEnabled();
    expect(
      (STATUS_TONE as Record<string, { label: string }>).QUARANTINED
    ).toMatchObject({ label: 'Quarantined' });
  });
});
