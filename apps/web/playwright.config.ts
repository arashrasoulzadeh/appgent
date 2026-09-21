import { defineConfig, devices } from '@playwright/test'
import path from 'path'

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: 'html',
  use: {
    baseURL: 'http://localhost:3000',
    trace: 'on-first-retry',
  },
  // Chromium, not WebKit: WebKit's cookie/ITP handling in headless CI is
  // markedly stricter than Chromium's and has repeatedly dropped the
  // session cookie set by the API (a different port on localhost) between
  // login's POST and the following navigation — every test past the
  // login step failed with the app stuck on /login despite the login
  // request itself succeeding. Chromium is the standard, reliable choice
  // for headless CI E2E and doesn't have this problem.
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
      },
    },
  ],
  webServer: {
    command: 'pnpm dev',
    cwd: path.join(__dirname),
    url: 'http://localhost:3000',
    reuseExistingServer: !process.env.CI,
    timeout: 180000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
})