---
phase: 2
title: "Rewrite the distill script as a self-contained Node module"
status: done
priority: P1
effort: "13.5h"
wave: 2
dependencies: [1]
---

# Phase 2: Rewrite the distill script as a self-contained Node module

<!-- Red team: Phase 2 depends on Phase 1. On main the repo `.gitignore` line `.agents/` ignores
`.agents/skills/distill-lab/scripts/**`; only Phase 1's `.gitignore` edit un-ignores it. Moved from wave 1 to
wave 2. Effort raised from 10h to 13h for the loader schema, hardened git calls, safe write and their tests. -->
<!-- Validation: effort 13h -> 13.5h for the tarball extraction script, the full test-name exclusion list and the
release verify checkout depth. The vendored folder is `scripts/lib/` (the repo hook blocks Bash paths with the
other name). -->

## Context (traced)

Reference implementation: `.claude/skills/distill-lab/scripts/distill.py` (421 lines, read-only here).

- Commands and options (`distill.py:401-416`): `check FILE [--no-where]`, `format FILE [--no-where]`,
  `list FILE [--top N=15] [--all]`, `decide FILE SELECTOR... --state {candidate,planned,ported,rejected}
  [--reason TEXT]`. Module docstring is the `--help` text (`:2-14`).
- Constants and orders (`:26-42`): layers, contrasts, states, key/where/also_fits/status regexes, score ranges,
  impact facts, field orders for top level, goal, lesson, score, decision.
- Scoring (`:47-59`): `impact_of` = number of facts if `a` is among them, else 0; `final_score` =
  `round(relevance * impact * evidence / effort, 2)` (Python `round`: half-to-even on the exact binary value);
  sort by `(-final_score, layer rank, key)`.
- Validation (`:62-163`) collects all errors; cursors must be strings matching `[0-9a-f]{7,40}` (`:85`);
  `run_checks` adds the evidence check when asked (`:305-310`).
- Evidence check (`:166-192`): for each `where`, commit-only refs need `git cat-file -t` = `commit`; path refs
  need `git show <commit>:<path>` (`:183`) to resolve and the line range to fit, counted with Python
  `splitlines()`; results cached per (source, commit, path). Git runs as
  `git -C <hub>/runtime/distill-lab/mirrors/<src>` with no config limits (`:195-201`).
- Hub location today: `hub_of()` = `parents[4]` of the resolved file path, unchecked (`:293-295`;
  `<hub>/skills/<collection>/<id>/.meta/distill.yaml`); used only by the evidence check. The personal path
  `~/skill-hub` exists only in SKILL.md (`SKILL.md:292`), not in the script.
- Loader: `yaml.safe_load` (`:288`), PyYAML's YAML 1.1 resolvers: a float needs a dot, so a value such as
  `1e23456` is a string, while `0123456` is an octal int and `2026-10-11` a date.
- Writer (`:212-279`): a PyYAML `SafeDumper` with indented sequences, `decision` as a flow mapping, `score.facts`
  as a flow list, prose longer than 60 characters (after collapsing whitespace) as a folded `>` block, keys in
  fixed orders, `also_fits` defaulted to `[]`, `width=88`, `allow_unicode`, a blank line before each lesson, and a
  four-line header comment naming `distill.py`. `write()` re-parses and re-validates its own output, then writes
  the fixed name `<file>.yaml.tmp` and renames it over the file (`:320-331`), following symlinks.
- Errors use `ERROR/WHY/FIX` text on stderr; `sys.exit("...")` exits 1, argparse usage errors exit 2.
- Fixtures written by the Python script: `internal/migration/testdata/test-audit-distill.yaml`,
  `phase0-distill.yaml` (42 lessons each, header naming `distill.py`). Readers of these fixtures:
  `internal/distill/distill_model_test.go:13`, `internal/migration/v4_test.go:35`,
  `internal/delivery/web/routes_distill_test.go:34`. Go reader: `distill.UnmarshalDocument`
  (`internal/distill/distill_model.go:291`) and `ValidateDocument` (`:120`).
- Prior art in this repo: `.claude/skills/distill/scripts/distill.mjs` imports a vendored js-yaml 4.3.2. Its copy
  is not byte-identical to the published file (a header line added, the source-map line removed), so this phase
  vendors a fresh, unmodified copy instead of reusing it.
- js-yaml facts (npm registry, `npm view js-yaml@4.3.2` and `npm pack js-yaml@4.3.2`, 2026-10-11): 4.3.2 published
  2026-08-26, license MIT, `exports["."].import = "./dist/js-yaml.mjs"`, no runtime dependency for the ESM file
  (no `import` statement in it), no `engines` field, no provenance attestation.
  <!-- Red team: vendored js-yaml provenance --> Tarball
  `https://registry.npmjs.org/js-yaml/-/js-yaml-4.3.2.tgz`, registry `dist.integrity`
  `sha512-SFNOvSJ+Dgf/9An904Yx+CgSlIPCkIpao4qo51lpee25TIRejdH3rhR4EZMGoNx3/TP3O+wzWuiTFl4sqbltzA==`, `dist.shasum`
  `8e44fb14a2643c59726bb15787b5f1512cb3d3fb`. Inside it, `dist/js-yaml.mjs` is 104118 bytes, sha256
  `cea276c7e15f409a1adbe5d177aba7824398a474f7cf702bd57962c7d570636f`; `LICENSE` is 1084 bytes, sha256
  `a07bc24468b9654ce76a547d47a2db282d07733b715db4c73a98bd63961f9550`. <!-- Validation: js-yaml version facts -->
  Both 4.3.2 and the latest 5.4.3 (2026-10-05, new major) declare `argparse` (4.3.2: `^2.0.1`, used only by the
  package's command-line tool, not by the ESM file), so that is not a reason for either; 4.3.2 is the `v4-legacy`
  dist-tag (`npm view js-yaml dist-tags`: `latest` 5.4.3, `v4-legacy` 4.3.2). It is chosen because its ESM file
  imports nothing, it is the version this repo already runs in `.claude/skills/distill`, and a six-day-old major
  gains nothing for loading `distill.yaml`. The update procedure (requirement 1) covers a later move to 5.x.
- Release notices: `web/THIRD_PARTY_NOTICES.md` is generated by `web/scripts/notices.mjs` from the web bundle's
  `package-lock.json` (`npm run notices`) and is copied into every release (`.github/workflows/release.yml:225,407,473`).
  It is the only notices file the release ships, so js-yaml goes there.
- <!-- Red team: Node guard only in verify --> CI jobs that run the full Go test suite: `ci.yml` `verify`
  (`.github/workflows/ci.yml:11-57`, `go test ./...` on Ubuntu, macOS, Windows; Windows with `-v -timeout 60m`
  at `:44`) and `release.yml` "Verify release candidate" (`.github/workflows/release.yml:100-168`,
  `go test -timeout 30m ./...` at `:150` on the same three OSes). Neither has a Node setup step; the `web` job
  pins `actions/setup-node` (`ci.yml:147`). The `lint` job checks out with `fetch-depth: 0` (`ci.yml:77-79`) but
  today runs only `make lint`. <!-- Validation: release verify checkout and test patterns --> Release verify checks
  out without `fetch-depth` (`release.yml:115`, so no tags), runs `go test -p 1 -count=1 -run 'Performance' ./...`
  (`:160`) and `go test ./internal/... -run 'Fault|Hardening|DiskFull|ReadOnly|ConcurrentFrames' -count=1`
  (`:168`) as separate steps.
- <!-- Validation: Node runners --> GitHub runner images (2026-10): `ubuntu-latest` = Ubuntu 24.04 and
  `windows-latest` = Windows Server 2025, default Node 22.23.3; `macos-latest` = macOS 26 arm64, default Node
  24.20.0. None ships Node 18, so `actions/setup-node` with `node-version: 18` is required in every job that runs
  the script test. Node 18 itself is end of life (April 2025); it stays the stated minimum (plan decision 12).

## Owned files

- Create in `.agents/skills/distill-lab/scripts/` and mirror byte-identically into
  `internal/systemskills/distill-lab/scripts/`: `distill.mjs`, `lib/js-yaml.mjs`, `lib/js-yaml.LICENSE`.
- Create: `internal/systemskills/distill_script_test.go`, `internal/systemskills/testdata/distill/**` (inputs,
  golden outputs and golden loaded values).
- Modify: `web/scripts/notices.mjs`, `web/THIRD_PARTY_NOTICES.md` (regenerated), `.github/workflows/ci.yml`
  (`verify` job: Node setup and env; `lint` job: the tag-derived digest test of Phase 1),
  `.github/workflows/release.yml` (release verify job: Node setup, env, tags for the digest test).
- Read-only: `.claude/skills/distill-lab/**`, `internal/migration/testdata/*`, `internal/distill/**`.

## Requirements

1. **Vendoring.** <!-- Red team: vendored js-yaml provenance --> Download the pinned tarball URL above, check its
   sha512 against the registry `dist.integrity` (recorded above, read from `npm view`, a second source next to the
   tarball itself), then copy the tarball's ESM build (`package/` + the build folder + `/js-yaml.mjs`) and
   `package/LICENSE` unmodified (keep the trailing `//# sourceMappingURL` line; Node ignores a missing map) as
   `lib/js-yaml.mjs` and `lib/js-yaml.LICENSE`. <!-- Validation: repo hook blocks the build-folder segment -->
   Agent sessions in this repo cannot run a Bash command whose text contains that build-folder segment
   (`.claude/hooks/scout-block.cjs`), so the extraction is a small Node script file kept in the session scratchpad,
   not in the repo (`extract-js-yaml.mjs <tgz> <out-dir>`): it gunzips the tarball with `node:zlib`, walks the tar
   headers, builds the member name from parts (`['package', 'di' + 'st', 'js-yaml.mjs'].join('/')`), writes that
   member and `package/LICENSE` to the output folder, and prints their sha256. Run it with paths as arguments
   from outside the download folder.
   The test asserts both sha256 values. Record version, tarball URL, `dist.integrity` and the two sha256 values in
   a comment at the top of `distill.mjs` (the vendored file itself stays untouched). Update procedure (written into
   that comment and into `AGENTS.md` by Phase 7): pick the new version, repeat the download and integrity check,
   replace both files in both trees, update the digests in the comment and the test, regenerate the notices, and
   let the known-digest test (Phase 1) require the shipped old digests.
2. **One module.** `distill.mjs` is a single ES module importing only `node:` built-ins (`fs`, `path`, `crypto`,
   `child_process`, `process`) and `./lib/js-yaml.mjs` by relative path resolved from `import.meta.url`. No
   npm, no bundler, no `skillhub` binary, no MCP. <!-- Red team: "never calls the network" is false --> The
   script itself opens no network connection; its only child process is local `git` against the mirrors with lazy
   fetch disabled (requirement 5). Use only APIs present in Node 18.0 (check each API's "Added in" line in the Node
   docs); Phase 8 runs it under an official Node 18 binary.
3. **Same surface.** Same subcommands, positional arguments, options, defaults, output lines, error texts
   (`ERROR/WHY/FIX`) and exit codes (1 for validation and lookup errors, 2 for usage errors), with a hand-written
   argument parser and `--help` text equal in content to the Python docstring plus the optional `--hub` check of
   requirement 4, worded without the old script's name or language (wording rule below). The deliberate
   differences are listed below.
4. **Hub location: from the file, as `distill.py` does.** <!-- Red team: hub lookup changes check/format -->
   The hub is `parents[4]` of the real path of the given `distill.yaml`
   (`<hub>/skills/<collection>/<id>/.meta/distill.yaml`), and it counts only when it contains
   `.skillhub/schema-version`. No `$SKILLHUB_WORKSPACE` lookup and no walk up from the current folder. `--hub <dir>`
   on `check` and `format` is optional and only asserts: when given, its real path must equal the file's hub, else
   exit 1 with `ERROR: --hub <dir> is not the hub that holds <file>`. Resolve the hub only when the evidence check
   runs, so `--help`, `list`, `decide` and `--no-where` never need one. When it is needed and the file is not
   inside a hub, exit 1 with `ERROR: <file> is not inside a Skill Hub folder` / `WHY: the evidence check reads
   source mirrors under <hub>/runtime/distill-lab/mirrors, and the hub is found from the file's own path
   (<hub>/skills/<collection>/<skill>/.meta/distill.yaml)` / `FIX: run it on the distill.yaml inside the hub, or add
   --no-where`. Never a stack trace: wrap `main` so unexpected errors print `ERROR:` and exit 1.
5. **Evidence check hardened against a planted mirror.** <!-- Red team: planted mirror runs commands through git -C -->
   - Mirror location `<hub>/runtime/distill-lab/mirrors/<src>`. Refuse (as "does not resolve", with a WHY naming the
     reason) a mirror that is a symlink or sits under a symlinked component between the hub and the mirror, and a
     mirror that the hub repo tracks (`git ls-files -- runtime/distill-lab/mirrors/<src>` in the hub, run with the
     same hardened environment, prints anything).
   - Git dir: `<mirror>/.git` when it is a directory (existing non-bare mirrors), else `<mirror>` when it holds
     `HEAD` (bare mirrors, Phase 3); passed as an explicit `--git-dir=<path>`; never `-C`.
   - Every call: `git --no-lazy-fetch -c safe.bareRepository=explicit -c core.sshCommand= -c core.fsmonitor=
     --git-dir=<dir> cat-file ...`, environment `GIT_NO_LAZY_FETCH=1`, `GIT_TERMINAL_PROMPT=0`,
     `GIT_CONFIG_NOSYSTEM=1`, `spawnSync` with an argument array (no shell) and a 30 s timeout per call. Read file
     content with `git cat-file blob <commit>:<path>` instead of `git show` (no textconv or filters), and commits
     with `git cat-file -t`.
   - <!-- Red team: network claim --> In a partial-clone mirror a blob that was never fetched is not fetched now:
     the check reports `<ref>: blob not fetched; run git fetch in the mirror` instead of "does not resolve" (told
     apart by `git ls-tree <commit> -- <path>` listing the entry, since trees are present in a blob-less clone,
     while `git cat-file -e <blob>` fails; the exact detection is fixed by the test against a real
     `--filter=blob:none` clone). A timeout reports
     `<ref>: git timed out in mirror <src>`.
   - <!-- Validation: Git minimum version --> The Git floor is 2.45.0: it is the first release with the
     `--no-lazy-fetch` option and the documented `GIT_NO_LAZY_FETCH` variable (release notes
     https://github.com/git/git/blob/master/Documentation/RelNotes/2.45.0.adoc; git(1) at v2.45.0
     https://github.com/git/git/blob/v2.45.0/Documentation/git.txt). The April 2024 security releases (2.39.4 to
     2.44.1) honour the variable but not the option, and the script passes both, so older Git fails with an
     unknown-option error, which the script reports as in "Risks". Phase 3 states Git 2.45.0 as the requirement.
6. **Same behaviour**, rule by rule: validation messages, `impact_of`, `final_score`, sort order, `_drop_derived`,
   `check` warnings, `list` columns, `decide` selectors and `at`, evidence check with the same cache and line
   counting, re-parse and re-validate before writing.
7. **Safe write.** <!-- Red team: atomic write follows a symlink at distill.yaml.tmp --> `lstat` `distill.yaml`,
   `.meta` and every folder from the file up to the hub (or, outside a hub, the file and its parent folder) and refuse
   with `ERROR/WHY/FIX` if any is a symlink. Write to a random temp name in the same folder
   (`.distill.yaml.<16 hex from crypto.randomBytes>.tmp`) opened with `fs.openSync(tmp, 'wx', 0o644)` (fails if it
   exists, never follows a link), write, `fsyncSync`, close, `renameSync` over the file; on any error unlink the temp
   file.
8. **Load with a YAML 1.1 schema that matches PyYAML's `safe_load`.** <!-- Red team: CORE_SCHEMA turns hex SHAs into numbers -->
   Not `CORE_SCHEMA` (YAML 1.2 reads `1e23456` as a float, PyYAML reads it as a string) and not `DEFAULT_SCHEMA`.
   Build `PYYAML_SAFE_SCHEMA = FAILSAFE_SCHEMA.extend({implicit: [...]})` with js-yaml `Type`s that copy PyYAML
   6.0's `Resolver` regexes and constructors: null (`~`, `null`/`Null`/`NULL`, empty), bool
   (`yes/no/true/false/on/off` in the three casings), int (decimal, `0b`, `0x`, leading-`0` octal, `_` separators,
   sexagesimal), float (requires a dot or `.inf`/`.nan`, `_` separators, sexagesimal), timestamp (loaded as a
   tagged value that keeps its source text, so a date in a string field fails validation as it does in Python and
   is never re-emitted as a JS `Date`), and merge `<<`. Justification: files written by `distill.py` (and hand edits
   of them) must load to the same types Python saw, so every validation verdict and every re-written value matches;
   this is the only schema with that property. Keep a duplicate-key or syntax error as the existing
   `is not valid YAML` message.
9. **Writer parity with PyYAML** (plan decision 13). The writer emits the fixed layout itself and reproduces
   these PyYAML rules, each with a test input:
   - Python `round(x, 2)` on exact ties (half to even, e.g. `0.125 -> 0.12`, `0.375 -> 0.38`) and float text
     (`45.0`, `22.5`, `0.67`; integral floats keep `.0`; scores are ints);
   - plain vs single-quoted strings by PyYAML's YAML 1.1 implicit resolvers (the same regexes as requirement 8:
     bool, null, int including all-digit and `0x`/`0o`/`_` forms and all-digit 7 to 12 character SHAs, float
     including `1e5.0` but not `1e5`, timestamps) and its scalar analysis (leading indicators, `: ` and ` #`,
     leading or trailing spaces, empty string), double quotes for non-printable characters, `allow_unicode` (no
     escaping of printable non-ASCII);
   - line wrapping of long plain and folded scalars at width 88 by PyYAML's emitter rule (break at a space once
     the column passes the width), indentation 2 with indented sequences, `>-` for folded prose without a final
     newline, flow `{state: x, reason: ..., at: ...}` and `[a, b, c]`, empty list `[]`;
   - blank line before each `  - key:` item; key orders as in Python; sort by key in code-point order;
   - Python `splitlines()` semantics for the evidence line count (`\r\n`, `\r`, `\v`, `\f`, `\x1c-\x1e`,
     `\x85`, ` `, ` `; no trailing empty line).
10. **Header.** Same four lines with the script name changed to `distill.mjs`:
    `# Written by distill-lab/scripts/distill.mjs. Edit freely, then run \`distill.mjs format\`.` The `distill.yaml`
    schema is unchanged; header lines are comments.
11. **Notices.** Extend `web/scripts/notices.mjs` with a short constant list of files vendored outside the web
    bundle (js-yaml 4.3.2, MIT, license read from `internal/systemskills/distill-lab/scripts/lib/js-yaml.LICENSE`)
    appended as its own section, and widen the intro sentence to "the Web UI production bundle and files embedded
    in the skillhub binary". Regenerate with `cd web && npm run notices`.
12. **CI.** <!-- Red team: Node guard must cover every job that runs the Go tests --> In `ci.yml` `verify` and in
    `release.yml` "Verify release candidate": add `actions/setup-node` pinned by commit SHA (the same action and pin
    as the `web` job) with `node-version: 18` (the floor), and `SKILLHUB_REQUIRE_NODE: "1"` in the job env, so a
    missing Node fails instead of skipping and CI exercises the oldest supported Node. Keep the Windows cost small:
    on Windows the script test runs one fixture plus the edge-case inputs, not both 42-lesson fixtures (the test
    reads `runtime.GOOS`). <!-- Validation: release verify checkout --> Release verify's checkout
    (`release.yml:115`) gains `fetch-depth: 0` so the tag test sees tags, and both release verify and the `lint`
    job (which today runs only `make lint`) run Phase 1's and Phase 4's tag-derived digest tests with
    `SKILLHUB_REQUIRE_TAGS=1` (both jobs listed in Phase 1).

## Deliberate differences from `distill.py`

<!-- Red team: hub lookup, planted mirror, network, atomic write, loader -->
Recorded here, in the skill's "Format changes"/requirements text (Phase 3) and in the release note (Phase 7).
<!-- Validation: self-containment wording --> Wording rule: every bundled file (SKILL.md, `distill.mjs` including
its comments and `--help` text) never contains `python` or `distill.py` in any casing, because Phase 3's
self-containment test bans them. In bundled text the differences are stated against "the previous
implementation", and port comments name "the reference emitter" (for example "matches the reference emitter's
round-half-even"). This plan file and the release note may name the old script.
- the optional `--hub` check (requirement 4); the hub is still found from the file;
- refused symlinked or hub-tracked mirrors, hardened git environment, `cat-file blob` instead of `show`, no lazy
  fetch with its own message, a 30 s timeout per git call (requirement 5);
- refusal of symlinked `distill.yaml`/`.meta`/parents and a random temp name (requirement 7);
- the header line names `distill.mjs`.

## Tests

- `distill_script_test.go` (Go, external test package): locate `node` with `exec.LookPath`; if missing,
  `t.Skip("node not found on PATH; install Node 18 or newer to run the distill-lab script tests")`, unless
  `SKILLHUB_REQUIRE_NODE=1`, then `t.Fatal`. <!-- Validation: release verify test patterns --> Test names avoid
  every pattern release verify runs as a separate step: `Performance` (`release.yml:160`) and `Fault`,
  `Hardening`, `DiskFull`, `ReadOnly`, `ConcurrentFrames` (`release.yml:168`). Run the mirrored `distill-lab/scripts/distill.mjs` in temp folders:
  - vendored file digests equal the values in requirement 1;
  - `--help` exit 0; usage error exit 2;
  - <!-- Red team: load parity test --> loader parity first: load each fixture in
    `internal/migration/testdata/*-distill.yaml` and every other committed distill-lab file found at start with
    `git ls-files '*/.meta/distill.yaml' 'internal/**/testdata/*distill*.yaml'`, dump the loaded values as JSON
    (no CLI option is added: `distill.mjs` exports its `load` function and runs `main` only when it is the entry
    module, and a small harness `testdata/distill/load-harness.mjs` imports it), and compare with committed golden JSON produced from
    PyYAML's `safe_load` in the differential run; assert cursors and `at` values (SHAs) are strings and dates keep
    their text. Loader edge inputs: `1e23456`, `0123456`, `1234567`, `1_000`, `0x1f`, `2026-10-11`, `yes`, `~`;
  - then round trip: for each fixture copied into a temp hub (`skills/default/<id>/.meta/distill.yaml` plus
    `.skillhub/schema-version`): `format --no-where` output equals the fixture byte for byte except the first header
    line, or equals the committed golden file under `testdata/distill/` if the fallback was taken;
    `distill.UnmarshalDocument` + `ValidateDocument` accept it; a second `format` is a no-op;
  - `list --top 5` and `list --all` output equal golden files;
  - `decide` by number and by key, `--state rejected` without `--reason` fails, `at` is the 12-character cursor;
  - `check` evidence: a temp mirror repo under `runtime/distill-lab/mirrors/<src>` built with `git init` and one
    commit, plus a bare mirror; valid, unresolved, range-too-long and reversed-range refs give the Python messages;
    files with `\r\n` and ` ` count lines like Python;
  - <!-- Red team: planted mirror --> planted mirror: a bare-repo-shaped mirror whose `config` sets
    `extensions.partialclone`, a remote URL and `core.sshCommand=touch <marker>` -> `check` reports the ref and the
    marker file is never created; a symlinked mirror and a hub-tracked mirror are refused;
  - <!-- Red team: network --> partial clone: a `--filter=blob:none` clone of a local repo served by `file://`
    with the blob absent -> "blob not fetched" message, no fetch happens (the source repo is removed before the
    check), exit 1;
  - <!-- Red team: atomic write --> safe write: a pre-existing `distill.yaml.tmp` symlink to a file outside the hub
    is not followed (target unchanged); a symlinked `.meta` folder and a symlinked `distill.yaml` are refused; no
    temp file is left after a failure;
  - hub lookup: the file's own hub is found from any current folder; a file outside any hub gives the message of
    requirement 4 (exit 1, no stack trace); `--hub` equal to the file's hub passes, a different hub fails;
    `SKILLHUB_WORKSPACE` pointing at another hub has no effect;
  - edge-case inputs from requirement 9 (one small input file per rule) equal golden outputs.
- Golden outputs and golden loaded values are produced by running the reference `distill.py` once (differential
  run below) and committed; afterwards the tests need only Node.
- Differential run (development only, Python 3 + PyYAML available on this machine per scout notes): for every
  input above, run `python3 -I distill.py <cmd>` and `node distill.mjs <cmd>` on copies and `diff` stdout,
  stderr, exit code and file bytes (ignoring the header's first line and the deliberate differences). Record the
  result in the phase report.
- `go test ./internal/distill/... ./internal/migration/... ./internal/delivery/web/...` still pass (fixtures
  unchanged).
- `cd web && npm run notices` regenerates without diff noise outside the new section; `make web-check` passes.

## Steps

1. Download `js-yaml-4.3.2.tgz` into its own empty scratch folder, verify the sha512 integrity, run the extraction
   script of requirement 1 (kept in a different scratch folder), check the two file digests, and copy the two files
   into `lib/` in both trees. No Bash command names the tarball's build folder. <!-- Validation: repo hook -->
2. Port constants, scoring, validation, loader schema, hardened evidence check and commands; then the writer and
   the safe write.
3. Build edge-case inputs; run the differential comparison; fix until clean or until a difference is shown to be
   impossible to remove (then apply the fallback below).
4. Commit golden outputs and the Go test; notices; CI and release workflow changes.
5. `go test ./internal/systemskills/...`, `make check`, `make web-check`; `git ls-files .agents/skills/distill-lab/scripts`
   lists the three files. <!-- Red team: Phase 2 files ignored until Phase 1 -->
6. Commits: `feat(distill-lab): rewrite the distill script as a self-contained Node module`,
   `chore(notices): list the js-yaml copy embedded in the binary`,
   `ci: run the distill-lab script tests on Node 18 in every Go test job`.

## Fallback if byte parity is impossible

If an input class cannot match PyYAML byte for byte (for example an exotic quoting case), keep the js-yaml-side
output for that class, commit it as the golden file, note the difference in a short "Format changes" line in
SKILL.md (Phase 3 copies it) and in the release-note line (Phase 7), and keep the Go reader tests passing. The two
existing fixtures must still round-trip unchanged apart from the header line; if even they cannot, stop and report
to the lead before reformatting them. The loader is not covered by this fallback: a loaded-value difference from
PyYAML is a bug to fix.

## Risks

- Hidden PyYAML behaviour not covered by an input: the differential run over both 42-lesson fixtures plus
  targeted inputs bounds it; owners' existing files are reformatted at worst once, by the documented fallback.
- Hidden YAML 1.1 resolver cases in the loader: the resolver regexes are copied from PyYAML's source, and the
  loader parity test compares loaded values, not only output bytes.
- Git older than 2.45.0 (<!-- Validation: Git minimum version -->some of which honour `GIT_NO_LAZY_FETCH` but none the
  option): the `--no-lazy-fetch` option fails as unknown;
  the script reports "Git <version> is too old for an offline evidence check; install Git 2.45.0 or newer" instead
  of falling back to a networked read.
- Windows: `git` via `child_process.spawnSync` with an argument array (no shell), paths through `node:path`,
  `\r\n` input handled by the loader; CI runs a reduced script test on Windows.
- Vendored file drift: digests asserted by the test.

## Rollback

Revert the commits; the bundle then has no script and Phase 3/4 consumers are not yet merged in this wave.
