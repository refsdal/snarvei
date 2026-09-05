import { defineConfig, devices } from "@playwright/test";

// Runs against the REAL image started by scripts/e2e-stack.sh (port 3300).
// No webServer block: the stack is a container, started and stopped around
// the run by the script (locally) or by ci.yml.
export default defineConfig({
  testDir: ".",
  fullyParallel: false,
  // signUpUi's throttle retry (support.ts) can legitimately need several
  // 2.5s-spaced attempts once five spec files' sign-ups are contending for
  // the same per-IP credential-signup throttle; give it enough room to work
  // rather than the 30s default.
  timeout: 60_000,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never", outputFolder: "playwright-report" }]] : "list",
  outputDir: "test-results",
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://127.0.0.1:3300",
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
