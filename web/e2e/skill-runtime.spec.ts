import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, test } from '@playwright/test';
import type { RunningServer } from './support/server';
import { startServer } from './support/server';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const binaryPath = path.resolve(__dirname, '../.e2e/skillhub');

let server: RunningServer;

test.beforeAll(async () => {
  server = await startServer();
});

test.afterAll(async () => {
  if (server) {
    await server.stop();
  }
});

test('skill-runtime: review shows approve command, no approve button, and runtime tab shows env key without value', async ({
  page,
}) => {
  test.setTimeout(120_000);

  const ws = server.ws;
  const metaPath = path.join(ws, 'skills', 'core', 'smoke-skill', 'skill.meta.yaml');
  const metaContent = fs.readFileSync(metaPath, 'utf8');
  // 1. Mark smoke-skill third-party by adding origin to provenance
  const updatedMeta = metaContent.replace(
    '    created_by: skillhub',
    `    created_by: skillhub
    origin:
        kind: github
        repository: https://github.com/example/skills
        commit: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`,
  );
  fs.writeFileSync(metaPath, updatedMeta);
  // 2. Run skillhub rebuild
  execFileSync(binaryPath, ['rebuild', '--workspace', ws], { stdio: 'pipe' });

  // 3. Store E2E_TOKEN with secret value piped on stdin
  const secretValue = 'SECRET-PIPED-VALUE-DO-NOT-LEAK-9988';
  execFileSync(binaryPath, ['skill', 'env', 'set', 'smoke-skill', 'E2E_TOKEN'], {
    input: secretValue,
    env: {
      ...process.env,
      SKILLHUB_WORKSPACE: ws,
    },
    stdio: ['pipe', 'pipe', 'pipe'],
  });

  // 4. Open startup URL to set session token, then navigate to smoke-skill
  await page.goto(server.url);
  await page.goto(`${server.origin}/skills/smoke-skill`);

  // Review tab
  await page.getByRole('tab', { name: 'Review' }).click();
  await expect(page.getByText('Review required')).toBeVisible();
  await expect(
    page.getByText(/skillhub skill edit smoke-skill --approve-content sha256:/),
  ).toBeVisible();

  // Assert security requirement: no approve button
  const approveButtons = page.getByRole('button', { name: /approve/i });
  await expect(approveButtons).toHaveCount(0);

  // 5. Open Runtime tab
  await page.getByRole('tab', { name: 'Runtime' }).click();
  await expect(page.getByText('E2E_TOKEN')).toBeVisible();
  // Assert secret value is never in DOM
  const pageContent = await page.content();
  expect(pageContent).not.toContain(secretValue);
});
