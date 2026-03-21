// @ts-check
import { defineConfig, devices } from "@playwright/test";
import path from "path";

/**
 * Playwright config for E2E tests against a live asset-service instance
 * backed by real PostgreSQL + Pocket-ID Docker containers.
 *
 *   webServer   – runs e2e/start-server.sh (docker + provision + app server)
 *   globalSetup – seeds data, performs admin OIDC login, saves auth state
 */
export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: "html",

  globalSetup: path.resolve(__dirname, "e2e/global-setup.ts"),

  use: {
    baseURL: "http://localhost:8080",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    ignoreHTTPSErrors: true, // Pocket-ID uses self-signed certs
  },

  projects: [
    {
      name: "mobile-portrait",
      use: {
        browserName: "chromium",
        viewport: { width: 390, height: 844 },
        isMobile: true,
        hasTouch: true,
        userAgent:
          "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148",
      },
    },
    {
      name: "desktop",
      use: {
        ...devices["Desktop Chrome"],
      },
    },
  ],

  webServer: {
    command: "bash e2e/start-server.sh",
    url: "http://localhost:8080",
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
