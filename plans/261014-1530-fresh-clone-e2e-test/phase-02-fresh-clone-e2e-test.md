# Phase 2: Fresh-clone end-to-end test

## Context

Plan: [plan.md](plan.md). Depends on phase 1 (`docSectionCommands`, `fillDocPlaceholders`). The package
`internal/delivery/mcpserver` already has everything else: `TestMain` HOME/XDG isolation, `buildSkillHub`
(`subprocess_test.go:712`), `newMachine`/`machine.env()` (`remote_e2e_test.go:39-69`), `toolCall`, `resolveFor`,
`decodeSkillGet`-style decoding (`remote_e2e_test.go:187-224`), the MCP result types (`types.go:148,214`), and the
`.mcp.json`-to-process pattern of `TestRemoteHubEndToEndThroughATunnel` (`remote_e2e_test.go:257`).

Behaviour the assertions rest on (read in source, 2026-10-14):

- `connect` writes `mcpServers.skillhub = {type: stdio, command: <binary>, args: [mcp, serve, --profile, runtime,
  --workspace, <hub>]}` (`internal/hostintegration/integration.go:347`).
- An active skill whose `SKILL.md` frontmatter `name` differs from its id is reported by the catalog build as a
  servability warning (`internal/catalog/servable.go` `servableSkillWarnings`) and stays published.
- `skill_resolve` drops such a skill as primary or supporting (`internal/app/resolver.go:242-256`).
- `skill_get` on it returns `local.status = unavailable`, `reason_codes = [not_servable]`
  (`internal/app/skill_snapshot.go:424-425`), and `skills/list` omits it
  (`distribution_integrity_test.go:116`). `skill_get` still returns `content` from the canonical `SKILL.md`
  (`skill_tools.go:178`) today. Decided 2026-10-14: in the runtime profile it must not; plan
  `261014-1650-runtime-skill-get-gate` lands first, and this test asserts the runtime refusal.

## Requirements

- One test, `TestFreshCloneOnANewMachineServesTheHubsSkills`, with two subtests that share one fixture:
  - `init from the Git URL`: the `docs/user-guide.md` "## Move to another machine" lines.
  - `git clone then init`: `git clone --quiet <bare> ~/skill-hub` (the user's own step, which the troubleshooting
    section presupposes), then the `docs/troubleshooting.md` "### Fresh clone on a new machine" line, then the
    user-guide `connect` line.
  - Both then run the `README.md` "### Check" line and the MCP checks.
- Skip on Windows: `t.Skip("the documented commands are POSIX shell lines")`.
- Every failure names the step, the doc line and the process output/stderr; no check only logs.
- Every process gets the machine's env; nothing reads or writes the real HOME, the live hub or a real project.

## Files

- Create `internal/delivery/mcpserver/fresh_clone_e2e_test.go`.
- Read only: `remote_e2e_test.go`, `subprocess_test.go`, `distribution_test.go`, `types.go`, the three docs.
- No production file changes. A defect found here goes to phase 3's follow-up list, not into this file's commit.

## Steps

1. **Binary and machines.** `binary := buildSkillHub(t)`; `old := newMachine(t, "old")`. Each subtest makes its own
   `newMachine(t, "new")`, so the hub stored by `init` on one does not leak into the other. Build a `docEnv` for a
   machine: `machine.env()` plus `PATH=<dir of binary>:<inherited PATH>` (later entries win, as `env()` documents).
2. **Old machine: build and commit a sample hub** (helper `commitSampleHub(t, binary, old) (hub, bare string)`):
   - `mkdir <old home>/skill-hub`; run `skillhub init` there (`cmd.Dir`), exit 0.
   - Good skill A through the CLI, with the flags of `TestCLICreateActivateThenMCPListGetRead`
     (`skill create --id fresh-clone-cli ... --yes`, write its `SKILL.md`, `skill activate ... --yes`), run with
     `cmd.Dir = hub` instead of `--workspace`.
   - Good skill B hand-written like `newBundleHubFixture`: `skills/default/fresh-clone-notes/.meta/skill.yaml`
     (active, reviewed, one distinct trigger), `SKILL.md` with matching `name`, and `references/notes.md`.
   - Broken skill C: `skills/default/fresh-clone-broken/.meta/skill.yaml` active with its own trigger, `SKILL.md`
     frontmatter `name: some-other-name`.
   - `git add skills && git commit -q -m "add sample skills"` in the hub, then `git status --porcelain` must be
     empty (proves what moves is only what Git holds; if init-written files show up as untracked, that is a finding
     for phase 3, not something to `git add`).
   - `git clone --bare --quiet <hub> <old root>/hub.git` → `bare`. Record `git -C hub rev-parse HEAD` and
     `git -C bare for-each-ref` for the final unchanged check.
3. **New machine: run the documented lines** (helper `runDocLine(t, machine, line string)` → combined output; runs
   `sh -c line` with `cmd.Dir = machine.home()` and `docEnv`; `t.Fatalf` on non-zero exit with line and output).
   - Read lines with `docSectionCommands`; assert the expected count per section; assert the README second-machine
     line equals the user-guide `init` line.
   - Fill placeholders: `<git-url>` → `bare`, `<your-project>` → `<new home>/project` (create the folder first;
     it is the user's existing project).
   - Run them in doc order. From the `init` output assert it names `fresh-clone-broken` in a servability warning
     (the user-visible signal) and does not name A or B.
   - From the `status` output assert exit 0 and that it reports the workspace valid and the index current (match
     the README sample wording `Workspace valid` and `search index current`; if `status` exits non-zero only because
     of host integration of a fresh machine, that is a finding, not a reason to drop the check).
4. **Start the server the agent host would start.** Read `<project>/.mcp.json`, take `mcpServers.skillhub`; assert
   `EvalSymlinks(command) == EvalSymlinks(binary)` and `args == [mcp serve --profile runtime --workspace <new hub>]`
   with `<new hub>` = `EvalSymlinks(<new home>/skill-hub)`. Start it with `mcp.CommandTransport`
   (`TerminateDuration: 15s`), `cmd.Dir = project`, env = machine env, stderr into a buffer that is attached to
   every failure. Register `skills/list` as in `startRemoteBenchSession`. Close the session in `t.Cleanup`.
5. **MCP assertions** (helper `assertServesSampleHub(t, session, newHub, oldHub)`):
   - `resolveFor(t, session, <A's trigger>, ...)` → `Resolution.Primary.ID == "fresh-clone-cli"`; same for B.
   - `skill_get` A and B: not `IsError`; `local.status == "ready"`; `local.path` is inside `<new hub>/runtime/` and
     not inside the old machine's root; the bytes of `<local.path>/SKILL.md` equal the old hub's committed
     `SKILL.md`; for B also `references/notes.md`.
   - `resolveFor(t, session, <C's trigger>, ...)` → primary is nil or not `fresh-clone-broken`, and C is in no
     `Supporting` entry.
   - `skill_get` C (runtime profile): a tool error with no `content` and no `local.path`, as defined by
     plan `261014-1650-runtime-skill-get-gate`.
   - `skills/list` (all pages): A and B listed, C absent.
6. **Nothing else changed.** After both subtests: old hub HEAD and `git status --porcelain` unchanged, bare refs
   unchanged.

## Validation (narrowest first)

```bash
go test -count=1 -v -run '^TestFreshCloneOnANewMachineServesTheHubsSkills$' ./internal/delivery/mcpserver/
go test -count=1 -race -run '^TestFreshClone' ./internal/delivery/mcpserver/
go test -count=1 ./internal/delivery/mcpserver/
make check
```

Before the first green run, check the isolation by hand once: `ls ~/skill-hub` and `git -C /home/vantt/skill-hub
status` show no change, and no new entry appears in the real `~/.config/skillhub`.

## Risks

- `init <bare path>`: `TestInitFromARemoteHubChecksItOut` shows a local bare path works as a Git URL; if a
  `file://` URL is needed on macOS, use it and note why.
- Skill A's trigger may also match B (both are short fixture texts): give each skill one distinctive trigger and
  `not_for` the other's, and assert primary only, never ranking among supporting skills.
- `status` wording changes: the assertion names the README sample it mirrors, so a wording change fails with a
  pointer to the doc that must change too.
- `skill create` flags drift: the test fails at fixture setup with the CLI output, which is the right failure.

## Rollback

Delete `internal/delivery/mcpserver/fresh_clone_e2e_test.go`. No production code changes.
