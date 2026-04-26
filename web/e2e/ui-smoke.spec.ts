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
const RUN_LOCALLY_DOCS_URL =
  'https://github.com/aryanVijaywargia/Continua/blob/main/docs/guides/run-locally.md';

const TRACE_DETAILS = new Map([
  [TRACE_ONE.id, TRACE_DETAIL],
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
        session.name?.toLowerCase().includes(q)
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
      expect(route.request().headers().authorization).toBe(
        `Bearer ${E2E_OPERATOR_TOKEN}`
      );
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

  await expect(
    page.getByRole('heading', {
      name: /Find the run, isolate the failure, and jump straight into the workspace/i,
    })
  ).toBeVisible();
  await expect(page).toHaveURL(new RegExp(`project_id=${PRIMARY_PROJECT_ID}`));

  if (isMobile) {
    await page.getByRole('button', { name: 'Open navigation' }).click();
    const projectSwitcher = page.locator('#mobile-project-switcher');
    await expect(projectSwitcher).toBeVisible();
    await expect(projectSwitcher).toHaveValue(PRIMARY_PROJECT_ID);
    await projectSwitcher.selectOption(SECONDARY_PROJECT_ID);
    await expect(page).toHaveURL(new RegExp(`project_id=${SECONDARY_PROJECT_ID}`));
    await page.getByRole('button', { name: 'Open navigation' }).click();
    await expect(page.locator('#mobile-project-switcher')).toHaveValue(
      SECONDARY_PROJECT_ID
    );
    return;
  }

  const projectSwitcher = page.locator('#project-switcher');
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
    () =>
      expect(
        page.getByRole('heading', {
          name: /Trace the work that matters before it turns into support debt/i,
        })
      ).toBeVisible(),
    `${prefix}-overview`
  );

  await gotoAndCapture(
    page,
    testInfo,
    `/traces?project_id=${PRIMARY_PROJECT_ID}`,
    () =>
      expect(
        page.getByRole('heading', {
          name: /Find the run, isolate the failure, and jump straight into the workspace/i,
        })
      ).toBeVisible(),
    `${prefix}-traces`
  );

  await gotoAndCapture(
    page,
    testInfo,
    `/sessions?project_id=${PRIMARY_PROJECT_ID}`,
    () =>
      expect(
        page.getByRole('heading', {
          name: /Follow a user journey across multiple traces without losing narrative context/i,
        })
      ).toBeVisible(),
    `${prefix}-sessions`
  );

  await gotoAndCapture(
    page,
    testInfo,
    `/settings?project_id=${PRIMARY_PROJECT_ID}`,
    () =>
      expect(
        page.getByRole('heading', {
          name: /Manage your operator session and debugger workspace/i,
        })
      ).toBeVisible(),
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
    () => expect(page.getByRole('heading', { name: 'Traces' })).toBeVisible(),
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

test('walks the public demo flow from landing through debugger reads', async ({ page }, testInfo) => {
  const isMobile = testInfo.project.name === 'mobile-chromium';
  if (!isMobile) {
    await page.setViewportSize({ width: 1024, height: 900 });
  }

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
  await expect(page.getByRole('link', { name: 'Run locally with your own traces' })).toBeVisible();

  await page.goto('/traces');
  await expect(
    page.getByRole('heading', {
      name: /Find the run, isolate the failure, and jump straight into the workspace/i,
    })
  ).toBeVisible();
  await expect(page.locator('#project-switcher')).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Settings' })).toHaveCount(0);

  await page.goto(`/traces/${TRACE_ONE.id}`);
  await expect(
    isMobile
      ? page.getByRole('button', { name: 'Summary' })
      : page.getByRole('heading', { name: 'Execution Waterfall' })
  ).toBeVisible();

  await page.goto(`/sessions/${SESSION_ID}`);
  await expect(page.getByRole('heading', { name: 'Traces' })).toBeVisible();

  await page.goto(
    `/sessions/${SESSION_ID}/compare?baseline_trace_id=${SESSION_COMPARE.baseline.id}&candidate_trace_id=${SESSION_COMPARE.candidate.id}`
  );
  await expect(page.getByRole('heading', { name: 'Span Diff' })).toBeVisible();
});
