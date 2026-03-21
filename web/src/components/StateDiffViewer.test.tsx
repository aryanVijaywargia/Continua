import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { createTimelineEvent } from '../test/traceFixtures';
import { extractStateChanges } from '../utils/stateChanges';
import { StateDiffViewer } from './StateDiffViewer';

describe('StateDiffViewer', () => {
  it('groups changes by namespace and renders scalar/object values appropriately', () => {
    const changes = extractStateChanges([
      createTimelineEvent({
        id: 'scalar-change',
        event_type: 'state_change',
        span_name: 'Checkout span',
        payload: {
          key: 'status',
          namespace: 'order',
          old_value: 'pending',
          new_value: 'approved',
        },
      }),
      createTimelineEvent({
        id: 'object-change',
        event_type: 'state_change',
        span_name: 'Config span',
        payload: {
          key: 'flags',
          namespace: 'config',
          old_value: { tier: 'standard' },
          new_value: { tier: 'priority' },
        },
      }),
    ]);

    render(<StateDiffViewer changes={changes} />);

    expect(screen.getByText('order')).toBeInTheDocument();
    expect(screen.getByText('config')).toBeInTheDocument();
    expect(screen.getByText('status')).toBeInTheDocument();
    expect(screen.getByText('pending')).toBeInTheDocument();
    expect(screen.getByText('approved')).toBeInTheDocument();
    expect(screen.getAllByText('tier').length).toBeGreaterThan(0);
    expect(screen.getByText('Old value')).toBeInTheDocument();
    expect(screen.getByText('New value')).toBeInTheDocument();
  });

  it('renders an empty state when no changes are present', () => {
    render(<StateDiffViewer changes={[]} />);

    expect(
      screen.getByText('No structured state changes recorded for this trace.')
    ).toBeInTheDocument();
  });

  it('uses the fallback namespace label for unscoped changes', () => {
    const changes = extractStateChanges([
      createTimelineEvent({
        id: 'general-change',
        event_type: 'state_change',
        payload: {
          key: 'phase',
          new_value: 'running',
        },
      }),
    ]);

    render(<StateDiffViewer changes={changes} />);

    expect(screen.getByText('General')).toBeInTheDocument();
    expect(screen.getByText('phase')).toBeInTheDocument();
    expect(screen.getByText('running')).toBeInTheDocument();
  });
});
