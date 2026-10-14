import { createHash } from 'node:crypto';
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

// A skill another machine sent: the hub keeps it in a queue under runtime/ until a person reads it.
// The queue is written the way the hub's /mcp/submissions endpoint writes it.
const CROCKFORD = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

function seedSubmission(skillId: string, label: string, files: Record<string, string>): string {
  let tail = '';
  for (let i = 0; i < 26; i += 1) tail += CROCKFORD[Math.floor(Math.random() * CROCKFORD.length)];
  const id = `SUB-${tail}`;
  const dir = path.join(ws, 'runtime', 'submissions', id);
  const listed = Object.entries(files).map(([rel, body]) => {
    const full = path.join(dir, 'files', rel);
    fs.mkdirSync(path.dirname(full), { recursive: true });
    fs.writeFileSync(full, body);
    return { path: rel, size: Buffer.byteLength(body), digest: `sha256:${createHash('sha256').update(body).digest('hex')}` };
  });
  fs.writeFileSync(
    path.join(dir, 'submission.json'),
    JSON.stringify({
      submission_id: id,
      skill_id: skillId,
      client_label: label,
      submitted_at: '2026-10-14T10:00:00Z',
      excluded: [{ path: '.env', reason: 'looks like a secret' }],
      files: listed,
      total_bytes: listed.reduce((sum, file) => sum + file.size, 0),
    }),
  );
  return id;
}

test('waiting for review: a skill submitted from another machine is read in full and approved', async ({ page }) => {
  test.setTimeout(120_000);
  const skillMd = '---\nname: release-notes\ndescription: Write the release notes. Use when the team ships.\n---\n\n# Release notes\n\nSummarize what shipped.\n';
  const sub = seedSubmission('release-notes', 'machine-a', { 'SKILL.md': skillMd, 'scripts/notes.sh': 'echo notes\n' });
  const headBefore = Number(git('rev-list', '--count', 'HEAD'));

  await page.goto(server.url);
  await page.goto(`${server.origin}/review`);
  const queue = page.getByRole('list', { name: 'Skills waiting for review' });
  await expect(queue.getByText('Submitted · needs approval')).toBeVisible();
  await expect(queue.getByText('Submitted from machine-a')).toBeVisible();
  await queue.getByRole('link', { name: 'release-notes' }).click();
  await expect(page).toHaveURL(new RegExp(`/review/submissions/${sub}$`));

  await expect(page.getByText(/Origin: Submitted from machine-a/)).toBeVisible();
  await expect(page.getByText(/Left out by the submitting machine: \.env \(looks like a secret\)/)).toBeVisible();
  const approve = page.getByRole('button', { name: 'Approve' });
  await expect(approve).toBeDisabled();
  const changed = page.getByRole('list', { name: 'Changed files' });
  await expect(changed.locator('li')).toHaveCount(2);
  await changed.locator('summary', { hasText: 'scripts/notes.sh' }).click();
  await expect(changed.getByText('echo notes')).toBeVisible();
  await expect(approve).toBeDisabled();
  await changed.locator('summary', { hasText: 'SKILL.md' }).click();
  await expect(approve).toBeEnabled();

  await approve.click();
  await expect(page.getByTestId('approve-result')).toContainText(/Approved\. Committed [0-9a-f]{7}\./, { timeout: 15000 });
  await expect(page.getByTestId('approve-result')).toContainText('It is a draft now');
  expect(Number(git('rev-list', '--count', 'HEAD'))).toBe(headBefore + 1);
  expect(git('log', '-1', '--format=%s')).toBe('Add release-notes (submitted from machine-a), approve content');
  expect(git('status', '--porcelain=v1')).toBe('');
  expect(fs.existsSync(path.join(ws, 'runtime', 'submissions', sub))).toBe(false);

  await page.getByRole('link', { name: 'Back to Waiting for review' }).click();
  await expect(page.getByRole('list', { name: 'Skills waiting for review' }).getByText('Submitted · needs approval')).toHaveCount(0);
});

test('waiting for review: a submission can be discarded and changes nothing', async ({ page }) => {
  const sub = seedSubmission('scratch-skill', 'machine-b', {
    'SKILL.md': '---\nname: scratch-skill\ndescription: Scratch. Use when testing.\n---\n\n# Scratch\n',
  });
  const head = git('rev-parse', 'HEAD');
  await page.goto(server.url);
  await page.goto(`${server.origin}/review/submissions/${sub}`);
  await page.getByRole('button', { name: 'Discard' }).click();
  await expect(page.getByText('Discard this submission?')).toBeVisible();
  await page.getByRole('button', { name: 'Yes, discard' }).click();
  await expect(page.getByTestId('discard-result')).toContainText('Discarded.');
  expect(fs.existsSync(path.join(ws, 'runtime', 'submissions', sub))).toBe(false);
  expect(git('rev-parse', 'HEAD')).toBe(head);
  await page.goto(`${server.origin}/review/submissions/${sub}`);
  await expect(page.getByText('Page not found.')).toBeVisible();
});
