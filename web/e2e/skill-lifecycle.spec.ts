import { execFileSync } from 'node:child_process';
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

test('skill lifecycle journey: create, edit routing, activate, deprecate, and archive', async ({
  page,
}) => {
  test.setTimeout(120_000);


  // 1. Open startup URL to set session token
  await page.goto(server.url);

  // 2. Navigate to Create Skill
  await page.goto(`${server.origin}/skills/create`);
  await expect(page.locator('h1')).toHaveText('Create skill');

  // Fill Identity
  await page.locator('#create-id').fill('e2e-skill');
  await page.locator('#create-name').fill('E2E Skill');
  await page.locator('#create-desc').fill('E2E test description.');

  // Click Preview draft
  await page.getByRole('button', { name: 'Preview draft' }).click();

  // Modal opens: click Confirm create
  const createModal = page.locator('.fg-modal');
  await expect(createModal).toBeVisible({ timeout: 15000 });
  await expect(createModal.locator('#pp-t')).toBeVisible();

  await createModal.getByRole('button', { name: /create skill/i }).click();

  // Redirects to /skills/e2e-skill
  await page.waitForURL(new RegExp('/skills/e2e-skill'), { timeout: 15000 });

  // 3. Review tab shows not ready
  await expect(page.locator('.t-title')).toHaveText('E2E Skill');
  await expect(page.getByText('Missing activation requirements')).toBeVisible();

  const activateBtn = page.getByRole('button', { name: 'Activate skill' });
  await expect(activateBtn).toBeDisabled();

  // 4. Switch to Editor tab and satisfy activation requirements
  await page.getByRole('tab', { name: 'Editor' }).click();
  await page.waitForTimeout(300);
  // Replace untouched scaffold in content textarea
  const contentTextarea = page.locator('textarea[aria-label="SKILL.md content"]');
  await contentTextarea.fill(
    '---\nname: e2e-skill\ndescription: E2E test description.\n---\n\n# E2E Skill\n\nReal procedural instructions to replace untouched scaffold.\n',
  );
  // Fill Triggers and Min scope
  await page.locator('#edit-trigs').fill('run e2e');
  await page.locator('#edit-notfor').fill('none');
  await page.locator('#edit-scope').selectOption('multi_step');

  // Click Preview changes
  await page.getByRole('button', { name: 'Preview changes' }).click();

  const updateModal = page.locator('.fg-modal');
  await expect(updateModal).toBeVisible({ timeout: 15000 });
  await updateModal.getByRole('button', { name: /save changes/i }).click();
  await expect(updateModal).toBeHidden({ timeout: 10000 });

  // 5. Switch back to Review tab
  await page.getByRole('tab', { name: 'Review' }).click();
  await page.waitForTimeout(500);

  // Activation readiness should now be ready
  await expect(page.getByText('Missing activation requirements')).toBeHidden();
  await expect(activateBtn).toBeEnabled();

  // 6. Activate skill
  await activateBtn.click();
  const activateModal = page.locator('.fg-modal');
  await expect(activateModal).toBeVisible({ timeout: 15000 });
  await activateModal.getByRole('button', { name: /^activate skill$/i }).click();
  await expect(activateModal).toBeHidden({ timeout: 10000 });

  // Badge should now be Active
  await expect(page.locator('.fg-chip', { hasText: 'Active' })).toBeVisible();

  // 7. Deprecate skill
  const deprecateBtn = page.getByRole('button', { name: 'Deprecate' });
  await expect(deprecateBtn).toBeVisible();
  await deprecateBtn.click();

  const deprecateModal = page.locator('.fg-modal');
  await expect(deprecateModal).toBeVisible({ timeout: 15000 });
  await deprecateModal.getByRole('button', { name: /^deprecate skill$/i }).click();
  await expect(deprecateModal).toBeHidden({ timeout: 10000 });

  // Badge should now be Deprecated
  await expect(page.locator('.fg-chip', { hasText: 'Deprecated' })).toBeVisible();

  // 8. Archive skill
  const archiveBtn = page.getByRole('button', { name: 'Archive' });
  await expect(archiveBtn).toBeVisible();
  await archiveBtn.click();

  // ConfirmDialog opens
  const confirmDialog = page.locator('[role="alertdialog"]');
  await expect(confirmDialog).toBeVisible();
  await confirmDialog.getByRole('button', { name: 'Archive skill' }).click();

  // Transition preview modal opens
  const archiveModal = page.locator('.fg-modal');
  await expect(archiveModal).toBeVisible({ timeout: 15000 });
  await archiveModal.getByRole('button', { name: /^archive skill$/i }).click();
  await expect(archiveModal).toBeHidden({ timeout: 10000 });

  // Badge should now be Archived and no lifecycle actions remain
  await expect(page.locator('.fg-chip', { hasText: 'Archived' })).toBeVisible();
  await expect(page.getByText('Read-only — no transitions from archived.')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Activate skill' })).toBeHidden();
  await expect(page.getByRole('button', { name: 'Deprecate' })).toBeHidden();
  await expect(page.getByRole('button', { name: 'Archive' })).toBeHidden();
});

test('conflict journey and reload persistence', async ({ page }) => {
  test.setTimeout(120_000);

  const binaryPath = path.resolve(__dirname, '../.e2e/skillhub');

  // Open startup URL to set session token
  await page.goto(server.url);
  await page.goto(`${server.origin}/skills/smoke-skill?tab=editor`);
  await expect(page.locator('.t-title')).toHaveText('Smoke Skill');

  // 1. Edit content in the UI
  const textarea = page.locator('textarea[aria-label="SKILL.md content"]');
  await textarea.fill('# Smoke Skill\n\nEdited in UI.');

  // 2. Modify skill outside the UI via CLI
  execFileSync(
    binaryPath,
    ['skill', 'edit', 'smoke-skill', '--description', 'Changed outside', '--yes'],
    {
      env: {
        ...process.env,
        SKILLHUB_WORKSPACE: server.ws,
      },
      stdio: 'pipe',
    },
  );

  // 3. Click Preview changes in browser -> triggers edit_conflict -> Conflict Drawer
  await page.getByRole('button', { name: 'Preview changes' }).click();

  const drawer = page.locator('.fg-drawer');
  await expect(drawer).toBeVisible({ timeout: 10000 });
  await expect(drawer.getByText('SKILL.md changed since you opened it')).toBeVisible();

  // Assert no overwrite action exists
  await expect(drawer.getByRole('button', { name: /overwrite/i })).toBeHidden();

  // 4. Click "Use latest as base" -> conflict clears and preview succeeds
  await drawer.getByRole('button', { name: 'Keep my edits on the latest version' }).click();
  await expect(drawer).toBeHidden({ timeout: 10000 });

  const modal = page.locator('.fg-modal');
  await expect(modal).toBeVisible({ timeout: 10000 });

  // 5. Reload while proposal preview is open -> modal gone, editor still shows draft text
  await page.reload();
  await expect(page.locator('.fg-modal')).toBeHidden();
  await expect(page.locator('textarea[aria-label="SKILL.md content"]')).toHaveValue(/Edited in UI/);
});

test('add skill validation rejects local paths without network request', async ({ page }) => {
  let requestedAddPreview = false;
  page.on('request', (req) => {
    if (req.url().includes('/api/v1/skills/add/preview')) {
      requestedAddPreview = true;
    }
  });

  await page.goto(server.url);
  await page.goto(`${server.origin}/skills/add`);
  await expect(page.locator('h1')).toHaveText('Add from GitHub');

  // Fill local path
  const urlInput = page.locator('#gh-url');
  await urlInput.fill('/etc/passwd');

  // Click Discover skills
  await page.getByRole('button', { name: 'Discover skills' }).click();

  // Assert inline error is visible
  await expect(page.locator('.fg-banner--danger')).toBeVisible();
  await expect(page.locator('.fg-banner--danger')).toHaveText(/The web UI accepts only public GitHub URLs/);

  // Assert no request was sent to /api/v1/skills/add/preview
  expect(requestedAddPreview).toBe(false);
});
