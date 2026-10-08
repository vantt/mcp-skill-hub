#!/usr/bin/env node
// distill — portable reference-learning helper. Node 18+, no npm install:
// the only dependency, js-yaml, is vendored next to this file.
//
// The learning area (docs/distillery/) is YAML that this script validates,
// scores, sorts and rewrites in one canonical layout. Agents and humans edit
// the files freely; every write through this script is all-or-nothing: when
// anything is invalid it lists the errors and writes nothing.
//
// Usage:
//   distill.mjs init                                   create the learning area
//   distill.mjs intake <name> --url <u> [--type t] [--why text]   capture a source for triage
//   distill.mjs add <name> --type git-repo|paper|living-doc --url <u> [--ref r] [--local dir]
//   distill.mjs delta <name>                           what to read since the cursor
//   distill.mjs format [--no-where]                    validate, score, sort, rewrite
//   distill.mjs seal <name> [--version v] [--domains all|d1,d2] [--backfill]
//                                                      validate everything, then move the cursor
//   distill.mjs check [--no-where]                     validate only (exit 1 on errors)
//   distill.mjs rank [--source s] [--domain d] [--state st] [--top N | --all]
//   distill.mjs decide <n|key>... --state <st> [--reason r] [--local name] [--host path]
//   distill.mjs outcome <key> <confirmed|ineffective|adjusted> <note...>
//   distill.mjs status [--json] [--repo]               local dashboard (no network)
//   distill.mjs find <term>                            matching lessons, deep-dives, matrix rows
//   distill.mjs map [term]                             source lesson <-> local name, for ported/adapted
//   distill.mjs domain-rename <old> <new> [definition...]  rename a domain everywhere, in one write
//   distill.mjs migrate [--dry-run] [--force]          convert a v1 markdown area to this layout
// Every command accepts --root <dir> (default: nearest directory upward with docs/distillery/).
//
// Refusal format: ERROR (rule) / WHY (reason) / FIX (next action).

import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import yaml from "./vendor/js-yaml.mjs";

const AREA = "docs/distillery";
const CONFIG = "distill.yaml";
const LESSONS = "lessons";
const CLONES_DIR = "upstreams";
const MARK_START = "# DISTILL:START";
const MARK_END = "# DISTILL:END";
const TYPES = ["git-repo", "paper", "living-doc"];
const LAYERS = ["enforcement", "content", "history", "validation", "craft", "wording", "ecosystem"];
const CONTRASTS = ["new", "extends", "contradicts", "already-covered"];
const STATES = ["candidate", "planned", "in-progress", "ported", "adapted", "rejected"];
const FACTS = ["a", "b", "c", "d", "e"];
const OUTCOMES = ["confirmed", "ineffective", "adjusted"];
const UNCLASSIFIED = "unclassified";
const KEY_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const REF_RE = /^([a-z0-9][a-z0-9-]*)@([A-Za-z0-9._-]+)(?::([^#\s]+)(?:#(\S+))?)?$/;
const SHA_RE = /^[0-9a-f]{7,40}$/;
const LINES_RE = /^L(\d+)(?:-L(\d+))?$/;
const STATUS_RE = /^(removed|superseded-by:[a-z0-9-]+|moved-to:\S+)$/;
const DEFAULT_DOMAINS = ["harness", "skills", "hooks", "workflow", "orchestration", "context-memory", "planning",
  "quality-gates", "docs-style", "tooling", "config-packaging", "repo-layout", "safety", "self-improvement", "ux",
  "testing-evals"];

const CONFIG_ORDER = ["goal", "domains", "sources", "intake"];
const GOAL_ORDER = ["status", "purpose", "core_domains", "in_scope", "out_of_scope", "failures_it_prevents"];
const SOURCE_ORDER = ["name", "type", "url", "ref", "local", "scope", "derived_from", "upstream_of_host", "cursor",
  "analyzed", "domains_covered", "coverage", "notes"];
const LESSON_ORDER = ["key", "domains", "layer", "what", "notable", "where", "legacy_where", "contrast", "host", "score",
  "final_score", "keywords", "also_fits", "status", "found_by", "outcome", "decision", "legacy"];
const SCORE_ORDER = ["relevance", "facts", "impact", "evidence", "effort", "why"];
const DECISION_ORDER = ["state", "reason", "at", "local"];

// ---------- small utils ----------

function fail(error, why, fix) {
  console.error(`ERROR: ${error}\nWHY: ${why}\nFIX: ${fix}`);
  process.exit(2);
}

function failList(errors, what = "the learning area is invalid") {
  console.error(`ERROR: ${what}; nothing was written`);
  for (const e of errors) console.error(`  - ${e}`);
  console.error("FIX: correct each item above, then rerun the same command");
  process.exit(1);
}

function git(repoDir, args, opts = {}) {
  try {
    return execFileSync("git", ["-C", repoDir, ...args], {
      encoding: "utf8",
      input: opts.input,
      maxBuffer: 256 * 1024 * 1024,
      stdio: [opts.input === undefined ? "ignore" : "pipe", "pipe", opts.quietErr ? "ignore" : "pipe"],
    }).trimEnd();
  } catch (e) {
    if (opts.soft) return null;
    throw e;
  }
}

function parseArgs(argv) {
  const args = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith("--")) args[a.slice(2)] = argv[i + 1] !== undefined && !argv[i + 1].startsWith("--") ? argv[++i] : true;
    else args._.push(a);
  }
  return args;
}

function today() {
  return new Date().toISOString().slice(0, 10);
}

const text = (v) => typeof v === "string" && v.trim() !== "";
const textList = (v) => Array.isArray(v) && v.every(text);
const isInt = (v, lo, hi) => Number.isInteger(v) && v >= lo && v <= hi;
const asList = (v) => (Array.isArray(v) ? v : []);
const short = (s, n = 110) => { const t = String(s || "").replace(/\s+/g, " ").trim(); return t.length > n ? t.slice(0, n - 1) + "…" : t; };

function ordered(obj, order) {
  const out = {};
  for (const k of order) if (obj[k] !== undefined) out[k] = obj[k];
  for (const k of Object.keys(obj)) if (!(k in out) && obj[k] !== undefined) out[k] = obj[k];
  return out;
}

function findRoot(args) {
  if (args.root) return path.resolve(args.root);
  let dir = process.cwd();
  while (true) {
    if (fs.existsSync(path.join(dir, AREA))) return dir;
    const parent = path.dirname(dir);
    if (parent === dir) return null;
    dir = parent;
  }
}

function isV1(root) {
  const area = path.join(root, AREA);
  return !fs.existsSync(path.join(area, CONFIG)) &&
    ["taxonomy.txt", "porting-log.md", "sources"].some((f) => fs.existsSync(path.join(area, f)));
}

function requireRoot(args) {
  const root = findRoot(args);
  if (!root)
    fail(`no learning area found upward from ${process.cwd()}`, `every command except init needs an existing ${AREA}/ directory`,
      `run "distill.mjs init" at the project root first, or pass --root <dir>`);
  if (isV1(root))
    fail(`${AREA} uses the old markdown layout (taxonomy.txt, sources/*.md, porting-log.md)`,
      `lessons, scores and decisions now live in ${CONFIG} and ${LESSONS}/<domain>.yaml, which this script validates`,
      `commit the area, then run "distill.mjs migrate --dry-run" to preview and "distill.mjs migrate" to convert`);
  if (!fs.existsSync(path.join(root, AREA, CONFIG)))
    fail(`${AREA}/${CONFIG} is missing`, `it holds the goal, domains and sources every command reads`, `run "distill.mjs init"`);
  return root;
}

function loadYaml(file) {
  try {
    return yaml.load(fs.readFileSync(file, "utf8"), { schema: yaml.CORE_SCHEMA });
  } catch (e) {
    fail(`${file} is not valid YAML`, String(e.message || e).split("\n")[0], "correct the syntax and rerun");
  }
}

// ---------- area model ----------

function loadArea(root) {
  const area = path.join(root, AREA);
  const config = loadYaml(path.join(area, CONFIG)) || {};
  const lessons = [];
  const errors = [];
  const dir = path.join(area, LESSONS);
  const files = fs.existsSync(dir) ? fs.readdirSync(dir).filter((f) => f.endsWith(".yaml")).sort() : [];
  for (const f of files) {
    const data = loadYaml(path.join(dir, f));
    if (data == null) continue;
    if (typeof data !== "object" || !Array.isArray(data.lessons)) { errors.push(`${LESSONS}/${f}: top level must be "lessons: [...]"`); continue; }
    for (const k of Object.keys(data)) if (k !== "lessons") errors.push(`${LESSONS}/${f}: unknown top-level key ${k}`);
    for (const l of data.lessons) {
      if (l && typeof l === "object") Object.defineProperty(l, "_file", { value: f, enumerable: false, writable: true });
      lessons.push(l);
    }
  }
  return { root, config, lessons, files, loadErrors: errors };
}

const sourcesOf = (config) => asList(config.sources).filter((s) => s && typeof s === "object");
const sourceByName = (config, name) => sourcesOf(config).find((s) => s.name === name);
const domainNames = (config) => asList(config.domains).map((d) => (typeof d === "string" ? d : d && d.name)).filter(Boolean);
const cloneDir = (root, s) => path.join(root, s.local || path.join(CLONES_DIR, s.name));
const parseRef = (ref) => { const m = typeof ref === "string" && ref.match(REF_RE); return m ? { src: m[1], at: m[2], path: m[3], anchor: m[4] } : null; };
const retired = (l) => typeof l.status === "string" && l.status !== "";

// Impact is the number of the five impact facts that hold (see SKILL.md); it is
// derived, never typed. A lesson is ranked only when it is fully scored, inside
// the goal (relevance >= 1), not already covered by the host, and not retired.
function derive(l) {
  const s = l.score && typeof l.score === "object" ? l.score : {};
  const impact = Array.isArray(s.facts) ? s.facts.length : null;
  const scored = isInt(s.relevance, 0, 3) && impact !== null && isInt(s.evidence, 1, 3) && isInt(s.effort, 1, 3);
  let group = "ranked";
  if (retired(l)) group = "retired";
  else if (!scored) group = "unscored";
  else if (s.relevance === 0) group = "outside-goal";
  else if (l.contrast === "already-covered") group = "already-covered";
  const final = group === "ranked" ? Math.round((s.relevance * impact * s.evidence / s.effort) * 100) / 100 : null;
  return { impact, final, group };
}

function sortLessons(lessons) {
  const meta = new Map(lessons.map((l) => [l, derive(l)]));
  const layerIdx = (l) => (LAYERS.includes(l.layer) ? LAYERS.indexOf(l.layer) : LAYERS.length);
  return [...lessons].sort((a, b) => {
    const ma = meta.get(a), mb = meta.get(b);
    const ra = ma.group === "ranked", rb = mb.group === "ranked";
    if (ra !== rb) return ra ? -1 : 1;
    if (ra) {
      const d = mb.final - ma.final || layerIdx(a) - layerIdx(b) || b.score.evidence - a.score.evidence ||
        a.score.effort - b.score.effort || asList(b.where).length - asList(a.where).length;
      if (d) return d;
    }
    return String(a.key).localeCompare(String(b.key));
  });
}

// ---------- validation ----------

function validate(area, { where = true } = {}) {
  const { config, lessons, root } = area;
  const errors = [...area.loadErrors];
  const warnings = [];
  for (const k of Object.keys(config)) if (!CONFIG_ORDER.includes(k)) errors.push(`${CONFIG}: unknown top-level key ${k}`);

  // goal
  const g = config.goal;
  if (!g || typeof g !== "object") errors.push(`${CONFIG}: goal is missing`);
  else {
    if (!["draft", "confirmed"].includes(g.status)) errors.push("goal.status must be draft or confirmed");
    const lists = GOAL_ORDER.slice(3);
    if (g.status === "confirmed") {
      if (!text(g.purpose)) errors.push("goal.purpose must be written before the goal is confirmed");
      for (const f of lists) if (!(textList(g[f]) && g[f].length)) errors.push(`goal.${f} must be a non-empty list before the goal is confirmed`);
    } else {
      if (!text(g.purpose)) warnings.push("goal is empty: relevance and impact have no yardstick until it is written and confirmed");
      else warnings.push("goal is a draft: ask the human to confirm it; scores made against a draft may need rescoring");
      for (const f of lists) if (g[f] !== undefined && !textList(g[f])) errors.push(`goal.${f} must be a list of text`);
    }
    for (const k of Object.keys(g)) if (!GOAL_ORDER.includes(k)) errors.push(`goal: unknown field ${k}`);
  }
  const core = asList(g && g.core_domains);
  if (g && g.core_domains !== undefined && !textList(g.core_domains)) errors.push("goal.core_domains must be a list of domain names");

  // domains
  const domains = domainNames(config);
  if (!Array.isArray(config.domains) || !domains.length) errors.push(`${CONFIG}: domains must list at least one domain`);
  const seenDomain = new Set();
  const undefinedDomains = [];
  for (const d of asList(config.domains)) {
    const name = typeof d === "string" ? d : d && d.name;
    if (!(typeof name === "string" && KEY_RE.test(name))) { errors.push(`domains: ${JSON.stringify(d)} needs a kebab-case name`); continue; }
    if (name === UNCLASSIFIED) errors.push(`domains: "${UNCLASSIFIED}" is reserved for lessons awaiting a taxonomy decision`);
    if (seenDomain.has(name)) errors.push(`domains: duplicate ${name}`);
    seenDomain.add(name);
    if (typeof d !== "object" || !text(d.definition)) undefinedDomains.push(name);
  }
  if (undefinedDomains.length) warnings.push(`${undefinedDomains.length} domain(s) without definition: ${undefinedDomains.join(", ")} (consult mode maps features to domains by definition)`);

  for (const d of core) if (!domainNames(config).includes(d)) errors.push(`goal.core_domains names unknown domain ${d}`);

  // sources
  const names = new Set();
  for (const s of asList(config.sources)) {
    const at = `source ${s && s.name ? s.name : JSON.stringify(s)}`;
    if (!s || typeof s !== "object" || !(typeof s.name === "string" && KEY_RE.test(s.name))) { errors.push(`${at}: needs a kebab-case name`); continue; }
    if (names.has(s.name)) errors.push(`${at}: duplicate name`);
    names.add(s.name);
    if (!TYPES.includes(s.type)) errors.push(`${at}: type must be one of ${TYPES.join(", ")}`);
    if (!text(s.url)) errors.push(`${at}: url is required`);
    for (const k of Object.keys(s)) if (!SOURCE_ORDER.includes(k)) errors.push(`${at}: unknown field ${k}`);
    if (s.cursor != null && !(typeof s.cursor === "string" && /^[A-Za-z0-9._-]+$/.test(s.cursor))) errors.push(`${at}: cursor must be a commit, version or date`);
    if (s.type === "git-repo" && s.cursor && !SHA_RE.test(s.cursor)) errors.push(`${at}: a git-repo cursor must be a commit sha`);
    for (const d of asList(s.domains_covered)) if (!domains.includes(d)) errors.push(`${at}: domains_covered has unknown domain ${d}`);
    for (const d of asList(s.derived_from)) if (!sourceByName(config, d)) errors.push(`${at}: derived_from names unknown source ${d}`);
    if (s.upstream_of_host !== undefined && typeof s.upstream_of_host !== "boolean") errors.push(`${at}: upstream_of_host must be true or false`);
    if (s.coverage !== undefined) {
      const c = s.coverage;
      if (!c || typeof c !== "object" || !text(String(c.at || ""))) errors.push(`${at}: coverage.at must name the commit or version the pass read`);
      else {
        if (!textList(asList(c.read)) || !asList(c.read).length) errors.push(`${at}: coverage.read must list what the pass read`);
        for (const n of asList(c.not_read))
          if (!n || typeof n !== "object" || !text(n.path) || !text(n.reason)) errors.push(`${at}: every coverage.not_read item needs path and reason`);
      }
    }
    if (s.cursor) {
      const missing = domains.filter((d) => !asList(s.domains_covered).includes(d));
      if (missing.length) warnings.push(`${at}: backfill needed for ${missing.join(", ")}`);
      if (!s.coverage) warnings.push(`${at}: no coverage recorded (the next seal will require one)`);
    }
  }
  for (const it of asList(config.intake))
    if (!it || typeof it !== "object" || !text(it.name) || !text(it.url)) errors.push(`intake: every item needs name and url (${JSON.stringify(it)})`);

  // lessons
  const keys = new Map();
  for (const l of lessons) if (l && typeof l === "object" && typeof l.key === "string") keys.set(l.key, (keys.get(l.key) || 0) + 1);
  lessons.forEach((l, i) => {
    let at = `lesson #${i + 1}`;
    if (!l || typeof l !== "object") { errors.push(`${at}: must be a mapping`); return; }
    if (!(typeof l.key === "string" && KEY_RE.test(l.key))) errors.push(`${at}: key must be kebab-case`);
    else { at = `lesson ${l.key}`; if (keys.get(l.key) > 1) errors.push(`${at}: duplicate key`); }
    const legacy = l.legacy === true;
    const soft = legacy ? warnings : errors;
    for (const k of Object.keys(l)) if (!LESSON_ORDER.includes(k)) errors.push(`${at}: unknown field ${k}`);
    if (l.legacy !== undefined && l.legacy !== true) errors.push(`${at}: legacy may only be true`);
    if (!(Array.isArray(l.domains) && l.domains.length)) errors.push(`${at}: domains must list at least the primary domain`);
    else for (const d of l.domains) if (!(domains.includes(d) || d === UNCLASSIFIED)) errors.push(`${at}: unknown domain ${d} (add it to ${CONFIG} domains or use ${UNCLASSIFIED})`);
    if (!LAYERS.includes(l.layer)) (legacy && l.layer == null ? () => {} : (m) => errors.push(m))(`${at}: layer must be one of ${LAYERS.join(", ")}`);
    if (!text(l.what)) errors.push(`${at}: what must be non-empty text`);
    if (!text(l.notable)) soft.push(`${at}: notable must name the mechanism, its trade-off and what it means for the goal`);
    if (!Array.isArray(l.where) || (!legacy && !l.where.length)) errors.push(`${at}: where must list at least one source@commit[:path#Lx-Ly]`);
    for (const ref of asList(l.where)) {
      const r = parseRef(ref);
      if (!r) { errors.push(`${at}: bad where ${JSON.stringify(ref)} (use source@commit:path#Lx-Ly or source@commit)`); continue; }
      const s = sourceByName(config, r.src);
      if (!s) { errors.push(`${at}: where cites unknown source ${r.src}`); continue; }
      if (s.type === "git-repo" && !SHA_RE.test(r.at)) soft.push(`${at}: ${ref} must pin a commit sha for git source ${r.src}`);
      if (s.type === "git-repo" && r.anchor && !LINES_RE.test(r.anchor)) errors.push(`${at}: ${ref} line anchor must be #Lx or #Lx-Ly`);
    }
    if (l.legacy_where !== undefined && !text(l.legacy_where)) errors.push(`${at}: legacy_where must be text`);
    if (!CONTRASTS.includes(l.contrast)) (legacy && l.contrast == null ? () => {} : (m) => errors.push(m))(`${at}: contrast must be one of ${CONTRASTS.join(", ")}`);
    if (l.host !== undefined && !text(l.host)) errors.push(`${at}: host must be a host path or name`);
    if (l.contrast === "already-covered" && !l.host) warnings.push(`${at}: already-covered without host (say where the host already does it)`);

    const s = l.score;
    if (!s || typeof s !== "object") errors.push(`${at}: score is missing`);
    else {
      for (const k of Object.keys(s)) if (!SCORE_ORDER.includes(k)) errors.push(`${at}: unknown score field ${k}`);
      const chk = (name, ok, msg) => { if (s[name] == null && legacy) return; if (!ok) errors.push(`${at}: score.${name} ${msg}`); };
      chk("relevance", isInt(s.relevance, 0, 3), "must be an integer 0-3");
      chk("evidence", isInt(s.evidence, 1, 3), "must be an integer 1-3");
      chk("effort", isInt(s.effort, 1, 3), "must be an integer 1-3");
      chk("facts", Array.isArray(s.facts) && s.facts.every((f) => FACTS.includes(f)) && new Set(s.facts).size === s.facts.length,
        `must list the impact facts that hold, from ${FACTS.join(" ")} (empty list = impact 0)`);
      if (Array.isArray(s.facts) && s.facts.length && !s.facts.includes("a"))
        errors.push(`${at}: score.facts needs fact a (a concrete failure the host has today or its written design would cause) before any other fact counts`);
      if (s.impact !== undefined && Array.isArray(s.facts) && s.impact !== s.facts.length)
        warnings.push(`${at}: score.impact ${s.impact} ≠ ${s.facts.length} facts; format rewrites it`);
      if (!text(s.why)) errors.push(`${at}: score.why must explain the weights that drive the priority`);
    }
    if (l.keywords !== undefined && !textList(l.keywords)) errors.push(`${at}: keywords must be a list of text`);
    if (l.also_fits !== undefined && !(Array.isArray(l.also_fits) && l.also_fits.every((x) => typeof x === "string" && /^[a-z0-9][a-z0-9:-]*$/.test(x))))
      errors.push(`${at}: also_fits entries look like project:<name>, skill:<name> or upstream`);
    if (l.status !== undefined) {
      if (!(typeof l.status === "string" && STATUS_RE.test(l.status))) errors.push(`${at}: status must be removed, superseded-by:<key> or moved-to:<name>`);
      else if (l.status.startsWith("superseded-by:") && !keys.has(l.status.slice(14))) errors.push(`${at}: ${l.status} names no existing lesson`);
    }
    if (l.found_by !== undefined && l.found_by !== "human") errors.push(`${at}: found_by may only be human`);
    const d = l.decision;
    if (!d || typeof d !== "object" || !STATES.includes(d.state)) errors.push(`${at}: decision.state must be one of ${STATES.join(", ")}`);
    else {
      for (const k of Object.keys(d)) if (!DECISION_ORDER.includes(k)) errors.push(`${at}: unknown decision field ${k}`);
      if (d.state === "rejected" && !text(d.reason)) errors.push(`${at}: a rejected decision needs a reason (it stops re-evaluation)`);
      if (["ported", "adapted"].includes(d.state) && !text(l.host)) errors.push(`${at}: ${d.state} needs host (where it landed in the host)`);
    }
    if (l.outcome !== undefined) {
      const o = l.outcome;
      if (!o || typeof o !== "object" || !OUTCOMES.includes(o.result) || !text(o.note)) errors.push(`${at}: outcome needs result (${OUTCOMES.join("|")}) and note`);
    }

    // A lesson whose primary domain is a core domain serves the goal's purpose
    // unless the scorer says why it is peripheral.
    if (core.includes(asList(l.domains)[0]) && !retired(l) && isInt(s?.relevance, 0, 3) && s.relevance < 3 && !/peripheral:/i.test(String(s.why || "")))
      warnings.push(`${at}: primary domain ${l.domains[0]} is a core domain but relevance is ${s.relevance}; score 3, or start a sentence of why with "peripheral:" saying why it is not core`);

    // Independence: evidence 3 claims convergence (or data). Sources linked by
    // derived_from are one line of descent, so they count once.
    const m = derive(l);
    if (m.group === "ranked" && s.evidence === 3) {
      const groups = independentGroups(config, asList(l.where).map((r) => parseRef(r)?.src).filter(Boolean));
      if (groups < 2) warnings.push(`${at}: evidence 3 rests on one independent source; keep it only for a removal or change backed by data, and say so in why`);
    }
  });

  if (where) checkWhere(area, errors, warnings);
  checkDeepDives(area, warnings);
  checkMatrix(area, warnings);
  return { errors, warnings };
}

function independentGroups(config, srcs) {
  const parent = new Map();
  const find = (x) => { while (parent.get(x) !== x) x = parent.get(x); return x; };
  const add = (x) => { if (!parent.has(x)) parent.set(x, x); };
  for (const s of srcs) add(s);
  for (const s of sourcesOf(config)) for (const d of asList(s.derived_from)) { add(s.name); add(d); parent.set(find(s.name), find(d)); }
  return new Set([...new Set(srcs)].map(find)).size;
}

// Evidence check: every where must resolve in the source's local copy at the
// commit it cites. Legacy lessons (migrated from v1) only warn.
function checkWhere(area, errors, warnings) {
  const { config, lessons, root } = area;
  const bySource = new Map();
  for (const l of lessons) for (const ref of asList(l.where)) {
    const r = parseRef(ref);
    if (!r) continue;
    if (!bySource.has(r.src)) bySource.set(r.src, []);
    bySource.get(r.src).push({ l, ref, r });
  }
  for (const [src, items] of bySource) {
    const s = sourceByName(config, src);
    if (!s) continue;
    const dir = cloneDir(root, s);
    if (s.type !== "git-repo") {
      if (!fs.existsSync(dir)) continue;
      for (const { l, ref, r } of items)
        if (r.path && !fs.existsSync(path.join(dir, r.path)))
          (l.legacy ? warnings : errors).push(`lesson ${l.key}: ${ref} names no file under ${path.relative(root, dir)}`);
      continue;
    }
    if (!fs.existsSync(path.join(dir, ".git"))) {
      warnings.push(`source ${src}: clone missing at ${path.relative(root, dir)}; ${items.length} where ref(s) unverified`);
      continue;
    }
    const specs = items.map(({ r }) => (r.path ? `${r.at}:${r.path}` : `${r.at}^{commit}`));
    const out = (git(dir, ["cat-file", "--batch-check=%(objecttype) %(objectname)"], { input: specs.join("\n") + "\n", soft: true }) || "").split("\n");
    const lineCache = new Map();
    items.forEach(({ l, ref, r }, i) => {
      const push = (msg) => (l.legacy ? warnings : errors).push(`lesson ${l.key}: ${msg}`);
      const [type, oid] = (out[i] || "").split(" ");
      if (!r.path) { if (type !== "commit") push(`${ref} is not a commit in the clone of ${src}`); return; }
      if (!["blob", "tree"].includes(type)) { push(`${ref} does not resolve in the clone of ${src}`); return; }
      if (!r.anchor) return;
      const m = r.anchor.match(LINES_RE);
      if (!m || type !== "blob") { push(`${ref} has a line anchor on something that is not a file`); return; }
      if (!lineCache.has(oid)) lineCache.set(oid, (git(dir, ["cat-file", "-p", oid], { soft: true }) || "").split("\n").length);
      const n = lineCache.get(oid), a = Number(m[1]), b = Number(m[2] || m[1]);
      if (b < a) push(`${ref} line range is reversed`);
      else if (b > n) push(`${ref} line range exceeds the file's ${n} lines`);
    });
  }
}

function frontmatter(textIn) {
  const m = textIn.match(/^---\n([\s\S]*?)\n---/);
  if (!m) return null;
  try { return yaml.load(m[1]); } catch { return null; }
}

function checkDeepDives(area, warnings) {
  const dir = path.join(area.root, AREA, "deep-dives");
  if (!fs.existsSync(dir)) return;
  const keys = new Set(area.lessons.map((l) => l && l.key));
  for (const f of fs.readdirSync(dir).filter((f) => f.endsWith(".md"))) {
    const fm = frontmatter(fs.readFileSync(path.join(dir, f), "utf8")) || {};
    for (const pin of asList(fm.based_on)) {
      const [src, cursor] = String(pin).split("@");
      const s = sourceByName(area.config, src);
      if (!s) warnings.push(`deep-dive ${f}: based_on unknown source ${src}`);
      else if (cursor && s.cursor && !s.cursor.startsWith(cursor) && !cursor.startsWith(s.cursor)) warnings.push(`deep-dive ${f}: based on ${src}@${cursor}, source now @${s.cursor}`);
    }
    for (const e of asList(fm.entries)) {
      const key = String(e).split(":").pop();
      if (!keys.has(key)) warnings.push(`deep-dive ${f}: entry ${e} names no lesson`);
    }
  }
}

function checkMatrix(area, warnings) {
  const file = path.join(area.root, AREA, "comparison-matrix.md");
  if (!fs.existsSync(file)) return;
  const t = fs.readFileSync(file, "utf8");
  const keys = new Set(area.lessons.map((l) => l && l.key));
  for (const m of t.matchAll(/\]\(lessons\/[a-z0-9-]+\.yaml#([a-z0-9-]+)\)/g)) if (!keys.has(m[1])) warnings.push(`comparison-matrix: link to missing lesson ${m[1]}`);
  if (/\]\(sources\/[a-z0-9-]+\.md#/.test(t)) warnings.push("comparison-matrix: old sources/<name>.md links remain (migrate rewrites the ones it can resolve)");
}

// ---------- canonical writer ----------

const DUMP = { lineWidth: 88, noRefs: true };

function indent(s, n) {
  const pad = " ".repeat(n);
  return s.split("\n").map((line) => (line ? pad + line : line)).join("\n");
}

// Multi-line prose (notes, v1 text) keeps its line breaks as a literal block;
// one-line prose is folded at the line width.
function dumpField(k, v, flow = false) {
  const multiline = typeof v === "string" && v.includes("\n");
  return yaml.dump({ [k]: v }, { ...DUMP, lineWidth: multiline ? -1 : DUMP.lineWidth, flowLevel: flow ? 1 : -1 }).trimEnd();
}

function lessonText(l) {
  const m = derive(l);
  const item = { ...l };
  if (item.score && typeof item.score === "object") {
    item.score = ordered({ ...item.score, impact: m.impact === null ? undefined : m.impact }, SCORE_ORDER);
    if (item.score.impact === undefined) delete item.score.impact;
  }
  item.final_score = m.final === null ? undefined : m.final;
  if (item.decision) item.decision = ordered(item.decision, DECISION_ORDER);
  // A migrated lesson stops being legacy once every v2 field is filled in.
  if (item.legacy && LAYERS.includes(item.layer) && CONTRASTS.includes(item.contrast) && m.impact !== null &&
      isInt(item.score?.relevance, 0, 3) && isInt(item.score?.evidence, 1, 3) && isInt(item.score?.effort, 1, 3) &&
      asList(item.where).length && !item.legacy_where) delete item.legacy;
  const o = ordered(item, LESSON_ORDER);
  const parts = [];
  for (const [k, v] of Object.entries(o)) {
    if (v === undefined) continue;
    const flow = k === "decision" && JSON.stringify(v).length < 70;
    parts.push(dumpField(k, v, flow));
  }
  const body = parts.join("\n");
  return "  - " + indent(body, 4).slice(4);
}

function lessonsFileText(domain, lessons) {
  return `# Lessons whose primary domain is "${domain}". Written by distill.mjs: edit freely, then run\n` +
    "# `distill.mjs format`. final_score = relevance × impact × evidence / effort (impact = number of\n" +
    "# facts); ranked lessons come first by final_score, then unranked ones by key.\n" +
    "lessons:\n\n" + sortLessons(lessons).map(lessonText).join("\n\n") + "\n";
}

function configText(config) {
  const c = ordered(config, CONFIG_ORDER);
  if (c.goal) c.goal = ordered(c.goal, GOAL_ORDER);
  if (Array.isArray(c.sources)) c.sources = c.sources.map((s) => (s && typeof s === "object" ? ordered(s, SOURCE_ORDER) : s));
  const head = "# distill learning area: goal, domains, sources (cursor + coverage) and intake.\n" +
    "# Written by distill.mjs; comments are not kept, so put reasons in fields. Lessons live in lessons/<domain>.yaml.\n";
  const blocks = Object.entries(c).map(([k, v]) => {
    if (k !== "sources" || !Array.isArray(v) || !v.length) return dumpField(k, v);
    return "sources:\n" + v.map((s) => "  - " + indent(Object.entries(s).filter(([, x]) => x !== undefined).map(([sk, sv]) => dumpField(sk, sv)).join("\n"), 4).slice(4)).join("\n");
  });
  return head + blocks.join("\n\n") + "\n";
}

function render(area) {
  const files = new Map();
  files.set(path.join(area.root, AREA, CONFIG), configText(area.config));
  const byDomain = new Map();
  for (const l of area.lessons) {
    const d = l.domains[0];
    if (!byDomain.has(d)) byDomain.set(d, []);
    byDomain.get(d).push(l);
  }
  for (const [d, ls] of byDomain) files.set(path.join(area.root, AREA, LESSONS, `${d}.yaml`), lessonsFileText(d, ls));
  return files;
}

// All-or-nothing write: validate, render, re-parse the rendered text and
// validate it again, then write every file through temp + rename and remove
// lesson files that no longer have lessons.
function writeArea(area, opts = {}) {
  const { errors, warnings } = validate(area, opts);
  if (errors.length) failList(errors);
  const files = render(area);
  const reparsed = { ...area, config: yaml.load(files.get(path.join(area.root, AREA, CONFIG))), lessons: [], loadErrors: [] };
  for (const [f, t] of files) if (f.endsWith(".yaml") && path.dirname(f).endsWith(LESSONS)) reparsed.lessons.push(...yaml.load(t).lessons);
  const again = validate(reparsed, { where: false });
  if (again.errors.length) failList(["canonical output failed validation (bug in distill.mjs)", ...again.errors]);
  fs.mkdirSync(path.join(area.root, AREA, LESSONS), { recursive: true });
  const tmps = [];
  for (const [f, t] of files) { fs.writeFileSync(f + ".tmp", t); tmps.push(f); }
  for (const f of tmps) fs.renameSync(f + ".tmp", f);
  const dir = path.join(area.root, AREA, LESSONS);
  for (const f of fs.readdirSync(dir)) if (f.endsWith(".yaml") && !files.has(path.join(dir, f))) fs.unlinkSync(path.join(dir, f));
  return warnings;
}

function printWarnings(warnings, limit = 30) {
  if (!warnings.length) return;
  if (!limit) return console.log(`(${warnings.length} warning(s); run "distill.mjs check" to see them)`);
  console.log(`Warnings (${warnings.length}):`);
  for (const w of warnings.slice(0, limit)) console.log(`  - ${w}`);
  if (warnings.length > limit) console.log(`  … ${warnings.length - limit} more (run "check" to see all)`);
}

// ---------- commands: setup ----------

function emptyConfig(domains) {
  return {
    goal: { status: "draft", purpose: "", in_scope: [], out_of_scope: [], failures_it_prevents: [] },
    domains: domains.map((name) => ({ name, definition: "" })),
    sources: [],
    intake: [],
  };
}

function ensureGitignore(root, created) {
  const block = `${MARK_START}\n/${CLONES_DIR}/\n${MARK_END}\n`;
  const gi = path.join(root, ".gitignore");
  const giText = fs.existsSync(gi) ? fs.readFileSync(gi, "utf8") : "";
  const already = giText.split("\n").some((l) => l.trim() === `/${CLONES_DIR}/` || l.trim() === `${CLONES_DIR}/`);
  if (giText.includes(MARK_START)) {
    const updated = giText.replace(new RegExp(`${MARK_START}[\\s\\S]*?${MARK_END}\\n?`), block);
    if (updated !== giText) { fs.writeFileSync(gi, updated); created.push(".gitignore (refreshed block)"); }
  } else if (!already) {
    fs.writeFileSync(gi, giText + (giText.endsWith("\n") || giText === "" ? "" : "\n") + block);
    created.push(".gitignore (appended block)");
  }
}

function cmdInit(args) {
  const root = args.root ? path.resolve(args.root) : process.cwd();
  if (isV1(root))
    fail(`${AREA} already holds an old markdown learning area`, "init would create a second, empty area beside it", `run "distill.mjs migrate --dry-run" instead`);
  const created = [];
  for (const d of [path.join(root, AREA, LESSONS), path.join(root, CLONES_DIR)])
    if (!fs.existsSync(d)) { fs.mkdirSync(d, { recursive: true }); created.push(path.relative(root, d) + "/"); }
  const cfg = path.join(root, AREA, CONFIG);
  if (!fs.existsSync(cfg)) { fs.writeFileSync(cfg, configText(emptyConfig(DEFAULT_DOMAINS))); created.push(path.relative(root, cfg)); }
  ensureGitignore(root, created);
  console.log(created.length ? `Initialized learning area at ${root}:\n  - ${created.join("\n  - ")}` : `Learning area at ${root} already up to date; nothing written.`);
  console.log(`Next: write the goal and domain definitions in ${AREA}/${CONFIG}, then "distill.mjs add <name> --type <t> --url <u>".`);
}

function cmdIntake(args) {
  const [name] = args._;
  if (!name || !args.url) fail("intake needs <name> --url", "an intake item is a source waiting for triage", `distill.mjs intake meta-skill --url https://github.com/x/y --why "..."`);
  const root = requireRoot(args);
  const area = loadArea(root);
  area.config.intake = asList(area.config.intake);
  if (area.config.intake.some((i) => i.name === name || i.url === args.url) || sourceByName(area.config, name))
    fail(`"${name}" is already in intake or sources`, "a source is captured once", `see "distill.mjs status"`);
  area.config.intake.push({ name, type: args.type || undefined, url: args.url, added: today(), why: args.why || undefined });
  printWarnings(writeArea(area, { where: false }), 0);
  console.log(`Captured ${name} in intake. Triage decides whether to "add" it.`);
}

function cmdAdd(args) {
  const [name] = args._;
  if (!name || !args.type || !args.url)
    fail("add needs <name> --type --url", "a source cannot be registered without its identity and cursor type", `distill.mjs add meta-skill --type git-repo --url https://github.com/x/y`);
  if (!TYPES.includes(args.type)) fail(`unknown type "${args.type}"`, "cursor semantics differ per type", `use one of: ${TYPES.join(" | ")}`);
  if (!KEY_RE.test(name)) fail(`source name "${name}" is not kebab-case`, "names appear in every where reference", "pick a short kebab-case name");
  const root = requireRoot(args);
  const area = loadArea(root);
  if (sourceByName(area.config, name)) fail(`source "${name}" already exists`, "overwriting would destroy its cursor and coverage", `edit ${AREA}/${CONFIG} directly, or pick another name`);
  area.config.sources = asList(area.config.sources);
  area.config.sources.push({ name, type: args.type, url: args.url, ref: args.ref || undefined, local: args.local || undefined, cursor: null, analyzed: null, domains_covered: [] });
  const before = asList(area.config.intake).length;
  area.config.intake = asList(area.config.intake).filter((i) => i.name !== name && i.url !== args.url);
  printWarnings(writeArea(area, { where: false }), 0);
  console.log(`Added source ${name} (${args.type})${before !== area.config.intake.length ? "; removed its intake item" : ""}.`);
  const dest = args.local || path.join(CLONES_DIR, name);
  if (args.type === "git-repo") console.log(`Next: git clone --filter=blob:none ${args.url} ${dest} && distill.mjs delta ${name}`);
  else if (args.type === "paper") console.log(`Next: save a copy under ${dest}/, read it once, then "distill.mjs seal ${name} --version <v>".`);
  else console.log(`Next: fetch ${args.url}, extract, then "distill.mjs seal ${name} --version <v>".`);
}

// ---------- commands: the pass ----------

function headOf(root, s) {
  const dir = cloneDir(root, s);
  if (!fs.existsSync(path.join(dir, ".git")))
    fail(`clone missing at ${path.relative(root, dir)}`, "git-repo cursors are computed from a local clone", `git clone --filter=blob:none ${s.url} ${path.relative(root, dir)}`);
  return { dir, head: git(dir, ["rev-parse", "HEAD"]).slice(0, 12) };
}

function cmdDelta(args) {
  const [name] = args._;
  if (!name) fail("delta needs <name>", "there is no default source", "distill.mjs delta <name>");
  const root = requireRoot(args);
  const area = loadArea(root);
  const s = sourceByName(area.config, name);
  if (!s) fail(`unknown source "${name}"`, `not in ${AREA}/${CONFIG}`, `pick one of: ${sourcesOf(area.config).map((x) => x.name).join(", ") || "(none)"}`);
  const missing = domainNames(area.config).filter((d) => !asList(s.domains_covered).includes(d));
  if (s.cursor && missing.length) console.log(`== ${name}: domains needing BACKFILL (scan the snapshot at the cursor for these only) ==\n  - ${missing.join("\n  - ")}\n`);
  if (s.type !== "git-repo") {
    console.log(`== ${name}: ${s.type}, cursor ${s.cursor || "never"} ==`);
    console.log(s.type === "paper" && s.cursor ? "Immutable: extract once, never delta again." : `Fetch ${s.url}, compare its version/changelog with the cursor, read the gap, then: distill.mjs seal ${name} --version <v>`);
    return;
  }
  const { dir } = headOf(root, s);
  if (git(dir, ["pull", "--ff-only", "--quiet"], { soft: true, quietErr: true }) === null) console.error("(warn: pull failed; using local state)");
  const head = git(dir, ["rev-parse", "HEAD"]).slice(0, 12);
  const scope = asList(s.scope);
  const tail = scope.length ? ["--", ...scope] : [];
  if (!s.cursor) {
    console.log(`== ${name}: never analyzed, FULL SCAN at ${head} ==`);
    console.log(`Top level:\n${git(dir, ["ls-tree", "--name-only", "HEAD", ...scope])}`);
    console.log(`\nHistory to read for negative space (removals and reverts):\n${git(dir, ["log", "--format=%h %ad %s", "--date=short", "-n", "40", "--diff-filter=D", ...tail]) || "(none in scope)"}`);
  } else {
    if (git(dir, ["rev-parse", "--verify", "--quiet", `${s.cursor}^{commit}`], { soft: true, quietErr: true }) === null)
      fail(`cursor ${s.cursor} is not a commit in the clone`, "the clone was recreated with other history or the cursor was hand-edited", `check "git -C ${path.relative(root, dir)} log", then rescan in full and seal`);
    if (head.startsWith(s.cursor) || s.cursor.startsWith(head)) return console.log(`== ${name}: up to date (cursor = HEAD = ${head}); no change ==`);
    console.log(`== ${name}: ${s.cursor}..${head} ==`);
    console.log(git(dir, ["log", "--reverse", "--format=%n%h %ad %s", "--date=short", "--name-status", `${s.cursor}..HEAD`, ...tail]));
  }
  console.log(`\nAfter the pass: set coverage.at: ${head} for ${name} in ${CONFIG}, then "distill.mjs seal ${name}".`);
}

function cmdFormat(args) {
  const root = requireRoot(args);
  const area = loadArea(root);
  const warnings = writeArea(area, { where: !args["no-where"] });
  console.log(`OK: wrote ${area.lessons.length} lessons in ${new Set(area.lessons.map((l) => l.domains[0])).size} domain file(s).`);
  printWarnings(warnings);
}

function cmdSeal(args) {
  const [name] = args._;
  if (!name) fail("seal needs <name>", "sealing moves one source's cursor", "distill.mjs seal <name>");
  const root = requireRoot(args);
  const area = loadArea(root);
  const s = sourceByName(area.config, name);
  if (!s) fail(`unknown source "${name}"`, `not in ${AREA}/${CONFIG}`, `pick one of: ${sourcesOf(area.config).map((x) => x.name).join(", ")}`);
  let target;
  if (args.backfill) {
    if (!s.cursor) fail("--backfill needs an existing cursor", "a backfill reads the snapshot at the cursor without moving it", `run a full pass and "distill.mjs seal ${name}" first`);
    target = s.cursor;
  } else if (s.type === "git-repo") target = headOf(root, s).head;
  else {
    target = args.version || null;
    if (!target) fail(`${s.type} seal needs --version <v>`, "the cursor of a non-git source is the version or date you read", `distill.mjs seal ${name} --version <v>`);
  }
  const at = s.coverage && String(s.coverage.at || "");
  if (!at || !(at.startsWith(target) || target.startsWith(at)))
    fail(`coverage for ${name} is not stamped with ${target}`, "a pass is complete only when it records what it read and what it skipped, at the commit it read",
      `set coverage: {at: ${target}, read: [...], not_read: [{path, reason}]} for ${name} in ${AREA}/${CONFIG}, then seal again`);
  if (args.domains) {
    const all = domainNames(area.config);
    const want = args.domains === "all" ? all : String(args.domains).split(",").map((x) => x.trim());
    for (const d of want) if (!all.includes(d)) fail(`domain "${d}" is not in ${CONFIG}`, "domains_covered must stay comparable with the taxonomy", "add the domain first, or fix the spelling");
    s.domains_covered = [...new Set([...asList(s.domains_covered), ...want])];
  }
  const before = s.cursor;
  s.cursor = target;
  s.analyzed = today();
  const warnings = writeArea(area, { where: true });
  console.log(`Sealed ${name}: cursor ${before || "never"} → ${target}${args.backfill ? " (backfill, cursor kept)" : ""}; ${area.lessons.length} lessons valid.`);
  printWarnings(warnings);
}

function cmdCheck(args) {
  const root = requireRoot(args);
  const area = loadArea(root);
  const { errors, warnings } = validate(area, { where: !args["no-where"] });
  const files = render(area);
  const stale = [...files].filter(([f, t]) => !fs.existsSync(f) || fs.readFileSync(f, "utf8") !== t).map(([f]) => path.relative(path.join(root, AREA), f));
  const extra = area.files.filter((f) => !files.has(path.join(root, AREA, LESSONS, f)));
  if (!errors.length && (stale.length || extra.length)) warnings.push(`not in canonical layout: ${[...stale, ...extra.map((f) => `${LESSONS}/${f}`)].join(", ")}; run "distill.mjs format"`);
  if (errors.length) { console.log(`✗ ${errors.length} error(s)`); for (const e of errors) console.log(`  - ${e}`); }
  else console.log(`✓ ${area.lessons.length} lessons, ${sourcesOf(area.config).length} sources, goal ${area.config.goal?.status}`);
  printWarnings(warnings, 200);
  process.exit(errors.length ? 1 : 0);
}

// ---------- commands: reading and deciding ----------

function numbered(area) {
  return sortLessons(area.lessons.filter((l) => l && typeof l === "object")).map((l, i) => ({ n: i + 1, l, m: derive(l) }));
}

const GROUP_LABEL = { "unscored": "not yet scored against the goal", "outside-goal": "outside the goal (relevance 0)", "already-covered": "already covered by the host", "retired": "retired (removed, moved or superseded)" };

function cmdRank(args) {
  const root = requireRoot(args);
  const area = loadArea(root);
  const { errors } = validate(area, { where: false });
  if (errors.length) failList(errors, "rank needs a valid area");
  let rows = numbered(area);
  const all = rows;
  if (args.source) rows = rows.filter(({ l }) => asList(l.where).some((r) => parseRef(r)?.src === args.source));
  if (args.domain) rows = rows.filter(({ l }) => l.domains.includes(args.domain));
  if (args.state) rows = rows.filter(({ l }) => l.decision.state === args.state);
  const ranked = rows.filter((r) => r.m.group === "ranked");
  const top = args.all ? ranked.length : Number(args.top || 20);
  const finals = new Map();
  for (const r of all) if (r.m.final !== null) finals.set(r.m.final, (finals.get(r.m.final) || 0) + 1);
  console.log("Lessons by priority (final_score = relevance × impact × evidence / effort; '=' marks a tie):");
  for (const { n, l, m } of ranked.slice(0, top)) {
    const s = l.score;
    const tie = finals.get(m.final) > 1 ? "=" : " ";
    console.log(`${String(n).padStart(4)} ${tie}${String(m.final).padStart(5)}  r${s.relevance} i${m.impact} e${s.evidence} f${s.effort}  ${l.decision.state.padEnd(11)} ${String(l.layer).padEnd(11)} ${String(l.contrast).padEnd(15)} ${l.key}`);
  }
  if (ranked.length > top) console.log(`  … ${ranked.length - top} more ranked (--all)`);
  for (const g of Object.keys(GROUP_LABEL)) {
    const inG = rows.filter((r) => r.m.group === g);
    if (!inG.length) continue;
    console.log(`\n${inG.length} ${GROUP_LABEL[g]}${args.all ? ":" : " (--all lists them)"}`);
    if (args.all) for (const { n, l } of inG) console.log(`${String(n).padStart(4)}         ${l.decision.state.padEnd(11)} ${l.key}`);
  }
}

function cmdDecide(args) {
  const sels = args._;
  if (!sels.length || !STATES.includes(args.state))
    fail("decide needs <n|key>... --state <state>", "a decision names the lessons and their new state", `distill.mjs decide 1 3 --state planned  (states: ${STATES.join(", ")})`);
  if (args.state === "rejected" && !text(args.reason)) fail("rejected needs --reason", "the reason is what stops the lesson from being proposed again", `distill.mjs decide 4 --state rejected --reason "..."`);
  const root = requireRoot(args);
  const area = loadArea(root);
  const rows = numbered(area);
  const byKey = new Map(rows.map((r) => [r.l.key, r.l]));
  const chosen = sels.map((sel) => {
    const l = /^\d+$/.test(sel) ? rows[Number(sel) - 1]?.l : byKey.get(sel);
    if (!l) fail(`no lesson "${sel}"`, `there are ${rows.length} lessons, numbered as "distill.mjs rank --all" prints them`, "run rank, then use its numbers or the key");
    return l;
  });
  for (const l of chosen) {
    const src = parseRef(asList(l.where)[0])?.src;
    const cursor = src ? sourceByName(area.config, src)?.cursor : null;
    const d = { state: args.state };
    if (text(args.reason)) d.reason = args.reason;
    else if (l.decision?.reason && l.decision.state === args.state) d.reason = l.decision.reason;
    if (args.state !== "candidate" && cursor) d.at = cursor;
    if (text(args.local)) d.local = args.local;
    else if (l.decision?.local) d.local = l.decision.local;
    if (text(args.host)) l.host = args.host;
    l.decision = d;
  }
  const warnings = writeArea(area, { where: false });
  for (const l of chosen) console.log(`${l.key}: ${args.state}${l.decision.at ? ` @${l.decision.at}` : ""}`);
  printWarnings(warnings.filter((w) => chosen.some((l) => w.includes(l.key))));
}

function cmdOutcome(args) {
  const [key, result, ...note] = args._;
  if (!key || !OUTCOMES.includes(result) || !note.length)
    fail("outcome needs <key> <result> <note>", "the outcome closes the loop from predicted value to real value", `distill.mjs outcome ratchet-over-doctrine confirmed "caught 3 regressions in a month"`);
  const root = requireRoot(args);
  const area = loadArea(root);
  const l = area.lessons.find((x) => x.key === key);
  if (!l) fail(`no lesson "${key}"`, "outcome is recorded on an existing lesson", "check the key with find or rank");
  if (!["ported", "adapted"].includes(l.decision?.state)) fail(`${key} is ${l.decision?.state}, not ported or adapted`, "an outcome describes what happened after a lesson was applied", `record the port first: distill.mjs decide ${key} --state ported --host <path>`);
  l.outcome = { result, note: note.join(" ") };
  writeArea(area, { where: false });
  console.log(`${key}: outcome ${result}`);
}

function cmdStatus(args) {
  const root = requireRoot(args);
  const area = loadArea(root);
  const srcs = sourcesOf(area.config);
  if (args.repo) {
    console.log("== distill sources (managed upstreams) ==");
    for (const s of srcs) {
      const rel = path.relative(root, cloneDir(root, s));
      const has = fs.existsSync(path.join(root, rel));
      console.log(`  ${has ? "●" : "○"} ${s.name.padEnd(24)} ${s.type.padEnd(10)} ${has ? rel : "(no clone)"}`);
    }
    return console.log(`  ${srcs.length} source(s)`);
  }
  const domains = domainNames(area.config);
  const sources = srcs.map((s) => {
    const o = { name: s.name, type: s.type, cursor: s.cursor || null, backfill: s.cursor ? domains.filter((d) => !asList(s.domains_covered).includes(d)).length : 0, coverage: !!s.coverage };
    if (s.type === "git-repo" && s.cursor) {
      const dir = cloneDir(root, s);
      if (!fs.existsSync(path.join(dir, ".git"))) o.behind = "no-clone";
      else { const n = git(dir, ["rev-list", "--count", `${s.cursor}..HEAD`], { soft: true, quietErr: true }); o.behind = n === null ? "?" : Number(n); }
    }
    return o;
  });
  const rows = numbered(area);
  const groups = {}, states = {};
  for (const { l, m } of rows) { groups[m.group] = (groups[m.group] || 0) + 1; states[l.decision?.state] = (states[l.decision?.state] || 0) + 1; }
  const stale = [];
  checkDeepDives(area, stale);
  const out = { goal: area.config.goal?.status || "missing", sources, intake_pending: asList(area.config.intake).length, lessons: rows.length, groups, states, deep_dive_warnings: stale };
  if (args.json) return console.log(JSON.stringify(out, null, 2));
  console.log(`== distill status (local view; "delta <name>" pulls upstream) ==\n  goal: ${out.goal}`);
  for (const s of sources) {
    const behind = s.behind === undefined ? "" : s.behind === "no-clone" ? " · no clone" : s.behind === 0 ? " · clone even with cursor" : ` · clone ${s.behind} commit(s) past cursor`;
    console.log(`  ${s.cursor ? "●" : "○"} ${s.name} (${s.type}) cursor=${s.cursor || "never"}${behind}${s.backfill ? ` · backfill ${s.backfill} domains` : ""}${s.cursor && !s.coverage ? " · no coverage" : ""}`);
  }
  console.log(`  lessons: ${rows.length} · ${Object.entries(groups).map(([g, n]) => `${g} ${n}`).join(" · ")}`);
  console.log(`  decisions: ${Object.entries(states).map(([g, n]) => `${g} ${n}`).join(" · ")} · intake pending: ${out.intake_pending}`);
  if (stale.length) console.log(`  deep-dives:\n    - ${stale.join("\n    - ")}`);
  const next = [];
  if (out.goal !== "confirmed") next.push("write and confirm the goal");
  if (out.intake_pending) next.push("triage intake");
  for (const s of sources) {
    if (!s.cursor) next.push(`full pass on ${s.name}`);
    else if (typeof s.behind === "number" && s.behind > 0) next.push(`delta pass on ${s.name}`);
    if (s.backfill) next.push(`backfill ${s.name}`);
  }
  if (groups.unscored) next.push(`score ${groups.unscored} unscored lessons`);
  if (rows.some(({ l, m }) => m.group === "ranked" && l.decision.state === "candidate")) next.push("decide top candidates (rank)");
  if (next.length) console.log(`  suggested next: ${next.slice(0, 5).join(" · ")}`);
}

function cmdFind(args) {
  const [term] = args._;
  if (!term) fail("find needs <term>", "it searches lessons, deep-dives and matrix rows", "distill.mjs find ratchet");
  const root = requireRoot(args);
  const area = loadArea(root);
  const rx = new RegExp(term.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "i");
  let hits = 0;
  for (const { n, l, m } of numbered(area)) {
    if (!rx.test(JSON.stringify(l))) continue;
    hits++;
    console.log(`== #${n} ${l.key} [${l.domains.join(", ")} · ${l.layer || "?"} · ${l.decision.state}${m.final !== null ? ` · ${m.final}` : ` · ${m.group}`}] (${LESSONS}/${l.domains[0]}.yaml)`);
    console.log(`   what: ${short(l.what, 300)}\n   notable: ${short(l.notable, 300)}\n   where: ${asList(l.where).join(", ") || l.legacy_where || "-"}\n`);
  }
  const files = [["comparison-matrix.md", (t) => t.split("\n").filter((x) => /^\|/.test(x) && rx.test(x))]];
  for (const [f, pick] of files) {
    const p = path.join(root, AREA, f);
    if (!fs.existsSync(p)) continue;
    const rows = pick(fs.readFileSync(p, "utf8"));
    if (rows.length) { hits += rows.length; console.log(`== ${f} (${rows.length} rows) ==\n${rows.join("\n")}\n`); }
  }
  const dd = path.join(root, AREA, "deep-dives");
  if (fs.existsSync(dd))
    for (const f of fs.readdirSync(dd).filter((x) => x.endsWith(".md")))
      for (const block of fs.readFileSync(path.join(dd, f), "utf8").split(/^(?=#{2,3} )/m))
        if (/^#{2,3} /.test(block) && rx.test(block)) { hits++; console.log(`== deep-dives/${f} :: ${block.split("\n")[0].replace(/^#+ /, "")} ==\n${block.trim().slice(0, 1200)}\n`); }
  if (!hits) console.log(`No matches for "${term}". Try the source's own vocabulary (keywords) or a broader term.`);
}

function cmdMap(args) {
  const [term] = args._;
  const root = requireRoot(args);
  const area = loadArea(root);
  const rx = term ? new RegExp(term.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "i") : null;
  const rows = area.lessons.filter((l) => ["ported", "adapted"].includes(l.decision?.state) && (!rx || rx.test(JSON.stringify(l))));
  if (!rows.length) return console.log(term ? `No ported/adapted lessons match "${term}".` : "Nothing ported/adapted yet.");
  console.log("source lesson  →  local name  (host location)");
  for (const l of rows) {
    const srcs = [...new Set(asList(l.where).map((r) => parseRef(r)?.src).filter(Boolean))].join(" + ") || "?";
    console.log(`  ${srcs}:${l.key}\n    → ${l.decision.local || l.key}  (${l.host}) [${l.decision.state}${l.decision.at ? ` @${l.decision.at}` : ""}]${l.outcome ? ` outcome: ${l.outcome.result}` : ""}`);
  }
}

// Rename a domain everywhere it appears (domain list, sources' domains_covered,
// lessons' domains, lesson file name, matrix links) in one validated write.
function cmdDomainRename(args) {
  const [from, to, ...definition] = args._;
  if (!from || !to) fail("domain-rename needs <old> <new> [definition...]", "a rename names both domains", `distill.mjs domain-rename routing skill-recommendation "choosing the right skill for a task"`);
  if (!KEY_RE.test(to) || to === UNCLASSIFIED) fail(`"${to}" is not a usable domain name`, "domain names are kebab-case and unclassified is reserved", "pick another name");
  const root = requireRoot(args);
  const area = loadArea(root);
  const names = domainNames(area.config);
  if (!names.includes(from)) fail(`no domain "${from}"`, `the domains in ${CONFIG} are: ${names.join(", ")}`, "check the spelling");
  if (names.includes(to)) fail(`domain "${to}" already exists`, "a rename into an existing domain would silently merge two domains", "merge by editing lessons' domains, then remove the old domain");
  area.config.domains = asList(area.config.domains).map((d) => {
    const name = typeof d === "string" ? d : d.name;
    if (name !== from) return d;
    const o = typeof d === "string" ? { name: to } : { ...d, name: to };
    if (definition.length) o.definition = definition.join(" ");
    return o;
  });
  for (const s of sourcesOf(area.config)) s.domains_covered = asList(s.domains_covered).map((d) => (d === from ? to : d));
  let moved = 0;
  for (const l of area.lessons) if (Array.isArray(l.domains) && l.domains.includes(from)) { l.domains = l.domains.map((d) => (d === from ? to : d)); moved++; }
  const warnings = writeArea(area, { where: false });
  const matrix = path.join(root, AREA, "comparison-matrix.md");
  if (fs.existsSync(matrix)) fs.writeFileSync(matrix, fs.readFileSync(matrix, "utf8").split(`](${LESSONS}/${from}.yaml#`).join(`](${LESSONS}/${to}.yaml#`));
  console.log(`Renamed domain ${from} → ${to}: ${moved} lesson(s) retagged.`);
  printWarnings(warnings, 0);
}

// ---------- migrate (v1 markdown → v2 YAML) ----------

function parseTable(textIn) {
  const lines = textIn.split("\n").filter((l) => /^\|/.test(l));
  if (lines.length < 2) return { header: [], rows: [] };
  const cells = (l) => l.replace(/^\|/, "").replace(/\|\s*$/, "").split("|").map((c) => c.trim());
  const header = cells(lines[0]).map((c) => c.toLowerCase());
  return { header, rows: lines.slice(1).filter((l) => !/^\|\s*:?-/.test(l)).map(cells) };
}

function v1Frontmatter(t) {
  const m = t.match(/^---\n([\s\S]*?)\n---\n?/);
  if (!m) return { data: {}, body: t };
  const data = {};
  for (const line of m[1].split("\n")) {
    const kv = line.match(/^([A-Za-z_]+):\s*(.*)$/);
    if (!kv) continue;
    let v = kv[2].replace(/\s+#.*$/, "").trim();
    if (v.startsWith("[") && v.endsWith("]")) v = v.slice(1, -1).split(",").map((s) => s.trim()).filter(Boolean);
    data[kv[1]] = v === "null" || v === "" ? null : v;
  }
  return { data, body: t.slice(m[0].length) };
}

const slugify = (s) => s.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "entry";

function parseV1Source(name, textIn, taxonomy) {
  const { data, body } = v1Frontmatter(textIn);
  const entries = [];
  const notes = [];
  let domain = null, entry = null, field = null;
  const flush = () => { if (entry) entries.push(entry); entry = null; field = null; };
  for (const line of body.split("\n")) {
    const h2 = line.match(/^## (.+?)\s*$/);
    const h3 = line.match(/^### (.+?)\s*$/);
    if (h2) { flush(); const d = h2[1].trim(); domain = taxonomy.includes(d) ? d : null; if (!domain) notes.push(line); continue; }
    if (h3 && (domain || /^[a-z0-9-]+$/.test(h3[1].trim()))) { flush(); entry = { slug: h3[1].trim(), domain: domain || UNCLASSIFIED, fields: {}, extra: [] }; continue; }
    if (entry) {
      const f = line.match(/^- \*\*([^*]+?):\*\*\s?(.*)$/);
      if (f) { field = f[1].trim(); entry.fields[field] = (entry.fields[field] ? entry.fields[field] + "\n" : "") + f[2]; }
      else if (line.trim() === "") field = null;
      else if (field) entry.fields[field] += "\n" + line;
      else entry.extra.push(line);
      continue;
    }
    if (domain) { if (line.trim()) notes.push(line); } else notes.push(line);
  }
  flush();
  return { meta: data, entries, notes: notes.join("\n").trim() };
}

function pinWhere(src, seen, whereText) {
  const refs = [];
  let rest = whereText || "";
  for (const m of (whereText || "").matchAll(/`([^`]+)`/g)) {
    const p = m[1].trim().replace(/\/$/, "");
    if (!/[/.]/.test(p) || /[\s*|,]/.test(p) || /^(https?:|@)/.test(p)) continue;
    if (seen) refs.push(`${src}@${seen}:${p}`);
    rest = rest.replace(m[0], "");
  }
  const leftover = rest.replace(/[\s,.;:()]+/g, "");
  return { refs: [...new Set(refs)], legacy: leftover || !seen ? whereText.trim() : undefined };
}

function cmdMigrate(args) {
  const root = findRoot(args);
  if (!root) fail(`no ${AREA}/ found upward from ${process.cwd()}`, "migrate converts an existing v1 area", "cd into the project that holds it, or pass --root");
  const area = path.join(root, AREA);
  if (fs.existsSync(path.join(area, CONFIG))) fail(`${AREA}/${CONFIG} already exists`, "the area is already migrated", `run "distill.mjs check"`);
  if (!isV1(root)) fail(`${AREA} holds no v1 files`, "migrate reads taxonomy.txt, sources/*.md and porting-log.md", `run "distill.mjs init" for a new area`);
  if (!args.force && !args["dry-run"]) {
    const dirty = git(root, ["status", "--porcelain", "--", AREA], { soft: true, quietErr: true });
    if (dirty === null || dirty !== "")
      fail(`${AREA} has uncommitted changes or is not in a git repository`, "migrate deletes the v1 files; Git history is how you get them back", `commit ${AREA} first (or pass --force if you have another copy)`);
  }
  const report = [];
  const read = (f) => (fs.existsSync(path.join(area, f)) ? fs.readFileSync(path.join(area, f), "utf8") : "");

  // taxonomy → domains (an inline "# comment" becomes the definition)
  const domains = [];
  for (const line of read("taxonomy.txt").split("\n")) {
    const m = line.match(/^\s*([a-z0-9-]+)\s*(?:#\s*(.*))?$/);
    if (m) domains.push({ name: m[1], definition: (m[2] || "").trim() });
  }
  const taxonomy = domains.map((d) => d.name);
  const config = { ...emptyConfig([]), domains };

  // sources/*.md → sources + lessons
  const lessons = [];
  const keyOwner = new Map();
  const srcDir = path.join(area, "sources");
  const srcFiles = fs.existsSync(srcDir) ? fs.readdirSync(srcDir).filter((f) => f.endsWith(".md")).sort() : [];
  for (const f of srcFiles) {
    const name = f.replace(/\.md$/, "");
    const p = parseV1Source(name, fs.readFileSync(path.join(srcDir, f), "utf8"), taxonomy);
    const m = p.meta;
    const type = TYPES.includes(m.type) ? m.type : "git-repo";
    const cursor = (type === "git-repo" ? m.last_analyzed_commit : m.last_analyzed_version || m.extracted_date) || m.last_analyzed_commit || null;
    const known = new Set(["name", "type", "url", "local", "last_analyzed_commit", "last_analyzed_version", "last_analyzed_date", "extracted_date", "domains_covered"]);
    const extraFm = Object.entries(m).filter(([k, v]) => !known.has(k) || (type === "git-repo" && k === "last_analyzed_version" && v)).map(([k, v]) => `${k}: ${Array.isArray(v) ? v.join(", ") : v}`);
    const placeholder = /^# .*\n*>\s*Chưa phân tích.*$/s.test(p.notes) || /^# [^\n]*$/.test(p.notes);
    const notes = [extraFm.length ? `v1 frontmatter kept: ${extraFm.join("; ")}` : "", placeholder ? "" : p.notes].filter(Boolean).join("\n\n");
    config.sources.push({
      name, type, url: m.url || "(unknown)", local: m.local && m.local !== path.join(CLONES_DIR, name) ? m.local : undefined,
      cursor: cursor ? String(cursor).replace(/[^A-Za-z0-9._-]/g, "") : null, analyzed: m.last_analyzed_date || m.extracted_date || null,
      domains_covered: asList(m.domains_covered).filter((d) => taxonomy.includes(d)), notes: notes || undefined,
    });
    for (const e of p.entries) {
      let key = KEY_RE.test(e.slug) ? e.slug : slugify(e.slug);
      if (keyOwner.has(key)) { report.push(`key ${key} exists in ${keyOwner.get(key)}; ${name}'s copy renamed ${key}-${name}`); key = `${key}-${name}`; }
      keyOwner.set(key, name);
      const F = e.fields;
      const seen = (String(F.Seen || "").match(/[A-Za-z0-9._-]+/) || [])[0] || cursor || null;
      const pinned = pinWhere(name, seen, [F.Where, F["Where else relevant"]].filter(Boolean).join("\n"));
      const known2 = new Set(["What", "Where", "Where else relevant", "Notable", "Keywords", "Seen", "Status"]);
      const extras = Object.entries(F).filter(([k]) => !known2.has(k)).map(([k, v]) => `${k}: ${v}`);
      const st = String(F.Status || "");
      let status;
      if (/^`?moved-to-([a-z0-9-]+)/.test(st)) status = `moved-to:${st.match(/moved-to-([a-z0-9-]+)/)[1]}`;
      else if (/superseded-by-([a-z0-9-]+)/.test(st)) status = `superseded-by:${st.match(/superseded-by-([a-z0-9-]+)/)[1]}`;
      else if (/removed|superseded/.test(st)) status = "removed";
      const notable = [F.Notable, ...extras, ...e.extra.filter((x) => x.trim()), st ? `Status (v1): ${st}` : ""].filter(Boolean).join("\n").trim();
      lessons.push({
        key, domains: [e.domain], layer: null, what: (F.What || "").trim() || "(v1 entry without What)", notable: notable || undefined,
        where: pinned.refs, legacy_where: pinned.legacy, contrast: null,
        score: { relevance: null, facts: null, evidence: null, effort: null, why: "legacy v1 entry; not yet scored against the goal" },
        keywords: F.Keywords ? F.Keywords.split(/[,;]/).map((k) => k.replace(/[`.]/g, "").trim()).filter(Boolean) : undefined,
        status, decision: { state: "candidate" }, legacy: true,
      });
    }
  }
  for (const l of lessons) if (l.status?.startsWith("superseded-by:") && !keyOwner.has(l.status.slice(14))) { report.push(`${l.key}: ${l.status} names no entry; kept as removed`); l.status = "removed"; }

  // porting-log.md rows → decisions on lessons (one authority)
  const byKey = new Map(lessons.map((l) => [l.key, l]));
  const { header, rows } = parseTable(read("porting-log.md"));
  const col = (...w) => header.findIndex((c) => w.some((x) => c.startsWith(x)));
  const [iF, iSrc, iSt, iSc, iLoc, iDest, iCom, iNote] = [col("feature"), col("nguồn", "source"), col("status"), col("score"), col("local"), col("đích", "dest"), col("commit"), col("ghi chú", "note")];
  let attached = 0, created = 0;
  for (const r of rows) {
    const feature = r[iF] || "";
    if (!feature) continue;
    const srcText = r[iSrc] || "";
    const refs = [...srcText.matchAll(/([a-z0-9-]+):([a-z0-9-]+)/g)].map((m) => m[2]).filter((k) => byKey.has(k));
    let target = byKey.get(feature) || (refs.length ? byKey.get(refs[0]) : null);
    if (target && target._decided) target = null;
    const state = STATES.includes(r[iSt]) ? r[iSt] : "candidate";
    const note = (r[iNote] || "").trim();
    const reason = [`porting-log row ${feature} (${srcText})`, note].filter(Boolean).join(": ");
    if (!target) {
      const first = refs.length ? byKey.get(refs[0]) : null;
      let key = KEY_RE.test(feature) ? feature : slugify(feature);
      if (byKey.has(key)) key = `${key}-port`;
      target = { key, domains: first ? [...first.domains] : [UNCLASSIFIED], layer: null, what: `Porting-log row: ${feature} (${srcText})`, notable: note || undefined,
        where: [], contrast: null, score: { relevance: null, facts: null, evidence: null, effort: null, why: "legacy porting-log row; not yet scored against the goal" },
        decision: { state: "candidate" }, legacy: true };
      lessons.push(target); byKey.set(key, target); created++;
      if (!first) report.push(`porting row ${feature}: no source entry resolved from "${srcText}"; created lesson ${key} in ${UNCLASSIFIED}`);
    } else attached++;
    target._decided = true;
    for (const k of refs) if (byKey.get(k) !== target) target.where = [...new Set([...target.where, ...byKey.get(k).where])];
    const sc = String(r[iSc] || "").match(/R([1-3])\s*E([1-3])\s*F([1-3])/);
    if (sc) target.score = { relevance: null, facts: null, evidence: Number(sc[2]), effort: Number(sc[3]), why: `legacy porting score R${sc[1]} E${sc[2]} F${sc[3]} (reach dropped); relevance and impact not yet scored against the goal` };
    const local = r[iLoc] && !/^[—-]$/.test(r[iLoc]) ? r[iLoc] : undefined;
    const dest = r[iDest] && !/^[—-]$/.test(r[iDest]) ? r[iDest] : undefined;
    const commit = r[iCom] && !/^[—-]$/.test(r[iCom]) ? r[iCom] : undefined;
    if (dest || commit) target.host = [dest, commit ? `@ ${commit}` : ""].filter(Boolean).join(" ");
    if (["ported", "adapted"].includes(state) && !target.host) target.host = "(not recorded in the v1 porting log)";
    target.decision = { state, reason: state === "candidate" && !note ? undefined : reason, local };
    if (state === "rejected" && !text(target.decision.reason)) target.decision.reason = "(no reason recorded in the v1 porting log)";
  }
  for (const l of lessons) delete l._decided;

  // intake.md table → intake
  const it = parseTable(read("intake.md"));
  for (const r of it.rows) if (r[0] && r[2]) config.intake.push({ name: slugify(r[0]), type: r[1] || undefined, url: r[2], added: r[3] || undefined, why: r[4] || undefined });

  const v2 = { root, config, lessons, files: [], loadErrors: [] };
  const { errors, warnings } = validate(v2, { where: false });
  if (errors.length) failList(errors, "migrated area would be invalid");

  // matrix links → lessons/<domain>.yaml#key
  const matrixFile = path.join(area, "comparison-matrix.md");
  let matrix = fs.existsSync(matrixFile) ? fs.readFileSync(matrixFile, "utf8") : null, relinked = 0, unresolved = 0;
  if (matrix) matrix = matrix.replace(/\]\(sources\/([a-z0-9-]+)\.md#([a-z0-9-]+)\)/g, (all, src, slug) => {
    const l = byKey.get(slug);
    if (!l) { unresolved++; return all; }
    relinked++;
    return `](${LESSONS}/${l.domains[0]}.yaml#${slug})`;
  });

  const decided = lessons.filter((l) => l.decision.state !== "candidate").length;
  console.log(`${args["dry-run"] ? "DRY RUN: would migrate" : "Migrated"} ${AREA}:`);
  console.log(`  domains ${domains.length} (${domains.filter((d) => !d.definition).length} without definition) · sources ${config.sources.length} · intake ${config.intake.length}`);
  console.log(`  lessons ${lessons.length} (all legacy: layer, contrast, relevance and impact to be scored) · porting rows attached ${attached}, created ${created} · decided ${decided}`);
  console.log(`  where pinned ${lessons.reduce((n, l) => n + l.where.length, 0)} refs · legacy_where kept on ${lessons.filter((l) => l.legacy_where).length} lessons`);
  if (matrix) console.log(`  comparison-matrix links rewritten ${relinked}, unresolved ${unresolved}`);
  if (fs.existsSync(path.join(area, "state"))) console.log(`  ${AREA}/state/ is no longer read (decisions now live on lessons); delete it when you are ready`);
  for (const r of report.slice(0, 20)) console.log(`  note: ${r}`);
  if (report.length > 20) console.log(`  … ${report.length - 20} more notes`);
  if (args["dry-run"]) { console.log("Nothing written."); return; }
  writeArea(v2, { where: false });
  if (matrix !== null) fs.writeFileSync(matrixFile, matrix);
  for (const f of ["taxonomy.txt", "intake.md", "porting-log.md"]) if (fs.existsSync(path.join(area, f))) fs.unlinkSync(path.join(area, f));
  for (const f of srcFiles) fs.unlinkSync(path.join(srcDir, f));
  if (fs.existsSync(srcDir) && !fs.readdirSync(srcDir).length) fs.rmdirSync(srcDir);
  console.log(`Removed the v1 files (they stay in Git history). Next: "distill.mjs check", then write the goal and domain definitions.`);
  printWarnings(warnings, 10);
}

// ---------- dispatch ----------

const [cmd, ...rest] = process.argv.slice(2);
const args = parseArgs(rest);
const commands = { init: cmdInit, intake: cmdIntake, add: cmdAdd, delta: cmdDelta, format: cmdFormat, seal: cmdSeal, check: cmdCheck,
  rank: cmdRank, decide: cmdDecide, outcome: cmdOutcome, status: cmdStatus, find: cmdFind, map: cmdMap,
  "domain-rename": cmdDomainRename, migrate: cmdMigrate };
if (!commands[cmd])
  fail(`unknown command "${cmd || ""}"`, "distill automates the mechanical parts of the learning lifecycle", `use one of: ${Object.keys(commands).join(" | ")} (see the header comment for flags)`);
commands[cmd](args);
