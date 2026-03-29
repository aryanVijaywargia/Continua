import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { DecisionTraceEntry } from '../utils/reasoning';
import { createTimelineEvent } from '../test/traceFixtures';
import { ReasoningTab } from './ReasoningTab';

const SAMPLE_ENTRIES: DecisionTraceEntry[] = [
  {
    event: createTimelineEvent({
      id: 'reasoning-entry-1',
      span_id: 'span-1',
      span_name: 'Planner span',
      event_type: 'decision',
      timestamp: '2026-03-14T10:00:01.000Z',
      payload: {
        question: 'Which model?',
        chosen: 'gpt-5.4',
        reasoning: 'Need the higher-accuracy model.',
        alternatives: ['gpt-5.4-mini'],
      },
    }),
    spanId: 'span-1',
    spanName: 'Planner span',
    question: 'Which model?',
    chosen: 'gpt-5.4',
    reasoning: 'Need the higher-accuracy model.',
    alternatives: ['gpt-5.4-mini'],
  },
];

describe('ReasoningTab', () => {
  it('renders decision entries', () => {
    render(<ReasoningTab entries={SAMPLE_ENTRIES} onSelectSpan={vi.fn()} />);

    expect(screen.getByRole('heading', { name: 'Reasoning' })).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /Planner span.*Which model\?/i })
    ).toBeInTheDocument();
    expect(screen.getByText('Need the higher-accuracy model.')).toBeInTheDocument();
    expect(screen.getByText('gpt-5.4-mini')).toBeInTheDocument();
  });

  it('renders an empty state when there are no reasoning entries', () => {
    render(<ReasoningTab entries={[]} onSelectSpan={vi.fn()} />);

    expect(
      screen.getByText('No reasoning decisions recorded for this trace.')
    ).toBeInTheDocument();
  });

  it('fires onSelectSpan on click and keyboard activation', async () => {
    const user = userEvent.setup();
    const onSelectSpan = vi.fn();

    render(<ReasoningTab entries={SAMPLE_ENTRIES} onSelectSpan={onSelectSpan} />);

    const row = screen.getByRole('button', {
      name: /Planner span.*Which model\?/i,
    });

    await user.click(row);
    row.focus();
    await user.keyboard('{Enter}');
    await user.keyboard('{Space}');

    expect(onSelectSpan).toHaveBeenCalledTimes(3);
    expect(onSelectSpan).toHaveBeenNthCalledWith(1, 'span-1');
    expect(onSelectSpan).toHaveBeenNthCalledWith(2, 'span-1');
    expect(onSelectSpan).toHaveBeenNthCalledWith(3, 'span-1');
  });
});
