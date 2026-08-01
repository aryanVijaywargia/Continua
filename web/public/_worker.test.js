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
    const detail = await detailResponse.json();
    expect(detail.engine.result).toEqual({
      greeting: 'hello, demo',
      approval: 'approved',
    });

    const historyResponse = await request(
      '/v1/engine/runs/' + completed.engine.run_id + '/history'
    );
    const history = await historyResponse.json();
    expect(history.events.map((event) => event.event_type)).toContain(
      'workflow.completed'
    );

    const resultResponse = await request(
      '/v1/engine/runs/' + completed.engine.run_id + '/result'
    );
    const result = await resultResponse.json();
    expect(result.status).toBe('COMPLETED');
    expect(result.result.greeting).toBe('hello, demo');

    const pendingResponse = await request(
      '/v1/engine/runs/' + waiting.engine.run_id + '/pending-work'
    );
    const pending = await pendingResponse.json();
    expect(pending.current_wait).toEqual({
      kind: 'signal',
      signal_name: 'approval',
    });

    const healthResponse = await request('/v1/engine/health');
    const health = await healthResponse.json();
    expect(health.projector.lag_rows).toBe(0);
    expect(health.workers[0].status).toBe('active');
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
