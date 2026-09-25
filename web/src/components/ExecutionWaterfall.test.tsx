import { fireEvent, render, screen, within } from '@testing-library/react';
import type { Span } from '../api/client';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getAccessibleSummary, type RetrySafetyAssessment } from '../utils/retrySafety';
import { buildSpanTree, deriveVisibleRows } from '../utils/spanTree';
import {
  createSpan,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import {
  DERIVED_DURATION_NOTE,
  ExecutionWaterfall,
  WATERFALL_ROW_HEIGHT,
} from './ExecutionWaterfall';
import { getStepDuration, type StepDuration } from './trace/traceSteps';
import type { StepDurations } from './trace/useStepDurations';

function buildDurations(spans: Span[]): StepDurations {
  const byId = new Map<string, StepDuration>();
  for (const span of spans) {
    byId.set(span.span_id, getStepDuration(span, Date.parse('2026-03-14T10:00:10.000Z')));
  }
  const values = [...byId.values()];
  return {
    byId,
    anyDerived: values.some((value) => value.derived),
    allDerived: values.length > 0 && values.every((value) => value.derived),
  };
}

function renderWaterfall(spans: Span[], onSelectSpan = vi.fn()) {
  const tree = buildSpanTree(spans);
  const rows = deriveVisibleRows(tree, new Set(spans.map((span) => span.span_id)));
  render(
    <ExecutionWaterfall
      events={[]}
      rows={rows}
      spans={spans}
      durations={buildDurations(spans)}
      selectedSpanId={null}
      onSelectSpan={onSelectSpan}
      revealTarget={null}
      traceStartedAt={spans[0].started_at}
      traceEndedAt={spans[0].ended_at}
    />
  );
  return onSelectSpan;
}

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
        durations={buildDurations([rootSpan, failedSpan, completedSpan])}
        onSelectSpan={vi.fn()}
        revealTarget={null}
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
      .getByRole('region', { name: 'Execution steps' });

    const badge = within(section).getByLabelText(getAccessibleSummary('unsafe'));
    expect(badge).toHaveClass('whitespace-nowrap');

    const failedName = Array.from(section.querySelectorAll('span')).find(
      (element) =>
        element.textContent === 'Failed waterfall span' &&
        element.className.includes('truncate')
    );
    expect(failedName).not.toBeUndefined();
    expect(failedName).toHaveClass('truncate');

    const failedRowButton = screen.getByRole('button', {
      name: 'Select step Failed waterfall span',
    });
    expect(
      within(failedRowButton).queryByLabelText(getAccessibleSummary('unsafe'))
    ).not.toBeInTheDocument();

    const completedName = Array.from(section.querySelectorAll('span')).find(
      (element) =>
        element.textContent === 'Completed waterfall span' &&
        element.className.includes('truncate')
    );
    expect(completedName).not.toBeUndefined();
    expect(completedName).toHaveClass('truncate');
    expect(
      within(section).getAllByLabelText(getAccessibleSummary('unsafe'))
    ).toHaveLength(1);
  });

  it('uses a uniform row height and selects a step from its row', () => {
    expect(WATERFALL_ROW_HEIGHT).toBe(33);
    const root = createSpan({
      span_id: 'root',
      name: 'Root step',
      started_at: '2026-03-14T10:00:00.000Z',
      ended_at: '2026-03-14T10:00:02.000Z',
      latency_ms: 2000,
    });
    const onSelectSpan = renderWaterfall([root]);

    expect(screen.getByText('All 1 steps')).toBeInTheDocument();
    expect(screen.getByText('0 – 2.00s')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Select step Root step' }));
    expect(onSelectSpan).toHaveBeenCalledWith('root');
    expect(screen.queryByText(DERIVED_DURATION_NOTE)).not.toBeInTheDocument();
  });

  it('marks derived durations and shows the honesty note', () => {
    const root = {
      ...createSpan({
        span_id: 'derived-root',
        name: 'Derived root',
        started_at: '2026-03-14T10:00:00.000Z',
        ended_at: '2026-03-14T10:00:01.500Z',
      }),
      latency_ms: undefined,
    };
    renderWaterfall([root]);

    expect(screen.getByText(DERIVED_DURATION_NOTE)).toBeInTheDocument();
    expect(screen.getAllByText(/derived/i).length).toBeGreaterThan(0);
  });

  it('hides fast steps when focus slow steps is on', () => {
    const root = createSpan({
      span_id: 'focus-root',
      name: 'Focus root',
      started_at: '2026-03-14T10:00:00.000Z',
      ended_at: '2026-03-14T10:00:02.000Z',
      latency_ms: 2000,
    });
    const fast = createSpan({
      span_id: 'focus-fast',
      name: 'Fast child',
      parent_span_id: root.span_id,
      started_at: '2026-03-14T10:00:00.000Z',
      ended_at: '2026-03-14T10:00:00.002Z',
      latency_ms: 2,
    });
    renderWaterfall([root, fast]);

    const toggle = screen.getByRole('button', { name: /Focus slow steps/ });
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('aria-pressed', 'true');
    expect(screen.queryByRole('button', { name: 'Select step Fast child' })).not.toBeInTheDocument();
    expect(screen.getByText('1 of 2 steps')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Show all 2' }));
    expect(screen.getByRole('button', { name: 'Select step Fast child' })).toBeInTheDocument();
  });

  it('shows an empty state without steps', () => {
    render(
      <ExecutionWaterfall
        events={[]}
        rows={[]}
        spans={[]}
        durations={buildDurations([])}
        selectedSpanId={null}
        onSelectSpan={vi.fn()}
        revealTarget={null}
      />
    );
    expect(screen.getByText('No steps were recorded for this trace.')).toBeInTheDocument();
  });
});
