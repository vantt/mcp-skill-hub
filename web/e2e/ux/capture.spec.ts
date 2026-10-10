import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import AxeBuilder from '@axe-core/playwright';
import { test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { startServer, type RunningServer } from '../support/server';
import { FLOWS, type Flow } from './flows';
import { prepareHub, type UxHub } from './seed-states';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Evaluation capture, not a pass/fail suite: it saves what each flow looks like and
// never asserts on looks. A person (or an agent reading the screenshots) scores the
// output with docs/design/web-ux-rubric.md.
const OUT_DIR = process.env.UX_OUT_DIR
  ? path.resolve(process.env.UX_OUT_DIR)
  : path.resolve(__dirname, '../../test-results/ux');

const WIDTHS = [
  { name: '1280', width: 1280, height: 800 },
  { name: '390', width: 390, height: 844 },
];

interface FlowMetrics {
  overflowX: boolean;
  viewportHeight: number;
  pageHeight: number;
  primaryActions: Array<{ label: string; belowFold: boolean; disabled: boolean }>;
  clipped: Array<{ tag: string; text: string }>;
}

// Runs in the page. Reports layout facts that a screenshot makes hard to judge.
function collectMetrics(): FlowMetrics {
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const main = document.getElementById('main');
  const visible = (el: Element) => {
    const r = el.getBoundingClientRect();
    const cs = getComputedStyle(el);
    return r.width > 0 && r.height > 0 && cs.visibility !== 'hidden' && cs.display !== 'none';
  };
  const text = (el: Element) => (el.textContent ?? '').trim().replace(/\s+/g, ' ').slice(0, 80);

  const primaryActions = [...document.querySelectorAll('.fg-btn--primary')]
    .filter(visible)
    .map((el) => ({
      label: text(el),
      belowFold: el.getBoundingClientRect().bottom > vh,
      disabled: (el as HTMLButtonElement).disabled === true,
    }));

  const clipped: Array<{ tag: string; text: string }> = [];
  for (const el of document.querySelectorAll('pre, code, button, a, td, th, h1, h2, h3, label, p, li, span')) {
    if (clipped.length >= 25 || !visible(el)) {
      continue;
    }
    const cs = getComputedStyle(el);
    const clips = cs.overflowX !== 'visible' || cs.textOverflow === 'ellipsis';
    const hiddenText = clips && el.clientWidth > 0 && el.scrollWidth > el.clientWidth + 1;
    const offscreen = el.getBoundingClientRect().right > vw + 1 && text(el) !== '';
    if (hiddenText || offscreen) {
      clipped.push({ tag: el.tagName.toLowerCase(), text: text(el) });
    }
  }

  return {
    overflowX: document.documentElement.scrollWidth > vw,
    viewportHeight: vh,
    pageHeight: (main?.scrollHeight ?? document.documentElement.scrollHeight) + (main?.getBoundingClientRect().top ?? 0),
    primaryActions,
    clipped,
  };
}

// The app scrolls inside #main, so a plain full-page shot only shows one viewport.
// Grow the viewport to the content height for the shot, then restore it.
async function screenshotWholePage(page: Page, file: string, width: number, height: number) {
  const contentHeight = await page.evaluate(() => {
    const main = document.getElementById('main');
    return Math.ceil((main?.scrollHeight ?? 0) + (main?.getBoundingClientRect().top ?? 0));
  });
  const tall = Math.min(Math.max(height, contentHeight), 8000);
  await page.setViewportSize({ width, height: tall });
  await page.waitForTimeout(100);
  await page.screenshot({ path: file, fullPage: true, animations: 'disabled' });
  await page.setViewportSize({ width, height });
}

async function captureFlow(page: Page, server: RunningServer, hub: UxHub, flow: Flow) {
  const index: Record<string, unknown> = { id: flow.id, row: flow.row, tier: flow.tier, title: flow.title, route: flow.route, task: flow.task, widths: {} };
  for (const w of WIDTHS) {
    const entry: Record<string, unknown> = {};
    (index.widths as Record<string, unknown>)[w.name] = entry;
    try {
      await page.setViewportSize({ width: w.width, height: w.height });
      await page.goto(`${server.origin}${flow.route}`);
      await page.waitForLoadState('networkidle');
      await page.locator('h1').first().waitFor({ timeout: 10000 }).catch(() => undefined);
      if (flow.prepare) {
        await flow.prepare({ page, hub });
        await page.waitForTimeout(200);
      }

      const metrics = await page.evaluate(collectMetrics);
      fs.writeFileSync(path.join(OUT_DIR, `${flow.id}-${w.name}.metrics.json`), JSON.stringify(metrics, null, 2));

      const axe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze();
      const violations = axe.violations.map((v) => ({
        id: v.id,
        impact: v.impact,
        help: v.help,
        nodes: v.nodes.map((n) => n.target.join(' ')).slice(0, 10),
      }));
      fs.writeFileSync(path.join(OUT_DIR, `${flow.id}-${w.name}.axe.json`), JSON.stringify(violations, null, 2));
      entry.axeSeriousOrCritical = violations.filter((v) => v.impact === 'serious' || v.impact === 'critical').length;
      entry.overflowX = metrics.overflowX;
      entry.clipped = metrics.clipped.length;

      if (w.name === WIDTHS[0]?.name) {
        const body = await page.evaluate(() => document.body.innerText);
        fs.writeFileSync(path.join(OUT_DIR, `${flow.id}.txt`), `${body}\n`);
      }
      await screenshotWholePage(page, path.join(OUT_DIR, `${flow.id}-${w.name}.png`), w.width, w.height);
    } catch (err) {
      entry.error = err instanceof Error ? err.message.split('\n')[0] : String(err);
    }
  }
  return index;
}

test.describe('UX evaluation capture', () => {
  let hub: UxHub;
  let server: RunningServer;

  test.beforeAll(async () => {
    fs.rmSync(OUT_DIR, { recursive: true, force: true });
    fs.mkdirSync(OUT_DIR, { recursive: true });
    hub = prepareHub();
    // startServer spawns the binary with the current process environment, so swap in
    // the isolated one just for the spawn. The browser is launched after it is restored.
    const saved = { ...process.env };
    Object.assign(process.env, hub.env);
    try {
      server = await startServer({ workspace: hub.ws });
    } finally {
      for (const key of Object.keys(process.env)) {
        if (!(key in saved)) {
          delete process.env[key];
        }
      }
      Object.assign(process.env, saved);
    }
  });

  test.afterAll(async () => {
    await server?.stop();
    hub?.cleanup();
  });

  test('capture every flow at 1280 and 390 px', async ({ page }) => {
    test.setTimeout(900_000);
    await page.goto(server.url);

    const flows: unknown[] = [];
    for (const flow of FLOWS) {
      flows.push(await captureFlow(page, server, hub, flow));
    }
    fs.writeFileSync(
      path.join(OUT_DIR, 'index.json'),
      JSON.stringify({ hub: hub.origin, skipped: hub.skipped, flows }, null, 2),
    );
    console.log(`UX capture written to ${OUT_DIR}`);
  });
});
