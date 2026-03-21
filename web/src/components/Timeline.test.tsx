import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { createTimelineEvent } from '../test/traceFixtures';
import { Timeline } from './Timeline';

describe('Timeline semantic event rendering', () => {
  it('renders state_change rows with inline old/new values', () => {
    render(
      <Timeline
        events={[
          createTimelineEvent({
            event_type: 'state_change',
            payload: {
              key: 'status',
              old_value: 'pending',
              new_value: 'approved',
            },
          }),
        ]}
        traceStatus="COMPLETED"
        isLive={false}
        onSelectSpan={vi.fn()}
        spanIndex={new Map()}
      />
    );

    expect(screen.getByText('status')).toBeInTheDocument();
    expect(screen.getByText('pending')).toBeInTheDocument();
    expect(screen.getByText('approved')).toBeInTheDocument();
  });

  it('falls back to the message when semantic fields are missing', () => {
    render(
      <Timeline
        events={[
          createTimelineEvent({
            event_type: 'decision',
            message: 'Generic fallback',
            payload: {
              chosen: 'gpt-4.1',
            },
          }),
        ]}
        traceStatus="COMPLETED"
        isLive={false}
        onSelectSpan={vi.fn()}
        spanIndex={new Map()}
      />
    );

    expect(screen.getByText('Generic fallback')).toBeInTheDocument();
  });

  it('renders decision alternatives inline when they are present', () => {
    render(
      <Timeline
        events={[
          createTimelineEvent({
            event_type: 'decision',
            payload: {
              question: 'Which model?',
              chosen: 'gpt-4.1',
              alternatives: ['gpt-4o-mini', 'claude-3-haiku'],
            },
          }),
        ]}
        traceStatus="COMPLETED"
        isLive={false}
        onSelectSpan={vi.fn()}
        spanIndex={new Map()}
      />
    );

    expect(screen.getByText('Which model?')).toBeInTheDocument();
    expect(screen.getByText('Alternatives')).toBeInTheDocument();
    expect(screen.getByText('gpt-4o-mini')).toBeInTheDocument();
    expect(screen.getByText('claude-3-haiku')).toBeInTheDocument();
  });
});
