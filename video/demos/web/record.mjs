// Record the web UI scenes of the intro video with Playwright's built-in video recording.
//
//   node video/demos/web/record.mjs
//
// It starts `skillhub serve web` inside the demo sandbox (made by happy-path.sh, with a submission
// from seed-review.sh) on a fixed loopback port, walks two scenes in Chromium and stops the server:
//
//   web-edit.webm     Skills -> commit-style -> Files -> SKILL.md -> Edit -> Review -> Commit to the hub
//   web-approve.webm  Waiting for review -> release-checklist -> open every file -> Approve
//   <scene>-end.png   the last screen of each scene
//
// Environment:
//   DEMO_SANDBOX      sandbox folder (default ${TMPDIR:-/tmp}/skillhub-demo)
//   DEMO_WEB_PORT     port of the web UI (default 17421); a busy port is an error, never skipped
//   DEMO_RECORDINGS   output folder (default <repo>/video/recordings/web)
//   DEMO_WEB_THEME    light (default) or dark
//   DEMO_EDIT_SKILL   skill edited in the first scene (default commit-style)
//   DEMO_REVIEW_SKILL skill approved in the second scene (default release-checklist)
//   DEMO_SLOWMO       milliseconds between browser actions (default 150)
//   DEMO_PAUSE        milliseconds to hold on each screen (default 1200)
//   DEMO_HEADED=1     show the browser window while recording
//
// Playwright comes from web/node_modules: run `npm ci` in web/ and `npx playwright install chromium`.
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const repo = path.resolve(here, '../../..');
const sandbox = path.resolve(process.env.DEMO_SANDBOX || path.join(process.env.TMPDIR || '/tmp', 'skillhub-demo'));
const port = Number(process.env.DEMO_WEB_PORT || 17421);
const outDir = path.resolve(process.env.DEMO_RECORDINGS || path.join(repo, 'video/recordings/web'));
const theme = process.env.DEMO_WEB_THEME === 'dark' ? 'dark' : 'light';
const editSkill = process.env.DEMO_EDIT_SKILL || 'commit-style';
const reviewSkill = process.env.DEMO_REVIEW_SKILL || 'release-checklist';
const slowMo = Number(process.env.DEMO_SLOWMO || 150);
const pauseMs = Number(process.env.DEMO_PAUSE || 1200);

function fail(message) {
  console.error(`record.mjs: ${message}`);
  process.exitCode = 1;
}

function loadPlaywright() {
  const require = createRequire(path.join(repo, 'web/package.json'));
  try {
    return require('@playwright/test');
  } catch {
    throw new Error('Playwright not found in web/node_modules: run `npm ci` in web/, then `npx playwright install chromium`');
  }
}

function portIsFree(p) {
  return new Promise((resolve) => {
    const probe = net.createServer();
    probe.once('error', () => resolve(false));
    probe.once('listening', () => probe.close(() => resolve(true)));
    probe.listen(p, '127.0.0.1');
  });
}

// startServer runs serve web from the hub folder inside the sandbox environment (sandbox.sh sets
// HOME, XDG and Git config), and resolves with the tokened URL it prints.
function startServer() {
  const sandboxSh = path.join(here, '..', 'sandbox.sh');
  const script = `. "$1" "$2" 2>/dev/null || exit 1; cd "$HOME/skill-hub" || exit 1; exec skillhub serve web --addr 127.0.0.1:${port} --no-open`;
  const proc = spawn('bash', ['-c', script, 'bash', sandboxSh, sandbox], {
    stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, DEMO_FRESH: '0' },
  });
  const pids = path.join(sandbox, 'run', 'pids');
  fs.appendFileSync(pids, `${proc.pid}\n`);
  const forget = () => {
    const kept = fs.readFileSync(pids, 'utf8').split('\n').filter((line) => line && line !== String(proc.pid));
    fs.writeFileSync(pids, kept.length ? `${kept.join('\n')}\n` : '');
  };
  proc.once('exit', forget);
  const stop = () =>
    new Promise((resolve) => {
      if (proc.exitCode !== null || proc.signalCode !== null) return resolve();
      const timer = setTimeout(() => proc.kill('SIGKILL'), 3000);
      proc.once('exit', () => {
        clearTimeout(timer);
        resolve();
      });
      proc.kill('SIGTERM');
    });
  const ready = new Promise((resolve, reject) => {
    let out = '';
    const timer = setTimeout(() => reject(new Error(`serve web printed no URL within 20 s:\n${out}`)), 20000);
    const onData = (chunk) => {
      out += chunk.toString();
      const match = out.match(new RegExp(`http://127\\.0\\.0\\.1:${port}/#token=[0-9a-fA-F]+`));
      if (match) {
        clearTimeout(timer);
        resolve(match[0]);
      }
    };
    proc.stdout.on('data', onData);
    proc.stderr.on('data', onData);
    proc.once('exit', (code) => {
      clearTimeout(timer);
      reject(new Error(`serve web exited with code ${code}:\n${out}`));
    });
  });
  return { ready, stop };
}

const pause = (page, factor = 1) => page.waitForTimeout(pauseMs * factor);

// Type at the end of a textarea, key by key, so the edit is visible in the recording.
async function typeAtEnd(page, textbox, text) {
  await textbox.click();
  await textbox.evaluate((el) => {
    el.setSelectionRange(el.value.length, el.value.length);
    el.scrollTop = el.scrollHeight;
  });
  await page.keyboard.type(text, { delay: 45 });
}

async function sceneEdit(page, origin) {
  const nav = page.getByRole('navigation', { name: 'Primary' });
  await nav.getByRole('link', { name: 'Skills' }).click();
  await pause(page);
  // Each row of the skills table opens its skill.
  await page.getByRole('row').filter({ hasText: editSkill }).first().getByRole('cell').first().click();
  await pause(page);
  await page.getByRole('tab', { name: 'Files' }).click();
  await page.getByRole('treeitem', { name: 'SKILL.md' }).click();
  await pause(page);
  await page.getByRole('button', { name: 'Edit', exact: true }).click();
  const textbox = page.getByRole('textbox', { name: 'Edit SKILL.md' });
  await typeAtEnd(page, textbox, '\n4. Write the body in the past tense: what the commit changed.\n');
  await page.getByText(/1 change not committed|changes? not committed/).first().waitFor();
  await pause(page);
  await page.getByRole('button', { name: /Review/ }).click();
  await page.getByRole('heading', { name: 'Review changes' }).waitFor();
  const files = page.getByRole('list', { name: 'Changed files' });
  const summary = files.locator('summary', { hasText: 'SKILL.md' });
  if (await summary.count()) await summary.first().click();
  await pause(page, 1.5);
  await page.getByRole('button', { name: 'Commit to the hub' }).click();
  await page.getByTestId('commit-result').filter({ hasText: /Committed/ }).waitFor({ timeout: 20000 });
  await pause(page, 2);
  if (!page.url().startsWith(origin)) throw new Error(`left the web UI: ${page.url()}`);
}

async function sceneApprove(page) {
  const nav = page.getByRole('navigation', { name: 'Primary' });
  await nav.getByRole('link', { name: /Waiting for review/ }).click();
  const queue = page.getByRole('list', { name: 'Skills waiting for review' });
  await queue.waitFor();
  await pause(page);
  const item = queue.getByRole('link', { name: reviewSkill });
  if ((await item.count()) === 0) {
    throw new Error(`${reviewSkill} is not waiting for review: run video/demos/web/seed-review.sh first`);
  }
  await item.first().click();
  const approve = page.getByRole('button', { name: 'Approve' });
  await approve.waitFor();
  await pause(page);
  // Approve turns on once every changed file was opened.
  const summaries = page.getByRole('list', { name: 'Changed files' }).locator('summary');
  const count = await summaries.count();
  for (let i = 0; i < count; i += 1) {
    await summaries.nth(i).click();
    await pause(page, 0.8);
  }
  await approve.click();
  await page.getByTestId('approve-result').filter({ hasText: /Approved/ }).waitFor({ timeout: 20000 });
  await pause(page, 2);
}

async function record(browser, url, name, scene) {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-demo-video-'));
  const context = await browser.newContext({
    viewport: { width: 1280, height: 720 },
    deviceScaleFactor: 1.5,
    colorScheme: theme,
    recordVideo: { dir: tmp, size: { width: 1920, height: 1080 } },
  });
  const page = await context.newPage();
  const origin = new URL(url).origin;
  try {
    // The tokened address opens the session; the token stays in the URL fragment, which the
    // recording does not show (there is no address bar in a Playwright video).
    await page.goto(url);
    await page.waitForLoadState('networkidle');
    await pause(page);
    await scene(page, origin);
    // The last screen as a still, to check the result without playing the video.
    await page.screenshot({ path: path.join(outDir, `${name}-end.png`) });
  } finally {
    await context.close();
  }
  const target = path.join(outDir, `${name}.webm`);
  await page.video().saveAs(target);
  fs.rmSync(tmp, { recursive: true, force: true });
  return target;
}

async function main() {
  if (!fs.existsSync(path.join(sandbox, '.skillhub-demo-sandbox')) || !fs.existsSync(path.join(sandbox, 'home/skill-hub'))) {
    throw new Error(`no demo hub in ${sandbox}: run video/demos/happy-path.sh first`);
  }
  if (!(await portIsFree(port))) {
    throw new Error(`port ${port} is in use: stop what listens there (lsof -i :${port}) or set DEMO_WEB_PORT`);
  }
  const { chromium } = loadPlaywright();
  fs.mkdirSync(outDir, { recursive: true });

  const server = startServer();
  const onSignal = () => server.stop().then(() => process.exit(130));
  process.once('SIGINT', onSignal);
  process.once('SIGTERM', onSignal);
  let browser;
  try {
    const url = await server.ready;
    browser = await chromium.launch({ headless: process.env.DEMO_HEADED !== '1', slowMo });
    const edit = await record(browser, url, 'web-edit', sceneEdit);
    console.log(`Recorded ${edit}`);
    const approve = await record(browser, url, 'web-approve', sceneApprove);
    console.log(`Recorded ${approve}`);
  } finally {
    await browser?.close();
    await server.stop();
  }
}

main().catch((err) => fail(err.message));
