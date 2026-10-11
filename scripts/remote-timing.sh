#!/bin/sh
# Time one skill from a hub on another machine, the way an agent gets it.
#
# Run this on machine A, the machine that uses the hub. It starts the same
# stdio proxy your agent host starts, resolves a task, gets the skill twice and
# prints how long each step took: the first get downloads the skill, the
# second one is served from the cache on this machine.
#
# Usage:
#   scripts/remote-timing.sh --skill <id> [--task <text>] [--project <dir>]
#   scripts/remote-timing.sh --skill <id> --hub <url> --token-file <path> [--task <text>]
#
# Without --hub it uses the Skill Hub entry `skillhub connect --hub` wrote for
# Claude Code: <dir>/.mcp.json (default: the current directory), else
# ~/.claude.json. With --hub it starts `skillhub mcp proxy` itself (the
# skillhub on your PATH, or SKILLHUB_BIN). The token is read only by the
# proxy, from the token file or SKILLHUB_HUB_TOKEN; this script never reads
# or prints it.
#
# The proxy runs with a new, empty skill cache in a temporary directory, so
# the first get is a real download; your own cache is not touched, and the
# temporary directory is removed at the end. Nothing changes on the hub
# except the usual usage records of the calls.
#
# Needs python3 (for the MCP messages and the timer). POSIX shells on Linux
# and macOS; on Windows run it in WSL or Git Bash with python3.
set -eu

usage() {
	sed -n '9,11p' "$0" | sed 's/^# //'
	exit 2
}

skill='' task='' project='.' hub='' token_file=''
while [ $# -gt 0 ]; do
	case "$1" in
	--skill) [ $# -ge 2 ] || usage; skill=$2; shift 2 ;;
	--task) [ $# -ge 2 ] || usage; task=$2; shift 2 ;;
	--project) [ $# -ge 2 ] || usage; project=$2; shift 2 ;;
	--hub) [ $# -ge 2 ] || usage; hub=$2; shift 2 ;;
	--token-file) [ $# -ge 2 ] || usage; token_file=$2; shift 2 ;;
	-h | --help) usage ;;
	*) echo "Unknown argument: $1" >&2; usage ;;
	esac
done
if [ -z "$skill" ]; then
	echo "Give the skill to time with --skill <id>." >&2
	usage
fi
if ! command -v python3 >/dev/null 2>&1; then
	echo "This script needs python3 for the MCP messages and the timer; install it and run the script again." >&2
	exit 2
fi

cache=$(mktemp -d "${TMPDIR:-/tmp}/skillhub-remote-timing.XXXXXX")
trap 'rm -rf "$cache"' EXIT INT TERM

TIMING_SKILL=$skill TIMING_TASK=$task TIMING_PROJECT=$project TIMING_HUB=$hub \
	TIMING_TOKEN_FILE=$token_file TIMING_CACHE=$cache TIMING_BIN=${SKILLHUB_BIN:-skillhub} \
	python3 - <<'PYTHON'
import json
import os
import queue
import subprocess
import sys
import threading
import time

skill = os.environ["TIMING_SKILL"]
task = os.environ["TIMING_TASK"] or "use the " + skill + " skill"
cache = os.environ["TIMING_CACHE"]


process = None


def fail(message):
    print("ERROR: " + message, file=sys.stderr)
    if process is not None and process.poll() is None:
        # Stop the proxy before the shell removes its temporary cache.
        process.kill()
        process.wait()
    sys.exit(1)


def entry_command():
    """The proxy command: from --hub, or from the Claude Code entry connect wrote."""
    hub = os.environ["TIMING_HUB"]
    if hub:
        command = [os.environ["TIMING_BIN"], "mcp", "proxy", "--hub", hub]
        if os.environ["TIMING_TOKEN_FILE"]:
            command += ["--token-file", os.environ["TIMING_TOKEN_FILE"]]
        return command, hub, "--hub " + hub
    candidates = [
        os.path.join(os.environ["TIMING_PROJECT"], ".mcp.json"),
        os.path.join(os.path.expanduser("~"), ".claude.json"),
    ]
    for path in candidates:
        try:
            with open(path, encoding="utf-8") as handle:
                servers = json.load(handle).get("mcpServers", {})
        except (OSError, ValueError):
            continue
        entry = servers.get("skillhub")
        if not entry:
            continue
        args = entry.get("args", [])
        if "proxy" not in args or "--hub" not in args:
            fail(path + " connects to a hub on this machine; this timing is for a remote hub. "
                 "Run `skillhub connect --hub <url> --token-file <path> --yes` first, or pass --hub and --token-file.")
        return [entry["command"]] + args, args[args.index("--hub") + 1], path
    fail("No Skill Hub entry for a remote hub was found in " + " or ".join(candidates) +
         ". Pass --project <dir>, or --hub <url> --token-file <path>.")


command, hub_url, origin = entry_command()
environment = dict(os.environ, XDG_STATE_HOME=cache)
try:
    process = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, env=environment, text=True, bufsize=1)
except OSError as error:
    fail("cannot start " + command[0] + ": " + str(error))

answers = queue.Queue()
errors = []


def read_answers():
    for line in process.stdout:
        try:
            answers.put(json.loads(line))
        except ValueError:
            pass
    answers.put(None)


def read_errors():
    for line in process.stderr:
        errors.append(line.rstrip("\n"))


threading.Thread(target=read_answers, daemon=True).start()
threading.Thread(target=read_errors, daemon=True).start()
next_id = 0


def request(method, params, timeout=120):
    global next_id
    next_id += 1
    started = time.perf_counter()
    process.stdin.write(json.dumps({"jsonrpc": "2.0", "id": next_id, "method": method, "params": params}) + "\n")
    process.stdin.flush()
    deadline = started + timeout
    while True:
        try:
            answer = answers.get(timeout=max(0.0, deadline - time.perf_counter()))
        except queue.Empty:
            fail(method + " got no answer within " + str(timeout) + " s.")
        if answer is None:
            fail("the proxy stopped:\n" + "\n".join(errors))
        if answer.get("id") == next_id:
            elapsed = (time.perf_counter() - started) * 1000
            if "error" in answer:
                fail(method + ": " + answer["error"].get("message", str(answer["error"])))
            return answer["result"], elapsed


def tool(name, arguments):
    result, elapsed = request("tools/call", {"name": name, "arguments": arguments})
    structured = result.get("structuredContent") or {}
    if result.get("isError"):
        problem = structured.get("error") or {}
        fail(name + ": " + problem.get("code", "error") + ": " + problem.get("message", json.dumps(result)) +
             ("\nWhat to do: " + problem["suggested_action"] if problem.get("suggested_action") else ""))
    return structured.get("result") or {}, elapsed


_, start_ms = request("initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
                                     "clientInfo": {"name": "skillhub-remote-timing", "version": "1"}})
process.stdin.write(json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}) + "\n")
process.stdin.flush()

resolved, resolve_ms = tool("skill_resolve", {"schema_version": "1", "request_id": "remote-timing",
                                              "task": {"description": task, "scope": "multi_step"}})
resolution = resolved.get("resolution", {})
primary = (resolution.get("primary") or {}).get("id")
picked = [primary] + [item.get("id") for item in resolution.get("supporting") or []]
resolve_note = "recommends " + primary if primary else "no skill fits this task"
if skill not in picked:
    resolve_note += " (not " + skill + "; the get below still works)"

rows = [("start the proxy", start_ms, "ready"), ("skill_resolve", resolve_ms, resolve_note)]
labels = {"fetched": "downloaded from the hub", "hit": "served from the cache on this machine",
          "offline": "the hub could not be reached; cached copy used"}
for step in ("skill_get, first fetch", "skill_get, cached repeat"):
    got, elapsed = tool("skill_get", {"skill_id": skill})
    remote = got.get("remote") or {}
    local = got.get("local") or {}
    if not remote:
        note = "the hub answered without a skill folder (status " + local.get("status", "unknown") + ")"
    else:
        note = labels.get(remote.get("cache"), remote.get("cache", ""))
    rows.append((step, elapsed, note))

process.stdin.close()
try:
    process.wait(timeout=30)
except subprocess.TimeoutExpired:
    process.kill()

print("Skill Hub remote timing")
print("  Hub:     " + hub_url)
print("  Entry:   " + origin)
print("  Skill:   " + skill)
print("  Task:    " + task)
print("  Cache:   a new, empty cache for this run (removed afterwards)")
print()
width = max(len(row[0]) for row in rows)
print("  " + "Step".ljust(width) + "  " + "Time".rjust(9) + "  Result")
for name, elapsed, note in rows:
    print("  " + name.ljust(width) + "  " + ("%.1f ms" % elapsed).rjust(9) + "  " + note)
print()
print("Targets: first fetch within 500 ms on a LAN and within 2 s on a 100 ms link;")
print("cached repeat within 20 ms. The first call of a session also opens the")
print("connection to the hub, so skill_resolve pays a few extra round trips here.")
PYTHON
