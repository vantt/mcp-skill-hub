# Release and install pipeline for skillhub: audit, survey, and recommendation

Date: 2026-09-30. Scope: research only. No repo file was changed except this report.

## Bottom line

The release CI exists and is strong on supply-chain evidence, but nobody can install skillhub with one command today. Six problems block that:

1. No release has ever been published. `git ls-remote --tags origin` is empty, and `releases/latest/download/install.sh` returns HTTP 404. The repo is public.
2. The README one-liner fails even after a release exists, because `install.sh` requires `--version` and the README omits it.
3. The installer will not run without `cosign`, which most users do not have.
4. There is no Windows installer.
5. Upgrading or uninstalling requires a git clone.
6. Only linux/amd64 and darwin/arm64 are built.

Recommendation: keep the hand-written workflow and make small, additive changes. Publish `install.sh` and a new `install.ps1` as release assets with the version baked in. Verify SHA-256 by default and check the cosign signature only when cosign is already installed. Make a re-run of the installer act as an upgrade, and add an uninstall flag. Add the four missing cheap targets. Test the literal one-liners on all three OSes after each publish. Leave GoReleaser, package managers, and `self-update` until later.

---

## Part A: audit of the current state

### What exists

| Item | Finding |
|---|---|
| Release CI | Yes. `.github/workflows/release.yml` (609 lines, hand-written, no GoReleaser). |
| Trigger | A tag push matching `v*.*.*` publishes (`release.yml:4-6`). A `workflow_dispatch` run on main builds a private, non-installable draft artifact (`release.yml:7-12, 324-368`). |
| Gate | The full test suite, race detector, vet, fuzzing and installer lifecycle run on ubuntu, macOS and Windows before build (`release.yml:79-192`). |
| Build | Built on ubuntu with CGO disabled, `-trimpath`, ldflags version stamping, deterministic tar/zip, and a Syft SPDX SBOM per target (`release.yml:194-322`). |
| Targets | `linux/amd64` tar.gz, `darwin/arm64` tar.gz and `windows/amd64` zip only (`release.yml:205-217`). |
| Asset names | `skillhub-<VERSION>-<os>-<arch>.tar.gz`. The version has no `v` prefix (`release.yml:279`). |
| Checksums | `checksums.txt`, which covers the archives, SBOMs and `install.sh` (`release.yml:402-411`). |
| Signatures | Keyless cosign `sign-blob --bundle` on every asset, plus SBOM attestations (`release.yml:413-448`). |
| Provenance | `actions/attest-build-provenance` on the archives, then `gh attestation verify` (`release.yml:494-516`). |
| Publish | `gh release create --verify-tag --generate-notes`, marked as a prerelease when the SemVer has a suffix (`release.yml:595-609`). |
| install.sh hosting | A release asset copied from `scripts/install.sh` (`release.yml:396-400`). The README points to `releases/latest/download/install.sh` (`README.md:69`). |
| Installer platforms | Only `linux-amd64` and `darwin-arm64`. It rejects Windows explicitly (`install.sh:49-54`, `installer-common.sh:38-41`). |
| Verification in installer | The installer downloads `checksums.txt` and its sigstore bundle, runs mandatory `cosign verify-blob` against the exact tag workflow identity, then checks the archive SHA-256 (`install.sh:230-282`). |
| Install dir | `$HOME/.local/bin/skillhub` plus a `.skillhub-managed` marker. `--prefix` and `SKILLHUB_INSTALL_DIR` override it (`install.sh:6, 39-47`). |
| PATH | Not handled. There is no notice and no profile edit. The README tells users to `export PATH` manually (`README.md:16`). |
| Upgrade | `scripts/upgrade.sh`, which sources `scripts/lib/installer-common.sh` and so needs a clone (`upgrade.sh:14-16`). It has good rollback (`upgrade.sh:82-111`). |
| Uninstall | `scripts/uninstall.sh`, which also needs the clone (`uninstall.sh:11-13`). It is safe: it removes only the managed binary and marker, and purges config only with `--purge-config --yes`. |
| Version pinning | `--version` or `SKILLHUB_VERSION` is mandatory. There is no "latest" default (`install.sh:75`). |
| Installer tests | `scripts/test-installer-lifecycle.sh` uses a fake curl and a fake cosign and runs on ubuntu and macOS only (`ci.yml:50-64`, `release.yml:174-180`). |
| GoReleaser | Not used. There is no `.goreleaser*` file. |
| Evaluation workflow | Runs on a nightly cron and by hand. It is unrelated to releases (`evaluation.yml:3-10`). |

### Gaps and risks

| # | Sev | Gap | Evidence |
|---|---|---|---|
| A1 | P0 | The README one-liner fails with "A release version is required". It runs `install.sh` without `--version`. | `README.md:69` vs `install.sh:75` |
| A2 | P0 | `cosign` is a hard dependency. Without it, the install aborts. | `install.sh:169`, `installer-common.sh:158`, `README.md:72` |
| A3 | P0 | There is no Windows installer, even though a Windows zip is built. The installer tests skip Windows. | `install.sh:52`, `release.yml:175`, `ci.yml:56` |
| A4 | P0 | Upgrade and uninstall scripts exist only in the repo and source `scripts/lib`, so the "no clone" goal is broken after the first install. | `upgrade.sh:14-16`, `uninstall.sh:11-13`, `docs/release-runbook.md:75-81` |
| A5 | P0 | Re-running the installer does not upgrade. It refuses because the target already exists. | `install.sh:360-361` |
| A6 | P0 | No tag or release exists. The pipeline has never run end to end in production. The first real run will surface issues nobody has seen. | `git ls-remote --tags` is empty; `gh release list` is empty; the URL returns 404 |
| A7 | P1 | Missing cheap targets: linux/arm64 (Graviton, Raspberry Pi, ARM containers), darwin/amd64 (Intel Macs) and windows/arm64. The binary is pure Go, so each is a single matrix row. | `release.yml:205-217`, `ci.yml:97-101` |
| A8 | P1 | Mandatory cosign inside a piped `install.sh` adds little protection. An attacker who can replace release assets can also replace `install.sh` and delete the check. The signature only protects the "verify install.sh first" route. | `release.yml:396-441`, runbook `59-66` |
| A9 | P1 | There is no PATH notice. On macOS, `~/.local/bin` is not on PATH by default, so users get "command not found". | `install.sh:382` prints only the install path |
| A10 | P1 | `releases/latest/download/*` ignores prereleases. If the first tag is `v1.0.0-rc.1`, the README URL keeps returning 404. | GitHub REST semantics; `release.yml:65-68` |
| A11 | P1 | The docs contradict each other. The runbook says the one-liner is "tested end to end" with `--version`; the README omits it. `README.md:66` says only "Linux amd64 and macOS arm64". | `docs/release-runbook.md:68-73`, `README.md:64-72` |
| A12 | P1 | There is no post-publish smoke test of the literal user command on real runners. The lifecycle test uses fake curl and fake cosign. | `scripts/test-installer-lifecycle.sh:174-260` |
| A13 | P2 | Verification logic is copied three times: in the sign job, and twice in the publish job. SemVer validation is copied twice. This is about 150 lines of maintenance weight. | `release.yml:51-63` vs `115-139`; `450-492` vs `547-593` |
| A14 | P2 | `check-latest: true` lets the Go patch version drift between the verify and build jobs, which weakens the reproducibility claim. | `release.yml:145, 229` |
| A15 | P2 | There is no in-binary update path (`skillhub self-update`). | `internal/delivery/cli/root.go` has no update command |
| A16 | P2 | Binaries are not code-signed or notarized. This is fine for curl and irm downloads, because neither adds a quarantine or Mark-of-the-Web flag. It becomes a problem only for browser downloads, Homebrew casks, or Defender heuristics. | n/a |

What is good and worth keeping: the deterministic archives, pinned action SHAs, `permissions: {}` by default, tag-identity-scoped signing, archive-member validation (`install.sh:283-298`), atomic rename, the managed marker, upgrade rollback, and the uninstall that never touches workspaces.

---

## Part B: survey of well-known installers and pipelines

### External projects

| Project | Unix one-liner | Windows one-liner | Script hosting | Verification in installer | Install dir / PATH | Upgrade / uninstall | CI tooling / package managers |
|---|---|---|---|---|---|---|---|
| uv / ruff (Astral) | `curl -LsSf https://astral.sh/uv/install.sh \| sh` | `powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 \| iex"` | Custom domain proxying a cargo-dist release asset. Pin in the URL: `astral.sh/uv/0.x.y/install.sh`. | SHA-256 values are embedded in the installer per release (I confirmed about 30 `_checksum_value` lines). No cosign. | `~/.local/bin`. It edits the shell profile or the Windows HKCU `Environment\Path`. `UV_NO_MODIFY_PATH=1` opts out. | `uv self update`. Uninstall means removing the binaries by hand. | cargo-dist. brew, winget, and others. |
| bun | `curl -fsSL https://bun.com/install \| bash` | `powershell -c "irm bun.sh/install.ps1\|iex"` | Custom domain | None seen in the script. It downloads `releases/latest/download/bun-$target.zip`. | `~/.bun/bin`. It appends to bash, zsh and fish rc files. | `bun upgrade`. Uninstall is `rm -rf ~/.bun` or `uninstall.ps1`. | Custom workflow. brew, scoop, npm. |
| deno | `curl -fsSL https://deno.land/install.sh \| sh` | `irm https://deno.land/install.ps1 \| iex` | Custom domain. The version comes from `dl.deno.land/release-latest.txt`. | None | `~/.deno/bin`. It prompts interactively before editing rc files, and only on a TTY. | `deno upgrade [--version]` | Custom workflow. |
| rustup | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` | Download `rustup-init.exe` | Custom domain | TLS only for the script | `~/.cargo/bin`. It writes `~/.cargo/env` and sources it from the profile. `--no-modify-path` opts out. | `rustup self update` and `rustup self uninstall` | Custom workflow. |
| mise | `curl https://mise.run \| sh` | winget, scoop, choco | Custom domain | SHA-256 embedded per target. There is also an optional GPG-signed `install.sh.sig` for the review-first route. | `~/.local/bin`. PATH comes from `mise activate`. | `mise self-update`, `mise implode` | Custom workflow. winget, scoop, choco, brew. |
| chezmoi | `sh -c "$(curl -fsLS get.chezmoi.io)"` | `iex "&{$(irm 'https://get.chezmoi.io/ps1')}"` | Custom domain, with the script kept in the repo (`assets/scripts/install.sh`, `install.ps1`) | SHA-256 against `checksums.txt`. It does not run cosign. Cosign-signed checksums are published for users who want to verify by hand. | `./bin` by default, `-b DIR` to override. It does not touch PATH. | `chezmoi upgrade` | GoReleaser v2: scoop, winget, choco, nfpm, sboms, cosign sign. |
| starship | `curl -sS https://starship.rs/install.sh \| sh` | winget, scoop | Custom domain | None | `/usr/local/bin`, using sudo only if needed. It prints instructions and does not edit profiles. | Re-run the script | Custom workflow. |
| zoxide | `curl -sSfL .../raw.githubusercontent.com/.../install.sh \| sh` | winget, scoop | Raw GitHub | None. It resolves "latest" through the GitHub API. | `~/.local/bin`. It warns if the directory is not on PATH. | Re-run the script | cargo. |
| fnm | `curl -fsSL https://fnm.vercel.app/install \| bash` | winget, scoop | Custom domain | None. It uses `releases/latest/download`. | `~/.local/share/fnm`. It edits rc files; `--skip-shell` opts out. | Re-run the script | Custom workflow. |
| atuin | `curl --proto '=https' --tlsv1.2 -LsSf https://setup.atuin.sh \| sh` | n/a | Custom domain | cargo-dist style | cargo-dist style | `atuin update` via the receipt | cargo-dist. |
| ollama | `curl -fsSL https://ollama.com/install.sh \| sh` | `irm https://ollama.com/install.ps1 \| iex` | The custom domain returns a 307 redirect to `github.com/ollama/ollama/releases/latest/download/install.sh`, a release asset. | TLS | `/usr/local` plus a systemd unit. `OLLAMA_VERSION` pins. | Re-run the script | Custom workflow. |
| gh (GitHub CLI) | No curl-to-sh route. Package managers only. | winget, scoop, MSI | n/a | `checksums.txt` plus build-provenance attestations (`gh attestation verify`) | Handled by the package manager | Handled by the package manager | GoReleaser plus nfpm and MSI. |
| lazygit, k9s | No script. Package managers only. | scoop, winget | n/a | `checksums.txt`. k9s also publishes SBOMs. | Handled by the package manager | Handled by the package manager | GoReleaser. |
| GoReleaser | n/a | n/a | n/a | Signs the checksum file with cosign `sign-blob --bundle`. It signs nothing by default. Attestations are done with a separate `actions/attest` step. | n/a | n/a | OSS features: `homebrew_casks` (the old `brews` formula publisher is deprecated), `scoops`, `winget` (needs a fork of `winget-pkgs` plus a PAT), `nfpms`, `sboms`. |

Patterns across the survey:

- **Verification.** Not one mainstream curl-to-sh installer makes the user install cosign. The common default is SHA-256 over TLS: uv, mise, chezmoi, herdr and forgentX all do this. Several projects (bun, deno, starship, zoxide, fnm, ollama) skip checksums entirely. Signatures and attestations are offered as a manual, optional step (chezmoi, gh, mise's GPG route).
- **Hosting.** Large projects use a short custom domain. Smaller ones use raw GitHub or a release asset. ollama combines both: a short domain redirects to the release asset, which keeps the script versioned with the binaries.
- **Pinning.** uv puts the version in the URL. Most others use an environment variable or flag with "latest" as the default. None requires a version.
- **PATH.** The field splits into two camps. Projects that edit profiles by default (uv, bun, rustup, fnm) all offer an opt-out. Projects that only print instructions (chezmoi, starship, zoxide, herdr, forgentX) avoid rc-file side effects and uninstall residue. On Windows, the projects with installers write the user PATH through the HKCU registry (uv, herdr).
- **Upgrade.** Almost every project with its own installer has an in-binary `self update` command (uv, bun, deno, rustup, mise, chezmoi). Re-running the installer is the universal fallback.
- **Pipelines.** Go CLIs overwhelmingly use GoReleaser (gh, lazygit, k9s, chezmoi). Rust CLIs use cargo-dist or custom workflows.

### The user's own projects (primary references)

| | herdr-gateway (`herdr-go`) | herdr (herdrdev upstream, plus the `vantt/` fork) | forgentX (`fgctl`) | skillhub today |
|---|---|---|---|---|
| Unix one-liner | `curl -fSL https://raw.githubusercontent.com/vantt/herdr-go/main/install.sh \| bash` | Upstream: `curl -fsSL https://herdr.dev/install.sh \| sh`. Fork: raw `vantt/install.sh`. | `curl -fsSL https://raw.githubusercontent.com/vantt/forgent/main/install.sh \| sh` | `releases/latest/download/install.sh \| sh`, which is broken (A1) |
| Windows | `irm https://raw.githubusercontent.com/vantt/herdr-go/main/install.ps1 \| iex` | `powershell -ExecutionPolicy Bypass -c "irm https://herdr.dev/install.ps1 \| iex"`, plus `install.cmd` for cmd.exe | None (Linux x64 only) | None |
| Script hosting | Raw main. `install.sh` is also packed inside the tarball. | A custom domain that serves `distribution/` | Raw main and a release asset | Release asset only |
| Latest resolution | `releases/latest/download/<unversioned asset>` | `herdr.dev/latest.json`, a manifest with version, URLs and SHA-256 per target. `herdr update` reads the same manifest. | A redirect of `/releases/latest` via curl's `%{url_effective}`, which avoids the API rate limit | None (`--version` is required) |
| Verification | None in the installers. `herdr-go update` verifies `checksums.txt` (`src/update/checksum.rs`). | SHA-256 from the manifest, in both sh and ps1 | SHA-256 against `SHA256SUMS`, plus rejection of symlinked archive members | Mandatory cosign plus SHA-256 |
| Install dir / PATH | `~/.local/bin`. Windows uses `%LOCALAPPDATA%\herdr-go\bin`. No PATH edit. | `~/.local/bin` with a printed `export PATH` hint. On Windows it rewrites the user PATH through the registry (`Prepend-PathEntry`). | `~/.local/bin` with a printed PATH notice and a next step (`fgctl init`) | `~/.local/bin`, no notice |
| Upgrade / uninstall | Re-run the installer to overwrite, or run `herdr-go update`. The same script takes `--uninstall` / `-Uninstall`. | `herdr update`. The fork adds a `.bak-*` backup plus `--rollback`. | Re-run with an atomic `mv` | Separate scripts that need a clone |
| Release trigger | Tag `v*`, cut by `scripts/release.sh`, which checks main, a clean tree and version lockstep before tagging | Tag `v*` with admin gates. After release, a job commits the updated `latest.json`. | Tag `v*`, a single job. The tag version must match `package.json`. | Tag `v*.*.*`, or a draft by manual dispatch |
| Installer CI tests | Post-publish smoke of the real `install.sh` on macOS and `install.ps1` on Windows. An `update-smoke` job installs the previous tag and then updates to the new one. | Python unit tests with a fake `uname` (`scripts/test_unix_installer.py`), a Windows PowerShell 5.1 installer test in `ci.yml`, and a Windows ARM64 installer workflow | `scripts/ci-external-consumer.sh` runs `install.sh` against the built assets before publishing | Fixture lifecycle tests with fake curl and cosign; Unix only |

Lessons from the user's projects:

- **Keep:** the same-script `--uninstall` flag, a PowerShell 5.1-compatible `install.ps1`, the post-publish literal smoke test, the upgrade-from-previous-tag smoke test, forgentX's redirect-based latest resolution, forgentX's pre-publish external-consumer test, and the `release.sh` preflight.
- **Do not copy:** herdr-gateway's unverified installer download, and raw-main hosting. With raw main, the script on `main` can drift ahead of the published assets. herdr-gateway's own `update-smoke` comment records exactly this breakage.
- skillhub already exceeds all three projects on verification. The job is to remove friction, not to add evidence.

---

## Part C: recommendation for skillhub

### Ranked options

**Pipeline**

| Rank | Option | Ease for user | Simplicity / dependencies | Maintenance | Risk |
|---|---|---|---|---|---|
| 1 | **Keep the hand-written `release.yml` and make additive fixes** | Same | No new tool; the team already knows it | Medium. Trim duplication later (A13). | Low, and already written. It does need a first real run (A6). |
| 2 | Switch to GoReleaser v2 | Same | One more tool. Replaces about 400 lines with about 60 of YAML. | Low | Medium. It would mean re-validating deterministic archives, SBOMs and the identity-scoped signatures, which is churn before a first release. |
| 3 | cargo-dist style generated installers | n/a | Rust-centric | n/a | Poor fit for Go. |

Adopt GoReleaser only when you want Homebrew, Scoop or winget. That is its main advantage over the current workflow.

**Signature verification in the installer**

| Rank | Option | Rationale |
|---|---|---|
| 1 | **SHA-256 always. Verify the cosign bundle only if `cosign` is already on PATH, and fail closed if that check fails. Add `SKILLHUB_REQUIRE_SIGNATURE=1` to make it mandatory.** | Zero extra dependencies for users. Strict users keep full protection. This matches chezmoi, gh and mise. It is also honest about A8: piped scripts cannot protect themselves. |
| 2 | Keep cosign mandatory | Strongest in theory, but blocks most users. None of the surveyed projects do this. |
| 3 | Use `gh attestation verify` inside the installer | It needs `gh` to be installed and logged in, so it is worse than cosign on the dependency axis. Keep it as a documented manual step. |

**Script hosting**

| Rank | Option | Rationale |
|---|---|---|
| 1 | **Release asset, with the version baked in at release time** (`https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh`) | The script always matches its assets, is immutable per tag, and is covered by `checksums.txt` and a signature. Pinning works through the URL (`/releases/download/vX.Y.Z/install.sh`), as uv and ollama do. No second host is needed. The README URL already points here. |
| 2 | Raw `main` (the user's pattern) | Shorter to reason about, but the script can drift ahead of the released assets. |
| 3 | A custom short domain that redirects to #1 (the ollama pattern) | Nicest URL. Needs a domain and DNS. Can be added later without changing #1. |

### Target design (one command per OS)

```sh
# Linux / macOS
curl -fsSL https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh | sh
# pin:       .../releases/download/v1.2.3/install.sh | sh
# uninstall: curl -fsSL .../install.sh | sh -s -- --uninstall
```

```powershell
# Windows (PowerShell 5.1+)
powershell -ExecutionPolicy ByPass -c "irm https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.ps1 | iex"
# pin: $env:SKILLHUB_VERSION='1.2.3'; irm .../install.ps1 | iex
# uninstall: $env:SKILLHUB_UNINSTALL='1'; irm .../install.ps1 | iex
```

Behaviour of both scripts:

- **Version.** The release job replaces an `@SKILLHUB_VERSION@` placeholder when it copies the script (`release.yml:400`). `--version` or `SKILLHUB_VERSION` still overrides it. An unrendered script, such as one run from a checkout, keeps today's "version required" error.
- **Platform.** The installer detects OS and architecture and supports linux, darwin and windows on both amd64 and arm64.
- **Verification.** It downloads `checksums.txt` and checks the archive's SHA-256. If cosign is present, it also verifies the bundle and fails closed on a bad signature. If cosign is absent, it prints one line saying the signature was not checked and how to check it.
- **Install location.** Unix installs to `~/.local/bin/skillhub` (unchanged). Windows installs to `%LOCALAPPDATA%\skillhub\bin\skillhub.exe`, following the herdr-gateway layout.
- **PATH.** On Unix the script prints the exact `export PATH=...` line for the detected shell when the directory is missing, and does not edit profiles (herdr and forgentX do the same). On Windows it adds the directory to the user PATH through the HKCU registry and to the current session, then tells the user to open a new terminal (uv and herdr do the same). Windows has no shell-profile convention, so editing the registry is the only way to give users one-command ease there.
- **Re-run means upgrade.** For a managed install, the script keeps the upgrade logic from `upgrade.sh:82-111` (stage, back up, swap, version check, roll back). It still refuses to touch an unmanaged binary. On Windows, the running `.exe` is renamed to `.old` before the swap.
- **Uninstall.** `--uninstall` on Unix, or `SKILLHUB_UNINSTALL=1` on Windows, because `irm | iex` cannot pass parameters. It keeps today's `uninstall.sh` semantics: the managed binary and marker only, and config only with `--purge-config --yes`.
- **Tools needed.** Unix needs sh, curl, tar, and one of sha256sum, shasum or openssl. Windows needs nothing beyond built-in PowerShell (`Invoke-WebRequest`, `Get-FileHash`, `Expand-Archive`).

### Prioritized fix list

**P0: needed for the one-command goal**

| # | Fix | Why | Borrowed from |
|---|---|---|---|
| 1 | Bake a default version into the released `install.sh`, so the README command works without `--version` (A1). | This is the core user promise. | uv (versioned installer URL), herdr-gateway and forgentX ("latest" by default) |
| 2 | Make cosign optional: auto-verify when present, and `SKILLHUB_REQUIRE_SIGNATURE=1` to require it (A2, A8). Update `README.md:72`. | Removes the only unusual dependency. Needs the user's approval, because it reverses a documented fail-closed decision. | chezmoi, gh, mise, uv |
| 3 | Add `scripts/install.ps1` (PowerShell 5.1, SHA-256, user PATH through the registry, re-run upgrade, uninstall). Publish it as a release asset and include it in `checksums.txt` and signing (A3). | Windows users need a one-liner. The zip is already built. | uv install.ps1, herdr-gateway install.ps1, herdr distribution/install.ps1 |
| 4 | Fold the upgrade and uninstall logic into the standalone `install.sh`: re-run upgrades a managed install, and `--uninstall` removes it. Then delete `upgrade.sh`, `uninstall.sh` and `scripts/lib` (A4, A5). | No clone needed. One script is easier to maintain than four. | herdr-gateway `--uninstall`; uv and forgentX re-run |
| 5 | Add the linux/arm64, darwin/amd64 and windows/arm64 build rows and allow them in the installers (A7). | Each is one matrix row for pure Go. Without them, installs fail for many users. | herdr-gateway, all surveyed projects |
| 6 | Cut the first release as a stable tag (for example `v0.1.0`, not `-rc`) after a manual-dispatch draft passes. Document that `latest` ignores prereleases (A6, A10). | `releases/latest/download` returns 404 until a stable release exists. | GitHub REST semantics |

**P1: prevent regressions and friction**

| # | Fix | Why | Borrowed from |
|---|---|---|---|
| 7 | Print a PATH notice with the exact line for the user's shell, and a next step (`skillhub init`) (A9). | macOS users otherwise hit "command not found". | herdr, forgentX, zoxide |
| 8 | Add a post-publish job that runs the literal one-liners on ubuntu, macOS and Windows against the new tag, then runs `skillhub version`. Add an upgrade-from-previous-tag smoke test from the second release onward (A12). | Proves the real user path. The current test uses fake curl and cosign. | herdr-gateway `macos-install-smoke`, `windows-install-smoke`, `update-smoke` |
| 9 | Add a Windows installer fixture test to `ci.yml`, using the same local-fixture mode as the sh test (A3). | Keeps `install.ps1` from rotting between releases. | herdr `ci.yml` PowerShell 5.1 test, `windows-arm64.yml` |
| 10 | Rewrite `README.md:9-18, 64-72`, `docs/release-runbook.md:57-83` and the install reference in `docs/user-guide.md:223` around the two one-liners. Keep build-from-source as the alternative (A11). | The docs contradict each other today. | n/a |

**P2: later, if wanted**

| # | Fix | Why | Borrowed from |
|---|---|---|---|
| 11 | Add `skillhub self-update [--version]` using only the Go standard library: fetch `checksums.txt` and the archive, check SHA-256, swap atomically, rename-then-replace on Windows. Accept only installs that carry the managed marker. | Nearly every surveyed tool has one. The installer re-run covers the need meanwhile. | uv, bun, deno, mise, chezmoi, herdr-gateway `src/update` |
| 12 | Trim `release.yml`: verify once in the sign job, drop the repeated block in publish, share one SemVer check, and pin the exact Go version (A13, A14). | Cuts about 150 lines. The evidence stays the same. | n/a |
| 13 | Package managers: a Homebrew tap (cask) and a Scoop bucket, then winget. Do this with GoReleaser at that point. | Gives users native upgrades. Needs extra repos and tokens. | chezmoi, lazygit, k9s, gh |
| 14 | A short custom domain that redirects to the release asset. | Friendlier URL. | ollama, astral.sh, herdr.dev |
| 15 | A `scripts/release.sh` preflight: on main, clean tree, tag unused, CI green, then tag and push. | Prevents release accidents. | herdr-gateway `release.sh`, forgentX `prepare-release.mjs` |

### Adoption risk

- **Hand-written workflow.** No outside dependency to go stale. Its main risk is that it has never run for real; mitigate with the draft dispatch plus fix #8.
- **cosign and sigstore.** Mature and CNCF-backed. Making it optional removes the risk that users cannot install cosign.
- **GoReleaser.** Very mature, with frequent releases, but its v1 to v2 config break (2024) shows the churn risk. Defer it.
- **GitHub `releases/latest/download`.** Stable and widely used (bun, fnm, herdr-gateway, ollama). The one caveat is that it ignores prereleases.
- **Windows PATH through the registry.** Proven in uv and herdr. Be careful to keep `REG_EXPAND_SZ` entries unexpanded (uv reads the value with `DoNotExpandEnvironmentNames`).

## Open decisions for the user

1. **Mandatory cosign.** Recommended: make it optional, with an opt-in environment variable to require it. This reverses the "fails closed" wording in `docs/release-runbook.md:83`, so it needs your explicit approval.
2. **Unix PATH.** Print-only (recommended, matches your projects), or edit shell profiles like uv and bun with an opt-out.
3. **Windows install directory.** `%LOCALAPPDATA%\skillhub\bin` (recommended, matches herdr-gateway) or `$HOME\.local\bin` (matches uv and the Unix path).
4. **Custom domain.** None for now (recommended), or a short domain that redirects to the release asset.
5. **Package managers and GoReleaser.** Defer (recommended), or add Homebrew and Scoop now, which means adopting GoReleaser.
6. **`skillhub self-update`.** P2 (recommended), or bring it into the first release.
7. **Extra targets.** Add all three (recommended), or only linux/arm64 plus darwin/amd64.

## Limitations

- I did not run the release workflow or a dispatch draft. The findings about pipeline behaviour come from reading `release.yml`.
- I inspected installer scripts from their live URLs on 2026-09-30. Their behaviour can change.
- I read cargo-dist's checksum and attestation details from the uv script itself; the dist documentation page was incomplete.
- I did not assess Windows Defender or SmartScreen behaviour for an unsigned Go `.exe`. It depends on reputation and needs testing on real machines.

## Sources

- uv: https://docs.astral.sh/uv/getting-started/installation/ and the live scripts https://astral.sh/uv/install.sh and https://astral.sh/uv/install.ps1 (embedded SHA-256 values, HKCU PATH).
- bun: https://bun.com/docs/installation and https://bun.sh/install
- deno: https://docs.deno.com/runtime/getting_started/installation/ and https://deno.land/install.sh
- rustup: https://rust-lang.github.io/rustup/installation/other.html
- mise: https://mise.jdx.dev/installing-mise.html and https://mise.run (embedded checksums)
- chezmoi: https://www.chezmoi.io/install/, https://get.chezmoi.io, and https://github.com/twpayne/chezmoi/blob/master/.config/goreleaser.yaml
- starship: https://raw.githubusercontent.com/starship/starship/master/install/install.sh
- zoxide: https://raw.githubusercontent.com/ajeetdsouza/zoxide/main/install.sh
- fnm: https://raw.githubusercontent.com/Schniz/fnm/master/.ci/install.sh
- atuin: https://github.com/atuinsh/atuin (README, `setup.atuin.sh`)
- ollama: https://ollama.com/install.sh (redirects to a release asset) and https://github.com/ollama/ollama/blob/main/scripts/install.ps1
- GoReleaser: https://goreleaser.com/customization/sign/, https://goreleaser.com/customization/attestations/, https://goreleaser.com/customization/winget/
- GitHub attestations: https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations and https://cli.github.com/manual/gh_attestation_verify
- GitHub releases (latest excludes prereleases): https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository
- go-selfupdate (reference only; not recommended as a dependency): https://github.com/creativeprojects/go-selfupdate
- Release assets inspected with `gh api`: cli/cli, jesseduffield/lazygit, derailed/k9s, twpayne/chezmoi
- Local: `/home/vantt/projects/herdr-gateway/{install.sh,install.ps1,.github/workflows/release.yml,scripts/}`, `/home/vantt/projects/herdr/{distribution/,.github/workflows/,scripts/test_unix_installer.py,vantt/install.sh}`, `/home/vantt/projects/forgentX/{install.sh,.github/workflows/release.yml}`

Status: DONE
