import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { expect, test } from '@playwright/test';
import { createHubSkill, gitIdentity, makeCLIRunner, resolveBinaryPath } from './support/seed-workspace';
import { startServer, type RunningServer } from './support/server';

// A skill with companion files: a script, two reference documents, an old one to delete and a
// binary image. The hub is built offline through the CLI and always lives in a temporary folder.
const SKILL_ID = 'pdf-tools';
const SKILL_DIR = `skills/core/${SKILL_ID}`;
const FORMS = 'references/forms.md';
const TABLES = 'references/tables.md';
const OLD = 'references/old.md';
const SCRIPT = 'scripts/extract.sh';
const LOGO_BYTES = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52]);

let server: RunningServer;
let ws: string;

function git(...args: string[]): string {
  return execFileSync('git', ['-C', ws, ...args], { encoding: 'utf-8', env: { ...process.env, ...gitIdentity } }).trim();
}

function seedFilesWorkspace(): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-e2e-files-'));
  const runCLI = makeCLIRunner(dir);
  runCLI(['init']);
  createHubSkill(runCLI, {
    id: SKILL_ID,
    name: 'PDF tools',
    description: 'Fill and read PDF forms.',
    body: '# PDF tools\n\nRun scripts/extract.sh, then follow references/forms.md.\n',
    triggers: ['fill a pdf form'],
    notFor: ['write prose'],
    files: {
      [FORMS]: 'Forms\nline one\n',
      [TABLES]: 'Tables\n',
      [OLD]: 'Obsolete notes\n',
      [SCRIPT]: '#!/bin/sh\necho "extract"\n',
      'assets/logo.png': LOGO_BYTES,
    },
    executable: [SCRIPT],
  });
  runCLI(['skill', 'activate', SKILL_ID, '--yes']);
  return dir;
}

test.beforeAll(async () => {
  resolveBinaryPath();
  ws = seedFilesWorkspace();
  server = await startServer({ workspace: ws });
});

test.afterAll(async () => {
  await server?.stop();
  if (ws) fs.rmSync(ws, { recursive: true, force: true });
});

test('files tab: edit, add, upload, rename and delete, review them, and commit once', async ({ page }) => {
  test.setTimeout(120_000);
  const requestedUrls: string[] = [];
  page.on('request', (req) => requestedUrls.push(req.url()));

  // The tokened address stores the session for the later direct visits.
  await page.goto(server.url);
  await page.goto(`${server.origin}/skills/${SKILL_ID}?tab=files`);
  await expect(page.locator('.t-title').first()).toHaveText('PDF tools');

  // 1. SKILL.md comes first, folders hold their files, binary files are marked as such.
  const items = page.getByRole('treeitem');
  await expect(items.first()).toHaveAccessibleName('SKILL.md');
  await expect(page.getByRole('treeitem', { name: FORMS })).toBeVisible();
  await expect(page.getByRole('treeitem', { name: 'assets/logo.png' })).toBeVisible();
  await page.getByRole('treeitem', { name: 'assets/logo.png' }).click();
  await expect(page.getByText(/This is a binary file \(16 bytes\)/)).toBeVisible();
  await expect(page.getByRole('button', { name: 'Edit', exact: true })).toHaveCount(0);
  await expect(page.getByText(/not committed/)).toHaveCount(0);
  const commitsBefore = Number(git('rev-list', '--count', 'HEAD'));

  // 2. Edit SKILL.md and a reference document.
  await page.getByRole('treeitem', { name: 'SKILL.md' }).click();
  await page.getByRole('button', { name: 'Edit', exact: true }).click();
  const skillMd = page.getByRole('textbox', { name: 'Edit SKILL.md' });
  const skillText = await skillMd.inputValue();
  await skillMd.fill(skillText.replace('follow references/forms.md', 'follow references/forms.md and references/ocr.md'));

  await page.getByRole('treeitem', { name: FORMS }).click();
  await page.getByRole('button', { name: 'Edit', exact: true }).click();
  await page.getByRole('textbox', { name: `Edit ${FORMS}` }).fill('Forms\nline one\nline two\n');
  await expect(page.getByText('2 changes not committed')).toBeVisible();
  await expect(page.getByText(/agents still get the current version/)).toBeVisible();

  // 3. Add a file, upload a binary file, rename one file and delete another.
  await page.getByRole('button', { name: '+ File' }).click();
  await page.getByLabel(/Path of the new file/).fill('references/ocr.md');
  await page.getByRole('button', { name: 'Add', exact: true }).click();
  await page.getByRole('textbox', { name: 'Edit references/ocr.md' }).fill('# OCR\n\nScan first.\n');

  await page.getByLabel('Upload files').setInputFiles({ name: 'pixel.bin', mimeType: 'application/octet-stream', buffer: Buffer.from([0, 1, 2, 3]) });
  await expect(page.getByRole('treeitem', { name: /pixel\.bin \(added\)/ })).toBeVisible();

  await page.getByRole('treeitem', { name: TABLES }).click();
  await page.getByRole('button', { name: 'Rename', exact: true }).click();
  await page.getByLabel('New path').fill('references/data-tables.md');
  await page.getByRole('button', { name: 'Rename', exact: true }).last().click();
  await expect(page.getByRole('treeitem', { name: /data-tables\.md \(renamed from references\/tables\.md\)/ })).toBeVisible();

  await page.getByRole('treeitem', { name: OLD }).click();
  await page.getByRole('button', { name: 'Delete', exact: true }).click();
  await expect(page.getByRole('treeitem', { name: `${OLD} (deleted)` })).toBeVisible();
  await expect(page.getByText('6 changes not committed')).toBeVisible();

  // Nothing reached the hub yet, and a reload keeps the pending changes.
  expect(git('status', '--porcelain=v1')).toBe('');
  await page.reload();
  await expect(page.getByText('6 changes not committed')).toBeVisible();

  // 4. Review: every file with its mark and line counts, the routing lint and the message.
  await page.getByRole('button', { name: /Review/ }).click();
  await expect(page.getByRole('heading', { name: 'Review changes' })).toBeVisible();
  const files = page.getByRole('list', { name: 'Changed files' });
  await expect(files.getByText('SKILL.md', { exact: true })).toBeVisible({ timeout: 15000 });
  await expect(files.locator('li')).toHaveCount(6);
  await expect(files.locator('.app-filetree__mark--M')).toHaveCount(2);
  await expect(files.locator('.app-filetree__mark--A')).toHaveCount(2);
  await expect(files.locator('.app-filetree__mark--R')).toHaveCount(1);
  await expect(files.locator('.app-filetree__mark--D')).toHaveCount(1);
  await files.locator('summary', { hasText: FORMS }).click();
  await expect(files.getByText('line two')).toBeVisible();
  await expect(page.getByText(/Routing lint/).first()).toBeVisible();
  await expect(page.getByLabel('Commit message')).toHaveValue(`Commit ${SKILL_ID}: 6 files`);

  // 5. Commit: one commit with the typed message, the reach line, and exactly those paths.
  await page.getByLabel('Commit message').fill('Tidy the PDF references');
  await page.getByRole('button', { name: 'Commit to the hub' }).click();
  await expect(page.getByTestId('commit-result')).toContainText(/Committed [0-9a-f]{7}\. Agents using this hub get it now\./, { timeout: 15000 });
  await expect(page.getByText(`${SKILL_ID} committed`).first()).toBeVisible();

  expect(Number(git('rev-list', '--count', 'HEAD'))).toBe(commitsBefore + 1);
  expect(git('log', '-1', '--format=%s')).toBe('Tidy the PDF references');
  const changed = git('show', '--no-renames', '--name-status', '--format=', 'HEAD')
    .split('\n')
    .filter((line) => line.includes(SKILL_DIR));
  expect(changed.map((line) => line.replace(`${SKILL_DIR}/`, '')).sort()).toEqual(
    [
      'A\treferences/pixel.bin',
      'A\treferences/data-tables.md',
      'A\treferences/ocr.md',
      `D\t${OLD}`,
      `D\t${TABLES}`,
      'M\tSKILL.md',
      `M\t${FORMS}`,
    ].sort(),
  );
  expect(git('status', '--porcelain=v1')).toBe('');
  expect(fs.readFileSync(path.join(ws, SKILL_DIR, FORMS), 'utf-8')).toBe('Forms\nline one\nline two\n');
  expect(fs.readFileSync(path.join(ws, SKILL_DIR, 'references/pixel.bin'))).toEqual(Buffer.from([0, 1, 2, 3]));
  expect(fs.statSync(path.join(ws, SKILL_DIR, SCRIPT)).mode & 0o777).toBe(0o755);

  // 6. Back on the files tab nothing is pending, and the tree shows the committed files.
  await page.getByRole('button', { name: 'Back to files' }).click();
  await expect(page.getByText(/not committed/)).toHaveCount(0);
  await expect(page.getByRole('treeitem', { name: 'references/data-tables.md' })).toBeVisible();
  await expect(page.getByRole('treeitem', { name: OLD })).toHaveCount(0);

  // 7. Checkout to a project: the dialog gives the command.
  await page.getByRole('button', { name: 'Checkout to a project' }).click();
  await expect(page.getByRole('dialog').getByText(`skillhub skill checkout ${SKILL_ID}`)).toBeVisible();
  await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click();

  // Every request stayed on the loopback origin.
  expect(requestedUrls.length).toBeGreaterThan(0);
  for (const reqUrl of requestedUrls) {
    expect(reqUrl.startsWith(server.origin) || reqUrl.startsWith('data:'), `Non-loopback request observed: ${reqUrl}`).toBe(true);
  }
});

test('files tab: a change made on the hub meanwhile opens the conflict drawer', async ({ page }) => {
  test.setTimeout(60_000);
  await page.goto(server.url);
  await page.goto(`${server.origin}/skills/${SKILL_ID}?tab=files&file=${encodeURIComponent('references/data-tables.md')}`);
  await page.getByRole('button', { name: 'Edit', exact: true }).click();
  await page.getByRole('textbox', { name: 'Edit references/data-tables.md' }).fill('Tables\nmine\n');
  await expect(page.getByText('1 change not committed')).toBeVisible();

  // Someone else changes the same file and commits it.
  fs.writeFileSync(path.join(ws, SKILL_DIR, 'references/data-tables.md'), 'Tables\ntheirs\n');
  git('add', '-A');
  git('commit', '--quiet', '-m', 'Change the tables elsewhere');

  await page.getByRole('button', { name: /Review/ }).click();
  const drawer = page.locator('.fg-drawer');
  await expect(drawer).toBeVisible({ timeout: 15000 });
  await expect(drawer.getByText('The hub changed after you started these changes.')).toBeVisible();
  await expect(drawer.getByRole('button', { name: /overwrite/i })).toHaveCount(0);

  // Keeping the edits puts them on the latest files; the review is made again against them and
  // shows exactly what will be overwritten.
  await drawer.getByRole('button', { name: 'Keep my edits on the latest version' }).click();
  await expect(drawer).toBeHidden({ timeout: 10000 });
  await expect(page.getByLabel('Commit message')).toBeVisible({ timeout: 15000 });
  const diffs = page.getByRole('list', { name: 'Changed files' });
  await diffs.locator('summary').first().click();
  await expect(diffs.getByText('theirs')).toBeVisible();
  await expect(diffs.getByText('mine')).toBeVisible();
  await page.getByRole('button', { name: 'Back to files' }).click();
  await page.getByRole('button', { name: 'Discard' }).click();
  await page.getByRole('button', { name: 'Discard changes' }).click();
  await expect(page.getByText(/not committed/)).toHaveCount(0);
});

test('files tab: a phone screen has no sideways scroll', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(server.url);
  for (const route of [`/skills/${SKILL_ID}?tab=files`, '/review']) {
    await page.goto(`${server.origin}${route}`);
    await page.waitForLoadState('networkidle');
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow, `${route} scrolls sideways by ${overflow}px`).toBeLessThanOrEqual(0);
  }
});

test('old editor and resources links open Routing and Files', async ({ page }) => {
  await page.goto(server.url);
  await page.goto(`${server.origin}/skills/${SKILL_ID}?tab=resources`);
  await expect(page.getByRole('tab', { name: 'Files' })).toHaveAttribute('aria-selected', 'true');
  await page.goto(`${server.origin}/skills/${SKILL_ID}?tab=editor`);
  await expect(page.getByRole('tab', { name: 'Routing' })).toHaveAttribute('aria-selected', 'true');
});
