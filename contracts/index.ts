import { fileURLToPath } from 'url';
import { dirname, resolve } from 'path';

export * from './websocket/events';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

export function getOpenAPIBundlePath(): string {
  return resolve(__dirname, '..', 'openapi', 'openapi.bundle.yaml');
}

export const OPENAPI_BUNDLE_URL = new URL(
  '../openapi/openapi.bundle.yaml',
  import.meta.url
);
