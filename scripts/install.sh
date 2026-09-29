#!/bin/sh
# Install a prebuilt Skill Hub binary supplied by the release process.
set -eu

binary_path=${1:-}
install_dir=${SKILLHUB_INSTALL_DIR:-"$HOME/.local/bin"}

if [ -z "$binary_path" ]; then
	printf '%s\n' "skillhub install prototype: provide a path to a released skillhub binary." >&2
	printf '%s\n' "Example: SKILLHUB_INSTALL_DIR=~/.local/bin scripts/install.sh ./skillhub" >&2
	exit 0
fi

if [ ! -f "$binary_path" ]; then
	printf 'ERROR: Release artifact not found: %s\n' "$binary_path" >&2
	exit 1
fi

if [ ! -x "$binary_path" ]; then
	printf 'ERROR: Release artifact is not executable: %s\n' "$binary_path" >&2
	exit 1
fi

if [ -e "$install_dir/skillhub" ]; then
	printf 'ERROR: Refusing to overwrite existing installation: %s\n' "$install_dir/skillhub" >&2
	printf '%s\n' "FIX: Remove or rename the existing binary, then retry." >&2
	exit 1
fi

mkdir -p "$install_dir"
cp "$binary_path" "$install_dir/skillhub"
printf 'Installed skillhub to %s\n' "$install_dir/skillhub"
