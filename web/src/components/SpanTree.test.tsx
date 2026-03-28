import { render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getAccessibleSummary, type RetrySafetyAssessment } from '../utils/retrySafety';
import { buildSpanTree, deriveVisibleRows } from '../utils/spanTree';
import {
  createSpan,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import { SpanTree } from './SpanTree';

beforeEach(() => {
  resetTestEntityCounter();
});

describe('SpanTree retry safety', () => {
  it('shows compact retry-safety badges on failed spans while preserving existing chips', () => {
    const rootSpan = createSpan({
      span_id: 'root-span',
      name: 'Root span',
      status: 'COMPLETED',
    });
    const failedSpan = createSpan({
      span_id: 'failed-span',
      name: 'Failed span',
      parent_span_id: rootSpan.span_id,
      status: 'FAILED',
    });
    const completedSpan = createSpan({
      span_id: 'completed-span',
      name: 'Completed span',
      parent_span_id: rootSpan.span_id,
      status: 'COMPLETED',
    });

    const rows = deriveVisibleRows(buildSpanTree([rootSpan, failedSpan, completedSpan]), new Set([rootSpan.span_id]));

    render(
      <SpanTree
        rows={rows}
        expandedSpanIds={new Set([rootSpan.span_id])}
        selectedSpanId={failedSpan.span_id}
        onSelectSpan={vi.fn()}
        onToggleExpand={vi.fn()}
        failedSpanIds={new Set([failedSpan.span_id])}
        primaryAncestorPath={new Set([rootSpan.span_id, failedSpan.span_id])}
        revealPath={new Set()}
        revealKey={0}
        inlineErrorPreviews={new Map()}
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

    const failedRow = screen.getByRole('button', { name: 'Select span Failed span' });
    expect(within(failedRow).getByText('Selected')).toBeInTheDocument();
    expect(within(failedRow).getByText('Failure path')).toBeInTheDocument();
    expect(within(failedRow).getByText('Failed')).toBeInTheDocument();
    expect(
      within(failedRow).getByLabelText(getAccessibleSummary('unsafe'))
    ).toBeInTheDocument();

    const completedRow = screen.getByRole('button', { name: 'Select span Completed span' });
    expect(
      within(completedRow).queryByLabelText(getAccessibleSummary('unsafe'))
    ).not.toBeInTheDocument();
  });
});
