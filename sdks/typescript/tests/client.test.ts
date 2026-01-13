import { describe, it, expect } from 'vitest';
import { ContinuaClient, VERSION } from '../src/index';

describe('ContinuaClient', () => {
  it('should have a version', () => {
    expect(VERSION).toBe('0.0.0');
  });

  it('should create a client with default baseUrl', () => {
    const client = new ContinuaClient();
    expect(client.baseUrl).toBe('http://localhost:8080');
  });

  it('should create a client with custom baseUrl', () => {
    const client = new ContinuaClient({ baseUrl: 'https://api.example.com' });
    expect(client.baseUrl).toBe('https://api.example.com');
  });
});
