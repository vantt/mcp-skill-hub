---
phase: 1
title: "Folder import: excluded files, executable bit, add preview and add messages"
status: pending
effort: 1d
depends_on: []
---

# Phase 1: folder import and `skill add` messages

## Context

`skill add <dir>` copies every regular file of the folder into the hub, `.env` included, writes every new file
with mode 0644, and prints a preview that counts only companion files and lists none of them. Its text output also
hides errors and warnings that `--json` shows. Reproduced on 2026-10-14 with a folder holding `SKILL.md`,
`scripts/bump.sh` (0755) and `.env`:

```text
$ skillhub skill add ../src/team-release
Add team-release as a draft from local.
2 files, 188 B. Origin retained. Agent use: off.
...
$ skillhub skill add ../src/team-release --yes
Draft team-release added. Agent use: off. Changes are not committed.
$ ls -la skills/default/team-release skills/default/team-release/scripts
-rw-r--r--  .env
-rw-r--r--  scripts/bump.sh
$ skillhub skill review team-release
Files:        4 files, 327 B
```

## Items, evidence and fixes

### 1.1 Secret and junk files are copied into the hub (report CLI P0 #1, UC5a pain 1)

**Evidence.**
- `internal/source/filesystem.go:395-445`: the capture walk keeps every regular file; there is no exclusion.
- `internal/app/skill_discovery.go:445-503`: discovery adds every resource under the skill folder as a companion;
  only hub metadata (`workspace.IsHubMeta`, `:460`) is skipped.
- `internal/app/skill_add.go:652-657`: every companion becomes a `mutation.Change`.
- `internal/delivery/cli/skill_add.go:113-114`: the preview prints only a count
  (`len(preview.Resources)`) and total bytes, never the paths.
- The same discovery feeds `source import` (`internal/app/source_import.go:204`) and the source inventory
  (`internal/app/source.go:269`, `internal/app/upstream.go:329`). `skill update` reads upstream files through
  `validateAndFilterUpstreamPaths` (`internal/app/upstream_update.go:230-260`), which also keeps every file.
- `maintain validate` passes on a hub holding `skills/default/team-release/.env` (report UC5a).

**Fix.**
1. New `internal/source/import_exclude.go` with one exported helper, `ExcludedFromImport(rel string) (reason
   string, excluded bool)`, applied to a slash path relative to the skill or source root:
   - directories anywhere in the path: `.git/`, `node_modules/`, `__pycache__/` (reason "build output or
     version-control data");
   - base names: `.env` and `.env.*`, `*.pem`, the SSH private key names `id_rsa`, `id_dsa`, `id_ecdsa`,
     `id_ed25519` (each with an optional `_sk` suffix), and `.DS_Store` (reason "looks like a secret" or
     "system file"). No other pattern. See plan open question 2 for the narrowing of `id_*` and the added `.git/`.
2. `FilesystemAdapter.captureDirectory`: when a directory matches, record it and return `fs.SkipDir`; when a file
   matches, record it and skip it. The manifest gets `Excluded []string` (`json:"excluded,omitempty"`), so a large
   `node_modules/` no longer trips the 8 MiB limit. Excluded paths are not hashed, so the folder digest stays a
   digest of what is imported.
3. `DiscoverSkillsFromResources`: skip a resource whose skill-relative path matches the helper and append it to a
   new `DiscoveredSkillItem.Excluded []ExcludedFile` (`path`, `reason`; `json:"excluded,omitempty"`). This covers
   Git sources, `source import` and the inventory with no other call site.
4. `skill_add.go`: merge the capture's excluded list (local) and the item's list into a new
   `SkillAddProposal.Excluded` field (`json:"excluded,omitempty"`) and the same field on `SkillAddResult`.
5. `upstream_update.go`: in `validateAndFilterUpstreamPaths`, drop excluded upstream paths with a warning
   `import_excluded` naming each. Drop the same paths from the local and base maps before the three-way plan, so a
   file a hub already holds from before this fix is neither updated nor deleted by an update.
6. `metadata_lint.go`, `workspaceLintWarnings`: add a warning `secret_like_file` listing every file under
   `skills/` that the helper excludes, with the fix "Remove it from the hub with `git rm --cached` and delete it, or
   move the secret out of the skill folder". Validation still passes (warning, not error).
7. `internal/delivery/cli/source_import.go`: print one "Left out: <path> (<reason>), ..." line per item that has
   exclusions.

**Tests first (must fail today).**
- `internal/source/import_exclude_test.go`: table test of the helper: `.env`, `.env.local`, `config/.env`,
  `key.pem`, `id_ed25519`, `.DS_Store`, `node_modules/x/i.js`, `a/__pycache__/m.pyc`, `.git/config` excluded;
  `id_mapping.py`, `environment.md`, `scripts/run.sh`, `SKILL.md` kept.
- `internal/source/filesystem_limits_test.go` (or a new `filesystem_exclude_test.go`): a captured folder with `.env` and `node_modules/` lists neither in
  `Resources` and both in `Excluded`.
- `internal/app/skill_add_test.go`: `TestSkillAddFromFolderLeavesOutSecretFiles`: preview `Resources` lacks
  `.env`, `Excluded` holds it, and after confirm `skills/<c>/<id>/.env` does not exist.
- `internal/app/source_import_test.go`: a source with a `.env` beside `SKILL.md` imports without it.
- `internal/app/upstream_update_test.go`: upstream adds `.env` → not in the preview files, warning present; a
  local `.env` already in the hub is untouched after apply.
- `internal/app/metadata_lint_test.go`: a hub with `skills/c/s/.env` validates with a `secret_like_file` warning.

### 1.2 Executable bit is lost (report web UC1 "So với CLI", user item 2)

**Evidence.**
- `internal/source/types.go:130-133`: `Resource` has `Path` and `Size` only.
- `internal/source/filesystem.go:440`: the capture never reads the mode. `internal/source/git_repository.go:350`
  drops the mode it already checks at `:301` and `:392` (`filemode.Executable`).
- `internal/mutation/mutation.go:33-38`: `Change` has no mode; `internal/mutation/filesystem.go:101` writes every
  new file 0644 (an existing file keeps its mode).
- Reproduced: `scripts/bump.sh` 0755 in the folder, `-rw-r--r--` in the hub.

**Fix.**
1. `source.Resource` gets `Executable bool` (`json:"executable,omitempty"`). The filesystem capture sets it from
   `info.Mode()&0o111 != 0`; the Git adapter from `filemode.Executable`. The manifest JSON gains the field
   (omitted when false, so old manifests read the same).
2. `DiscoveredCompanion` gets `Executable bool` (`json:"executable,omitempty"`), copied from the resource.
3. `mutation.Change` gets `Executable bool` (`json:"executable,omitempty"`), and the internal transaction
   `change` (`internal/mutation/transaction.go:26`) carries it so crash recovery publishes the same mode. A new
   file is created with 0755 when it is set. A replaced file keeps today's rule (existing mode), plus the exec
   bits when the change sets it.
4. `proposalDigest` (`internal/mutation/mutation.go:408-419`) appends an `x` marker for an executable change only,
   so digests of every existing proposal are unchanged and a stored proposal cannot flip a mode unnoticed.
5. `skill_add.go:652-657` and `source_import.go:760-763` set `Executable` from the companion.

**Tests first.**
- `internal/mutation/mutation_test.go`: an executable change creates a 0755 file; recovery after the fault
  injection before publish keeps 0755; the digest of a non-executable write set equals the digest computed before
  the change (golden value).
- `internal/app/skill_add_test.go`: `TestSkillAddFromFolderKeepsExecutableScripts` (0755 in, 0755 in the hub).
- `internal/source/adapters_test.go`: an executable blob in a test repository lists `Executable: true`.
- Skip the mode assertions on Windows (`runtime.GOOS == "windows"`).

### 1.3 Preview lists every file, and counts match review (report CLI UC1a pain 4, P0 #1)

**Evidence.** `internal/delivery/cli/skill_add.go:113-114` prints `len(preview.Resources)` (companions only, so
`SKILL.md` is not counted) next to `TotalBytes` (which includes `SKILL.md`): "2 files, 188 B" for three files
imported. Review prints "4 files" because it also counts `.meta/skill.yaml` (fixed for review in phase 2).

**Fix.** `writeSkillAddPreview` prints `Files (N, B):` then one line per file, `SKILL.md` first, then each
companion with its size and `(executable)` when set; then `Left out (not copied into the hub):` with one line per
excluded path and reason. N counts `SKILL.md` plus companions, never hub metadata, which is the rule phase 2 applies
to review and the web.

**Tests first.** `internal/delivery/cli/skill_add_test.go`: for the folder with `SKILL.md`, `scripts/bump.sh` and
`.env`, the preview text contains `Files (2,`, lists `SKILL.md` and `scripts/bump.sh`, and contains `Left out` and
`.env`.

### 1.4 Re-adding an unchanged folder reports a new draft (report CLI UC1a pain 2, user item 3)

**Evidence.** Reproduced on an active skill: the preview prints
`skillhub skill confirm --proposal '' --proposal-digest '' --base-version ''` and `--yes` prints
"Draft team-release added. Agent use: off." (exit 0). The app knows: `lookupReplayedSkillAdd`
(`internal/app/skill_add.go:678-725`) and `ConfirmSkillAdd` (`:875-897`) return `Replayed: true` and the summary
"already added (replayed)"; the printers `writeSkillAddPreview` and `writeSkillAddResult`
(`internal/delivery/cli/skill_add.go:88-145`) ignore it.

**Fix.** When `Replayed` is set, both printers print
"<id> was already added from this source; nothing changed. It is <state> now." with the state from
`Assessment.Canonical` (lifecycle), then `Next: skillhub skill review <id>`, and no confirm command. Exit 0. The
message deliberately does not promise an update path (D7 brings it).

**Tests first.** `internal/delivery/cli/skill_add_test.go`: add, activate, add again (preview and `--yes`): output
contains "already added" and "active", contains no `--proposal ''`, and no "Draft ... added".

### 1.5 An error preview under `--yes` reports "proposal is stale" (report CLI UC1b pain 1, user item 7)

**Evidence.** Reproduced: `skill add ../nope` prints "cannot access local directory"; `skill add ../nope --yes`
prints "ERROR: The proposal is stale. WHY: workspace state changed after preview". `runSkillAdd`
(`internal/delivery/cli/skill_add.go:70-80`) confirms with `--yes` without checking the preview's error, and
`ConfirmSkillAdd` (`internal/app/skill_add.go:899-904`) turns the empty plan into a stale error. `--ref <sha>`
(report) is one case of this.

**Fix.** Two guards: `runSkillAdd` writes the preview through `writeResult` when `app.ErrorOf(preview) != nil`
and returns (exit 2); `ConfirmSkillAdd` returns the preview's own error result when the preview carries one, so
every adapter (CLI confirm, web, MCP) gets the real cause.

**Tests first.** CLI: `skill add <missing dir> --yes` → exit 2, stderr contains "cannot access local directory",
not "stale". App: `ConfirmSkillAdd` on an error preview returns that error code.

### 1.6 Repository over 8 MiB gives an unrelated FIX (report CLI UC1b pain 2)

**Evidence.** `internal/app/skill_add.go:440-442` wraps the list error as a plain error; the CLI then prints the
generic `skillErrorFix` text "Correct the skill fields or workspace state and retry."
(`internal/delivery/cli/skill.go:832`). The local capture limit (`internal/app/skill_add.go:322-330`) has the same
gap.

**Fix.** In `captureRemoteSkillAddSource` and `captureLocalSkillAddSource`, `errors.As(err,
&sourcepkg.LimitExceededError{})` returns an invalid-request proposal: WHY keeps the limit message; FIX for a
repository: "Point at one skill folder with `--path <folder>` (or a `/tree/<ref>/<folder>` URL), or pick one skill
with `--skill <name>`."; for a folder: "Add the skill's own folder, not a parent; build output and secrets are left
out automatically." The app, not the CLI, owns the text, so the web shows it too.

**Tests first.** `internal/app/skill_add_test.go` with a fake Git adapter whose `List` returns a
`LimitExceededError`: the proposal's FIX mentions `--path` and `--skill`.

### 1.7 License warning only in `--json` (report CLI UC1b pain 3, user item 8)

**Evidence.** `internal/app/skill_add.go:832-836` adds `license_warning` to `Warnings`; `writeSkillAddPreview`
prints no warning.

**Fix.** The preview prints each warning with `p.Warning`, after the file list. For a local origin, skip the
"License is unknown" case (your own folder needs no license) but keep a declared proprietary license.

**Tests first.** CLI test with a fake Git source whose `SKILL.md` declares `license: Proprietary`: text preview
contains "proprietary". Local folder without a license: no license line.

## Steps

1. Write the failing tests of 1.1-1.7; run them and record the failures.
2. Add the helper and the `Executable` fields (source, discovery, mutation); make the source and mutation tests pass.
3. Wire exclusion and mode through discovery, `skill_add.go`, `source_import.go`, `upstream_update.go`,
   `metadata_lint.go`.
4. Fix the CLI printers and the two `--yes` guards; add the limit FIX texts.
5. Docs: in `docs/curating-skills.md`, "Add a skill" (lines 134-170): replace "Skill Hub copies the other files in
   the folder too, hidden ones included, so keep secrets out of it" with what is left out and that the preview lists
   every file; update the sample preview at lines 158-166 to the new shape. No other doc changes.

## Validation

```bash
go test ./internal/source/ -run 'Exclud|Executable' -count=1
go test ./internal/mutation/ -count=1
go test ./internal/app/ -run 'SkillAdd|SourceImport|UpstreamUpdate|Lint' -count=1
go test ./internal/delivery/cli/ -run 'SkillAdd|SourceImport' -count=1
make check
```

## Risks

- **Mutation core.** `Change` and the WAL record gain a field. Mitigation: additive JSON, digest unchanged for
  non-executable changes (golden test), recovery test. Ask for a code review of `internal/mutation` before merge.
- **Folder digest change.** A folder that held an excluded file gets a new digest, so one replay lookup after the
  upgrade misses and the add reports the existing-skill conflict instead of "already added". Acceptable.
- **False positives.** A skill that truly ships a `.pem` (for example a test certificate) loses it with a visible
  "Left out" line. Open question 1.
- **Upstream update.** Dropping excluded paths from all three sides must not mark them deleted; covered by the test.
- **Strict decoders.** The transaction manifest (`internal/mutation/recovery.go:379`) and the snapshot manifest
  (`internal/source/filesystem.go:568`) reject unknown fields. New code reads old records unchanged. Old code
  cannot read a record that carries the new fields: see rollback.

## Rollback

Revert the phase's commits while no mutation is in progress, because a pending transaction record that carries
`executable` is rejected by the old decoder. Snapshot manifests with the new fields sit in
`<hub>/runtime/cache/snapshots`, which the old code treats as missing history: delete that folder after the revert
(it is a cache; the next add captures again). 0755 files stay valid hub content
(`100755` is accepted by `internal/app/git_index_snapshot.go:116`). Excluded files that were never copied need no
rollback.
