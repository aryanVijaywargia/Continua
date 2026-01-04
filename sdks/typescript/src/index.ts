/**
 * Continua SDK for TypeScript/JavaScript
 */

export const VERSION = '0.0.0';

// TODO: Implement SDK client
export class ContinuaClient {
  constructor(private options: { baseUrl?: string } = {}) {}

  get baseUrl(): string {
    return this.options.baseUrl ?? 'http://localhost:8080';
  }
}
