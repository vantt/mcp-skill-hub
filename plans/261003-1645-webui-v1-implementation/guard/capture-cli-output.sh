#!/usr/bin/env bash
# Captures normalized CLI output (human and --json) for a fixed scenario so the
# before/after state of the CLI can be compared mechanically and by a human.
#
# Usage: capture-cli-output.sh <output-dir>
#
# Each case writes <case>.human.txt and/or <case>.json.txt containing:
#   exit=<code>
#   --- stdout ---
#   <normalized stdout>
#   --- stderr ---
#   <normalized stderr>
# Volatile values (temp paths, timestamps, digests, generated IDs, commits) are
# replaced by placeholders so two runs on the same code produce identical files.
set -uo pipefail

OUT="${1:?usage: capture-cli-output.sh <output-dir>}"
PLAN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT="$(git -C "$PLAN_DIR" rev-parse --show-toplevel)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"
rm -f "$OUT"/*.txt
(cd "$ROOT" && go build -o "$WORK/skillhub" ./cmd/skillhub) || { echo "build failed" >&2; exit 2; }

WS="$WORK/workspace"
export HOME="$WORK/home"
export SKILLHUB_WORKSPACE="$WS"
export GIT_AUTHOR_NAME=guard GIT_AUTHOR_EMAIL=guard@example.invalid GIT_COMMITTER_NAME=guard GIT_COMMITTER_EMAIL=guard@example.invalid
mkdir -p "$HOME"
cd "$WORK" || exit 2

normalize() {
	sed -E \
		-e "s#$WORK#<TMP>#g" \
		-e 's#[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})#<TIME>#g' \
		-e 's#sha256:[0-9a-f]{64}#<DIGEST>#g' \
		-e 's#\b[0-9a-f]{40}\b#<COMMIT>#g' \
		-e 's#\b[0-9a-f]{64}\b#<HEX64>#g' \
		-e 's#\b(OP|RUN|APP|INC|PROP|PRP|REC|GEN|SNAP|OUT|CAND|EVT|RES)-[A-Za-z0-9_-]+#\1-<ID>#g' \
		-e 's#\bgen-[A-Za-z0-9_-]+#gen-<ID>#g' \
		-e 's#\b[0-9]+ms\b#<MS>#g' \
		-e 's#/[0-9]{4}/[0-9]{2}/#/<YYYY>/<MM>/#g'
}

run_case() { # run_case <name> <mode human|json> <args...>
	local name="$1" mode="$2"; shift 2
	local stdout stderr code
	if [[ "$mode" == "json" ]]; then set -- "$@" --json; fi
	stdout="$("$WORK/skillhub" "$@" 2>"$WORK/stderr")"
	code=$?
	stderr="$(cat "$WORK/stderr")"
	{
		echo "exit=$code"
		echo "--- stdout ---"
		printf '%s\n' "$stdout" | normalize
		echo "--- stderr ---"
		printf '%s\n' "$stderr" | normalize
	} >"$OUT/$name.$mode.txt"
}

both() { local name="$1"; shift; run_case "$name" human "$@"; run_case "$name" json "$@"; }

cat >"$WORK/long-skill.md" <<'MD'
# Long skill

Use this skill when a change needs a careful, multi-step review that covers correctness, regressions, security posture, documentation impact, and rollout risk across several modules, because a single pass routinely misses cross-module contracts and the reviewer must record evidence for each finding before recommending a fix.

## Steps

1. Read the diff.
2. Trace each changed contract to its callers.
MD

"$WORK/skillhub" init "$WS" --yes >/dev/null 2>&1 || { echo "init failed" >&2; exit 2; }
git -C "$WS" add -A >/dev/null 2>&1 && git -C "$WS" commit -qm init >/dev/null 2>&1

LONG_DESCRIPTION="Review a multi-module change for correctness, regressions, security posture, documentation impact and rollout risk, recording evidence for every finding before recommending a fix, and never approving a change whose cross-module contracts were not traced to their callers."

both status-empty status
both skill-list-empty skill list
both source-list-empty source list
both skill-create-missing-args skill create
both skill-create-preview skill create long-review --collection core --name "Long Review" --description "$LONG_DESCRIPTION" --content-file "$WORK/long-skill.md"
"$WORK/skillhub" skill create long-review --collection core --name "Long Review" --description "$LONG_DESCRIPTION" --content-file "$WORK/long-skill.md" --yes >/dev/null 2>&1
both skill-list-one-draft skill list
both skill-show-draft skill show long-review
both skill-review-draft skill review long-review
both skill-activate-missing-fields skill activate long-review --yes
both skill-review-unknown skill review does-not-exist
both status-dirty status
both diff-dirty diff
both validate validate

echo "captured $(ls "$OUT" | wc -l | tr -d ' ') files into $OUT"
