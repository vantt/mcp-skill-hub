import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {
  attachFilesystemSource,
  makeCLIRunner,
  resolveBinaryPath,
  seedDistillWorkspace,
  type CLIRunner,
} from '../support/seed-workspace';

// Skill ids created by seedUxStates. Flows refer to these so the same flow list
// works on a clone of the live hub and on a self-contained fixture workspace.
export const UX_SKILLS = {
  active: 'ux-active',
  draft: 'ux-draft',
  deprecated: 'ux-deprecated',
  archived: 'ux-archived',
  needsSetup: 'ux-needs-setup',
} as const;

export const UX_SOURCE = 'ux-source';

export interface UxHub {
  ws: string;
  root: string;
  // Environment for every child process: HOME and XDG_* point inside root so no run
  // can read or write the real user profile.
  env: NodeJS.ProcessEnv;
  runCLI: CLIRunner;
  // States that could not be created through the CLI, with the reason.
  skipped: Array<{ state: string; reason: string }>;
  origin: 'clone' | 'fixture';
  cleanup: () => void;
}

export function isolatedEnv(root: string): NodeJS.ProcessEnv {
  const home = path.join(root, 'home');
  fs.mkdirSync(home, { recursive: true });
  return {
    ...process.env,
    HOME: home,
    USERPROFILE: home,
    XDG_CONFIG_HOME: path.join(root, 'xdg', 'config'),
    XDG_DATA_HOME: path.join(root, 'xdg', 'data'),
    XDG_STATE_HOME: path.join(root, 'xdg', 'state'),
    XDG_CACHE_HOME: path.join(root, 'xdg', 'cache'),
    GIT_CONFIG_GLOBAL: path.join(home, '.gitconfig'),
    GIT_CONFIG_NOSYSTEM: '1',
    GIT_TERMINAL_PROMPT: '0',
  };
}

function writeTemp(root: string, name: string, body: string): string {
  const file = path.join(root, name);
  fs.writeFileSync(file, body);
  return file;
}

function createSkill(
  runCLI: CLIRunner,
  root: string,
  id: string,
  opts: { complete: boolean; runtimeYaml?: string },
): void {
  const content = writeTemp(
    root,
    `${id}.md`,
    `---\nname: ${id}\ndescription: Sample skill for the UX evaluation.\n---\n\n# ${id}\n\nSample instructions.\n`,
  );
  const args = [
    'skill', 'create', id,
    '--collection', 'core',
    '--name', id,
    '--description', 'Sample skill for the UX evaluation.',
    '--content-file', content,
  ];
  if (opts.complete) {
    args.push(
      '--operation', 'review',
      '--trigger', `review ${id} changes`,
      '--not-for', 'writing new features',
      '--min-scope', 'single_step',
    );
  }
  args.push('--yes');
  runCLI(args);
  if (opts.runtimeYaml) {
    const runtime = writeTemp(root, `${id}-runtime.yaml`, opts.runtimeYaml);
    runCLI(['skill', 'edit', id, '--runtime-file', runtime, '--yes']);
  }
}

// seedUxStates adds one skill per lifecycle state and one filesystem source. It only
// uses the CLI (plus the same catalog file the existing E2E seed writes for sources).
export function seedUxStates(hub: Pick<UxHub, 'ws' | 'root' | 'runCLI' | 'skipped'>): void {
  const { ws, root, runCLI, skipped } = hub;
  const attempt = (state: string, fn: () => void) => {
    try {
      fn();
    } catch (err) {
      skipped.push({ state, reason: err instanceof Error ? err.message.split('\n')[0] ?? '' : String(err) });
    }
  };

  attempt('active', () => {
    createSkill(runCLI, root, UX_SKILLS.active, { complete: true });
    runCLI(['skill', 'activate', UX_SKILLS.active, '--yes']);
  });
  attempt('draft', () => createSkill(runCLI, root, UX_SKILLS.draft, { complete: false }));
  attempt('deprecated', () => {
    createSkill(runCLI, root, UX_SKILLS.deprecated, { complete: true });
    runCLI(['skill', 'activate', UX_SKILLS.deprecated, '--yes']);
    runCLI(['skill', 'deprecate', UX_SKILLS.deprecated, '--yes']);
  });
  attempt('archived', () => {
    createSkill(runCLI, root, UX_SKILLS.archived, { complete: true });
    runCLI(['skill', 'activate', UX_SKILLS.archived, '--yes']);
    runCLI(['skill', 'deprecate', UX_SKILLS.archived, '--yes']);
    runCLI(['skill', 'archive', UX_SKILLS.archived, '--yes']);
  });
  attempt('needs-setup', () => {
    createSkill(runCLI, root, UX_SKILLS.needsSetup, {
      complete: true,
      runtimeYaml:
        'requires:\n  bins:\n    - ux-evaluation-missing-tool\nsetup:\n  check: ux-evaluation-missing-tool --version\n',
    });
  });
  attempt('source', () => {
    attachFilesystemSource(
      ws,
      runCLI,
      UX_SOURCE,
      '# UX source\nKnowledge a curator may fold into a skill.\n',
      UX_SKILLS.active,
    );
  });

  // The CLI cannot create these states, so they are recorded rather than faked:
  // a strict-invalid skill needs a hand-broken file, and a tracked upstream needs an
  // https GitHub remote. A clone of a real hub brings real upstream-tracked skills.
  skipped.push({
    state: 'strict-invalid',
    reason: 'no CLI path writes a file that fails strict validation; needs a hand-broken file',
  });
  skipped.push({
    state: 'upstream-update',
    reason: 'skill add accepts https GitHub locators only; clone a hub with upstream-tracked skills (UX_HUB_SOURCE)',
  });
}

// externalEdit changes a skill through the CLI while the web editor still holds the
// old base version, which is how an editing conflict is produced.
export function externalEdit(hub: Pick<UxHub, 'runCLI'>, id: string, description: string): void {
  hub.runCLI(['skill', 'edit', id, '--description', description, '--yes']);
}

// prepareHub builds the workspace the evaluation runs against. With UX_HUB_SOURCE set
// to a Git workspace it is cloned (the source is only read); otherwise a small
// fixture workspace is seeded. Either way HOME and XDG_* are isolated.
export function prepareHub(): UxHub {
  resolveBinaryPath();
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-ux-'));
  const env = isolatedEnv(root);
  const skipped: UxHub['skipped'] = [];
  const source = process.env.UX_HUB_SOURCE;

  let ws: string;
  let origin: UxHub['origin'];
  if (source) {
    ws = path.join(root, 'hub');
    execFileSync('git', ['clone', '--quiet', '--no-hardlinks', source, ws], { env, stdio: 'pipe' });
    origin = 'clone';
  } else {
    ws = seedDistillWorkspace(env).ws;
    origin = 'fixture';
  }

  const runCLI = makeCLIRunner(ws, env);
  runCLI(['rebuild']);
  const hub: UxHub = {
    ws,
    root,
    env,
    runCLI,
    skipped,
    origin,
    cleanup: () => {
      fs.rmSync(root, { recursive: true, force: true });
      if (origin === 'fixture') {
        fs.rmSync(ws, { recursive: true, force: true });
      }
    },
  };
  seedUxStates(hub);
  runCLI(['rebuild']);
  return hub;
}
