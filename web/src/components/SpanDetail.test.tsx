import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { Span } from '../api/client';
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
});
