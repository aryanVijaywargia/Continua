import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { getAccessibleSummary } from '../utils/retrySafety';
import { createTimelineEvent } from '../test/traceFixtures';
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

  it('falls back to the message when semantic fields are missing', () => {
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

  it('renders well-formed wait rows with semantic summaries', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'wait',
          payload: {
            wait_kind: 'tool_call',
            phase: 'entered',
          },
        }),
        createTimelineEvent({
          event_type: 'wait',
          payload: {
            wait_kind: 'model_response',
            phase: 'resolved',
            resolution: 'success',
          },
        }),
      ],
    });

    expect(screen.getByText('Entered wait: tool_call')).toBeInTheDocument();
    expect(
      screen.getByText('Resolved wait: model_response → success')
    ).toBeInTheDocument();
  });

  it('renders phase-only wait fallbacks when strict parsing fails', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'wait',
          payload: {
            phase: 'paused',
          },
        }),
      ],
    });

    expect(screen.getByText('Paused wait')).toBeInTheDocument();
  });

  it('falls back to generic wait text when the wait payload is fully malformed', () => {
    renderTimeline({
      events: [
        createTimelineEvent({
          event_type: 'wait',
          message: 'Generic wait fallback',
          payload: {},
        }),
      ],
    });

    expect(screen.getByText('Generic wait fallback')).toBeInTheDocument();
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
