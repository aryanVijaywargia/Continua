# TypeScript SDK

## Project Structure

```
sdks/typescript/
├── package.json
├── tsconfig.json
├── src/
│   ├── index.ts            # Public exports
│   ├── client.ts           # HTTP client
│   ├── tracer.ts           # Trace/span management
│   ├── context.ts          # AsyncLocalStorage context
│   ├── types.ts            # Type definitions
│   └── integrations/
│       └── vercel-ai.ts    # Vercel AI SDK integration
├── tests/
└── dist/                   # Built output
```

## Core API

### Basic Usage

```typescript
import { Continua } from '@continua/sdk';

const client = new Continua({
    apiKey: process.env.CONTINUA_API_KEY,
    baseUrl: 'http://localhost:8080'
});

// Functional API
await client.trace('agent_run', async (trace) => {
    await client.span('thinking', { kind: 'LLM' }, async (span) => {
        const response = await openai.chat.completions.create({...});
        span.setMetadata({ model: 'gpt-4', tokens: response.usage });
    });
});
```

### Decorator Style (Experimental)

```typescript
import { trace, span } from '@continua/sdk/decorators';

class Agent {
    @trace('agent_run')
    async run(input: string) {
        return this.think(input);
    }

    @span('thinking', { kind: 'LLM' })
    async think(input: string) {
        return openai.chat.completions.create({...});
    }
}
```

## Implementation

### Client

```typescript
// src/client.ts
import { Trace, Span, SpanKind, CreateTraceRequest, CreateSpanRequest } from './types';

export interface ContinuaConfig {
    apiKey: string;
    baseUrl?: string;
}

export class Continua {
    private baseUrl: string;
    private headers: HeadersInit;

    constructor(config: ContinuaConfig) {
        this.baseUrl = config.baseUrl ?? 'http://localhost:8080';
        this.headers = {
            'Authorization': `Bearer ${config.apiKey}`,
            'Content-Type': 'application/json'
        };
    }

    async trace<T>(name: string, fn: (trace: Trace) => Promise<T>): Promise<T> {
        const trace = await this.createTrace({ name });
        setCurrentTrace(trace.id);

        try {
            const result = await fn(trace);
            await this.completeTrace(trace.id, 'COMPLETED');
            return result;
        } catch (error) {
            await this.completeTrace(trace.id, 'FAILED');
            throw error;
        } finally {
            clearCurrentTrace();
        }
    }

    async span<T>(
        name: string,
        options: { kind?: SpanKind },
        fn: (span: Span) => Promise<T>
    ): Promise<T> {
        const traceId = getCurrentTrace();
        if (!traceId) throw new Error('No active trace');

        const span = await this.createSpan({
            traceId,
            parentSpanId: getCurrentSpan(),
            name,
            kind: options.kind ?? 'CUSTOM'
        });
        setCurrentSpan(span.id);

        try {
            const result = await fn(span);
            await this.completeSpan(span.id, 'COMPLETED');
            return result;
        } catch (error) {
            await this.completeSpan(span.id, 'FAILED');
            throw error;
        } finally {
            clearCurrentSpan();
        }
    }

    private async createTrace(req: CreateTraceRequest): Promise<Trace> {
        const resp = await fetch(`${this.baseUrl}/api/traces`, {
            method: 'POST',
            headers: this.headers,
            body: JSON.stringify(req)
        });
        return resp.json();
    }

    private async createSpan(req: CreateSpanRequest): Promise<Span> {
        const resp = await fetch(`${this.baseUrl}/api/spans`, {
            method: 'POST',
            headers: this.headers,
            body: JSON.stringify(req)
        });
        return resp.json();
    }
}
```

### Context with AsyncLocalStorage

```typescript
// src/context.ts
import { AsyncLocalStorage } from 'async_hooks';

interface TraceContext {
    traceId: string;
    spanId?: string;
}

const storage = new AsyncLocalStorage<TraceContext>();

export function getCurrentTrace(): string | undefined {
    return storage.getStore()?.traceId;
}

export function getCurrentSpan(): string | undefined {
    return storage.getStore()?.spanId;
}

export function setCurrentTrace(traceId: string): void {
    const store = storage.getStore();
    if (store) {
        store.traceId = traceId;
    } else {
        storage.enterWith({ traceId });
    }
}

export function setCurrentSpan(spanId: string): void {
    const store = storage.getStore();
    if (store) {
        store.spanId = spanId;
    }
}

export function clearCurrentTrace(): void {
    storage.disable();
}

export function clearCurrentSpan(): void {
    const store = storage.getStore();
    if (store) {
        store.spanId = undefined;
    }
}

// Run function with context
export function runWithContext<T>(context: TraceContext, fn: () => T): T {
    return storage.run(context, fn);
}
```

### Types

```typescript
// src/types.ts
export type SpanKind = 'LLM' | 'TOOL' | 'CHAIN' | 'AGENT' | 'CUSTOM';
export type SpanStatus = 'SCHEDULED' | 'STARTED' | 'COMPLETED' | 'FAILED';
export type TraceStatus = 'RUNNING' | 'COMPLETED' | 'FAILED';

export interface Trace {
    id: string;
    sessionId?: string;
    name: string;
    status: TraceStatus;
    startedAt: string;
    endedAt?: string;
    totalTokensIn?: number;
    totalTokensOut?: number;
    totalCostUsd?: number;
    metadata?: Record<string, unknown>;
}

export interface Span {
    id: string;
    traceId: string;
    parentSpanId?: string;
    name: string;
    kind: SpanKind;
    status: SpanStatus;
    startedAt: string;
    endedAt?: string;
    tokensIn?: number;
    tokensOut?: number;
    costUsd?: number;
    latencyMs?: number;
    errorMessage?: string;
    metadata?: Record<string, unknown>;

    setMetadata(metadata: Record<string, unknown>): void;
}

export interface CreateTraceRequest {
    name: string;
    sessionId?: string;
    metadata?: Record<string, unknown>;
}

export interface CreateSpanRequest {
    traceId: string;
    parentSpanId?: string;
    name: string;
    kind: SpanKind;
    metadata?: Record<string, unknown>;
}
```

### Vercel AI SDK Integration

```typescript
// src/integrations/vercel-ai.ts
import { Continua } from '../client';

export function wrapVercelAI(client: Continua) {
    return {
        async generateText(options: GenerateTextOptions) {
            return client.span('generateText', { kind: 'LLM' }, async (span) => {
                const result = await originalGenerateText(options);
                span.setMetadata({
                    model: options.model,
                    tokens: result.usage
                });
                return result;
            });
        }
    };
}
```

## Testing

```bash
cd sdks/typescript
pnpm test
```

```typescript
// tests/client.test.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { Continua } from '../src/client';

describe('Continua', () => {
    let client: Continua;

    beforeEach(() => {
        client = new Continua({
            apiKey: 'test',
            baseUrl: 'http://localhost:8080'
        });
    });

    it('creates trace with nested spans', async () => {
        await client.trace('test', async (trace) => {
            expect(trace.id).toBeDefined();

            await client.span('child', { kind: 'LLM' }, async (span) => {
                expect(span.traceId).toBe(trace.id);
            });
        });
    });
});
```

## Build

```bash
pnpm build  # Outputs to dist/
```

```json
// package.json
{
    "name": "@continua/sdk",
    "main": "./dist/index.js",
    "types": "./dist/index.d.ts",
    "exports": {
        ".": "./dist/index.js",
        "./decorators": "./dist/decorators.js"
    }
}
```
