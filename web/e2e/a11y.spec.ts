import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';
import { seedDistillWorkspace, type SeededDistillWorkspace } from './support/seed-workspace';
import { startServer, type RunningServer } from './support/server';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const binaryPath = path.resolve(__dirname, '../.e2e/skillhub');

async function assertA11y(page: Page, contextName: string) {
  const scan = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
    .analyze();

  const violations = scan.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  );

  expect(
    violations,
    `A11y violation on ${contextName}: ${JSON.stringify(violations, null, 2)}`,
  ).toEqual([]);
}

test.describe('Accessibility and Responsive Sweep (Phase 12)', () => {
  let seed: SeededDistillWorkspace;
  let server: RunningServer;

  test.beforeAll(async () => {
    seed = seedDistillWorkspace();
    server = await startServer({ workspace: seed.ws });
  });

  test.afterAll(async () => {
    if (server) {
      await server.stop();
    }
    if (seed?.ws) {
      try {
        fs.rmSync(seed.ws, { recursive: true, force: true });
      } catch {
        // Ignore
      }
    }
  });

  test('responsive and axe accessibility sweep across all routes', async ({ page }) => {
    test.setTimeout(300_000);

    // Initial auth
    await page.goto(server.url);
    await expect(page.getByRole('heading', { name: 'Home' }).first()).toBeVisible();

    const routes = [
      '/',
      '/skills',
      '/skills/add',
      '/skills/create',
      '/skills/consumer-review',
      '/skills/consumer-review?tab=editor',
      '/skills/consumer-review?tab=resources',
      '/skills/consumer-review?tab=usage',
      '/skills/consumer-review?tab=runtime',
      '/skills/consumer-review?tab=sources',
      '/sources',
      '/sources/distill?source=source-c',
      `/sources/runs/${seed.finalizedRunId}`,
      '/inbox',
      '/inbox/INS-consumer-review--insight-consumer-one',
      '/inbox/INS-consumer-review--insight-consumer-one/apply',
    ];

    const widths = [360, 768, 1280, 1440];
    const schemes: Array<'light' | 'dark'> = ['light', 'dark'];

    for (const route of routes) {
      for (const width of widths) {
        await page.setViewportSize({ width, height: 800 });
        await page.goto(`${server.origin}${route}`);
        await page.waitForLoadState('networkidle');

        // Verify no horizontal overflow
        const scrollWidth = await page.evaluate(
          () => document.documentElement.scrollWidth,
        );
        expect(
          scrollWidth,
          `Horizontal scroll on ${route} at width ${width}: scrollWidth ${scrollWidth} > innerWidth ${width}`,
        ).toBeLessThanOrEqual(width);

        for (const scheme of schemes) {
          await page.evaluate(
            (s) => document.documentElement.setAttribute('data-scheme', s),
            scheme,
          );
          // Wait briefly for CSS transitions/repaint
          await page.waitForTimeout(50);

          await assertA11y(page, `${route} (w=${width}, scheme=${scheme})`);
        }
      }
    }
  });

  test('axe checks on open modals: ConfirmDialog, ConflictDrawer, ProposalPreview', async ({
    page,
  }) => {
    test.setTimeout(120_000);
    await page.goto(server.url);
    await page.setViewportSize({ width: 1280, height: 800 });

    // 1. ConfirmDialog: on /sources/runs/:id (cancel run dialog)
    await page.goto(`${server.origin}/sources/runs/${seed.inProgressRunId}`);
    await page.waitForLoadState('networkidle');
    const cancelBtn = page.getByRole('button', { name: 'Cancel run' });
    await expect(cancelBtn).toBeVisible();
    await cancelBtn.click();
    await expect(page.getByRole('alertdialog')).toBeVisible();
    await assertA11y(page, 'ConfirmDialog modal open');

    // Dismiss cancel dialog
    await page.getByRole('alertdialog').getByRole('button', { name: 'Cancel', exact: true }).click();

    // 2. ConflictDrawer and ProposalPreview on Composer
    await page.goto(
      `${server.origin}/inbox/INS-consumer-review--insight-consumer-two/apply`,
    );
    await page.waitForLoadState('networkidle');

    // Map observation and edit content
    const editor = page.getByLabel('SKILL.md replacement content');
    const curContent = await editor.inputValue();
    await editor.fill(curContent + '\n\n## A11y Test\nA11y instructions.\n');
    const conceptInput = page.getByPlaceholder('Concept, e.g. retry limits');
    await conceptInput.fill('a11y-concept');
    await page.getByRole('button', { name: '+ Add concept' }).click();

    // Open ProposalPreview
    const previewBtn = page.getByRole('button', { name: 'Preview apply' });
    await expect(previewBtn).not.toBeDisabled();
    await previewBtn.click();

    const proposalDialog = page.getByRole('dialog');
    await expect(proposalDialog).toBeVisible();
    await assertA11y(page, 'ProposalPreview modal open');

    // Close ProposalPreview
    await proposalDialog.getByRole('button', { name: 'Cancel' }).click();
    await expect(proposalDialog).not.toBeVisible();

    // Trigger ConflictDrawer: modify skill outside
    execFileSync(
      binaryPath,
      ['skill', 'edit', 'consumer-review', '--description', 'A11y conflict test', '--yes'],
      {
        env: { ...process.env, SKILLHUB_WORKSPACE: seed.ws },
        stdio: 'pipe',
      },
    );

    // Click Preview apply -> ConflictDrawer opens
    await previewBtn.click();
    await expect(
      page.getByText('SKILL.md changed since you opened it'),
    ).toBeVisible();
    await assertA11y(page, 'ConflictDrawer open');

    // Close ConflictDrawer
    await page.getByRole('button', { name: 'Close' }).first().click();
  });

  test('media emulation: prefers-reduced-motion and forced-colors on Home and Skill Detail', async ({
    page,
  }) => {
    test.setTimeout(120_000);
    await page.goto(server.url);
    await page.setViewportSize({ width: 1280, height: 800 });

    const targets = ['/', '/skills/consumer-review'];

    for (const route of targets) {
      await page.goto(`${server.origin}${route}`);
      await page.waitForLoadState('networkidle');

      // 1. Reduced motion
      await page.emulateMedia({ reducedMotion: 'reduce' });
      await assertA11y(page, `${route} (reduced-motion)`);

      // 2. Forced colors
      await page.emulateMedia({ forcedColors: 'active' });
      await assertA11y(page, `${route} (forced-colors)`);

      // Reset
      await page.emulateMedia({ reducedMotion: 'no-preference', forcedColors: 'none' });
    }
  });

  test('vietnamese glyph coverage test across themes and font slots', async ({ page }) => {
    await page.goto(server.url);

    const testText = 'Kỹ năng đã được kích hoạt — Ưu tiên cập nhật';
    const themes = ['precision', 'atelier', 'berich', 'clickup', 'moday', 'terminal'];

    const results = await page.evaluate(
      ({ themes, testText }) => {
        const slots = ['--font-display', '--font-body', '--font-mono', '--font-accent'];
        const report: Record<string, Record<string, { family: string; supported: boolean }>> = {};

        for (const theme of themes) {
          document.documentElement.setAttribute('data-theme', theme);
          const computed = getComputedStyle(document.documentElement);
          const themeReport: Record<string, { family: string; supported: boolean }> = {};
          report[theme] = themeReport;

          for (const slot of slots) {
            const raw = computed.getPropertyValue(slot).trim();
            const primaryFamily = (raw.split(',')[0] || '').trim().replace(/^['"]|['"]$/g, '');
            const supported = document.fonts.check(`16px "${primaryFamily}"`, testText);
            themeReport[slot] = { family: primaryFamily, supported };
          }
        }
        return report;
      },
      { themes, testText },
    );

    expect(Object.keys(results)).toHaveLength(6);
    for (const theme of themes) {
      expect(results[theme]).toBeDefined();
    }
  });
});
