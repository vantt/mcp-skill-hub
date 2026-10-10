import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export interface SeededDistillWorkspace {
  ws: string;
}

export type CLIRunner = (args: string[]) => string;

export function resolveBinaryPath(): string {
  const binaryPath = path.resolve(__dirname, '../../.e2e/skillhub');
  if (!fs.existsSync(binaryPath)) {
    throw new Error(`skillhub binary not found at ${binaryPath}. Run make web-e2e to build.`);
  }
  return binaryPath;
}

// makeCLIRunner runs the built skillhub binary against one workspace. Pass env to
// replace the inherited environment (for example an isolated HOME and XDG_*).
export function makeCLIRunner(ws: string, env: NodeJS.ProcessEnv = process.env): CLIRunner {
  const binaryPath = resolveBinaryPath();
  return (args) =>
    execFileSync(binaryPath, args, {
      encoding: 'utf-8',
      env: { ...env, SKILLHUB_WORKSPACE: ws },
      stdio: 'pipe',
    });
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

  // 1. init <ws> --yes; create and activate consumer-review
  runCLI(['init', ws, '--yes']);

  const consumerReviewMd = path.join(os.tmpdir(), `consumer-review-${Date.now()}.md`);
  fs.writeFileSync(
    consumerReviewMd,
    '---\nname: consumer-review\ndescription: Review consumer changes.\n---\n\n# Consumer Review\n\nReview consumer changes carefully.\n',
  );
  try {
    runCLI([
      'skill',
      'create',
      'consumer-review',
      '--collection',
      'core',
      '--name',
      'Consumer Review',
      '--description',
      'Review consumer changes.',
      '--content-file',
      consumerReviewMd,
      '--trigger',
      'review consumer changes',
      '--not-for',
      'write prose',
      '--min-scope',
      'single_step',
      '--yes',
    ]);
    runCLI(['skill', 'activate', 'consumer-review', '--yes']);
  } finally {
    try {
      fs.unlinkSync(consumerReviewMd);
    } catch {
      // Ignore
    }
  }

  // 2. Three filesystem sources attached to consumer-review
  const sourceBody = (label: string) => `# Source ${label}\nFirst version of source ${label.toLowerCase()} knowledge.\n`;
  attachFilesystemSource(ws, runCLI, 'source-a', sourceBody('A'), 'consumer-review');
  attachFilesystemSource(ws, runCLI, 'source-b', sourceBody('B'), 'consumer-review');
  attachFilesystemSource(ws, runCLI, 'source-c', sourceBody('C'), 'consumer-review');

  return { ws };
}
