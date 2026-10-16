#!/usr/bin/env bash
# Record the web UI scenes: queue a skill for review (seed-review.sh), then record (record.mjs).
# Run video/demos/happy-path.sh first; both use the same sandbox.
#
#   video/demos/web/run.sh [SANDBOX_DIR]
set -eu
here=$(cd -P "$(dirname "${BASH_SOURCE[0]}")" && pwd)
export DEMO_SANDBOX=${1:-${DEMO_SANDBOX:-${TMPDIR:-/tmp}/skillhub-demo}}
"$here/seed-review.sh" "$DEMO_SANDBOX"
node "$here/record.mjs"
