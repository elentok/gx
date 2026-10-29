import { defineConfig } from "@playwright/test";

// A flaky flow is a hard fail, so retries stay off. Every flow starts its own
// fixture server, which is why flows can run side by side.
export default defineConfig({
  testDir: ".",
  testMatch: "*.e2e.ts",
  globalSetup: "./global-setup.ts",
  fullyParallel: true,
  retries: 0,
  workers: 3,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [["list"]],
  use: {
    ignoreHTTPSErrors: true, trace: "retain-on-failure" },
  projects: [{ name: "chromium", use: { browserName: "chromium" } }],
});
