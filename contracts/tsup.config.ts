import { defineConfig } from 'tsup';

export default defineConfig({
  entry: ['index.ts', 'websocket/events.ts'],
  format: ['esm'],
  dts: true,
  clean: true,
  sourcemap: true,
});
