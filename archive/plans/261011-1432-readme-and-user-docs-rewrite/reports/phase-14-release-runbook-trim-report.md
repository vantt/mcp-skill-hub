# Release runbook trim report

## Changes (docs/release-runbook.md, 200 -> 166 lines)

- `## Install, upgrade, and uninstall`: heading kept (anchor `#install-upgrade-and-uninstall` survives); body is now
  one link line to `installation.md` plus the maintainer fact about what the uninstaller removes.
- Preflight intro links `CONTRIBUTING.md#build-and-test` for the everyday `make` gates. Commands unchanged.
- Added a blank line before the "Prerelease tags" paragraph so it no longer renders inside the pipeline list.
- No release step, command, table, signature policy, migration, recovery, rollback or support text changed.
  `### Manual artifact verification` heading unchanged (linked from installation.md).

## Verification

- `git diff docs/release-runbook.md`: only the install section, one intro sentence and one blank line.
- No "simply" or exclamation marks remain.

## Concerns

- The old runbook said the uninstaller removes "only the managed binary and install metadata marker". The
  installer also removes the `PATH` lines it added (`scripts/install.sh:574` help text; installation.md says the
  same). The kept sentence now lists all three, so the runbook and installation.md agree.
- The local test gate block (`go test`, `-race`, `go vet`, gofmt, installer lifecycle) differs from
  CONTRIBUTING's `make check`; kept as is per "do not change the commands".
