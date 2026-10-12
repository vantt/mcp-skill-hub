# Skill Hub release runbook

This document details the release process, supported platforms, verification procedures, and recovery paths for Skill Hub releases.

## Supported platforms

Skill Hub compiles without CGO and ships pre-built archives for six target platforms:

| OS | Architecture | Archive format |
|---|---|---|
| Linux | `amd64` | `skillhub-<version>-linux-amd64.tar.gz` |
| Linux | `arm64` | `skillhub-<version>-linux-arm64.tar.gz` |
| macOS (Darwin) | `amd64` | `skillhub-<version>-darwin-amd64.tar.gz` |
| macOS (Darwin) | `arm64` | `skillhub-<version>-darwin-arm64.tar.gz` |
| Windows | `amd64` | `skillhub-<version>-windows-amd64.zip` |
| Windows | `arm64` | `skillhub-<version>-windows-arm64.zip` |

Additionally, releases publish `install.sh`, `install.ps1`, `checksums.txt`, SPDX JSON SBOMs, and Sigstore signature bundles.

## Release preflight and gate

Before tagging a release, run local tests and the release preflight script. [Contributing](../CONTRIBUTING.md#build-and-test) describes the everyday `make` gates; the release gate below also runs the installer lifecycle test.

```bash
# 1. Run local test gates
go test ./...
go test -race ./...
go vet ./...
test -z "$(gofmt -l cmd internal schemas)"
bash -n scripts/install.sh scripts/test-installer-lifecycle.sh
scripts/test-installer-lifecycle.sh

# 2. Run release preflight check
scripts/release-preflight.sh v0.1.0
```

`scripts/release-preflight.sh` verifies that:
1. The working tree is clean.
2. The current branch is `main`.
3. The specified tag does not already exist locally or on remote.
4. GitHub CI status is green for the HEAD commit.

To tag and push in one command, run:
```bash
scripts/release-preflight.sh --push v0.1.0
```

If the web UI changed since the last release, rerun `make web-shots` before tagging and commit the refreshed images in `docs/images/web/`. They are captured from an invented demo hub, so the README and the user guide show the current screens.

### CI, the Go toolchain and prereleases

- **Pin the Go patch version.** `ci.yml` and `release.yml` both build and test with an exact Go patch version (`go-version: "1.26.9"`), and every job must use the same one. Go 1.26.0 on macOS crashes a forked child before it can run `git`, which showed up as random `git init: signal: segmentation fault` failures in the macOS jobs. It looked like a runner flake and was not: the same suite passes on 1.26.9. Release binaries are built by `release.yml`, so a stale pin there ships a toolchain bug; check both files when you raise the version. The `go` line in `go.mod` and `web/go.mod` is also `1.26.9`, so a source build with an older Go downloads that toolchain instead of using the crashing one; raise all of them together.
- **Tell macOS users of the older releases to update.** The crash above comes from the Go 1.26.0 toolchain that built v0.2.0 through v0.4.0, and the product runs `git` as a child process (`init` and the history commands). Users of those releases on macOS who see `git init: signal: segmentation fault` should update to v0.4.1 or later, which is built with Go 1.26.9. A crash in a released binary has not been reproduced; the link rests on the CI evidence above.
- **Find a failure that only one runner shows.** Add a temporary workflow on a throwaway branch that runs a matrix over the suspected variable (here the Go version) and, when it fails, prints the crash reports (`~/Library/Logs/DiagnosticReports` on macOS). Delete the branch afterwards.
- **A flaky test blocks the release.** The release workflow runs the whole test suite on Linux, macOS and Windows before it builds anything, so a flaky test stops the tag with nothing published. If the failure is a known flake, rerun only the failed job with `gh run rerun <run-id> --failed`. If the code or a test must change, no release exists yet: fix it, wait for green CI, and use the next prerelease number (for example `v0.5.0-alpha.2`) or delete the unpublished tag and tag again.
- **Prereleases.** A tag such as `v0.5.0-alpha.1` is published as a GitHub prerelease and is never "latest", so the installers behind `releases/latest/download/` do not offer it. Install it with the installer attached to that tag and an explicit version, for example `curl -fsSL https://github.com/vantt/mcp-skill-hub/releases/download/v0.5.0-alpha.1/install.sh | sh -s -- --version 0.5.0-alpha.1`. On Windows set `$env:SKILLHUB_VERSION = '0.5.0-alpha.1'` first and run the `install.ps1` from the same tag.

### Network MCP listener and remote clients

`skillhub serve` is a network listener that serves the three runtime tools to agents on other machines, and `skillhub mcp proxy` is the client side that `skillhub connect --hub` registers (see the [remote hub guide](remote-hub-guide.md)). Before tagging a release that touches the MCP server, the proxy, the skill cache, `connect` or `doctor`:

- Run `make remote-bench`. It times stdio, loopback HTTP and a simulated 100 ms link and fails when a budget in [docs/design/remote-hub.md](design/remote-hub.md#final-numbers) is missed. The safety tests (curation tools unreachable, the Host and Origin checks, a token on every request, the token in no output) run in `go test ./...`.
- Keep remote clients and hubs of neighbouring releases compatible. Tool schemas only grow. Raise `app.RemoteAPILevel` only when a client of one level cannot work with a hub of another; `doctor` then tells users which side to update. Say in the release notes when both machines must be updated together.
- The hub token is a per-workspace secret in `<workspace>/runtime/config/.hub-token` (mode 0600, ignored by Git). It is never part of a release artifact, and an upgrade keeps it, so connected machines keep working.
- The listener speaks plain HTTP by design and binds `0.0.0.0` by default (a user decision). It must keep requiring the token on every request, print the plain-HTTP warning on every start of `serve` and `serve web`, and keep the Origin check strict.

## Automated release pipeline

Pushing a `v*.*.*` tag initiates the automated GitHub Actions release workflow (`.github/workflows/release.yml`):

1. **Prepare:** Validates the SemVer tag and determines release vs. prerelease classification.
2. **Verify:** Runs test suites, lifecycle installer tests (Unix and Windows), fuzzing, and static checks on Linux, macOS, and Windows runners.
3. **Web UI Build:** Runs `make web-build`, generates third-party software notices (`npm run notices`), and uploads the `web-dist` artifact containing compiled embedded web assets and `THIRD_PARTY_NOTICES.md`.
4. **Build & Package:** Downloads `web-dist` into `internal/delivery/web/dist` for Go asset embedding, cross-compiles CGO-free binaries for all 6 target platforms, attaches SPDX SBOMs via Syft, packages tar.gz/zip archives, and computes platform artifacts.
5. **Sign & Attest:**
   - Copies standalone installers (`install.sh`, `install.ps1`) and `THIRD_PARTY_NOTICES.md` from `web-dist` into `release/`.
   - Computes deterministic `checksums.txt` over all release assets.
   - Signs `checksums.txt` and all platform archives using keyless Sigstore cosign with GitHub OIDC tokens.
   - Generates GitHub Build Provenance and SBOM attestations using GitHub Artifact Attestations (`actions/attest-build-provenance`).
6. **Publish:** Creates GitHub Release uploading all release assets in `release/*` (including binaries, installers, checksums, signatures, SBOMs, and `THIRD_PARTY_NOTICES.md`).
7. **Post-publish smoke test:** Executes on three runners (`ubuntu-latest`, `macos-latest`, `windows-latest`) testing:
   - Literal one-liner installation and automatic PATH configuration.
   - Upgrade flow from previous stable release (if available).
   - **Web UI release smoke:** Starts `skillhub serve web --no-open --addr 127.0.0.1:0` in the background on a freshly initialized hub, extracts the URL and token, verifies authenticated `GET /api/v1/session` (200), unauthenticated request refusal (401), and HTML root element rendering (`id="root"`), then terminates the server.
   - Complete uninstallation via `SKILLHUB_UNINSTALL=1` and verification of binary removal.

Prerelease tags (such as `v0.1.0-rc.1`) are published as GitHub prereleases and do not update the `latest` release pointer.

## Signature policy and verification

### Installer policy

- **SHA-256 integrity:** Always required and strictly enforced across all platforms.
- **Sigstore verification:** By default, installers check if `cosign` is present on the system:
  - If `cosign` is found, the installer verifies the signature against the repository's release workflow identity. If verification fails, installation aborts.
  - If `cosign` is absent, the installer proceeds after verifying SHA-256 checksums and prints a notice.
- **Strict mode:** Set `SKILLHUB_REQUIRE_SIGNATURE=1` to mandate signature verification. If `cosign` is missing or verification fails, the installer aborts immediately.

### Manual artifact verification

Download release assets into an empty folder and verify:

```bash
export OWNER=vantt
export REPO=mcp-skill-hub
export VERSION=0.1.0

# 1. Verify checksums file with Cosign
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity "https://github.com/${OWNER}/${REPO}/.github/workflows/release.yml@refs/tags/v${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# 2. Verify archive checksum
sha256sum --check --strict checksums.txt

# 3. Verify archive Sigstore bundle
cosign verify-blob \
  --bundle skillhub-${VERSION}-linux-amd64.tar.gz.sigstore.json \
  --certificate-identity "https://github.com/${OWNER}/${REPO}/.github/workflows/release.yml@refs/tags/v${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  skillhub-${VERSION}-linux-amd64.tar.gz

# 4. Verify GitHub build provenance attestation
gh attestation verify skillhub-${VERSION}-linux-amd64.tar.gz --repo ${OWNER}/${REPO}
```

## Install, upgrade, and uninstall

User install, upgrade and uninstall steps are in [Installation](installation.md).

The uninstaller removes only what the installer manages: the binary, the install marker and the `PATH` lines the installer added. It never deletes user workspaces, skills or host connection files.

## Canonical schema migration

A newer binary never modifies canonical schema files automatically during `status`, `doctor`, `maintain rebuild`, or startup. For workspaces created with older schema revisions:

```bash
cd /path/to/hub && skillhub doctor
cd /path/to/hub && skillhub maintain migrate               # read-only diff
cd /path/to/hub && skillhub maintain migrate --to 1 --yes  # apply migration
```

## Disaster recovery: clone, rebuild, and serve

Canonical Git files are the portability boundary. The runtime catalog, operational state, caches, and telemetry are disposable and can be rebuilt on any machine:

```bash
# 1. Fetch the hub into a new, empty folder (builds the search index)
mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init <remote-url>

# 2. Check for pending transaction recovery
cd ~/skill-hub && skillhub doctor

# 3. If recovery is needed:
cd ~/skill-hub && skillhub doctor --fix

# 4. Validate canonical truth, rebuild search index offline, and re-check health
cd ~/skill-hub && skillhub maintain validate
cd ~/skill-hub && skillhub maintain rebuild
cd ~/skill-hub && skillhub status

# 5. Connect a project; its agents then start the hub over stdio
cd <your-project> && skillhub connect
```

## Rollback

- **Failed in-place update:** `skillhub update` and `install.sh` automatically restore the previous working binary if verification or activation fails.
- **Faulty published release:** Stop rollout, mark the GitHub release as draft or delete the broken release according to repository policy, and publish a new patch version. Never retarget or overwrite an existing Git tag.
- **Local binary rollback:** Reinstall a known good version by setting `SKILLHUB_VERSION=v0.0.X` when running the installer script.
- **Interrupted canonical migration:** Run `skillhub doctor`, inspect any pending recovery journal, and run `skillhub doctor --fix`.
- **Corrupted derived state:** Stop any running `skillhub` processes for that workspace, delete only the disposable `runtime/` folder (never `.skillhub/transactions/` or canonical files), and run `skillhub maintain rebuild`.

## Support and compatibility

Diagnose issues using:
- `skillhub version --json`
- `skillhub status --json`
- `skillhub doctor --json`

See `docs/mcp-compatibility-matrix.json` for verified agent clients and versions.
