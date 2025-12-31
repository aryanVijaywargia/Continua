import { describe, it, expect } from 'vitest';
import { ContinuaClient } from './client';

describe('ContinuaClient', () => {
  it('should create a client with config', () => {
    const client = new ContinuaClient({
      baseUrl: 'http://localhost:8243',
    });
    expect(client).toBeDefined();
  });

  it('should create a client with auth token', () => {
    const client = new ContinuaClient({
      baseUrl: 'http://localhost:8243',
      authToken: 'test-token',
    });
    expect(client).toBeDefined();
  });
});
