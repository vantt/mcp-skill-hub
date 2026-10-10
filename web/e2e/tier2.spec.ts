import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, test } from '@playwright/test';
import { startServer, type RunningServer } from './support/server';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const binaryPath = path.resolve(__dirname, '../.e2e/skillhub');

// Weekly set-up and change flows: Editor, Create, Add from GitHub, upstream updates.
test.describe('Tier 2 weekly flows', () => {
  let server: RunningServer;

  test.beforeEach(async ({ page }) => {
    server = await startServer();
    await page.goto(server.url);
  });

  test.afterEach(async () => {
    await server?.stop();
  });

  const cli = (...args: string[]) =>
    execFileSync(binaryPath, args, {
      env: { ...process.env, SKILLHUB_WORKSPACE: server.ws },
      stdio: 'pipe',
    }).toString();

  test('create form explains its fields and keeps Preview draft on screen', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(`${server.origin}/skills/create`);

    await expect(page.getByText(/A draft is not routed to agents yet/).first()).toBeVisible();
    await expect(page.getByText(/What this skill should not be used for/)).toBeVisible();
    await expect(page.getByText(/How big a job has to be/)).toBeVisible();
    await expect(page.locator('#create-notfor')).toHaveAttribute('placeholder', /typo fixes/);

    // Preview draft is inside the first screen without scrolling.
    const box = await page.getByRole('button', { name: 'Preview draft' }).boundingBox();
    expect(box).not.toBeNull();
    expect(box!.y + box!.height).toBeLessThanOrEqual(800);

    // A missing description is caught on the form, before anything is sent.
    await page.locator('#create-id').fill('tier2-new');
    await page.locator('#create-name').fill('Tier2 New');
    await page.getByRole('button', { name: 'Preview draft' }).click();
    await expect(page.getByText('Description is required.')).toBeVisible();

    // An id that is taken is explained by the server, not left as a generic error.
    await page.locator('#create-id').fill('smoke-skill');
    await page.locator('#create-desc').fill('Taken id.');
    await page.getByRole('button', { name: 'Preview draft' }).click();
    await expect(page.getByRole('alert').filter({ hasText: /already|exists|used/i })).toBeVisible();

    await page.locator('#create-id').fill('tier2-new');
    await page.getByRole('button', { name: 'Preview draft' }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByText('Create draft skill tier2-new?')).toBeVisible();
    await expect(dialog.getByText(/A draft is not routed to agents yet/)).toBeVisible();
    await expect(dialog.getByText('Exact file changes')).toBeVisible();
  });

  test('editor preview says what changes in words, then the saved result shows', async ({ page }) => {
    await page.goto(`${server.origin}/skills/smoke-skill?tab=editor`);
    const save = page.getByRole('button', { name: 'Preview changes' });
    await expect(save).toBeDisabled();

    await page.locator('#edit-desc').fill('Reworded by the Tier 2 test.');
    await save.click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByText('Save changes to smoke-skill?')).toBeVisible();
    await expect(dialog.getByText(/Description: "Smoke test skill\." becomes "Reworded by the Tier 2 test\."/)).toBeVisible();
    await dialog.getByRole('button', { name: 'Save changes' }).click();

    await expect(page.getByRole('status').filter({ hasText: 'Saved 1 change to smoke-skill.' })).toBeVisible();
    await expect(save).toBeDisabled();
    await expect(page.locator('#edit-desc')).toHaveValue('Reworded by the Tier 2 test.');
    expect(cli('skill', 'show', 'smoke-skill')).toContain('Reworded by the Tier 2 test.');
  });

  test('conflict shows the latest version and keeps an edit made elsewhere', async ({ page }) => {
    await page.goto(`${server.origin}/skills/smoke-skill?tab=editor`);
    await page.locator('#edit-trigs').fill('smoke it');
    cli('skill', 'edit', 'smoke-skill', '--description', 'Changed outside the browser', '--yes');

    await page.getByRole('button', { name: 'Preview changes' }).click();
    const drawer = page.locator('.fg-drawer');
    await expect(drawer.getByText('What changed meanwhile')).toBeVisible();
    await expect(drawer.getByText(/Description: "Smoke test skill\." becomes "Changed outside the browser"/)).toBeVisible();
    await expect(drawer.getByText(/Triggers: added "smoke it"/)).toBeVisible();

    await drawer.getByRole('button', { name: 'Keep my edits on the latest version' }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByText('Save changes to smoke-skill?')).toBeVisible();
    // The preview carries only the edit made here; the outside description is not reverted.
    await expect(dialog.getByText(/Triggers: added "smoke it"/)).toBeVisible();
    await expect(dialog.getByText(/Description:/)).toHaveCount(0);
    await dialog.getByRole('button', { name: 'Save changes' }).click();
    await expect(dialog).toBeHidden();

    const shown = cli('skill', 'show', 'smoke-skill');
    expect(shown).toContain('Changed outside the browser');
    expect(shown).toContain('smoke it');
  });

  test('Go to field on the Review tab puts the cursor in that field', async ({ page }) => {
    await page.goto(`${server.origin}/skills/smoke-skill`);
    const row = page
      .locator('div')
      .filter({ has: page.getByText('Triggers', { exact: true }), hasText: 'Go to field' })
      .last();
    await row.getByRole('button', { name: 'Go to field' }).click();
    await expect(page.getByRole('tab', { name: 'Editor' })).toHaveAttribute('aria-selected', 'true');
    await expect(page.locator('#edit-trigs')).toBeFocused();
  });

  test('add from GitHub starts empty and says nothing is written yet', async ({ page }) => {
    await page.goto(`${server.origin}/skills/add`);
    const url = page.locator('#gh-url');
    await expect(url).toHaveValue('');
    await expect(url).toHaveAttribute('placeholder', /OWNER\/REPO/);
    await expect(page.getByRole('button', { name: 'Discover skills' })).toBeDisabled();
    await expect(page.getByText(/Nothing is written to your hub until you confirm/)).toBeVisible();
    await page.getByText('Advanced', { exact: false }).first().click();
    await expect(page.getByText(/The folder the imported skills are filed under/)).toBeVisible();
  });

  test('upstream updates are reachable from the Skills list without typing a URL', async ({ page }) => {
    await page.goto(`${server.origin}/skills`);
    await page.getByLabel('Upstream').selectOption('updates');
    await expect(page).toHaveURL(/upstream=updates/);
    await expect(page.getByText(/No skill has an upstream update right now/)).toBeVisible();
    await expect(page.getByText('skillhub skill outdated --check')).toBeVisible();
  });
});
