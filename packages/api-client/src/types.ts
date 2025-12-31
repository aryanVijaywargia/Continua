// Type re-exports will be populated after proto generation
// For now, export placeholder types

export interface AgentExecution {
  executionId: string;
  tenantId: string;
  agentType: string;
  status: string;
}

export interface AgentEvent {
  eventId: number;
  eventType: string;
  eventTime: Date;
}

export type AgentExecutionStatus =
  | 'UNSPECIFIED'
  | 'RUNNING'
  | 'COMPLETED'
  | 'FAILED'
  | 'CANCELLED';

export interface HealthResponse {
  status: string;
  version: string;
  commit: string;
  build_time: string;
}
