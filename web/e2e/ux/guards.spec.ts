import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { startServer, type RunningServer } from '../support/server';
import { FLOWS } from './flows';
import { collectMetrics, openFlow } from './page-checks';
import { prepareHub, type UxHub } from './seed-states';

// Regression guards for the defects the UX evaluation found (docs/design/web-ux-scorecard.md).
// They run on every flow in flows.ts against a self-contained fixture hub, so a fix that
// is undone, or a new screen with the same fault, turns the default E2E run red.

const PHONE = { width: 390, height: 844 };
const DESKTOP = { width: 1280, height: 800 };

// Addresses that must end on a plain page, never on an error the user cannot act on.
const ERROR_ROUTES = [
  '/inbox',
  '/runs',
  '/insights',
  '/no/such/page',
  '/skills/skill-that-does-not-exist',
  '/skills/ux-active?tab=not-a-tab',
  '/sources/distill?source=source-that-does-not-exist',
];

// Text an API or a program wrote for itself: transport and envelope words, JavaScript
// leftovers. Any of it in the visible page means a raw failure reached the user.
const RAW_ERROR = /Unknown API path|Failed to (?:load|fetch)|Request failed|HTTP \d{3}|internal_error|validation_failed|\[object Object\]|\bundefined\b|\bNaN\b|TypeError|\{"/;

// Words for features the product no longer has (insights, runs, the inbox). A screen that names
// one sends the user looking for something that is not there.
const RETIRED_FEATURE = /\binsights?\b|\binbox\b|Recent runs|Open a run|curation_run/i;

// A snake_case word in prose is an internal id shown as if it were a label ("pending_insights").
// Commands, code, field values and the "Exact file changes" and "Technical details" sections (a literal patch, ids, digests)
// are exempt: they are meant to be read literally.
const INTERNAL_ID = /\b[a-z][a-z0-9]*(?:_[a-z0-9]+)+\b/;

// Runs in the page: the text a person can read, without code blocks, inputs and scripts.
function visibleProse(): string {
  const skip = 'code, pre, input, textarea, script, style, noscript, select, option';
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  const parts: string[] = [];
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const el = node.parentElement;
    if (!el || el.closest(skip)) {
      continue;
    }
    const heading = el.closest('details')?.querySelector('summary')?.textContent?.trim();
    if (heading === 'Technical details' || heading === 'Exact file changes') {
      continue;
    }
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0 || getComputedStyle(el).visibility === 'hidden') {
      continue;
    }
    parts.push(node.textContent ?? '');
  }
  return parts.join('\n').replace(/[ \t]+/g, ' ');
}

async function expectNoRawText(page: Page, where: string) {
  const prose = await page.evaluate(visibleProse);
  expect(prose, `${where}: raw error or program text on the page`).not.toMatch(RAW_ERROR);
  expect(prose, `${where}: names a feature the product no longer has`).not.toMatch(RETIRED_FEATURE);
  const internal = prose.match(INTERNAL_ID);
  const ctx = internal ? prose.slice(Math.max(0, internal.index! - 120), internal.index! + 80) : '';
  expect(internal, `${where}: internal identifier shown as a label: ${internal?.[0]} in: ${ctx}`).toBeNull();
}

test.describe('UX regression guards', () => {
  let hub: UxHub;
  let server: RunningServer;

  test.beforeAll(async () => {
    hub = prepareHub('');
    server = await startServer({ workspace: hub.ws, env: hub.env });
  });

  test.afterAll(async () => {
    await server?.stop();
    hub?.cleanup();
  });

  test.beforeEach(async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    // The tokened address stores the session; every later direct visit needs it first.
    await page.goto(server.url);
  });

  test('no raw API error, internal id or retired feature name shows on any flow or error route', async ({ page }) => {
    test.setTimeout(300_000);
    for (const size of [DESKTOP, PHONE]) {
      for (const flow of FLOWS) {
        await openFlow(page, server, hub, flow, size.width, size.height);
        await expectNoRawText(page, `${flow.id} at ${size.width}px`);
      }
    }
    for (const route of ERROR_ROUTES) {
      await page.goto(`${server.origin}${route}`);
      await page.waitForLoadState('networkidle');
      await page.locator('h1, [class*="heading"]').first().waitFor({ timeout: 10000 }).catch(() => undefined);
      await expectNoRawText(page, route);
      await expect(page.getByRole('link', { name: 'Skills' }).first(), `${route} offers a way out`).toBeVisible();
    }
  });

  test('a taken skill id is refused with its reason, not a generic rule message', async ({ page }) => {
    await page.goto(`${server.origin}/skills/create`);
    await page.locator('#create-id').fill('ux-active');
    await page.locator('#create-name').fill('Taken');
    await page.locator('#create-desc').fill('The id already exists.');
    await page.getByRole('button', { name: 'Preview draft' }).click();
    const banner = page.locator('.fg-banner--danger').first();
    await expect(banner).toBeVisible();
    await expect(banner).toContainText('already exists');
    expect(await banner.textContent()).not.toMatch(/conflicts with validation rules/);
    await expectNoRawText(page, 'create with a taken id');
  });

  test('no flow scrolls sideways or clips text at 390 px', async ({ page }) => {
    test.setTimeout(300_000);
    for (const flow of FLOWS) {
      await openFlow(page, server, hub, flow, PHONE.width, PHONE.height);
      const metrics = await page.evaluate(collectMetrics);
      expect(metrics.overflowX, `${flow.id} scrolls sideways at ${PHONE.width}px`).toBe(false);
      expect(metrics.clipped, `${flow.id} has clipped or off-screen text at ${PHONE.width}px`).toEqual([]);
    }
  });

  test('every copy button copies the whole command it sits next to', async ({ page }) => {
    test.setTimeout(300_000);
    let checked = 0;
    for (const size of [DESKTOP, PHONE]) {
      for (const flow of FLOWS) {
        await openFlow(page, server, hub, flow, size.width, size.height);
        // An open dialog covers the page behind it, so only its own commands can be clicked.
        const scope = (await page.getByRole('dialog').count()) > 0 ? page.getByRole('dialog') : page;
        const buttons = scope.locator('code + button');
        const count = await buttons.count();
        for (let i = 0; i < count; i++) {
          const button = buttons.nth(i);
          const code = button.locator('xpath=preceding-sibling::code[1]');
          const shown = ((await code.textContent()) ?? '').trim();
          const hidden = await code.evaluate((el) => {
            const cs = getComputedStyle(el);
            return cs.overflowX !== 'visible' && el.scrollWidth > el.clientWidth + 1;
          });
          expect(hidden, `${flow.id} at ${size.width}px: command is cut off: ${shown}`).toBe(false);

          await button.scrollIntoViewIfNeeded();
          await button.click();
          const copied = await page.evaluate(() => navigator.clipboard.readText());
          expect(copied, `${flow.id} at ${size.width}px: copy button does not copy the whole command`).toBe(shown);
          checked++;
        }
      }
    }
    // The flows reach commands on Home, the trust card, the Runtime tab and the upstream list;
    // finding none would mean the guard stopped looking, not that the commands are fine.
    expect(checked, 'copy buttons found by the guard').toBeGreaterThanOrEqual(6);
  });
});
