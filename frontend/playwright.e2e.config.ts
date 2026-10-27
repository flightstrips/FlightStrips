import {defineConfig, devices} from "@playwright/test";

const onlyProject = process.argv.find(arg => arg.startsWith("--project="))?.split("=")[1];

export default defineConfig({
  testDir: "./tests/e2e",
  timeout: 120_000,
  expect: {timeout: 15_000},
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: !!process.env.CI,
  outputDir: "e2e-artifacts/results",
  reporter: [["list"], ["html", {outputFolder: "e2e-artifacts/report", open: "never"}]],
  use: {actionTimeout: 15_000, navigationTimeout: 30_000, ...devices["Desktop Chrome"], viewport: {width: 1920, height: 1080}, baseURL: "http://127.0.0.1:4181", screenshot: "only-on-failure", trace: "retain-on-failure", video: "retain-on-failure"},
  projects: [
    {name: "aman", testMatch: "aman.spec.ts", metadata: {backend: "http://127.0.0.1:4180"}},
    {name: "strips", testMatch: "strips.spec.ts", metadata: {backend: "http://127.0.0.1:4182"}},
  ],
  webServer: [
    ...(!onlyProject || onlyProject === "aman" ? [{command: "node tests/e2e/backend.mjs aman 4180", url: "http://127.0.0.1:4180/e2e/ready", timeout: 180_000, reuseExistingServer: false}] : []),
    ...(!onlyProject || onlyProject === "strips" ? [{command: "node tests/e2e/backend.mjs strips 4182", url: "http://127.0.0.1:4182/e2e/ready", timeout: 180_000, reuseExistingServer: false}] : []),
    {command: "node tests/e2e/frontend.mjs", url: "http://127.0.0.1:4181", timeout: 180_000, reuseExistingServer: false},
  ],
});
