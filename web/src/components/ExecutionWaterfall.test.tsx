import { render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getAccessibleSummary, type RetrySafetyAssessment } from '../utils/retrySafety';
import { buildSpanTree, deriveVisibleRows } from '../utils/spanTree';
import {
  createSpan,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import { ExecutionWaterfall } from './ExecutionWaterfall';

beforeEach(() => {
  resetTestEntityCounter();
});

describe('ExecutionWaterfall retry safety', () => {
  it('renders failed-span badges in the label container without breaking truncation', () => {
    const rootSpan = createSpan({
      span_id: 'waterfall-root',
      name: 'Waterfall root',
      status: 'COMPLETED',
      started_at: '2026-03-14T10:00:00.000Z',
      ended_at: '2026-03-14T10:00:04.000Z',
      latency_ms: 4000,
    });
    const failedSpan = createSpan({
      span_id: 'waterfall-failed',
      name: 'Failed waterfall span',
      parent_span_id: rootSpan.span_id,
      status: 'FAILED',
      started_at: '2026-03-14T10:00:01.000Z',
      ended_at: '2026-03-14T10:00:02.000Z',
      latency_ms: 1000,
    });
    const completedSpan = createSpan({
      span_id: 'waterfall-completed',
      name: 'Completed waterfall span',
      parent_span_id: rootSpan.span_id,
      status: 'COMPLETED',
      started_at: '2026-03-14T10:00:02.000Z',
      ended_at: '2026-03-14T10:00:03.000Z',
      latency_ms: 1000,
    });

    const rows = deriveVisibleRows(
      buildSpanTree([rootSpan, failedSpan, completedSpan]),
      new Set([rootSpan.span_id])
    );

    render(
      <ExecutionWaterfall
        events={[]}
        rows={rows}
        selectedSpanId={null}
        onSelectSpanAndShowDetails={vi.fn()}
        revealTarget={null}
        revealVersion={0}
        spans={[rootSpan, failedSpan, completedSpan]}
        traceStartedAt={rootSpan.started_at}
        traceEndedAt={rootSpan.ended_at}
        spanAssessments={
          new Map<string, RetrySafetyAssessment>([
            [
              failedSpan.span_id,
              {
                classification: 'unsafe',
                reason: 'mutating_non_idempotent',
                decisiveSpanId: failedSpan.span_id,
                decisiveSpanName: failedSpan.name,
                decisiveEventId: 'effect-1',
              },
            ],
          ])
        }
      />
    );

    const section = screen
      .getByRole('heading', { name: 'Execution Waterfall' })
      .closest('section');
    expect(section).not.toBeNull();

    const badge = within(section!).getByLabelText(getAccessibleSummary('unsafe'));
    expect(badge).toHaveClass('whitespace-nowrap');

    const failedName = Array.from(section!.querySelectorAll('div')).find(
      (element) =>
        element.textContent === 'Failed waterfall span' &&
        element.className.includes('truncate')
    );
    expect(failedName).not.toBeUndefined();
    expect(failedName).toHaveClass('truncate');

    const failedBar = screen.getByRole('button', {
      name: 'Select waterfall span Failed waterfall span',
    });
    expect(
      within(failedBar).queryByLabelText(getAccessibleSummary('unsafe'))
    ).not.toBeInTheDocument();

    const completedName = Array.from(section!.querySelectorAll('div')).find(
      (element) =>
        element.textContent === 'Completed waterfall span' &&
        element.className.includes('truncate')
    );
    expect(completedName).not.toBeUndefined();
    expect(completedName).toHaveClass('truncate');
    expect(
      within(section!).getAllByLabelText(getAccessibleSummary('unsafe'))
    ).toHaveLength(1);
  });
});
