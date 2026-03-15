# TypeScript SDK status

The TypeScript SDK is currently a stub, not a tracing client.

## What exists

```
sdks/typescript/
├── package.json
├── src/index.ts
└── tests/client.test.ts
```

Current exported behavior:
- `VERSION`
- `ContinuaClient` with a configurable `baseUrl`

## What does not exist yet
- trace/span helpers
- batching
- ingest client
- provider integrations
- context propagation
- generated runtime client wrappers

## How to approach TS SDK work
- treat anything beyond the current stub as new product work
- check whether the change should start with OpenSpec
- do not document or implement against imaginary existing files like `client.ts`, `context.ts`, or adapter modules unless you are creating them in the task

## Current verification
- `pnpm --filter @continua/sdk test`
- `pnpm --filter @continua/sdk type-check`
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
