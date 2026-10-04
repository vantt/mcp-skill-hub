import AxeBuilder from '@axe-core/playwright';
import { expect, test } from '@playwright/test';
import type { RunningServer } from './support/server';
import { startServer } from './support/server';

let server: RunningServer;

test.beforeAll(async () => {
  server = await startServer();
});

test.afterAll(async () => {
  if (server) {
    await server.stop();
  }
});

test('smoke: home and skills routes, theme persistence, loopback-only requests, and axe accessibility', async ({
  page,
}) => {
  test.setTimeout(120_000);
  // 1. Record requests and assert each starts with origin
  const requestedUrls: string[] = [];
  page.on('request', (req) => {
    requestedUrls.push(req.url());
  });

  // 2. Open startup URL with session token
  await page.goto(server.url);

  // Home h1 is visible
  const homeHeading = page.locator('h1');
  await expect(homeHeading).toBeVisible();
  await expect(homeHeading).toHaveText('Home');

  // 3. Navigate to Skills
  await page.click('a[href="/skills"]');
  await expect(page).toHaveURL(new RegExp('/skills'));

  // Skills heading and smoke-skill row are visible
  await expect(page.locator('h1')).toHaveText('Skills');
  const smokeRow = page.locator('tr', { hasText: 'smoke-skill' });
  await expect(smokeRow).toBeVisible();

  // 4. Type in search and see URL query change
  const searchInput = page.getByPlaceholder('Search id or name…');
  await searchInput.fill('smoke');
  await expect(page).toHaveURL(/q=smoke/);
  await expect(smokeRow).toBeVisible();

  // 5. Switch scheme to dark in Appearance menu
  const appButton = page.getByRole('button', { name: 'Appearance' });
  await appButton.click();

  const darkOption = page.getByRole('radio', { name: 'Dark' });
  await darkOption.click();

  // Verify dark scheme attribute applied
  await expect(page.locator('html')).toHaveAttribute('data-scheme', 'dark');

  // Reload and assert dark scheme persists
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('data-scheme', 'dark');

  // 6. Assert all recorded network requests were loopback-only (origin or data:)
  expect(requestedUrls.length).toBeGreaterThan(0);
  for (const reqUrl of requestedUrls) {
    const isAllowed = reqUrl.startsWith(server.origin) || reqUrl.startsWith('data:');
    expect(isAllowed, `Non-loopback request observed: ${reqUrl}`).toBe(true);
  }

  // 7. Run Axe accessibility checks on / and /skills in light and dark at 1280 and 390
  const routesToCheck = ['/', '/skills'];
  const viewports = [
    { width: 1280, height: 800, name: 'desktop' },
    { width: 390, height: 844, name: 'mobile' },
  ];
  const schemes: Array<'light' | 'dark'> = ['light', 'dark'];

  for (const route of routesToCheck) {
    for (const vp of viewports) {
      for (const scheme of schemes) {
        await page.setViewportSize({ width: vp.width, height: vp.height });
        await page.goto(`${server.origin}${route}`);

        await page.evaluate((s) => {
          document.documentElement.setAttribute('data-scheme', s);
        }, scheme);

        // Wait for fonts/layout to settle
        await page.waitForTimeout(200);

        const scan = await new AxeBuilder({ page })
          .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
          .analyze();

        const violations = scan.violations.filter(
          (v) => v.impact === 'serious' || v.impact === 'critical',
        );

        expect(
          violations,
          `A11y violation on ${route} (${vp.name}, ${scheme}): ${JSON.stringify(violations, null, 2)}`,
        ).toEqual([]);
      }
    }
  }
});
