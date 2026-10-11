#!/bin/sh
# Measure resolving and getting skills over stdio, loopback HTTP and a
# simulated slow link, and fail when a budget of the remote hub is missed
# (the budgets are listed in docs/design/remote-hub.md).
#
# Usage: scripts/remote-bench.sh        (or: make remote-bench)
#
# Environment (all optional):
#   REMOTE_HUB              Git repository of a hub to measure. It is cloned
#                           into a temp directory and only the clone is
#                           touched. Unset: fixture skills shaped like
#                           test-audit and herdr-cook-plan.
#   REMOTE_BENCH_OUT        File that receives the report (default: a new
#                           file in a temp directory).
#   SKILLHUB_REMOTE_SKILLS  Comma-separated skill IDs (default
#                           test-audit,herdr-cook-plan).
#   SKILLHUB_REMOTE_RUNS    Agent sessions per transport (default 20).
#   SKILLHUB_REMOTE_WARM    Warm skill_resolve calls per session (default 10).
#   SKILLHUB_REMOTE_RTT     Round trip of the simulated link (default 100ms).
#
# The Go test driver isolates HOME and every XDG_* directory, binds only
# 127.0.0.1 ports chosen by the system, and stops every process it starts.
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

work=$(mktemp -d "${TMPDIR:-/tmp}/skillhub-remote-bench.XXXXXX")
report=${REMOTE_BENCH_OUT:-$work/remote-bench.txt}
log=$work/go-test.log

echo "Running the remote hub benchmark (about a minute)..."
status=0
SKILLHUB_PERF=1 SKILLHUB_REMOTE_BENCH_REPORT="$report" SKILLHUB_REMOTE_HUB="${REMOTE_HUB:-}" \
	go test -count=1 -timeout 30m -run '^TestRemoteBudgetBench$' ./internal/delivery/mcpserver/ >"$log" 2>&1 || status=$?

if [ -s "$report" ]; then
	echo
	cat "$report"
	echo "Report: $report"
fi
if [ "$status" -ne 0 ]; then
	echo
	if grep -q 'budget missed:' "$log"; then
		echo "FAILED: these budgets were missed:"
		grep 'budget missed:' "$log" | sed 's/^.*budget missed: /  /'
	else
		echo "FAILED: the benchmark did not finish. Go test output:"
		cat "$log"
	fi
	exit "$status"
fi
rm -f "$log"
if [ "$report" != "$work/remote-bench.txt" ]; then
	rmdir "$work" 2>/dev/null || true
fi
echo "All budgets met."
