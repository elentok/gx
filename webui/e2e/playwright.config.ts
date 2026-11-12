import { defineConfig } from "@playwright/test"

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
    ignoreHTTPSErrors: true,
    // Requests a worker answers bypass page.route; only the PWA steps allow workers.
    serviceWorkers: "block",
    // Without a limit a click on a vanished element retries until the test timeout and fails with no call log.
    actionTimeout: 20_000,
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { browserName: "chromium" } }],
})
