import { execFileSync } from 'node:child_process';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export interface SeededDistillWorkspace {
  ws: string;
  finalizedRunId: string;
  inProgressRunId: string;
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

  // Prepare source-a
  const prepAOut = runCLI(['distill', 'prepare', 'source-a', '--json']);
  const prepA = JSON.parse(prepAOut);
  const runAId = prepA.results[0].run.id as string;

  // Start source-a
  const startAOut = runCLI(['distill', 'start', runAId, '--json']);
  const startA = JSON.parse(startAOut);
  const runA = startA.run;
  // Build submission for source-a
  const skillMdA = fs.readFileSync(path.join(ws, 'runtime', 'fixtures', 'source-a', 'SKILL.md'));
  const digestA = 'sha256:' + crypto.createHash('sha256').update(skillMdA).digest('hex');

  const evidenceA = {
    revision: {
      kind: runA.to_revision.kind,
      value: runA.to_revision.value,
      content_digest: runA.to_revision.content_digest,
    },
    run_id: runA.id,
    package_digest: runA.package_digest,
    path: 'SKILL.md',
    locator: 'SKILL.md',
    digest: digestA,
  };

  const coverageA = (runA.changed_resources || []).map((res: { path: string }) => ({
    resource: res.path,
    status: 'analyzed',
    reason: 'analyzed for distillation',
  }));

  const submissionA = {
    coverage: coverageA,
    findings: [
      {
        stable_key: 'review-practice-one',
        status: 'active',
        what: 'Practice one description',
        vocabulary: ['consumer', 'review'],
        evidence: [evidenceA],
      },
      {
        stable_key: 'review-practice-two',
        status: 'active',
        what: 'Practice two description',
        vocabulary: ['consumer', 'changes'],
        evidence: [evidenceA],
      },
    ],
    insights: [
      {
        stable_key: 'insight-consumer-one',
        skill_id: 'consumer-review',
        recommendation: 'Incorporate practice one into consumer review.',
        observation_ids: ['OBS-source-a--review-practice-one'],
        category: 'reliability',
        priority: 'high',
        rationale: 'Improves review reliability.',
      },
      {
        stable_key: 'insight-consumer-two',
        skill_id: 'consumer-review',
        recommendation: 'Incorporate practice two into consumer review.',
        observation_ids: ['OBS-source-a--review-practice-two'],
        category: 'clarity',
        priority: 'medium',
        rationale: 'Improves review clarity.',
      },
    ],
  };

  const submissionAFile = path.join(os.tmpdir(), `sub-a-${Date.now()}.json`);
  fs.writeFileSync(submissionAFile, JSON.stringify(submissionA, null, 2));
  try {
    runCLI(['distill', 'submit', runAId, '--submission', submissionAFile, '--json']);
  } finally {
    try {
      fs.unlinkSync(submissionAFile);
    } catch {
      // Ignore
    }
  }

  // 5. Repeat steps 2-4 for source-b but stop after distill start (in_progress)
  writeSource('source-b', '# Source B\nFirst version of source b knowledge.\n');
  runCLI(['source', 'attach', 'source-b', '--skill-id', 'consumer-review', '--yes']);
  runCLI(['source', 'check', 'source-b']);

  const prepBOut = runCLI(['distill', 'prepare', 'source-b', '--json']);
  const prepB = JSON.parse(prepBOut);
  const runBId = prepB.results[0].run.id as string;
  runCLI(['distill', 'start', runBId, '--json']);

  // 6. Repeat steps 2-3 for source-c, then only source check source-c
  writeSource('source-c', '# Source C\nFirst version of source c knowledge.\n');
  runCLI(['source', 'attach', 'source-c', '--skill-id', 'consumer-review', '--yes']);
  runCLI(['source', 'check', 'source-c']);

  return {
    ws,
    finalizedRunId: runAId,
    inProgressRunId: runBId,
  };
}
