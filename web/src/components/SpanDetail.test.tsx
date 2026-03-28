import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { Span, TimelineEvent } from '../api/client';
import {
  getAccessibleSummary,
  getReasonExplanation,
  type RetrySafetyAssessment,
} from '../utils/retrySafety';
import { SpanDetail } from './SpanDetail';

function createSpan(overrides: Partial<Span> = {}): Span {
  const spanId = overrides.span_id ?? 'span-1';

  return {
    id: overrides.id ?? `uuid-${spanId}`,
    trace_id: overrides.trace_id ?? 'trace-1',
    span_id: spanId,
    parent_span_id: overrides.parent_span_id,
    name: overrides.name ?? spanId,
    kind: overrides.kind ?? 'CHAIN',
    status: overrides.status ?? 'COMPLETED',
    started_at: overrides.started_at ?? '2026-03-14T10:00:00.000Z',
    ended_at: overrides.ended_at,
    tokens_in: overrides.tokens_in,
    tokens_out: overrides.tokens_out,
    cost_usd: overrides.cost_usd,
    latency_ms: overrides.latency_ms,
    error_message: overrides.error_message,
    model: overrides.model,
    provider: overrides.provider,
    input: overrides.input,
    input_truncated: overrides.input_truncated,
    input_original_size_bytes: overrides.input_original_size_bytes,
    input_truncation_reason: overrides.input_truncation_reason,
    output: overrides.output,
    output_truncated: overrides.output_truncated,
    output_original_size_bytes: overrides.output_original_size_bytes,
    output_truncation_reason: overrides.output_truncation_reason,
    metadata: overrides.metadata,
  };
}

function createRetrySafetyAssessment(
  overrides: Partial<RetrySafetyAssessment> = {}
): RetrySafetyAssessment {
  return {
    classification: overrides.classification ?? 'retryable',
    reason: overrides.reason ?? 'read_only_effect',
    decisiveSpanId: overrides.decisiveSpanId ?? 'span-1',
    decisiveSpanName: overrides.decisiveSpanName ?? 'span-1',
    decisiveEventId: overrides.decisiveEventId ?? 'effect-1',
    effectKind: overrides.effectKind,
    hasExternalSideEffect: overrides.hasExternalSideEffect,
    idempotent: overrides.idempotent,
    idempotencyKey: overrides.idempotencyKey,
  };
}

describe('SpanDetail parent navigation', () => {
  it('omits the parent row for root spans', () => {
    render(
      <SpanDetail
        span={createSpan({ span_id: 'root', name: 'Root span' })}
        breadcrumbPath={[{ spanId: 'root', name: 'Root span' }]}
        onSelectSpan={vi.fn()}
        spanIndex={new Map()}
      />
    );

    expect(screen.queryByText('Parent Span ID:')).not.toBeInTheDocument();
  });

  it('renders unresolved parent IDs as plain text', () => {
    render(
      <SpanDetail
        span={createSpan({
          span_id: 'child',
          name: 'Child span',
          parent_span_id: 'missing-parent',
        })}
        breadcrumbPath={[{ spanId: 'child', name: 'Child span' }]}
        onSelectSpan={vi.fn()}
        spanIndex={new Map()}
      />
    );

    expect(screen.getByText('missing-parent')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'missing-parent' })).not.toBeInTheDocument();
  });

  it('renders resolvable parent IDs as buttons wired to selection', () => {
    const onSelectSpan = vi.fn();
    const parentSpan = createSpan({ span_id: 'root', name: 'Root span' });
    const childSpan = createSpan({
      span_id: 'child',
      name: 'Child span',
      parent_span_id: 'root',
    });

    render(
      <SpanDetail
        span={childSpan}
        breadcrumbPath={[
          { spanId: 'root', name: 'Root span' },
          { spanId: 'child', name: 'Child span' },
        ]}
        onSelectSpan={onSelectSpan}
        spanIndex={new Map([['root', parentSpan], ['child', childSpan]])}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'Select parent span root' }));

    expect(onSelectSpan).toHaveBeenCalledWith('root');
  });

  it('supports keyboard activation for the parent span button', async () => {
    const user = userEvent.setup();
    const onSelectSpan = vi.fn();
    const parentSpan = createSpan({ span_id: 'root', name: 'Root span' });
    const childSpan = createSpan({
      span_id: 'child',
      name: 'Child span',
      parent_span_id: 'root',
    });

    render(
      <SpanDetail
        span={childSpan}
        breadcrumbPath={[
          { spanId: 'root', name: 'Root span' },
          { spanId: 'child', name: 'Child span' },
        ]}
        onSelectSpan={onSelectSpan}
        spanIndex={new Map([['root', parentSpan], ['child', childSpan]])}
      />
    );

    const parentButton = screen.getByRole('button', {
      name: 'Select parent span root',
    });
    parentButton.focus();
    await user.keyboard('{Enter}');

    expect(onSelectSpan).toHaveBeenCalledWith('root');
  });

  it('renders valid decision events and skips incomplete ones', () => {
    const span = createSpan({ span_id: 'decision-span', name: 'Decision span' });
    const events: TimelineEvent[] = [
      {
        id: 'decision-valid',
        trace_id: 'trace-1',
        span_id: 'decision-span',
        event_type: 'decision',
        timestamp: '2026-03-14T10:00:00.000Z',
        source: 'explicit',
        payload: {
          question: 'Which model?',
          chosen: 'gpt-4.1',
          reasoning: 'Need higher accuracy',
        },
      },
      {
        id: 'decision-invalid',
        trace_id: 'trace-1',
        span_id: 'decision-span',
        event_type: 'decision',
        timestamp: '2026-03-14T10:00:01.000Z',
        source: 'explicit',
        payload: {
          question: 'Should be hidden',
        },
      },
    ];

    render(
      <SpanDetail
        span={span}
        breadcrumbPath={[{ spanId: 'decision-span', name: 'Decision span' }]}
        onSelectSpan={vi.fn()}
        spanIndex={new Map([['decision-span', span]])}
        events={events}
      />
    );

    expect(screen.getByText('Decisions')).toBeInTheDocument();
    expect(screen.getByText('Which model?')).toBeInTheDocument();
    expect(screen.getByText('gpt-4.1')).toBeInTheDocument();
    expect(screen.queryByText('Should be hidden')).not.toBeInTheDocument();
  });
});

describe('SpanDetail retry safety', () => {
  it.each([
    ['retryable', 'read_only_effect'],
    ['unsafe', 'mutating_non_idempotent'],
    ['unknown', 'no_effect_events'],
  ] as const)(
    'renders the %s retry safety section for failed spans',
    (classification, reason) => {
      render(
        <SpanDetail
          span={createSpan({ span_id: 'failed-span', status: 'FAILED' })}
          breadcrumbPath={[{ spanId: 'failed-span', name: 'failed-span' }]}
          onSelectSpan={vi.fn()}
          spanIndex={new Map()}
          retrySafety={createRetrySafetyAssessment({
            classification,
            reason,
          })}
        />
      );

      expect(screen.getByText('Retry Safety')).toBeInTheDocument();
      expect(
        screen.getByLabelText(getAccessibleSummary(classification))
      ).toBeInTheDocument();
      expect(screen.getByText(getReasonExplanation(reason))).toBeInTheDocument();
    }
  );

  it('shows supporting semantic fields when effect metadata is available', () => {
    render(
      <SpanDetail
        span={createSpan({ span_id: 'failed-span', status: 'FAILED' })}
        breadcrumbPath={[{ spanId: 'failed-span', name: 'failed-span' }]}
        onSelectSpan={vi.fn()}
        spanIndex={new Map()}
        retrySafety={createRetrySafetyAssessment({
          effectKind: 'api_call',
          hasExternalSideEffect: true,
          idempotent: true,
          idempotencyKey: 'key-1',
          reason: 'mutating_idempotent_with_key',
        })}
      />
    );

    expect(screen.getByText('effect_kind:')).toBeInTheDocument();
    expect(screen.getByText('api_call')).toBeInTheDocument();
    expect(screen.getByText('has_external_side_effect:')).toBeInTheDocument();
    expect(screen.getAllByText('true')).toHaveLength(2);
    expect(screen.getByText('idempotent:')).toBeInTheDocument();
    expect(screen.getByText('idempotency_key:')).toBeInTheDocument();
    expect(screen.getByText('key-1')).toBeInTheDocument();
  });

  it('omits the retry safety section for non-failed spans', () => {
    render(
      <SpanDetail
        span={createSpan({ span_id: 'completed-span', status: 'COMPLETED' })}
        breadcrumbPath={[{ spanId: 'completed-span', name: 'completed-span' }]}
        onSelectSpan={vi.fn()}
        spanIndex={new Map()}
        retrySafety={createRetrySafetyAssessment()}
      />
    );

    expect(screen.queryByText('Retry Safety')).not.toBeInTheDocument();
  });
});
