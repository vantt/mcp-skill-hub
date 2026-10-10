import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { attachFilesystemSource, makeCLIRunner, resolveBinaryPath, type CLIRunner } from '../support/seed-workspace';
import { isolatedEnv } from './seed-states';

// Demo hub for the README and user-guide screenshots. Everything is invented and made
// with the CLI of the built binary inside a throwaway root: HOME and XDG_* point into
// it, the Git identity and commit dates are fixed, and nothing is read from or cloned
// from a real hub. It never touches ~/skill-hub.

export const DEMO_SKILLS = {
  active: 'release-checklist',
  readyDraft: 'incident-notes',
  draft: 'api-design-review',
  deprecated: 'meeting-summary',
  needsSetup: 'changelog-writer',
} as const;

export const DEMO_SOURCE = 'acme-agent-skills';

// Fixed date for every Git commit the demo hub makes, so the history shown in the UI
// is the same on every run.
export const DEMO_DATE = '2026-09-15T10:00:00Z';

export interface DemoHub {
  ws: string;
  root: string;
  env: NodeJS.ProcessEnv;
  runCLI: CLIRunner;
  cleanup: () => void;
}

interface DemoSkill {
  id: string;
  name: string;
  description: string;
  body: string;
  // Routing fields; a draft that leaves them out is "not ready to activate".
  routing?: { operation: string; trigger: string; notFor: string };
  runtimeYaml?: string;
}

const SKILLS: DemoSkill[] = [
  {
    id: DEMO_SKILLS.active,
    name: 'Release checklist',
    description: 'Make a release safe: version, changelog, tests and a rollback plan.',
    body: '# Release checklist\n\n1. Confirm the version number and the changelog entry.\n2. Run the full test suite.\n3. Write down how to roll back.\n',
    routing: {
      operation: 'review',
      trigger: 'prepare a release or cut a version tag',
      notFor: 'writing new features',
    },
  },
  {
    id: DEMO_SKILLS.readyDraft,
    name: 'Incident notes',
    description: 'Turn a stream of chat messages into a timeline and a short summary after an outage.',
    body: '# Incident notes\n\nCollect the timeline first, then the impact, then the follow-ups.\n',
    routing: {
      operation: 'summarize',
      trigger: 'write up an incident or outage',
      notFor: 'live debugging',
    },
  },
  {
    id: DEMO_SKILLS.draft,
    name: 'API design review',
    description: 'Review a new HTTP API for naming, errors and versioning before it ships.',
    body: '# API design review\n\nCheck resource names, error shapes and how the API is versioned.\n',
  },
  {
    id: DEMO_SKILLS.deprecated,
    name: 'Meeting summary',
    description: 'Summarize a meeting into decisions and action items.',
    body: '# Meeting summary\n\nList the decisions, then who does what by when.\n',
    routing: {
      operation: 'summarize',
      trigger: 'summarize a meeting',
      notFor: 'taking live notes',
    },
  },
  {
    id: DEMO_SKILLS.needsSetup,
    name: 'Changelog writer',
    description: 'Draft a changelog from merged pull requests with the acme-changelog tool.',
    body: '# Changelog writer\n\nRun acme-changelog for the range of the release, then tidy the wording.\n',
    routing: {
      operation: 'write',
      trigger: 'write a changelog',
      notFor: 'release notes for customers',
    },
    runtimeYaml: 'requires:\n  bins:\n    - acme-changelog\nsetup:\n  check: acme-changelog --version\n',
  },
];

function createDemoSkill(runCLI: CLIRunner, root: string, skill: DemoSkill): void {
  const content = path.join(root, `${skill.id}.md`);
  fs.writeFileSync(content, `---\nname: ${skill.id}\ndescription: ${JSON.stringify(skill.description)}\n---\n\n${skill.body}`);
  const args = [
    'skill', 'create', skill.id,
    '--collection', 'core',
    '--name', skill.name,
    '--description', skill.description,
    '--content-file', content,
  ];
  if (skill.routing) {
    args.push(
      '--operation', skill.routing.operation,
      '--trigger', skill.routing.trigger,
      '--not-for', skill.routing.notFor,
      '--min-scope', 'single_step',
    );
  }
  args.push('--yes');
  runCLI(args);
  if (skill.runtimeYaml) {
    const runtime = path.join(root, `${skill.id}-runtime.yaml`);
    fs.writeFileSync(runtime, skill.runtimeYaml);
    runCLI(['skill', 'edit', skill.id, '--runtime-file', runtime, '--yes']);
  }
}

// An upstream update cannot be made here: adding from GitHub, checking upstream and
// previewing an update all fetch over the network, so that state is not shown.
export function prepareDemoHub(): DemoHub {
  resolveBinaryPath();
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-shots-'));
  const env: NodeJS.ProcessEnv = {
    ...isolatedEnv(root),
    GIT_AUTHOR_NAME: 'Demo Curator',
    GIT_AUTHOR_EMAIL: 'demo@example.invalid',
    GIT_COMMITTER_NAME: 'Demo Curator',
    GIT_COMMITTER_EMAIL: 'demo@example.invalid',
    GIT_AUTHOR_DATE: DEMO_DATE,
    GIT_COMMITTER_DATE: DEMO_DATE,
  };
  const ws = path.join(root, 'skill-hub');
  const runCLI = makeCLIRunner(ws, env);
  // A failure while seeding must not leave the throwaway root behind.
  try {
    runCLI(['init', ws, '--yes']);

    for (const skill of SKILLS) {
      createDemoSkill(runCLI, root, skill);
    }
    // incident-notes stays a complete draft so the activate confirmation can be shown.
    for (const id of [DEMO_SKILLS.active, DEMO_SKILLS.needsSetup, DEMO_SKILLS.deprecated]) {
      runCLI(['skill', 'activate', id, '--yes']);
    }
    runCLI(['skill', 'deprecate', DEMO_SKILLS.deprecated, '--yes']);

    attachFilesystemSource(
      ws,
      runCLI,
      DEMO_SOURCE,
      '# Acme agent skills\nShared release and incident practices a curator may fold into a skill.\n',
      DEMO_SKILLS.active,
    );
    runCLI(['rebuild']);

    // Commit everything so the screens show a clean workspace, as a curator would have it.
    for (const args of [['add', '-A'], ['commit', '--quiet', '-m', 'Add demo skills and source']]) {
      execFileSync('git', args, { cwd: ws, env, stdio: 'pipe' });
    }
  } catch (err) {
    fs.rmSync(root, { recursive: true, force: true });
    throw err;
  }

  return {
    ws,
    root,
    env,
    runCLI,
    cleanup: () => fs.rmSync(root, { recursive: true, force: true }),
  };
}
