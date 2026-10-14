import fs from 'node:fs';
import path from 'node:path';
import type { Page } from '@playwright/test';
import { commitHub } from '../support/seed-workspace';
import { externalEdit, UX_SKILLS, UX_SOURCE, type UxHub } from './seed-states';

export interface FlowContext {
  page: Page;
  hub: Pick<UxHub, 'runCLI' | 'ws'>;
}

export interface Flow {
  id: string;
  // Row number in the Tier table of plan.md; a row can have several captured states.
  row: number;
  tier: 1 | 2 | 3;
  title: string;
  route: string;
  // The one thing a first-time user should be able to finish from this screen.
  task: string;
  preconditions: string;
  // Puts the page in the state to capture (open a dialog, fill a form). A throw is
  // recorded as a capture error; it never fails the run.
  prepare?: (ctx: FlowContext) => Promise<void>;
}

const skillPath = (id: string, tab?: string) => `/skills/${id}${tab ? `?tab=${tab}` : ''}`;

// A companion file of the active skill, written on first use so the file tree has a file to open
// on the fixture hub and on a hub clone alike.
const UX_RESOURCE_REL = 'references/checklist.md';
const UX_RESOURCE = `skills/core/${UX_SKILLS.active}/${UX_RESOURCE_REL}`;
const UX_RESOURCE_BODY = '---\ntitle: Review checklist\n---\n\n# Review checklist\n\n- Read the change description first.\n- Run the checks before you approve.\n';

function ensureCompanionFile(hub: FlowContext['hub']): boolean {
  const file = path.join(hub.ws, UX_RESOURCE);
  if (fs.existsSync(file)) return false;
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, UX_RESOURCE_BODY);
  hub.runCLI(['maintain', 'rebuild']);
  commitHub(hub.ws, 'Add a reference document to the active skill');
  return true;
}

// Flows follow the Tier table in plan.md, most-used first. The user confirmed that
// order on 2026-10-10 (usage is not recorded; the order is reasoned).
export const FLOWS: Flow[] = [
  // Tier 1: every visit or daily
  {
    id: 'home',
    row: 1,
    tier: 1,
    title: 'Open the hub, see what needs me',
    route: '/',
    task: 'Say in one glance what needs attention and click the next action.',
    preconditions: 'Workspace with skills in several states.',
  },
  {
    id: 'skills-list',
    row: 2,
    tier: 1,
    title: 'Find a skill',
    route: '/skills',
    task: 'Find one skill by name or by state and open it.',
    preconditions: 'At least one skill per lifecycle state.',
  },
  {
    id: 'skill-overview',
    row: 3,
    tier: 1,
    title: 'Check one skill: overview',
    route: skillPath(UX_SKILLS.active),
    task: 'Say what the skill is for, when agents get it and where it comes from.',
    preconditions: `${UX_SKILLS.active} is active with routing fields and a source attached.`,
  },
  {
    id: 'skill-review',
    row: 3,
    tier: 1,
    title: 'Check one skill: can agents use it',
    route: skillPath(UX_SKILLS.needsSetup, 'review'),
    task: 'Tell whether agents can use this skill and what to run to trust its content.',
    preconditions: `${UX_SKILLS.needsSetup} has a runtime block with a missing binary.`,
  },
  {
    id: 'skill-review-draft',
    row: 3,
    tier: 1,
    title: 'Check one skill: draft that is not ready',
    route: skillPath(UX_SKILLS.draft, 'review'),
    task: 'Tell why a draft cannot be activated and what to fix first.',
    preconditions: `${UX_SKILLS.draft} has no trigger, not-for or scope.`,
  },
  {
    id: 'lifecycle-activate',
    row: 4,
    tier: 1,
    title: 'Activate, deprecate, archive: disabled activate',
    route: skillPath(UX_SKILLS.draft, 'review'),
    task: 'Understand why Activate is unavailable and how to make it available.',
    preconditions: `${UX_SKILLS.draft} is a draft.`,
  },
  {
    id: 'lifecycle-deprecate',
    row: 4,
    tier: 1,
    title: 'Activate, deprecate, archive: deprecate confirm',
    route: skillPath(UX_SKILLS.active),
    task: 'Deprecate an active skill after reading what will change.',
    preconditions: `${UX_SKILLS.active} is active.`,
    prepare: async ({ page }) => {
      await page.getByRole('button', { name: 'Deprecate' }).first().click();
      await page.getByRole('dialog').waitFor({ timeout: 5000 });
    },
  },
  // Tier 2: weekly
  {
    id: 'editor',
    row: 5,
    tier: 2,
    title: 'Fix a skill routing fields: routing tab',
    route: skillPath(UX_SKILLS.active, 'routing'),
    task: 'Change the description and see what the change does before saving.',
    preconditions: `${UX_SKILLS.active} is active.`,
  },
  {
    id: 'editor-preview',
    row: 5,
    tier: 2,
    title: 'Fix a skill routing fields: preview before apply',
    route: skillPath(UX_SKILLS.active, 'routing'),
    task: 'Read the preview and tell exactly what will change before confirming.',
    preconditions: 'Description edited in the form.',
    prepare: async ({ page }) => {
      await page.locator('#edit-desc').fill('Description changed during the UX evaluation.');
      await page.getByRole('button', { name: 'Preview changes' }).click();
      await page.getByRole('dialog').waitFor({ timeout: 10000 });
    },
  },
  {
    id: 'editor-conflict',
    row: 5,
    tier: 2,
    title: 'Fix a skill routing fields: conflict drawer',
    route: skillPath(UX_SKILLS.deprecated, 'routing'),
    task: 'Understand that the skill changed elsewhere and choose how to continue.',
    preconditions: 'The skill is edited through the CLI after the form loaded.',
    prepare: async ({ page, hub }) => {
      await page.locator('#edit-desc').waitFor({ timeout: 10000 });
      externalEdit(hub, UX_SKILLS.deprecated, 'Changed outside the browser.');
      await page.locator('#edit-desc').fill('Browser draft that now conflicts.');
      await page.getByRole('button', { name: 'Preview changes' }).click();
      await page.getByRole('dialog').waitFor({ timeout: 10000 });
    },
  },
  {
    id: 'create',
    row: 6,
    tier: 2,
    title: 'Create a skill: form',
    route: '/skills/create',
    task: 'Fill the form and know which fields are needed to activate later.',
    preconditions: 'None.',
  },
  {
    id: 'create-preview',
    row: 6,
    tier: 2,
    title: 'Create a skill: preview draft',
    route: '/skills/create',
    task: 'Preview the new draft and see what will be written before confirming.',
    preconditions: 'Identity fields filled.',
    prepare: async ({ page }) => {
      await page.locator('#create-id').fill('ux-new-skill');
      await page.locator('#create-name').fill('UX New Skill');
      await page.locator('#create-desc').fill('Created during the UX evaluation.');
      await page.getByRole('button', { name: 'Preview draft' }).click();
      await page.getByRole('dialog').waitFor({ timeout: 10000 });
    },
  },
  {
    id: 'add',
    row: 7,
    tier: 2,
    title: 'Add skills from GitHub: discover',
    route: '/skills/add',
    task: 'Paste a repository address and know what happens next.',
    preconditions: 'None; discovery needs the network so it is not run.',
  },
  {
    id: 'add-advanced',
    row: 7,
    tier: 2,
    title: 'Add skills from GitHub: advanced options',
    route: '/skills/add',
    task: 'Find out what the advanced options do without leaving the page.',
    preconditions: 'Advanced section opened.',
    prepare: async ({ page }) => {
      await page.getByText('Advanced', { exact: false }).first().click();
    },
  },
  {
    id: 'upstream-updates',
    row: 8,
    tier: 2,
    title: 'Act on upstream updates: list',
    route: '/skills?upstream=updates',
    task: 'See which skills have an update and open one.',
    preconditions: 'Real upstream-tracked skills (clone of a hub); empty on the fixture.',
  },
  // Tier 3: monthly or rare
  {
    id: 'sources',
    row: 9,
    tier: 3,
    title: 'Link and check learning sources',
    route: '/sources',
    task: 'See which sources are linked, check them and find the ones with news.',
    preconditions: `Source ${UX_SOURCE} attached to ${UX_SKILLS.active}.`,
  },
  {
    id: 'distill-empty',
    row: 10,
    tier: 3,
    title: 'Hand a distill job to the curator agent: nothing selected',
    route: '/sources/distill',
    task: 'Find out what to do when no source is selected.',
    preconditions: 'No source in the query string.',
  },
  {
    id: 'distill-source',
    row: 10,
    tier: 3,
    title: 'Hand a distill job to the curator agent: source selected',
    route: `/sources/distill?source=${UX_SOURCE}`,
    task: 'Copy the brief and tell the curator agent what to do with it.',
    preconditions: `Source ${UX_SOURCE} exists.`,
  },
  {
    id: 'inbox',
    row: 11,
    tier: 3,
    title: 'Triage lessons: inbox route',
    route: '/inbox',
    task: 'Reaching the old inbox address must lead somewhere useful, not a raw error.',
    preconditions: 'The inbox route was removed from the app.',
  },
  {
    id: 'skill-tab-files',
    row: 12,
    tier: 3,
    title: 'Skill tabs: Files',
    route: skillPath(UX_SKILLS.active, 'files'),
    task: 'See which files the skill carries.',
    preconditions: `${UX_SKILLS.active} is active.`,
  },
  {
    id: 'skill-file-pending',
    row: 12,
    tier: 3,
    title: 'Skill tabs: Files, a pending change',
    route: `${skillPath(UX_SKILLS.active, 'files')}&file=${encodeURIComponent(UX_RESOURCE_REL)}`,
    task: 'Change a reference document and know that nothing is committed before the review is confirmed.',
    preconditions: `${UX_SKILLS.active} carries a reference document next to its SKILL.md.`,
    prepare: async ({ page, hub }) => {
      if (ensureCompanionFile(hub)) {
        await page.reload();
        await page.waitForLoadState('networkidle');
      }
      await page.getByRole('button', { name: 'Edit', exact: true }).click();
      await page.getByRole('textbox', { name: `Edit ${UX_RESOURCE_REL}` }).fill(`${UX_RESOURCE_BODY}- Note what you could not check.\n`);
      await page.getByText('not committed').waitFor({ timeout: 10000 });
    },
  },
  {
    id: 'skill-files-review',
    row: 12,
    tier: 3,
    title: 'Skill tabs: Files, review the changes',
    route: `${skillPath(UX_SKILLS.active, 'files')}&file=${encodeURIComponent(UX_RESOURCE_REL)}`,
    task: 'Read each changed file, the routing lint and the commit message before committing.',
    preconditions: 'A pending change in the Files tab.',
    prepare: async ({ page, hub }) => {
      ensureCompanionFile(hub);
      await page.reload();
      await page.getByRole('button', { name: 'Edit', exact: true }).click();
      await page.getByRole('textbox', { name: `Edit ${UX_RESOURCE_REL}` }).fill(`${UX_RESOURCE_BODY}- Note what you could not check.\n`);
      await page.getByRole('button', { name: /Review/ }).click();
      await page.getByLabel('Commit message').waitFor({ timeout: 10000 });
    },
  },
  {
    id: 'review-queue',
    row: 14,
    tier: 3,
    title: 'Waiting for review: the list',
    route: '/review',
    task: 'See what needs a person before agents get it, and open one.',
    preconditions: 'A draft skill exists.',
  },
  {
    id: 'review-content',
    row: 14,
    tier: 3,
    title: 'Waiting for review: read the whole skill',
    route: `/skills/${UX_SKILLS.draft}/review-content`,
    task: 'Read every file of a skill, with its license and origin.',
    preconditions: `${UX_SKILLS.draft} is a draft.`,
  },
  {
    id: 'skill-tab-usage',
    row: 12,
    tier: 3,
    title: 'Skill tabs: Usage',
    route: skillPath(UX_SKILLS.active, 'usage'),
    task: 'See whether agents have used the skill.',
    preconditions: 'No usage recorded yet.',
  },
  {
    id: 'skill-tab-runtime',
    row: 12,
    tier: 3,
    title: 'Skill tabs: Runtime',
    route: skillPath(UX_SKILLS.needsSetup, 'runtime'),
    task: 'See what this machine still needs for the skill and the command to fix it.',
    preconditions: `${UX_SKILLS.needsSetup} requires a binary that is missing.`,
  },
  {
    id: 'skill-tab-sources',
    row: 12,
    tier: 3,
    title: 'Skill tabs: Sources',
    route: skillPath(UX_SKILLS.active, 'sources'),
    task: 'See which sources feed the skill and open the distill hand-off.',
    preconditions: `Source ${UX_SOURCE} attached.`,
  },
  {
    id: 'appearance-menu',
    row: 13,
    tier: 3,
    title: 'Theme and workspace status: appearance menu',
    route: '/',
    task: 'Switch theme and find where the workspace status is shown.',
    preconditions: 'None.',
    prepare: async ({ page }) => {
      await page.getByRole('button', { name: 'Appearance' }).click();
      await page.getByRole('dialog').waitFor({ timeout: 5000 });
    },
  },
];
