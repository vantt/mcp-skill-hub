#!/usr/bin/env bash
# Record the terminal scenes of the intro video with VHS, in order, from the repository root:
#
#   video/demos/tapes/render.sh [scene...]     scenes: install create-hub connect first-skill remote
#
# Needs vhs, ttyd and ffmpeg on PATH, and the binary under test in SKILLHUB_BIN (or Go, to build
# one; see video/demos/README.md). Videos go to video/recordings/tapes/. Each scene uses its own
# throwaway sandbox under /tmp; create-hub, connect and first-skill share /tmp/skillhub-tape-a and
# must run in that order.
#
# The remote scene prepares a hub in /tmp/skillhub-tape-b off camera and runs `skillhub serve` there
# with its defaults (every interface, port 7420), so the visible `skillhub token invite` is the
# README's exact line. It refuses to start when port 7420 is busy: stop your own hub server first.
# The invite lists this computer's network addresses: check the video before you use it.
set -eu

here=$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo=$(cd -P "$here/../../.." && pwd)
cd "$repo"
out=video/recordings/tapes
mkdir -p "$out"

for tool in vhs ttyd ffmpeg; do
	command -v "$tool" >/dev/null 2>&1 || { printf 'render.sh: %s not found on PATH\n' "$tool" >&2; exit 1; }
done

port_open() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }

remote() {
	port_open 7420 && { printf 'render.sh: port 7420 is in use; stop the hub server that listens there\n' >&2; return 1; }
	local pidfile=/tmp/skillhub-tape-b/run/serve.pid
	# The hub computer, off camera: a hub with one active skill, connected, serving.
	(
		DEMO_FRESH=1 . video/demos/sandbox.sh /tmp/skillhub-tape-b 2>/dev/null
		. $DEMO_DIR/tapes/helpers.sh
		mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init >/dev/null
		skillhub connect -g >/dev/null
		cd ~/skill-hub
		skillhub serve >"$DEMO_SANDBOX/logs/serve.log" 2>&1 &
		echo $! >"$pidfile"
		echo $! >>"$DEMO_SANDBOX/run/pids"
	)
	# Stop the server however the script ends: a failed vhs run exits through set -e.
	trap 'kill "$(cat '"$pidfile"')" 2>/dev/null || true' EXIT
	for _ in $(seq 1 50); do port_open 7420 && break; sleep 0.2; done
	port_open 7420 || { printf 'render.sh: skillhub serve did not start; see /tmp/skillhub-tape-b/logs/serve.log\n' >&2; return 1; }

	vhs "$here/05-hub-invite.tape"
	# Take the first line the invite offers for a network address (not loopback, which needs a tunnel).
	local line
	line=$(grep -oE 'skillhub connect --hub http://[0-9.]+:7420 --invite [A-Z0-9-]+' "$out/05-hub-invite.txt" |
		grep -v '127\.0\.0\.1' | head -1)
	[ -n "$line" ] || { printf 'render.sh: the invite printed no connect line for a network address\n' >&2; return 1; }
	sed "s|@CONNECT_LINE@|$line|" "$here/06-agent-connect.tape.in" >"$here/06-agent-connect.tape"
	vhs "$here/06-agent-connect.tape"
}

scenes=${*:-install create-hub connect first-skill remote}
for scene in $scenes; do
	printf '==> %s\n' "$scene"
	case "$scene" in
	install) vhs "$here/01-install.tape" ;;
	create-hub) vhs "$here/02-create-hub.tape" ;;
	connect) vhs "$here/03-connect.tape" ;;
	first-skill) vhs "$here/04-first-skill.tape" ;;
	remote) remote ;;
	*) printf 'render.sh: unknown scene %s\n' "$scene" >&2; exit 2 ;;
	esac
done
