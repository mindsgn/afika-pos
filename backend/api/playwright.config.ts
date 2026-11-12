import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:4173', browserName: 'chromium' },
  webServer: {
    command: 'go run ./e2e/server.go',
    url: 'http://127.0.0.1:4173/v1/health',
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});
