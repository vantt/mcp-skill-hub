import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { makeCLIRunner, resolveBinaryPath } from './support/seed-workspace';
import { startServer, type RunningServer } from './support/server';

// A skill with three companion files: an executable script, a reference document with YAML
// frontmatter and a binary image. The workspace is built offline through the CLI.
const SKILL_ID = 'resource-skill';
const SKILL_DIR = 'skills/core/resource-skill';
const RUN_SH = `${SKILL_DIR}/scripts/run.sh`;
const GUIDE_MD = `${SKILL_DIR}/references/guide.md`;
const LOGO_PNG = `${SKILL_DIR}/assets/logo.png`;

const RUN_SH_BODY = '#!/bin/sh\necho "first version"\n';
const GUIDE_BODY = '---\ntitle: Release guide\naudience: maintainers\n---\n\n# Release guide\n\nTag the release after the checks pass.\n';
const LOGO_BYTES = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52]);

let server: RunningServer;
let ws: string;

function seedResourceWorkspace(): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-e2e-resources-'));
  const runCLI = makeCLIRunner(dir);
  runCLI(['init', dir]);

  // Outside the workspace: a stray file in it would be refused as non-canonical content.
  const contentFile = path.join(os.tmpdir(), `${SKILL_ID}-${Date.now()}.md`);
  fs.writeFileSync(
    contentFile,
    `---\nname: ${SKILL_ID}\ndescription: Release a package.\n---\n\n# Resource Skill\n\nRun scripts/run.sh, then follow references/guide.md.\n`,
  );
  try {
    runCLI([
      'skill', 'create', SKILL_ID,
      '--collection', 'core',
      '--name', 'Resource Skill',
      '--description', 'Release a package.',
      '--content-file', contentFile,
      '--yes',
    ]);
  } finally {
    fs.rmSync(contentFile, { force: true });
  }

  const write = (rel: string, body: string | Buffer, mode?: number) => {
    const abs = path.join(dir, rel);
    fs.mkdirSync(path.dirname(abs), { recursive: true });
    fs.writeFileSync(abs, body);
    if (mode !== undefined) fs.chmodSync(abs, mode);
  };
  write(RUN_SH, RUN_SH_BODY, 0o755);
  write(GUIDE_MD, GUIDE_BODY);
  write(LOGO_PNG, LOGO_BYTES);
  runCLI(['rebuild']);
  return dir;
}

interface PreviewResponse {
  diff: { added?: string[]; modified?: string[]; deleted?: string[] };
  full_diff: string;
}

// Resolves with the next successful resource preview proposal the page receives.
async function waitForPreview(page: Page): Promise<PreviewResponse> {
  const res = await page.waitForResponse(
    (r) => r.url().includes(`/skills/${SKILL_ID}/resources/preview`) && r.request().method() === 'POST' && r.ok(),
  );
  return (await res.json()) as PreviewResponse;
}

// The "+++ b/<path>" header of every file the patch touches.
function diffHeaders(patch: string): string[] {
  return patch.split('\n').filter((line) => line.startsWith('+++ '));
}

async function openResources(page: Page) {
  await page.goto(`${server.origin}/skills/${SKILL_ID}?tab=resources`);
  await expect(page.locator('.t-title')).toHaveText('Resource Skill');
  await expect(page.getByRole('button', { name: `Edit ${RUN_SH}` })).toBeEnabled();
}

test.beforeAll(async () => {
  resolveBinaryPath();
  ws = seedResourceWorkspace();
  server = await startServer({ workspace: ws });
});

test.afterAll(async () => {
  await server?.stop();
  if (ws) fs.rmSync(ws, { recursive: true, force: true });
});

test('resources tab: view a binary file, edit a reference document and a script, resolve a conflict', async ({
  page,
}) => {
  test.setTimeout(120_000);
  const requestedUrls: string[] = [];
  page.on('request', (req) => requestedUrls.push(req.url()));

  // The tokened address stores the session for the later direct visits.
  await page.goto(server.url);
  await openResources(page);

  // 1. Text files get Edit, the binary file gets View only; SKILL.md stays in the Editor tab.
  await expect(page.getByRole('button', { name: `Edit ${GUIDE_MD}` })).toBeVisible();
  await expect(page.getByRole('button', { name: `View ${LOGO_PNG}` })).toBeVisible();
  await expect(page.getByRole('button', { name: `Edit ${LOGO_PNG}` })).toHaveCount(0);
  await expect(page.getByRole('button', { name: new RegExp(`SKILL\\.md$`) })).toHaveCount(0);

  // 2. A binary file opens read-only: a notice with size and digest, never its bytes.
  await page.getByRole('button', { name: `View ${LOGO_PNG}` }).click();
  await expect(page).toHaveURL(new RegExp(`file=${encodeURIComponent(LOGO_PNG)}`));
  await expect(page.getByText(/^Binary file: its content is not shown\. Size 16 bytes, digest /)).toBeVisible();
  await expect(page.getByTestId('resource-source')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Edit', exact: true })).toHaveCount(0);
  await expect(page.getByRole('textbox')).toHaveCount(0);
  await page.getByRole('button', { name: 'Back to resources' }).click();
  await expect(page).not.toHaveURL(/file=/);

  // 3. Edit the reference document: the Markdown preview shows the frontmatter as a YAML block.
  await page.getByRole('button', { name: `Edit ${GUIDE_MD}` }).click();
  await expect(page).toHaveURL(/edit=1/);
  const guideArea = page.getByRole('textbox', { name: 'guide.md content' });
  await expect(guideArea).toHaveValue(GUIDE_BODY);
  await expect(page.getByLabel('Frontmatter')).toContainText('title: Release guide');
  await expect(page.getByRole('heading', { name: 'Release guide' })).toBeVisible();

  const editedGuide = GUIDE_BODY.replace('Tag the release after the checks pass.', 'Tag the release after review and checks.');
  await guideArea.fill(editedGuide);
  await expect(page.locator('.fg-markdown').getByText('Tag the release after review and checks.')).toBeVisible();

  const guidePreview = waitForPreview(page);
  await page.getByRole('button', { name: 'Preview changes' }).click();
  const guideProposal = await guidePreview;
  expect(guideProposal.diff.modified).toEqual([GUIDE_MD]);
  expect(guideProposal.diff.added ?? []).toEqual([]);
  expect(guideProposal.diff.deleted ?? []).toEqual([]);
  expect(diffHeaders(guideProposal.full_diff)).toEqual([`+++ b/${GUIDE_MD}`]);
  expect(guideProposal.full_diff).toContain('\n-Tag the release after the checks pass.\n');
  expect(guideProposal.full_diff).toContain('\n+Tag the release after review and checks.\n');

  const guideModal = page.locator('.fg-modal');
  await expect(guideModal).toBeVisible({ timeout: 15000 });
  await expect(guideModal.getByText(`changed: ${GUIDE_MD}`)).toBeVisible();
  await guideModal.getByText('Exact file changes').click();
  await expect(guideModal).toContainText('Tag the release after the checks pass.');
  await expect(guideModal).toContainText('Tag the release after review and checks.');
  await expect(guideModal).not.toContainText('SKILL.md');
  await expect(guideModal).not.toContainText('run.sh');

  // Nothing is written before the confirm.
  expect(fs.readFileSync(path.join(ws, GUIDE_MD), 'utf-8')).toBe(GUIDE_BODY);
  await guideModal.getByRole('button', { name: 'Save changes' }).click();
  await expect(guideModal).toBeHidden({ timeout: 10000 });
  await expect(page.getByText('Saved guide.md.')).toBeVisible();
  expect(fs.readFileSync(path.join(ws, GUIDE_MD), 'utf-8')).toBe(editedGuide);

  // After saving the view reads the file and the address no longer asks for the editor, so a reload
  // opens the saved text to read, with no draft.
  await expect(page).not.toHaveURL(/edit=1/);
  await page.reload();
  await expect(page.getByRole('textbox', { name: 'guide.md content' })).toHaveCount(0);
  await expect(page.locator('.fg-markdown').getByText('Tag the release after review and checks.')).toBeVisible();

  // 4. Edit the script, then change it on disk before previewing: the conflict drawer opens.
  await openResources(page);
  await page.getByRole('button', { name: `Edit ${RUN_SH}` }).click();
  const scriptArea = page.getByRole('textbox', { name: 'run.sh content' });
  await expect(scriptArea).toHaveValue(RUN_SH_BODY);
  const editedScript = '#!/bin/sh\necho "edited in the browser"\n';
  await scriptArea.fill(editedScript);

  const runShAbs = path.join(ws, RUN_SH);
  fs.writeFileSync(runShAbs, '#!/bin/sh\necho "changed on disk"\n');
  expect(fs.statSync(runShAbs).mode & 0o777).toBe(0o755);

  await page.getByRole('button', { name: 'Preview changes' }).click();
  const drawer = page.locator('.fg-drawer');
  await expect(drawer).toBeVisible({ timeout: 10000 });
  await expect(drawer.getByText('run.sh changed since you opened it')).toBeVisible();
  await expect(drawer.getByRole('button', { name: /overwrite/i })).toHaveCount(0);

  const scriptPreview = waitForPreview(page);
  await drawer.getByRole('button', { name: 'Keep my edits on the latest version' }).click();
  await expect(drawer).toBeHidden({ timeout: 10000 });
  const scriptProposal = await scriptPreview;
  expect(diffHeaders(scriptProposal.full_diff)).toEqual([`+++ b/${RUN_SH}`]);
  expect(scriptProposal.full_diff).toContain('\n-echo "changed on disk"\n');
  expect(scriptProposal.full_diff).toContain('\n+echo "edited in the browser"\n');
  const scriptModal = page.locator('.fg-modal');
  await expect(scriptModal).toBeVisible({ timeout: 15000 });
  await scriptModal.getByRole('button', { name: 'Save changes' }).click();
  await expect(scriptModal).toBeHidden({ timeout: 10000 });
  await expect(page.getByText('Saved run.sh.')).toBeVisible();

  expect(fs.readFileSync(runShAbs, 'utf-8')).toBe(editedScript);
  expect(fs.statSync(runShAbs).mode & 0o777).toBe(0o755);

  // 5. Every request stayed on the loopback origin.
  expect(requestedUrls.length).toBeGreaterThan(0);
  for (const reqUrl of requestedUrls) {
    const isAllowed = reqUrl.startsWith(server.origin) || reqUrl.startsWith('data:');
    expect(isAllowed, `Non-loopback request observed: ${reqUrl}`).toBe(true);
  }
});
