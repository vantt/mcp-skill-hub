# shellcheck shell=bash
# Source this file to turn the current bash shell into a throwaway "demo machine":
#
#   . video/demos/sandbox.sh [DIR]
#
# DIR (default: $DEMO_SANDBOX, else ${TMPDIR:-/tmp}/skillhub-demo) holds everything the demo writes:
#   DIR/home     HOME, with XDG_CONFIG/DATA/STATE/CACHE_HOME and the per-user Skill Hub config inside it
#   DIR/bin      the skillhub binary under test, first on PATH, plus browser-opening stubs
#   DIR/run      XDG_RUNTIME_DIR, the opened-URL log and other run files
#   DIR/logs     raw (unsanitized) command output of the demo scripts
#   DIR/.private strings the sanitizer removes (real HOME, user, hostname, Git identity); mode 600
#
# The real HOME, ~/.claude, ~/.config and any hub of yours are never read or written once the file is
# sourced: HOME, every XDG directory, Git's global config and the Skill Hub config point into DIR.
#
# Environment:
#   SKILLHUB_BIN=<path>    copy this binary into DIR/bin instead of building one from the repository
#   SKILLHUB_BIN=release   install the latest release with the README's install.sh into DIR/bin (network)
#   DEMO_REBUILD=1         rebuild or reinstall even when DIR/bin/skillhub exists
#   DEMO_FRESH=1           wipe an existing sandbox in DIR first (only a folder this file created)
#   DEMO_OPEN_BROWSER=1    let `skillhub serve web` open a real browser (stubbed by default)
#
# Works with bash 3.2+ on Linux and macOS. It prints one line, the sandbox folder, to stderr.

_demo_sandbox_fail() {
	printf 'sandbox.sh: %s\n' "$*" >&2
	return 1
}

_demo_sandbox_main() {
	if [ -z "${BASH_VERSION:-}" ]; then
		printf 'sandbox.sh: source this file from bash\n' >&2
		return 1
	fi
	local here repo root marker real_home
	here=$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd) || return 1
	repo=$(cd -P "$here/../.." && pwd) || return 1
	root=${1:-${DEMO_SANDBOX:-${TMPDIR:-/tmp}/skillhub-demo}}
	[ "$root" = / ] && { _demo_sandbox_fail "refusing /: pick a throwaway folder"; return 1; }
	root=${root%/}
	marker=.skillhub-demo-sandbox

	mkdir -p "$root" || _demo_sandbox_fail "cannot create $root" || return 1
	root=$(cd -P "$root" && pwd) || return 1

	# Refuse folders that are not throwaway: the filesystem root, the real HOME or a parent of it,
	# and a non-empty folder this file did not create.
	real_home=${_DEMO_REAL_HOME:-$HOME}
	case "$root" in
	/ | "$real_home" | "$repo" | "$repo"/*) _demo_sandbox_fail "refusing $root: pick a throwaway folder, not your HOME or this repository" || return 1 ;;
	esac
	case "$real_home/" in
	"$root"/*) _demo_sandbox_fail "refusing $root: it contains your HOME" || return 1 ;;
	esac
	if [ ! -f "$root/$marker" ] && [ -n "$(ls -A "$root" 2>/dev/null)" ]; then
		_demo_sandbox_fail "refusing $root: not empty and not a demo sandbox" || return 1
	fi
	if [ "${DEMO_FRESH:-0}" = 1 ] && [ -f "$root/$marker" ]; then
		# Stop anything a previous run left behind in this sandbox before deleting it.
		if [ -f "$root/run/pids" ]; then
			while read -r pid; do
				# Only a skillhub process: a PID from an old run may belong to something else by now.
				case "$(ps -p "$pid" -o comm= 2>/dev/null)" in *skillhub*) kill "$pid" 2>/dev/null ;; esac
			done <"$root/run/pids"
		fi
		rm -rf "${root:?}/"* "$root/.private" || return 1
	fi
	touch "$root/$marker"

	local home=$root/home
	mkdir -p "$home/.config" "$home/.local/share" "$home/.local/state" "$home/.cache" \
		"$root/bin" "$root/run" "$root/logs" || return 1
	chmod 700 "$root/run"

	# Remember what must never appear in a transcript, before HOME changes. Only the first time:
	# sourcing again from inside the sandbox would record the sandbox itself.
	if [ ! -f "$root/.private" ] && [ -n "${_DEMO_PRIVATE:-}" ] && [ -f "$_DEMO_PRIVATE" ]; then
		# A sandbox made from inside another one (a second demo machine) inherits its list.
		(umask 077 && cp "$_DEMO_PRIVATE" "$root/.private") || return 1
	elif [ ! -f "$root/.private" ]; then
		(
			umask 077
			{
				printf 'REAL_HOME=%s\n' "$real_home"
				printf 'REAL_USER=%s\n' "${USER:-$(id -un 2>/dev/null)}"
				printf 'REAL_HOST=%s\n' "$(hostname 2>/dev/null)"
				printf 'REAL_HOST=%s\n' "$(hostname -s 2>/dev/null)"
				printf 'REAL_HOST=%s\n' "$(uname -n 2>/dev/null)"
				printf 'REAL_GIT_NAME=%s\n' "$(git config --global user.name 2>/dev/null)"
				printf 'REAL_GIT_EMAIL=%s\n' "$(git config --global user.email 2>/dev/null)"
			} >"$root/.private"
		) || return 1
	fi

	# The binary under test. Building happens before HOME changes so Go keeps its own caches.
	if [ ! -x "$root/bin/skillhub" ] || [ "${DEMO_REBUILD:-0}" = 1 ]; then
		case "${SKILLHUB_BIN:-}" in
		release) ;; # installed below, inside the sandbox
		"")
			command -v go >/dev/null 2>&1 || _demo_sandbox_fail "go not found: install Go or set SKILLHUB_BIN" || return 1
			if [ ! -f "$repo/internal/delivery/web/dist/index.html" ]; then
				printf 'sandbox.sh: the web UI is not built, so serve web shows no pages; run `make web-build` first\n' >&2
			fi
			local version commit pkg=github.com/vantt/skillhub-mcp/internal/version
			version=$(git -C "$repo" describe --tags --always 2>/dev/null || echo dev)
			commit=$(git -C "$repo" rev-parse --short HEAD 2>/dev/null || echo none)
			(cd "$repo" && go build -ldflags "-X $pkg.Version=${version#v} -X $pkg.Commit=$commit" \
				-o "$root/bin/skillhub" ./cmd/skillhub) || _demo_sandbox_fail "go build failed" || return 1
			;;
		*)
			[ -x "$SKILLHUB_BIN" ] || _demo_sandbox_fail "SKILLHUB_BIN is not an executable file: $SKILLHUB_BIN" || return 1
			cp "$SKILLHUB_BIN" "$root/bin/skillhub" || return 1
			;;
		esac
	fi

	local node_bin
	node_bin=$(command -v node 2>/dev/null || true)

	# From here on the shell is the demo machine.
	export _DEMO_REAL_HOME=$real_home _DEMO_PRIVATE=$root/.private
	export DEMO_SANDBOX=$root DEMO_REPO=$repo DEMO_DIR=$here
	export HOME=$home USERPROFILE=$home
	export XDG_CONFIG_HOME=$home/.config XDG_DATA_HOME=$home/.local/share
	export XDG_STATE_HOME=$home/.local/state XDG_CACHE_HOME=$home/.cache XDG_RUNTIME_DIR=$root/run
	export SKILLHUB_USER_CONFIG_DIR=$home/.config/skillhub
	export GIT_CONFIG_GLOBAL=$home/.gitconfig GIT_CONFIG_NOSYSTEM=1 GIT_CEILING_DIRECTORIES=$root
	export HISTFILE=$home/.bash_history LESSHISTFILE=-
	local sys=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin
	[ -d /opt/homebrew/bin ] && sys=/opt/homebrew/bin:$sys
	export PATH=$root/bin:$sys
	export PS1='$ ' PS2='> ' HOSTNAME=demo-host
	unset PROMPT_COMMAND PS0 GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE \
		GIT_AUTHOR_NAME GIT_AUTHOR_EMAIL GIT_COMMITTER_NAME GIT_COMMITTER_EMAIL SSH_AUTH_SOCK
	# No hub selection, agent session or credential of the real user leaks into the demo.
	local name
	for name in $(env | sed -n 's/^\([A-Za-z_][A-Za-z0-9_]*\)=.*/\1/p'); do
		case "$name" in
		SKILLHUB_USER_CONFIG_DIR) ;;
		SKILLHUB_* | CLAUDE* | ANTHROPIC_* | CODEX_* | GEMINI_* | OPENAI_* | *TOKEN* | *SECRET* | *PASSWORD* | *API_KEY*) unset "$name" ;;
		esac
	done

	if [ ! -f "$GIT_CONFIG_GLOBAL" ]; then
		printf '[user]\n\tname = Demo User\n\temail = demo@example.com\n[init]\n\tdefaultBranch = main\n' >"$GIT_CONFIG_GLOBAL"
	fi

	# Browser opening: stubs that only log the URL (it holds a session token) inside the sandbox.
	local stub
	for stub in xdg-open open; do
		if [ "${DEMO_OPEN_BROWSER:-0}" = 1 ]; then
			rm -f "$root/bin/$stub"
		else
			printf '#!/bin/sh\nprintf "%%s\\n" "$*" >>"%s/run/opened-urls.log"\n' "$root" >"$root/bin/$stub"
			chmod +x "$root/bin/$stub"
		fi
	done

	# Node, if installed, without putting its (often personal) folder on PATH: `skillhub doctor`
	# checks for it, and the distill-lab system skill runs with it.
	if [ ! -e "$root/bin/node" ] && [ -n "$node_bin" ]; then
		ln -s "$node_bin" "$root/bin/node"
	fi

	if [ ! -x "$root/bin/skillhub" ] && [ "${SKILLHUB_BIN:-}" = release ]; then
		command -v curl >/dev/null 2>&1 || _demo_sandbox_fail "curl not found" || return 1
		curl -fsSL https://github.com/vantt/skillhub-mcp/releases/latest/download/install.sh |
			SKILLHUB_INSTALL_DIR=$root/bin sh >"$root/logs/install.log" 2>&1 ||
			_demo_sandbox_fail "release install failed; see $root/logs/install.log" || return 1
	fi
	[ -x "$root/bin/skillhub" ] || _demo_sandbox_fail "no skillhub binary in $root/bin" || return 1

	cd "$HOME" || return 1
	printf 'Demo sandbox: %s\n' "$root" >&2
}

_demo_sandbox_main "$@"
