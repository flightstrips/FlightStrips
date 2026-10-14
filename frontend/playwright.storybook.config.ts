import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests/storybook',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  workers: 2,
  retries: process.env.CI ? 1 : 0,
  use: {
    ...devices['Desktop Chrome'],
    channel: 'chromium',
    baseURL: 'http://127.0.0.1:6010',
    viewport: { width: 1920, height: 1080 },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  webServer: {
    command: 'npx vite preview --config .storybook/vite.config.ts --outDir storybook-static --host 127.0.0.1 --port 6010 --strictPort',
    url: 'http://127.0.0.1:6010',
    reuseExistingServer: !process.env.CI,
  },
});
