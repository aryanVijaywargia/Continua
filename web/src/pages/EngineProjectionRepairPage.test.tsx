import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { clearApiKey, setApiKey } from '../api/client';
import { ThemeProvider } from '../hooks/ThemeProvider';
import {
  createDeferredResponse,
  jsonResponse,
  readRequestUrl,
  type RequestInput,
} from './testUtils';
import { EngineProjectionRepairPage } from './EngineProjectionRepairPage';

let fetchMock: ReturnType<typeof vi.fn>;

const RESULT_ROWS = [
  {
    run_id: 'run-repair-1',
    trace_id: 'trace-repair-1',
    projection_state: 'summary_only',
    action: 'would_repair',
    reason: 'repair_requested',
  },
  {
    run_id: 'run-repair-2',
    trace_id: 'trace-repair-2',
    projection_state: 'summary_only',
    action: 'would_repair',
    reason: 'no_events_to_project',
  },
];

function backfillResponse({
  dryRun = true,
  eligibleCount = 2,
  results = RESULT_ROWS,
}: {
  dryRun?: boolean;
  eligibleCount?: number;
  results?: typeof RESULT_ROWS;
} = {}) {
  return {
    dry_run: dryRun,
    limit: 50,
    eligible_count: eligibleCount,
    repair_requested_count: dryRun ? 0 : eligibleCount,
    skipped_count: 0,
    results,
  };
}

function renderRepairPage(initialEntry = '/tools/engine-projections') {
  return render(
    <ThemeProvider>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route
            path="/tools/engine-projections"
            element={<EngineProjectionRepairPage />}
          />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );
}

function backfillCalls(): Array<[RequestInput, RequestInit]> {
  return fetchMock.mock.calls.filter(([input, init]) => {
    const url = new URL(readRequestUrl(input as RequestInput), 'http://localhost');
    return (
      url.pathname === '/v1/engine/projections/backfill' &&
      (init as RequestInit | undefined)?.method === 'POST'
    );
  }) as Array<[RequestInput, RequestInit]>;
}

function requestBody(call: [RequestInput, RequestInit]) {
  return JSON.parse(call[1].body as string) as Record<string, unknown>;
}

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

describe('EngineProjectionRepairPage', () => {
  it('renders the form with the summary_only default and helper copy', () => {
    renderRepairPage();

    expect(
      screen.getByRole('heading', { name: 'Engine projection repair' })
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/^Projection state/)).toHaveValue('summary_only');
    expect(
      screen.getByText(
        'Default repair target is summary_only. up_to_date, catching_up, and journal_expired return zero eligible rows.'
      )
    ).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Apply repair' })).not.toBeInTheDocument();
  });

  it('posts an exact dry-run body then an exact apply body through the confirmation modal', async () => {
    const user = userEvent.setup();
    fetchMock
      .mockResolvedValueOnce(jsonResponse(backfillResponse()))
      .mockResolvedValueOnce(
        jsonResponse(backfillResponse({ dryRun: false }))
      );
    renderRepairPage();

    await user.click(screen.getByRole('button', { name: 'Run dry run' }));

    expect(await screen.findByText('run-repair-1')).toBeInTheDocument();
    expect(backfillCalls()).toHaveLength(1);
    expect(requestBody(backfillCalls()[0])).toEqual({
      dry_run: true,
      limit: 50,
      engine_projection_state: 'summary_only',
    });

    const applyButton = screen.getByRole('button', { name: 'Apply repair' });
    expect(applyButton).toBeEnabled();
    await user.click(applyButton);

    expect(
      screen.getByText(/^Last dry run found 2 eligible runs\./)
    ).toBeInTheDocument();
    const modal = screen
      .getByRole('heading', { name: 'Apply projection repair' })
      .closest('.fixed');
    expect(modal).not.toBeNull();
    await user.click(
      within(modal as HTMLElement).getByRole('button', { name: 'Apply repair' })
    );

    await waitFor(() => expect(backfillCalls()).toHaveLength(2));
    expect(requestBody(backfillCalls()[1])).toEqual({
      dry_run: false,
      limit: 50,
      engine_projection_state: 'summary_only',
    });
  });

  it('renders a successful empty result when zero runs are eligible', async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValueOnce(
      jsonResponse(backfillResponse({ eligibleCount: 0, results: [] }))
    );
    renderRepairPage();

    await user.click(screen.getByRole('button', { name: 'Run dry run' }));

    expect(await screen.findByRole('heading', { name: 'No eligible runs' })).toBeInTheDocument();
    expect(
      screen.getByText(
        'The dry run completed successfully and found no repair targets.'
      )
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Apply repair' })).toBeDisabled();
    expect(screen.getByText('No eligible runs to repair.')).toBeInTheDocument();
    expect(screen.queryByText('Backend failure')).not.toBeInTheDocument();
  });

  it('converts older_than local input to a UTC ISO-8601 string', async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValueOnce(
      jsonResponse(backfillResponse({ eligibleCount: 0, results: [] }))
    );
    renderRepairPage();
    const localValue = '2026-03-14T10:00';

    fireEvent.change(screen.getByLabelText(/^Older than/), {
      target: { value: localValue },
    });
    await user.click(screen.getByRole('button', { name: 'Run dry run' }));

    await waitFor(() => expect(backfillCalls()).toHaveLength(1));
    expect(requestBody(backfillCalls()[0]).older_than).toBe(
      new Date(localValue).toISOString()
    );
  });

  it('blocks submission client-side when limit exceeds 100', async () => {
    const user = userEvent.setup();
    renderRepairPage();
    const limit = screen.getByLabelText(/^Limit/);

    await user.clear(limit);
    await user.type(limit, '150');

    expect(screen.getByText('Limit must be 100 or less.')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Run dry run' }));
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('clears results and disables apply when a filter changes after a dry run', async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValueOnce(jsonResponse(backfillResponse()));
    renderRepairPage();

    await user.click(screen.getByRole('button', { name: 'Run dry run' }));
    expect(await screen.findByText('run-repair-1')).toBeInTheDocument();

    await user.type(screen.getByLabelText(/^Instance key/), 'instance-filter');

    expect(screen.queryByText('run-repair-1')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Apply repair' })).toBeDisabled();
    expect(screen.getByText('Run a fresh dry run to apply.')).toBeInTheDocument();
  });

  it('prevents double submission and locks the form during apply', async () => {
    const user = userEvent.setup();
    const dryRunDeferred = createDeferredResponse();
    const applyDeferred = createDeferredResponse();
    fetchMock
      .mockImplementationOnce(() => dryRunDeferred.promise)
      .mockImplementationOnce(() => applyDeferred.promise);
    renderRepairPage();

    const dryRunButton = screen.getByRole('button', { name: 'Run dry run' });
    await user.click(dryRunButton);
    await user.click(dryRunButton);

    expect(backfillCalls()).toHaveLength(1);
    expect(screen.getByRole('button', { name: 'Running…' })).toBeDisabled();

    await act(async () => {
      dryRunDeferred.resolve(jsonResponse(backfillResponse()));
    });
    expect(await screen.findByText('run-repair-1')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Apply repair' }));
    const modal = screen
      .getByRole('heading', { name: 'Apply projection repair' })
      .closest('.fixed');
    expect(modal).not.toBeNull();
    await user.click(
      within(modal as HTMLElement).getByRole('button', { name: 'Apply repair' })
    );

    expect(screen.getByLabelText(/^Instance key/)).toBeDisabled();
    expect(screen.getByLabelText(/^Definition name/)).toBeDisabled();
    expect(screen.getByLabelText(/^Projection state/)).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Run dry run' })).toBeDisabled();
    const applyingButtons = screen.getAllByRole('button', { name: 'Applying…' });
    expect(applyingButtons).toHaveLength(2);
    applyingButtons.forEach((button) => expect(button).toBeDisabled());

    await act(async () => {
      applyDeferred.resolve(
        jsonResponse(backfillResponse({ dryRun: false }))
      );
    });
    await waitFor(() =>
      expect(
        screen.queryByRole('heading', { name: 'Apply projection repair' })
      ).not.toBeInTheDocument()
    );
    expect(backfillCalls().filter((call) => requestBody(call).dry_run === false)).toHaveLength(1);
  });

  it('maps backend 400 failures to a correction error without retrying or clearing form values', async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ code: 'invalid_request', message: 'Choose a supported filter.' }, 400)
    );
    renderRepairPage();
    const instanceKey = screen.getByLabelText(/^Instance key/);
    const definitionName = screen.getByLabelText(/^Definition name/);

    await user.type(instanceKey, 'instance-400');
    await user.type(definitionName, 'definition-400');
    await user.click(screen.getByRole('button', { name: 'Run dry run' }));

    expect(await screen.findByText('Request needs correction')).toBeInTheDocument();
    expect(screen.getByText(/Choose a supported filter\./)).toBeInTheDocument();
    expect(instanceKey).toHaveValue('instance-400');
    expect(definitionName).toHaveValue('definition-400');
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('maps backend 401 failures to the authentication banner', async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ code: 'invalid_api_key', message: 'Invalid API key' }, 401)
    );
    renderRepairPage();

    await user.click(screen.getByRole('button', { name: 'Run dry run' }));

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Authentication required');
    expect(alert).toHaveTextContent('Invalid API key');
  });

  it('maps backend 404 failures to the engine-disabled state', async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ code: 'not_found', message: 'Not found' }, 404)
    );
    renderRepairPage();

    await user.click(screen.getByRole('button', { name: 'Run dry run' }));

    expect(await screen.findByText('Engine API disabled')).toBeInTheDocument();
    expect(
      screen.getByText(/The projection repair endpoint is not available on this server\./)
    ).toBeInTheDocument();
  });

  it('maps backend 500 failures to the backend-failure state', async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ code: 'internal', message: 'Database unavailable' }, 500)
    );
    renderRepairPage();

    await user.click(screen.getByRole('button', { name: 'Run dry run' }));

    expect(await screen.findByText('Backend failure')).toBeInTheDocument();
  });

  it('links result rows to project-scoped trace URLs', async () => {
    const user = userEvent.setup();
    const projectId = '123e4567-e89b-12d3-a456-426614174999';
    fetchMock.mockResolvedValueOnce(
      jsonResponse({
        ...backfillResponse({ eligibleCount: 1, results: [RESULT_ROWS[0]] }),
        results: [
          {
            ...RESULT_ROWS[0],
            run_id: 'run-x',
            trace_id: 'trace-x',
          },
        ],
      })
    );
    renderRepairPage(`/tools/engine-projections?project_id=${projectId}`);

    await user.click(screen.getByRole('button', { name: 'Run dry run' }));

    expect(await screen.findByRole('link', { name: 'trace-x' })).toHaveAttribute(
      'href',
      `/traces/trace-x?project_id=${projectId}`
    );
  });
});
