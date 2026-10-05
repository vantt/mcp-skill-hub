import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, test } from '@playwright/test';
import type { RunningServer } from './support/server';
import { startServer } from './support/server';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

let server: RunningServer;

test.beforeAll(async () => {
  server = await startServer();
});

test.afterAll(async () => {
  if (server) {
    await server.stop();
  }
});

test('runtime parity and content trust flow', async ({ page }) => {
  test.setTimeout(120_000);

  const binaryPath = path.resolve(__dirname, '../.e2e/skillhub');
  const ws = server.ws;
  const metaPath = path.join(ws, 'skills/core/smoke-skill/skill.meta.yaml');

  // 1. Mark smoke-skill third-party by adding provenance.origin
  let metaContent = fs.readFileSync(metaPath, 'utf-8');
  metaContent = metaContent.replace(
    /provenance:\s*\n\s*created_by:\s*skillhub/,
    `provenance:
    created_by: skillhub
    origin:
        kind: github
        repository: https://github.com/example/smoke-skills
        commit: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`,
  );
  fs.writeFileSync(metaPath, metaContent, 'utf-8');

  // 2. Run skillhub rebuild
  execFileSync(binaryPath, ['rebuild', '--json'], {
    env: { ...process.env, SKILLHUB_WORKSPACE: ws },
    stdio: 'pipe',
  });

  // 3. Store E2E_TOKEN with secret value piped on stdin
  const secretEnvValue = 'SECRET-E2E-VALUE-DO-NOT-LEAK';
  execFileSync(
    binaryPath,
    ['skill', 'env', 'set', 'smoke-skill', 'E2E_TOKEN'],
    {
      input: secretEnvValue,
      env: { ...process.env, SKILLHUB_WORKSPACE: ws },
      stdio: ['pipe', 'pipe', 'pipe'],
    },
  );

  // 4. Open startup URL to set session token
  await page.goto(server.url);

  // 5. Navigate to /skills/smoke-skill
  await page.goto(`${server.origin}/skills/smoke-skill`);

  // Review tab shows "Review required" and approve command
  await expect(page.getByText('Review required')).toBeVisible();
  await expect(
    page.getByText(/skillhub skill edit smoke-skill --approve-content sha256:/),
  ).toBeVisible();

  // No button named /approve/i
  const approveButtons = await page.getByRole('button', { name: /approve/i }).all();
  expect(approveButtons.length).toBe(0);

  // 6. Click Runtime tab
  await page.getByRole('tab', { name: 'Runtime' }).click();

  // Runtime tab shows E2E_TOKEN
  await expect(page.getByText('E2E_TOKEN')).toBeVisible();

  // Verify the secret value was never leaked in DOM
  const pageContent = await page.content();
  expect(pageContent).not.toContain(secretEnvValue);
});
