import type { HealthResponse } from './types';

export interface ContinuaClientConfig {
  baseUrl: string;
  authToken?: string;
}

export class ContinuaClient {
  private readonly config: ContinuaClientConfig;

  constructor(config: ContinuaClientConfig) {
    this.config = config;
  }

  async health(): Promise<HealthResponse> {
    const response = await fetch(`${this.config.baseUrl}/health`);
    if (!response.ok) {
      throw new Error(`Health check failed: ${response.status}`);
    }
    return response.json() as Promise<HealthResponse>;
  }

  // Placeholder methods - will be implemented with Connect clients after proto generation
  async startExecution(agentType: string, input: unknown) {
    // TODO: Implement with generated Connect client
    throw new Error('Not implemented');
  }

  async getExecution(executionId: string) {
    // TODO: Implement with generated Connect client
    throw new Error('Not implemented');
  }

  async listExecutions() {
    // TODO: Implement with generated Connect client
    throw new Error('Not implemented');
  }

  async listEvents(executionId: string) {
    // TODO: Implement with generated Connect client
    throw new Error('Not implemented');
  }

  async replayExecution(executionId: string) {
    // TODO: Implement with generated Connect client
    throw new Error('Not implemented');
  }
}
