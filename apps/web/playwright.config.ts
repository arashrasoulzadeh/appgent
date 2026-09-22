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
  // Chromium, not WebKit — WebKit's ITP cookie handling was originally
  // suspected as the cause of every post-login test failing, but
  // switching engines alone did NOT fix it (still failed identically on
  // Chromium), which disproves that theory. Chromium remains the
  // standard, reliable choice for headless CI E2E regardless.
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
      },
    },
  ],
  // `next start` (a real production build), not `next dev`. The actual
  // root cause of every post-login test failing: zero requests ever
  // reached the API during the whole E2E run (confirmed from the API's
  // own request logs) — not even the login POST — meaning form
  // interactions weren't reaching React's event handlers at all. `next
  // dev`'s Turbopack JIT-compiles each route on first request, so the
  // server can respond with HTML before the client JS bundle has
  // finished compiling/hydrating; the auth spec's beforeEach only waited
  // for the email input to be "attached" to the DOM, not for hydration
  // to finish, so Playwright could fill/click the form before React's
  // onSubmit was actually wired up. `next start` serves an already-built,
  // already-hydrated app — no compile-on-first-request race — and is
  // what's actually deployed in production anyway (see docker-compose.yml
  // web service / apps/web/Dockerfile), so this is more representative
  // too, not just a workaround.
  webServer: {
    command: 'pnpm build && pnpm start',
    cwd: path.join(__dirname),
    url: 'http://localhost:3000',
    reuseExistingServer: !process.env.CI,
    // Bumped from 180s: a real `next build` takes meaningfully longer
    // than `next dev`'s near-instant startup, and this timeout now has
    // to cover both the build and the server becoming ready.
    timeout: 300000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
})