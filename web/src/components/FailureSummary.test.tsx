import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { FailureSummary as FailureSummaryData } from '../utils/failureAnalysis';
import {
  getAccessibleSummary,
  getReasonExplanation,
  type RetrySafetyAssessment,
} from '../utils/retrySafety';
import { createSpan, resetTestEntityCounter } from '../test/traceFixtures';
import { FailureSummary } from './FailureSummary';

beforeEach(() => {
  resetTestEntityCounter();
});

describe('FailureSummary retry safety', () => {
  it.each([
    ['retryable', 'read_only_effect'],
    ['unsafe', 'mutating_non_idempotent'],
    ['unknown', 'mutating_missing_idempotent'],
  ] as const)(
    'renders the %s badge and explanation when a trace assessment is present',
    (classification, reason) => {
      render(
        <FailureSummary
          summary={createSummary()}
          onJumpToPrimaryFailedSpan={vi.fn()}
          traceRetrySafety={createAssessment({ classification, reason })}
        />
      );

      expect(screen.getByText('Trace retry safety')).toBeInTheDocument();
      expect(
        screen.getByLabelText(getAccessibleSummary(classification))
      ).toBeInTheDocument();
      expect(screen.getByText(getReasonExplanation(reason))).toBeInTheDocument();
    }
  );

  it('renders decisive-span navigation when the decisive span differs from the primary span', async () => {
    const user = userEvent.setup();
    const onJump = vi.fn();

    render(
      <FailureSummary
        summary={createSummary()}
        onJumpToPrimaryFailedSpan={onJump}
        traceRetrySafety={createAssessment({
          classification: 'unsafe',
          reason: 'mutating_non_idempotent',
          decisiveSpanId: 'secondary-failed',
          decisiveSpanName: 'Secondary failed',
        })}
      />
    );

    expect(
      screen.getByText(/Determined by failed span/i)
    ).toHaveTextContent('Secondary failed');

    await user.click(
      screen.getByRole('button', {
        name: 'Jump to decisive span Secondary failed',
      })
    );

    expect(onJump).toHaveBeenCalledWith('secondary-failed');
  });

  it('renders no retry-safety panel when no assessment is provided', () => {
    render(
      <FailureSummary
        summary={createSummary()}
        onJumpToPrimaryFailedSpan={vi.fn()}
      />
    );

    expect(screen.queryByText('Trace retry safety')).not.toBeInTheDocument();
  });
});

function createSummary(
  overrides: Partial<FailureSummaryData> = {}
): FailureSummaryData {
  const primaryFailedSpan = createSpan({
    span_id: 'primary-failed',
    name: 'Primary failed',
    status: 'FAILED',
  });

  return {
    primaryFailedSpan,
    failedSpanCount: 1,
    errorEventCount: 2,
    errorPreview: 'Primary failed inline preview',
    failureTimestamp: '2026-03-14T10:00:01.000Z',
    breadcrumbPath: [{ spanId: primaryFailedSpan.span_id, name: primaryFailedSpan.name }],
    ...overrides,
  };
}

function createAssessment(
  overrides: Partial<RetrySafetyAssessment> = {}
): RetrySafetyAssessment {
  return {
    classification: overrides.classification ?? 'retryable',
    reason: overrides.reason ?? 'read_only_effect',
    decisiveSpanId: overrides.decisiveSpanId ?? 'primary-failed',
    decisiveSpanName: overrides.decisiveSpanName ?? 'Primary failed',
    decisiveEventId: overrides.decisiveEventId ?? 'effect-1',
    effectKind: overrides.effectKind,
    hasExternalSideEffect: overrides.hasExternalSideEffect,
    idempotent: overrides.idempotent,
    idempotencyKey: overrides.idempotencyKey,
  };
}
