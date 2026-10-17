# Does `skill commit` / `skill checkout` move the whole skill folder?

Researched 2026-10-17 16:08 (Asia/Saigon) against `main` at `fc2a8df` and the
installed binary `skillhub 0.7.0` (`7ef39f7`).

## Answer

Both commands already move the whole skill folder, not only `SKILL.md`. The
claim that `skill commit` pushes a single file did not reproduce with either
the current source or the installed binary. No code change is needed for the
file set. The likely source of the impression is that the output lists only the
files that *changed*, which is often just `SKILL.md`.

## Evidence

### Code paths

| Path | What it reads or writes | Where |
|------|-------------------------|-------|
| commit, read side (local and remote) | `readProjectFolder` walks the folder with `filepath.WalkDir` and keeps every regular file | `internal/app/skill_commit_folder.go:76` |
| commit, new skill | `discoveredFromTree` turns every non-`SKILL.md` file into a companion; `buildSkillAddChanges` writes each one | `internal/app/skill_commit_plan.go:235`, `internal/app/skill_add.go:705` |
| commit, existing skill | three-way merge over the full hub tree and full project tree | `internal/app/skill_commit_plan.go:21` |
| commit, hub on another machine | `Send` bundles `preview.tree`, the whole folder | `internal/app/skill_remote.go:190` |
| checkout, local | `readHubSkillTree` then `writeCheckoutFolder` writes every file with its execute bit | `internal/app/skill_checkout.go:66`, `:136` |
| checkout, remote | `fetch` extracts every file listed in the hub bundle | `internal/app/skill_remote.go:355` |

The only files left out are the ones `folderExclusion` names: the checkout
marker, hub `.meta/`, symlinks, non-regular files, `.git/`, `node_modules/`,
`__pycache__/`, `.env*` (except `.example`/`.sample`), `*.pem`, SSH keys and
`.DS_Store` (`internal/source/import_exclude.go:30`).

### Empirical run

A temporary hub plus a project folder with `SKILL.md`,
`scripts/run.sh` (executable) and `references/notes.md`:

1. `skill commit .claude/skills/demo-full --yes` committed all three files plus
   `.meta/skill.yaml`; `run.sh` kept mode `0755` in the hub. Same result with
   the installed 0.7.0 binary.
2. `skill checkout demo-full` in a fresh project wrote all three files and the
   marker; `run.sh` was `0755`.
3. After editing `notes.md`, adding `references/extra.md` and deleting
   `scripts/run.sh`, `skill commit demo-full --yes` produced one hub commit
   with all three changes (modify, add, delete).

### Nested folders

A second run used files up to four folders deep
(`references/api/v2/schemas/user.json`, `scripts/lib/util/run.sh`), a binary
`assets/img/logo.png`, a hidden `.hidden/cfg/settings.ini` and a file name with
a space. Commit, checkout and a follow-up commit (modify a deep file, add a new
`references/api/v3/` folder, delete a deep file) all carried every file at its
full path; the PNG came back byte-identical and `run.sh` kept `0755`. An empty
folder is not carried, because Git does not store empty folders.

### Hub on another machine, viewed in the web UI

A temporary hub ran `serve` (127.0.0.1:17420) and `serve web` (17421); a
project connected with `connect --hub` ran `skill commit <id>` from the project
root.

- A skill the hub did not have: the bundle stored all 4 files under
  `runtime/submissions/SUB-…/files/`, the web API answered "4 files, 4
  changed", and approving wrote all of them to the hub.
- The same skill sent again after editing only `SKILL.md`: the web API
  answered "4 files, 1 changed", case `replace`. The review screen lists only
  `SKILL.md` and folds the other three under "… 3 unchanged files (expand)"
  (`web/src/screens/review/FullReviewScreen.tsx:124`, `:250`).

A third run seeded the hub with a `SKILL.md`-only `pdf-to-epub`, then sent a
3-file folder from a remote project (case `replace`). Approving through the web
API added `references/epub.md` and `scripts/convert.py`. A symlinked subfolder
in the same project was left out, and the preview said
`Left out: tools (symbolic link)`.

The user later reported that the real hub at `100.93.158.78` holds only
`SKILL.md` for `pdf-to-epub` on its Files tab, which lists the stored tree. That
hub is another machine and needs a token this machine does not have, so the
cause there is not yet confirmed.

So the folded "unchanged files" line is the most likely reason the web UI
looked like it received only `SKILL.md`.

## Why it may look like only SKILL.md is sent

- **The preview and the hub commit show changes, not the folder.** After a
  checkout, a commit lists only files that differ from the checkout base
  (`describeRemoteFiles` at `internal/app/skill_remote.go:143`, and the Git
  diff locally). If only `SKILL.md` was edited, that is the only file shown,
  although the remote bundle still carries every file.
- **Symlinked companions are skipped.** A folder whose `scripts/` or
  `references/` are symlinks (common when skills are installed by linking)
  sends only real files; the preview lists the rest under "excluded" as
  "symbolic link".
- **`node_modules/`, `__pycache__/`, `.env` and keys are skipped** by design.

## Side finding

A remote checkout (hub on another machine) drops execute bits: `fetch` builds
`treeFile{Data: data}` without `Executable`, and the hub bundle format omits the
bit by design (`internal/skillruntime/remote_cache.go:72`). Scripts arrive as
`0644` in the project. The local checkout keeps them. This was read from code,
not reproduced against a running `serve`.

## Next steps

1. If you saw only `SKILL.md`, rerun with `--json` and check `files` and
   `excluded`; a symlink or ignored folder will show there.
2. Decide whether the remote-checkout execute-bit loss matters; fixing it means
   adding `executable` to the hub bundle file list, a contract change in
   `docs/contracts/`.
3. Optionally make the preview say "N changed of M files sent" so the full
   folder is visible.

## Unresolved questions

- Which exact command, hub mode (local or `--hub`) and folder produced the
  single-file result you saw? With that, the cause can be pinned to one of the
  three explanations above.
