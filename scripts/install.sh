#!/bin/sh
# Download, verify, atomically install, upgrade, or uninstall Skill Hub.
set -eu

SKILLHUB_SCRIPT_NAME=${0##*/}
SKILLHUB_PREFIX=${SKILLHUB_PREFIX:-"$HOME/.local"}
SKILLHUB_RELEASE_BASE=${SKILLHUB_RELEASE_BASE:-https://github.com/vantt/mcp-skill-hub/releases/download}
DEFAULT_VERSION='@SKILLHUB_VERSION@'
SKILLHUB_VERSION=${SKILLHUB_VERSION:-$DEFAULT_VERSION}
SKILLHUB_OS=${SKILLHUB_OS:-}
SKILLHUB_ARCH=${SKILLHUB_ARCH:-}
SKILLHUB_INSTALL_DIR=${SKILLHUB_INSTALL_DIR:-}
SKILLHUB_CONFIG_DIR=${SKILLHUB_CONFIG_DIR:-}
SKILLHUB_LOCAL_FIXTURE=
SKILLHUB_DRY_RUN=0
SKILLHUB_UNINSTALL=0
SKILLHUB_PURGE_CONFIG=0
SKILLHUB_YES=0
SKILLHUB_NO_MODIFY_PATH=${SKILLHUB_NO_MODIFY_PATH:-0}
SKILLHUB_REQUIRE_SIGNATURE=${SKILLHUB_REQUIRE_SIGNATURE:-0}

SKILLHUB_CONNECT_TIMEOUT=10
SKILLHUB_DOWNLOAD_TIMEOUT=120
SKILLHUB_CHECKSUM_MAX_BYTES=1048576
SKILLHUB_BUNDLE_MAX_BYTES=4194304
SKILLHUB_ARCHIVE_MAX_BYTES=134217728
SKILLHUB_SIGSTORE_ISSUER=https://token.actions.githubusercontent.com
SKILLHUB_SIGSTORE_WORKFLOW=https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml
SKILLHUB_FIXTURE_VERIFIER=${SKILLHUB_FIXTURE_VERIFIER:-}

skillhub_error() {
	printf 'ERROR: %s\n' "$*" >&2
	exit 1
}

skillhub_usage_error() {
	printf 'ERROR: %s\n' "$*" >&2
	printf 'Run %s --help for usage.\n' "$SKILLHUB_SCRIPT_NAME" >&2
	exit 2
}

skillhub_command_exists() {
	command -v "$1" >/dev/null 2>&1
}

skillhub_resolve_install_paths() {
	if [ -n "${SKILLHUB_INSTALL_DIR:-}" ]; then
		SKILLHUB_BIN_DIR=$SKILLHUB_INSTALL_DIR
	else
		SKILLHUB_BIN_DIR=$SKILLHUB_PREFIX/bin
	fi
	SKILLHUB_BIN_DIR=${SKILLHUB_BIN_DIR%/}
	SKILLHUB_TARGET=$SKILLHUB_BIN_DIR/skillhub
	SKILLHUB_MARKER=$SKILLHUB_BIN_DIR/.skillhub-managed
}

skillhub_validate_platform() {
	case "$SKILLHUB_OS-$SKILLHUB_ARCH" in
		linux-amd64|linux-arm64|darwin-amd64|darwin-arm64) ;;
		*) skillhub_error "Unsupported shell installer platform '$SKILLHUB_OS/$SKILLHUB_ARCH'; supported platforms are linux/amd64, linux/arm64, darwin/amd64, and darwin/arm64. Windows releases are not installed by this shell installer." ;;
	esac
}

skillhub_detect_platform() {
	if [ -z "${SKILLHUB_OS:-}" ]; then
		case "$(uname -s)" in
			Linux) SKILLHUB_OS=linux ;;
			Darwin) SKILLHUB_OS=darwin ;;
			*) skillhub_error "Unsupported operating system '$(uname -s)'." ;;
		esac
	fi
	if [ -z "${SKILLHUB_ARCH:-}" ]; then
		case "$(uname -m)" in
			x86_64|amd64) SKILLHUB_ARCH=amd64 ;;
			arm64|aarch64) SKILLHUB_ARCH=arm64 ;;
			*) skillhub_error "Unsupported architecture '$(uname -m)'." ;;
		esac
	fi
	skillhub_validate_platform
}

skillhub_validate_version() {
	[ -n "$SKILLHUB_VERSION" ] && [ "$SKILLHUB_VERSION" != '@SKILLHUB_VERSION@' ] || skillhub_error "A release version is required. Pass --version VERSION or set SKILLHUB_VERSION."
	version=$SKILLHUB_VERSION
	case "$version" in
		v*) version=${version#v} ;;
	esac
	invalid_version="Invalid release version '$SKILLHUB_VERSION'; expected SemVer such as 1.2.3, v1.2.3, or v1.2.3-rc.1+build.5."
	case "$version" in
		''|*[!0-9A-Za-z.+-]*) skillhub_error "$invalid_version" ;;
	esac

	core_prerelease=$version
	case "$version" in
		*+*)
			build=${version#*+}
			core_prerelease=${version%%+*}
			case "$build" in
				''|*+*|.*|*.|*..*|*[!0-9A-Za-z.-]*) skillhub_error "$invalid_version" ;;
			esac
			old_ifs=$IFS
			IFS=.
			set -- $build
			IFS=$old_ifs
			for identifier in "$@"; do
				case "$identifier" in
					''|*[!0-9A-Za-z-]*) skillhub_error "$invalid_version" ;;
				esac
			done
			;;
	esac

	core=$core_prerelease
	case "$core_prerelease" in
		*-*)
			prerelease=${core_prerelease#*-}
			core=${core_prerelease%%-*}
			case "$prerelease" in
				''|.*|*.|*..*|*[!0-9A-Za-z.-]*) skillhub_error "$invalid_version" ;;
			esac
			old_ifs=$IFS
			IFS=.
			set -- $prerelease
			IFS=$old_ifs
			for identifier in "$@"; do
				case "$identifier" in
					''|*[!0-9A-Za-z-]*) skillhub_error "$invalid_version" ;;
					0|*[!0-9]*) ;;
					0*) skillhub_error "$invalid_version" ;;
				esac
			done
			;;
	esac

	case "$core" in
		''|.*|*.|*..*|*[!0-9.]*) skillhub_error "$invalid_version" ;;
	esac
	old_ifs=$IFS
	IFS=.
	set -- $core
	IFS=$old_ifs
	[ "$#" -eq 3 ] || skillhub_error "$invalid_version"
	for component in "$@"; do
		case "$component" in
			0|[1-9]*) ;;
			*) skillhub_error "$invalid_version" ;;
		esac
		case "$component" in
			*[!0-9]*|0?*) skillhub_error "$invalid_version" ;;
		esac
	done
	SKILLHUB_ARTIFACT_VERSION=$version
	SKILLHUB_RELEASE_TAG=v$version
	SKILLHUB_VERSION=$version
}

skillhub_validate_source() {
	if [ -n "$SKILLHUB_LOCAL_FIXTURE" ]; then
		[ -d "$SKILLHUB_LOCAL_FIXTURE" ] || skillhub_error "Local fixture directory not found: $SKILLHUB_LOCAL_FIXTURE"
		[ -n "$SKILLHUB_FIXTURE_VERIFIER" ] || skillhub_error "Local fixture mode requires SKILLHUB_FIXTURE_VERIFIER."
		[ -f "$SKILLHUB_FIXTURE_VERIFIER" ] && [ -x "$SKILLHUB_FIXTURE_VERIFIER" ] || skillhub_error "Fixture verifier is not an executable file: $SKILLHUB_FIXTURE_VERIFIER"
		return
	fi
	case "$SKILLHUB_RELEASE_BASE" in
		https://*) ;;
		*) skillhub_error "Release base must use HTTPS. Use --local-fixture DIR for explicit local test data." ;;
	esac
}

skillhub_require_tools() {
	skillhub_command_exists tar || skillhub_error "Required command not found: tar"
	if ! skillhub_command_exists sha256sum && ! skillhub_command_exists shasum && ! skillhub_command_exists openssl; then
		skillhub_error "SHA-256 verification requires sha256sum, shasum, or openssl."
	fi
	if [ -z "$SKILLHUB_LOCAL_FIXTURE" ]; then
		skillhub_command_exists curl || skillhub_error "Downloading releases requires curl."
		if [ "$SKILLHUB_REQUIRE_SIGNATURE" -eq 1 ] && ! skillhub_command_exists cosign; then
			skillhub_error "Signature verification is required (SKILLHUB_REQUIRE_SIGNATURE=1) but cosign is not installed. Install cosign from https://github.com/sigstore/cosign to continue."
		fi
	fi
}

skillhub_make_temp_dir() {
	SKILLHUB_TEMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/skillhub-install.XXXXXXXX") || skillhub_error "Could not create a temporary directory."
}

skillhub_cleanup_temp() {
	if [ -n "${SKILLHUB_TEMP_DIR:-}" ] && [ -d "$SKILLHUB_TEMP_DIR" ]; then
		rm -rf "$SKILLHUB_TEMP_DIR"
	fi
}

skillhub_file_size() {
	wc -c < "$1" | tr -d '[:space:]'
}

skillhub_enforce_size() {
	file=$1
	max_bytes=$2
	label=$3
	size=$(skillhub_file_size "$file") || skillhub_error "Could not determine size of $label."
	case "$size" in
		''|*[!0-9]*) skillhub_error "Could not determine size of $label." ;;
	esac
	[ "$size" -le "$max_bytes" ] || skillhub_error "$label exceeds the maximum allowed size of $max_bytes bytes."
}

skillhub_download() {
	source_name=$1
	destination=$2
	max_bytes=$3
	if [ -n "$SKILLHUB_LOCAL_FIXTURE" ]; then
		[ -f "$SKILLHUB_LOCAL_FIXTURE/$source_name" ] || skillhub_error "Fixture file not found: $SKILLHUB_LOCAL_FIXTURE/$source_name"
		skillhub_enforce_size "$SKILLHUB_LOCAL_FIXTURE/$source_name" "$max_bytes" "$source_name"
		cp "$SKILLHUB_LOCAL_FIXTURE/$source_name" "$destination" || skillhub_error "Could not copy fixture file: $source_name"
		return
	fi
	url=${SKILLHUB_RELEASE_BASE%/}/$SKILLHUB_RELEASE_TAG/$source_name
	case "$url" in
		https://*) ;;
		*) skillhub_error "Refusing non-HTTPS download: $url" ;;
	esac
	curl --fail --location --proto '=https' --tlsv1.2 --silent --show-error \
		--connect-timeout "$SKILLHUB_CONNECT_TIMEOUT" --max-time "$SKILLHUB_DOWNLOAD_TIMEOUT" \
		--max-filesize "$max_bytes" --output "$destination" "$url" || skillhub_error "Download failed: $url"
	skillhub_enforce_size "$destination" "$max_bytes" "$source_name"
}

skillhub_sha256() {
	file=$1
	if skillhub_command_exists sha256sum; then
		sha256sum "$file" | awk '{print $1}'
	elif skillhub_command_exists shasum; then
		shasum -a 256 "$file" | awk '{print $1}'
	else
		openssl dgst -sha256 "$file" | awk '{print $NF}'
	fi
}

skillhub_verify_manifest() {
	manifest=$1
	bundle=$2
	identity=$SKILLHUB_SIGSTORE_WORKFLOW@refs/tags/$SKILLHUB_RELEASE_TAG
	if [ -n "$SKILLHUB_LOCAL_FIXTURE" ]; then
		"$SKILLHUB_FIXTURE_VERIFIER" "$manifest" "$bundle" "$identity" "$SKILLHUB_SIGSTORE_ISSUER" || skillhub_error "Fixture checksum manifest authentication failed."
	else
		cosign verify-blob --bundle "$bundle" --certificate-identity "$identity" \
			--certificate-oidc-issuer "$SKILLHUB_SIGSTORE_ISSUER" "$manifest" >/dev/null || skillhub_error "Sigstore verification failed for checksums.txt."
	fi
	printf 'Authenticated checksums.txt with Sigstore bundle\n'
}

skillhub_verify_archive() {
	archive=$1
	manifest=$2
	archive_name=$3
	expected=
	matches=0
	while IFS=' ' read -r hash listed_name extra; do
		[ -n "$hash" ] || continue
		listed_name=${listed_name#\*}
		case "$listed_name" in
			"$archive_name")
				[ -z "${extra:-}" ] || skillhub_error "Malformed checksum entry for $archive_name."
				expected=$hash
				matches=$((matches + 1))
				;;
		esac
	done < "$manifest"
	[ "$matches" -eq 1 ] || skillhub_error "Checksum manifest must contain exactly one entry for $archive_name."
	case "$expected" in
		*[!0-9A-Fa-f]*|'') skillhub_error "Invalid SHA-256 checksum for $archive_name." ;;
	esac
	[ "${#expected}" -eq 64 ] || skillhub_error "Invalid SHA-256 checksum length for $archive_name."
	actual=$(skillhub_sha256 "$archive") || skillhub_error "Could not calculate SHA-256 for $archive_name."
	expected=$(printf '%s' "$expected" | tr 'A-F' 'a-f')
	[ "$actual" = "$expected" ] || skillhub_error "SHA-256 verification failed for $archive_name."
	printf 'Verified SHA-256 for %s\n' "$archive_name"
}

skillhub_fetch_candidate() {
	archive_name=skillhub-$SKILLHUB_ARTIFACT_VERSION-$SKILLHUB_OS-$SKILLHUB_ARCH.tar.gz
	archive=$SKILLHUB_TEMP_DIR/$archive_name
	manifest=$SKILLHUB_TEMP_DIR/checksums.txt
	bundle=$SKILLHUB_TEMP_DIR/checksums.txt.sigstore.json
	extract_dir=$SKILLHUB_TEMP_DIR/extract
	mkdir "$extract_dir" || skillhub_error "Could not prepare extraction directory."
	skillhub_download checksums.txt "$manifest" "$SKILLHUB_CHECKSUM_MAX_BYTES"

	identity=$SKILLHUB_SIGSTORE_WORKFLOW@refs/tags/$SKILLHUB_RELEASE_TAG
	if [ -n "$SKILLHUB_LOCAL_FIXTURE" ]; then
		skillhub_download checksums.txt.sigstore.json "$bundle" "$SKILLHUB_BUNDLE_MAX_BYTES"
		skillhub_verify_manifest "$manifest" "$bundle"
	elif skillhub_command_exists cosign; then
		skillhub_download checksums.txt.sigstore.json "$bundle" "$SKILLHUB_BUNDLE_MAX_BYTES"
		skillhub_verify_manifest "$manifest" "$bundle"
	else
		printf 'Signature not checked (cosign not installed). To verify: %s\n' \
			"cosign verify-blob --bundle checksums.txt.sigstore.json --certificate-identity $identity --certificate-oidc-issuer $SKILLHUB_SIGSTORE_ISSUER checksums.txt"
	fi

	skillhub_download "$archive_name" "$archive" "$SKILLHUB_ARCHIVE_MAX_BYTES"
	skillhub_verify_archive "$archive" "$manifest" "$archive_name"
	entries=$(tar -tzf "$archive") || skillhub_error "Could not inspect release archive: $archive_name"
	case "$entries" in
		skillhub) archive_member=skillhub ;;
		./skillhub) archive_member=./skillhub ;;
		*) skillhub_error "Release archive must contain exactly one non-traversing skillhub binary." ;;
	esac
	entry_details=$(LC_ALL=C tar -tvzf "$archive") || skillhub_error "Could not inspect release archive type: $archive_name"
	case "$entry_details" in
		-*) ;;
		*) skillhub_error "Release archive's skillhub entry must be a regular file, not a link or special file." ;;
	esac
	tar -xzf "$archive" -C "$extract_dir" "$archive_member" || skillhub_error "Could not extract release archive: $archive_name"
	SKILLHUB_CANDIDATE=$extract_dir/skillhub
	[ -f "$SKILLHUB_CANDIDATE" ] && [ ! -L "$SKILLHUB_CANDIDATE" ] || skillhub_error "Release archive does not contain a regular skillhub binary."
	chmod 755 "$SKILLHUB_CANDIDATE" || skillhub_error "Could not make the downloaded binary executable."
	"$SKILLHUB_CANDIDATE" version >/dev/null 2>&1 || skillhub_error "Downloaded skillhub binary failed its version check."
}

skillhub_is_managed() {
	[ -f "$SKILLHUB_MARKER" ] && [ ! -L "$SKILLHUB_MARKER" ] && [ "$(sed -n '1p' "$SKILLHUB_MARKER")" = "skillhub-managed-v1" ]
}

skillhub_path_contains() {
	check_dir=$1
	case ":$PATH:" in
		*:"$check_dir":*) return 0 ;;
		*) return 1 ;;
	esac
}

skillhub_append_marked_block() {
	target_file=$1
	entry_line=$2

	if [ -f "$target_file" ] && grep -q '# >>> skillhub >>>' "$target_file" 2>/dev/null; then
		return 0
	fi

	if [ -e "$target_file" ] && [ ! -w "$target_file" ]; then
		printf 'Notice: %s is read-only; skipping shell profile update. Add this line manually:\n  %s\n' "$target_file" "$entry_line"
		return 0
	fi

	target_dir=$(dirname "$target_file")
	if [ ! -d "$target_dir" ]; then
		mkdir -p "$target_dir" || {
			printf 'Notice: could not create directory %s; skipping shell profile update.\n' "$target_dir"
			return 0
		}
	fi
	if [ ! -w "$target_dir" ]; then
		printf 'Notice: directory %s is read-only; skipping shell profile update.\n' "$target_dir"
		return 0
	fi

	if [ -s "$target_file" ] && [ -n "$(tail -c 1 "$target_file" 2>/dev/null)" ]; then
		printf '\n' >> "$target_file"
	fi

	cat >> "$target_file" <<EOF
# >>> skillhub >>>
$entry_line
# <<< skillhub <<<
EOF
	[ -n "${SKILLHUB_PRIMARY_MODIFIED:-}" ] || SKILLHUB_PRIMARY_MODIFIED=$target_file
}

skillhub_configure_path() {
	if skillhub_path_contains "$SKILLHUB_BIN_DIR"; then
		return 0
	fi

	if [ "$SKILLHUB_NO_MODIFY_PATH" -eq 1 ]; then
		printf 'Notice: %s is not in your PATH.\n' "$SKILLHUB_BIN_DIR"
		printf 'Add it to your shell configuration file by running:\n'
		case "${SHELL:-}" in
			*fish*)
				printf '  fish_add_path "%s"\n' "$SKILLHUB_BIN_DIR"
				;;
			*)
				printf '  export PATH="%s:$PATH"\n' "$SKILLHUB_BIN_DIR"
				;;
		esac
		return 0
	fi

	SKILLHUB_PRIMARY_MODIFIED=""

	posix_line="export PATH=\"$SKILLHUB_BIN_DIR:\$PATH\""
	fish_line="fish_add_path \"$SKILLHUB_BIN_DIR\""

	bashrc="$HOME/.bashrc"
	bash_profile="$HOME/.bash_profile"
	zdotdir=${ZDOTDIR:-$HOME}
	zshrc="$zdotdir/.zshrc"
	fish_conf_dir="${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d"
	fish_rc="$fish_conf_dir/skillhub.fish"
	profile="$HOME/.profile"

	has_bash=0
	has_zsh=0
	has_fish=0

	case "${SHELL:-}" in
		*bash*) has_bash=1 ;;
		*zsh*) has_zsh=1 ;;
		*fish*) has_fish=1 ;;
	esac

	[ -f "$bashrc" ] && has_bash=1
	[ -f "$zshrc" ] && has_zsh=1
	[ -f "$fish_rc" ] || [ -d "${XDG_CONFIG_HOME:-$HOME/.config}/fish" ] && has_fish=1

	edited_any=0

	if [ "$has_bash" -eq 1 ]; then
		skillhub_append_marked_block "$bashrc" "$posix_line"
		edited_any=1
		if [ "$SKILLHUB_OS" = "darwin" ] && [ -f "$bash_profile" ]; then
			skillhub_append_marked_block "$bash_profile" "$posix_line"
		fi
	fi

	if [ "$has_zsh" -eq 1 ]; then
		skillhub_append_marked_block "$zshrc" "$posix_line"
		edited_any=1
	fi

	if [ "$has_fish" -eq 1 ]; then
		skillhub_append_marked_block "$fish_rc" "$fish_line"
		edited_any=1
	fi

	if [ "$edited_any" -eq 0 ]; then
		skillhub_append_marked_block "$profile" "$posix_line"
	fi

	primary_source=""
	case "${SHELL:-}" in
		*fish*)
			primary_source="source $fish_rc"
			;;
		*zsh*)
			primary_source="source $zshrc"
			;;
		*bash*)
			if [ "$SKILLHUB_OS" = "darwin" ] && [ -f "$bash_profile" ]; then
				primary_source="source $bash_profile"
			else
				primary_source="source $bashrc"
			fi
			;;
		*)
			if [ -n "$SKILLHUB_PRIMARY_MODIFIED" ]; then
				primary_source="source $SKILLHUB_PRIMARY_MODIFIED"
			else
				primary_source="source $profile"
			fi
			;;
	esac

	printf 'Open a new terminal or run: %s\n' "$primary_source"
}

skillhub_remove_marked_block() {
	file=$1
	[ -f "$file" ] || return 0
	grep -q '# >>> skillhub >>>' "$file" 2>/dev/null || return 0

	if [ "$SKILLHUB_DRY_RUN" -eq 1 ]; then
		printf 'Would remove skillhub PATH block from %s\n' "$file"
		return 0
	fi

	if [ ! -w "$file" ]; then
		printf 'Notice: %s is read-only; could not remove skillhub PATH block.\n' "$file"
		return 0
	fi

	tmp_file=$file.tmp.$$
	sed '/# >>> skillhub >>>/,/# <<< skillhub <<</d' "$file" > "$tmp_file"
	mv -f "$tmp_file" "$file"

	case "$file" in
		*/fish/conf.d/skillhub.fish)
			if [ ! -s "$file" ]; then
				rm -f "$file"
				printf 'Removed %s\n' "$file"
				return 0
			fi
			;;
	esac
	printf 'Removed skillhub PATH configuration from %s\n' "$file"
}

skillhub_uninstall() {
	skillhub_resolve_install_paths
	SKILLHUB_CONFIG_DIR=${SKILLHUB_CONFIG_DIR:-"${XDG_CONFIG_HOME:-$HOME/.config}/skillhub"}

	if [ "$SKILLHUB_PURGE_CONFIG" -eq 1 ]; then
		case "$SKILLHUB_CONFIG_DIR" in
			''|/|"$HOME"|"$HOME/") skillhub_error "Refusing unsafe application config path: $SKILLHUB_CONFIG_DIR" ;;
		esac
		case "/$SKILLHUB_CONFIG_DIR/" in
			*/../*|*/./*) skillhub_error "Refusing application config path containing dot segments: $SKILLHUB_CONFIG_DIR" ;;
		esac
		printf 'Config purge preview: %s\n' "$SKILLHUB_CONFIG_DIR"
		printf '%s\n' 'Canonical workspaces and host configuration will not be removed.'
		if [ "$SKILLHUB_DRY_RUN" -ne 1 ] && [ "$SKILLHUB_YES" -ne 1 ]; then
			skillhub_error "Config purge requires --yes after reviewing the preview."
		fi
	elif [ "$SKILLHUB_YES" -eq 1 ]; then
		skillhub_usage_error "--yes is valid only with --purge-config."
	fi

	managed_target=0
	if [ -e "$SKILLHUB_TARGET" ] || [ -L "$SKILLHUB_TARGET" ]; then
		skillhub_is_managed || skillhub_error "Refusing to remove unmanaged path: $SKILLHUB_TARGET"
		managed_target=1
		if [ "$SKILLHUB_DRY_RUN" -eq 1 ]; then
			action='Would remove'
		else
			action='Removing'
		fi
		printf '%s %s\n' "$action" "$SKILLHUB_TARGET"
	elif [ -e "$SKILLHUB_MARKER" ] || [ -L "$SKILLHUB_MARKER" ]; then
		skillhub_is_managed || skillhub_error "Invalid installation marker: $SKILLHUB_MARKER"
		managed_target=1
		printf 'Managed binary is already absent: %s\n' "$SKILLHUB_TARGET"
	else
		printf 'No managed skillhub installation found at %s\n' "$SKILLHUB_TARGET"
	fi

	skillhub_remove_marked_block "$HOME/.bashrc"
	skillhub_remove_marked_block "$HOME/.bash_profile"
	skillhub_remove_marked_block "${ZDOTDIR:-$HOME}/.zshrc"
	if [ -n "${ZDOTDIR:-}" ] && [ "$ZDOTDIR" != "$HOME" ]; then
		skillhub_remove_marked_block "$HOME/.zshrc"
	fi
	skillhub_remove_marked_block "${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d/skillhub.fish"
	skillhub_remove_marked_block "$HOME/.profile"
	if [ "$SKILLHUB_DRY_RUN" -eq 1 ]; then
		[ -e "$SKILLHUB_MARKER" ] && printf 'Would remove %s\n' "$SKILLHUB_MARKER"
		[ "$SKILLHUB_PURGE_CONFIG" -eq 1 ] && [ -e "$SKILLHUB_CONFIG_DIR" ] && printf 'Would remove application config %s\n' "$SKILLHUB_CONFIG_DIR"
		exit 0
	fi

	if [ "$managed_target" -eq 1 ] || skillhub_is_managed; then
		rm -f "$SKILLHUB_TARGET" || skillhub_error "Could not remove managed binary: $SKILLHUB_TARGET"
		rm -f "$SKILLHUB_MARKER" || skillhub_error "Could not remove installation marker: $SKILLHUB_MARKER"
	fi

	if [ "$SKILLHUB_PURGE_CONFIG" -eq 1 ] && [ -e "$SKILLHUB_CONFIG_DIR" ]; then
		rm -rf "$SKILLHUB_CONFIG_DIR" || skillhub_error "Could not remove application config: $SKILLHUB_CONFIG_DIR"
		printf 'Removed application config %s\n' "$SKILLHUB_CONFIG_DIR"
	fi

	printf '%s\n' 'Uninstall complete. Canonical workspaces and host configuration were preserved.'
	exit 0
}

usage() {
	cat <<'USAGE'
Usage: install.sh [options]

Download, verify, atomically install, upgrade, or uninstall Skill Hub.

Options:
  -v, --version VERSION      Release version to install (e.g. 1.0.0 or v1.0.0).
                             Overrides the default version baked into this script.
      --uninstall            Remove managed binary, marker, and installer PATH modifications.
      --purge-config         When uninstalling, also remove application configuration.
  -y, --yes                  Confirm configuration purge without prompt.
      --prefix DIR           Installation prefix (default: $HOME/.local). Binary installed to DIR/bin.
      --install-dir DIR      Install directly into DIR (overrides --prefix).
      --config-dir DIR       Application configuration directory (default: $XDG_CONFIG_HOME/skillhub).
      --no-modify-path       Do not append PATH configuration to shell profile files.
      --require-signature    Fail if cosign is not installed instead of skipping signature check.
      --os OS                Target OS (linux, darwin).
      --arch ARCH            Target architecture (amd64, arm64).
      --release-base URL     Base URL for release downloads.
      --local-fixture DIR    Install from local test fixture directory.
      --dry-run              Print actions without modifying the filesystem.
  -h, --help                 Show this help message.

Re-running install.sh with an existing managed installation automatically upgrades it.
If the installed version matches the target version, the installer is a no-op.
USAGE
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--help|-h)
			usage
			exit 0
			;;
		--version|-v)
			[ "$#" -ge 2 ] || skillhub_usage_error "Missing value for $1"
			SKILLHUB_VERSION=$2
			shift 2
			;;
		--version=*)
			SKILLHUB_VERSION=${1#*=}
			shift
			;;
		--uninstall)
			SKILLHUB_UNINSTALL=1
			shift
			;;
		--purge-config)
			SKILLHUB_PURGE_CONFIG=1
			shift
			;;
		--yes|-y)
			SKILLHUB_YES=1
			shift
			;;
		--no-modify-path)
			SKILLHUB_NO_MODIFY_PATH=1
			shift
			;;
		--require-signature)
			SKILLHUB_REQUIRE_SIGNATURE=1
			shift
			;;
		--prefix)
			[ "$#" -ge 2 ] || skillhub_usage_error "Missing value for $1"
			SKILLHUB_PREFIX=$2
			shift 2
			;;
		--prefix=*)
			SKILLHUB_PREFIX=${1#*=}
			shift
			;;
		--install-dir)
			[ "$#" -ge 2 ] || skillhub_usage_error "Missing value for $1"
			SKILLHUB_INSTALL_DIR=$2
			shift 2
			;;
		--install-dir=*)
			SKILLHUB_INSTALL_DIR=${1#*=}
			shift
			;;
		--config-dir)
			[ "$#" -ge 2 ] || skillhub_usage_error "Missing value for $1"
			SKILLHUB_CONFIG_DIR=$2
			shift 2
			;;
		--config-dir=*)
			SKILLHUB_CONFIG_DIR=${1#*=}
			shift
			;;
		--os)
			[ "$#" -ge 2 ] || skillhub_usage_error "Missing value for $1"
			SKILLHUB_OS=$2
			shift 2
			;;
		--os=*)
			SKILLHUB_OS=${1#*=}
			shift
			;;
		--arch)
			[ "$#" -ge 2 ] || skillhub_usage_error "Missing value for $1"
			SKILLHUB_ARCH=$2
			shift 2
			;;
		--arch=*)
			SKILLHUB_ARCH=${1#*=}
			shift
			;;
		--release-base)
			[ "$#" -ge 2 ] || skillhub_usage_error "Missing value for $1"
			SKILLHUB_RELEASE_BASE=$2
			shift 2
			;;
		--release-base=*)
			SKILLHUB_RELEASE_BASE=${1#*=}
			shift
			;;
		--local-fixture)
			[ "$#" -ge 2 ] || skillhub_usage_error "Missing value for $1"
			SKILLHUB_LOCAL_FIXTURE=$2
			shift 2
			;;
		--local-fixture=*)
			SKILLHUB_LOCAL_FIXTURE=${1#*=}
			shift
			;;
		--dry-run)
			SKILLHUB_DRY_RUN=1
			shift
			;;
		--)
			shift
			[ "$#" -eq 0 ] || skillhub_usage_error "Unexpected positional argument: $1"
			break
			;;
		*)
			skillhub_usage_error "Unknown option or argument: $1"
			;;
	esac
done

if [ "$SKILLHUB_UNINSTALL" -eq 1 ]; then
	skillhub_uninstall
fi

if [ "$SKILLHUB_PURGE_CONFIG" -eq 1 ] || [ "$SKILLHUB_YES" -eq 1 ]; then
	skillhub_usage_error "--purge-config and --yes are valid only with --uninstall."
fi

skillhub_validate_version
skillhub_detect_platform
skillhub_validate_source
skillhub_resolve_install_paths

is_upgrade=0
is_same_version=0

if [ -e "$SKILLHUB_TARGET" ] || [ -L "$SKILLHUB_TARGET" ]; then
	skillhub_is_managed || skillhub_error "Refusing to overwrite existing unmanaged binary at $SKILLHUB_TARGET. Only installations managed by skillhub installer (with a valid .skillhub-managed marker) can be upgraded."
	is_upgrade=1
	installed_version=
	if [ -f "$SKILLHUB_MARKER" ]; then
		installed_version=$(sed -n 's/^version=//p' "$SKILLHUB_MARKER" | head -n 1)
	fi
	if [ -n "$installed_version" ] && [ "$installed_version" = "$SKILLHUB_VERSION" ]; then
		is_same_version=1
	fi
elif [ -e "$SKILLHUB_MARKER" ] || [ -L "$SKILLHUB_MARKER" ]; then
	skillhub_error "Refusing installation because a marker already exists without a binary: $SKILLHUB_MARKER"
fi

if [ "$SKILLHUB_DRY_RUN" -eq 1 ]; then
	if [ "$is_same_version" -eq 1 ]; then
		printf 'Dry run: skillhub %s is already installed at %s\n' "$SKILLHUB_VERSION" "$SKILLHUB_TARGET"
	elif [ "$is_upgrade" -eq 1 ]; then
		printf 'Dry run: would upgrade skillhub to %s at %s\n' "$SKILLHUB_VERSION" "$SKILLHUB_TARGET"
	else
		printf 'Dry run: would install skillhub %s to %s\n' "$SKILLHUB_VERSION" "$SKILLHUB_TARGET"
	fi
	exit 0
fi

if [ "$is_same_version" -eq 1 ]; then
	printf 'skillhub %s is already installed at %s\n' "$SKILLHUB_VERSION" "$SKILLHUB_TARGET"
	skillhub_configure_path
	printf 'Next step:\n  skillhub init ~/skillhub --yes\n'
	exit 0
fi

skillhub_require_tools
skillhub_make_temp_dir
trap 'skillhub_cleanup_temp' EXIT HUP INT TERM
skillhub_fetch_candidate

mkdir -p "$SKILLHUB_BIN_DIR" || skillhub_error "Could not create install directory: $SKILLHUB_BIN_DIR"

if [ "$is_upgrade" -eq 1 ]; then
	install_temp=$SKILLHUB_BIN_DIR/.skillhub.new.$$
	backup=$SKILLHUB_BIN_DIR/.skillhub.rollback.$$
	marker_temp=$SKILLHUB_BIN_DIR/.skillhub-managed.tmp.$$
	marker_backup=$SKILLHUB_TEMP_DIR/managed-marker.backup
	trap 'rm -f "$install_temp" "$marker_temp"; skillhub_cleanup_temp' EXIT HUP INT TERM

	cp "$SKILLHUB_CANDIDATE" "$install_temp" || skillhub_error "Could not stage the upgraded binary."
	chmod 755 "$install_temp" || skillhub_error "Could not set executable permissions on the staged binary."
	cp -p "$SKILLHUB_TARGET" "$backup" || skillhub_error "Could not create rollback copy of the installed binary."
	cp "$SKILLHUB_MARKER" "$marker_backup" || skillhub_error "Could not preserve the installation marker."
	printf 'skillhub-managed-v1\nversion=%s\n' "$SKILLHUB_VERSION" > "$marker_temp" || skillhub_error "Could not stage the updated installation marker."
	chmod 600 "$marker_temp" || skillhub_error "Could not protect the updated installation marker."

	mv -f "$install_temp" "$SKILLHUB_TARGET" || skillhub_error "Could not atomically replace $SKILLHUB_TARGET"
	if ! "$SKILLHUB_TARGET" version >/dev/null 2>&1; then
		if mv -f "$backup" "$SKILLHUB_TARGET" && cp "$marker_backup" "$SKILLHUB_MARKER"; then
			skillhub_error "Upgraded binary failed its version check; restored the previous installation."
		fi
		skillhub_error "Upgraded binary failed its version check and automatic rollback failed. Recovery copy: $backup"
	fi
	if ! mv -f "$marker_temp" "$SKILLHUB_MARKER"; then
		if mv -f "$backup" "$SKILLHUB_TARGET" && cp "$marker_backup" "$SKILLHUB_MARKER"; then
			skillhub_error "Could not update the installation marker; restored the previous installation."
		fi
		skillhub_error "Could not update the installation marker and automatic rollback failed. Recovery copy: $backup"
	fi
	rm -f "$backup"
	trap 'skillhub_cleanup_temp' EXIT HUP INT TERM
	printf 'Upgraded skillhub to %s at %s\n' "$SKILLHUB_VERSION" "$SKILLHUB_TARGET"
else
	install_temp=$SKILLHUB_BIN_DIR/.skillhub.new.$$
	marker_temp=$SKILLHUB_BIN_DIR/.skillhub-managed.tmp.$$
	trap 'rm -f "$install_temp" "$marker_temp"; skillhub_cleanup_temp' EXIT HUP INT TERM
	cp "$SKILLHUB_CANDIDATE" "$install_temp" || skillhub_error "Could not stage the binary in $SKILLHUB_BIN_DIR"
	chmod 755 "$install_temp" || skillhub_error "Could not set executable permissions on the staged binary."
	printf 'skillhub-managed-v1\nversion=%s\n' "$SKILLHUB_VERSION" > "$marker_temp" || skillhub_error "Could not stage the installation marker."
	chmod 600 "$marker_temp" || skillhub_error "Could not protect the installation marker."

	mv "$install_temp" "$SKILLHUB_TARGET" || skillhub_error "Could not atomically install $SKILLHUB_TARGET"
	if ! mv "$marker_temp" "$SKILLHUB_MARKER"; then
		rm -f "$SKILLHUB_TARGET"
		skillhub_error "Could not atomically write $SKILLHUB_MARKER"
	fi
	trap 'skillhub_cleanup_temp' EXIT HUP INT TERM
	printf 'Installed skillhub %s to %s\n' "$SKILLHUB_VERSION" "$SKILLHUB_TARGET"
fi

skillhub_configure_path
printf 'Next step:\n  skillhub init ~/skillhub --yes\n'
