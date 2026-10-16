#!/usr/bin/env bash
# Put one skill under "Waiting for review" in the demo hub, offline, the way a second machine does
# (docs/curating-skills.md "From another machine"):
#
#   video/demos/web/seed-review.sh [SANDBOX_DIR]
#
# Run it after happy-path.sh, on the same sandbox. It starts `skillhub serve` on loopback, makes an
# invite, connects a second demo machine (SANDBOX_DIR/peer) with it, sends the release-checklist
# fixture with `skill commit`, and stops the server. Transcripts of those steps go to
# $DEMO_OUT/remote/ (raw material for the "hub on another computer" scene).
#
# The hub names a submission after the sending machine's hostname; the script sends it with
# `--label $DEMO_PEER_LABEL` (default "laptop") so the real hostname never appears on screen.
#
# Environment: DEMO_HUB_PORT (default 17420), DEMO_PEER_LABEL, DEMO_OUT.
set -u

here=$(cd -P "$(dirname "${BASH_SOURCE[0]}")/.." && pwd) || exit 1
root=${1:-${DEMO_SANDBOX:-${TMPDIR:-/tmp}/skillhub-demo}}
[ -f "$root/.skillhub-demo-sandbox" ] && [ -d "$root/home/skill-hub" ] ||
	{ printf 'seed-review.sh: no demo hub in %s; run video/demos/happy-path.sh first\n' "$root" >&2; exit 1; }
# shellcheck source=video/demos/sandbox.sh
. "$here/sandbox.sh" "$root" 2>/dev/null || exit 1
# shellcheck source=video/demos/lib.sh
. "$here/lib.sh"

port=${DEMO_HUB_PORT:-17420}
label=${DEMO_PEER_LABEL:-laptop}
DEMO_OUT=$DEMO_OUT/remote
mkdir -p "$DEMO_OUT" || demo_die "cannot create $DEMO_OUT"
rm -f "$DEMO_OUT"/[0-9][0-9]-*.txt
command -v perl >/dev/null 2>&1 || demo_die "perl is required"

port_open() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; }
if port_open "$port"; then
	demo_die "port $port is in use; stop what listens there (lsof -i :$port) or set DEMO_HUB_PORT"
fi

# The hub machine: serve on loopback only, and stop it whatever happens next.
cd "$HOME/skill-hub" || exit 1
skillhub serve --addr "127.0.0.1:$port" >"$DEMO_SANDBOX/logs/serve-mcp.log" 2>&1 &
serve_pid=$!
echo "$serve_pid" >>"$DEMO_SANDBOX/run/pids"
stop_serve() {
	kill "$serve_pid" 2>/dev/null && wait "$serve_pid" 2>/dev/null
	grep -vx "$serve_pid" "$DEMO_SANDBOX/run/pids" >"$DEMO_SANDBOX/run/pids.new" 2>/dev/null
	mv -f "$DEMO_SANDBOX/run/pids.new" "$DEMO_SANDBOX/run/pids"
}
trap stop_serve EXIT
for _ in $(seq 1 50); do port_open "$port" && break; sleep 0.2; done
port_open "$port" || demo_die "skillhub serve did not start; see $DEMO_SANDBOX/logs/serve-mcp.log"

demo_step token-invite "skillhub token invite --addr 127.0.0.1:$port" ''
invite=$(grep -o -- '--invite [A-Z0-9-]*' "$DEMO_SANDBOX/logs/$(printf %02d "$DEMO_STEP_NO")-token-invite.log" | head -1)
[ -n "$invite" ] || demo_die "token invite printed no code"

# The second machine: its own sandbox inside this one, same binary, the fixture skill in a project.
(
	SKILLHUB_BIN=$DEMO_SANDBOX/bin/skillhub
	# shellcheck source=video/demos/sandbox.sh
	. "$here/sandbox.sh" "$root/peer" 2>/dev/null || exit 1
	mkdir -p ~/my-project/.claude/skills && cd ~/my-project || exit 1
	git init -q . || exit 1
	cp -R "$here/fixtures/release-checklist" .claude/skills/release-checklist || exit 1
	demo_step peer-connect "skillhub connect --hub http://127.0.0.1:$port ${invite} -g"
	demo_step peer-doctor 'skillhub doctor'
	demo_step peer-send "skillhub skill commit release-checklist --label $(printf %q "$label")" y
) || exit 1
DEMO_STEP_NO=$((DEMO_STEP_NO + 3))

found=0
for meta in "$HOME"/skill-hub/runtime/submissions/SUB-*/submission.json; do
	[ -f "$meta" ] && found=1
done
[ "$found" = 1 ] || demo_die "no submission arrived at the hub"

demo_step hub-status 'skillhub status'
demo_check_clean "$DEMO_OUT" || demo_die "a transcript still holds private data; see above"
printf 'release-checklist waits for review in %s/home/skill-hub\n' "$DEMO_SANDBOX" >&2
