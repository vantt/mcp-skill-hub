import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, test } from '@playwright/test';
import { seedDistillWorkspace, type SeededDistillWorkspace } from './support/seed-workspace';
import { startServer, type RunningServer } from './support/server';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const binaryPath = path.resolve(__dirname, '../.e2e/skillhub');

test.describe('Sources and Runs Distill Journey', () => {
  let seed: SeededDistillWorkspace;
  let server: RunningServer;

  test('seed workspace verification @seed', () => {
    seed = seedDistillWorkspace();

    const getRunA = execFileSync(
      binaryPath,
      ['distill', 'get', seed.finalizedRunId, '--json'],
      {
        encoding: 'utf-8',
        env: { ...process.env, SKILLHUB_WORKSPACE: seed.ws },
      },
    );
    const parsedA = JSON.parse(getRunA);
    expect(parsedA.run.state).toBe('finalized');

    const getRunB = execFileSync(
      binaryPath,
      ['distill', 'get', seed.inProgressRunId, '--json'],
      {
        encoding: 'utf-8',
        env: { ...process.env, SKILLHUB_WORKSPACE: seed.ws },
      },
    );
    const parsedB = JSON.parse(getRunB);
    expect(parsedB.run.state).toBe('in_progress');
  });

  test('sources to distill to run return journey', async ({ page }) => {
    test.setTimeout(120_000);
    if (!seed) {
      seed = seedDistillWorkspace();
    }
    server = await startServer({ workspace: seed.ws });

    try {
      // 1. Authenticate with server startup URL
      await page.goto(server.url);
      await expect(page.getByRole('heading', { name: 'Home' }).first()).toBeVisible();

      // 2. Open /sources?filter=ready
      await page.goto(`${server.origin}/sources?filter=ready`);
      await expect(page.getByRole('heading', { name: 'Sources' }).first()).toBeVisible();

      // Assert source-c is listed with a checkbox
      const sourceCRow = page.locator('div', { hasText: 'source-c' }).first();
      await expect(sourceCRow).toBeVisible();
      const sourceCCheckbox = page.getByLabel('Select source-c for distillation');
      await expect(sourceCCheckbox).toBeVisible();
      await expect(sourceCCheckbox).not.toBeChecked();

      // Assert source-a is NOT listed because it was already distilled
      await expect(page.getByText('source-a')).not.toBeVisible();

      // 3. Select source-c and click "Distill with Curator Agent"
      await sourceCCheckbox.check();
      await expect(sourceCCheckbox).toBeChecked();

      const distillBtn = page.getByRole('link', { name: /Distill with Curator Agent/i });
      await expect(distillBtn).toBeVisible();
      await distillBtn.click();

      // 4. On /sources/distill: assert brief contains curation_run_start and key
      await expect(page).toHaveURL(/sources\/distill/);
      await expect(page.getByRole('heading', { name: 'Distill with Curator Agent' }).first()).toBeVisible();

      const briefPre = page.locator('pre');
      await expect(briefPre).toBeVisible();
      const briefContent = await briefPre.textContent();
      expect(briefContent).toContain('curation_run_start');
      expect(briefContent).toContain('source-c');
      expect(briefContent).toContain('idempotency_key:');

      // 5. Paste finalizedRunId and click Open runs
      const pasteTextarea = page.getByLabel('Paste run IDs returned by agent');
      await pasteTextarea.fill(seed.finalizedRunId);

      const openRunsBtn = page.getByRole('button', { name: 'Open runs' });
      await openRunsBtn.click();

      // Click the run link
      const runLink = page.getByRole('link', { name: seed.finalizedRunId });
      await expect(runLink).toBeVisible();
      await runLink.click();

      // 6. On Run page: shows finalized panel with Open Inbox CTA
      await expect(page).toHaveURL(new RegExp(`/sources/runs/${seed.finalizedRunId}`));
      await expect(page.locator('.fg-card__title', { hasText: 'Finalized' })).toBeVisible();
      const openInboxLink = page.getByRole('link', { name: 'Open Inbox →' });
      await expect(openInboxLink).toBeVisible();
      await expect(openInboxLink).toHaveAttribute('href', '/inbox');

      // 7. Back on /sources: recent-runs panel lists finalizedRunId
      await page.goto(`${server.origin}/sources`);
      const recentPanel = page.locator('section', { hasText: 'Recent runs on this browser' });
      await expect(recentPanel).toBeVisible();
      await expect(recentPanel.getByRole('link', { name: seed.finalizedRunId })).toBeVisible();

      // 8. Open inProgressRunId by URL
      await page.goto(`${server.origin}/sources/runs/${seed.inProgressRunId}`);
      await expect(page.locator('.fg-card__title', { hasText: 'In progress' })).toBeVisible();

      // 9. Cancel run -> dialog consequence -> confirm -> cancelled
      const cancelBtn = page.getByRole('button', { name: 'Cancel run' });
      await expect(cancelBtn).toBeVisible();
      await cancelBtn.click();

      // ConfirmDialog shows consequence text
      const dialog = page.getByRole('alertdialog');
      await expect(dialog).toBeVisible();
      await expect(
        dialog.getByText(
          'Cancelling this run will stop processing. The source cursor does not advance, and an agent working on this run will fail to submit.',
        ),
      ).toBeVisible();

      const confirmCancelBtn = dialog.getByRole('button', { name: 'Cancel run' });
      await confirmCancelBtn.click();

      // Run updates to cancelled
      await expect(page.locator('.fg-card__title', { hasText: 'Cancelled' })).toBeVisible();
      await expect(
        page.getByText('This run was cancelled. The source cursor was not advanced.'),
      ).toBeVisible();
      await expect(page.getByRole('button', { name: 'Cancel run' })).not.toBeVisible();
    } finally {
      if (server) {
        await server.stop();
      }
      if (seed?.ws) {
        try {
          fs.rmSync(seed.ws, { recursive: true, force: true });
        } catch {
          // Ignore
        }
      }
    }
  });
});
