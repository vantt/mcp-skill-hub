import { execFileSync } from 'node:child_process';
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { seedDistillWorkspace } from './support/seed-workspace';
import { startServer, type RunningServer } from './support/server';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const binaryPath = path.resolve(__dirname, '../.e2e/skillhub');

function assertLoopbackOnly(page: Page, server: RunningServer) {
  page.on('request', (req) => {
    const url = req.url();
    expect(
      url.startsWith(server.origin) || url.startsWith('data:'),
      `Non-loopback or cross-origin request intercepted: ${url}`,
    ).toBe(true);
  });
}

test.describe('Shipped User Flow Journeys (Spec 04 §3)', () => {
  let server: RunningServer;

  test.afterEach(async () => {
    if (server) {
      await server.stop();
    }
  });

  test('flow 3.1: review and activate a draft, including runtime tab and CLI approve command for third-party content', async ({
    page,
  }) => {
    test.setTimeout(120_000);
    server = await startServer();
    assertLoopbackOnly(page, server);

    const ws = server.ws;
    const metaPath = path.join(ws, 'skills', 'core', 'smoke-skill', '.meta', 'skill.yaml');
    const metaContent = fs.readFileSync(metaPath, 'utf8');

    // 1. Mark smoke-skill third-party by adding origin to provenance
    const updatedMeta = `${metaContent.trimEnd()}\nprovenance:\n    origin:\n        kind: github\n        repository: https://github.com/example/skills\n        commit: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n`;
    fs.writeFileSync(metaPath, updatedMeta);

    // 2. Run skillhub rebuild
    execFileSync(binaryPath, ['rebuild', '--workspace', ws], { stdio: 'pipe' });

    // 3. Store SECRET_API_KEY with secret value piped on stdin
    const secretValue = 'SECRET-API-KEY-VALUE-DO-NOT-LEAK-1234';
    execFileSync(binaryPath, ['skill', 'env', 'set', 'smoke-skill', 'SECRET_API_KEY'], {
      input: secretValue,
      env: { ...process.env, SKILLHUB_WORKSPACE: ws },
      stdio: ['pipe', 'pipe', 'pipe'],
    });

    // 4. Authenticate and navigate to smoke-skill
    await page.goto(server.url);
    await page.goto(`${server.origin}/skills/smoke-skill`);

    // Review tab shows Review required and approve command
    await page.getByRole('tab', { name: 'Review' }).click();
    await expect(page.getByText('Review required')).toBeVisible();
    await expect(
      page.getByText(/skillhub skill edit smoke-skill --approve-content sha256:/),
    ).toBeVisible();

    // Security check: no approve button in WebUI
    const approveButtons = page.getByRole('button', { name: /approve/i });
    await expect(approveButtons).toHaveCount(0);

    // Runtime tab shows env key without value
    await page.getByRole('tab', { name: 'Runtime' }).click();
    await expect(page.getByText('SECRET_API_KEY')).toBeVisible();
    const pageText = await page.content();
    expect(pageText).not.toContain(secretValue);

    // 5. Approve content outside via CLI and activate
    const contentPath = path.join(ws, 'skills', 'core', 'smoke-skill', 'SKILL.md');
    const contentBytes = fs.readFileSync(contentPath);
    const digest = 'sha256:' + crypto.createHash('sha256').update(contentBytes).digest('hex');

    execFileSync(
      binaryPath,
      [
        'skill',
        'edit',
        'smoke-skill',
        '--trigger',
        'smoke test',
        '--not-for',
        'none',
        '--min-scope',
        'single_step',
        '--approve-content',
        digest,
        '--yes',
      ],
      {
        env: { ...process.env, SKILLHUB_WORKSPACE: ws },
        stdio: 'pipe',
      },
    );

    await page.reload();
    await page.getByRole('tab', { name: 'Review' }).click();
    const activateBtn = page.getByRole('button', { name: 'Activate skill' });
    await expect(activateBtn).toBeEnabled();
    await expect(activateBtn).toBeVisible();
    await activateBtn.click();

    // Confirm activation
    const modal = page.locator('.fg-modal');
    await expect(modal).toBeVisible();
    await modal.getByRole('button', { name: /confirm|activate/i }).click();

    await expect(page.getByText('Active', { exact: true }).first()).toBeVisible();
  });

  test('flow 3.2: add skill validation rejects invalid URL and local paths', async ({ page }) => {
    test.setTimeout(120_000);
    server = await startServer();
    assertLoopbackOnly(page, server);

    await page.goto(server.url);
    await page.goto(`${server.origin}/skills/add`);
    await expect(page.getByRole('heading', { name: 'Add from GitHub' }).first()).toBeVisible();
    const urlInput = page.locator('#gh-url');

    // 1. Reject local paths
    await urlInput.fill('/etc/passwd');
    await page.getByRole('button', { name: 'Discover skills' }).click();
    await expect(page.locator('.fg-banner--danger')).toBeVisible();
    await expect(page.locator('.fg-banner--danger')).toHaveText(/The web UI accepts only public GitHub URLs/);

    // 2. Reject non-GitHub URL
    await urlInput.fill('https://gitlab.com/owner/repo');
    await page.getByRole('button', { name: 'Discover skills' }).click();
    await expect(page.locator('.fg-banner--danger')).toBeVisible();
    await expect(page.locator('.fg-banner--danger')).toHaveText(/The web UI accepts only public GitHub URLs/);
  });

  test('flow 3.3: create a skill draft', async ({ page }) => {
    test.setTimeout(120_000);
    server = await startServer();
    assertLoopbackOnly(page, server);

    await page.goto(server.url);
    await page.goto(`${server.origin}/skills/create`);
    await expect(page.getByRole('heading', { name: 'Create skill' }).first()).toBeVisible();

    await page.locator('#create-id').fill('journey-create-skill');
    await page.locator('#create-name').fill('Journey Create Skill');
    await page.locator('#create-desc').fill('Created in journey test flow 3.3.');

    await page.getByRole('button', { name: 'Preview draft' }).click();

    const createModal = page.locator('.fg-modal');
    await expect(createModal).toBeVisible();
    await createModal.getByRole('button', { name: /create skill/i }).click();

    await page.waitForURL(new RegExp('/skills/journey-create-skill'));
    await expect(page.getByRole('heading', { name: 'journey-create-skill' }).first()).toBeVisible();
    await expect(page.getByText('Draft', { exact: true }).first()).toBeVisible();
  });

  test('flow 3.4: edit skill with conflict detection and resolution', async ({ page }) => {
    test.setTimeout(120_000);
    server = await startServer();
    assertLoopbackOnly(page, server);

    await page.goto(server.url);
    await page.goto(`${server.origin}/skills/smoke-skill`);

    await page.getByRole('tab', { name: 'Editor' }).click();
    const descInput = page.locator('#edit-desc');
    await descInput.fill('Draft description before conflict.');

    // Edit outside via CLI
    execFileSync(
      binaryPath,
      ['skill', 'edit', 'smoke-skill', '--description', 'Concurrent edit outside', '--yes'],
      {
        env: { ...process.env, SKILLHUB_WORKSPACE: server.ws },
        stdio: 'pipe',
      },
    );

    // Click Preview changes -> ConflictDrawer opens
    await page.getByRole('button', { name: 'Preview changes' }).click();
    await expect(page.getByText('SKILL.md changed since you opened it')).toBeVisible();

    // Use latest as base
    await page.getByRole('button', { name: 'Use latest as base' }).click();
    await expect(page.getByText('SKILL.md changed since you opened it')).not.toBeVisible();
  });

  test('flow 3.5: sources - check now, select a source, and open the distill handoff', async ({
    page,
  }) => {
    test.setTimeout(120_000);
    const seed = seedDistillWorkspace();
    server = await startServer({ workspace: seed.ws });
    assertLoopbackOnly(page, server);

    try {
      await page.goto(server.url);
      await page.goto(`${server.origin}/sources`);

      // 1. Check now on source-c
      const checkNowBtn = page.getByRole('button', { name: 'Check now' }).first();
      await expect(checkNowBtn).toBeVisible();
      await checkNowBtn.click();
      await expect(checkNowBtn).toBeVisible();

      // 2. Select source-c checkbox and click Distill with Curator Agent
      const sourceCCheckbox = page.getByLabel('Select source-c for distillation');
      await sourceCCheckbox.check();

      const distillBtn = page.getByRole('link', { name: /Distill with Curator Agent/i });
      await expect(distillBtn).toBeVisible();
      await distillBtn.click();

      // 3. On Distill screen: verify brief
      await expect(page).toHaveURL(/sources\/distill/);
      const brief = await page.locator('pre').textContent();
      expect(brief).toContain('curation_run_start');
      expect(brief).toContain('source-c');
      expect(brief).toContain('idempotency_key:');
    } finally {
      if (seed?.ws) {
        try {
          fs.rmSync(seed.ws, { recursive: true, force: true });
        } catch {
          // Ignore
        }
      }
    }
  });

  test('flow 3.7: deprecate and archive a skill', async ({ page }) => {
    test.setTimeout(120_000);
    server = await startServer();
    assertLoopbackOnly(page, server);

    // 1. Satisfy routing requirements and activate via CLI
    execFileSync(
      binaryPath,
      [
        'skill',
        'edit',
        'smoke-skill',
        '--trigger',
        'smoke test',
        '--not-for',
        'none',
        '--min-scope',
        'single_step',
        '--yes',
      ],
      {
        env: { ...process.env, SKILLHUB_WORKSPACE: server.ws },
        stdio: 'pipe',
      },
    );
    execFileSync(
      binaryPath,
      ['skill', 'activate', 'smoke-skill', '--yes'],
      {
        env: { ...process.env, SKILLHUB_WORKSPACE: server.ws },
        stdio: 'pipe',
      },
    );

    await page.goto(server.url);
    await page.goto(`${server.origin}/skills/smoke-skill`);
    await expect(page.getByText('Active', { exact: true }).first()).toBeVisible();

    // 2. Deprecate skill
    const deprecateBtn = page.getByRole('button', { name: 'Deprecate' });
    await expect(deprecateBtn).toBeVisible();
    await deprecateBtn.click();

    const deprecateModal = page.locator('.fg-modal');
    await expect(deprecateModal).toBeVisible();
    const reasonTextarea = deprecateModal.locator('textarea');
    if (await reasonTextarea.isVisible()) {
      await reasonTextarea.fill('Deprecating in flow 3.7.');
    }
    await deprecateModal.getByRole('button', { name: /confirm/i }).click();
    await expect(page.getByText('Deprecated', { exact: true }).first()).toBeVisible();

    // 3. Archive skill
    const archiveBtn = page.getByRole('button', { name: 'Archive' });
    await expect(archiveBtn).toBeVisible();
    await archiveBtn.click();

    // ConfirmDialog opens
    const confirmDialog = page.locator('[role="alertdialog"]');
    await expect(confirmDialog).toBeVisible();
    await confirmDialog.getByRole('button', { name: 'Archive skill' }).click();

    // Transition preview modal opens
    const archiveModal = page.locator('.fg-modal');
    await expect(archiveModal).toBeVisible();
    await archiveModal.getByRole('button', { name: /confirm/i }).click();

    await expect(page.getByText('Archived', { exact: true }).first()).toBeVisible();
  });
});
