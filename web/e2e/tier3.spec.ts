import fs from 'node:fs';
import { expect, test } from '@playwright/test';
import { seedDistillWorkspace } from './support/seed-workspace';
import { startServer, type RunningServer } from './support/server';

// Monthly and rare flows: Sources, the distill handoff, the old inbox address and the nav.
test.describe('Tier 3 occasional flows', () => {
  let server: RunningServer;
  let seededWorkspace: string | undefined;

  test.afterEach(async () => {
    await server?.stop();
    if (seededWorkspace) {
      fs.rmSync(seededWorkspace, { recursive: true, force: true });
      seededWorkspace = undefined;
    }
  });

  test('every nav item opens a page, and the old inbox address is not a raw error', async ({ page }) => {
    server = await startServer();
    await page.goto(server.url);

    const nav = page.getByRole('navigation', { name: 'Primary' });
    await expect(nav.getByRole('link')).toHaveCount(3);
    for (const name of ['Skills', 'Sources', 'Home']) {
      await nav.getByRole('link', { name }).click();
      await expect(page.getByText('Page not found.')).toHaveCount(0);
      await expect(page.getByText(/Unknown API path|Failed to load/)).toHaveCount(0);
    }

    await page.goto(`${server.origin}/inbox`);
    await expect(page.getByText('Page not found.')).toBeVisible();
    await expect(page.getByText(/Unknown API path/)).toHaveCount(0);
    await expect(page.getByRole('main').getByRole('link', { name: 'Sources' })).toBeVisible();
  });

  test('sources explain what a source is when there are none', async ({ page }) => {
    server = await startServer();
    await page.goto(server.url);
    await page.goto(`${server.origin}/sources`);
    await expect(page.getByText(/A source is a repository your skills track or learn from/)).toBeVisible();
    await expect(page.getByText(/\bruns?\b/i)).toHaveCount(0);
  });

  test('distill handoff with nothing selected offers the ready sources, then a brief in the distill-lab flow', async ({
    page,
  }) => {
    test.setTimeout(120_000);
    const seed = seedDistillWorkspace();
    seededWorkspace = seed.ws;
    server = await startServer({ workspace: seed.ws });
    await page.goto(server.url);
    await page.goto(`${server.origin}/sources`);

    // The button works with nothing ticked: it opens the picker instead of a dead end.
    await page.getByRole('link', { name: 'Distill with Curator Agent' }).click();
    await expect(page).toHaveURL(/\/sources\/distill$/);
    await expect(page.getByText('Sources ready to distill')).toBeVisible();
    await page.getByRole('checkbox').first().check();
    await page.getByRole('link', { name: 'Continue with selected sources' }).click();

    await expect(page).toHaveURL(/\/sources\/distill\?source=/);
    const brief = await page.locator('pre').textContent();
    expect(brief).toContain('Use the distill-lab skill');
    expect(brief).toContain('consumer-review');
    expect(brief).not.toContain('curation_');
  });

  test('sources and distill pages fit a phone without sideways scrolling', async ({ page }) => {
    test.setTimeout(120_000);
    const seed = seedDistillWorkspace();
    seededWorkspace = seed.ws;
    server = await startServer({ workspace: seed.ws });
    await page.setViewportSize({ width: 390, height: 800 });
    await page.goto(server.url);

    for (const route of ['/sources', '/sources/distill']) {
      await page.goto(`${server.origin}${route}`);
      await expect(page.getByRole('heading', { level: 1 }).first()).toBeVisible();
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      );
      expect(overflow, `${route} scrolls sideways`).toBeLessThanOrEqual(0);
    }

    // Every action in a source row stays inside the card.
    await page.goto(`${server.origin}/sources`);
    const unwatch = page.getByRole('button', { name: 'Unwatch' }).first();
    const box = await unwatch.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.x + box!.width).toBeLessThanOrEqual(390);
  });
});
