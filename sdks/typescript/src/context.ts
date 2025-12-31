export interface Message {
  role: 'system' | 'user' | 'assistant' | 'tool';
  content: string;
}

export class AgentContext {
  private messages: Message[] = [];
  private memory: Map<string, unknown> = new Map();

  addMessage(message: Message) {
    this.messages.push(message);
  }

  getMessages(): Message[] {
    return [...this.messages];
  }

  setMemory(key: string, value: unknown) {
    this.memory.set(key, value);
  }

  getMemory<T>(key: string): T | undefined {
    return this.memory.get(key) as T | undefined;
  }

  isReplaying(): boolean {
    // TODO: Implement replay detection
    return false;
  }
}
