import { describe, it, expect } from 'vitest';
import { Agent, type AgentConfig } from './agent';
import { AgentContext } from './context';

class TestAgent extends Agent {
  async run(context: AgentContext, input: unknown): Promise<unknown> {
    return { received: input };
  }
}

describe('Agent', () => {
  it('should create an agent with config', () => {
    const config: AgentConfig = { name: 'test-agent' };
    const agent = new TestAgent(config);
    expect(agent).toBeDefined();
  });

  it('should run and return result', async () => {
    const agent = new TestAgent({ name: 'test-agent' });
    const context = new AgentContext();
    const result = await agent.run(context, { test: 'data' });
    expect(result).toEqual({ received: { test: 'data' } });
  });
});

describe('AgentContext', () => {
  it('should store and retrieve messages', () => {
    const context = new AgentContext();
    context.addMessage({ role: 'user', content: 'hello' });
    expect(context.getMessages()).toHaveLength(1);
    expect(context.getMessages()[0].content).toBe('hello');
  });

  it('should store and retrieve memory', () => {
    const context = new AgentContext();
    context.setMemory('key', 'value');
    expect(context.getMemory('key')).toBe('value');
  });
});
