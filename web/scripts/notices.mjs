/* global console */
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const webDir = path.resolve(__dirname, '..');
const lockPath = path.join(webDir, 'package-lock.json');
const outputPath = path.join(webDir, 'THIRD_PARTY_NOTICES.md');

const lock = JSON.parse(fs.readFileSync(lockPath, 'utf8'));
const rootDeps = Object.keys(lock.packages?.['']?.dependencies || {});

function resolveDep(fromPath, depName) {
  let current = fromPath;
  while (current.length > 0) {
    const candidate = current + '/node_modules/' + depName;
    if (lock.packages[candidate]) return candidate;
    const lastNM = current.lastIndexOf('/node_modules/');
    if (lastNM === -1) break;
    current = current.slice(0, lastNM);
  }
  const rootCand = 'node_modules/' + depName;
  if (lock.packages[rootCand]) return rootCand;
  return null;
}

const reachable = new Map();
const queue = [];
for (const dep of rootDeps) {
  const resolved = resolveDep('', dep);
  if (resolved) queue.push(resolved);
}

while (queue.length > 0) {
  const p = queue.shift();
  if (reachable.has(p)) continue;
  const pkgInfo = lock.packages[p];
  if (!pkgInfo) continue;
  const name = pkgInfo.name || p.replace(/^(.*node_modules\/)/, '');
  reachable.set(p, { name, version: pkgInfo.version, path: p, license: pkgInfo.license });
  const deps = Object.assign({}, pkgInfo.dependencies || {});
  for (const dep of Object.keys(deps)) {
    const resolved = resolveDep(p, dep);
    if (resolved && !reachable.has(resolved)) queue.push(resolved);
  }
}

const packages = Array.from(reachable.values());
packages.sort((a, b) => a.name.localeCompare(b.name));

let output = '# Third-Party Software Notices\n\n';
output += 'This document contains third-party software notices and licenses for dependencies used in the Skill Hub Web UI production bundle.\n\n';

for (const pkg of packages) {
  const fullPkgDir = path.join(webDir, pkg.path);
  let licenseField = pkg.license;
  if (!licenseField) {
    try {
      const pkgJson = JSON.parse(fs.readFileSync(path.join(fullPkgDir, 'package.json'), 'utf8'));
      licenseField = pkgJson.license || (pkgJson.licenses ? JSON.stringify(pkgJson.licenses) : 'UNKNOWN');
    } catch {
      licenseField = 'UNKNOWN';
    }
  }

  let licenseText = '';
  try {
    const files = fs.readdirSync(fullPkgDir);
    const licenseFileName = files.find((f) => /^licen[sc]e/i.test(f) || /^copying/i.test(f));
    if (licenseFileName) {
      licenseText = fs.readFileSync(path.join(fullPkgDir, licenseFileName), 'utf8').trim();
    }
  } catch (err) {
    void err;
  }

  output += `## ${pkg.name}@${pkg.version}\n\n`;
  output += `License: ${licenseField}\n\n`;
  if (licenseText) {
    output += '```\n' + licenseText + '\n```\n\n';
  }
}

fs.writeFileSync(outputPath, output.trimEnd() + '\n', 'utf8');
console.log(`Generated notices for ${packages.length} packages at ${outputPath}`);
