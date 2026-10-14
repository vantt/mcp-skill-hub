import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { expect, test } from '@playwright/test';
import { commitHub, createHubSkill, gitIdentity, makeCLIRunner, resolveBinaryPath } from './support/seed-workspace';
import { startServer, type RunningServer } from './support/server';

// A third-party skill whose content was approved and then changed upstream, and a new draft. The
// hub is built offline; "third party" is a recorded source repository, nothing is fetched.
const PDF = 'pdf';
const DRAFT = 'team-release';
const PDF_DIR = `skills/core/${PDF}`;

let server: RunningServer;
let ws: string;

function git(...args: string[]): string {
  return execFileSync('git', ['-C', ws, ...args], { encoding: 'utf-8', env: { ...process.env, ...gitIdentity } }).trim();
}

function seedReviewWorkspace(): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-e2e-review-'));
  const runCLI = makeCLIRunner(dir);
  runCLI(['init']);

  createHubSkill(runCLI, {
    id: PDF,
    name: 'PDF',
    description: 'Fill and read PDF forms.',
    body: '# PDF\n\nRun scripts/fill.py for forms.\n',
    triggers: ['fill a pdf form'],
    notFor: ['write prose'],
    files: { 'scripts/fill.py': 'print("one")\n', 'references/forms.md': 'Forms\n', LICENSE: 'Proprietary License\n\nAll rights reserved.\n' },
  });
  const metaPath = path.join(dir, PDF_DIR, '.meta', 'skill.yaml');
  fs.writeFileSync(
    metaPath,
    `${fs.readFileSync(metaPath, 'utf-8').trimEnd()}\nsources:\n    - id: ""\n      kind: github\n      repo: github.com/anthropics/skills\n      ref: main\n      roles:\n        - upstream\n`,
  );
  commitHub(dir, 'Record where the pdf skill came from');
  runCLI(['skill', 'activate', PDF, '--yes']);

  // Approve the content as a person would from a terminal, then let it change.
  const review = JSON.parse(runCLI(['skill', 'review', PDF, '--json'])) as { content_trust: { content_digest: string } };
  runCLI(['skill', 'edit', PDF, '--approve-content', review.content_trust.content_digest, '--yes']);
  fs.writeFileSync(path.join(dir, PDF_DIR, 'scripts/fill.py'), 'print("one")\nprint("two")\n');
  commitHub(dir, 'The upstream script changed');

  createHubSkill(runCLI, {
    id: DRAFT,
    name: 'Team release',
    description: 'Release the team package.',
    body: '# Team release\n\nTag, build and publish.\n',
    triggers: ['release the team package'],
    notFor: ['hotfixes'],
  });
  return dir;
}

test.beforeAll(async () => {
  resolveBinaryPath();
  ws = seedReviewWorkspace();
  server = await startServer({ workspace: ws });
});

test.afterAll(async () => {
  await server?.stop();
  if (ws) fs.rmSync(ws, { recursive: true, force: true });
});

test('waiting for review: a third-party skill goes from the list to a full review and an approval', async ({ page }) => {
  test.setTimeout(120_000);
  await page.goto(server.url);

  // 1. The list names what waits, and the navigation carries the count.
  await page.goto(`${server.origin}/review`);
  await expect(page.getByRole('heading', { name: 'Waiting for review (2)' })).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Primary' }).getByLabel('2 waiting')).toBeVisible();
  const queue = page.getByRole('list', { name: 'Skills waiting for review' });
  await expect(queue.getByText('Content changed · needs approval')).toBeVisible();
  await expect(queue.getByText('New draft')).toBeVisible();
  await expect(queue.getByText('github.com/anthropics/skills')).toBeVisible();

  // 2. Open the third-party skill: license, origin and what it is compared with are on top.
  await queue.getByRole('link', { name: 'PDF' }).click();
  await expect(page).toHaveURL(new RegExp(`/skills/${PDF}/review-content$`));
  await expect(page.getByText(/License: Proprietary License \(LICENSE\) · Origin: github\.com\/anthropics\/skills/)).toBeVisible();
  await expect(page.getByText(/Compared with the content you last approved/)).toBeVisible();

  // 3. Approve stays disabled until every changed file was opened.
  const approve = page.getByRole('button', { name: 'Approve' });
  await expect(approve).toBeDisabled();
  await expect(page.getByTestId('approve-hint')).toContainText('open every changed file first: 1 left');
  const changed = page.getByRole('list', { name: 'Changed files' });
  await expect(changed.locator('li')).toHaveCount(1);
  await changed.locator('summary', { hasText: 'scripts/fill.py' }).click();
  await expect(changed.getByText('print("two")')).toBeVisible();
  await expect(approve).toBeEnabled();
  await expect(page.getByTestId('approve-hint')).toHaveCount(0);

  // 4. Approve: one commit, the reach line, and the skill leaves the list.
  const before = Number(git('rev-list', '--count', 'HEAD'));
  await approve.click();
  await expect(page.getByTestId('approve-result')).toContainText(/Approved\. Committed [0-9a-f]{7}\. This machine's agents get it now/, { timeout: 15000 });
  expect(Number(git('rev-list', '--count', 'HEAD'))).toBe(before + 1);
  expect(git('status', '--porcelain=v1')).toBe('');

  await page.getByRole('link', { name: 'Back to Waiting for review' }).click();
  await expect(page.getByRole('heading', { name: 'Waiting for review (1)' })).toBeVisible();
  await expect(page.getByRole('list', { name: 'Skills waiting for review' }).getByText('Content changed · needs approval')).toHaveCount(0);
});

test('waiting for review: a draft is read in full and has nothing to approve', async ({ page }) => {
  await page.goto(server.url);
  await page.goto(`${server.origin}/skills/${DRAFT}/review-content`);
  await expect(page.getByText(/A new draft: every file is new/)).toBeVisible();
  await expect(page.getByRole('button', { name: 'Approve' })).toHaveCount(0);
  await expect(page.getByText(/nothing to approve/)).toBeVisible();
  await expect(page.getByRole('list', { name: 'Changed files' }).locator('li')).not.toHaveCount(0);
});

test('the content trust card sends a person to the review screen, not to a command', async ({ page }) => {
  await page.goto(server.url);
  // The skill was approved in the first test; change it again so its trust card asks for review.
  fs.writeFileSync(path.join(ws, PDF_DIR, 'scripts/fill.py'), 'print("three")\n');
  commitHub(ws, 'The upstream script changed again');
  await page.goto(`${server.origin}/skills/${PDF}?tab=review`);
  const link = page.getByRole('link', { name: 'Review and approve the content' });
  await expect(link).toBeVisible();
  await expect(page.getByText('--approve-content')).toHaveCount(0);
  await link.click();
  await expect(page).toHaveURL(new RegExp(`/skills/${PDF}/review-content$`));
});
