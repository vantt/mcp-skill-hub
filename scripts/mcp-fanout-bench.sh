#!/usr/bin/env bash
# Start N `skillhub mcp serve` processes on one temp workspace, call
# skill_resolve in each at a fixed rate, and print total memory, latency and
# telemetry events written against expected.
#
# Usage: scripts/mcp-fanout-bench.sh [rate-per-process ...]   (default: 1 10)
#   A rate of 0 means back-to-back calls (load far beyond real use; loss is
#   reported but does not fail the run).
#
# Environment (optional):
#   SKILLHUB_FANOUT_PROCS     processes (default 20)
#   SKILLHUB_FANOUT_DURATION  duration per scenario (default 30s)
#   SKILLHUB_FANOUT_IDLE      quiet time after the calls before memory is sampled
#                             again (default 45s; agent sessions are mostly idle)
#   SKILLHUB_MEMORY_SKILLS    skills in the fixture catalog (default 32)
#   SKILLHUB_MEMORY_BINARY    serve this skillhub binary instead of the worktree's
#   SKILLHUB_MEMORY_ENV       extra environment for every served process,
#                             for example "GOMEMLIMIT=16MiB GOGC=50"
#
# The Go test driver isolates HOME and every XDG_* directory and uses only
# temp directories; it never reads or writes a real hub. Every process it
# starts is stopped by closing its stdin. Resident memory is sampled from
# /proc on Linux and from ps on macOS; Windows is not supported.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

rates=("$@")
if [ "${#rates[@]}" -eq 0 ]; then
	rates=(1 10)
fi

status=0
for rate in "${rates[@]}"; do
	case "$rate" in
	'' | *[!0-9]*)
		echo "rate must be a non-negative integer, got '$rate'" >&2
		exit 2
		;;
	esac
	echo "== fan-out: ${SKILLHUB_FANOUT_PROCS:-20} processes, rate ${rate}/s per process, ${SKILLHUB_FANOUT_DURATION:-30s}"
	if ! SKILLHUB_PERF=1 SKILLHUB_FANOUT_RATE="$rate" SKILLHUB_FANOUT_IDLE="${SKILLHUB_FANOUT_IDLE:-45s}" \
		go test -count=1 -timeout 30m -run '^TestMCPFanoutBench$' -v ./internal/delivery/mcpserver/ |
		grep -E '^\s+memory_budget_test\.go:[0-9]+: |^(--- |FAIL|ok)'; then
		status=1
	fi
done
exit "$status"
