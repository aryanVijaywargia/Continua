import { zodToJsonSchema } from 'zod-to-json-schema';
import { writeFileSync } from 'fs';
import { WebSocketEvent, ClientMessage } from './events';

const serverEventsSchema = zodToJsonSchema(WebSocketEvent, {
  name: 'WebSocketEvent',
  $refStrategy: 'none',
});

const clientMessagesSchema = zodToJsonSchema(ClientMessage, {
  name: 'ClientMessage',
  $refStrategy: 'none',
});

const combined = {
  $schema: 'http://json-schema.org/draft-07/schema#',
  definitions: {
    WebSocketEvent: serverEventsSchema,
    ClientMessage: clientMessagesSchema,
  },
};

writeFileSync(
  new URL('./events.schema.json', import.meta.url),
  JSON.stringify(combined, null, 2)
);

console.log('✅ Generated events.schema.json');
