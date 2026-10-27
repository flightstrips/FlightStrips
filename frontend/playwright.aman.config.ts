import {defineConfig, devices} from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  timeout: 120_000,
  expect: {timeout: 15_000},
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: !!process.env.CI,
  outputDir: "aman-e2e-artifacts/results",
  reporter: [["list"], ["html", {outputFolder: "aman-e2e-artifacts/report", open: "never"}]],
  use: {baseURL: "http://127.0.0.1:4181", screenshot: "only-on-failure", trace: "retain-on-failure", video: "retain-on-failure"},
  projects: [{name: "aman-chromium", use: {...devices["Desktop Chrome"], viewport: {width: 1920, height: 1080}}}],
  webServer: [
    {command: "node tests/e2e/backend.mjs", url: "http://127.0.0.1:4180/e2e/ready", timeout: 180_000, reuseExistingServer: false},
    {command: "node tests/e2e/frontend.mjs", url: "http://127.0.0.1:4181", timeout: 180_000, reuseExistingServer: false},
  ],
});
