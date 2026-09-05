import { defineConfig, devices } from "@playwright/test";

// Runs against the REAL image started by scripts/e2e-stack.sh (port 3300).
// No webServer block: the stack is a container, started and stopped around
// the run by the script (locally) or by ci.yml.
export default defineConfig({
  testDir: ".",
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never", outputFolder: "playwright-report" }]] : "list",
  outputDir: "test-results",
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://127.0.0.1:3300",
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
