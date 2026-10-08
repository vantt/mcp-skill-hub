#!/usr/bin/env bash
# Mechanical guard for plan 261003-1645-webui-v1-implementation.
#
# Usage:
#   guard.sh baseline        # planner only: freeze baseline metrics
#   guard.sh check <phase>   # executor: run at the end of every phase, phase = 0..6
#
# Every check prints a PASS or FAIL line. The last line is either
#   GUARD RESULT: PASS (phase N)
# or
#   GUARD RESULT: FAIL (phase N) - <count> failure(s)
# The script exits 0 only on PASS. Executors must not edit anything under guard/.
set -uo pipefail

PLAN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT="$(git -C "$PLAN_DIR" rev-parse --show-toplevel)"
GUARD_DIR="$PLAN_DIR/guard"
BASE_DIR="$GUARD_DIR/baseline"
PLAN_REL="${PLAN_DIR#"$ROOT"/}"
cd "$ROOT" || exit 2

FAILURES=0
pass() { printf 'PASS  %s\n' "$1"; }
fail() { printf 'FAIL  %s\n' "$1"; FAILURES=$((FAILURES + 1)); }
check() { # check "<description>" <command...>
	local description="$1"; shift
	if "$@" >/dev/null 2>&1; then pass "$description"; else fail "$description"; fi
}

# ---------- collectors ----------
non_test() { ls "$@" 2>/dev/null | grep -v '_test\.go$'; }
collect_testfuncs() {
	grep -rhoE '^func (Test|Fuzz|Benchmark)[A-Za-z0-9_]+' --include='*_test.go' internal cmd | sed 's/^func //' | sort
}
collect_skips() {
	grep -rnE '\b[tbf]\.Skip(f|Now)?\(' --include='*_test.go' internal cmd | wc -l | tr -d ' '
}
collect_json_tags() {
	grep -rhoE 'json:"[^"]+"' --include='*.go' --exclude='*_test.go' internal | sort
}
collect_mcp_tools() {
	grep -hoE 'Name:[[:space:]]+"[a-z]+(_[a-z]+)+"' $(non_test internal/delivery/mcpserver/*.go) | sed -E 's/Name:[[:space:]]+//' | sort
}
count_assertions() { # count_assertions <files...> -> total t.Fatal/t.Error/strings.Contains lines
	local total=0 file
	for file in "$@"; do
		[[ -f "$file" ]] && total=$((total + $(grep -cE 'strings\.Contains\(|t\.(Fatalf|Errorf|Fatal|Error)\(' "$file")))
	done
	echo "$total"
}
collect_cli_assertions_by_file() {
	local file
	for file in internal/delivery/cli/*_test.go; do printf '%s %s\n' "$file" "$(count_assertions "$file")"; done
}
collect_cli_exit_assertions() {
	cat internal/delivery/cli/*_test.go | grep -oE '\b(code|exitCode|exit) ?(!=|==) ?[0-9]+' | sed -E 's/ //g' | sort | uniq -c | sed -E 's/^ +//'
}
MCP_PAGING_TESTS=(internal/delivery/mcpserver/server_test.go internal/delivery/mcpserver/hardening_test.go)

# ---------- baseline ----------
if [[ "${1:-}" == "baseline" ]]; then
	mkdir -p "$BASE_DIR"
	git rev-parse HEAD >"$BASE_DIR/base_commit.txt"
	collect_testfuncs >"$BASE_DIR/testfuncs.txt"
	collect_skips >"$BASE_DIR/skips.txt"
	collect_json_tags >"$BASE_DIR/json_tags.txt"
	collect_mcp_tools >"$BASE_DIR/mcp_tools.txt"
	collect_cli_assertions_by_file >"$BASE_DIR/cli_assertions_by_file.txt"
	collect_cli_exit_assertions >"$BASE_DIR/cli_exit_assertions.txt"
	count_assertions "${MCP_PAGING_TESTS[@]}" >"$BASE_DIR/mcp_paging_assertions.txt"
	git ls-files -- '*_test.go' | sort >"$BASE_DIR/test_files.txt"
	bash "$GUARD_DIR/capture-cli-output.sh" "$BASE_DIR/cli-before" || { echo "capture failed" >&2; exit 2; }
	(cd "$GUARD_DIR" && {
		sha256sum guard.sh capture-cli-output.sh
		find baseline -type f ! -name guard.sha256 | sort | xargs sha256sum
	} >baseline/guard.sha256)
	echo "baseline written to $BASE_DIR (commit $(cat "$BASE_DIR/base_commit.txt"))"
	exit 0
fi

if [[ "${1:-}" != "check" || -z "${2:-}" || ! "${2:-}" =~ ^[0-6]$ ]]; then
	echo "usage: guard.sh check <phase 0..6>" >&2
	exit 2
fi
PHASE="$2"
BASE_COMMIT="$(cat "$BASE_DIR/base_commit.txt")"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

GUARD_COMMIT="$(git log -1 --format=%H -- "$PLAN_REL/guard" 2>/dev/null)"
echo "== Guard phase $PHASE =="
echo "== Baseline commit: $BASE_COMMIT | Guard commit: ${GUARD_COMMIT:-none} =="
echo "== The user compares both hashes with the ones recorded at handover. =="

# ---------- guard integrity ----------
if (cd "$GUARD_DIR" && sha256sum -c --quiet baseline/guard.sha256) >/dev/null 2>&1; then pass "guard files match pinned checksums"; else fail "guard files do not match pinned checksums"; fi
if [[ -n "$GUARD_COMMIT" ]] && git diff --quiet "$GUARD_COMMIT" -- "$PLAN_REL/guard" && [[ -z "$(git ls-files --others --exclude-standard -- "$PLAN_REL/guard")" ]]; then
	pass "guard directory is committed and has no uncommitted or untracked changes"
else
	fail "guard directory is not committed, or has uncommitted/untracked changes"
fi

# ---------- Go checks (every phase) ----------
check "go build ./... exits 0" go build ./...
check "go vet ./... exits 0" go vet ./...
if go test -count=1 ./... >"$TMP/test.log" 2>&1; then pass "go test -count=1 ./... exits 0"; else fail "go test -count=1 ./... exits 0 (see output below)"; tail -40 "$TMP/test.log"; fi
if make lint LINT_BASE="$BASE_COMMIT" >"$TMP/lint.log" 2>&1; then pass "golangci-lint reports no new issues since the baseline commit"; else fail "golangci-lint reports new issues (see output below)"; tail -40 "$TMP/lint.log"; fi
UNFORMATTED="$(gofmt -l cmd internal schemas 2>/dev/null)"
if [[ -z "$UNFORMATTED" ]]; then pass "gofmt -l reports no files"; else fail "gofmt -l reports: $UNFORMATTED"; fi
if go list ./... 2>/dev/null | grep -qE 'mcp-skill-hub/web(/|$)'; then fail "go list ./... includes the web/ tree (web/go.mod guard missing)"; else pass "go list ./... excludes the web/ tree"; fi

collect_testfuncs >"$TMP/testfuncs.txt"
MISSING_TESTS="$(comm -23 "$BASE_DIR/testfuncs.txt" "$TMP/testfuncs.txt")"
if [[ -z "$MISSING_TESTS" ]]; then pass "no baseline test function was removed or renamed"; else fail "baseline test functions missing: $(echo "$MISSING_TESTS" | tr '\n' ' ')"; fi
SKIPS="$(collect_skips)"
if [[ "$SKIPS" == "$(cat "$BASE_DIR/skips.txt")" ]]; then pass "t.Skip count unchanged ($SKIPS)"; else fail "t.Skip count changed: baseline $(cat "$BASE_DIR/skips.txt"), now $SKIPS"; fi
collect_json_tags >"$TMP/json_tags.txt"
REMOVED_TAGS="$(comm -23 "$BASE_DIR/json_tags.txt" "$TMP/json_tags.txt")"
if [[ -z "$REMOVED_TAGS" ]]; then pass "no JSON field tag was removed or renamed"; else fail "JSON tags removed/renamed: $(echo "$REMOVED_TAGS" | sort -u | tr '\n' ' ')"; fi
collect_mcp_tools >"$TMP/mcp_tools.txt"
if diff -q "$BASE_DIR/mcp_tools.txt" "$TMP/mcp_tools.txt" >/dev/null; then pass "MCP tool name set unchanged"; else fail "MCP tool name set changed"; fi

{
	git diff "$BASE_COMMIT" -U0 -- '*.go' | grep -E '^\+' | grep -vE '^\+\+\+'
	git ls-files --others --exclude-standard -- '*.go' | xargs -r cat
} >"$TMP/added.txt"
FORBIDDEN="$(grep -nE '\b[tbf]\.Skip(f|Now)?\(|//[[:space:]]*nolint|//go:build[[:space:]]+ignore|\b(TODO|FIXME|XXX|HACK)\b|not implemented|testing\.Testing\(\)|testing\.Short\(\)|func TestMain\(' "$TMP/added.txt" || true)"
if [[ -z "$FORBIDDEN" ]]; then pass "no forbidden patterns in added Go code"; else fail "forbidden patterns in added Go code: $(echo "$FORBIDDEN" | head -5 | tr '\n' ' ')"; fi
EMPTY_CONTAINS="$(grep -nE 'strings\.Contains\([^,]+, *""\)' "$TMP/added.txt" || true)"
if [[ -z "$EMPTY_CONTAINS" ]]; then pass "no added strings.Contains(x, \"\") assertions"; else fail "vacuous strings.Contains(x, \"\") added: $EMPTY_CONTAINS"; fi

DELETED_TESTS="$(git diff "$BASE_COMMIT" --name-only --diff-filter=D -- '*_test.go')"
if [[ -z "$DELETED_TESTS" ]]; then pass "no test file deleted"; else fail "test files deleted: $DELETED_TESTS"; fi
MODIFIED_TESTS="$( { git diff "$BASE_COMMIT" --name-only --diff-filter=MR -- '*_test.go'; git diff --name-only --diff-filter=MR -- '*_test.go'; } | sort -u | grep -Fxf "$BASE_DIR/test_files.txt" || true)"
ALLOWED_RE='^$'
(( PHASE >= 5 )) && ALLOWED_RE='^internal/delivery/mcpserver/(server|hardening)_test\.go$'
UNEXPECTED="$(echo "$MODIFIED_TESTS" | grep -vE "$ALLOWED_RE" | grep -v '^$' || true)"
if [[ -z "$UNEXPECTED" ]]; then pass "no baseline test file modified outside the allowed set"; else fail "baseline test files modified: $(echo "$UNEXPECTED" | tr '\n' ' ')"; fi
NOW_MCP="$(count_assertions "${MCP_PAGING_TESTS[@]}" internal/delivery/paging/*_test.go)"
if (( NOW_MCP >= $(cat "$BASE_DIR/mcp_paging_assertions.txt") )); then pass "MCP paging assertions did not drop ($NOW_MCP)"; else fail "MCP paging assertions dropped: baseline $(cat "$BASE_DIR/mcp_paging_assertions.txt"), now $NOW_MCP"; fi

BASE_ASSERTIONS=0; NOW_ASSERTIONS=0
while read -r file count; do
	BASE_ASSERTIONS=$((BASE_ASSERTIONS + count))
	NOW_ASSERTIONS=$((NOW_ASSERTIONS + $(count_assertions "$file")))
done <"$BASE_DIR/cli_assertions_by_file.txt"
if (( NOW_ASSERTIONS >= BASE_ASSERTIONS )); then pass "assertions in baseline CLI test files did not drop ($NOW_ASSERTIONS >= $BASE_ASSERTIONS)"; else fail "assertions in baseline CLI test files dropped: baseline $BASE_ASSERTIONS, now $NOW_ASSERTIONS"; fi
if diff -q "$BASE_DIR/cli_exit_assertions.txt" <(collect_cli_exit_assertions) >/dev/null; then pass "CLI test exit-code expectations unchanged"; else fail "CLI test exit-code expectations changed"; fi

if bash "$GUARD_DIR/capture-cli-output.sh" "$TMP/cli-now" >/dev/null 2>&1; then
	JSON_DIFF="$(cd "$BASE_DIR/cli-before" && for f in *.json.txt; do diff -q "$f" "$TMP/cli-now/$f" >/dev/null 2>&1 || echo "$f"; done)"
	if [[ -z "$JSON_DIFF" ]]; then pass "CLI --json output identical to baseline"; else fail "CLI --json output differs from baseline: $(echo "$JSON_DIFF" | tr '\n' ' ')"; fi
	HUMAN_DIFF="$(cd "$BASE_DIR/cli-before" && for f in *.human.txt; do diff -q "$f" "$TMP/cli-now/$f" >/dev/null 2>&1 || echo "$f"; done)"
	if [[ -z "$HUMAN_DIFF" ]]; then pass "CLI human output identical to baseline"; else fail "CLI human output differs from baseline: $(echo "$HUMAN_DIFF" | tr '\n' ' ')"; fi
else
	fail "capture-cli-output.sh failed to run"
fi

# ---------- helpers ----------
REQUIRED_TESTS=()
require_test() { REQUIRED_TESTS+=("$1"); }
run_required_tests() {
	(( ${#REQUIRED_TESTS[@]} == 0 )) && return
	local pattern name
	pattern="^($(IFS='|'; echo "${REQUIRED_TESTS[*]}"))\$"
	go test -count=1 -v -run "$pattern" ./internal/... >"$TMP/required.log" 2>&1
	for name in "${REQUIRED_TESTS[@]}"; do
		if grep -qE -- "^--- PASS: $name \(" "$TMP/required.log"; then pass "required Go test ran and passed: $name"; else fail "required Go test missing, skipped, or failing: $name"; fi
	done
}
require_file() { if [[ -f "$1" ]]; then pass "file exists: $1"; else fail "file missing: $1"; fi; }
require_grep() { # require_grep "<description>" <pattern> <files...>
	local description="$1" pattern="$2"; shift 2
	if grep -qE -- "$pattern" "$@" 2>/dev/null; then pass "$description"; else fail "$description"; fi
}
forbid_grep() { # forbid_grep "<description>" <pattern> <files...>
	local description="$1" pattern="$2"; shift 2
	local hits
	hits="$(grep -nE -- "$pattern" "$@" 2>/dev/null || true)"
	if [[ -z "$hits" ]]; then pass "$description"; else fail "$description: $(echo "$hits" | head -5 | tr '\n' ' ')"; fi
}
forbid_rgrep() { # forbid_rgrep "<description>" <pattern> <dirs...>  (recursive)
	local description="$1" pattern="$2"; shift 2
	local hits
	hits="$(grep -rnE -- "$pattern" "$@" 2>/dev/null || true)"
	if [[ -z "$hits" ]]; then pass "$description"; else fail "$description: $(echo "$hits" | head -5 | tr '\n' ' ')"; fi
}
web_sources() { non_test internal/delivery/web/*.go; }
frontend_checks() {
	require_file web/go.mod
	require_file web/package-lock.json
	require_file web/src/design-system/PATCHES.md
	if make web-check >"$TMP/web-check.log" 2>&1; then pass "make web-check exits 0"; else fail "make web-check failed (see output below)"; tail -40 "$TMP/web-check.log"; fi
	if make web-e2e >"$TMP/web-e2e.log" 2>&1; then pass "make web-e2e exits 0"; else fail "make web-e2e failed (see output below)"; tail -40 "$TMP/web-e2e.log"; fi
	local dirs=(web/src) ; [[ -d web/e2e ]] && dirs+=(web/e2e)
	forbid_rgrep "no focused or skipped frontend tests" '\b(it|test|describe)\.(only|skip)\(|\bx(it|describe)\(' "${dirs[@]}"
	forbid_rgrep "no TypeScript or ESLint suppression comments" '@ts-ignore|@ts-nocheck|@ts-expect-error|eslint-disable' "${dirs[@]}"
	forbid_rgrep "no dangerouslySetInnerHTML" 'dangerouslySetInnerHTML' web/src
	forbid_rgrep "no CDN or remote font references in web/src" 'fonts\.googleapis|fonts\.gstatic|unpkg\.com|cdn\.jsdelivr|cdnjs\.' web/src
}

# ---------- phase checks (cumulative) ----------
phase1() {
	echo "-- phase 1: web adapter foundation"
	local f
	for f in server.go security.go throttle.go listen.go errors.go assets.go routes_read.go dist/.gitkeep; do require_file "internal/delivery/web/$f"; done
	require_file internal/delivery/cli/serve.go
	require_grep "root.go dispatches serve" 'case "serve":' internal/delivery/cli/root.go
	require_grep "root.go dispatches the web alias" 'case "web":' internal/delivery/cli/root.go
	require_grep "help.go documents serve" '"serve":' internal/delivery/cli/help.go
	require_grep "global help lists serve web" 'serve web' internal/delivery/cli/help.go
	require_grep "token comparison is constant-time" 'subtle\.ConstantTimeCompare' internal/delivery/web/security.go
	require_grep "errors are classified by app.ClassifyError" 'app\.ClassifyError\(' internal/delivery/web/errors.go
	forbid_grep "web adapter does not import the MCP adapter" 'delivery/mcpserver' $(web_sources)
	forbid_grep "web adapter never calls run-mutating services" 'PrepareDistillRuns|StartDistillRun|RetryDistillRun|SubmitDistillRun' $(web_sources)
	forbid_grep "web adapter never confirms without explicit pins" 'ConfirmProposal\(|DispatchConfirmProposal\(' $(web_sources)
	forbid_grep "web adapter keeps no substring error rules" 'strings\.(Contains|HasPrefix)\(err' $(web_sources)
	local name
	for name in TestErrorStatusCoversSchemaCodes TestSecurityMiddleware TestAuthThrottle TestListenRule TestStartupOutput TestAssetsServing TestReadEndpointsGolden TestServeWildcardAnswersOnLoopback TestServeWebCommand; do require_test "$name"; done
}
phase2() {
	echo "-- phase 2: frontend foundation"
	frontend_checks
	local f
	for f in web/src/state/local-store.test.ts web/src/api/client.test.ts web/src/screens/home/home.test.tsx web/src/screens/skills/skills.test.tsx web/e2e/smoke.spec.ts web/.nvmrc scripts/web-dev.sh; do require_file "$f"; done
	require_grep "Makefile has web-check" '^web-check:' Makefile
	require_grep "Makefile has web-e2e" '^web-e2e:' Makefile
	require_grep "Makefile has web-dev" '^web-dev:' Makefile
	require_grep "CI has a web job" '^  web:' .github/workflows/ci.yml
	require_grep "release has a web job" '^  web:' .github/workflows/release.yml
	require_grep "eslint forbids JSX literals" 'jsx-no-literals' web/eslint.config.js
}
phase3() {
	echo "-- phase 3: skill authoring"
	local f
	for f in internal/delivery/web/routes_skill_write.go internal/delivery/web/locator.go web/src/components/markdown.test.tsx web/src/components/proposal-preview.test.tsx web/src/components/conflict-drawer.test.tsx web/e2e/skill-lifecycle.spec.ts; do require_file "$f"; done
	require_grep "skill confirm loads the stored proposal" 'LoadSkillProposal\(' internal/delivery/web/routes_skill_write.go
	require_grep "skill confirm uses ConfirmSkillMutation" 'ConfirmSkillMutation\(' internal/delivery/web/routes_skill_write.go
	require_grep "add confirm loads the add proposal" 'LoadSkillAddProposal\(' internal/delivery/web/routes_skill_write.go
	local name
	for name in TestLocatorValidation TestSkillWriteEndpoints TestSkillConfirmRequiresAllPins; do require_test "$name"; done
}
phase4() {
	echo "-- phase 4: sources and runs"
	local f
	for f in internal/delivery/web/routes_sources.go internal/delivery/web/routes_runs.go web/src/domain/source-status.test.ts web/src/domain/handoff-brief.test.ts web/src/state/recent-runs.test.ts web/e2e/sources-runs.spec.ts web/e2e/support/seed-workspace.ts; do require_file "$f"; done
	require_grep "watch confirm checks the proposal command" '"source_watch"' internal/delivery/web/routes_sources.go
	require_grep "app exports ErrDistillRunNotFound" 'ErrDistillRunNotFound *= *errors\.New\("distill run not found"\)' internal/app/*.go
	local name
	for name in TestRoutesNeverMutateRuns TestSourceEndpoints TestRunEndpoints; do require_test "$name"; done
}
phase5() {
	echo "-- phase 5: inbox, insight, composer"
	local f
	for f in internal/delivery/paging/paging.go internal/delivery/paging/paging_test.go internal/delivery/web/routes_insights.go web/src/domain/evidence-set.test.ts web/src/domain/insight-staleness.test.ts web/src/state/composer-draft.test.ts web/e2e/insight-apply.spec.ts; do require_file "$f"; done
	forbid_grep "paging helpers no longer defined in mcpserver" '^func (decodeCursor|encodeCursor|normalizeLimit|makePage|pageOwner|cursorChecksum|pageDigest)\b|^type page\[|^var cursorMACKey' $(non_test internal/delivery/mcpserver/*.go)
	forbid_grep "web insight handlers do not send idempotency keys" 'IdempotencyKey' internal/delivery/web/routes_insights.go
	require_grep "app exports ErrInsightNotFound" 'ErrInsightNotFound *= *errors\.New\("insight not found"\)' internal/app/*.go
	local name
	for name in TestOpaqueCursorMultiPageAndIntegrity TestInboxPaging TestInsightEndpoints; do require_test "$name"; done
}
phase6() {
	echo "-- phase 6: hardening, docs, release"
	local f
	for f in docs/contracts/web-api.md web/THIRD_PARTY_NOTICES.md web/e2e/a11y.spec.ts; do require_file "$f"; done
	forbid_grep "architecture doc no longer excludes the Web UI from V1" 'Web UI không thuộc V1|Không Web UI trong V1|^- Web UI trong V1\.$' docs/design/01-system-architecture.md
	forbid_grep "error-code registry no longer says it adds no web UI" 'adds no web UI' docs/contracts/error-codes.md
	require_grep "README documents skillhub serve web" 'skillhub serve web' README.md
	require_grep "user guide documents --loopback-only" '\-\-loopback-only' docs/user-guide.md
	require_grep "release smoke test launches serve web" 'serve web' .github/workflows/release.yml
	require_grep "spec 04 explains not_found comes from HTTP 404" '404' docs/use-cases/04-webui-user-flows-and-screen-specs.md
}

(( PHASE >= 1 )) && phase1
(( PHASE >= 2 )) && phase2
(( PHASE >= 3 )) && phase3
(( PHASE >= 4 )) && phase4
(( PHASE >= 5 )) && phase5
(( PHASE >= 6 )) && phase6
run_required_tests

echo
if (( FAILURES == 0 )); then
	echo "GUARD RESULT: PASS (phase $PHASE)"
	exit 0
fi
echo "GUARD RESULT: FAIL (phase $PHASE) - $FAILURES failure(s)"
exit 1
