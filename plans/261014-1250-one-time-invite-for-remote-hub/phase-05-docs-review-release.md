# Phase 5: Docs, security review, release

## Context
Plan: [plan.md](plan.md). Depends on phases 2, 3 and 4 being merged into local `main`.

## Requirements
- Docs: invite is the default way to connect another machine, `scp` the fallback (no SSH login needed any more).
  Update `README.md` and `README.vi.md` (section "Use one hub from several machines": the three steps become
  `token invite` on the hub machine, paste the printed line on the agent machine, restart the agent),
  `docs/remote-hub-guide.md` (new section "Connect with an invite", the token file location per platform, how to
  get a new invite, why plain LAN is refused, Windows note), `docs/command-reference.md`,
  `docs/design/remote-hub.md` (route `/mcp/pair`, single-use claim, threat notes). No `--print` text anywhere.
- Independent security review by an agent that did not write the code, against the threat list in `plan.md`
  (brute force, tunnel-shared remote address, redirect and proxy leakage, error oracle, token or code in logs, race on
  redeem including Windows, file permissions, downgrade to plain LAN, forwarding headers) and the first red-team
  report in `plans/reports/red-team-261014-invite-plan.md`. Every blocker is fixed before release; a finding needs a test.
- Verify: `gofmt -l .`, `go vet ./...` with `GOOS=windows` and `GOOS=darwin`, `make check`, trailer check on every
  commit, no plan ids in code.
- Real check on this machine with an isolated HOME and temp hub: start `skillhub serve --addr 127.0.0.1:<port>`,
  run `token invite`, run the printed `connect` line from another temp HOME, confirm the token file and modes, confirm
  a second use fails. Never against `/home/vantt/skill-hub`.
- Install the new binary at `~/.local/bin/skillhub` the same way as the last one (keep the previous as a backup).
- Release: tell `mcp-skill-hub-68` that the user asked for a release after completion; 68 pushes, waits for green CI
  on Linux, Windows and macOS, tags **v0.6.7**, writes the notes (mention: invite, `serve --print` was never released,
  the `update` fix, and the reinstall note for v0.6.4 and older). Do not tag or push from this session.

## Files
The doc files above only; fixes found by the review in the files of phases 1 to 4.

## Validation
Review report in `plans/reports/`, `make check` exit 0, CI green on three operating systems, the release visible on
GitHub with signed assets.

## Risk and rollback
A blocker from the review stops the release; nothing is tagged until it is fixed. Before the tag, rollback is
`git revert` of the merge commits.
