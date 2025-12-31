import { describe, it, expect } from 'vitest';
import { ContinuaClient } from '@continua/api-client';

const API_URL = process.env.CONTINUA_API_URL || 'http://localhost:8243';

describe('Health', () => {
  it('should return healthy status with all fields', async () => {
    const client = new ContinuaClient({ baseUrl: API_URL });
    const health = await client.health();

    expect(health.status).toBe('ok');
    expect(health.version).toBeDefined();
    expect(health.commit).toBeDefined();
    expect(health.build_time).toBeDefined();
  });
});
