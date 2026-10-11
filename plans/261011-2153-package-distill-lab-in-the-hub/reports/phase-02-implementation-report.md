# Phase 2 implementation report: the distill script as a self-contained Node module

Branch `feat/distill-lab-bundle-and-script`, worktree `/home/vantt/projects/mcp-skill-hub-distill`, on top of Phase 1.

## Commits

1. `feat(distill-lab): rewrite the distill script as a self-contained Node module`: `scripts/distill.mjs`,
   `scripts/lib/js-yaml.mjs`, `scripts/lib/js-yaml.LICENSE` in `.agents/skills/distill-lab/` and the embed mirror
   `internal/systemskills/distill-lab/`, `internal/systemskills/distill_script_test.go`,
   `internal/systemskills/testdata/distill/**`, this report.
2. `chore(notices): list the js-yaml copy embedded in the binary`: `web/scripts/notices.mjs`,
   `web/THIRD_PARTY_NOTICES.md` (regenerated; the only diff is the widened intro sentence and the new section).
3. `ci: run the distill-lab script tests on Node 18 in every Go test job`: `ci.yml` verify (Node 18 via the
   pinned `actions/setup-node`, `SKILLHUB_REQUIRE_NODE=1`), `ci.yml` lint (tag test with `SKILLHUB_REQUIRE_TAGS=1`),
   `release.yml` verify (Node 18, `SKILLHUB_REQUIRE_NODE=1`, `fetch-depth: 0`, tag test step).

## Vendoring

Tarball `https://registry.npmjs.org/js-yaml/-/js-yaml-4.3.2.tgz` downloaded into its own scratch folder; its sha512
equals the registry `dist.integrity` and its sha1 the `dist.shasum`. A scratch script (`extract-js-yaml.mjs`,
not committed) extracted the ESM build and `package/LICENSE`: 104118 bytes / sha256 `cea276c7...0636f` and 1084
bytes / sha256 `a07bc244...f9550`, as the plan recorded. Copied unmodified to both trees; the test asserts both
digests in both trees. Provenance and the update procedure are in the header comment of `distill.mjs`.

## Design as built

- One ES module; imports `node:fs`, `node:path`, `node:crypto`, `node:child_process`, `node:process`, `node:url`
  (for the entry-module check) and `./lib/js-yaml.mjs`. It exports `load`, `loadText`, `main` and the value
  classes for the test harness; `main` runs only when the file is the entry module. No `python`/`distill.py`
  in the module or the vendored files (checked with grep).
- Loader: `FAILSAFE_SCHEMA` extended with the reference loader's YAML 1.1 implicit resolvers (bool, float, int,
  merge, null, timestamp, value; same regexes, same first-character tables, same `$` rule), its constructors
  (octal, binary, hex, sexagesimal, `_`, `.inf/.nan`, dates and datetimes with offsets), typed mapping keys
  (an unquoted `1` and a quoted `'1'` stay different keys; key order kept, including integer-like keys),
  floats kept distinct from integers, and `<<` merges flattened in the reference order. The writer is a port of
  the reference emitter (representer, anchors `&id001`, scalar analysis, plain/single/double/folded writers,
  width 88, indent 2, unicode) driving the fixed layout. Python-runtime details are reproduced: `round(x, 2)`
  half-to-even on the exact binary value, float `repr`, `str.split/strip` whitespace, `\d` of any script,
  `splitlines()` line counting, `repr()` in messages.
- Evidence check: hub from the file (`realpath`, five parents, `.skillhub/schema-version`), resolved only when
  the check runs; `--hub` only asserts. Git runs as `git --no-lazy-fetch -c safe.bareRepository=explicit
  -c core.sshCommand= -c core.fsmonitor= --git-dir=<dir> cat-file ...` with `GIT_NO_LAZY_FETCH=1`,
  `GIT_TERMINAL_PROMPT=0`, `GIT_CONFIG_NOSYSTEM=1`, argument arrays, 30 s timeout; `ls-tree` + `cat-file -e`
  tell a missing partial-clone blob apart. A Git older than 2.45.0 or a missing git gives an ERROR/WHY/FIX exit 1,
  checked only once a mirror exists.
- Safe write: `lstat` of the file and every folder up to the hub (or the file and its folder outside a hub),
  random `.<name>.<16 hex>.tmp` opened `wx` 0644, fsync, rename; temp removed on failure.

## Differential run against the previous implementation

Python 3.14 with PyYAML 6.0.3, `python3 -I`, separate copies per run, comparing stdout, stderr, exit code and
the file (header line and script name masked).

- Loaded values (typed JSON) identical for both 42-lesson fixtures and all six edge inputs
  (`loader-scalars`, `quoting`, `wrapping`, `numbers-and-order`, `anchors-and-timestamps`, `invalid`).
- `format --no-where` byte-identical (apart from the header line) for both fixtures and the four valid edge
  inputs; no fallback reformat was needed.
- 69 command runs identical: `check`, `list` (default, `--top 5`, `--all`, `--top=-3`, `--top 0`, `--to ' 2 '`),
  `decide` (numbers, keys, rejected with and without reason, out-of-range, unknown key, duplicates, interleaved
  options), the 37 validation messages of `invalid.yaml`, and argparse-style usage errors. The only differences:
  the `check`/`format` usage line shows `[--hub HUB]`, and the `decide` usage continuation lines are one column
  wider because the script name is one character longer.
- Evidence check identical on a scratch hub with a non-bare and a bare mirror (valid, unresolved, too-long and
  reversed ranges, missing mirror, `\r\n` and ` `/`\v`/`\f`/`\x1c` line counting).
- Control experiment: a plain `git show` on the planted partial-clone mirror runs the planted `core.sshCommand`;
  the script does not (test asserts the marker never appears).

## Tests

`internal/systemskills/distill_script_test.go` (package `systemskills_test`, 15 tests, none matching the release
patterns `Performance|Fault|Hardening|DiskFull|ReadOnly|ConcurrentFrames`): vendored digests, help and usage
exit codes, loader parity (golden JSON per committed distill file found with `git ls-files` plus the inputs),
rounding, fixture round trip + Go reader/validator + second-format no-op, edge-case goldens, list goldens, decide
goldens and the rejected-without-reason error, validation messages golden, evidence check against real mirrors
(run from an unrelated folder with `SKILLHUB_WORKSPACE` naming another hub, `--hub` match and mismatch), planted
mirror and partial clone (no fetch, no command), symlinked and hub-tracked mirrors, safe write (old fixed temp
name symlink not followed, symlinked `distill.yaml` and `.meta` refused, no temp left), hub needed only for the
evidence check, and the embedded bundle copied alone runs `--help`, `list` and fails `check` with ERROR/WHY/FIX.
Windows runs one 42-lesson fixture.

## Validation

- `go test ./internal/systemskills/...` pass (Node 24.21.0), and all script tests pass under the official Node
  v18.20.8 linux-x64 binary (downloaded by a scratch script, checked against `SHASUMS256.txt`).
- `make fmt-check vet` and `go test -count=1 ./...` pass (includes `internal/distill`, `internal/migration`,
  `internal/delivery/web`; fixtures unchanged).
- `make web-check` pass; `npm run notices` regenerates with no diff outside the new section (run with an
  isolated HOME and npm cache; `npm ci` populated the worktree's ignored `web/node_modules`).
- `make lint LINT_BASE=327f2ed`: one finding, in Phase 1's file (see concerns); none in this phase's files.
- `git ls-files .agents/skills/distill-lab/scripts` lists the three files after the commit.

## Differences not in the plan's list (Phase 3 and Phase 7 should name them)

1. Duplicate mapping keys: the previous loader kept the last value silently; the new one reports
   `is not valid YAML` (requirement 8 asks for this).
2. Inputs that crashed the previous script with a traceback (missing file, `list`/`decide` on a malformed file,
   impossible timestamps, non-UTF-8 input) now print ERROR/WHY/FIX; the exit code stays 1. A non-UTF-8 blob in
   the evidence check is counted with replacement characters instead of crashing.
3. Output is UTF-8 with LF on every OS (the previous script wrote the locale encoding and CRLF on Windows).
4. A `where` that names a directory resolved with `git show` (a tree listing) and now does not resolve
   (`cat-file blob`, as requirement 5 specifies).
5. On `check`/`format`, `--h` is now ambiguous between `--help` and `--hub`.
6. Git calls drop inherited `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_COMMON_DIR`,
   `GIT_OBJECT_DIRECTORY`, `GIT_ALTERNATE_OBJECT_DIRECTORIES` and `GIT_NAMESPACE`, so an outer git context cannot
   redirect the reads (small addition to the listed hardening).

Residual loader gaps, all YAML that no distill file uses: NEL/LS/PS as line breaks inside scalars (js-yaml
parses YAML 1.2), mapping keys that are collections (the previous loader rejected them as unhashable; js-yaml
stringifies them, only partly detected), and explicit `!!binary`/`!!set`/`!!omap`/`!!pairs` tags (now invalid YAML).

## Concerns

- `make lint` reports SA9009 at `internal/systemskills/bundles.go:25` (Phase 1): the comment line starting
  `// go:embed cannot reach ...` reads as an ineffective directive. Not this phase's file; a one-word rewording
  fixes it.
- The CI tag-test step runs `-run 'CoverReleased'` over `./internal/systemskills/...` and
  `./internal/hostintegration/...`; Phase 4 must name its wrapper tag test to match (for example
  `TestKnownWrappersCoverReleasedRenders`).
- Typed keys rely on js-yaml's documented `listener` option and on `String(key)` of constructed values; a js-yaml
  update must rerun the loader parity test, which would catch a change.
- Not run here: Node 18.0.0 exactly (18.20.8 was used; CI runs the latest 18.x), Windows and macOS (symlink
  subtests skip where links cannot be created).
- The workflow edits sit on `327f2ed`; a rebase onto the onboarding release may conflict in `ci.yml` and
  `release.yml` (plan "Re-verify at start").
