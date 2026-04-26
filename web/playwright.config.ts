import { defineConfig, devices } from '@playwright/test';

const previewPort = process.env.PLAYWRIGHT_PORT ?? '4173';
const localBaseURL = `http://127.0.0.1:${previewPort}`;
const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? localBaseURL;
const webServer = process.env.PLAYWRIGHT_BASE_URL
  ? undefined
  : {
      command: `pnpm build && pnpm exec vite preview --host 127.0.0.1 --port ${previewPort} --strictPort`,
      url: localBaseURL,
      reuseExistingServer: false,
      timeout: 120_000,
    };

export default defineConfig({
  testDir: './e2e',
  timeout: 30_000,
  expect: {
    timeout: 10_000,
  },
  reporter: 'list',
  use: {
    baseURL,
    trace: 'retain-on-failure',
  },
  webServer,
  projects: [
    {
      name: 'desktop-chromium',
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1440, height: 1024 },
      },
    },
    {
      name: 'mobile-chromium',
      use: {
        ...devices['Pixel 5'],
      },
    },
  ],
});
