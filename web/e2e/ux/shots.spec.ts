import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { startServer, type RunningServer } from '../support/server';
import { DEMO_DATE, DEMO_SKILLS, DEMO_SOURCE, prepareDemoHub, type DemoHub } from './demo-hub';
import { externalEdit } from './seed-states';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Screenshot capture for the README and the user guide, run by `make web-shots`.
// Like the ux capture it writes files, so the default E2E run skips it: without
// SHOTS_CAPTURE the whole suite is skipped.
const ENABLED = Boolean(process.env.SHOTS_CAPTURE);
const OUT_DIR = process.env.SHOTS_OUT_DIR
  ? path.resolve(process.env.SHOTS_OUT_DIR)
  : path.resolve(__dirname, '../../../docs/images/web');

const DESKTOP = { width: 1280, height: 800 };
const MOBILE = { width: 390, height: 844 };

// A shot is one image. `fit: 'content'` trims or grows the height to the page content
// (never above maxHeight); `fit: 'viewport'` keeps the fixed viewport, which is what a
// dialog needs because it is centred in the window.
interface Shot {
  file: string;
  route: string;
  state: string;
  caption: string;
  alt: string;
  size?: { width: number; height: number };
  theme?: 'light' | 'dark';
  fit?: 'content' | 'viewport';
  maxHeight?: number;
  // Runs after the page loaded; opens a dialog or fills a form.
  prepare?: (page: Page, hub: DemoHub) => Promise<void>;
  // Capture after the others, because it changes the hub.
  last?: boolean;
}

// Shots in the plan that cannot be made on an offline demo hub. They are listed in the
// manifest so a reader knows the guide describes them in text only.
const SKIPPED = [
  {
    file: 'upstream-updates',
    reason: 'An upstream update is only produced by a real network check against GitHub; the state cannot be made offline without faking it.',
  },
  {
    file: 'add-from-github-review',
    reason: 'The review step appears only after discovery has fetched a repository over the network.',
  },
];

const skillPath = (id: string, tab?: string) => `/skills/${id}${tab ? `?tab=${tab}` : ''}`;
const waitForDialog = (page: Page) => page.getByRole('dialog').waitFor({ timeout: 10000 });

const SHOTS: Shot[] = [
  {
    file: 'home',
    route: '/',
    state: 'Home with the next action and the status of the hub',
    caption: 'Home: what needs attention, and the one next step to take.',
    alt: 'The Skill Hub home screen showing the status of the hub and the next action.',
  },
  {
    file: 'skills-list',
    route: '/skills',
    state: 'List with draft, active and deprecated skills',
    caption: 'Skills: every skill with its state, and filters to find one.',
    alt: 'The skills list with a state filter and skills that are draft, active or deprecated.',
  },
  {
    file: 'skill-review',
    route: skillPath(DEMO_SKILLS.active),
    state: 'Review tab of an active skill',
    caption: 'Review: whether agents can use this skill, and what to check first.',
    alt: 'The Review tab of an active skill showing its content check and readiness.',
  },
  {
    file: 'skill-review-draft',
    route: skillPath(DEMO_SKILLS.draft),
    state: 'Draft that is missing activation requirements',
    caption: 'A draft that cannot be activated yet, with links to the fields to fix.',
    alt: 'The Review tab of a draft skill listing the missing fields with links to fix each one.',
  },
  {
    file: 'skill-activate-confirm',
    route: skillPath(DEMO_SKILLS.readyDraft),
    state: 'Confirmation before activating a draft',
    caption: 'Activate: the confirmation that shows what changes before anything is saved.',
    alt: 'A confirmation dialog asking to activate a skill, listing what will change.',
    fit: 'viewport',
    prepare: async (page) => {
      await page.getByRole('button', { name: /^Activate/ }).first().click();
      await waitForDialog(page);
    },
  },
  {
    file: 'editor-preview',
    route: skillPath(DEMO_SKILLS.active, 'editor'),
    state: 'Editor tab with a preview of the change',
    caption: 'Editor: preview the exact change before it is saved.',
    alt: 'The editor tab with a dialog that previews the changed description as a diff.',
    fit: 'viewport',
    prepare: async (page) => {
      await page
        .locator('#edit-desc')
        .fill('Make a release safe: version, changelog, tests, rollback plan and who to tell.');
      await page.getByRole('button', { name: 'Preview changes' }).click();
      await waitForDialog(page);
    },
  },
  {
    file: 'editor-conflict',
    route: skillPath(DEMO_SKILLS.deprecated, 'editor'),
    state: 'Conflict drawer after an edit made elsewhere',
    caption: 'Conflict: the skill changed somewhere else while the form was open.',
    alt: 'The conflict drawer explaining that the skill changed elsewhere and offering ways to continue.',
    fit: 'viewport',
    last: true,
    prepare: async (page, hub) => {
      await page.locator('#edit-desc').waitFor({ timeout: 10000 });
      externalEdit(hub, DEMO_SKILLS.deprecated, 'Summarize a meeting into decisions, owners and dates.');
      await page.locator('#edit-desc').fill('Summarize a meeting into decisions and action items, with owners.');
      await page.getByRole('button', { name: 'Preview changes' }).click();
      await waitForDialog(page);
    },
  },
  {
    file: 'create-skill',
    route: '/skills/create',
    state: 'Create form with the field hints',
    caption: 'Create a skill: the form tells you which fields are needed to activate it later.',
    alt: 'The create skill form with hints under the fields.',
    // The action bar sticks to the bottom of the window and covers a field unless the
    // window is as tall as the form.
    maxHeight: 1500,
  },
  {
    file: 'add-from-github-discover',
    route: '/skills/add',
    state: 'Add from GitHub, first step, with a placeholder address',
    caption: 'Add from GitHub: paste a repository address to find skills in it.',
    alt: 'The add from GitHub form with a repository address filled in.',
    prepare: async (page) => {
      await page.locator('#gh-url').fill('https://github.com/acme/agent-skills');
    },
  },
  {
    file: 'sources',
    route: '/sources',
    state: 'Sources list with one source and its check result',
    caption: 'Sources: where skills learn from, and when each was last checked.',
    alt: 'The sources list with one source and the result of its last check.',
  },
  {
    file: 'distill-handoff',
    route: `/sources/distill?source=${DEMO_SOURCE}`,
    state: 'Distill hand-off brief with a selected source',
    caption: 'Distill: copy the brief and give it to your curator agent.',
    alt: 'The distill hand-off screen with a brief ready to copy for the curator agent.',
  },
  {
    file: 'mobile-skills',
    route: '/skills',
    state: 'Skills list on a phone-width screen',
    caption: 'The same list on a 390 px wide screen.',
    alt: 'The skills list on a narrow phone screen.',
    size: MOBILE,
    maxHeight: 860,
  },
  {
    file: 'dark-skills-list',
    route: '/skills',
    state: 'Skills list in the dark theme',
    caption: 'The same list in the dark theme.',
    alt: 'The skills list in the dark theme.',
    theme: 'dark',
  },
];

// Values that must never reach an image: the session token, anything that looks like a
// token, a home path, a host name or an email. Returns what was found, never the value.
async function findPrivateValues(page: Page, secrets: string[]): Promise<string[]> {
  return page.evaluate((known) => {
    const parts = [document.body.innerText];
    for (const el of document.querySelectorAll('input, textarea')) {
      parts.push((el as HTMLInputElement).value, el.getAttribute('placeholder') ?? '');
    }
    const text = parts.join('\n');
    const found: string[] = [];
    if (/token/i.test(text)) found.push('the word "token"');
    if (text.includes('/home/') || /\/Users\/|C:\\Users\\/i.test(text)) found.push('a home path');
    if (text.includes('@')) found.push('an "@" sign (email)');
    if (/\b[0-9a-f]{32,}\b/i.test(text)) found.push('a long hex string');
    if (/127\.0\.0\.1|localhost/.test(text)) found.push('a local host name');
    for (const secret of known) {
      if (secret && text.includes(secret)) found.push('a known private value');
    }
    return found;
  }, secrets);
}

// Replaces the throwaway workspace path with the path a user would see, in text and
// in field values, before the shot.
// Dates the hub wrote with the real clock (a source check, for example) are shown as the
// demo date, so a rerun on another day gives the same image.
async function showFriendlyPaths(page: Page, paths: string[], demoDay: string) {
  await page.evaluate(([list, day]) => {
    const friendly = (s: string) =>
      (list as string[])
        .reduce((acc, p) => acc.split(p).join('~/skill-hub'), s)
        .replace(/\b\d{4}-\d{2}-\d{2}\b/g, day as string);
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const next = friendly(node.nodeValue ?? '');
      if (next !== node.nodeValue) node.nodeValue = next;
    }
    for (const el of document.querySelectorAll('input, textarea')) {
      const input = el as HTMLInputElement;
      const next = friendly(input.value);
      if (next !== input.value) input.value = next;
    }
  }, [paths, demoDay] as const);
}

// Bottom edge of the lowest element inside the page area. The area itself stretches to
// the window, so it cannot be used to tell how much content there is.
async function contentHeight(page: Page): Promise<number> {
  return page.evaluate(() => {
    let bottom = 0;
    for (const el of document.querySelectorAll('#main *')) {
      const r = el.getBoundingClientRect();
      if (r.width > 0 && r.height > 0 && getComputedStyle(el).position !== 'fixed') {
        bottom = Math.max(bottom, r.bottom);
      }
    }
    return Math.ceil(bottom);
  });
}

test.describe('Screenshots for README and user guide', () => {
  test.skip(!ENABLED, 'capture only: run make web-shots');

  let hub: DemoHub;
  let server: RunningServer;
  let token: string;

  test.beforeAll(async () => {
    fs.mkdirSync(OUT_DIR, { recursive: true });
    for (const name of fs.readdirSync(OUT_DIR)) {
      if (name.endsWith('.png') || name === 'manifest.json') {
        fs.rmSync(path.join(OUT_DIR, name));
      }
    }
    hub = prepareDemoHub();
    server = await startServer({ workspace: hub.ws, env: hub.env });
    token = new URL(server.url).hash.replace('#token=', '');
  });

  test.afterAll(async () => {
    await server?.stop();
    hub?.cleanup();
  });

  test('the private-value check fails when a token is on the page', async ({ page }) => {
    await page.goto(server.url);
    await page.locator('h1').first().waitFor({ timeout: 10000 });
    expect(await findPrivateValues(page, [token])).toEqual([]);

    await page.evaluate((value) => {
      const probe = document.createElement('p');
      probe.id = 'planted-token';
      probe.textContent = `session ${value}`;
      document.body.appendChild(probe);
    }, token);
    const found = await findPrivateValues(page, [token]);
    expect(found.length).toBeGreaterThan(0);
  });

  test('capture the shot list', async ({ browser }) => {
    test.setTimeout(300_000);
    const manifest: Array<Record<string, unknown>> = [];
    const privatePaths = [hub.ws, hub.root, fs.realpathSync(hub.root), os.tmpdir()];
    const ordered = [...SHOTS.filter((s) => !s.last), ...SHOTS.filter((s) => s.last)];

    for (const shot of ordered) {
      const size = shot.size ?? DESKTOP;
      const theme = shot.theme ?? 'light';
      const context = await browser.newContext({
        viewport: size,
        deviceScaleFactor: 2,
        colorScheme: theme,
        reducedMotion: 'reduce',
        locale: 'en-US',
        timezoneId: 'UTC',
      });
      const page = await context.newPage();
      try {
        // Dates shown by the page use a fixed clock; the hub's Git history is fixed too.
        await page.clock.setFixedTime(new Date(DEMO_DATE));
        await page.goto(server.url);
        await page.locator('h1').first().waitFor({ timeout: 10000 });
        await page.goto(`${server.origin}${shot.route}`);
        await page.waitForLoadState('networkidle');
        await page.locator('h1').first().waitFor({ timeout: 10000 });
        if (shot.prepare) {
          await shot.prepare(page, hub);
        }
        await page.evaluate(() => document.fonts.ready);
        await showFriendlyPaths(page, privatePaths, DEMO_DATE.slice(0, 10));
        await page.waitForTimeout(300);

        const found = await findPrivateValues(page, [token, ...privatePaths]);
        if (found.length > 0) {
          throw new Error(`${shot.file}: private values on the page: ${found.join(', ')}`);
        }

        let height = size.height;
        if ((shot.fit ?? 'content') === 'content') {
          // Two passes: a sticky action bar and the page layout both depend on the window
          // height, so the first size is only an estimate.
          for (let pass = 0; pass < 2; pass += 1) {
            const wanted = (await contentHeight(page)) + 24;
            height = Math.min(Math.max(wanted, 480), shot.maxHeight ?? 1000);
            await page.setViewportSize({ width: size.width, height });
            await page.waitForTimeout(150);
          }
        }
        const file = `${shot.file}.png`;
        // scale 'css' renders at twice the density and hands back CSS-pixel width, so
        // text stays sharp and the file stays small.
        await page.screenshot({ path: path.join(OUT_DIR, file), animations: 'disabled', scale: 'css' });
        manifest.push({
          file,
          route: shot.route.replace(/\?.*$/, '') + (shot.route.includes('?') ? '?…' : ''),
          state: shot.state,
          width: size.width,
          height,
          theme,
          caption: shot.caption,
          alt: shot.alt,
        });
      } finally {
        await context.close();
      }
    }

    const order = new Map(SHOTS.map((s, i) => [`${s.file}.png`, i]));
    manifest.sort((a, b) => (order.get(String(a.file)) ?? 0) - (order.get(String(b.file)) ?? 0));
    fs.writeFileSync(path.join(OUT_DIR, 'manifest.json'), `${JSON.stringify({ images: manifest, skipped: SKIPPED }, null, 2)}\n`);
    console.log(`Screenshots written to ${OUT_DIR}`);
  });
});
