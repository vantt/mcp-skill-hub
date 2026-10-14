import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export interface SeededDistillWorkspace {
  ws: string;
}

// A runner executes the skillhub binary for one hub. A second argument runs it from another folder,
// for example the project a skill is written in.
export type CLIRunner = (args: string[], cwd?: string) => string;

export function resolveBinaryPath(): string {
  const binaryPath = path.resolve(__dirname, '../../.e2e/skillhub');
  if (!fs.existsSync(binaryPath)) {
    throw new Error(`skillhub binary not found at ${binaryPath}. Run make web-e2e to build.`);
  }
  return binaryPath;
}

let userConfigDir: string | undefined;

// e2eUserConfigDir is a temporary per-user Skill Hub config directory for this test
// process. `skillhub init` records the hub path there, never in the real user profile.
export function e2eUserConfigDir(): string {
  userConfigDir ??= fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-e2e-user-config-'));
  return userConfigDir;
}

// makeCLIRunner runs the built skillhub binary against one workspace, from inside
// that folder (`skillhub init` acts on the current folder, which must exist). Pass env
// to replace the inherited environment (for example an isolated HOME and XDG_*).
export function makeCLIRunner(ws: string, env: NodeJS.ProcessEnv = process.env): CLIRunner {
  const binaryPath = resolveBinaryPath();
  return (args, cwd = ws) =>
    execFileSync(binaryPath, args, {
      cwd,
      encoding: 'utf-8',
      env: { SKILLHUB_USER_CONFIG_DIR: e2eUserConfigDir(), ...gitIdentity, ...env, SKILLHUB_WORKSPACE: ws },
      stdio: 'pipe',
    });
}

// Every commit the hub makes needs an author; the suite's own environment has no Git config.
export const gitIdentity: NodeJS.ProcessEnv = {
  GIT_AUTHOR_NAME: 'E2E Curator',
  GIT_AUTHOR_EMAIL: 'e2e@example.invalid',
  GIT_COMMITTER_NAME: 'E2E Curator',
  GIT_COMMITTER_EMAIL: 'e2e@example.invalid',
};

// A skill to put on the hub the way a person does: write it in a project folder, then commit it.
export interface HubSkillSpec {
  id: string;
  // The title people see; set by a metadata edit after the commit.
  name?: string;
  description: string;
  // Markdown after the frontmatter.
  body: string;
  collection?: string;
  operation?: string;
  triggers?: string[];
  notFor?: string[];
  examples?: string[];
  minScope?: string;
  // Other files of the skill, relative to its folder.
  files?: Record<string, string | Buffer>;
  executable?: string[];
}

// createHubSkill adds a draft skill to the hub: `skill commit` of a project folder that was never
// checked out. Routing is given when the skill should be ready to activate.
export function createHubSkill(runCLI: CLIRunner, spec: HubSkillSpec): void {
  const project = fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-e2e-project-'));
  try {
    const folder = path.join(project, '.claude', 'skills', spec.id);
    const write = (rel: string, body: string | Buffer) => {
      const abs = path.join(folder, rel);
      fs.mkdirSync(path.dirname(abs), { recursive: true });
      fs.writeFileSync(abs, body);
    };
    write('SKILL.md', `---\nname: ${spec.id}\ndescription: ${JSON.stringify(spec.description)}\n---\n\n${spec.body}`);
    for (const [rel, body] of Object.entries(spec.files ?? {})) write(rel, body);
    for (const rel of spec.executable ?? []) fs.chmodSync(path.join(folder, rel), 0o755);

    const args = ['skill', 'commit', spec.id, '--collection', spec.collection ?? 'core', '--min-scope', spec.minScope ?? 'single_step'];
    for (const trigger of spec.triggers ?? []) args.push('--trigger', trigger);
    for (const notFor of spec.notFor ?? []) args.push('--not-for', notFor);
    for (const example of spec.examples ?? []) args.push('--example', example);
    args.push('--yes');
    runCLI(args, project);
    // The commit gives the skill the id as its title and no operation; the metadata edit fixes both.
    const edit: string[] = [];
    if (spec.name && spec.name !== spec.id) edit.push('--name', spec.name);
    if (spec.operation) edit.push('--operation', spec.operation);
    if (edit.length > 0) runCLI(['skill', 'edit', spec.id, ...edit, '--yes']);
  } finally {
    fs.rmSync(project, { recursive: true, force: true });
  }
}

// commitHub records everything in the hub as one Git commit, so the next change is not refused for
// files that differ from Git.
export function commitHub(ws: string, message: string, env: NodeJS.ProcessEnv = process.env): void {
  for (const args of [['add', '-A'], ['commit', '--quiet', '--allow-empty', '-m', message]]) {
    execFileSync('git', args, { cwd: ws, env: { ...gitIdentity, ...env }, stdio: 'pipe' });
  }
}

// attachFilesystemSource writes a filesystem source into the workspace catalog,
// attaches it to a skill and checks it, so the Sources and Distill screens have data.
export function attachFilesystemSource(
  ws: string,
  runCLI: CLIRunner,
  id: string,
  skillContent: string,
  skillId: string,
): void {
  const fixtureDir = path.join(ws, 'runtime', 'fixtures', id);
  fs.mkdirSync(fixtureDir, { recursive: true });
  fs.writeFileSync(path.join(fixtureDir, 'SKILL.md'), skillContent);

  const catalogDir = path.join(ws, 'sources', 'catalog');
  fs.mkdirSync(catalogDir, { recursive: true });
  const yaml = `schema_version: 1
id: ${id}
adapter: filesystem
locator:
  path: runtime/fixtures/${id}
status: watching
identity:
  name: ${id}
  canonical: runtime/fixtures/${id}
trust:
  source: test
  reviewed: true
monitoring:
  enabled: true
  cadence: weekly
limits:
  timeout_seconds: 20
  max_bytes: 8388608
  max_files: 100
  max_file_bytes: 2097152
`;
  fs.writeFileSync(path.join(catalogDir, `${id}.yaml`), yaml);
  runCLI(['source', 'attach', id, '--skill-id', skillId, '--yes']);
  runCLI(['source', 'check', id]);
}

export function seedDistillWorkspace(env: NodeJS.ProcessEnv = process.env): SeededDistillWorkspace {
  const ws = fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-seed-distill-'));
  const runCLI = makeCLIRunner(ws, env);

  // 1. init in <ws>; create and activate consumer-review
  runCLI(['init']);

  createHubSkill(runCLI, {
    id: 'consumer-review',
    name: 'Consumer Review',
    description: 'Review consumer changes.',
    body: '# Consumer Review\n\nReview consumer changes carefully.\n',
    triggers: ['review consumer changes'],
    notFor: ['write prose'],
  });
  runCLI(['skill', 'activate', 'consumer-review', '--yes']);

  // 2. Three filesystem sources attached to consumer-review
  const sourceBody = (label: string) => `# Source ${label}\nFirst version of source ${label.toLowerCase()} knowledge.\n`;
  attachFilesystemSource(ws, runCLI, 'source-a', sourceBody('A'), 'consumer-review');
  attachFilesystemSource(ws, runCLI, 'source-b', sourceBody('B'), 'consumer-review');
  attachFilesystemSource(ws, runCLI, 'source-c', sourceBody('C'), 'consumer-review');

  return { ws };
}
