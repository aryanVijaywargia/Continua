import { describe, expect, it, vi } from 'vitest';
import worker from './_worker.js';

const origin = 'https://demo.continua.test';

function createEnv() {
  return {
    ASSETS: {
      fetch: vi.fn(async () => new Response('Not found', { status: 404 })),
    },
  };
}

async function request(path, init) {
  return worker.fetch(new Request(origin + path, init), createEnv());
}

describe('public demo worker engine fixtures', () => {
  it('lists captured engine runs and filters by engine status', async () => {
    const response = await request('/api/traces?engine_only=true');
    const payload = await response.json();

    expect(response.status).toBe(200);
    expect(payload.total).toBe(2);
    expect(payload.traces.map((trace) => trace.engine.status)).toEqual([
      'WAITING',
      'COMPLETED',
    ]);
    expect(payload.traces.map((trace) => trace.engine.definition_name)).toEqual([
      'darklaunch.sleep-demo',
      'darklaunch.demo',
    ]);

    const filteredResponse = await request(
      '/api/traces?engine_only=true&engine_run_status=completed'
    );
    const filtered = await filteredResponse.json();

    expect(filtered.total).toBe(1);
    expect(filtered.traces[0].engine.status).toBe('COMPLETED');
  });

  it('serves engine detail, history, result, pending work, and health reads', async () => {
    const listResponse = await request('/api/traces?engine_only=true');
    const { traces } = await listResponse.json();
    const waiting = traces.find((trace) => trace.engine.status === 'WAITING');
    const completed = traces.find((trace) => trace.engine.status === 'COMPLETED');

    const detailResponse = await request('/api/traces/' + completed.id);
    expect(detailResponse.status).toBe(200);
    const detail = await detailResponse.json();
    expect(detail.engine.result).toEqual({
      greeting: 'hello, demo',
      approval: 'approved',
    });

    const historyResponse = await request(
      '/v1/engine/runs/' + completed.engine.run_id + '/history'
    );
    expect(historyResponse.status).toBe(200);
    const history = await historyResponse.json();
    expect(history.events.map((event) => event.event_type)).toContain(
      'workflow.completed'
    );

    const resultResponse = await request(
      '/v1/engine/runs/' + completed.engine.run_id + '/result'
    );
    expect(resultResponse.status).toBe(200);
    const result = await resultResponse.json();
    expect(result.status).toBe('COMPLETED');
    expect(result.result.greeting).toBe('hello, demo');

    const pendingResponse = await request(
      '/v1/engine/runs/' + waiting.engine.run_id + '/pending-work'
    );
    expect(pendingResponse.status).toBe(200);
    const pending = await pendingResponse.json();
    expect(pending.current_wait).toEqual({
      kind: 'signal',
      signal_name: 'approval',
    });

    const healthResponse = await request('/v1/engine/health');
    expect(healthResponse.status).toBe(200);
    const health = await healthResponse.json();
    expect(health.projector.lag_rows).toBe(0);
    expect(health.workers[0].status).toBe('active');
  });

  it('serves captured definitions and instances', async () => {
    const definitionsResponse = await request('/v1/engine/definitions');
    expect(definitionsResponse.status).toBe(200);
    const { definitions } = await definitionsResponse.json();
    expect(definitions.map((definition) => definition.definition_name)).toEqual([
      'darklaunch.demo',
      'darklaunch.sleep-demo',
    ]);

    const instanceResponse = await request('/v1/engine/instances/demo-approval-gate');
    expect(instanceResponse.status).toBe(200);
    expect((await instanceResponse.json()).current_run.status).toBe('WAITING');

    const missingResponse = await request('/v1/engine/instances/no-such-instance');
    expect(missingResponse.status).toBe(404);

    const malformedResponse = await request('/v1/engine/instances/%');
    expect(malformedResponse.status).toBe(404);
  });

  it('rejects engine writes in the read-only public demo', async () => {
    const response = await request('/v1/engine/runs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });

    expect(response.status).toBe(403);
    await expect(response.json()).resolves.toMatchObject({
      code: 'public_demo_read_only',
    });
  });
});

describe('public demo worker span payloads', () => {
  const traceId = 'e4271b6e-9b16-4c3d-a42a-07d04c0e5fa4';

  it('lists span summaries with presence flags and no inline payloads', async () => {
    const response = await request('/api/traces/' + traceId + '/spans');
    expect(response.status).toBe(200);
    const { spans } = await response.json();

    expect(spans.length).toBeGreaterThan(0);
    for (const span of spans) {
      expect(span).not.toHaveProperty('input');
      expect(span).not.toHaveProperty('output');
      expect(span).not.toHaveProperty('metadata');
      expect(span.has_input).toBe(true);
      expect(span.has_output).toBe(true);
    }

    const chainResponse = await request(
      '/api/traces/88e6b423-cb06-47dc-a649-dd9737b47ee5/spans'
    );
    const chain = (await chainResponse.json()).spans.find(
      (span) => span.name === 'code_analysis_chain'
    );
    expect(chain).toMatchObject({ has_input: false, has_output: false });
  });

  it('serves the captured payload of a known span', async () => {
    const listResponse = await request('/api/traces/' + traceId + '/spans');
    const [span] = (await listResponse.json()).spans;

    const response = await request(
      '/api/traces/' + traceId + '/spans/' + span.id + '/payload'
    );
    expect(response.status).toBe(200);
    const payload = await response.json();
    expect(payload.input[1]).toEqual({
      content: 'Plan the first dependency check for a stalled agent workflow.',
      role: 'user',
    });
    expect(payload.output.first_check).toMatch(/external dependencies/);
  });

  it('returns 404 for a span outside the trace', async () => {
    const otherSpanId = 'ad6a8500-16d5-490b-9ae9-370e335b2ac0';
    const response = await request(
      '/api/traces/' + traceId + '/spans/' + otherSpanId + '/payload'
    );
    expect(response.status).toBe(404);

    const unknownTrace = await request(
      '/api/traces/no-such-trace/spans/' + otherSpanId + '/payload'
    );
    expect(unknownTrace.status).toBe(404);
  });
});
