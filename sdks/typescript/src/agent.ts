import { ContinuaClient } from '@continua/api-client';
import { AgentContext } from './context';

export interface AgentConfig {
  name: string;
  version?: string;
  maxIterations?: number;
}

export abstract class Agent {
  protected readonly config: AgentConfig;
  protected client?: ContinuaClient;

  constructor(config: AgentConfig) {
    this.config = config;
  }

  abstract run(context: AgentContext, input: unknown): Promise<unknown>;

  setClient(client: ContinuaClient) {
    this.client = client;
  }
}
