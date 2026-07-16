import { expect, test, type Page, type Route, type TestInfo } from '@playwright/test';
import {
  OTHER_SESSION_ID,
  SESSION_COMPARE,
  SESSION_ID,
  SESSION_NARRATIVE,
  SESSION_ONE,
  SESSION_TWO,
  TRACE_DETAIL,
  TRACE_ONE,
  TRACE_THREE,
  TRACE_TWO,
  RUNNING_SESSION_NARRATIVE,
} from '../src/test/traceFixtures';

const E2E_OPERATOR_TOKEN = 'e2e-operator-token';
const PRIMARY_PROJECT_ID = '11111111-1111-1111-1111-111111111111';
const SECONDARY_PROJECT_ID = '22222222-2222-2222-2222-222222222222';
const RUN_LOCALLY_DOCS_URL = 'https://www.continua.in/docs/guides/installation';
const ENGINE_RUN_ID = '123e4567-e89b-12d3-a456-426614174100';
const ENGINE_TRACE_ID = 'engine-trace-1';
const STARTED_ENGINE_TRACE_ID = 'engine-trace-new';

const ENGINE_TRACE = {
  ...TRACE_ONE,
  id: ENGINE_TRACE_ID,
  name: 'Checkout workflow',
  status: 'COMPLETED',
  error_count: 0,
  engine: {
    run_id: ENGINE_RUN_ID,
    definition_name: 'checkout',
    definition_version: 'v1',
    projection_state: 'up_to_date',
    instance_key: 'checkout-42',
    status: 'COMPLETED',
    pending_work: {
      pending_activity_tasks: 0,
      pending_inbox_items: 0,
    },
    created_at: '2026-03-14T10:00:00.000Z',
    updated_at: '2026-03-14T10:00:03.000Z',
    completed_at: '2026-03-14T10:00:03.000Z',
  },
};

const ENGINE_TRACE_DETAIL = {
  ...TRACE_DETAIL,
  ...ENGINE_TRACE,
  trace_id: 'engine-checkout-42',
  output: { approved: true },
};

const TRACE_DETAILS = new Map([
  [TRACE_ONE.id, TRACE_DETAIL],
  [ENGINE_TRACE_ID, ENGINE_TRACE_DETAIL],
  [STARTED_ENGINE_TRACE_ID, { ...ENGINE_TRACE_DETAIL, id: STARTED_ENGINE_TRACE_ID }],
  [
    TRACE_TWO.id,
    {
      ...TRACE_DETAIL,
      ...TRACE_TWO,
      trace_id: 'external-trace-latency',
      user_id: 'user-456',
      tags: ['latency'],
      input: { prompt: 'latency check' },
      output: { answer: 'running' },
    },
  ],
  [
    TRACE_THREE.id,
    {
      ...TRACE_DETAIL,
      ...TRACE_THREE,
      trace_id: 'external-trace-alpha',
      user_id: 'user-123',
      tags: ['alpha'],
      input: { prompt: 'alpha' },
      output: { answer: 'complete' },
    },
  ],
]);

const ALL_TRACES = [TRACE_ONE, TRACE_TWO, TRACE_THREE];
const TRACE_SPANS = {
  spans: [
    {
      id: 'span-root',
      trace_id: TRACE_ONE.id,
      span_id: 'root',
      name: 'Checkout root',
      kind: 'CHAIN',
      status: 'FAILED',
      started_at: TRACE_ONE.started_at,
      ended_at: TRACE_ONE.ended_at,
      latency_ms: 2000,
      tokens_in: 120,
      tokens_out: 80,
      cost_usd: 0.12,
      metadata: { phase: 'checkout' },
    },
    {
      id: 'span-tool',
      trace_id: TRACE_ONE.id,
      span_id: 'tool-call',
      parent_span_id: 'root',
      name: 'Checkout tool',
      kind: 'TOOL',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:01.000Z',
      ended_at: '2026-03-14T10:00:02.000Z',
      latency_ms: 1000,
      error_message: 'Gateway timeout',
      metadata: { tool: 'charge-card' },
    },
  ],
};
const TRACE_TIMELINE = {
  events: [
    {
      id: 'timeline-error',
      trace_id: TRACE_ONE.id,
      span_id: 'tool-call',
      span_name: 'Checkout tool',
      event_type: 'error',
      timestamp: '2026-03-14T10:00:02.000Z',
      source: 'explicit',
      level: 'error',
      message: 'Gateway timeout',
      payload: { code: 'gateway_timeout' },
    },
  ],
  trace_status: 'FAILED',
  has_more: false,
};

async function bootstrapOperatorSession(page: Page) {
  await page.addInitScript((operatorToken) => {
    const e2eWindow = window as typeof window & {
      __CONTINUA_E2E_AUTH_BYPASS__?: boolean;
      __CONTINUA_E2E_AUTH_TOKEN__?: string;
    };
    e2eWindow.__CONTINUA_E2E_AUTH_BYPASS__ = true;
    e2eWindow.__CONTINUA_E2E_AUTH_TOKEN__ = operatorToken;
    window.localStorage.setItem('continua_api_key', operatorToken);
    window.localStorage.setItem('continua_theme_mode', 'light');
  }, E2E_OPERATOR_TOKEN);
}

async function fulfillJson(route: Route, data: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(data),
  });
}

function expectOperatorAuthHeader(route: Route) {
  expect(route.request().headers().authorization).toBe(
    `Bearer ${E2E_OPERATOR_TOKEN}`
  );
}

function filterTraces(url: URL) {
  let traces = [...ALL_TRACES];
  const sessionId = url.searchParams.get('session_id');
  const status = url.searchParams.get('status');
  const q = url.searchParams.get('q')?.toLowerCase();
  const userId = url.searchParams.get('user_id');
  const hasErrors = url.searchParams.get('has_errors') === 'true';
  const minDurationMs = Number(url.searchParams.get('min_duration_ms') ?? '');

  if (sessionId) {
    traces = traces.filter((trace) => trace.session_id === sessionId);
  }

  if (status) {
    traces = traces.filter((trace) => trace.status.toLowerCase() === status);
  }

  if (q) {
    traces = traces.filter((trace) => {
      const detail = TRACE_DETAILS.get(trace.id);
      return (
        trace.name.toLowerCase().includes(q) ||
        detail?.user_id?.toLowerCase().includes(q) ||
        trace.session_external_id?.toLowerCase().includes(q)
      );
    });
  }

  if (userId) {
    traces = traces.filter((trace) => TRACE_DETAILS.get(trace.id)?.user_id === userId);
  }

  if (hasErrors) {
    traces = traces.filter((trace) => (trace.error_count ?? 0) > 0);
  }

  if (!Number.isNaN(minDurationMs) && minDurationMs > 0) {
    traces = traces.filter((trace) => {
      if (!trace.ended_at) {
        return false;
      }

      return (
        new Date(trace.ended_at).getTime() - new Date(trace.started_at).getTime() >=
        minDurationMs
      );
    });
  }

  const offset = Number(url.searchParams.get('offset') ?? '0');
  const limit = Number(url.searchParams.get('limit') ?? '20');

  return {
    total: traces.length,
    traces: traces.slice(offset, offset + limit),
  };
}

function filterSessions(url: URL) {
  let sessions = [SESSION_ONE, SESSION_TWO];
  const q = url.searchParams.get('q')?.toLowerCase();
  const userId = url.searchParams.get('user_id');

  if (q) {
    sessions = sessions.filter(
      (session) =>
        session.external_id.toLowerCase().includes(q) ||
        session.name?.toLowerCase().includes(q) ||
        session.user_id?.toLowerCase().includes(q)
    );
  }

  if (userId) {
    sessions = sessions.filter((session) => session.user_id === userId);
  }

  const offset = Number(url.searchParams.get('offset') ?? '0');
  const limit = Number(url.searchParams.get('limit') ?? '20');

  return {
    total: sessions.length,
    sessions: sessions.slice(offset, offset + limit),
  };
}

async function mockApiRoutes(page: Page, mode: 'operator' | 'public-demo' = 'operator') {
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());

    if (url.pathname === '/api/auth/config') {
      if (mode === 'public-demo') {
        return fulfillJson(route, {
          enabled: false,
          public_demo_enabled: true,
          public_demo_label: 'Sample data',
        });
      }

      return fulfillJson(route, {
        enabled: true,
        domain: 'continua.us.auth0.com',
        client_id: 'e2e-client-id',
        audience: 'https://continua/e2e',
      });
    }

    if (mode === 'operator') {
      expectOperatorAuthHeader(route);
    }

    if (url.pathname === '/api/projects') {
      if (mode === 'public-demo') {
        return fulfillJson(route, { code: 'not_found', message: 'Resource not found' }, 404);
      }

      return fulfillJson(route, {
        projects: [
          {
            id: PRIMARY_PROJECT_ID,
            name: 'Primary Project',
            created_at: '2026-03-14T10:00:00.000Z',
            updated_at: '2026-03-14T10:00:00.000Z',
          },
          {
            id: SECONDARY_PROJECT_ID,
            name: 'Secondary Project',
            created_at: '2026-03-15T10:00:00.000Z',
            updated_at: '2026-03-15T10:00:00.000Z',
          },
        ],
      });
    }

    if (url.pathname === '/api/traces') {
      if (url.searchParams.get('engine_only') === 'true') {
        return fulfillJson(route, { traces: [ENGINE_TRACE], total: 1 });
      }
      return fulfillJson(route, filterTraces(url));
    }

    if (/^\/api\/traces\/[^/]+\/spans$/.test(url.pathname)) {
      return fulfillJson(route, TRACE_SPANS);
    }

    if (/^\/api\/traces\/[^/]+\/events$/.test(url.pathname)) {
      return fulfillJson(route, TRACE_TIMELINE);
    }

    if (/^\/api\/traces\/[^/]+$/.test(url.pathname)) {
      const traceId = url.pathname.split('/').at(-1) ?? '';
      return fulfillJson(
        route,
        TRACE_DETAILS.get(traceId) ?? { ...TRACE_DETAIL, id: traceId }
      );
    }

    if (url.pathname === '/api/sessions') {
      return fulfillJson(route, filterSessions(url));
    }

    if (/^\/api\/sessions\/[^/]+\/narrative$/.test(url.pathname)) {
      const sessionId = url.pathname.split('/').at(-2);
      return fulfillJson(
        route,
        sessionId === SESSION_ID ? SESSION_NARRATIVE : RUNNING_SESSION_NARRATIVE
      );
    }

    if (/^\/api\/sessions\/[^/]+\/compare$/.test(url.pathname)) {
      return fulfillJson(route, SESSION_COMPARE);
    }

    if (/^\/api\/sessions\/[^/]+$/.test(url.pathname)) {
      const sessionId = url.pathname.split('/').at(-1);
      if (sessionId === SESSION_ID) {
        return fulfillJson(route, SESSION_ONE);
      }
      if (sessionId === OTHER_SESSION_ID) {
        return fulfillJson(route, SESSION_TWO);
      }
      return fulfillJson(route, { code: 'not_found', message: 'Resource not found' }, 404);
    }

    return fulfillJson(route, { code: 'not_found', message: 'Resource not found' }, 404);
  });
}

async function mockEngineRoutes(page: Page) {
  await page.route('**/v1/engine/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    expectOperatorAuthHeader(route);

    if (url.pathname === '/v1/engine/definitions' && request.method() === 'GET') {
      return fulfillJson(route, {
        definitions: [
          {
            definition_name: 'checkout',
            definition_version: 'v1',
            enabled: true,
            live: true,
            runtime_published_at: '2026-03-14T09:00:00.000Z',
            published_at: '2026-03-14T08:00:00.000Z',
          },
          {
            definition_name: 'legacy-checkout',
            definition_version: 'v0',
            enabled: true,
            live: false,
            runtime_published_at: '2026-03-01T09:00:00.000Z',
            published_at: '2026-03-01T08:00:00.000Z',
          },
        ],
      });
    }

    if (/^\/v1\/engine\/instances\/[^/]+$/.test(url.pathname)) {
      return fulfillJson(route, { code: 'not_found', message: 'Instance not found' }, 404);
    }

    if (url.pathname === '/v1/engine/runs' && request.method() === 'POST') {
      expect(request.headers()['x-continua-engine-preview']).toBe('1');
      expect(request.postDataJSON()).toMatchObject({
        instance_key: 'checkout-42',
        definition_name: 'checkout',
        definition_version: 'v1',
      });
      return fulfillJson(route, {
        run_id: ENGINE_RUN_ID,
        instance_key: 'checkout-42',
        trace_id: STARTED_ENGINE_TRACE_ID,
      });
    }

    if (url.pathname === `/v1/engine/runs/${ENGINE_RUN_ID}/pending-work`) {
      return fulfillJson(route, {
        run_id: ENGINE_RUN_ID,
        current_wait: null,
        activities: [],
        timers: [],
        signals: [],
        pending_activity_tasks: 0,
        pending_inbox_items: 0,
      });
    }

    if (url.pathname === `/v1/engine/runs/${ENGINE_RUN_ID}/history`) {
      return fulfillJson(route, { events: [], has_more: false, expired: false });
    }

    if (url.pathname === `/v1/engine/runs/${ENGINE_RUN_ID}/result`) {
      return fulfillJson(route, {
        run_id: ENGINE_RUN_ID,
        status: 'COMPLETED',
        result: { approved: true },
      });
    }

    if (
      url.pathname === '/v1/engine/projections/backfill' &&
      request.method() === 'POST'
    ) {
      expect(request.headers()['x-continua-engine-preview']).toBe('1');
      expect(request.postDataJSON()).toEqual({
        dry_run: true,
        limit: 50,
        engine_projection_state: 'summary_only',
      });
      return fulfillJson(route, {
        dry_run: true,
        limit: 50,
        eligible_count: 1,
        repair_requested_count: 0,
        skipped_count: 0,
        results: [
          {
            run_id: ENGINE_RUN_ID,
            trace_id: ENGINE_TRACE_ID,
            projection_state: 'summary_only',
            action: 'would_repair',
            reason: 'repair_requested',
          },
        ],
      });
    }

    return fulfillJson(route, { code: 'not_found', message: 'Resource not found' }, 404);
  });
}

async function gotoAndCapture(
  page: Page,
  testInfo: TestInfo,
  path: string,
  waitFor: () => Promise<unknown>,
  screenshotName: string
) {
  await page.goto(path);
  await waitFor();
  await page.evaluate(async () => {
    if ('fonts' in document) {
      await document.fonts.ready;
    }
  });
  const screenshot = await page.screenshot({ fullPage: true, animations: 'disabled' });
  await testInfo.attach(screenshotName, {
    body: screenshot,
    contentType: 'image/png',
  });
}

test('opens a protected route and switches the selected project', async ({ page }, testInfo) => {
  const isMobile = testInfo.project.name === 'mobile-chromium';
  if (!isMobile) {
    await page.setViewportSize({ width: 1024, height: 900 });
  }
  await bootstrapOperatorSession(page);
  await mockApiRoutes(page, 'operator');
  await page.goto('/traces');

  await expect(page.getByRole('heading', { name: 'Traces' })).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`project_id=${PRIMARY_PROJECT_ID}`));

  if (isMobile) {
    await page.getByRole('button', { name: 'Open navigation' }).click();
    const projectSwitcher = page.getByRole('combobox', { name: 'Project' });
    await expect(projectSwitcher).toBeVisible();
    await expect(projectSwitcher).toHaveValue(PRIMARY_PROJECT_ID);
    await projectSwitcher.selectOption(SECONDARY_PROJECT_ID);
    await expect(page).toHaveURL(new RegExp(`project_id=${SECONDARY_PROJECT_ID}`));
    await page.getByRole('button', { name: 'Open navigation' }).click();
    await expect(page.getByRole('combobox', { name: 'Project' })).toHaveValue(
      SECONDARY_PROJECT_ID
    );
    return;
  }

  const projectSwitcher = page.getByLabel('Active project');
  await expect(projectSwitcher).toBeVisible();
  await expect(projectSwitcher).toHaveValue(PRIMARY_PROJECT_ID);
  await projectSwitcher.selectOption(SECONDARY_PROJECT_ID);
  await expect(page).toHaveURL(new RegExp(`project_id=${SECONDARY_PROJECT_ID}`));
  await expect(projectSwitcher).toHaveValue(SECONDARY_PROJECT_ID);
});

test('captures overview, traces, sessions, and settings shells', async ({ page }, testInfo) => {
  const prefix = testInfo.project.name;
  await bootstrapOperatorSession(page);
  await mockApiRoutes(page, 'operator');

  await gotoAndCapture(
    page,
    testInfo,
    `/dashboard?project_id=${PRIMARY_PROJECT_ID}`,
    () => expect(page.getByRole('heading', { name: 'Recent traces' })).toBeVisible(),
    `${prefix}-overview`
  );

  await gotoAndCapture(
    page,
    testInfo,
    `/traces?project_id=${PRIMARY_PROJECT_ID}`,
    () => expect(page.getByRole('heading', { name: 'Traces' })).toBeVisible(),
    `${prefix}-traces`
  );

  await gotoAndCapture(
    page,
    testInfo,
    `/sessions?project_id=${PRIMARY_PROJECT_ID}`,
    () => expect(page.getByRole('heading', { name: 'Sessions' })).toBeVisible(),
    `${prefix}-sessions`
  );

  await gotoAndCapture(
    page,
    testInfo,
    `/settings?project_id=${PRIMARY_PROJECT_ID}`,
    () => expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible(),
    `${prefix}-settings`
  );
});

test('captures trace, session, and compare workspaces', async ({ page }, testInfo) => {
  const prefix = testInfo.project.name;
  const isMobile = testInfo.project.name === 'mobile-chromium';
  await bootstrapOperatorSession(page);
  await mockApiRoutes(page, 'operator');

  await gotoAndCapture(
    page,
    testInfo,
    `/traces/${TRACE_ONE.id}?project_id=${PRIMARY_PROJECT_ID}`,
    () =>
      isMobile
        ? expect(page.getByRole('button', { name: 'Summary' })).toBeVisible()
        : expect(page.getByRole('heading', { name: 'Execution Waterfall' })).toBeVisible(),
    `${prefix}-trace-detail`
  );

  await gotoAndCapture(
    page,
    testInfo,
    `/sessions/${SESSION_ID}?project_id=${PRIMARY_PROJECT_ID}`,
    () => expect(page.getByRole('heading', { name: 'Session journey' })).toBeVisible(),
    `${prefix}-session-detail`
  );

  await gotoAndCapture(
    page,
    testInfo,
    `/sessions/${SESSION_ID}/compare?project_id=${PRIMARY_PROJECT_ID}&baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`,
    () => expect(page.getByRole('heading', { name: 'Span Diff' })).toBeVisible(),
    `${prefix}-session-compare`
  );
});

test('covers the engine runs console smoke flows', async ({ page }, testInfo) => {
  const isMobile = testInfo.project.name === 'mobile-chromium';
  await bootstrapOperatorSession(page);
  await mockApiRoutes(page, 'operator');
  await mockEngineRoutes(page);
  await page.goto(`/traces?project_id=${PRIMARY_PROJECT_ID}`);

  if (isMobile) {
    await page.getByRole('button', { name: 'Open navigation', exact: true }).click();
  }
  const navigation = page.getByRole('navigation', {
    name: isMobile ? 'Mobile primary' : 'Primary',
    exact: true,
  });
  expect((await navigation.getByRole('link').allTextContents()).map((text) => text.trim()))
    .toEqual(['Overview', 'Traces', 'Engine Runs', 'Sessions', 'Projects', 'Settings']);

  const engineRunsRequest = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return url.pathname === '/api/traces' && url.searchParams.get('engine_only') === 'true';
  });
  await navigation.getByRole('link', { name: 'Engine Runs', exact: true }).click();
  expect(new URL((await engineRunsRequest).url()).searchParams.get('engine_only')).toBe('true');
  await expect(page).toHaveURL(
    `/engine/runs?project_id=${PRIMARY_PROJECT_ID}`
  );
  const engineRunRow = page.getByRole('row').filter({ hasText: 'checkout · v1' });
  await expect(engineRunRow.getByText('checkout · v1', { exact: true })).toBeVisible();
  await expect(engineRunRow.getByText('Completed', { exact: true })).toBeVisible();

  await page.getByRole('button', { name: 'Start run', exact: true }).click();
  const startForm = page
    .getByRole('heading', { name: 'Start run', exact: true })
    .locator('xpath=ancestor::form');
  await startForm.getByRole('button', { name: 'Enter manually', exact: true }).click();
  await startForm.getByLabel(/^Instance key/).fill('checkout-42');
  await startForm.getByLabel(/^Definition name/).fill('checkout');
  await startForm.getByLabel(/^Definition version/).fill('v1');
  const startRequest = page.waitForRequest(
    (request) =>
      new URL(request.url()).pathname === '/v1/engine/runs' &&
      request.method() === 'POST'
  );
  await startForm.getByRole('button', { name: 'Start run', exact: true }).click();
  await startRequest;
  await expect(page).toHaveURL(
    new RegExp(String.raw`/traces/${STARTED_ENGINE_TRACE_ID}\?project_id=${PRIMARY_PROJECT_ID}`)
  );

  await page.getByRole('button', { name: 'Engine state', exact: true }).click();
  await expect(page.getByRole('button', { name: /^Overview \d+$/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /^Pending \d+$/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /^Engine history \d+$/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /^Result \d+$/ })).toBeVisible();

  await page.goto(`/settings?project_id=${PRIMARY_PROJECT_ID}`);
  const operationsSection = page
    .getByRole('heading', { name: 'Operations', exact: true })
    .locator('xpath=ancestor::section[1]');
  await expect(operationsSection).toBeVisible();
  await operationsSection
    .getByRole('link', { name: /^Engine projection repair/ })
    .click();
  await expect(page).toHaveURL(
    `/tools/engine-projections?project_id=${PRIMARY_PROJECT_ID}`
  );
  await expect(
    page.getByRole('heading', { name: 'Engine projection repair', exact: true })
  ).toBeVisible();
  await expect(
    page.getByRole('button', { name: 'Apply repair', exact: true })
  ).toHaveCount(0);
  await page.getByRole('button', { name: 'Run dry run', exact: true }).click();
  await expect(page.getByText(ENGINE_RUN_ID, { exact: true })).toBeVisible();
  await expect(page.getByText('Dry run', { exact: true })).toBeVisible();

  const screenshot = await page.screenshot({ fullPage: true, animations: 'disabled' });
  await testInfo.attach(`${testInfo.project.name}-engine-projection-dry-run`, {
    body: screenshot,
    contentType: 'image/png',
  });
});

test('starts an engine run from the live definition picker', async ({ page }, testInfo) => {
  await bootstrapOperatorSession(page);
  await mockApiRoutes(page, 'operator');
  await mockEngineRoutes(page);
  await page.goto(`/engine/runs?project_id=${PRIMARY_PROJECT_ID}`);

  await page.getByRole('button', { name: 'Start run', exact: true }).click();
  const startForm = page
    .getByRole('heading', { name: 'Start run', exact: true })
    .locator('xpath=ancestor::form');
  await startForm.getByLabel(/^Instance key/).fill('checkout-42');
  await startForm.getByRole('button', { name: 'Check', exact: true }).click();
  await expect(startForm.getByText('Instance key is available.')).toBeVisible();
  await startForm.getByLabel(/^Definition name/).selectOption('checkout');
  await startForm.getByLabel(/^Definition version/).selectOption('v1');
  await expect(
    startForm.getByRole('option', { name: /legacy-checkout.*not live/i })
  ).toHaveJSProperty('disabled', true);

  const screenshot = await page.screenshot({ fullPage: true, animations: 'disabled' });
  await testInfo.attach(`${testInfo.project.name}-engine-definition-picker`, {
    body: screenshot,
    contentType: 'image/png',
  });

  const startRequest = page.waitForRequest(
    (request) =>
      new URL(request.url()).pathname === '/v1/engine/runs' &&
      request.method() === 'POST'
  );
  await startForm.getByRole('button', { name: 'Start run', exact: true }).click();
  expect((await startRequest).postDataJSON()).toMatchObject({
    instance_key: 'checkout-42',
    definition_name: 'checkout',
    definition_version: 'v1',
  });
  await expect(page).toHaveURL(
    new RegExp(String.raw`/traces/${STARTED_ENGINE_TRACE_ID}\?project_id=${PRIMARY_PROJECT_ID}`)
  );
});

test('walks the public demo flow from landing through debugger reads', async ({ page }, testInfo) => {
  const isMobile = testInfo.project.name === 'mobile-chromium';
  if (!isMobile) {
    await page.setViewportSize({ width: 1024, height: 900 });
  }
  await page.addInitScript(() => {
    window.localStorage.setItem('continua_theme_mode', 'dark');
  });

  await mockApiRoutes(page, 'public-demo');

  await page.goto('/');
  await expect(page.getByRole('link', { name: 'Open Demo' }).first()).toBeVisible();
  await expect(page.getByRole('link', { name: 'Run Locally' }).first()).toHaveAttribute(
    'href',
    RUN_LOCALLY_DOCS_URL
  );

  await page.getByRole('link', { name: 'Open Demo' }).first().click();
  await expect(page).toHaveURL(/\/dashboard$/);
  await expect(page.getByText(/read-only demo/i)).toBeVisible();
  await expect(page.getByText(/sample traces/i)).toBeVisible();
  await expect(page.getByRole('link', { name: 'Run locally' })).toBeVisible();

  await page.goto('/traces');
  await expect(page.getByRole('heading', { name: 'Traces' })).toBeVisible();
  await expect(page.getByLabel('Active project')).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Settings' })).toHaveCount(0);

  await page.goto(`/traces/${TRACE_ONE.id}`);
  await expect(
    isMobile
      ? page.getByRole('button', { name: 'Summary' })
      : page.getByRole('heading', { name: 'Execution Waterfall' })
  ).toBeVisible();
  if (!isMobile) {
    const costStrip = page
      .locator('svg[aria-label="Cumulative cost chart"]')
      .locator('xpath=ancestor::div[contains(@class, "grid")][1]');
    await expect(costStrip).toBeVisible();
    await expect(costStrip).not.toHaveCSS(
      'background-color',
      'rgb(255, 255, 255)'
    );
  }

  await page.goto(`/sessions/${SESSION_ID}`);
  await expect(page.getByRole('heading', { name: 'Session journey' })).toBeVisible();

  await page.goto(
    `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`
  );
  await expect(page.getByRole('heading', { name: 'Span Diff' })).toBeVisible();
});
