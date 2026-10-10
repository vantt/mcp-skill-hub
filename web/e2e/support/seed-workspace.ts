import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export interface SeededDistillWorkspace {
  ws: string;
}

export function seedDistillWorkspace(): SeededDistillWorkspace {
  const binaryPath = path.resolve(__dirname, '../../.e2e/skillhub');
  if (!fs.existsSync(binaryPath)) {
    throw new Error(`skillhub binary not found at ${binaryPath}. Run make web-e2e to build.`);
  }

  const ws = fs.mkdtempSync(path.join(os.tmpdir(), 'skillhub-seed-distill-'));

  const runCLI = (args: string[]): string => {
    return execFileSync(binaryPath, args, {
      encoding: 'utf-8',
      env: {
        ...process.env,
        SKILLHUB_WORKSPACE: ws,
      },
      stdio: 'pipe',
    });
  };

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

  const writeSource = (id: string, skillContent: string) => {
    const fixtureDir = path.join(ws, 'runtime', 'fixtures', id);
    fs.mkdirSync(fixtureDir, { recursive: true });
    const skillPath = path.join(fixtureDir, 'SKILL.md');
    fs.writeFileSync(skillPath, skillContent);

    const catalogDir = path.join(ws, 'sources', 'catalog');
    fs.mkdirSync(catalogDir, { recursive: true });
    const catalogPath = path.join(catalogDir, `${id}.yaml`);
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
    fs.writeFileSync(catalogPath, yaml);
  };

  // 2. Write source-a
  writeSource('source-a', '# Source A\nFirst version of source a knowledge.\n');
  runCLI(['source', 'attach', 'source-a', '--skill-id', 'consumer-review', '--yes']);
  runCLI(['source', 'check', 'source-a']);

  writeSource('source-b', '# Source B\nFirst version of source b knowledge.\n');
  runCLI(['source', 'attach', 'source-b', '--skill-id', 'consumer-review', '--yes']);
  runCLI(['source', 'check', 'source-b']);

  // 3. Same for source-c
  writeSource('source-c', '# Source C\nFirst version of source c knowledge.\n');
  runCLI(['source', 'attach', 'source-c', '--skill-id', 'consumer-review', '--yes']);
  runCLI(['source', 'check', 'source-c']);

  return { ws };
}
