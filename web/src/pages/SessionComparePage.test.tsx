import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import {
  SESSION_COMPARE,
  SESSION_ID,
  buildFetchHandler,
  createSpan,
  createDeferredResponse,
  jsonResponse,
  renderTraceRoutes,
} from './testUtils';

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  localStorage.clear();
  setApiKey('test-key');
});

afterEach(() => {
  clearApiKey();
  localStorage.clear();
  vi.unstubAllGlobals();
});

describe('SessionComparePage', () => {
  it('shows a loading state while the comparison request is in flight', async () => {
    const deferredCompare = createDeferredResponse();
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () => deferredCompare.promise,
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    expect(await screen.findByText('Loading comparison...')).toBeInTheDocument();

    deferredCompare.resolve(jsonResponse(SESSION_COMPARE));

    expect(
      await screen.findByRole('heading', { name: SESSION_COMPARE.session.external_id })
    ).toBeInTheDocument();
  });

  it('renders overview, diff rows, and semantic expansion', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () => jsonResponse(SESSION_COMPARE),
      })
    );

    renderTraceRoutes([
      {
        pathname: `/sessions/${SESSION_ID}/compare`,
        search: `?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
        state: { returnTo: `/sessions/${SESSION_ID}?offset=20` },
      },
    ]);

    expect(await screen.findByRole('heading', { name: SESSION_COMPARE.session.external_id })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Back to Session/i })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}?offset=20`
    );
    expect(screen.getByRole('link', { name: SESSION_COMPARE.baseline.name })).toHaveAttribute(
      'href',
      `/traces/${SESSION_COMPARE.baseline.id}`
    );
    expect(screen.getByRole('link', { name: SESSION_COMPARE.candidate.name })).toHaveAttribute(
      'href',
      `/traces/${SESSION_COMPARE.candidate.id}`
    );

    const semanticButtons = screen.getAllByRole('button', { name: 'Show semantic details' });
    await user.click(semanticButtons[0]);
    await user.click(semanticButtons[1]);

    expect(await screen.findAllByText('Pick alpha path')).toHaveLength(2);
    expect(screen.getByText('Called retry tool')).toBeInTheDocument();
  });

  it('renders provenance badges and changed-field chips', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () => jsonResponse(SESSION_COMPARE),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    expect(
      await screen.findByRole('heading', { name: SESSION_COMPARE.session.external_id })
    ).toBeInTheDocument();
    expect(screen.getByText('Stable ID')).toBeInTheDocument();
    expect(screen.getByText('tokens_in')).toBeInTheDocument();

    await user.click(screen.getAllByRole('button', { name: 'Show semantic details' })[0]);

    expect(await screen.findByText('Heuristic')).toBeInTheDocument();
    expect(screen.getByText('chosen')).toBeInTheDocument();
  });

  it('renders a distinct group for candidate-only root branches', async () => {
    const comparisonWithCandidateOnlyRoot = {
      ...SESSION_COMPARE,
      span_diffs: [
        SESSION_COMPARE.span_diffs[0],
        {
          ...SESSION_COMPARE.span_diffs[1],
          candidate_span: SESSION_COMPARE.span_diffs[1].candidate_span
            ? {
                ...SESSION_COMPARE.span_diffs[1].candidate_span,
                parent_span_id: undefined,
              }
            : null,
          depth: 0,
        },
      ],
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () => jsonResponse(comparisonWithCandidateOnlyRoot),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    expect(await screen.findByRole('heading', { name: SESSION_COMPARE.session.external_id })).toBeInTheDocument();
    expect(screen.getByText('Candidate-only branches')).toBeInTheDocument();
  });

  it('falls back to the parent session URL when returnTo state is missing', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () => jsonResponse(SESSION_COMPARE),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    expect(await screen.findByRole('heading', { name: SESSION_COMPARE.session.external_id })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Back to Session/i })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`
    );
  });

  it('ignores invalid returnTo state and falls back to the canonical session URL', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () => jsonResponse(SESSION_COMPARE),
      })
    );

    renderTraceRoutes([
      {
        pathname: `/sessions/${SESSION_ID}/compare`,
        search: `?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
        state: { returnTo: '/traces' },
      },
    ]);

    expect(await screen.findByRole('heading', { name: SESSION_COMPARE.session.external_id })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Back to Session/i })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`
    );
  });

  it('preserves compare returnTo when opening a source trace', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () => jsonResponse(SESSION_COMPARE),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    expect(await screen.findByRole('link', { name: SESSION_COMPARE.baseline.name })).toBeInTheDocument();
    await user.click(screen.getByRole('link', { name: SESSION_COMPARE.baseline.name }));

    expect(await screen.findByText('Trace Context')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Session' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`
    );
  });

  it('routes span-row deep links through internal trace UUIDs while preserving compare returnTo', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () => jsonResponse(SESSION_COMPARE),
        spans: () =>
          jsonResponse({
            spans: [
              createSpan({
                id: 'shared-span-detail',
                trace_id: SESSION_COMPARE.baseline.id,
                span_id: 'shared-span',
                name: 'Plan',
                kind: 'AGENT',
                status: 'COMPLETED',
              }),
            ],
          }),
      })
    );

    const { router } = renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    const spanLinks = await screen.findAllByRole('link', { name: 'Plan' });
    expect(spanLinks[0]).toHaveAttribute(
      'href',
      `/traces/${SESSION_COMPARE.baseline.id}?span=shared-span`
    );

    await user.click(spanLinks[0]);

    await waitFor(() => {
      expect(router.state.location.pathname).toBe(`/traces/${SESSION_COMPARE.baseline.id}`);
    });

    expect(await screen.findByText('Trace Context')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Session' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`
    );
  });

  it('renders the empty diff state when no span rows are returned', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () =>
          jsonResponse({
            ...SESSION_COMPARE,
            span_diffs: [],
          }),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    expect(await screen.findByRole('heading', { name: SESSION_COMPARE.session.external_id })).toBeInTheDocument();
    expect(screen.getByText('No span rows were returned for this comparison. Both traces may be empty.')).toBeInTheDocument();
  });

  it('does not show a semantic expander for rows without semantic groups', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () =>
          jsonResponse({
            ...SESSION_COMPARE,
            span_diffs: [
              {
                ...SESSION_COMPARE.span_diffs[0],
                semantic_groups: [],
              },
            ],
          }),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    expect(await screen.findByRole('heading', { name: SESSION_COMPARE.session.external_id })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Show semantic details' })).not.toBeInTheDocument();
  });

  it('renders the 422 comparison ceiling detail state', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () =>
          jsonResponse(
            {
              code: 'comparison_too_large',
              message: 'Comparison exceeds the supported size limits',
              detail: {
                baseline_span_count: 501,
                candidate_span_count: 10,
                baseline_semantic_count: 200,
                candidate_semantic_count: 100,
                max_spans: 500,
                max_semantic_events: 1000,
              },
            },
            422
          ),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    expect(await screen.findByText('Comparison exceeds the v1 ceiling')).toBeInTheDocument();
    expect(screen.getByText('501 spans')).toBeInTheDocument();
    expect(screen.getByText('1000')).toBeInTheDocument();
  });

  it('renders auth recovery when the compare request returns 401', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () => jsonResponse({ message: 'Invalid or missing API key' }, 401),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid or missing API key');
    expect(screen.getByRole('link', { name: 'Go to Settings' })).toHaveAttribute(
      'href',
      '/settings'
    );
  });

  it('renders a generic error state for non-auth compare failures', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () => jsonResponse({ message: 'Comparison request failed' }, 500),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    expect(
      await screen.findByText('Error loading comparison: Comparison request failed')
    ).toBeInTheDocument();
  });

  it('keeps the compare page in its stacked layout contract', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () => jsonResponse(SESSION_COMPARE),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    const overviewHeading = await screen.findByRole('heading', {
      name: SESSION_COMPARE.session.external_id,
    });
    const overviewSection = overviewHeading.closest('section');
    const overviewLayout = overviewSection?.querySelector('div.flex');

    expect(overviewLayout).toHaveClass('flex-col');

    const semanticToggle = screen.getAllByRole('button', { name: 'Show semantic details' })[0];
    const spanRow = semanticToggle.closest('article');
    const spanGrid = spanRow?.querySelector('div.grid');

    expect(spanGrid).toHaveClass('lg:grid-cols-2');
  });
});
