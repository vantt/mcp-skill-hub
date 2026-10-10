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

Before tagging a release, run local tests and the release preflight script:

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
   - **Web UI release smoke:** Starts `skillhub serve web --loopback-only --no-open --addr 127.0.0.1:0` in the background on a freshly initialized workspace, extracts the URL and token, verifies authenticated `GET /api/v1/session` (200), unauthenticated request refusal (401), and HTML root element rendering (`id="root"`), then terminates the server.
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

### Standard one-liners

Linux / macOS:
```bash
curl -fsSL https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh | sh
```

Windows (PowerShell):
```powershell
powershell -ExecutionPolicy ByPass -c "irm https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.ps1 | iex"
```

### Options and environment variables

- `SKILLHUB_VERSION=v0.1.0`: Pin to a specific version.
- `SKILLHUB_PREFIX=$HOME/.local`: Custom installation prefix (Unix).
- `SKILLHUB_NO_MODIFY_PATH=1`: Do not alter shell profile or registry PATH.
- `SKILLHUB_REQUIRE_SIGNATURE=1`: Require valid Sigstore cosign verification.

### Upgrades

- **In-place:** Run `skillhub update` inside a terminal. It queries GitHub Releases for newer stable versions, verifies checksums and signatures, and atomically replaces the binary.
- **Re-running installer:** Simply re-execute the installation one-liner. Upgrades retain the prior binary as a backup until activation succeeds.

### Uninstall

- **Unix:**
  ```bash
  curl -fsSL https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh | sh -s -- --uninstall
  # or pass environment variable:
  SKILLHUB_UNINSTALL=1 sh install.sh
  ```
- **Windows:**
  ```powershell
  powershell -ExecutionPolicy ByPass -c "$env:SKILLHUB_UNINSTALL='1'; irm https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.ps1 | iex"
  ```

Uninstall removes only the managed binary and install metadata marker. User workspaces, skills, and host connection files are never deleted by the uninstaller.

## Canonical schema migration

A newer binary never modifies canonical schema files automatically during `status`, `doctor`, `rebuild`, or startup. For workspaces created with older schema revisions:

```bash
skillhub doctor --workspace /path/to/workspace
skillhub migrate --workspace /path/to/workspace             # read-only diff
skillhub migrate --workspace /path/to/workspace --to 1 --yes  # apply migration
```

## Disaster recovery: clone, rebuild, and serve

Canonical Git files are the portability boundary. The runtime catalog, operational state, caches, and telemetry are disposable and can be rebuilt on any machine:

```bash
# 1. Clone the canonical workspace into a new path
git clone <remote-url> ~/skillhub

# 2. Check for pending transaction recovery
skillhub doctor --workspace ~/skillhub

# 3. If recovery is needed:
skillhub doctor --fix --workspace ~/skillhub --yes

# 4. Validate canonical truth, rebuild search index offline, and re-check health
skillhub validate --workspace ~/skillhub
skillhub rebuild --workspace ~/skillhub
skillhub status --workspace ~/skillhub

# 5. Connect project or serve over stdio
skillhub mcp serve --workspace ~/skillhub
```

## Rollback

- **Failed in-place update:** `skillhub update` and `install.sh` automatically restore the previous working binary if verification or activation fails.
- **Faulty published release:** Stop rollout, mark the GitHub release as draft or delete the broken release according to repository policy, and publish a new patch version. Never retarget or overwrite an existing Git tag.
- **Local binary rollback:** Reinstall a known good version by setting `SKILLHUB_VERSION=v0.0.X` when running the installer script.
- **Interrupted canonical migration:** Run `skillhub doctor`, inspect any pending recovery journal, and run `skillhub doctor --fix --yes`.
- **Corrupted derived state:** Stop any running `skillhub` processes for that workspace, delete only the disposable `runtime/` folder (never `.skillhub/transactions/` or canonical files), and run `skillhub rebuild`.

## Support and compatibility

Diagnose issues using:
- `skillhub version --json`
- `skillhub status --json`
- `skillhub doctor --json`

See `docs/mcp-compatibility-matrix.json` for verified agent clients and versions.
