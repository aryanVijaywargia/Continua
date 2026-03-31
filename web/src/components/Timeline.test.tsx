import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { getAccessibleSummary } from '../utils/retrySafety';
import { createSpan, createTimelineEvent } from '../test/traceFixtures';
import { Timeline } from './Timeline';

type TimelineProps = Parameters<typeof Timeline>[0];

function renderTimeline(overrides: Partial<TimelineProps> = {}) {
  const props: TimelineProps = {
    events: [],
    traceStatus: 'COMPLETED',
    isLive: false,
    isLoading: false,
    error: null,
    onSelectSpan: vi.fn(),
    spanIndex: new Map(),
    ...overrides,
  };

  return render(<Timeline {...props} />);
}

describe('Timeline semantic event rendering', () => {
  it('renders state_change rows with inline old/new values', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'state_change',
          payload: {
            key: 'status',
            old_value: 'pending',
            new_value: 'approved',
          },
        }),
      ],
    });

    expect(screen.getByText('status')).toBeInTheDocument();
    expect(screen.getByText('pending')).toBeInTheDocument();
    expect(screen.getByText('approved')).toBeInTheDocument();
  });

  it('falls back to the message when decision semantic fields are missing', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'decision',
          message: 'Generic fallback',
          payload: {
            chosen: 'gpt-4.1',
          },
        }),
      ],
    });

    expect(screen.getByText('Generic fallback')).toBeInTheDocument();
  });

  it('renders decision alternatives inline when they are present', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'decision',
          payload: {
            question: 'Which model?',
            chosen: 'gpt-4.1',
            alternatives: ['gpt-4o-mini', 'claude-3-haiku'],
          },
        }),
      ],
    });

    expect(screen.getByText('Which model?')).toBeInTheDocument();
    expect(screen.getByText('Alternatives')).toBeInTheDocument();
    expect(screen.getByText('gpt-4o-mini')).toBeInTheDocument();
    expect(screen.getByText('claude-3-haiku')).toBeInTheDocument();
  });

  it('renders effect previews with the kind and mutating badge', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'effect',
          payload: {
            effect_kind: 'tool_call',
            has_external_side_effect: true,
            effect_id: 'effect-1',
          },
        }),
      ],
    });

    expect(screen.getByText('tool_call')).toBeInTheDocument();
    expect(screen.getByText('mutating')).toBeInTheDocument();
  });

  it('renders effect previews with the read-only badge', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'effect',
          payload: {
            effect_kind: 'model_call',
            has_external_side_effect: false,
          },
        }),
      ],
    });

    expect(screen.getByText('model_call')).toBeInTheDocument();
    expect(screen.getByText('read-only')).toBeInTheDocument();
  });

  it('renders wait previews with the kind, phase, and resolution pill', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'wait',
          payload: {
            wait_kind: 'human_approval',
            phase: 'resolved',
            resolution: 'approved',
          },
        }),
      ],
    });

    expect(screen.getByText('human_approval')).toBeInTheDocument();
    expect(screen.getByText('resolved')).toBeInTheDocument();
    expect(screen.getByText('approved')).toBeInTheDocument();
  });

  it('renders snapshot marker previews with the label and marker kind pill', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'snapshot_marker',
          payload: {
            marker_kind: 'milestone',
            label: 'Data loaded',
          },
        }),
      ],
    });

    expect(screen.getByText('Data loaded')).toBeInTheDocument();
    expect(screen.getByText('milestone')).toBeInTheDocument();
  });

  it('degrades malformed effect and wait payloads to generic rendering', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          id: 'bad-effect',
          event_type: 'effect',
          message: 'Fallback effect summary',
          payload: {
            effect_kind: 'api_call',
          },
        }),
        createTimelineEvent({
          id: 'bad-wait',
          event_type: 'wait',
          message: 'Fallback wait summary',
          payload: {
            wait_kind: 'human_approval',
          },
        }),
      ],
    });

    expect(screen.getByText('Fallback effect summary')).toBeInTheDocument();
    expect(screen.getByText('Fallback wait summary')).toBeInTheDocument();
    expect(screen.queryByText('mutating')).not.toBeInTheDocument();
    expect(screen.queryByText('read-only')).not.toBeInTheDocument();
  });

  it('falls back to generic rendering for malformed snapshot markers', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'snapshot_marker',
          message: 'Malformed marker fallback',
          payload: {
            marker_kind: 'milestone',
          },
        }),
      ],
    });

    expect(screen.getByText('Malformed marker fallback')).toBeInTheDocument();
    expect(screen.queryByText('milestone')).not.toBeInTheDocument();
  });

  it('preserves effect payload inspection and wait span navigation', async () => {
    const user = userEvent.setup();
    const onSelectSpan = vi.fn();
    const effectSpan = createSpan({ span_id: 'effect-span', name: 'Effect span' });
    const waitSpan = createSpan({ span_id: 'wait-span', name: 'Wait span' });

    renderTimeline({
      onSelectSpan,
      spanIndex: new Map([
        [effectSpan.span_id, effectSpan],
        [waitSpan.span_id, waitSpan],
      ]),
      events: [
        createTimelineEvent({
          id: 'effect-event',
          span_id: effectSpan.span_id,
          span_name: effectSpan.name,
          event_type: 'effect',
          message: 'Effect payload',
          payload: {
            effect_kind: 'tool_call',
            has_external_side_effect: true,
            effect_id: 'effect-1',
          },
        }),
        createTimelineEvent({
          id: 'wait-event',
          span_id: waitSpan.span_id,
          span_name: waitSpan.name,
          event_type: 'wait',
          payload: {
            wait_kind: 'human_approval',
            phase: 'entered',
            wait_id: 'wait-1',
          },
        }),
      ],
    });

    await user.click(screen.getAllByRole('button', { name: 'Show details' })[0]);
    expect(screen.getByText('effect_id')).toBeInTheDocument();
    expect(screen.getByText('"effect-1"')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: waitSpan.name }));
    expect(onSelectSpan).toHaveBeenCalledWith(waitSpan.span_id);
  });
});

describe('Timeline segmented filter', () => {
  it('defaults to All and shows all events', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          id: 'message-event',
          event_type: 'message',
          message: 'Narrative event',
        }),
        createTimelineEvent({
          id: 'effect-event',
          event_type: 'effect',
          payload: {
            effect_kind: 'api_call',
            has_external_side_effect: true,
          },
        }),
      ],
    });

    expect(screen.getByRole('radio', { name: 'All' })).toHaveAttribute(
      'aria-checked',
      'true'
    );
    expect(screen.getByText('Narrative event')).toBeInTheDocument();
    expect(screen.getByText('api_call')).toBeInTheDocument();
  });

  it('includes snapshot markers in the Semantic filter mode', async () => {
    const user = userEvent.setup();

    renderTimeline({
      events: [
        createTimelineEvent({
          id: 'message-event',
          event_type: 'message',
          message: 'Narrative event',
        }),
        createTimelineEvent({
          id: 'marker-event',
          event_type: 'snapshot_marker',
          payload: {
            marker_kind: 'milestone',
            label: 'Data loaded',
          },
        }),
      ],
    });

    await user.click(screen.getByRole('radio', { name: 'Semantic' }));

    expect(screen.getByText('Data loaded')).toBeInTheDocument();
    expect(screen.queryByText('Narrative event')).not.toBeInTheDocument();
  });

  it('filters to semantic explicit events when Semantic is selected', async () => {
    const user = userEvent.setup();

    renderTimeline({
      events: [
        createTimelineEvent({
          id: 'message-event',
          event_type: 'message',
          message: 'Narrative event',
        }),
        createTimelineEvent({
          id: 'state-event',
          event_type: 'state_change',
          payload: {
            key: 'status',
            old_value: 'pending',
            new_value: 'approved',
          },
        }),
        createTimelineEvent({
          id: 'effect-event',
          event_type: 'effect',
          payload: {
            effect_kind: 'api_call',
            has_external_side_effect: true,
          },
        }),
        createTimelineEvent({
          id: 'synthetic-event',
          event_type: 'span_completed',
          source: 'synthetic',
          span_name: 'Synthetic root',
        }),
      ],
    });

    await user.click(screen.getByRole('radio', { name: 'Semantic' }));

    expect(screen.queryByText('Narrative event')).not.toBeInTheDocument();
    expect(screen.queryByText('Synthetic root completed')).not.toBeInTheDocument();
    expect(screen.getByText('status')).toBeInTheDocument();
    expect(screen.getByText('api_call')).toBeInTheDocument();
  });

  it('filters to effect and wait events when Effects & waits is selected', async () => {
    const user = userEvent.setup();

    renderTimeline({
      events: [
        createTimelineEvent({
          id: 'decision-event',
          event_type: 'decision',
          payload: {
            question: 'Which model?',
            chosen: 'gpt-4.1',
          },
        }),
        createTimelineEvent({
          id: 'effect-event',
          event_type: 'effect',
          payload: {
            effect_kind: 'tool_call',
            has_external_side_effect: true,
          },
        }),
        createTimelineEvent({
          id: 'wait-event',
          event_type: 'wait',
          payload: {
            wait_kind: 'human_approval',
            phase: 'entered',
          },
        }),
      ],
    });

    await user.click(screen.getByRole('radio', { name: 'Effects & waits' }));

    expect(screen.queryByText('Which model?')).not.toBeInTheDocument();
    expect(screen.getByText('tool_call')).toBeInTheDocument();
    expect(screen.getByText('human_approval')).toBeInTheDocument();
  });

  it('intersects the Semantic filter with Errors only', async () => {
    const user = userEvent.setup();

    renderTimeline({
      events: [
        createTimelineEvent({
          id: 'semantic-error',
          event_type: 'effect',
          level: 'error',
          payload: {
            effect_kind: 'api_call',
            has_external_side_effect: true,
          },
        }),
        createTimelineEvent({
          id: 'semantic-info',
          event_type: 'decision',
          payload: {
            question: 'Which model?',
            chosen: 'gpt-4.1',
          },
        }),
        createTimelineEvent({
          id: 'non-semantic-error',
          event_type: 'log',
          level: 'error',
          message: 'Non-semantic error log',
        }),
        createTimelineEvent({
          id: 'synthetic-error',
          event_type: 'span_failed',
          source: 'synthetic',
          span_name: 'Synthetic failed span',
        }),
      ],
    });

    await user.click(screen.getByRole('radio', { name: 'Semantic' }));
    await user.click(screen.getByRole('button', { name: 'Show error events only' }));

    expect(screen.getByText('api_call')).toBeInTheDocument();
    expect(screen.queryByText('Which model?')).not.toBeInTheDocument();
    expect(screen.queryByText('Non-semantic error log')).not.toBeInTheDocument();
    expect(screen.queryByText('Synthetic failed span failed')).not.toBeInTheDocument();
  });

  it('intersects the Effects & waits filter with Errors only', async () => {
    const user = userEvent.setup();

    renderTimeline({
      events: [
        createTimelineEvent({
          id: 'effect-error',
          event_type: 'effect',
          level: 'error',
          payload: {
            effect_kind: 'tool_call',
            has_external_side_effect: true,
          },
        }),
        createTimelineEvent({
          id: 'wait-info',
          event_type: 'wait',
          payload: {
            wait_kind: 'human_approval',
            phase: 'entered',
          },
        }),
        createTimelineEvent({
          id: 'log-error',
          event_type: 'log',
          level: 'error',
          message: 'Plain error log',
        }),
      ],
    });

    await user.click(screen.getByRole('radio', { name: 'Effects & waits' }));
    await user.click(screen.getByRole('button', { name: 'Show error events only' }));

    expect(screen.getByText('tool_call')).toBeInTheDocument();
    expect(screen.queryByText('human_approval')).not.toBeInTheDocument();
    expect(screen.queryByText('Plain error log')).not.toBeInTheDocument();
  });

  it.each([
    {
      name: 'all with errors off',
      events: [],
      filterLabel: 'All',
      showErrorsOnly: false,
      message: 'No timeline events recorded for this trace yet.',
    },
    {
      name: 'all with errors only',
      events: [
        createTimelineEvent({
          event_type: 'message',
          message: 'Non-error event',
        }),
      ],
      filterLabel: 'All',
      showErrorsOnly: true,
      message: 'No error events for this trace.',
    },
    {
      name: 'semantic with errors off',
      events: [
        createTimelineEvent({
          event_type: 'message',
          message: 'Narrative event',
        }),
      ],
      filterLabel: 'Semantic',
      showErrorsOnly: false,
      message: 'No semantic events for this trace.',
    },
    {
      name: 'effects and waits with errors off',
      events: [
        createTimelineEvent({
          event_type: 'decision',
          payload: {
            question: 'Which model?',
            chosen: 'gpt-4.1',
          },
        }),
      ],
      filterLabel: 'Effects & waits',
      showErrorsOnly: false,
      message: 'No effect or wait events for this trace.',
    },
    {
      name: 'semantic with errors only',
      events: [
        createTimelineEvent({
          event_type: 'decision',
          payload: {
            question: 'Which model?',
            chosen: 'gpt-4.1',
          },
        }),
      ],
      filterLabel: 'Semantic',
      showErrorsOnly: true,
      message: 'No error-level semantic events for this trace.',
    },
    {
      name: 'effects and waits with errors only',
      events: [
        createTimelineEvent({
          event_type: 'effect',
          payload: {
            effect_kind: 'tool_call',
            has_external_side_effect: false,
          },
        }),
      ],
      filterLabel: 'Effects & waits',
      showErrorsOnly: true,
      message: 'No error-level effect or wait events for this trace.',
    },
  ])('renders the correct empty state for $name', async ({
    events,
    filterLabel,
    showErrorsOnly,
    message,
  }) => {
    const user = userEvent.setup();

    renderTimeline({ events });

    if (filterLabel !== 'All') {
      await user.click(screen.getByRole('radio', { name: filterLabel }));
    }
    if (showErrorsOnly) {
      await user.click(screen.getByRole('button', { name: 'Show error events only' }));
    }

    expect(screen.getByText(message)).toBeInTheDocument();
  });

  it('lets loading and error states take precedence over filtered empty states', async () => {
    const user = userEvent.setup();
    const onSelectSpan = vi.fn();
    const spanIndex = new Map();

    const { rerender } = render(
      <Timeline
        events={[
          createTimelineEvent({
            event_type: 'message',
            message: 'Narrative event',
          }),
        ]}
        traceStatus="RUNNING"
        isLive={true}
        onSelectSpan={onSelectSpan}
        spanIndex={spanIndex}
      />
    );

    await user.click(screen.getByRole('radio', { name: 'Semantic' }));
    expect(screen.getByText('No semantic events for this trace.')).toBeInTheDocument();

    rerender(
      <Timeline
        events={[]}
        traceStatus="RUNNING"
        isLive={true}
        isLoading={true}
        onSelectSpan={onSelectSpan}
        spanIndex={spanIndex}
      />
    );
    expect(screen.getByText('Loading timeline...')).toBeInTheDocument();

    rerender(
      <Timeline
        events={[]}
        traceStatus="FAILED"
        isLive={false}
        error="Timeline failed"
        onSelectSpan={onSelectSpan}
        spanIndex={spanIndex}
      />
    );
    expect(screen.getByText('Timeline failed')).toBeInTheDocument();
  });

  it('exposes the segmented control as a radiogroup with aria-checked state', async () => {
    const user = userEvent.setup();

    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'effect',
          payload: {
            effect_kind: 'tool_call',
            has_external_side_effect: true,
          },
        }),
      ],
    });

    expect(
      screen.getByRole('radiogroup', { name: 'Timeline event filter' })
    ).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'All' })).toHaveAttribute(
      'aria-checked',
      'true'
    );
    expect(screen.getByRole('radio', { name: 'Semantic' })).toHaveAttribute(
      'aria-checked',
      'false'
    );

    await user.click(screen.getByRole('radio', { name: 'Semantic' }));
    expect(screen.getByRole('radio', { name: 'Semantic' })).toHaveAttribute(
      'aria-checked',
      'true'
    );
    expect(screen.getByRole('radio', { name: 'All' })).toHaveAttribute(
      'aria-checked',
      'false'
    );
  });

  it('supports arrow-key navigation within the segmented control', async () => {
    const user = userEvent.setup();

    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'effect',
          payload: {
            effect_kind: 'tool_call',
            has_external_side_effect: true,
          },
        }),
      ],
    });

    const all = screen.getByRole('radio', { name: 'All' });
    const semantic = screen.getByRole('radio', { name: 'Semantic' });

    all.focus();
    await user.keyboard('{ArrowRight}');
    expect(semantic).toHaveFocus();
    expect(semantic).toHaveAttribute('aria-checked', 'true');

    await user.keyboard('{ArrowLeft}');
    expect(all).toHaveFocus();
    expect(all).toHaveAttribute('aria-checked', 'true');
  });

  it('applies an active filter to newly appended polled events', async () => {
    const user = userEvent.setup();
    const onSelectSpan = vi.fn();
    const spanIndex = new Map();

    const { rerender } = render(
      <Timeline
        events={[
          createTimelineEvent({
            id: 'initial-log',
            event_type: 'message',
            message: 'Initial log event',
          }),
        ]}
        traceStatus="RUNNING"
        isLive={true}
        onSelectSpan={onSelectSpan}
        spanIndex={spanIndex}
      />
    );

    await user.click(screen.getByRole('radio', { name: 'Effects & waits' }));
    expect(
      screen.getByText('No effect or wait events for this trace.')
    ).toBeInTheDocument();

    rerender(
      <Timeline
        events={[
          createTimelineEvent({
            id: 'initial-log',
            event_type: 'message',
            message: 'Initial log event',
          }),
          createTimelineEvent({
            id: 'polled-effect',
            event_type: 'effect',
            payload: {
              effect_kind: 'tool_call',
              has_external_side_effect: true,
            },
          }),
          createTimelineEvent({
            id: 'polled-log',
            event_type: 'log',
            message: 'Poll update log',
          }),
        ]}
        traceStatus="RUNNING"
        isLive={true}
        onSelectSpan={onSelectSpan}
        spanIndex={spanIndex}
      />
    );

    expect(screen.getByText('tool_call')).toBeInTheDocument();
    expect(screen.queryByText('Initial log event')).not.toBeInTheDocument();
    expect(screen.queryByText('Poll update log')).not.toBeInTheDocument();
  });
});

describe('Timeline retry safety badges', () => {
  it('renders retry-safety badges for well-formed effect rows on failed traces', () => {
    renderTimeline({
      traceStatus: 'FAILED',
      events: [
        createTimelineEvent({
          id: 'retryable-effect',
          event_type: 'effect',
          payload: {
            effect_kind: 'model_call',
            has_external_side_effect: false,
          },
        }),
        createTimelineEvent({
          id: 'unsafe-effect',
          event_type: 'effect',
          payload: {
            effect_kind: 'api_call',
            has_external_side_effect: true,
            idempotent: false,
          },
        }),
        createTimelineEvent({
          id: 'unknown-effect',
          event_type: 'effect',
          payload: {
            effect_kind: 'tool_call',
            has_external_side_effect: true,
          },
        }),
      ],
    });

    expect(screen.getByLabelText(getAccessibleSummary('retryable'))).toBeInTheDocument();
    expect(screen.getByLabelText(getAccessibleSummary('unsafe'))).toBeInTheDocument();
    expect(screen.getByLabelText(getAccessibleSummary('unknown'))).toBeInTheDocument();
  });

  it('renders an Unknown badge for malformed effect rows on failed traces', () => {
    renderTimeline({
      traceStatus: 'FAILED',
      events: [
        createTimelineEvent({
          id: 'bad-effect',
          event_type: 'effect',
          message: 'Malformed effect summary',
          payload: {
            effect_kind: 'api_call',
          },
        }),
      ],
    });

    expect(screen.getByText('Malformed effect summary')).toBeInTheDocument();
    expect(screen.getByLabelText(getAccessibleSummary('unknown'))).toBeInTheDocument();
  });

  it.each(['COMPLETED', 'RUNNING'] as const)(
    'does not render retry-safety badges for effect rows on %s traces',
    (traceStatus) => {
      renderTimeline({
        traceStatus,
        events: [
          createTimelineEvent({
            event_type: 'effect',
            payload: {
              effect_kind: 'model_call',
              has_external_side_effect: false,
            },
          }),
        ],
      });

      expect(screen.queryByLabelText(getAccessibleSummary('retryable'))).not.toBeInTheDocument();
      expect(screen.queryByLabelText(getAccessibleSummary('unsafe'))).not.toBeInTheDocument();
      expect(screen.queryByLabelText(getAccessibleSummary('unknown'))).not.toBeInTheDocument();
    }
  );

  it('keeps non-effect rows unchanged on failed traces', () => {
    renderTimeline({
      traceStatus: 'FAILED',
      events: [
        createTimelineEvent({
          event_type: 'message',
          message: 'Non-effect row',
        }),
      ],
    });

    expect(screen.getByText('Non-effect row')).toBeInTheDocument();
    expect(screen.queryByLabelText(getAccessibleSummary('retryable'))).not.toBeInTheDocument();
    expect(screen.queryByLabelText(getAccessibleSummary('unsafe'))).not.toBeInTheDocument();
    expect(screen.queryByLabelText(getAccessibleSummary('unknown'))).not.toBeInTheDocument();
  });
});
