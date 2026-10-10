import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  retries: 0,
  workers: 1,
  reporter: 'list',
  use: {
    ...devices['Desktop Chrome'],
  },
  projects: [
    {
      name: 'chromium',
      testIgnore: /e2e[\\/]ux[\\/]/,
      use: { ...devices['Desktop Chrome'] },
    },
    // Evaluation capture for the UX review, enabled only by `make web-ux` (UX_CAPTURE=1):
    // it needs a hub clone and writes screenshots, so the default E2E run skips it.
    ...(process.env.UX_CAPTURE
      ? [
          {
            name: 'ux',
            testMatch: /e2e[\\/]ux[\\/].*\.spec\.ts$/,
            use: { ...devices['Desktop Chrome'] },
          },
        ]
      : []),
  ],
});
