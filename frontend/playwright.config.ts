import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/browser",
  fullyParallel: false,
  workers: 1,
  timeout: 30000,
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:5174",
    headless: true,
    launchOptions: {
      executablePath:
        process.env.PLAYWRIGHT_CHROME_PATH || "/usr/bin/google-chrome",
      args: ["--no-sandbox"],
    },
    viewport: { width: 1440, height: 1024 },
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  webServer: [
    {
      command: "node tests/mock-api.mjs",
      url: "http://127.0.0.1:18991/health/ready",
      reuseExistingServer: false,
    },
    {
      command: "npm run dev -- --port 5174",
      url: "http://127.0.0.1:5174",
      reuseExistingServer: false,
      env: {
        PAYMENT_SERVICE_URL: "http://127.0.0.1:18991",
        APP_ENV: "test",
        PAYMENT_INTERNAL_TOKEN:
          "frontend-test-internal-token-at-least-32-characters",
        ADMIN_API_TOKEN: "frontend-test-admin-token-at-least-32-characters",
      },
    },
  ],
});
