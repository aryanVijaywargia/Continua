import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { EnginePendingWorkPanel } from './EnginePendingWorkPanel';

describe('EnginePendingWorkPanel', () => {
  it('renders current wait plus activity, timer, and signal details', () => {
    render(
      <EnginePendingWorkPanel
        data={{
          run_id: 'run-1',
          current_wait: {
            kind: 'signal',
            signal_name: 'approval',
          },
          activities: [
            {
              task_id: 'task-1',
              activity_key: 'charge-card',
              activity_type: 'payments.charge',
              status: 'scheduled',
              available_at: '2026-03-14T10:00:00.000Z',
              attempt_count: 2,
            },
            {
              task_id: 'task-2',
              activity_key: 'send-email',
              activity_type: 'notifications.email',
              status: 'queued',
              available_at: '2026-03-14T10:01:00.000Z',
              attempt_count: 1,
            },
          ],
          timers: [
            {
              inbox_id: 'timer-1',
              timer_key: 'approval-timeout',
              status: 'scheduled',
              available_at: '2026-03-14T10:05:00.000Z',
            },
          ],
          signals: [
            {
              inbox_id: 'signal-1',
              signal_name: 'manual_override',
              status: 'queued',
              available_at: '2026-03-14T10:02:00.000Z',
            },
          ],
          pending_activity_tasks: 2,
          pending_inbox_items: 2,
        }}
        isError={false}
        isLoading={false}
      />
    );

    expect(screen.getByText('Waiting on signal')).toBeInTheDocument();
    expect(screen.getByText('approval')).toBeInTheDocument();
    expect(screen.getByText('payments.charge · charge-card')).toBeInTheDocument();
    expect(screen.getByText('approval-timeout')).toBeInTheDocument();
    expect(screen.getByText('manual_override')).toBeInTheDocument();
    expect(screen.getByText('Activities: 2')).toBeInTheDocument();
    expect(screen.getByText('Inbox: 2')).toBeInTheDocument();
  });

  it('renders empty states for each pending-work section', () => {
    render(
      <EnginePendingWorkPanel
        data={{
          run_id: 'run-1',
          current_wait: null,
          activities: [],
          timers: [],
          signals: [],
          pending_activity_tasks: 0,
          pending_inbox_items: 0,
        }}
        isError={false}
        isLoading={false}
      />
    );

    expect(screen.getByText('No active wait reported.')).toBeInTheDocument();
    expect(screen.getByText('No pending activities.')).toBeInTheDocument();
    expect(screen.getByText('No pending timers.')).toBeInTheDocument();
    expect(screen.getByText('No pending signals.')).toBeInTheDocument();
  });

  it('renders a degraded state when the pending-work query fails', () => {
    render(
      <EnginePendingWorkPanel
        data={undefined}
        isError={true}
        isLoading={false}
        errorMessage="backend exploded"
      />
    );

    expect(
      screen.getByText('Pending work is temporarily unavailable. backend exploded')
    ).toBeInTheDocument();
    expect(screen.queryByText('No pending activities.')).not.toBeInTheDocument();
  });
});
