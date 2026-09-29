# Skill Hub

Skill Hub is a planned local-first, Git-backed hub for curated agent skills. The V1 target is a single Go binary with CLI and MCP adapters over shared application services; Git owns durable workspace state and derived local state remains rebuildable.

## Current foundation status

The current foundation provides workspace initialization, validation, doctor remediation, offline rebuild of immutable SQLite catalog generations, the curated skill lifecycle (`draft → active → deprecated → archived`), and source intake/monitoring. Managed semantic changes are previewed, explicitly confirmed, receipted, and published through a new catalog generation. A Go toolchain is the only development prerequisite; the binary requires neither Node nor Python at runtime. SQLite is embedded through the maintained pure-Go `modernc.org/sqlite` driver, so builds require neither CGo nor a system SQLite library.

Use these commands from the repository root:

```bash
# Print version and build metadata
go run ./cmd/skillhub version

# Print the same result as JSON
go run ./cmd/skillhub version --json

# Rebuild all derived catalog state from canonical files, without network access
go run ./cmd/skillhub rebuild --workspace /path/to/workspace

# Preview a draft; add --yes only after reviewing the proposal
go run ./cmd/skillhub skill create --workspace /path/to/workspace \
  --id reliability-review --collection software --name "Reliability Review" \
  --description "Review reliability risks." --trigger "review reliability" \
  --not-for "design a new service" --min-scope multi_step --full-diff
go run ./cmd/skillhub skill create --workspace /path/to/workspace \
  --id reliability-review --collection software --name "Reliability Review" \
  --description "Review reliability risks." --trigger "review reliability" \
  --not-for "design a new service" --min-scope multi_step --yes

# Preview/confirm lifecycle transitions and read an active skill
go run ./cmd/skillhub skill activate reliability-review --workspace /path/to/workspace
go run ./cmd/skillhub skill activate reliability-review --workspace /path/to/workspace --yes
go run ./cmd/skillhub skill show reliability-review --workspace /path/to/workspace

# Edit through a temporary $VISUAL/$EDITOR file, then validate and publish via the same mutation service
go run ./cmd/skillhub skill edit reliability-review --workspace /path/to/workspace --editor --yes

# Capture without network access, then inspect intake
go run ./cmd/skillhub source capture https://github.com/example/repo.git \
  --reason "Potential reliability source" --workspace /path/to/workspace
go run ./cmd/skillhub source list --workspace /path/to/workspace

# Onboarding performs explicit source inspection and returns exact confirmation pins
go run ./cmd/skillhub source triage SRCQ-ID --decision accept --source-id example-repo \
  --adapter git --workspace /path/to/workspace
go run ./cmd/skillhub source confirm --workspace /path/to/workspace \
  --proposal PROP-ID --proposal-digest sha256:DIGEST --base-version sha256:BASE

# Source checks are the explicit network boundary; status never fetches
go run ./cmd/skillhub check --all-due --workspace /path/to/workspace
go run ./cmd/skillhub check --all --workspace /path/to/workspace

# After an unmanaged canonical-file edit, validate first; rebuild publishes only valid bytes
go run ./cmd/skillhub validate --workspace /path/to/workspace
go run ./cmd/skillhub rebuild --workspace /path/to/workspace

# Validate the Go foundation
go test ./...
go test -race ./...
go vet ./...
```

MCP transport, distillation, semantic skill resolution, and production installation or publishing are not implemented yet.
