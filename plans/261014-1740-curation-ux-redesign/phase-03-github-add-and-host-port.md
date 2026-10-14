# Phase 3: GitHub add flow and the Git host port

## Goal

`skill add` is the command for skills with a tracked upstream on GitHub. Its preview shows everything a person needs
to decide (every file, excluded files, license, routing suggestions). On a TTY, one run chains draft → approve →
activate into one hub commit. URL parsing moves behind a `HostProvider` port, with GitHub as the only provider and
identical behaviour.

## Decisions implemented

- **D7 (URL part)**: `skill add` accepts only GitHub URLs, so a folder is refused with a pointer to
  `skill commit <folder>`. The preview lists every file and the excluded files, the license appears in text mode, and
  routing is suggested from `description` with `--edit-routing`. The TTY y/N chain is: add as draft, approve (always
  asked, since GitHub content is third-party), activate. All of it is one commit (D3). Without a TTY it stops at
  draft, and an agent never approves.
- **"Còn lại" (port)**: `HostProvider` in `internal/source`, GitHub only, behaviour identical.

## Prerequisites

Phase 1 (prompt helper, chain commit) and phase 2 (`SuggestRouting`, the shared new-skill builder, `skill commit
<folder>` existing as the replacement).

## Context

- `PreviewSkillAdd` (`app/skill_add.go:153`) → `resolveSkillAddSource` (`:286`) → local capture (`:294`) or remote
  capture (`:353`, `ParseGitHubLocatorWithOptions` `:364`, `ResolveGitHubRoute` `:378`). Origin kind `github` is
  hardcoded at `:452`, `source_import.go:630, 700` and `source_backfill.go:405`.
- License: `detectSkillLicense` (`skill_discovery.go:282-309`) gives `license_warning`, today shown only in JSON
  (`skill_add.go:832-836`). The P0 plan may already print it in text. If so, this phase only places it in the new
  preview layout.
- Locator: the host check at `internal/source/locator.go:212-215`, the canonical URL at `:238`, tree/blob parsing,
  `validGitHubName` at `:510`, and `ResolveGitHubRoute` at `:287`. Callers: `skill_add.go:364, 378`,
  `source_watch.go:95, 104`, `source.go:485-491`.
- GitHub-only wording: `skill_add.go:367`, `source_watch.go:17, 31, 73, 80, 98`, `web/locator.go:60`,
  `mcpserver/types.go:34, 44`, `skill_add_tools.go:23`. MCP descriptions feed the `tools/list` digest, so their wording
  stays byte-identical.
- `skill confirm PROP-…` stays as the agent's apply path for an add.

## Design

### Add flow

- `skill add <arg>`: if the arg is an existing path, or parses as `file://` without the test option, refuse (exit 2):
  "skill add takes a GitHub URL. To add a folder from this machine: skillhub skill commit <folder>". The JSON error uses
  the existing `invalid_request` code with that FIX. The local branch of `resolveSkillAddSource` stays, because
  `source import` and tests use it. Only the CLI entry refuses.
- Preview layout, sharing phase 2's printer:
  - header: id, source repo@ref, path;
  - `Files (N, size)` with every file;
  - `Left out` with the reasons;
  - `License: <spdx or "not declared">`, plus a warning line when proprietary or unknown;
  - routing suggestions;
  - routing lint;
  - the trust line: "Third-party: agents get it only after you approve its content";
  - the `--diff` hint (prints every file's content as an added-file diff).
- TTY chain (phase 1 deferred commit):
  `Add pdf as a draft? [y/N]` → `Approve this content for agents? [y/N]` → `Activate pdf now? [y/N]`. Activate is
  asked only when readiness passes. Each "n" stops the chain and commits what was accepted so far, in one commit.
- Without a TTY: `--yes` adds the draft with the accepted routing flags. It never approves or activates. The final
  lines print the human's next step ("A person approves it: skillhub skill add … in a terminal, or on the web
  Waiting for review page").
- Routing flags (`--trigger`, `--not-for`, `--example`, `--counter-example`, `--min-scope`, `--operation`) and
  `--edit-routing` are accepted by `add`, as in phase 2.

### Host port

New file `internal/source/host_github.go`, with `locator.go` reduced to the registry:

```go
// HostProvider turns a web URL of a hosting service into a Git repository route.
type HostProvider interface {
    Kind() string                       // stored as origin kind; "github"
    Host() string                       // "github.com", for user-facing text
    Matches(u *url.URL) bool
    ParseRoute(u *url.URL, ref, path string) (*ParsedRoute, error)
}
```

- The registry holds only `GitHubProvider`. `ParseLocatorWithOptions` iterates over it. No match returns exactly
  today's error text (`unsupported host …; only public github.com is supported`).
- `ParsedGitHubRoute` and `ResolveGitHubRoute` are renamed to `ParsedRoute` and `ResolveRoute`, with type aliases to
  keep the diff small. The `file://` test branch stays.
- The four hardcoded `"github"` origin writes take `Kind()` from the provider. The `== "github" || == "git"`
  comparisons (including `skillruntime/trust.go:39`) stay.
- CLI wording builds "supported hosts: github.com" from the registry. Web and MCP wording stay byte-identical (no
  `tools/list` digest change, no `web/src` change).
- Design note in the file header: a self-hosted provider will also need the address policy
  (`internal/source/policy.go`, `deniedAddressRanges`) and credentials, which this port does not solve.

## Files

- Create `internal/source/host_github.go` and `host_registry_test.go`. Modify `internal/source/locator.go`; keep
  `locator_test.go` expectations unchanged.
- Modify `internal/app/skill_add.go`, `source_watch.go`, `source.go`, `source_import.go`, `source_backfill.go`.
- Modify `internal/delivery/cli/skill_add.go` (folder refusal, the new preview, the chain, routing flags) and its tests.
- Update `docs/contracts/error-codes.md` only if the refusal needs its own wording entry.

## Steps (tests first)

1. Host registry tests: every case in `locator_test.go` passes unchanged through `ParseLocatorWithOptions`.
   `gitlab.com` gets the identical error text. `Kind()` is written to origin in the add, import and backfill
   fixtures.
2. Implement the port. Run `go test ./internal/source ./internal/app -run 'Locator|SkillAdd|SourceImport|Backfill|Watch'`.
3. CLI tests:
   - `skill add ./folder` is refused with the FIX.
   - A preview of the `file://` fixture repo, using the existing test option, lists every file, prints the license
     line in text, and shows routing suggestions.
   - TTY chain answers "y y y": one commit, approved, active, and `skill_get` serves it.
   - "y n": a draft-only commit.
   - No TTY with `--yes`: draft, unapproved, no prompt.
   - The `--approve-content` deny rule still covers `skill add`.
4. Implement the add flow changes.
5. `go test ./internal/delivery/mcpserver -run 'ToolsList|Budget|Digest'` proves the MCP wording did not move.

## Mock output

```text
$ skillhub skill add https://github.com/anthropics/skills/tree/main/skills/pdf
Add pdf from github.com/anthropics/skills@main (skills/pdf), tracked for upstream updates.
Files (11, 96 KB):
  SKILL.md                       9.1 KB
  forms.md                       4.0 KB
  reference.md                  16.2 KB
  scripts/fill_fillable_fields.py 3.3 KB
  …
License: Proprietary (LICENSE.txt). Check that you may use it.
Routing, suggested from the description (edit with --edit-routing):
  trigger   "fill a pdf form", "extract text from a pdf"
  not for   "edit a word document"
  min scope single_step
Third-party: agents get pdf only after you approve its content.
Show every file: skillhub skill add https://github.com/anthropics/skills/tree/main/skills/pdf --diff
Add pdf as a draft? [y/N] y
Approve this content for agents? [y/N] y
Activate pdf now? [y/N] y
Committed 7d3e0aa "Add pdf from github.com/anthropics/skills, approve content, activate".
Agents using this hub get it now; other clones after `git -C ~/skill-hub push`.
```

```text
$ skillhub skill add ~/work/team-release
ERROR: skill add takes a GitHub URL.
FIX: To add a folder from this machine: skillhub skill commit ~/work/team-release
```

## Validation

```bash
go test ./internal/source -count=1
go test ./internal/app -run 'SkillAdd|SourceImport|Backfill|Watch|Upstream' -count=1
go test ./internal/delivery/cli -run 'SkillAdd' -count=1
go test ./internal/delivery/mcpserver -count=1
make check
```

## Risks

- **Scripts or docs that use `skill add <folder>` break.** The refusal names the replacement, and phase 6 updates the
  docs. The release note lists it.
- **The rename touches many call sites.** Type aliases keep the diff small. The unchanged locator tests are the gate.
- **The approve prompt approves content the person saw only as a file list.** `--diff` is offered on the line above.
  D7 accepts this.

## Rollback

Revert the phase commits. Folder adds work again. The port is internal.

## Effort

1.5 days.
