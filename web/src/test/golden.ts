import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export function loadGolden<T = unknown>(name: string): T {
  const currentDir = path.dirname(fileURLToPath(import.meta.url));
  const goldenPath = path.resolve(
    currentDir,
    '../../../internal/delivery/web/testdata/golden',
    `${name}.json`,
  );
  const raw = fs.readFileSync(goldenPath, 'utf-8');
  return JSON.parse(raw) as T;
}
