#!/bin/sh
# Hermetic lifecycle and release-security coverage.
set -eu

unset XDG_CONFIG_HOME || true
XDG_CONFIG_HOME=""
export XDG_CONFIG_HOME
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/skillhub lifecycle.XXXXXXXX")
trap 'rm -rf "$TEST_ROOT"' EXIT HUP INT TERM
PREFIX="$TEST_ROOT/custom prefix"
PIPE_PREFIX="$TEST_ROOT/streamed install prefix"
PRERELEASE_PREFIX="$TEST_ROOT/prerelease install prefix"
CORRUPT_PREFIX="$TEST_ROOT/rejected install prefix"
UNMANAGED_PREFIX="$TEST_ROOT/unmanaged install prefix"
NO_COSIGN_PREFIX="$TEST_ROOT/no cosign install prefix"
WORKSPACE="$TEST_ROOT/canonical workspace"
HOST_CONFIG="$TEST_ROOT/host config.json"
APP_CONFIG="$TEST_ROOT/config root/skillhub"
HTTP_ROOT="$TEST_ROOT/http releases"
HTTP_LOG="$TEST_ROOT/http-urls.log"
COSIGN_LOG="$TEST_ROOT/cosign.log"
FAKE_BIN="$TEST_ROOT/fake-bin"

fail() {
	printf 'FAIL: %s\n' "$*" >&2
	exit 1
}

assert_file() {
	[ -f "$1" ] || fail "expected file: $1"
}

assert_absent() {
	[ ! -e "$1" ] && [ ! -L "$1" ] || fail "expected path to be absent: $1"
}

assert_output() {
	expected=$1
	shift
	actual=$("$@") || fail "command failed: $*"
	[ "$actual" = "$expected" ] || fail "expected '$expected', got '$actual' from: $*"
}

assert_exit_status() {
	expected_status=$1
	shift
	set +e
	"$@" >/dev/null 2>&1
	actual_status=$?
	set -e
	[ "$actual_status" -eq "$expected_status" ] || fail "expected exit status $expected_status, got $actual_status from: $*"
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		openssl dgst -sha256 "$1" | awk '{print $NF}'
	fi
}

sign_fixture_manifest() {
	manifest=$1
	bundle=$2
	sha256_file "$manifest" > "$bundle"
}

mkdir -p "$FAKE_BIN"
FIXTURE_VERIFIER="$FAKE_BIN/verify-fixture-bundle"
cat > "$FIXTURE_VERIFIER" <<'EOF'
#!/bin/sh
set -eu
[ "$#" -eq 4 ] || exit 20
manifest=$1
bundle=$2
identity=$3
issuer=$4
[ "$issuer" = https://token.actions.githubusercontent.com ] || exit 21
case "$identity" in
	https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml@refs/tags/v*) ;;
	*) exit 22 ;;
esac
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$manifest" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
	actual=$(shasum -a 256 "$manifest" | awk '{print $1}')
else
	actual=$(openssl dgst -sha256 "$manifest" | awk '{print $NF}')
fi
[ "$actual" = "$(sed -n '1p' "$bundle")" ] || exit 23
EOF
chmod 755 "$FIXTURE_VERIFIER"

make_fixture() {
	version=$1
	behavior=$2
	fixture_dir=$TEST_ROOT/fixtures/v$version
	mkdir -p "$fixture_dir/staging"
	cat > "$fixture_dir/staging/skillhub" <<EOF
#!/bin/sh
set -eu
case "\${1:-}" in
	version)
		if [ "$behavior" = fail-after-install ]; then
			exit 19
		fi
		printf '%s\n' "$version"
		;;
	*)
		exit 18
		;;
esac
EOF
	chmod 755 "$fixture_dir/staging/skillhub"
	tar -czf "$fixture_dir/skillhub-$version-linux-amd64.tar.gz" -C "$fixture_dir/staging" skillhub
	tar -czf "$fixture_dir/skillhub-$version-darwin-arm64.tar.gz" -C "$fixture_dir/staging" skillhub
	rm -rf "$fixture_dir/staging"

	manifest=$fixture_dir/checksums.txt
	bundle=$fixture_dir/checksums.txt.sigstore.json
	(
		cd "$fixture_dir"
		printf '%s  skillhub-%s-linux-amd64.tar.gz\n' "$(sha256_file "skillhub-$version-linux-amd64.tar.gz")" "$version" > "$manifest"
		printf '%s  skillhub-%s-darwin-arm64.tar.gz\n' "$(sha256_file "skillhub-$version-darwin-arm64.tar.gz")" "$version" >> "$manifest"
	)
	sign_fixture_manifest "$manifest" "$bundle"
	printf '%s\n' "$fixture_dir"
}

make_extra_entry_fixture() {
	fixture_dir=$TEST_ROOT/fixtures/extra-entry
	mkdir -p "$fixture_dir/staging"
	printf '#!/bin/sh\nexit 0\n' > "$fixture_dir/staging/skillhub"
	printf 'unexpected\n' > "$fixture_dir/staging/unexpected"
	chmod 755 "$fixture_dir/staging/skillhub"
	tar -czf "$fixture_dir/skillhub-5.0.0-linux-amd64.tar.gz" -C "$fixture_dir/staging" skillhub unexpected
	rm -rf "$fixture_dir/staging"
	manifest=$fixture_dir/checksums.txt
	bundle=$fixture_dir/checksums.txt.sigstore.json
	(
		cd "$fixture_dir"
		printf '%s  skillhub-5.0.0-linux-amd64.tar.gz\n' "$(sha256_file skillhub-5.0.0-linux-amd64.tar.gz)" > "$manifest"
	)
	sign_fixture_manifest "$manifest" "$bundle"
	printf '%s\n' "$fixture_dir"
}

make_link_fixture() {
	fixture_dir=$TEST_ROOT/fixtures/link-entry
	mkdir -p "$fixture_dir/staging"
	ln -s /etc/passwd "$fixture_dir/staging/skillhub"
	tar -czf "$fixture_dir/skillhub-6.0.0-linux-amd64.tar.gz" -C "$fixture_dir/staging" skillhub
	rm -rf "$fixture_dir/staging"
	manifest=$fixture_dir/checksums.txt
	bundle=$fixture_dir/checksums.txt.sigstore.json
	(
		cd "$fixture_dir"
		printf '%s  skillhub-6.0.0-linux-amd64.tar.gz\n' "$(sha256_file skillhub-6.0.0-linux-amd64.tar.gz)" > "$manifest"
	)
	sign_fixture_manifest "$manifest" "$bundle"
	printf '%s\n' "$fixture_dir"
}

fixture_v1=$(make_fixture 1.0.0 working)
fixture_v2=$(make_fixture 2.0.0 working)
fixture_prerelease=$(make_fixture '2.1.0-rc.1+build.5' working)
fixture_bad=$(make_fixture 3.0.0 fail-after-install)
fixture_corrupt=$(make_fixture 4.0.0 working)
fixture_extra=$(make_extra_entry_fixture)
fixture_link=$(make_link_fixture)
printf 'tampered\n' >> "$fixture_corrupt/skillhub-4.0.0-linux-amd64.tar.gz"

mkdir -p "$HTTP_ROOT/v1.0.0" "$HTTP_ROOT/v2.0.0" "$HTTP_ROOT/v2.1.0-rc.1+build.5"
cp "$fixture_v1"/* "$HTTP_ROOT/v1.0.0/"
cp "$fixture_v2"/* "$HTTP_ROOT/v2.0.0/"
cp "$fixture_prerelease"/* "$HTTP_ROOT/v2.1.0-rc.1+build.5/"

cat > "$FAKE_BIN/curl" <<'EOF'
#!/bin/sh
set -eu
output=
url=
connect=
maximum=
max_size=
while [ "$#" -gt 0 ]; do
	case "$1" in
		--output)
			[ "$#" -ge 2 ] || exit 30
			output=$2
			shift 2
			;;
		--connect-timeout)
			connect=$2
			shift 2
			;;
		--max-time)
			maximum=$2
			shift 2
			;;
		--max-filesize)
			max_size=$2
			shift 2
			;;
		--proto)
			[ "$2" = '=https' ] || exit 31
			shift 2
			;;
		--fail|--location|--tlsv1.2|--silent|--show-error|-fsSL)
			shift
			;;
		https://*)
			[ -z "$url" ] || exit 32
			url=$1
			shift
			;;
		*) exit 33 ;;
	esac
done
[ -n "$url" ] || exit 34
printf '%s\n' "$url" >> "$FIXTURE_HTTP_LOG"
case "$url" in
	https://fixtures.invalid/install.sh)
		source=$INSTALL_SCRIPT_SOURCE
		;;
	https://fixtures.invalid/releases/v*/*)
		relative=${url#https://fixtures.invalid/releases/}
		source=$FIXTURE_HTTP_ROOT/$relative
		[ "$connect" = 10 ] && [ "$maximum" = 120 ] && [ -n "$max_size" ] || exit 35
		;;
	*) exit 36 ;;
esac
[ -f "$source" ] || exit 37
if [ -n "$output" ]; then
	cp "$source" "$output"
else
	cat "$source"
fi
EOF
chmod 755 "$FAKE_BIN/curl"

cat > "$FAKE_BIN/cosign" <<'EOF'
#!/bin/sh
set -eu
[ "$#" -eq 8 ] || exit 40
[ "$1" = verify-blob ] || exit 41
[ "$2" = --bundle ] || exit 42
bundle=$3
[ "$4" = --certificate-identity ] || exit 43
[ "$5" = "$EXPECTED_COSIGN_IDENTITY" ] || exit 44
[ "$6" = --certificate-oidc-issuer ] || exit 45
[ "$7" = https://token.actions.githubusercontent.com ] || exit 46
manifest=$8
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$manifest" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
	actual=$(shasum -a 256 "$manifest" | awk '{print $1}')
else
	actual=$(openssl dgst -sha256 "$manifest" | awk '{print $NF}')
fi
[ "$actual" = "$(sed -n '1p' "$bundle")" ] || exit 47
printf '%s\n' "$5" >> "$FIXTURE_COSIGN_LOG"
EOF
chmod 755 "$FAKE_BIN/cosign"

mkdir -p "$WORKSPACE" "$(dirname -- "$HOST_CONFIG")" "$APP_CONFIG"
printf 'canonical-data\n' > "$WORKSPACE/skill.md"
printf 'host-data\n' > "$HOST_CONFIG"
printf 'app-data\n' > "$APP_CONFIG/settings.json"

# Unrendered version placeholder without --version or SKILLHUB_VERSION produces clear error.
assert_exit_status 1 "$SCRIPT_DIR/install.sh"
assert_exit_status 1 "$SCRIPT_DIR/install.sh" --dry-run

# Malformed versions and arguments are rejected without writing anything.
assert_exit_status 2 "$SCRIPT_DIR/install.sh" --version
assert_exit_status 2 "$SCRIPT_DIR/install.sh" --unknown
assert_exit_status 2 "$SCRIPT_DIR/install.sh" unexpected-positional-argument
for version in \
	1.2 v1.2 1.2.3.4 vv1.2.3 1.2.x \
	01.2.3 1.02.3 1.2.03 1.2.3-01 \
	1.2.3- 1.2.3+ 1.2.3-rc..1 1.2.3+build..1 \
	1.2.3-rc_1 1.2.3+build_1 1.2.3++build; do
	if "$SCRIPT_DIR/install.sh" --version "$version" --prefix "$CORRUPT_PREFIX" --os linux --arch amd64 --dry-run >/dev/null 2>&1; then
		fail "install accepted malformed version: $version"
	fi
done

# Supported platforms: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
for platform in 'linux amd64' 'linux arm64' 'darwin amd64' 'darwin arm64'; do
	set -- $platform
	"$SCRIPT_DIR/install.sh" --version 1.0.0 --prefix "$CORRUPT_PREFIX" --os "$1" --arch "$2" --dry-run >/dev/null || fail "supported platform rejected: $platform"
done

# Unsupported platforms are rejected with clear error.
for platform in 'windows amd64' 'windows arm64' 'freebsd amd64' 'linux mips' 'darwin x86'; do
	set -- $platform
	if "$SCRIPT_DIR/install.sh" --version 1.0.0 --prefix "$CORRUPT_PREFIX" --os "$1" --arch "$2" --dry-run >/dev/null 2>&1; then
		fail "install accepted unsupported platform: $platform"
	fi
done
assert_absent "$CORRUPT_PREFIX"

# Architecture detection: mapping aarch64 -> arm64 via fake uname.
cat > "$FAKE_BIN/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
	-s) echo Linux ;;
	-m) echo aarch64 ;;
	*) /usr/bin/uname "$@" ;;
esac
EOF
chmod 755 "$FAKE_BIN/uname"
PATH=$FAKE_BIN:$PATH "$SCRIPT_DIR/install.sh" --version 1.0.0 --prefix "$CORRUPT_PREFIX" --dry-run >/dev/null || fail "aarch64 architecture mapping failed"
rm -f "$FAKE_BIN/uname"

# Non-HTTPS sources are rejected, even for dry runs.
if "$SCRIPT_DIR/install.sh" --version 1.0.0 --release-base http://example.invalid --prefix "$PREFIX" --os linux --arch amd64 --dry-run >/dev/null 2>&1; then
	fail 'install accepted a non-HTTPS release base'
fi

# Local test mode requires a controlled bundle verifier.
if "$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$CORRUPT_PREFIX" --os linux --arch amd64 >/dev/null 2>&1; then
	fail 'local fixture mode succeeded without its controlled verifier'
fi

# curl|sh uses tag v1.0.0 but the archive name has no v prefix. The fake HTTP
# fixture also enforces every timeout/size option and exact URL.
: > "$HTTP_LOG"
: > "$COSIGN_LOG"
FIXTURE_HTTP_ROOT=$HTTP_ROOT FIXTURE_HTTP_LOG=$HTTP_LOG INSTALL_SCRIPT_SOURCE=$SCRIPT_DIR/install.sh PATH=$FAKE_BIN:$PATH \
	curl -fsSL https://fixtures.invalid/install.sh |
	PATH=$FAKE_BIN:$PATH FIXTURE_HTTP_ROOT=$HTTP_ROOT FIXTURE_HTTP_LOG=$HTTP_LOG FIXTURE_COSIGN_LOG=$COSIGN_LOG \
	EXPECTED_COSIGN_IDENTITY=https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml@refs/tags/v1.0.0 \
	sh -s -- --version 1.0.0 --release-base https://fixtures.invalid/releases --prefix "$PIPE_PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null || fail 'curl|sh install failed'
cat > "$TEST_ROOT/expected-install-urls" <<'EOF'
https://fixtures.invalid/install.sh
https://fixtures.invalid/releases/v1.0.0/checksums.txt
https://fixtures.invalid/releases/v1.0.0/checksums.txt.sigstore.json
https://fixtures.invalid/releases/v1.0.0/skillhub-1.0.0-linux-amd64.tar.gz
EOF
cmp -s "$TEST_ROOT/expected-install-urls" "$HTTP_LOG" || fail 'curl|sh requested unexpected release URLs'
assert_file "$PIPE_PREFIX/bin/skillhub"
assert_output 1.0.0 "$PIPE_PREFIX/bin/skillhub" version

# A release-shaped prerelease/build input uses the exact v-prefixed tag URL,
# unprefixed artifact version, checksum bundle name, and tag workflow identity.
: > "$HTTP_LOG"
: > "$COSIGN_LOG"
PATH=$FAKE_BIN:$PATH FIXTURE_HTTP_ROOT=$HTTP_ROOT FIXTURE_HTTP_LOG=$HTTP_LOG FIXTURE_COSIGN_LOG=$COSIGN_LOG \
EXPECTED_COSIGN_IDENTITY=https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml@refs/tags/v2.1.0-rc.1+build.5 \
	"$SCRIPT_DIR/install.sh" --version v2.1.0-rc.1+build.5 --release-base https://fixtures.invalid/releases --prefix "$PRERELEASE_PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null
cat > "$TEST_ROOT/expected-prerelease-urls" <<'EOF'
https://fixtures.invalid/releases/v2.1.0-rc.1+build.5/checksums.txt
https://fixtures.invalid/releases/v2.1.0-rc.1+build.5/checksums.txt.sigstore.json
https://fixtures.invalid/releases/v2.1.0-rc.1+build.5/skillhub-2.1.0-rc.1+build.5-linux-amd64.tar.gz
EOF
cmp -s "$TEST_ROOT/expected-prerelease-urls" "$HTTP_LOG" || fail 'prerelease install requested unexpected release URLs'
assert_output 2.1.0-rc.1+build.5 "$PRERELEASE_PREFIX/bin/skillhub" version
grep -q '^version=2.1.0-rc.1+build.5$' "$PRERELEASE_PREFIX/bin/.skillhub-managed" || fail 'prerelease install marker did not contain normalized version'

# Direct install accepts a v-prefixed input while using unprefixed artifacts.
SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER "$SCRIPT_DIR/install.sh" --version v1.0.0 --local-fixture "$fixture_v1" --prefix "$PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null
assert_file "$PREFIX/bin/skillhub"
assert_file "$PREFIX/bin/.skillhub-managed"
assert_output 1.0.0 "$PREFIX/bin/skillhub" version
grep -q '^version=1.0.0$' "$PREFIX/bin/.skillhub-managed" || fail 'install marker did not contain normalized version'

# Re-run installer with same version is a no-op that reports already installed.
same_version_output=$(SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER "$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$PREFIX" --os linux --arch amd64 --no-modify-path)
case "$same_version_output" in
	*"already installed"*) ;;
	*) fail "re-run did not report already installed: $same_version_output" ;;
esac
assert_output 1.0.0 "$PREFIX/bin/skillhub" version

# Production-path upgrade via re-running install.sh authenticates the manifest and preserves user data.
: > "$HTTP_LOG"
PATH=$FAKE_BIN:$PATH FIXTURE_HTTP_ROOT=$HTTP_ROOT FIXTURE_HTTP_LOG=$HTTP_LOG FIXTURE_COSIGN_LOG=$COSIGN_LOG \
EXPECTED_COSIGN_IDENTITY=https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml@refs/tags/v2.0.0 \
	"$SCRIPT_DIR/install.sh" --version v2.0.0 --release-base https://fixtures.invalid/releases --prefix "$PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null
cat > "$TEST_ROOT/expected-upgrade-urls" <<'EOF'
https://fixtures.invalid/releases/v2.0.0/checksums.txt
https://fixtures.invalid/releases/v2.0.0/checksums.txt.sigstore.json
https://fixtures.invalid/releases/v2.0.0/skillhub-2.0.0-linux-amd64.tar.gz
EOF
cmp -s "$TEST_ROOT/expected-upgrade-urls" "$HTTP_LOG" || fail 'upgrade requested unexpected release URLs'
assert_output 2.0.0 "$PREFIX/bin/skillhub" version
assert_file "$WORKSPACE/skill.md"
assert_file "$HOST_CONFIG"
assert_file "$APP_CONFIG/settings.json"

# A binary that fails only from the final path triggers rollback during re-run upgrade.
if SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER "$SCRIPT_DIR/install.sh" --version 3.0.0 --local-fixture "$fixture_bad" --prefix "$PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null 2>&1; then
	fail 'upgrade unexpectedly succeeded with a failing installed binary'
fi
assert_output 2.0.0 "$PREFIX/bin/skillhub" version
grep -q '^version=2.0.0$' "$PREFIX/bin/.skillhub-managed" || fail 'rollback did not preserve the prior marker'

# Refusing to overwrite or uninstall an unmanaged binary.
mkdir -p "$UNMANAGED_PREFIX/bin"
printf '#!/bin/sh\nexit 0\n' > "$UNMANAGED_PREFIX/bin/skillhub"
chmod 755 "$UNMANAGED_PREFIX/bin/skillhub"
if SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER "$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$UNMANAGED_PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null 2>&1; then
	fail 'installer unexpectedly overwrote unmanaged binary'
fi
if "$SCRIPT_DIR/install.sh" --uninstall --prefix "$UNMANAGED_PREFIX" >/dev/null 2>&1; then
	fail 'uninstall unexpectedly removed unmanaged binary'
fi
assert_file "$UNMANAGED_PREFIX/bin/skillhub"

# Signature verification matrix:
# 1. Cosign absent on PATH: install succeeds and prints warning notice.
# Prepare a clean PATH without fake cosign:
BIN_WITHOUT_COSIGN="$TEST_ROOT/bin-no-cosign"
mkdir -p "$BIN_WITHOUT_COSIGN"
cp "$FAKE_BIN/curl" "$BIN_WITHOUT_COSIGN/curl"
no_cosign_out=$(PATH=$BIN_WITHOUT_COSIGN:$PATH FIXTURE_HTTP_ROOT=$HTTP_ROOT FIXTURE_HTTP_LOG=$HTTP_LOG \
	"$SCRIPT_DIR/install.sh" --version v1.0.0 --release-base https://fixtures.invalid/releases --prefix "$NO_COSIGN_PREFIX" --os linux --arch amd64 --no-modify-path)
case "$no_cosign_out" in
	*"Signature not checked (cosign not installed)"*) ;;
	*) fail "missing signature notice when cosign absent: $no_cosign_out" ;;
esac
assert_file "$NO_COSIGN_PREFIX/bin/skillhub"
assert_output 1.0.0 "$NO_COSIGN_PREFIX/bin/skillhub" version

# 2. Cosign absent on PATH and SKILLHUB_REQUIRE_SIGNATURE=1: aborts.
if PATH=$BIN_WITHOUT_COSIGN:$PATH FIXTURE_HTTP_ROOT=$HTTP_ROOT FIXTURE_HTTP_LOG=$HTTP_LOG SKILLHUB_REQUIRE_SIGNATURE=1 \
	"$SCRIPT_DIR/install.sh" --version v1.0.0 --release-base https://fixtures.invalid/releases --prefix "$TEST_ROOT/req-sig" --os linux --arch amd64 --no-modify-path >/dev/null 2>&1; then
	fail 'install succeeded when cosign was absent but signature required'
fi
assert_absent "$TEST_ROOT/req-sig"

# 3. Bad signature when cosign is present: aborts.
if PATH=$FAKE_BIN:$PATH FIXTURE_HTTP_ROOT=$HTTP_ROOT FIXTURE_HTTP_LOG=$HTTP_LOG FIXTURE_COSIGN_LOG=$COSIGN_LOG \
	EXPECTED_COSIGN_IDENTITY=https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml@refs/tags/vWRONG \
	"$SCRIPT_DIR/install.sh" --version v1.0.0 --release-base https://fixtures.invalid/releases --prefix "$CORRUPT_PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null 2>&1; then
	fail 'installer accepted a mismatched cosign signature'
fi
assert_absent "$CORRUPT_PREFIX"

# Authentication, SHA-256, and archive shape all fail closed before activation.
tampered_bundle=$TEST_ROOT/tampered-bundle
cp -R "$fixture_v1" "$tampered_bundle"
printf 'not-a-valid-bundle\n' > "$tampered_bundle/checksums.txt.sigstore.json"
if SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER "$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$tampered_bundle" --prefix "$CORRUPT_PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null 2>&1; then
	fail 'installer accepted an unauthenticated checksum manifest'
fi
if SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER "$SCRIPT_DIR/install.sh" --version 4.0.0 --local-fixture "$fixture_corrupt" --prefix "$CORRUPT_PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null 2>&1; then
	fail 'installer accepted a corrupt archive'
fi
if SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER "$SCRIPT_DIR/install.sh" --version 5.0.0 --local-fixture "$fixture_extra" --prefix "$CORRUPT_PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null 2>&1; then
	fail 'installer accepted an archive containing an unexpected entry'
fi
if SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER "$SCRIPT_DIR/install.sh" --version 6.0.0 --local-fixture "$fixture_link" --prefix "$CORRUPT_PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null 2>&1; then
	fail 'installer accepted an archive link with a traversal target'
fi
assert_absent "$CORRUPT_PREFIX"

# Every downloaded asset class has an enforced byte limit.
oversize_checksums=$TEST_ROOT/oversize-checksums
cp -R "$fixture_v1" "$oversize_checksums"
dd if=/dev/null of="$oversize_checksums/checksums.txt" bs=1 seek=1048577 2>/dev/null
if SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER "$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$oversize_checksums" --prefix "$CORRUPT_PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null 2>&1; then
	fail 'installer accepted an oversized checksum manifest'
fi
oversize_bundle=$TEST_ROOT/oversize-bundle
cp -R "$fixture_v1" "$oversize_bundle"
dd if=/dev/null of="$oversize_bundle/checksums.txt.sigstore.json" bs=1 seek=4194305 2>/dev/null
if SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER "$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$oversize_bundle" --prefix "$CORRUPT_PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null 2>&1; then
	fail 'installer accepted an oversized Sigstore bundle'
fi
oversize_archive=$TEST_ROOT/oversize-archive
cp -R "$fixture_v1" "$oversize_archive"
dd if=/dev/null of="$oversize_archive/skillhub-1.0.0-linux-amd64.tar.gz" bs=1 seek=134217729 2>/dev/null
if SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER "$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$oversize_archive" --prefix "$CORRUPT_PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null 2>&1; then
	fail 'installer accepted an oversized release archive'
fi
assert_absent "$CORRUPT_PREFIX"

# Shell profile modification and idempotency testing across bash, zsh, fish, and profile.
FAKE_HOME="$TEST_ROOT/fake-user-home"
mkdir -p "$FAKE_HOME"

# 1. Bash: ~/.bashrc with missing trailing newline in initial content.
printf '# initial bashrc\nexport PREEXISTING=1' > "$FAKE_HOME/.bashrc"
SHELL=/bin/bash HOME=$FAKE_HOME SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER \
	"$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$FAKE_HOME/.local" --os linux --arch amd64 >/dev/null
grep -q '^export PREEXISTING=1$' "$FAKE_HOME/.bashrc" || fail 'preexisting bashrc content corrupted'
grep -q '^# >>> skillhub >>>$' "$FAKE_HOME/.bashrc" || fail 'marked block missing in .bashrc'
grep -q "export PATH=\"$FAKE_HOME/.local/bin:\$PATH\"" "$FAKE_HOME/.bashrc" || fail 'PATH export missing in .bashrc'
grep -q '^# <<< skillhub <<<$' "$FAKE_HOME/.bashrc" || fail 'closing marked block missing in .bashrc'

# Second run on same shell: idempotent (marked block appears exactly once).
SHELL=/bin/bash HOME=$FAKE_HOME SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER \
	"$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$FAKE_HOME/.local" --os linux --arch amd64 >/dev/null
block_count=$(grep -c '^# >>> skillhub >>>$' "$FAKE_HOME/.bashrc")
[ "$block_count" -eq 1 ] || fail "marked block not idempotent in .bashrc: count=$block_count"

# 2. Zsh: $ZDOTDIR/.zshrc
FAKE_ZDOT="$TEST_ROOT/fake-zdot"
mkdir -p "$FAKE_ZDOT"
printf '# initial zshrc\n' > "$FAKE_ZDOT/.zshrc"
SHELL=/usr/bin/zsh ZDOTDIR=$FAKE_ZDOT HOME=$FAKE_HOME SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER \
	"$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$FAKE_HOME/.local-zsh" --os linux --arch amd64 >/dev/null
grep -q '^# >>> skillhub >>>$' "$FAKE_ZDOT/.zshrc" || fail 'marked block missing in zshrc'
grep -q "export PATH=\"$FAKE_HOME/.local-zsh/bin:\$PATH\"" "$FAKE_ZDOT/.zshrc" || fail 'PATH export missing in zshrc'
# Idempotent:
SHELL=/usr/bin/zsh ZDOTDIR=$FAKE_ZDOT HOME=$FAKE_HOME SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER \
	"$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$FAKE_HOME/.local-zsh" --os linux --arch amd64 >/dev/null
zsh_block_count=$(grep -c '^# >>> skillhub >>>$' "$FAKE_ZDOT/.zshrc")
[ "$zsh_block_count" -eq 1 ] || fail "marked block not idempotent in zshrc: count=$zsh_block_count"

# 3. Fish: ~/.config/fish/conf.d/skillhub.fish
FISH_HOME="$TEST_ROOT/fake-fish-home"
mkdir -p "$FISH_HOME/.config/fish/conf.d"
XDG_CONFIG_HOME="" SHELL=/usr/bin/fish HOME=$FISH_HOME SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER \
	"$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$FISH_HOME/.local" --os linux --arch amd64 >/dev/null
fish_cfg="$FISH_HOME/.config/fish/conf.d/skillhub.fish"
assert_file "$fish_cfg"
grep -q "fish_add_path \"$FISH_HOME/.local/bin\"" "$fish_cfg" || fail 'fish_add_path missing in skillhub.fish'
# Idempotent:
XDG_CONFIG_HOME="" SHELL=/usr/bin/fish HOME=$FISH_HOME SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER \
	"$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$FISH_HOME/.local" --os linux --arch amd64 >/dev/null
fish_block_count=$(grep -c '^# >>> skillhub >>>$' "$fish_cfg")
[ "$fish_block_count" -eq 1 ] || fail "marked block not idempotent in fish config: count=$fish_block_count"

# 4. Fallback ~/.profile when no shell-specific rc files exist.
SH_HOME="$TEST_ROOT/fake-sh-home"
mkdir -p "$SH_HOME"
SHELL=/bin/sh HOME=$SH_HOME SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER \
	"$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$SH_HOME/.local" --os linux --arch amd64 >/dev/null
assert_file "$SH_HOME/.profile"
grep -q '^# >>> skillhub >>>$' "$SH_HOME/.profile" || fail 'marked block missing in .profile'
# Idempotent:
SHELL=/bin/sh HOME=$SH_HOME SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER \
	"$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$SH_HOME/.local" --os linux --arch amd64 >/dev/null
profile_block_count=$(grep -c '^# >>> skillhub >>>$' "$SH_HOME/.profile")
[ "$profile_block_count" -eq 1 ] || fail "marked block not idempotent in .profile: count=$profile_block_count"

# 5. Opt-out: --no-modify-path prints exact line and leaves shell profile untouched.
OPT_OUT_HOME="$TEST_ROOT/fake-opt-out-home"
mkdir -p "$OPT_OUT_HOME"
printf '# pristine\n' > "$OPT_OUT_HOME/.bashrc"
opt_out_msg=$(SHELL=/bin/bash HOME=$OPT_OUT_HOME SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER \
	"$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$OPT_OUT_HOME/.local" --os linux --arch amd64 --no-modify-path)
case "$opt_out_msg" in
	*"Notice:"*"is not in your PATH"*) ;;
	*) fail "missing PATH notice on opt-out: $opt_out_msg" ;;
esac
grep -q '# >>> skillhub >>>' "$OPT_OUT_HOME/.bashrc" && fail 'profile was modified despite --no-modify-path'

# 6. Read-only profile is skipped with a printed notice.
RO_HOME="$TEST_ROOT/fake-ro-home"
mkdir -p "$RO_HOME"
printf '# readonly\n' > "$RO_HOME/.bashrc"
chmod 444 "$RO_HOME/.bashrc"
ro_msg=$(SHELL=/bin/bash HOME=$RO_HOME SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER \
	"$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$RO_HOME/.local" --os linux --arch amd64)
case "$ro_msg" in
	*"is read-only; skipping shell profile update"*) ;;
	*) fail "missing read-only notice: $ro_msg" ;;
esac
chmod 644 "$RO_HOME/.bashrc"

# Uninstall:
# 1. Dry run removes nothing.
"$SCRIPT_DIR/install.sh" --uninstall --prefix "$PREFIX" --dry-run >/dev/null
assert_file "$PREFIX/bin/skillhub"
assert_file "$PREFIX/bin/.skillhub-managed"

# 2. Real uninstall removes binary, marker, and PATH marked block from shell profiles.
HOME=$FAKE_HOME ZDOTDIR=$FAKE_ZDOT "$SCRIPT_DIR/install.sh" --uninstall --prefix "$PREFIX" >/dev/null
assert_absent "$PREFIX/bin/skillhub"
assert_absent "$PREFIX/bin/.skillhub-managed"
assert_file "$WORKSPACE/skill.md"
assert_file "$HOST_CONFIG"
assert_file "$APP_CONFIG/settings.json"

# Profile cleanup verification:
HOME=$FAKE_HOME "$SCRIPT_DIR/install.sh" --uninstall --prefix "$FAKE_HOME/.local" >/dev/null
grep -q '# >>> skillhub >>>' "$FAKE_HOME/.bashrc" && fail 'marked block remained in .bashrc after uninstall'
grep -q '^export PREEXISTING=1$' "$FAKE_HOME/.bashrc" || fail 'preexisting user config removed during uninstall'

HOME=$FISH_HOME "$SCRIPT_DIR/install.sh" --uninstall --prefix "$FISH_HOME/.local" >/dev/null
assert_absent "$fish_cfg"

# Config purge is separate and never touches canonical data or host config.
SKILLHUB_FIXTURE_VERIFIER=$FIXTURE_VERIFIER "$SCRIPT_DIR/install.sh" --version 1.0.0 --local-fixture "$fixture_v1" --prefix "$PREFIX" --os linux --arch amd64 --no-modify-path >/dev/null
if SKILLHUB_CONFIG_DIR=$APP_CONFIG "$SCRIPT_DIR/install.sh" --uninstall --prefix "$PREFIX" --purge-config >/dev/null 2>&1; then
	fail 'config purge succeeded without --yes'
fi
assert_file "$APP_CONFIG/settings.json"
SKILLHUB_CONFIG_DIR=$APP_CONFIG "$SCRIPT_DIR/install.sh" --uninstall --prefix "$PREFIX" --purge-config --yes >/dev/null
assert_absent "$APP_CONFIG"
assert_file "$WORKSPACE/skill.md"
assert_file "$HOST_CONFIG"

printf '%s\n' 'Installer lifecycle tests passed.'
