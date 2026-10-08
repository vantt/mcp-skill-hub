---
title: "Phase 02 — Go foundation, CI and release skeleton"
status: done
---

# Phase 02 — Go foundation, CI and release skeleton

## Objective

Create the minimal dependency-disciplined Go project skeleton for a single `skillhub` binary, including CLI entrypoint, structured result/error primitives, build metadata, CI, and release/install scaffolding.

## Dependencies

- Phase 01 UX contracts exist and define public result/error shape.
- Final technical choices must preserve V1 constraints from `archive/final.md`: local-first, Go binary, no required Node/Python runtime, SQLite derived state, and CLI/MCP adapters over shared application services.

## Related files

Create or modify these paths during cook:

- `/home/vantt/projects/mcp-skill-hub/go.mod`
- `/home/vantt/projects/mcp-skill-hub/go.sum`
- `/home/vantt/projects/mcp-skill-hub/cmd/skillhub/main.go`
- `/home/vantt/projects/mcp-skill-hub/internal/version/version.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/result.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/errors.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/clock.go`
- `/home/vantt/projects/mcp-skill-hub/internal/app/id.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/root.go`
- `/home/vantt/projects/mcp-skill-hub/internal/delivery/cli/version.go`
- `/home/vantt/projects/mcp-skill-hub/internal/systemskills/embed.go`
- `/home/vantt/projects/mcp-skill-hub/system-skills/curator/SKILL.md`
- `/home/vantt/projects/mcp-skill-hub/scripts/install.sh`
- `/home/vantt/projects/mcp-skill-hub/.github/workflows/ci.yml`
- `/home/vantt/projects/mcp-skill-hub/.github/workflows/release.yml`
- `/home/vantt/projects/mcp-skill-hub/README.md`

Do not add workspace mutation, SQLite catalog, MCP transport, source adapters, or resolver logic in this phase.

## Requirements

- `go test ./...` works on the skeleton.
- `go run ./cmd/skillhub version` prints version/build metadata in human-readable form.
- `go run ./cmd/skillhub version --json` prints a JSON result envelope derived from the same application result type.
- The package dependency direction starts as `delivery → app`; domain/infrastructure packages must not import delivery.
- Build metadata supports version, commit, date, and dirty state through linker flags with safe local defaults.
- CI covers Linux, macOS, and Windows, plus lint/test/race/cross-compile smoke checks where practical.
- Release workflow is a skeleton only: archives/checksums/SBOM/signing placeholders are acceptable, but do not publish automatically.
- `system-skills/curator/SKILL.md` is a placeholder embedded asset and does not contain domain mutation logic.

## Implementation steps

1. Initialize the Go module with a stable module path chosen for this repository.
2. Add `cmd/skillhub/main.go` that delegates to `internal/delivery/cli`.
3. Implement the CLI root with only `version` and basic `--json` support.
4. Implement `internal/app.Result`, `internal/app.Error`, and stable error-code primitives shaped by Phase 01 contracts.
5. Add deterministic interfaces for clock and ID generation but keep implementations minimal.
6. Add `internal/version` with linker-flag-backed fields and defaults for local development.
7. Add a placeholder System Curator Skill under `system-skills/curator/SKILL.md` and embed it with `go:embed`.
8. Add `scripts/install.sh` as a non-destructive prototype that documents install intent and exits safely when no release artifact is provided.
9. Add CI workflow with format, test, race, and cross-platform build jobs.
10. Add release workflow skeleton with archive/checksum steps but no automatic publish unless explicitly configured later.
11. Update `README.md` only for current commands and build/test instructions introduced in this phase.
12. Keep dependencies minimal; if choosing a CLI library, document why it is needed and verify license compatibility.

## Acceptance criteria

- `go run ./cmd/skillhub version` succeeds.
- `go run ./cmd/skillhub version --json` emits valid JSON with version metadata.
- `go test ./...` succeeds.
- `go test -race ./...` succeeds or the plan records a platform-specific reason if race is unavailable.
- `go vet ./...` succeeds.
- `go list ./...` succeeds without requiring Node, Python, CGO, or a running database.
- CI and release workflow files are syntactically valid YAML.
- No package outside delivery imports `internal/delivery/...`.

## Validation commands

```bash
gofmt -w cmd internal
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/skillhub version
go run ./cmd/skillhub version --json | python3 -m json.tool >/dev/null
go list ./... >/dev/null
```

Optional dependency-direction check after packages exist:

```bash
if go list -deps ./internal/app ./internal/version 2>/dev/null | grep -q '/internal/delivery/'; then
  echo 'app/version must not depend on delivery' >&2
  exit 1
fi
```

## Risks

- Choosing a CLI or SQLite dependency too early can constrain later phases.
- Release workflow may imply publishing before V1 is ready.
- Placeholder system skill could accidentally encode business behavior that belongs in application services.

## Rollback

- Remove `go.mod`, `go.sum`, `cmd/`, `internal/`, `system-skills/`, `scripts/install.sh`, and CI/release workflows created in this phase.
- Revert README updates related only to build/test/version commands.
- No canonical workspace state or runtime database should exist yet.
