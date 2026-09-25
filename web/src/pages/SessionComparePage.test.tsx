import { screen, waitFor, within } from '@testing-library/react';
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

const COMPARE_TITLE = 'Checkout Session';
const COMPARE_URL = `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`;

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
      await screen.findByRole('heading', { name: COMPARE_TITLE })
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

    expect(await screen.findByRole('heading', { name: COMPARE_TITLE })).toBeInTheDocument();
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

    const toolbar = screen.getByRole('region', { name: 'Comparison toolbar' });
    expect(within(toolbar).getByText('-1.00s total (-33.3%)')).toBeInTheDocument();
    expect(within(toolbar).getByText('spans 1 → 2')).toBeInTheDocument();
    expect(within(toolbar).queryByText('usage not verified')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Step comparison' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Changed only · 2' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
    expect(screen.getByText('first change')).toBeInTheDocument();
    expect(screen.getByText('added')).toBeInTheDocument();
    expect(screen.getByText('aligned by span ID · 1 of 2 matched')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Ask/i })).not.toBeInTheDocument();

    const planRow = screen.getByRole('button', { name: 'Show details for Plan' }).closest('tr');
    expect(planRow).toHaveTextContent('+1.00s · +50.0%');

    await user.click(screen.getByRole('button', { name: 'Show details for Plan' }));
    await user.click(screen.getByRole('button', { name: 'Show details for Retry Tool' }));

    expect(await screen.findAllByText('Pick alpha path')).toHaveLength(2);
    expect(screen.getByText('Called retry tool')).toBeInTheDocument();
    expect(screen.getByText('Baseline timing')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Hide details for Plan' })).toHaveAttribute(
      'aria-expanded',
      'true'
    );
  });

  it('flags unverified usage instead of showing a usage delta', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () =>
          jsonResponse({
            ...SESSION_COMPARE,
            baseline: {
              ...SESSION_COMPARE.baseline,
              total_tokens_in: 0,
              total_tokens_out: 0,
              total_cost_usd: 0,
            },
          }),
      })
    );

    renderTraceRoutes([COMPARE_URL]);

    const toolbar = await screen.findByRole('region', { name: 'Comparison toolbar' });
    expect(within(toolbar).getByText('usage not verified')).toBeInTheDocument();
    expect(within(toolbar).queryByText(/^tokens /)).not.toBeInTheDocument();
  });

  it('swaps the pair when a picker selects the other role trace', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () => jsonResponse(SESSION_COMPARE),
      })
    );

    const { router } = renderTraceRoutes([COMPARE_URL]);

    const candidateSelect = await screen.findByRole('combobox', { name: 'Candidate trace' });
    expect(candidateSelect).toHaveValue(SESSION_COMPARE.candidate.id);

    await user.selectOptions(candidateSelect, SESSION_COMPARE.baseline.id);

    await waitFor(() => {
      const params = new URLSearchParams(router.state.location.search);
      expect(params.get('baseline_trace_id')).toBe(SESSION_COMPARE.candidate.id);
      expect(params.get('candidate_trace_id')).toBe(SESSION_COMPARE.baseline.id);
    });

    await user.click(await screen.findByRole('button', { name: 'Swap baseline and candidate' }));

    await waitFor(() => {
      const params = new URLSearchParams(router.state.location.search);
      expect(params.get('baseline_trace_id')).toBe(SESSION_COMPARE.baseline.id);
    });
  });

  it('shows engine metadata in compare headers when present', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () =>
          jsonResponse({
            ...SESSION_COMPARE,
            baseline: {
              ...SESSION_COMPARE.baseline,
              engine: {
                run_id: '123e4567-e89b-12d3-a456-426614174102',
                definition_name: 'checkout',
                definition_version: 'v1',
                projection_state: 'catching_up',
              },
            },
          }),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    expect(await screen.findByRole('heading', { name: COMPARE_TITLE })).toBeInTheDocument();
    expect(screen.getByText('checkout@v1 · Catching up')).toBeInTheDocument();
    expect(screen.getByText('Engine')).toBeInTheDocument();
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
      await screen.findByRole('heading', { name: COMPARE_TITLE })
    ).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Show details for Plan' }));

    expect(screen.getByText('Stable ID')).toBeInTheDocument();
    expect(screen.getByText('tokens_in')).toBeInTheDocument();

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

    expect(await screen.findByRole('heading', { name: COMPARE_TITLE })).toBeInTheDocument();
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

    expect(await screen.findByRole('heading', { name: COMPARE_TITLE })).toBeInTheDocument();
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

    expect(await screen.findByRole('heading', { name: COMPARE_TITLE })).toBeInTheDocument();
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

    expect(
      await screen.findByRole('button', { name: /Trace Context/i })
    ).toBeInTheDocument();
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

    await user.click(await screen.findByRole('button', { name: 'Show details for Plan' }));
    const baselineSpanLink = screen.getByRole('link', { name: 'Open baseline span' });
    expect(baselineSpanLink).toHaveAttribute(
      'href',
      `/traces/${SESSION_COMPARE.baseline.id}?span=shared-span`
    );
    expect(screen.getByRole('link', { name: 'Open candidate span' })).toHaveAttribute(
      'href',
      `/traces/${SESSION_COMPARE.candidate.id}?span=shared-span`
    );

    await user.click(baselineSpanLink);

    await waitFor(() => {
      expect(router.state.location.pathname).toBe(`/traces/${SESSION_COMPARE.baseline.id}`);
    });

    expect(
      await screen.findByRole('button', { name: /Trace Context/i })
    ).toBeInTheDocument();
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

    expect(await screen.findByRole('heading', { name: COMPARE_TITLE })).toBeInTheDocument();
    expect(screen.getByText('No span rows were returned for this comparison. Both traces may be empty.')).toBeInTheDocument();
  });

  it('says no payload difference is established for rows without semantic groups', async () => {
    const user = userEvent.setup();
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

    expect(await screen.findByRole('heading', { name: COMPARE_TITLE })).toBeInTheDocument();
    expect(screen.queryByText('No payload difference established')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Show details for Plan' }));

    expect(screen.getByText('No payload difference established')).toBeInTheDocument();
    expect(screen.getByText(/No semantic events were recorded for this step/)).toBeInTheDocument();
  });

  it('survives null changed-field arrays from older compare responses', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () =>
          jsonResponse({
            ...SESSION_COMPARE,
            span_diffs: [
              {
                ...SESSION_COMPARE.span_diffs[1],
                changed_fields: null,
                semantic_groups: SESSION_COMPARE.span_diffs[1].semantic_groups.map((group) => ({
                  ...group,
                  changed_fields: null,
                })),
              },
            ],
          }),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    ]);

    expect(
      await screen.findByRole('heading', { name: COMPARE_TITLE })
    ).toBeInTheDocument();
    expect(screen.getByText('Retry Tool')).toBeInTheDocument();

    await userEvent.setup().click(
      screen.getByRole('button', { name: 'Show details for Retry Tool' })
    );
    expect(screen.getByText('Called retry tool')).toBeInTheDocument();
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
    expect(screen.getByRole('link', { name: 'Sign in again' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`
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

  it('collapses unchanged steps and reveals them on request', async () => {
    const user = userEvent.setup();
    const planRow = SESSION_COMPARE.span_diffs[0];
    const unchangedRow = {
      ...planRow,
      diff_status: 'unchanged' as const,
      changed_fields: [],
      semantic_groups: [],
      baseline_span: planRow.baseline_span
        ? { ...planRow.baseline_span, id: 'load-cart-baseline', span_id: 'load-cart', name: 'Load Cart' }
        : null,
      candidate_span: planRow.candidate_span
        ? { ...planRow.candidate_span, id: 'load-cart-candidate', span_id: 'load-cart', name: 'Load Cart' }
        : null,
    };
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionCompare: () =>
          jsonResponse({
            ...SESSION_COMPARE,
            span_diffs: [unchangedRow, ...SESSION_COMPARE.span_diffs],
          }),
      })
    );

    renderTraceRoutes([COMPARE_URL]);

    expect(await screen.findByRole('heading', { name: COMPARE_TITLE })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Changed only · 2' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
    expect(
      screen.queryByRole('button', { name: 'Show details for Load Cart' })
    ).not.toBeInTheDocument();
    expect(screen.getByText(/1 step unchanged — same status, timing, and usage/)).toBeInTheDocument();
    expect(screen.getByText('Load Cart')).toBeInTheDocument();
    expect(screen.getByText('aligned by span ID · 2 of 3 matched')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Show them' }));

    expect(screen.getByRole('button', { name: 'Show all 3' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
    expect(screen.getByRole('button', { name: 'Show details for Load Cart' })).toBeInTheDocument();
    expect(screen.getByText('unchanged')).toBeInTheDocument();
    expect(screen.queryByText(/step unchanged/)).not.toBeInTheDocument();
    // The first changed row keeps the FIRST CHANGE label even when unchanged rows show above it.
    expect(
      screen.getByRole('button', { name: 'Show details for Plan' }).closest('tr')
    ).toHaveTextContent('first change');

    await user.click(screen.getByRole('button', { name: 'Changed only · 2' }));

    expect(
      screen.queryByRole('button', { name: 'Show details for Load Cart' })
    ).not.toBeInTheDocument();
  });
});
