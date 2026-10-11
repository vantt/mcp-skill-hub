import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { expect, test } from '@playwright/test';
import { startServer, type RunningServer } from './support/server';
import { makeCLIRunner } from './support/seed-workspace';

const SKILL_ID = 'track-me';
const UPSTREAM_REPO = 'https://github.com/example/track-me';
// The repository name equals the skill id, so the new source record skips that id.
const TRACKED_SOURCE_ID = 'track-me-1';
const WATCHED_SOURCE_ID = 'notes-fs';

// A skill copied from GitHub before upstream tracking existed: its metadata names the
// repository but no source record is linked, and one filesystem source is watched but
// not attached to any skill yet. Nothing here needs the network.
function seedWorkspace(): string {
  const ws = fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-e2e-skill-sources-'));
  const runCLI = makeCLIRunner(ws);
  runCLI(['init', ws, '--yes']);

  // Outside the workspace: a stray file inside it would be refused as off-layout.
  const contentFile = path.join(os.tmpdir(), `track-me-content-${process.pid}-${Date.now()}.md`);
  fs.writeFileSync(contentFile, '# Track Me\n\nInstructions copied from an upstream repository.\n');
  try {
    runCLI([
      'skill',
      'create',
      SKILL_ID,
      '--collection',
      'core',
      '--name',
      'Track Me',
      '--description',
      'A skill copied from GitHub without a tracked upstream.',
      '--content-file',
      contentFile,
      '--yes',
    ]);
  } finally {
    fs.rmSync(contentFile, { force: true });
  }

  const metaPath = path.join(ws, 'skills', 'core', SKILL_ID, '.meta', 'skill.yaml');
  const meta = fs.readFileSync(metaPath, 'utf-8');
  fs.writeFileSync(
    metaPath,
    `${meta.trimEnd()}\nsources:\n    - id: ""\n      kind: github\n      repo: ${UPSTREAM_REPO}\n      ref: main\n      roles:\n        - upstream\n`,
  );

  const fixtureDir = path.join(ws, 'runtime', 'fixtures', WATCHED_SOURCE_ID);
  fs.mkdirSync(fixtureDir, { recursive: true });
  fs.writeFileSync(path.join(fixtureDir, 'SKILL.md'), '# Notes\nIdeas worth learning from.\n');
  const catalogDir = path.join(ws, 'sources', 'catalog');
  fs.mkdirSync(catalogDir, { recursive: true });
  fs.writeFileSync(
    path.join(catalogDir, `${WATCHED_SOURCE_ID}.yaml`),
    `schema_version: 1
id: ${WATCHED_SOURCE_ID}
adapter: filesystem
locator:
  path: runtime/fixtures/${WATCHED_SOURCE_ID}
status: watching
identity:
  name: ${WATCHED_SOURCE_ID}
  canonical: runtime/fixtures/${WATCHED_SOURCE_ID}
trust:
  source: test
  reviewed: true
monitoring:
  enabled: true
  cadence: weekly
limits:
  timeout_seconds: 20
  max_bytes: 8388608
  max_files: 100
  max_file_bytes: 2097152
`,
  );
  return ws;
}

test.describe('Skill sources from the browser', () => {
  let server: RunningServer;
  let ws: string;

  test.beforeEach(async () => {
    ws = seedWorkspace();
    server = await startServer({ workspace: ws });
  });

  test.afterEach(async () => {
    await server?.stop();
    if (ws) fs.rmSync(ws, { recursive: true, force: true });
  });

  test('tracks an untracked upstream and links a watched source, offline', async ({ page }) => {
    test.setTimeout(120_000);
    const requestedUrls: string[] = [];
    page.on('request', (req) => requestedUrls.push(req.url()));

    await page.goto(server.url);
    await page.goto(`${server.origin}/skills/${SKILL_ID}`);

    // Overview opens first and shows the upstream the metadata names, not tracked yet.
    await expect(page.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true');
    const overviewSources = page.getByRole('region', { name: 'Sources', exact: true });
    await expect(overviewSources.getByText(`${UPSTREAM_REPO}@main`)).toBeVisible();
    await expect(overviewSources.getByText('Not tracked')).toBeVisible();

    // Sources tab: an untracked upstream cannot be checked, only tracked.
    await page.getByRole('tab', { name: /^Sources/ }).click();
    const upstream = page.locator('section.fg-card').filter({ hasText: 'Upstream repository' });
    await expect(upstream.getByText('Not tracked')).toBeVisible();
    await expect(upstream.getByRole('button', { name: 'Check now' })).toBeDisabled();

    await upstream.getByRole('button', { name: 'Track upstream' }).click();
    const trackDialog = page.getByRole('dialog');
    await expect(trackDialog.getByText(`Track upstream for ${SKILL_ID}?`)).toBeVisible();
    await expect(trackDialog.getByText(`sources/catalog/${TRACKED_SOURCE_ID}.yaml`, { exact: true })).toBeVisible();
    await expect(trackDialog.getByText(`skills/core/${SKILL_ID}/.meta/skill.yaml`, { exact: true })).toBeVisible();
    const exactChanges = trackDialog.locator('details').filter({ hasText: 'Exact file changes' });
    if (!(await exactChanges.evaluate((el) => (el as HTMLDetailsElement).open))) {
      await exactChanges.locator('summary').click();
    }
    await expect(exactChanges.getByText(`id: ${TRACKED_SOURCE_ID}`).first()).toBeVisible();
    await trackDialog.getByRole('button', { name: 'Track upstream' }).click();
    await expect(trackDialog).toBeHidden();

    await expect(upstream.getByText('Not tracked')).toHaveCount(0);
    await expect(upstream.getByRole('button', { name: 'Track upstream' })).toHaveCount(0);
    expect(fs.existsSync(path.join(ws, 'sources', 'catalog', `${TRACKED_SOURCE_ID}.yaml`))).toBe(true);
    expect(fs.readFileSync(path.join(ws, 'skills', 'core', SKILL_ID, '.meta', 'skill.yaml'), 'utf-8')).toContain(
      `id: ${TRACKED_SOURCE_ID}`,
    );

    // Add source: link the watched source as a learning reference, after a preview.
    await page.getByRole('button', { name: 'Add source' }).first().click();
    const form = page.locator('section').filter({ hasText: 'Add a learning reference' });
    await form.getByRole('radio', { name: 'Link a watched source' }).click();
    await form.getByLabel('Watched source').selectOption(WATCHED_SOURCE_ID);
    await form.getByRole('button', { name: 'Preview' }).click();

    const addDialog = page.getByRole('dialog');
    await expect(addDialog.getByText(`Add source ${WATCHED_SOURCE_ID}?`)).toBeVisible();
    await expect(addDialog.getByText(/^sources\/skills\/LINK-.*\.yaml$/)).toBeVisible();
    await addDialog.getByRole('button', { name: 'Add source' }).click();
    await expect(addDialog).toBeHidden();

    const learning = page.locator('section.fg-card').filter({ hasText: 'Learning references' });
    await expect(learning.getByText(WATCHED_SOURCE_ID, { exact: true })).toBeVisible();
    await expect(learning.getByRole('button', { name: 'Unlink' })).toBeVisible();

    // Back on Overview the new learning reference and the tracked upstream are listed.
    await page.getByRole('tab', { name: 'Overview' }).click();
    await expect(overviewSources.getByText(WATCHED_SOURCE_ID, { exact: true })).toBeVisible();
    await expect(overviewSources.getByText('Learning reference', { exact: true })).toBeVisible();
    await expect(overviewSources.getByText('Not tracked')).toHaveCount(0);

    expect(requestedUrls.length).toBeGreaterThan(0);
    for (const reqUrl of requestedUrls) {
      const isAllowed = reqUrl.startsWith(server.origin) || reqUrl.startsWith('data:');
      expect(isAllowed, `Non-loopback request observed: ${reqUrl}`).toBe(true);
    }
  });
});
