#!/usr/bin/env node
// distill — portable reference-learning helper. Zero npm deps, Node 18+.
// Sets up and maintains a project-local learning area (docs/distillery/)
// with incremental cursors per source. Markdown stays the source of truth;
// this script only computes deltas and performs mechanical, atomic updates.
//
// Usage:
//   distill.mjs init [--root <dir>]
//   distill.mjs add <name> --type git-repo|paper|living-doc --url <url> [--root <dir>]
//   distill.mjs delta <name> [--root <dir>]
//   distill.mjs seal <name> [--domains all|d1,d2] [--version <v>] [--root <dir>]
//   distill.mjs check [<name>] [--root <dir>]
//   distill.mjs rank [--root <dir>]        # candidates by impact score R*E/F
//   distill.mjs status [--json] [--repo] [--root <dir>]  # learning-area dashboard (local view, no network); --repo = bare source list (name/type/clone path)
//   distill.mjs find <term> [--root <dir>] # print matching entry blocks + matrix rows
//   distill.mjs map [term] [--root <dir>]  # source feature <-> local name for ported/adapted rows
//   distill.mjs move <feature-slug> <to-status> [--root <dir>]  # transition a porting row's Status via the state layer (docs/distillery/state)
//
// Refusal format: ERROR (rule) / WHY (reason) / FIX (next action).

import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

const REFS_DIR = "docs/distillery";
const CLONES_DIR = "upstreams";
const MARK_START = "# DISTILL:START";
const MARK_END = "# DISTILL:END";
const TYPES = ["git-repo", "paper", "living-doc"];

// ---------- small utils ----------

function fail(error, why, fix) {
  console.error(`ERROR: ${error}\nWHY: ${why}\nFIX: ${fix}`);
  process.exit(2);
}

function writeAtomic(file, content) {
  const tmp = `${file}.tmp`;
  fs.writeFileSync(tmp, content);
  fs.renameSync(tmp, file);
}

function git(repoDir, args, opts = {}) {
  try {
    return execFileSync("git", ["-C", repoDir, ...args], {
      encoding: "utf8",
      stdio: ["ignore", "pipe", opts.quietErr ? "ignore" : "pipe"],
    }).trim();
  } catch (e) {
    if (opts.soft) return null;
    throw e;
  }
}

function parseArgs(argv) {
  const args = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith("--")) args[a.slice(2)] = argv[i + 1] && !argv[i + 1].startsWith("--") ? argv[++i] : true;
    else args._.push(a);
  }
  return args;
}

function findRoot(args) {
  if (args.root) return path.resolve(args.root);
  let dir = process.cwd();
  while (true) {
    if (fs.existsSync(path.join(dir, REFS_DIR))) return dir;
    const parent = path.dirname(dir);
    if (parent === dir) return null;
    dir = parent;
  }
}

function requireRoot(args) {
  const root = findRoot(args);
  if (!root)
    fail(
      `no learning area found upward from ${process.cwd()}`,
      `every command except init needs an existing ${REFS_DIR}/ directory`,
      `run "distill.mjs init" at the project root first, or pass --root <dir>`
    );
  return root;
}

// ---------- frontmatter (minimal YAML: flat "key: value" + flow lists) ----------

function parseFrontmatter(text) {
  const m = text.match(/^---\n([\s\S]*?)\n---/);
  if (!m) return null;
  const data = {};
  for (const line of m[1].split("\n")) {
    const kv = line.match(/^([A-Za-z_]+):\s*(.*)$/);
    if (!kv) continue;
    let v = kv[2].replace(/\s+#.*$/, "").trim();
    if (v.startsWith("[") && v.endsWith("]"))
      v = v.slice(1, -1).split(",").map((s) => s.trim()).filter(Boolean);
    data[kv[1]] = v === "null" || v === "" ? null : v;
  }
  return { data, raw: m[0] };
}

function setFrontmatterKeys(text, updates) {
  const fm = parseFrontmatter(text);
  if (!fm) throw new Error("no frontmatter");
  let raw = fm.raw;
  for (const [k, v] of Object.entries(updates)) {
    const rendered = Array.isArray(v) ? `[${v.join(", ")}]` : String(v);
    const re = new RegExp(`^${k}:.*$`, "m");
    if (re.test(raw)) raw = raw.replace(re, `${k}: ${rendered}`);
    else raw = raw.replace(/\n---$/, `\n${k}: ${rendered}\n---`);
  }
  return text.replace(fm.raw, raw);
}

// ---------- shared lookups ----------

function sourcePath(root, name) {
  return path.join(root, REFS_DIR, "sources", `${name}.md`);
}

function loadSource(root, name) {
  const file = sourcePath(root, name);
  if (!fs.existsSync(file)) {
    const avail = fs.existsSync(path.join(root, REFS_DIR, "sources"))
      ? fs.readdirSync(path.join(root, REFS_DIR, "sources")).filter((f) => f.endsWith(".md")).map((f) => f.replace(/\.md$/, "")).join(", ")
      : "(none)";
    fail(`unknown source "${name}"`, `no index file at ${file}`, `pick one of: ${avail} — or create it with "distill.mjs add ${name} --type <t> --url <u>"`);
  }
  const text = fs.readFileSync(file, "utf8");
  const fm = parseFrontmatter(text);
  if (!fm || !fm.data.type)
    fail(`source "${name}" has no parseable frontmatter with a "type" key`, `delta/seal/check need type + cursor fields to operate`, `fix the frontmatter of ${file} (see extract-rules.md in the distill skill)`);
  return { file, text, meta: fm.data };
}

function loadTaxonomy(root) {
  const file = path.join(root, REFS_DIR, "taxonomy.txt");
  if (!fs.existsSync(file)) return [];
  return fs.readFileSync(file, "utf8").split("\n")
    .map((l) => l.replace(/#.*$/, "").trim()).filter(Boolean);
}

function cloneDir(root, name, meta) {
  return path.join(root, meta.local || path.join(CLONES_DIR, name));
}

function today() {
  return new Date().toISOString().slice(0, 10);
}

function listSourceNames(root) {
  const dir = path.join(root, REFS_DIR, "sources");
  return fs.existsSync(dir)
    ? fs.readdirSync(dir).filter((f) => f.endsWith(".md")).map((f) => f.replace(/\.md$/, ""))
    : [];
}

function currentCursor(meta) {
  return meta.last_analyzed_commit || meta.last_analyzed_version || meta.extracted_date || null;
}

// Deep-dives pin the source cursors they analyzed (frontmatter based_on:
// [name@cursor]); when a source moves past a pin, the dive is stale.
function deepDiveStaleness(root) {
  const dir = path.join(root, REFS_DIR, "deep-dives");
  const stale = [];
  if (!fs.existsSync(dir)) return stale;
  for (const f of fs.readdirSync(dir).filter((f) => f.endsWith(".md"))) {
    const fm = parseFrontmatter(fs.readFileSync(path.join(dir, f), "utf8"));
    const based = Array.isArray(fm?.data.based_on) ? fm.data.based_on : [];
    for (const pin of based) {
      const [src, cursor] = pin.split("@");
      const sf = sourcePath(root, src);
      if (!fs.existsSync(sf)) { stale.push(`${f}: based_on unknown source "${src}"`); continue; }
      const cur = currentCursor(parseFrontmatter(fs.readFileSync(sf, "utf8"))?.data || {});
      if (cursor && cur && cursor !== cur) stale.push(`${f}: based on ${src}@${cursor}, source now @${cur}`);
    }
  }
  return stale;
}

// ---------- templates (embedded so the script alone can bootstrap) ----------

const T = {
  taxonomy: `# Learning-domain taxonomy — one kebab-case domain per line, '#' comments allowed.
harness
skills
hooks
workflow
orchestration
context-memory
planning
quality-gates
docs-style
tooling
config-packaging
repo-layout
safety
self-improvement
ux
testing-evals
`,
  intake: `# Intake Queue

Nguồn học mới chờ triage. Gặp gì hay — repo, paper, blog, docs — thả vào đây ngay. Triage: đáng học → \`distill.mjs add <name> --type <t> --url <u>\` rồi xóa dòng; không đáng → xóa kèm lý do.

| Nguồn | Type (đoán) | URL | Ngày thêm | Vì sao đáng chú ý |
|---|---|---|---|---|
`,
  matrix: `# Feature Comparison Matrix

So sánh tính năng giữa các learning sources. Mỗi domain một bảng; ô có ✓ link về entry trong \`sources/<name>.md#<slug>\`. Ký hiệu: ✓ có | ~ một phần | ✗ không | ? chưa khảo sát. Matrix là curated view — chỉ hàng có đối chiếu đáng giá, không exhaustive.
`,
  portingLog: `# Porting Log

Nguồn sự thật duy nhất về trạng thái porting. Tính năng bị từ chối vẫn ghi lại kèm lý do.

- Status: \`candidate\` → \`planned\` → \`in-progress\` → \`ported\` / \`adapted\` / \`rejected\`
- Score \`R# E# F#\` chấm một lần lúc tạo candidate; xếp hạng bằng \`distill.mjs rank\`.
- Local = tên trong project ta sau khi port (bắt buộc khi ported/adapted); tra hai chiều bằng \`distill.mjs map\`.

| Feature | Nguồn | Status | Score | Local | Đích (path) | Commit | Ghi chú / Lý do |
|---|---|---|---|---|---|---|---|
`,
  source: (name, type, url) => {
    const cursor =
      type === "git-repo"
        ? `local: ${CLONES_DIR}/${name}\nlast_analyzed_commit: null\nlast_analyzed_date: null`
        : type === "paper"
          ? `extracted_date: null`
          : `last_analyzed_version: null\nlast_analyzed_date: null`;
    return `---\nname: ${name}\ntype: ${type}\nurl: ${url}\n${cursor}\ndomains_covered: []\n---\n\n# ${name} — Feature Index\n\n> Chưa phân tích. Chạy \`distill.mjs delta ${name}\` để bắt đầu.\n`;
  },
  gitignoreBlock: `${MARK_START}\n/${CLONES_DIR}/\n${MARK_END}\n`,
};

// ---------- commands ----------

function cmdInit(args) {
  const root = args.root ? path.resolve(args.root) : process.cwd();
  const refs = path.join(root, REFS_DIR);
  const created = [];
  for (const d of [path.join(refs, "sources"), path.join(root, CLONES_DIR)])
    if (!fs.existsSync(d)) { fs.mkdirSync(d, { recursive: true }); created.push(path.relative(root, d) + "/"); }
  const files = {
    [path.join(refs, "taxonomy.txt")]: T.taxonomy,
    [path.join(refs, "intake.md")]: T.intake,
    [path.join(refs, "comparison-matrix.md")]: T.matrix,
    [path.join(refs, "porting-log.md")]: T.portingLog,
  };
  for (const [f, content] of Object.entries(files))
    if (!fs.existsSync(f)) { writeAtomic(f, content); created.push(path.relative(root, f)); }
  // Managed .gitignore block — never touch bytes outside the markers.
  const gi = path.join(root, ".gitignore");
  const giText = fs.existsSync(gi) ? fs.readFileSync(gi, "utf8") : "";
  const alreadyIgnored = giText.split("\n").some((l) => l.trim() === `/${CLONES_DIR}/` || l.trim() === `${CLONES_DIR}/`);
  if (giText.includes(MARK_START)) {
    // refresh block content only
    const updated = giText.replace(new RegExp(`${MARK_START}[\\s\\S]*?${MARK_END}\\n?`), T.gitignoreBlock);
    if (updated !== giText) { writeAtomic(gi, updated); created.push(".gitignore (refreshed block)"); }
  } else if (!alreadyIgnored) {
    writeAtomic(gi, giText + (giText.endsWith("\n") || giText === "" ? "" : "\n") + T.gitignoreBlock);
    created.push(".gitignore (appended block)");
  }
  console.log(created.length ? `Initialized learning area at ${root}:\n  - ${created.join("\n  - ")}` : `Learning area at ${root} already up to date — nothing written.`);
  console.log(`Next: drop candidate sources into ${REFS_DIR}/intake.md, then "distill.mjs add <name> --type <t> --url <u>".`);
}

function cmdAdd(args) {
  const [name] = args._;
  if (!name || !args.type || !args.url)
    fail("add needs <name> --type --url", "a source file cannot be scaffolded without its identity and cursor type", `distill.mjs add beegog --type git-repo --url https://github.com/x/y`);
  if (!TYPES.includes(args.type))
    fail(`unknown type "${args.type}"`, `cursor semantics differ per type`, `use one of: ${TYPES.join(" | ")}`);
  const root = requireRoot(args);
  const file = sourcePath(root, name);
  if (fs.existsSync(file))
    fail(`source "${name}" already exists`, `overwriting would destroy its cursor and entries`, `edit ${path.relative(root, file)} directly, or pick another name`);
  writeAtomic(file, T.source(name, args.type, args.url));
  console.log(`Created ${path.relative(root, file)} (type: ${args.type}).`);
  if (args.type === "git-repo") console.log(`Next: git clone ${args.url} ${CLONES_DIR}/${name} && distill.mjs delta ${name}`);
  else if (args.type === "paper") console.log(`Next: save a copy under ${CLONES_DIR}/${name}/, extract once, then "distill.mjs seal ${name}".`);
  else console.log(`Next: fetch ${args.url}, extract, then "distill.mjs seal ${name} --version <v>".`);
}

function deltaGitRepo(root, name, meta) {
  const repo = cloneDir(root, name, meta);
  if (!fs.existsSync(path.join(repo, ".git")))
    fail(`clone missing at ${path.relative(root, repo)}`, `git-repo deltas are computed against a local clone`, `git clone ${meta.url} ${path.relative(root, repo)}`);
  if (git(repo, ["pull", "--ff-only", "--quiet"], { soft: true, quietErr: true }) === null)
    console.error("(warn: pull failed — using local state)");
  const head = git(repo, ["rev-parse", "--short", "HEAD"]);
  const last = meta.last_analyzed_commit;
  if (!last) {
    console.log(`== ${name}: never analyzed — FULL SCAN needed ==\nHEAD: ${head}\nTop-level:\n${git(repo, ["ls-tree", "--name-only", "HEAD"])}`);
    return;
  }
  if (git(repo, ["rev-parse", "--short", `${last}^{commit}`], { soft: true, quietErr: true }) === null)
    fail(`recorded cursor "${last}" is not a commit in the clone`, `the frontmatter was hand-edited badly or the clone was recreated with different history`, `verify history ("git -C ${path.relative(root, repo)} log"), then re-seal with "distill.mjs seal ${name}" after a fresh full scan`);
  if (git(repo, ["rev-parse", "--short", last]) === head) {
    console.log(`== ${name}: up to date (last analyzed = HEAD = ${head}) ==`);
  } else {
    console.log(`== ${name}: commits since ${last} ==`);
    console.log(git(repo, ["log", "--format=%h %ad %s", "--date=short", `${last}..HEAD`]));
    console.log(`\n== changed files ==\n${git(repo, ["diff", "--stat", `${last}..HEAD`])}`);
    console.log(`\nAfter analysis: distill.mjs seal ${name}`);
  }
}

function cmdDelta(args) {
  const [name] = args._;
  if (!name) fail("delta needs <name>", "there is no default source", "distill.mjs delta <name>");
  const root = requireRoot(args);
  const { meta } = loadSource(root, name);
  // domain backfill check (skip when never analyzed — the full scan covers everything)
  const analyzed = meta.last_analyzed_commit || meta.extracted_date || meta.last_analyzed_version;
  const covered = Array.isArray(meta.domains_covered) ? meta.domains_covered : [];
  const missing = loadTaxonomy(root).filter((d) => !covered.includes(d));
  if (analyzed && missing.length)
    console.log(`== ${name}: domains needing BACKFILL (scan current snapshot for these only) ==\n  - ${missing.join("\n  - ")}\n`);
  if (meta.type === "git-repo") return deltaGitRepo(root, name, meta);
  if (meta.type === "paper")
    return console.log(meta.extracted_date
      ? `== ${name}: paper, extracted ${meta.extracted_date} — immutable, no delta ==`
      : `== ${name}: paper, never extracted ==\nRead it once, extract entries, then: distill.mjs seal ${name}`);
  console.log(`== ${name}: living-doc ==\nrecorded: version=${meta.last_analyzed_version || "null"} date=${meta.last_analyzed_date || "null"}\nFetch ${meta.url}, compare changelog/version against the recorded cursor, extract what changed, then: distill.mjs seal ${name} --version <new>`);
}

function cmdSeal(args) {
  const [name] = args._;
  if (!name) fail("seal needs <name>", "sealing writes a cursor into one source file", "distill.mjs seal <name>");
  const root = requireRoot(args);
  const { file, text, meta } = loadSource(root, name);
  const updates = {};
  if (meta.type === "git-repo") {
    const repo = cloneDir(root, name, meta);
    updates.last_analyzed_commit = git(repo, ["rev-parse", "--short", "HEAD"]);
    updates.last_analyzed_date = today();
  } else if (meta.type === "paper") {
    updates.extracted_date = today();
  } else {
    if (!args.version && !meta.last_analyzed_version)
      fail("living-doc seal needs --version <v>", "without a version/date cursor the next delta has nothing to compare against", `distill.mjs seal ${name} --version <v> (use a changelog version or the fetch date)`);
    if (args.version) updates.last_analyzed_version = args.version;
    updates.last_analyzed_date = today();
  }
  if (args.domains) {
    const tax = loadTaxonomy(root);
    const covered = new Set(Array.isArray(meta.domains_covered) ? meta.domains_covered : []);
    for (const d of args.domains === "all" ? tax : String(args.domains).split(",").map((s) => s.trim())) {
      if (tax.length && !tax.includes(d))
        fail(`domain "${d}" is not in taxonomy.txt`, `domains_covered must stay comparable with the taxonomy for backfill detection`, `add it to ${REFS_DIR}/taxonomy.txt first, or fix the spelling`);
      covered.add(d);
    }
    updates.domains_covered = [...covered];
  }
  writeAtomic(file, setFrontmatterKeys(text, updates));
  console.log(`Sealed ${name}: ${Object.entries(updates).map(([k, v]) => `${k}=${Array.isArray(v) ? `[${v.length}]` : v}`).join(", ")}`);
}

function checkSource(root, name, taxonomy, matrixText) {
  const problems = [];
  const { text, meta } = loadSource(root, name);
  if (!TYPES.includes(meta.type)) problems.push(`type "${meta.type}" invalid`);
  const covered = Array.isArray(meta.domains_covered) ? meta.domains_covered : [];
  const unknown = covered.filter((d) => taxonomy.length && !taxonomy.includes(d));
  if (unknown.length) problems.push(`domains_covered not in taxonomy: ${unknown.join(", ")}`);
  const missing = taxonomy.filter((d) => !covered.includes(d));
  const analyzed = meta.last_analyzed_commit || meta.extracted_date || meta.last_analyzed_version;
  if (analyzed && missing.length) problems.push(`backfill needed: ${missing.join(", ")}`);
  if (meta.type === "git-repo" && meta.last_analyzed_commit) {
    const repo = cloneDir(root, name, meta);
    if (!fs.existsSync(path.join(repo, ".git"))) problems.push(`clone missing at ${path.relative(root, repo)}`);
    else {
      if (git(repo, ["rev-parse", `${meta.last_analyzed_commit}^{commit}`], { soft: true, quietErr: true }) === null)
        problems.push(`cursor ${meta.last_analyzed_commit} not a commit in clone`);
      // Where-path spot check: backticked path-like tokens must suffix-match a tracked file.
      // Entries marked Status: moved-to-*/removed legitimately point at paths gone from HEAD — skip them.
      const tracked = git(repo, ["ls-files"]).split("\n");
      const missingPaths = [];
      for (const block of text.split(/^### /m).slice(1)) {
        if (/\*\*Status:\*\*\s*(moved-to-|removed)/.test(block)) continue;
        for (const line of block.match(/^- \*\*Where:\*\*.*$/gm) || [])
          for (const tok of line.match(/`([^`]+)`/g) || []) {
            const p = tok.slice(1, -1).replace(/\/$/, "");
            if (!/[/.]/.test(p) || /[ *|]/.test(p)) continue; // not a concrete path
            if (!tracked.some((f) => f === p || f.startsWith(p + "/") || f.endsWith("/" + p) || f.includes("/" + p + "/"))) missingPaths.push(p);
          }
      }
      if (missingPaths.length) problems.push(`Where paths not found in clone HEAD: ${[...new Set(missingPaths)].join(", ")}`);
    }
  }
  // matrix anchors pointing into this source must resolve to a real "### slug" heading
  const slugs = new Set((text.match(/^### (.+)$/gm) || []).map((h) => h.slice(4).trim()));
  for (const mLink of matrixText.matchAll(new RegExp(`\\]\\(sources/${name}\\.md#([a-z0-9-]+)\\)`, "g")))
    if (!slugs.has(mLink[1])) problems.push(`matrix links to missing anchor #${mLink[1]}`);
  return problems;
}

function cmdCheck(args) {
  const root = requireRoot(args);
  const taxonomy = loadTaxonomy(root);
  const matrixFile = path.join(root, REFS_DIR, "comparison-matrix.md");
  const matrixText = fs.existsSync(matrixFile) ? fs.readFileSync(matrixFile, "utf8") : "";
  const names = args._[0]
    ? [args._[0]]
    : fs.readdirSync(path.join(root, REFS_DIR, "sources")).filter((f) => f.endsWith(".md")).map((f) => f.replace(/\.md$/, ""));
  let bad = 0;
  for (const name of names) {
    const problems = checkSource(root, name, taxonomy, matrixText);
    console.log(problems.length ? `✗ ${name}\n  - ${problems.join("\n  - ")}` : `✓ ${name}`);
    if (problems.length) bad++;
  }
  if (!args._[0]) {
    const stale = deepDiveStaleness(root);
    if (stale.length) { console.log(`✗ deep-dives\n  - ${stale.join("\n  - ")}`); bad++; }
  }
  process.exit(bad ? 1 : 0);
}

function cmdStatus(args) {
  const root = requireRoot(args);
  // --repo: bare list of managed upstreams (name/type/clone path), no pipeline
  // counts or delta network work — answers "which sources do we manage?".
  if (args.repo) {
    const rows = listSourceNames(root).map((name) => {
      const { meta } = loadSource(root, name);
      const rel = meta.local || path.join(CLONES_DIR, name);
      return { name, type: meta.type || "?", rel, hasClone: fs.existsSync(path.join(root, rel)) };
    });
    const wName = Math.max(4, ...rows.map((r) => r.name.length));
    const wType = Math.max(4, ...rows.map((r) => r.type.length));
    console.log("== distill sources (managed upstreams) ==");
    for (const r of rows)
      console.log(`  ${r.hasClone ? "●" : "○"} ${r.name.padEnd(wName)}  ${r.type.padEnd(wType)}  ${r.hasClone ? r.rel : "(no clone)"}`);
    console.log(`  ${rows.length} source(s)`);
    return;
  }
  const taxonomy = loadTaxonomy(root);
  const sources = [];
  for (const name of listSourceNames(root)) {
    const { meta } = loadSource(root, name);
    const covered = Array.isArray(meta.domains_covered) ? meta.domains_covered : [];
    const s = { name, type: meta.type, cursor: currentCursor(meta), backfill: taxonomy.filter((d) => !covered.includes(d)).length };
    if (meta.type === "git-repo" && s.cursor) {
      const repo = cloneDir(root, name, meta);
      if (!fs.existsSync(path.join(repo, ".git"))) s.behind = "no-clone";
      else {
        const n = git(repo, ["rev-list", "--count", `${s.cursor}..HEAD`], { soft: true, quietErr: true });
        s.behind = n === null ? "?" : Number(n);
      }
    }
    sources.push(s);
  }
  const intakeFile = path.join(root, REFS_DIR, "intake.md");
  const intake_pending = Math.max(0, fs.existsSync(intakeFile)
    ? fs.readFileSync(intakeFile, "utf8").split("\n").filter((l) => /^\|/.test(l)).length - 2
    : 0);
  let candidates = 0, unscored = 0;
  const logFile = path.join(root, REFS_DIR, "porting-log.md");
  if (fs.existsSync(logFile))
    for (const line of fs.readFileSync(logFile, "utf8").split("\n")) {
      if (!/^\|/.test(line) || !/\|\s*candidate\s*\|/.test(line)) continue;
      candidates++;
      if (!/R[1-3]\s*E[1-3]\s*F[1-3]/.test(line)) unscored++;
    }
  const stale_deep_dives = deepDiveStaleness(root);
  if (args.json)
    return console.log(JSON.stringify({ sources, intake_pending, candidates, unscored, stale_deep_dives }, null, 2));
  console.log("== distill status (local view — 'delta <name>' pulls upstream) ==");
  for (const s of sources) {
    const behind = s.behind === undefined ? "" : s.behind === 0 ? " · clone even with cursor" : ` · clone ${s.behind} commit(s) past cursor`;
    console.log(`  ${s.cursor ? "●" : "○"} ${s.name} (${s.type}) cursor=${s.cursor || "never"}${behind}${s.backfill ? ` · backfill ${s.backfill} domains` : ""}`);
  }
  console.log(`  intake pending: ${intake_pending} · candidates: ${candidates}${unscored ? ` (${unscored} UNSCORED)` : ""}`);
  if (stale_deep_dives.length) console.log(`  stale deep-dives:\n    - ${stale_deep_dives.join("\n    - ")}`);
  const next = [];
  if (intake_pending) next.push("triage intake");
  for (const s of sources) {
    if (!s.cursor) next.push(`full scan ${s.name}`);
    else if (typeof s.behind === "number" && s.behind > 0) next.push(`extract delta of ${s.name}`);
    if (s.backfill) next.push(`backfill ${s.name}`);
  }
  if (unscored) next.push("score unscored candidates");
  if (stale_deep_dives.length) next.push("refresh stale deep-dives");
  if (next.length) console.log(`  suggested next: ${next.slice(0, 4).join(" · ")}`);
}

function cmdFind(args) {
  const [term] = args._;
  if (!term) fail("find needs <term>", "it searches entry blocks, matrix rows and deep-dives for a term", "distill.mjs find reservation");
  const root = requireRoot(args);
  const rx = new RegExp(term.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "i");
  let hits = 0;
  const matrixFile = path.join(root, REFS_DIR, "comparison-matrix.md");
  if (fs.existsSync(matrixFile)) {
    const rows = fs.readFileSync(matrixFile, "utf8").split("\n").filter((l) => /^\|/.test(l) && rx.test(l));
    if (rows.length) { console.log(`== comparison-matrix.md (${rows.length} rows) ==\n${rows.join("\n")}\n`); hits += rows.length; }
  }
  for (const sub of ["sources", "deep-dives"]) {
    const dir = path.join(root, REFS_DIR, sub);
    if (!fs.existsSync(dir)) continue;
    for (const f of fs.readdirSync(dir).filter((f) => f.endsWith(".md")))
      for (const block of fs.readFileSync(path.join(dir, f), "utf8").split(/^(?=#{2,3} )/m))
        if (block.startsWith("### ") && rx.test(block)) {
          console.log(`== ${sub}/${f} :: ${block.split("\n")[0].slice(4)} ==\n${block.trim()}\n`);
          hits++;
        }
  }
  if (!hits) console.log(`No matches for "${term}". Try a source's own vocabulary (Keywords lines exist for this) or a broader term.`);
}

function cmdRank(args) {
  const root = requireRoot(args);
  const logFile = path.join(root, REFS_DIR, "porting-log.md");
  if (!fs.existsSync(logFile))
    fail("porting-log.md not found", "rank reads candidate rows from the porting log", `run "distill.mjs init" or create ${REFS_DIR}/porting-log.md`);
  const scored = [];
  const unscored = [];
  for (const line of fs.readFileSync(logFile, "utf8").split("\n")) {
    if (!/^\|/.test(line) || !/\|\s*candidate\s*\|/.test(line)) continue;
    const cells = line.split("|").map((c) => c.trim());
    const feature = cells[1] || "?";
    const m = line.match(/R([1-3])\s*E([1-3])\s*F([1-3])/);
    if (m) {
      const [r, e, f] = [Number(m[1]), Number(m[2]), Number(m[3])];
      scored.push({ feature, r, e, f, score: (r * e) / f });
    } else unscored.push(feature);
  }
  scored.sort((a, b) => b.score - a.score || a.f - b.f);
  console.log("Candidates by impact (score = R×E/F — derived, never stored):");
  for (const s of scored)
    console.log(`  ${s.score.toFixed(1).padStart(4)}  R${s.r} E${s.e} F${s.f}  ${s.feature}`);
  if (unscored.length)
    console.log(`\nUnscored candidates (add "R# E# F#" to their Score cell):\n  - ${unscored.join("\n  - ")}`);
  console.log(`\nDeep-dive hints: high R×E rows where the matrix shows sources diverging (hòa/~) — see deep-dive-protocol.md.`);
}

function cmdMap(args) {
  const [term] = args._;
  const root = requireRoot(args);
  const logFile = path.join(root, REFS_DIR, "porting-log.md");
  if (!fs.existsSync(logFile))
    fail("porting-log.md not found", "map reads ported/adapted rows from the porting log", `run "distill.mjs init" first`);
  const lines = fs.readFileSync(logFile, "utf8").split("\n").filter((l) => /^\|/.test(l));
  const header = lines.find((l) => /feature/i.test(l));
  if (!header) fail("porting-log has no table header", "column positions are resolved from the header row", "keep a header row containing 'Feature'");
  const cols = header.split("|").map((c) => c.trim().toLowerCase());
  const col = (...wants) => cols.findIndex((c) => wants.some((w) => c.startsWith(w)));
  const [iF, iSrc, iSt, iLoc, iDest, iCommit] = [col("feature"), col("nguồn", "source"), col("status"), col("local"), col("đích", "dest"), col("commit")];
  if (iLoc < 0)
    fail("no 'Local' column in porting-log.md", "the source↔local mapping lives in that column", "add a Local column (see extract-rules.md §Porting-log rules)");
  const rx = term ? new RegExp(term.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "i") : null;
  const rows = [], missing = [];
  for (const line of lines) {
    if (line === header || /^\|\s*-/.test(line)) continue;
    const c = line.split("|").map((x) => x.trim());
    if (!/^(ported|adapted)$/.test(c[iSt] || "")) continue;
    if (rx && !rx.test(line)) continue;
    const row = { src: c[iSrc], feature: c[iF], local: c[iLoc], dest: c[iDest], commit: c[iCommit], status: c[iSt] };
    if (!row.local || row.local === "—" || row.local === "-") missing.push(row.feature);
    rows.push(row);
  }
  if (!rows.length) return console.log(term ? `No ported/adapted rows match "${term}".` : "Nothing ported/adapted yet — mappings appear once candidates ship.");
  console.log("source feature  →  local name  (destination) [commit]");
  for (const r of rows) console.log(`  ${r.src}\n    → ${r.local}  (${r.dest}) [${r.commit}, ${r.status}]`);
  if (missing.length) console.log(`\nRows missing a Local name (fill them): ${missing.join(", ")}`);
}

// Transition one porting row's Status through the forgent state layer (D2/D3),
// then regenerate porting-log.md's Status column from the fresh view so the
// markdown stays a derived view, self-healing against drift. Only the Status
// column is state-derived; every other column stays hand-edited (D3).
async function cmdMove(args) {
  const [featureSlug, toStatus] = args._;
  if (!featureSlug || !toStatus)
    fail(
      "move needs <feature-slug> <to-status>",
      "move transitions one porting row's Status through the state layer, then resyncs porting-log.md",
      "distill.mjs move <feature-slug> <candidate|planned|in-progress|ported|adapted|rejected>"
    );
  const root = requireRoot(args);
  const stateDir = path.join(root, REFS_DIR, "state");
  const logFile = path.join(root, REFS_DIR, "porting-log.md");

  // Lazy import: distill.mjs declares zero npm deps and runs standalone in any
  // project — a top-level static import of the installed forgent package would
  // crash EVERY command wherever the dependency is absent, not just this verb.
  const { movePorting, listPorting } = await import("forgent/state/porting-store");

  try {
    movePorting(stateDir, { id: featureSlug, to: toStatus });
  } catch (e) {
    fail(
      `cannot move "${featureSlug}" to "${toStatus}"`,
      e && e.message ? e.message : String(e),
      `check the current status ("distill.mjs status" / "map") and legal transitions: candidate→planned→in-progress→ported/adapted/rejected`
    );
  }

  // Resync: rewrite ONLY each row's Status cell from the fresh view, preserving
  // every other cell's exact text/padding (targeted per-cell replace, never a
  // whole-line re-split/re-join). Reuses cmdMap's header-driven column detection.
  const view = listPorting(stateDir).porting;
  const text = fs.readFileSync(logFile, "utf8");
  const lines = text.split("\n");
  const header = lines.find((l) => /^\|/.test(l) && /feature/i.test(l));
  if (!header)
    fail(
      "porting-log.md has no table header",
      "the Status column position is resolved from a header row containing 'Feature'",
      "keep the standard porting-log.md header row (Feature … Status …)"
    );
  const cols = header.split("|").map((c) => c.trim().toLowerCase());
  const col = (...wants) => cols.findIndex((c) => wants.some((w) => c.startsWith(w)));
  const iF = col("feature");
  const iSt = col("status");
  if (iF < 0 || iSt < 0)
    fail(
      "porting-log.md header missing a Feature or Status column",
      "resync needs both the id (Feature) and Status column positions",
      "restore the standard porting-log.md header row"
    );

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (!/^\|/.test(line) || line === header || /^\|\s*-/.test(line)) continue;
    const segments = line.split("|");
    const id = (segments[iF] || "").trim();
    const record = view[id];
    // A state id with no matching markdown row (e.g. a seeded future entry) —
    // and a markdown row with no state record — are both left untouched.
    if (!record) continue;
    const seg = segments[iSt];
    if (seg === undefined) continue;
    const lead = seg.match(/^\s*/)[0];
    const trail = seg.match(/\s*$/)[0];
    segments[iSt] = lead + record.status + trail;
    lines[i] = segments.join("|");
  }
  writeAtomic(logFile, lines.join("\n"));
  console.log(`Moved ${featureSlug} → ${toStatus}; porting-log.md Status column resynced from the state view.`);
}

// ---------- dispatch ----------

const [cmd, ...rest] = process.argv.slice(2);
const args = parseArgs(rest);
const commands = { init: cmdInit, add: cmdAdd, delta: cmdDelta, seal: cmdSeal, check: cmdCheck, rank: cmdRank, status: cmdStatus, find: cmdFind, map: cmdMap, move: cmdMove };
if (!commands[cmd])
  fail(`unknown command "${cmd || ""}"`, "distill only automates the mechanical parts of the learning lifecycle", `use one of: ${Object.keys(commands).join(" | ")} (see header comment for flags)`);
await commands[cmd](args);
