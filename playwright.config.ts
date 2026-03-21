// @ts-check
import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: "html",

  use: {
    baseURL: "http://localhost:3333",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
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
    command:
      "REPO_ROOT=. E2E_ADDR=:3333 go run ./cmd/e2e-server",
    url: "http://localhost:3333",
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  },
});
