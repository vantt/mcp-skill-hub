import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { seedDistillWorkspace, type SeededDistillWorkspace } from './support/seed-workspace';
import { startServer, type RunningServer } from './support/server';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const binaryPath = path.resolve(__dirname, '../.e2e/skillhub');

async function checkA11y(page: Page, contextName: string) {
  const scan = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'])
    .analyze();

  const violations = scan.violations.filter(
    (v) => v.impact === 'serious' || v.impact === 'critical',
  );

  expect(
    violations,
    `A11y violations on ${contextName}: ${JSON.stringify(violations, null, 2)}`,
  ).toEqual([]);
}

test.describe('Inbox, Insight Detail, and Patch Composer Journey', () => {
  let seed: SeededDistillWorkspace;
  let server: RunningServer;

  test('inbox to decision to composer with conflict detection and axe checks', async ({
    page,
  }) => {
    test.setTimeout(180_000);
    seed = seedDistillWorkspace();
    server = await startServer({ workspace: seed.ws });

    try {
      // 1. Authenticate with server startup URL
      await page.goto(server.url);
      await expect(page.getByRole('heading', { name: 'Home' }).first()).toBeVisible();

      // 2. Open /inbox
      await page.goto(`${server.origin}/inbox`);
      await expect(page.getByRole('heading', { name: 'Inbox' }).first()).toBeVisible();

      // Verify both insights appear
      await expect(
        page.getByText('Incorporate practice one into consumer review.'),
      ).toBeVisible();
      await expect(
        page.getByText('Incorporate practice two into consumer review.'),
      ).toBeVisible();

      // Axe check on /inbox
      await checkA11y(page, '/inbox');

      // 3. Open first insight
      await page.getByText('Incorporate practice one into consumer review.').click();
      await expect(page).toHaveURL(/inbox\/INS-consumer-review--insight-consumer-one/);
      await expect(
        page.getByText('Incorporate practice one into consumer review.').first(),
      ).toBeVisible();

      // Axe check on /inbox/:id
      await checkA11y(page, '/inbox/:id');

      // 4. Reject with rationale
      await page.getByRole('button', { name: 'Reject' }).click();
      const rejectDialog = page.getByRole('dialog');
      await expect(rejectDialog).toBeVisible();
      const rejectTextarea = rejectDialog.getByPlaceholder(
        'Explain the reason for this decision…',
      );
      await rejectTextarea.fill('Not suitable for this project.');
      await rejectDialog.getByRole('button', { name: 'Confirm reject' }).click();

      // Verify status updates to rejected
      await expect(page.getByText('rejected')).toBeVisible();

      // 5. Reopen with rationale
      await page.getByRole('button', { name: 'Reopen' }).click();
      const reopenDialog = page.getByRole('dialog');
      await expect(reopenDialog).toBeVisible();
      const reopenTextarea = reopenDialog.getByPlaceholder(
        'Explain the reason for this decision…',
      );
      await reopenTextarea.fill('Reconsidering this improvement.');
      await reopenDialog.getByRole('button', { name: 'Confirm reopen' }).click();

      // If refused for lack of new evidence, assert refusal text and continue
      const refusalError = page.locator('.fg-banner--danger');
      await expect(refusalError).toBeVisible();
      const errText = await refusalError.textContent();
      expect(errText).toMatch(/reopen|evidence|conflicts with validation rules/i);
      await reopenDialog.getByRole('button', { name: 'Cancel' }).click();

      // Return to inbox to open second insight and compose patch
      await page.goto(`${server.origin}/inbox`);
      await expect(page.getByRole('heading', { name: 'Inbox' }).first()).toBeVisible();
      await page.getByText('Incorporate practice two into consumer review.').click();
      const planBtn = page.getByRole('button', { name: 'Plan' });
      if (await planBtn.isVisible()) {
        await planBtn.click();
        const planDialog = page.getByRole('dialog');
        await expect(planDialog).toBeVisible();
        await planDialog
          .getByPlaceholder('Explain the reason for this decision…')
          .fill('Plan for patch composition.');
        await planDialog.getByRole('button', { name: 'Confirm plan' }).click();
        await expect(page.getByText('planned')).toBeVisible();
      }

      // 6. Click Compose patch
      const composeBtn = page.getByRole('button', { name: 'Compose patch' });
      await expect(composeBtn).toBeVisible();
      await composeBtn.click();

      await expect(page).toHaveURL(/inbox\/INS-consumer-review--insight-consumer-two\/apply/);
      await expect(
        page.getByRole('heading', { name: 'Compose patch' }).first(),
      ).toBeVisible();

      // Axe check on /inbox/:id/apply
      await checkA11y(page, '/inbox/:id/apply');

      // 7. Conflict test: edit outside via CLI before previewing
      execFileSync(
        binaryPath,
        ['skill', 'edit', 'consumer-review', '--description', 'Changed outside', '--yes'],
        {
          env: { ...process.env, SKILLHUB_WORKSPACE: seed.ws },
          stdio: 'pipe',
        },
      );

      // In composer: edit content in textarea
      const editorTextarea = page.getByLabel('SKILL.md replacement content');
      const currentContent = await editorTextarea.inputValue();
      await editorTextarea.fill(
        currentContent + '\n\n## Practice Two\nApply practice two instructions.\n',
      );

      // Map required observation
      const conceptInput = page.getByPlaceholder('Concept, e.g. retry limits');
      await conceptInput.fill('practice-two');
      await page.getByRole('button', { name: '+ Add concept' }).click();

      // Check counter shows 1/1
      const counter = page.locator('span[aria-live="polite"]');
      await expect(counter).toHaveText('1/1');

      // Click Preview apply -> Conflict Drawer opens
      const previewBtn = page.getByRole('button', { name: 'Preview apply' });
      await expect(previewBtn).not.toBeDisabled();
      await previewBtn.click();

      await expect(
        page.getByText('SKILL.md changed since you opened it'),
      ).toBeVisible();

      // Resolve conflict by using latest as base
      await page.getByRole('button', { name: 'Use latest as base' }).click();
      await expect(
        page.getByText('SKILL.md changed since you opened it'),
      ).not.toBeVisible();

      // Edit content again to differ from latest base
      await editorTextarea.fill(
        currentContent + '\n\n## Practice Two Resolved\nApply practice two instructions.\n',
      );

      // Preview apply again -> ProposalPreview opens
      await previewBtn.click();
      const proposalModal = page.getByRole('dialog');
      await expect(proposalModal).toBeVisible();
      await expect(proposalModal.getByText('## Practice Two Resolved', { exact: true })).toBeVisible();

      // Confirm proposal
      const confirmApplyBtn = proposalModal.getByRole('button', { name: 'Apply patch' });
      await confirmApplyBtn.click();

      // Receipt displayed
      await expect(
        page.getByText('Insight patch applied successfully!'),
      ).toBeVisible();
      const viewReviewBtn = page.getByRole('link', { name: 'View Skill Review →' });
      await expect(viewReviewBtn).toBeVisible();
      await viewReviewBtn.click();

      await expect(page).toHaveURL(/skills\/consumer-review\?tab=review/);
    } finally {
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
    }
  });
});
