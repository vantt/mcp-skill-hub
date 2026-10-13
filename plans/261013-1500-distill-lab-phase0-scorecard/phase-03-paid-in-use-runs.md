---
phase: 3
title: "Paid in-use runs"
label: needs-user-go
status: pending
priority: P2
effort: "3h agent + wall time"
dependencies: [2]
---

# Phase 3: Paid in-use runs on a cloned hub

Starts only after the user says go and has approved: the pinned model, the per-run cap and the total budget, the
credential route, and the run count (default 75). Without that go, skip this phase. Phase 4 then reports static
evidence only.

## Sandbox (exact steps)

```bash
SCRATCH=<session scratchpad>                       # never /home/vantt/skill-hub or the repo
S=$(mktemp -d "$SCRATCH/m6-scorecard-XXXX")
mkdir -p "$S/home" "$S/xdg/config" "$S/xdg/state" "$S/xdg/cache" "$S/xdg/data"
export HOME="$S/home" XDG_CONFIG_HOME="$S/xdg/config" XDG_STATE_HOME="$S/xdg/state" \
       XDG_CACHE_HOME="$S/xdg/cache" XDG_DATA_HOME="$S/xdg/data"
unset SKILLHUB_WORKSPACE
LIVE=/home/vantt/skill-hub
F=skills/default/test-audit/.meta/distill.yaml
sha256sum "$LIVE/$F" > "$S/guard-before.txt"; git -C "$LIVE" status --porcelain >> "$S/guard-before.txt"
git clone --no-local --no-hardlinks "$LIVE" "$S/hub"          # reads the live hub only
skillhub maintain validate --workspace "$S/hub"                # sanity check of the clone; must pass
go run ./cmd/distill-scorecard variants --in "$S/hub/$F" --hub-content "$S/hub/skills/default/test-audit" --out "$S/variants"
```

In zsh, write `"${S}/..."` wherever a `:` follows a variable.

## Preflight (stop on any failure)

1. `claude --version` and `claude --help`: every flag in Phase 2 still exists. Record the version.
2. Authentication under the isolated HOME: `claude -p "reply ok" --tools "" --max-budget-usd 0.05`. If it fails,
   stop and ask the user for the credential route (for example an API key variable from their own shell). Never
   copy `~/.claude` credential files into the sandbox without the user's explicit go, and never print or log a key.
3. Dry run: `go run ./cmd/distill-scorecard run --sandbox "$S" --dry-run ...` prints 75 commands; spot-check 3.
4. One real run per family (4 runs) and check that the outputs parse against the schemas.

## Runs

```bash
go run ./cmd/distill-scorecard run --sandbox "$S" --scenarios cmd/distill-scorecard/scenarios/test-audit.json \
  --model <pinned id> --reps 5 --max-budget-usd <cap> --claude "$(command -v claude)"
go run ./cmd/distill-scorecard report --stats plans/261013-1500-distill-lab-phase0-scorecard/reports/scorecard-static.json \
  --runs "$S/runs" --thresholds cmd/distill-scorecard/thresholds.json \
  --out plans/261013-1500-distill-lab-phase0-scorecard/reports/
```

Afterwards: `sha256sum "$LIVE/$F"; git -C "$LIVE" status --porcelain` must equal `guard-before.txt`. Copy only
`manifest.json` and the per-run summaries into `reports/inuse/` (no raw transcripts, no environment). Then remove
`$S`. No process stays running, since each `claude -p` exits.

## Failure protocol

- The guard differs (live file or live git status changed): stop at once. Do not try to repair anything; report
  to the lead and the user with the two guard outputs.
- Authentication fails, or a flag is missing: stop and ask; do not substitute another host or model.
- More than 20% of runs in a family fail, or spend reaches the approved total: stop, keep the completed runs and
  report. Never raise the cap or the number of repetitions without a new go.
- Results land between thresholds: report "inconclusive"; adding repetitions (for example 10) needs a new go.

## Success criteria

- 75 runs (or the approved count) completed with schema-valid output, plus `reports/inuse/summary.json`.
- The guard is identical before and after; `$S` is removed.

## Risks

| Risk | L×I | Mitigation |
|---|---|---|
| Spend overrun | Low×Med | Per-run `--max-budget-usd`; the runner sums reported cost and stops at the approved total |
| Live hub touched | Low×High | Clone with `--no-local --no-hardlinks`; agent has no tools; guard before and after |
| Model or host drift between runs | Med×Med | Pinned model id; `claude --version` recorded; all runs in one session window |
| `-p` behaves differently from interactive use | Med×Med | Stated in the report as a limit, as in the curator measurement |

## Rollback

Delete `$S` and `reports/inuse/`. Nothing in the repo or the live hub changes in this phase.
