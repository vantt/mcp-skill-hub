# Research: 3-way merge, update-review UX, and cheap upstream fetch for skill folders

Date: 2026-10-04. Scope: 1-20 small text files per skill, Go single binary + embedded React 19 UI.

## Bottom line

- Merge engine: use **`github.com/epiclabs-io/diff3`'s structured `Diff3Merge[string]` API** (pure Go, MIT), and render the conflict markers and per-hunk UI data ourselves. Do not depend on its `Merge()` text output. Keep `git merge-file` as a **test oracle only**, never as a runtime dependency.
- Unified diff: use **`github.com/aymanbagabas/go-udiff`** on the server, with a ~100-line custom React renderer. Add no client diff library.
- Fetch: the repo **already uses go-git**, and go-git v5.19.2 supports **exact-SHA refspecs**. GitHub advertises `allow-reachable-sha1-in-want` (verified empirically below), so B and U can each be fetched with `Depth: 1` and no history.
- UX: a per-file decision table plus a single-file conflict editor (a textarea), with apply blocked while markers remain. No 3-pane editor.

## 1. Go merge / diff libraries

| Option | License | Activity (gh api, 2026-10) | Fit | Verdict |
|---|---|---|---|---|
| `git merge-file -p --diff3` | GPL-2 (exec, not linked) | Git core, stable | Gold-standard semantics. Needs git on PATH. `--zdiff3` needs git >= 2.35; local box has 2.34.1 and returns exit 129 | Test oracle / optional only |
| `epiclabs-io/diff3` | MIT | pushed 2026-05-20, 17 stars, no tags (pseudo-version) | Generic `Diff3Merge[T]` returns alternating `Ok` / `Conflict{A,O,B}` blocks. Has Myers and LCS. `MergeOptions` has excludeFalseConflicts | **Pick (structured API)** |
| `nasdf/diff3` | MIT | last push 2024-02, 3 commits, 23 stars | Built on diff-match-patch, string in/out | Reject (stale) |
| go-git merge | Apache-2 | active | COMPATIBILITY.md says merge is "partial: fast-forward only", with no 3-way merge | Not usable |
| `sergi/go-diff` | MIT | 2025-06, 2.1k stars | Character-level diff-match-patch, already an indirect dep | Not line-diff3. Optional for word highlight |
| `hexops/gotextdiff` | BSD-3 | 2023-09, effectively frozen | Unified diff (gopls fork) | Superseded by go-udiff |
| `aymanbagabas/go-udiff` | BSD-3 | pushed 2026-07-16, 235 stars | Maintained gopls-derived unified diff | **Pick for unified diff** |

Empirical checks (scratchpad, same inputs fed to both tools):
- `git merge-file -p --diff3 -L local -L base -L upstream local base up` produced standard 7-char markers with a `||||||| base` section. **Exit code = number of conflicts (2)**. Per the docs, 0 means clean, 1..127 is the conflict count (capped at 127), and a negative value means error. It also flags *adjacent* line edits as conflicts.
- `epiclabs-io/diff3.Merge(..., detailed=true)` gave the same two conflict regions. However, it emits **9-char markers** (`<<<<<<<<<`, `=========`), **omits the base section**, returns only a `Conflicts bool` with no count, and **dropped the trailing newline**. Hence the advice to use `Diff3Merge` blocks and serialize them ourselves (standard 7-char diff3 markers, newline preserved, count = number of conflict blocks).
- A non-overlapping case (prepend vs append) merged cleanly in epiclabs.

Correctness guard: add a golden test table that runs both engines and compares their conflict counts and clean results whenever `git` is available (`t.Skip` otherwise). Also pre-classify files before merging. Byte-identical files and "only one side changed" need no merge, and those cover most files in practice.

## 2. React/TS diff viewers

| Option | Size | Maintenance | Input | Verdict |
|---|---|---|---|---|
| Custom renderer over server hunks | ~0 KB dep | ours | JSON hunks from Go | **Pick** |
| `react-diff-viewer-continued` 4.4.0 | ~51 KB gz (bundlephobia), pulls emotion, refractor, js-yaml | active (2026-07) | old/new strings, client-side jsdiff | Too heavy for the need |
| `@git-diff-view/react` 0.1.7 | ~1.3 MB unpacked, pulls highlight.js/lowlight | active (2026-07), 743 stars, pre-1.0 | structured hunks or file pair | Nice, but pre-1.0 and heavy |
| `diff2html` 3.4.56 | ~2 MB unpacked, hogan + jsdiff | active, 3.4k stars | unified diff string | Workable for read-only, but HTML-string output fits React poorly |
| Monaco diff editor | wrapper is 5 KB, but monaco ships multi-MB workers/languages | active | file pair | Overkill for an embedded binary |

- Rendering a **server-provided diff is simpler and DRY**: Go already computes the 3-way classification, so let it also emit the hunks. The UI then stays dumb, tests live in Go, and there is a single diff algorithm, so the preview and the apply cannot disagree.
- The web UI currently has no diff or editor deps (React 19, react-query, react-markdown). Keep it that way.

## 3. Conflict UX patterns

- **VS Code merge editor**: Incoming / Current / Result panes, with per-conflict CodeLens actions (Accept Incoming, Current, Combination, Ignore). It is powerful but needs a full editor component.
- **JetBrains**: a 3-pane editor with left/right/result and per-chunk arrows. Same cost profile as VS Code.
- **GitHub web resolver**: a single text editor showing the file with markers. The user edits, then clicks "Mark as resolved" per file, and commit is blocked until every file is resolved. It handles only line-content conflicts; others go to the CLI. **This is the closest match to a minimal web flow.**
- **copier**: `--conflict inline` (the default) writes git-style markers into the file, while `--conflict rej` writes `.rej` patch files. Its docs call `.rej` inconvenient, which is why inline became the default. **cruft** leaves `.rej` files. Both rely on the user's VCS as the safety net, which our users may not have.
- **Minimal-yet-safe for preview/confirm:**
  1. Show a per-file table with status: unchanged, upstream-only, local-only, both-changed (clean / N conflicts), added-upstream, removed-upstream, added-local, removed-local, plus delete/modify conflicts.
  2. Each row gets a default action. upstream-only defaults to take upstream. local-only defaults to keep local. both-changed-clean defaults to auto-merged. both-changed-conflict defaults to *unresolved*. A removed-upstream file with local edits is a conflict that defaults to keep local.
  3. Each row can be overridden with take upstream, keep local, merged (auto), or edit manually. Edit manually opens a textarea prefilled with diff3 markers.
  4. Server-side, refuse to build the mutation preview while any file has action=unresolved or its content still matches a `^(<{7}|\|{7}|={7}|>{7})( |$)` line. The scan must be server-side and not client-only.
  5. The preview shows final per-file diffs (local to result) and states explicitly that the content-approval digest will be invalidated and re-review required. Confirm applies atomically and records new base = U.

## 4. Fetching a subfolder at a commit cheaply

- **Verified**: `GIT_TRACE_PACKET=1 git -c protocol.version=0 ls-remote https://github.com/anthropics/skills` advertises `allow-reachable-sha1-in-want`, `allow-tip-sha1-in-want`, `filter`, and `shallow`.
- go-git v5.19.2 (`remote.go` `isSupportedRefSpec`) accepts exact-SHA refspecs when either capability is present, and otherwise returns `ErrExactSHA1NotSupported`. This resolves the old src-d/go-git#628 limitation.
- So `remote.Fetch(RefSpecs: ["<sha>:refs/skillhub/base"], Depth: 1)` gets B, and likewise U. The existing `syncMirror` only fetches branch or tag tips at depth 1, so a fresh machine cannot read B today unless B is stored locally.
- If the vendored snapshot at B is already persisted, base content needs **no fetch at all**. That is preferable: it is offline and tamper-evident via the digest.
- `git ls-remote` / go-git `Remote.List` is the cheap way to get the upstream HEAD SHA, one round trip.
- **Sparse checkout** reduces only the working tree, not the transfer, and go-git's partial clone cannot backfill withheld objects. Not needed, because the code already reads trees and blobs straight from the bare mirror.
- **Tarball API** `GET /repos/{o}/{r}/tarball/{ref}` returns a 302 to the whole-repo archive with no subfolder filter. It is fine for small repos, poor for monorepos, and API-rate-limited when unauthenticated. Use it as a fallback only.
- **"Behind N commits" for a path:**
  - GitHub compare `/compare/{B}...{U}` gives repo-wide `ahead_by` (not path-scoped). Its `files` list is capped at 300 and has no path filter, so we would have to filter by prefix client-side.
  - List commits `?sha=U&path=<dir>` is path-scoped; paginate until B is reached.
  - Locally, a blobless clone (`--filter=blob:none`) runs `git log B..U -- path` at full-clone speed. Treeless clones are pathological for path history, and shallow clones make `git log` unreliable (GitHub blog).
  - Simplest honest metric: **"N files changed in this skill since B"** from the existing tree diff (`Diff()` already compares B and U trees). Offer the commit count only via the GitHub list-commits API, best-effort.

## Recommendations

1. Add an internal `merge3` package exposing `Merge(base, local, upstream []byte) (result []byte, conflicts int, hunks []Hunk)`. Build it on `epiclabs-io/diff3.Diff3MergeWithOptions[string]` and serialize standard 7-char diff3 markers with labels `local` / `base` / `upstream`, preserving the final newline.
2. Pin epiclabs by pseudo-version. Add golden tests cross-checked against `git merge-file -p --diff3` (skip when git is absent). If divergence appears, the interface lets us swap to vendoring or exec git without UI changes.
3. Short-circuit before merging. Equal hashes mean unchanged. If only one side changed, take that side. Treat binary/non-UTF-8 files or files over a size cap as whole-file choose-a-side, never line-merged.
4. Persist the B snapshot bytes (or their blob hashes plus content) at vendoring time, so review needs only U from the network.
5. Fetch U with go-git exact-SHA refspec at `Depth: 1` into the existing mirror after `Remote.List` resolves the ref to a SHA. On `ErrExactSHA1NotSupported`, fall back to the branch-tip fetch, or to the tarball for github.com.
6. Return per-file status, action defaults, and go-udiff unified hunks as JSON from one review endpoint. The React UI renders them with a small custom component and adds no diff dependency.
7. Per-file actions: take upstream, keep local, merged (auto), edit manually (textarea). The server rejects preview or apply while any file is unresolved or contains marker lines.
8. Route apply through the existing preview/confirm mutation. The preview shows local-to-result diffs, and confirm atomically writes files, sets base = U, and clears the content-approval digest so re-review is forced.
9. Show "N files changed upstream since <short B>". Show a commit count only when the GitHub API answers, and never block on it.
10. Do not use Monaco, a 3-pane editor, `.rej` files, or sparse/partial clones. None of them pays for itself at 1-20 small files.

## Limitations / unresolved questions

- I tested epiclabs diff3 only on two hand cases. Its LCS vs Myers choice and its false-conflict handling on CRLF or whitespace-only edits are untested.
- I did not measure bundle sizes for `@git-diff-view/react` or `diff2html` (bundlephobia failed); the figures above are npm unpacked sizes.
- GitHub Enterprise and non-GitHub hosts may not advertise `allow-*-sha1-in-want`; the fallback path is needed.
- Is the vendored B content already stored byte-exact locally? If not, recommendation 4 is new storage work.
- Should "keep local" on a removed-upstream file keep tracking it as local-only, or detach it from the source?
