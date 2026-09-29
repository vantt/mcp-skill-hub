#!/usr/bin/env node
// migrate-porting-state-backfill.mjs — one-time seed of docs/distillery/state/porting/
// from docs/distillery/porting-log.md's current rows (distillery-state-consumer D4).
//
// NOT a permanent distill.mjs subcommand (per porting-log.md's own
// beads-rust:default-off-migration-skill row) — a migration tool that's
// already done its job shouldn't pollute the CLI's verb list forever.
//
// addPorting always forces status='candidate' (porting-store.mjs's only
// legal entry point), so each row is replayed through the REAL transition
// chain (addPorting -> movePorting x N) to reach its current status — this
// also proves every row's status is actually reachable through porting.mjs's
// TRANSITIONS table, not just copied blind. Idempotent: rows already present
// in the view are skipped.

import fs from "node:fs";
import path from "node:path";
import { initStore, addPorting, movePorting, listPorting } from "forgent/state/porting-store";

const ROOT = process.cwd();
const LOG_FILE = path.join(ROOT, "docs/distillery/porting-log.md");
const STORE_DIR = path.join(ROOT, "docs/distillery/state");

function parseRows(logFile) {
  const lines = fs.readFileSync(logFile, "utf8").split("\n").filter((l) => /^\|/.test(l));
  const header = lines.find((l) => /feature/i.test(l));
  if (!header) throw new Error(`porting-log.md at ${logFile} has no table header row`);
  const cols = header.split("|").map((c) => c.trim().toLowerCase());
  const col = (...wants) => cols.findIndex((c) => wants.some((w) => c.startsWith(w)));
  const iFeature = col("feature");
  const iStatus = col("status");
  if (iFeature < 0 || iStatus < 0) throw new Error("porting-log.md header is missing a Feature or Status column");

  const rows = [];
  for (const line of lines) {
    if (line === header || /^\|\s*-/.test(line)) continue;
    const c = line.split("|").map((x) => x.trim());
    const feature = c[iFeature];
    const status = c[iStatus];
    if (!feature || !status) continue;
    rows.push({ id: feature, status });
  }
  return rows;
}

// The real transition chain from 'candidate' to reach each terminal status,
// per porting.mjs's TRANSITIONS table. 'rejected' uses the direct
// candidate->rejected escape hatch (shorter path; matches porting-log.md's
// evidence — no rejected row's notes indicate it passed through
// planned/in-progress first).
const CHAINS = {
  candidate: [],
  planned: ["planned"],
  "in-progress": ["planned", "in-progress"],
  ported: ["planned", "in-progress", "ported"],
  adapted: ["planned", "in-progress", "adapted"],
  rejected: ["rejected"],
};

function seedRow(dir, row) {
  const chain = CHAINS[row.status];
  if (!chain) throw new Error(`porting-log.md row "${row.id}" has unknown status "${row.status}"`);
  addPorting(dir, { id: row.id });
  for (const to of chain) {
    movePorting(dir, { id: row.id, to });
  }
}

function main() {
  const rows = parseRows(LOG_FILE);
  initStore(STORE_DIR);
  const existing = listPorting(STORE_DIR).porting;

  const counts = {};
  let seeded = 0;
  for (const row of rows) {
    if (existing[row.id]) continue;
    seedRow(STORE_DIR, row);
    counts[row.status] = (counts[row.status] || 0) + 1;
    seeded += 1;
  }

  const summary = Object.entries(counts)
    .map(([status, n]) => `${status}=${n}`)
    .join(" ");
  console.log(`seeded ${seeded} rows: ${summary}`);
}

main();
