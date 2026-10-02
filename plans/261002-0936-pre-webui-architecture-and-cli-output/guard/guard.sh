#!/usr/bin/env bash
# Mechanical guard for plan 261002-0936-pre-webui-architecture-and-cli-output.
#
# Usage:
#   guard.sh baseline        # planner only: freeze baseline metrics (already done)
#   guard.sh check <phase>   # executor: run after every phase, phase = 0..6
#
# Every check prints PASS/FAIL lines. The final line is either
#   GUARD RESULT: PASS (phase N)
# or
#   GUARD RESULT: FAIL (phase N) - <count> failure(s)
# The script exits 0 only on PASS. Executors must not edit this file, the
# baseline directory, the fixtures, or the approvals directory.
set -uo pipefail

PLAN_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT="$(git -C "$PLAN_DIR" rev-parse --show-toplevel)"
GUARD_DIR="$PLAN_DIR/guard"
BASE_DIR="$GUARD_DIR/baseline"
REPORTS="$PLAN_DIR/reports"
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
# Assertion count per CLI test file that existed at baseline ("<file> <count>").
collect_cli_assertions_by_file() {
	local file
	for file in internal/delivery/cli/*_test.go; do
		printf '%s %s\n' "$file" "$(grep -cE 'strings\.Contains\(|t\.(Fatalf|Errorf|Fatal|Error)\(' "$file")"
	done
}
# Exit-code expectations in CLI tests, e.g. "code != 2", counted per form.
collect_cli_exit_assertions() {
	cat internal/delivery/cli/*_test.go | grep -oE '\b(code|exitCode|exit) ?(!=|==) ?[0-9]+' | sed -E 's/ //g' | sort | uniq -c | sed -E 's/^ +//'
}
collect_root_funcs() {
	grep -oE '^func [A-Za-z0-9_]+' internal/delivery/cli/root.go | sed 's/^func //' | sort
}
# Prints "<file>|<Receiver.Func or Func>|<lines>" for every top-level func.
func_lengths() {
	awk '
		/^func / { name = $0; sub(/\{.*$/, "", name); start = FNR; inside = 1; next }
		/^}/ && inside { print FILENAME "|" name "|" (FNR - start + 1); inside = 0 }
	' "$@" | sed -E 's/\|func \(([A-Za-z_]+ )?\*?([A-Za-z0-9_]+)(\[[^]]*\])?\) ([A-Za-z0-9_]+)\(.*\|/|\2.\4|/; s/\|func ([A-Za-z0-9_]+)(\[[^]]*\])?\(.*\|/|\1|/'
}
app_sources() { non_test internal/app/*.go; }
collect_long_app_funcs() {
	func_lengths $(app_sources) | awk -F'|' '$3 > 120 { print $2 "|" $3 }' | sort
}
# Parameter lists of internal/app funcs with more than 6 comma-separated entries.
# Multi-line signatures are joined first so line breaks cannot hide parameters.
collect_wide_signatures() {
	awk '/^func /{ sig = $0; while (sig !~ /\{[[:space:]]*$/ && (getline line) > 0) sig = sig " " line; print sig }' $(app_sources) |
		sed -E 's/^func (\([^)]*\) )?[A-Za-z0-9_]+(\[[^]]*\])?\(//; s/\) .*$//; s/\)[[:space:]]*\{?[[:space:]]*$//' |
		awk -F',' 'NF > 6 { print }' | sort
}

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
	collect_long_app_funcs >"$BASE_DIR/long_app_funcs.txt"
	collect_wide_signatures >"$BASE_DIR/wide_signatures.txt"
	collect_root_funcs >"$BASE_DIR/root_funcs.txt"
	(cd "$GUARD_DIR" && {
		sha256sum guard.sh capture-cli-output.sh fixtures/error_characterization_test.go.txt
		find baseline -type f ! -name guard.sha256 | sort | xargs sha256sum
	} >baseline/guard.sha256)
	echo "baseline written to $BASE_DIR (commit $(cat "$BASE_DIR/base_commit.txt"))"
	exit 0
fi

if [[ "${1:-}" != "check" || -z "${2:-}" ]]; then
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
echo "== The user must confirm both hashes match the ones recorded when the plan was handed over. =="

# ---------- guard integrity ----------
if (cd "$GUARD_DIR" && sha256sum -c --quiet baseline/guard.sha256) >/dev/null 2>&1; then pass "guard files match pinned checksums"; else fail "guard files do not match pinned checksums"; fi
if [[ -n "$GUARD_COMMIT" ]] && git diff --quiet "$GUARD_COMMIT" -- "$PLAN_REL/guard" && [[ -z "$(git ls-files --others --exclude-standard -- "$PLAN_REL/guard")" ]]; then
	pass "guard directory is committed and has no uncommitted or untracked changes"
else
	fail "guard directory is not committed, or has uncommitted/untracked changes"
fi

# ---------- universal checks ----------
check "go build ./... exits 0" go build ./...
check "go vet ./... exits 0" go vet ./...
if go test -count=1 ./... >"$TMP/test.log" 2>&1; then pass "go test -count=1 ./... exits 0"; else fail "go test -count=1 ./... exits 0 (see output below)"; tail -40 "$TMP/test.log"; fi
if make lint LINT_BASE="$BASE_COMMIT" >"$TMP/lint.log" 2>&1; then pass "golangci-lint reports no new issues since the baseline commit"; else fail "golangci-lint reports new issues (see output below)"; tail -40 "$TMP/lint.log"; fi

UNFORMATTED="$(gofmt -l internal cmd 2>/dev/null)"
if [[ -z "$UNFORMATTED" ]]; then pass "gofmt -l reports no files"; else fail "gofmt -l reports: $UNFORMATTED"; fi

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

# Added Go lines (committed + uncommitted since baseline, plus untracked files).
{
	git diff "$BASE_COMMIT" -U0 -- '*.go' | grep -E '^\+' | grep -vE '^\+\+\+'
	git ls-files --others --exclude-standard -- '*.go' | xargs -r cat
} >"$TMP/added.txt"
FORBIDDEN="$(grep -nE '\b[tbf]\.Skip(f|Now)?\(|//[[:space:]]*nolint|//go:build[[:space:]]+ignore|\b(TODO|FIXME|XXX|HACK)\b|not implemented|panic\("unreachable|testing\.Testing\(\)|testing\.Short\(\)|func TestMain\(' "$TMP/added.txt" || true)"
if [[ -z "$FORBIDDEN" ]]; then pass "no forbidden patterns in added code (skip, nolint, build ignore, TODO/FIXME, not implemented, testing.Testing/Short, TestMain)"; else fail "forbidden patterns in added code: $FORBIDDEN"; fi

EMPTY_CONTAINS="$(grep -nE 'strings\.Contains\([^,]+, *""\)' "$TMP/added.txt" || true)"
if [[ -z "$EMPTY_CONTAINS" ]]; then pass "no added strings.Contains(x, \"\") assertions"; else fail "added vacuous strings.Contains(x, \"\") assertions: $EMPTY_CONTAINS"; fi

DELETED_TESTS="$(git diff "$BASE_COMMIT" --name-only --diff-filter=D -- '*_test.go')"
if [[ -z "$DELETED_TESTS" ]]; then pass "no test file deleted"; else fail "test files deleted: $DELETED_TESTS"; fi

MODIFIED_TESTS="$(git diff "$BASE_COMMIT" --name-only --diff-filter=MR -- '*_test.go')"
if (( PHASE < 5 )); then
	if [[ -z "$MODIFIED_TESTS" ]]; then pass "no existing test file modified (phases 0-4 may only add new test files)"; else fail "existing test files modified before phase 5: $(echo "$MODIFIED_TESTS" | tr '\n' ' ')"; fi
else
	OUTSIDE_CLI="$(echo "$MODIFIED_TESTS" | grep -vE '^internal/delivery/cli/[a-z_]+_test\.go$' | grep -v '^$' || true)"
	if [[ -z "$OUTSIDE_CLI" ]]; then pass "only internal/delivery/cli/*_test.go files were modified"; else fail "test files modified outside internal/delivery/cli: $(echo "$OUTSIDE_CLI" | tr '\n' ' ')"; fi
	JSON_EDITS="$(git diff "$BASE_COMMIT" -U0 -- 'internal/delivery/cli/*_test.go' | grep -E '^-' | grep -vE '^---' | grep -E -- '--json|json\.Unmarshal|json\.Decoder|schema_version|"code"' || true)"
	if [[ -z "$JSON_EDITS" ]]; then pass "no JSON-related assertion line was removed or changed in CLI tests"; else fail "JSON-related CLI test lines were removed or changed: $(echo "$JSON_EDITS" | head -5 | tr '\n' ' ')"; fi
fi

BASE_ASSERTIONS=0; NOW_ASSERTIONS=0
while read -r file count; do
	BASE_ASSERTIONS=$((BASE_ASSERTIONS + count))
	if [[ -f "$file" ]]; then NOW_ASSERTIONS=$((NOW_ASSERTIONS + $(grep -cE 'strings\.Contains\(|t\.(Fatalf|Errorf|Fatal|Error)\(' "$file"))); fi
done <"$BASE_DIR/cli_assertions_by_file.txt"
if (( NOW_ASSERTIONS >= BASE_ASSERTIONS )); then pass "assertions in baseline CLI test files did not drop ($NOW_ASSERTIONS >= $BASE_ASSERTIONS)"; else fail "assertions in baseline CLI test files dropped: baseline $BASE_ASSERTIONS, now $NOW_ASSERTIONS"; fi

if diff -q "$BASE_DIR/cli_exit_assertions.txt" <(collect_cli_exit_assertions) >/dev/null; then pass "CLI test exit-code expectations unchanged"; else fail "CLI test exit-code expectations changed (compare guard/baseline/cli_exit_assertions.txt)"; fi

# CLI behavior snapshot against the planner-captured baseline.
if bash "$GUARD_DIR/capture-cli-output.sh" "$TMP/cli-now" >/dev/null 2>&1; then
	JSON_DIFF="$(cd "$BASE_DIR/cli-before" && for f in *.json.txt; do diff -q "$f" "$TMP/cli-now/$f" >/dev/null 2>&1 || echo "$f"; done)"
	if [[ -z "$JSON_DIFF" ]]; then pass "CLI --json output identical to baseline for all captured cases"; else fail "CLI --json output differs from baseline: $(echo "$JSON_DIFF" | tr '\n' ' ')"; fi
	HUMAN_SHAPE=""; HUMAN_LOSS=""
	shape() { awk '/^exit=/{print} /^--- stdout ---/{s="out";next} /^--- stderr ---/{s="err";next} NF&&s{seen[s]=1} END{print "stdout:" (seen["out"]?"yes":"no"); print "stderr:" (seen["err"]?"yes":"no")}' "$1"; }
	for f in "$BASE_DIR"/cli-before/*.human.txt; do
		name="$(basename "$f")"; now="$TMP/cli-now/$name"
		[[ -f "$now" ]] || { HUMAN_SHAPE+="$name(missing) "; continue; }
		[[ "$(shape "$f")" == "$(shape "$now")" ]] || HUMAN_SHAPE+="$name "
		while IFS= read -r token; do
			grep -qF -- "$token" "$now" || HUMAN_LOSS+="$name:$token "
		done < <(grep -oE '[A-Z]+-<ID>|<[A-Z0-9]+>|\b(draft|active|deprecated|archived)\b|[a-z0-9]+(-[a-z0-9]+)+|[A-Za-z0-9_.<>-]+/[A-Za-z0-9_./<>-]+' "$f" | sort -u)
	done
	if [[ -z "$HUMAN_SHAPE" ]]; then pass "CLI human output keeps exit codes and stdout/stderr routing"; else fail "CLI human output changed exit code or stream routing: $HUMAN_SHAPE"; fi
	if [[ -z "$HUMAN_LOSS" ]]; then pass "CLI human output still contains every identifier, placeholder, state, and path from baseline"; else fail "CLI human output lost identifiers/paths: $(echo "$HUMAN_LOSS" | cut -c1-600)"; fi
else
	fail "capture-cli-output.sh failed to run"
fi

# ---------- helpers for phase checks ----------
REQUIRED_TESTS=()
require_test() { REQUIRED_TESTS+=("$1"); }
run_required_tests() {
	(( ${#REQUIRED_TESTS[@]} == 0 )) && return
	local pattern name
	pattern="^($(IFS='|'; echo "${REQUIRED_TESTS[*]}"))\$"
	go test -count=1 -v -run "$pattern" ./internal/... >"$TMP/required.log" 2>&1
	for name in "${REQUIRED_TESTS[@]}"; do
		if grep -qE -- "^--- PASS: $name \(" "$TMP/required.log"; then pass "required test ran and passed: $name"; else fail "required test missing, skipped, or failing: $name"; fi
	done
}
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
func_body() { # func_body <file> <func name>  -> prints the body of a top-level func
	awk -v name="$2" '$0 ~ "^func " name "\\(" { inside = 1 } inside { print } inside && /^}/ { exit }' "$1"
}
require_approval() { # require_approval <phase> <approved file>
	local stamp="$REPORTS/approvals/phase-$1.approved" expected
	expected="$(sha256sum "$2" 2>/dev/null | cut -d' ' -f1)"
	if [[ -f "$stamp" && -n "$expected" ]] && grep -qx "$expected" "$stamp"; then
		pass "user approval for phase $1 matches the approved file"
	else
		fail "missing or stale user approval: $stamp must contain sha256 of ${2#"$ROOT"/} (created by the user, never by the executor)"
	fi
}

# ---------- phase checks (cumulative) ----------
phase1() {
	echo "-- phase 1: skill detail read model"
	check "internal/app/skill_detail.go exists" test -f internal/app/skill_detail.go
	require_grep "GetSkillDetail is a SkillService method" 'func \(([a-z]+ )?SkillService\) GetSkillDetail\(ctx context\.Context, path, id string\) \(SkillDetail, error\)' internal/app/skill_detail.go
	local symbol
	for symbol in 'AssessSkillState\(' 'ReadSkillRationale\(' 'ReadSkillRouting\(' 'sha256\.Sum256\(' 'BasisCanonical'; do
		require_grep "skill_detail.go owns $symbol" "$symbol" internal/app/skill_detail.go
	done
	require_grep "skill_get handler calls GetSkillDetail" 'GetSkillDetail\(' internal/delivery/mcpserver/skill_tools.go
	forbid_grep "no mcpserver production file assesses skill state or reads routing/rationale" 'AssessSkillState|ReadSkillRouting|ReadSkillRationale' $(non_test internal/delivery/mcpserver/*.go)
	forbid_grep "skill_tools.go no longer hashes content or finds the entrypoint" 'sha256|hex\.Encode|SKILL\.md' internal/delivery/mcpserver/skill_tools.go
	forbid_grep "GetSkillDetail does not discard errors with blank identifiers" ', _ :?= ' internal/app/skill_detail.go
	# Pure refactor: no new tests. The existing MCP owner tests must still run and pass.
	require_test TestSkillUpdatePreviewWithExpectedContentDigest
	require_test TestSkillToolsLifecycleInMemory
	require_test TestSkillUpdatePreviewUnknownID
}
phase2() {
	echo "-- phase 2: shared error classification"
	check "internal/app/error_classify.go exists" test -f internal/app/error_classify.go
	require_grep "ClassifyError defined" '^func ClassifyError\(err error\) \*Error' internal/app/error_classify.go
	require_grep "ErrorOf defined" '^func ErrorOf\(value any, err error\) \*Error' internal/app/error_classify.go
	require_grep "Result.ApplicationError defined" '^func \(r Result\) ApplicationError\(\) \*Error' internal/app/result.go
	func_body internal/delivery/mcpserver/server.go safeToolError >"$TMP/safe.go"
	require_grep "safeToolError delegates to app.ClassifyError" 'app\.ClassifyError\(err\)' "$TMP/safe.go"
	forbid_grep "safeToolError keeps no sentinel or substring rules" 'errors\.(Is|As)\(err, *&?(telemetry|mutation|sourcepkg|skill|catalog|os|context)\.|strings\.(Contains|ToLower)\(' "$TMP/safe.go"
	require_grep "appResult/applicationError use app.ErrorOf" 'app\.ErrorOf\(' internal/delivery/mcpserver/server.go
	forbid_grep "no substring marker list remains in delivery code" '" are required"|"already used"|"stale proposal"|"confirmation pins"' $(non_test internal/delivery/mcpserver/*.go internal/delivery/cli/*.go)
	forbid_grep "applicationError no longer JSON round-trips results" 'json\.Unmarshal\(data, &envelope\)' internal/delivery/mcpserver/server.go
	check "error_characterization_test.go is byte-identical to the plan fixture" cmp -s "$GUARD_DIR/fixtures/error_characterization_test.go.txt" internal/delivery/mcpserver/error_characterization_test.go
	local message
	for message in \
		'insight decision requires a rationale' 'only a pending insight can be planned' 'only a rejected insight can be reopened' \
		'application preview requires at least one changed skill path' 'application path %s is unchanged' \
		'invalid observation ID' 'invalid operation ID'; do
		require_grep "insight.go: \"$message\" uses NewInvalidRequestError on the same line" "NewInvalidRequestError\(.*\"$message" internal/app/insight.go
		forbid_grep "insight.go: \"$message\" is no longer a plain error" "(errors\.New|fmt\.Errorf)\(\"$message" internal/app/insight.go
	done
	for message in \
		'only explicit blocking ambiguity or coverage decisions may pause a run' 'duplicate submitted observation' \
		'duplicate comparison stable identity' 'duplicate insight stable identity' 'invalid run ID'; do
		require_grep "distill.go: \"$message\" uses NewInvalidRequestError on the same line" "NewInvalidRequestError\(.*\"$message" internal/app/distill.go
		forbid_grep "distill.go: \"$message\" is no longer a plain error" "(errors\.New|fmt\.Errorf)\(\"$message" internal/app/distill.go
	done
	require_grep "failSubmission stores failureText(cause)" 'failureText\(cause\)' internal/app/distill.go
	require_test TestSafeToolErrorCharacterization
	require_test TestInsightValidationErrorsAreInvalidRequest
	require_test TestDistillValidationErrorsAreInvalidRequest
}
phase3() {
	echo "-- phase 3: proposal helper and service split"
	require_approval 02 "$REPORTS/phase-02-report.md"
	check "internal/app/proposal_confirm.go exists" test -f internal/app/proposal_confirm.go
	require_grep "verifyProposalPins defined with the planned signature" '^func verifyProposalPins\(now, expiresAt time\.Time, checkExpiry, zeroExpiryIsExpired bool, expected, supplied ConfirmationPins\) proposalRefusal' internal/app/proposal_confirm.go
	local file
	for file in skill_add.go source_watch.go source.go source_import.go insight.go skill_lifecycle.go; do
		require_grep "internal/app/$file calls verifyProposalPins" 'verifyProposalPins\(' "internal/app/$file"
	done
	func_lengths $(app_sources) >"$TMP/app_lengths.txt"
	local method limit=80 length
	for method in SkillAddService.PreviewSkillAdd DistillService.SubmitDistillRun SkillService.ReviewSkill SourceService.PreviewSourceWatch; do
		length="$(awk -F'|' -v m="$method" '$2 == m { print $3 }' "$TMP/app_lengths.txt")"
		if [[ -n "$length" && "$length" -le "$limit" ]]; then pass "$method is $length lines (<= $limit)"; else fail "$method is '${length:-missing}' lines (must exist and be <= $limit)"; fi
	done
	collect_long_app_funcs >"$TMP/long_now.txt"
	local new_long wide
	new_long="$(awk -F'|' 'NR == FNR { base[$1] = $2; next } !($1 in base) || $2 > base[$1] { print $1 " (" $2 " lines)" }' "$BASE_DIR/long_app_funcs.txt" "$TMP/long_now.txt")"
	if [[ -z "$new_long" ]]; then pass "no app function over 120 lines was added or grew"; else fail "app functions over 120 lines added or grown: $(echo "$new_long" | tr '\n' ' ')"; fi
	collect_wide_signatures >"$TMP/wide_now.txt"
	wide="$(comm -13 "$BASE_DIR/wide_signatures.txt" "$TMP/wide_now.txt")"
	if [[ -z "$wide" ]]; then pass "no new internal/app function signature has more than 6 parameters"; else fail "new internal/app signatures with more than 6 parameters: $(echo "$wide" | head -5 | tr '\n' ' ')"; fi
	forbid_grep "no package-level func literals in internal/app (they would hide step functions from length checks)" '^var [A-Za-z0-9_]+ = func' $(app_sources)
}
phase4() {
	echo "-- phase 4: termui package"
	check "internal/delivery/cli/termui/termui.go exists" test -f internal/delivery/cli/termui/termui.go
	require_grep "golang.org/x/term is a direct dependency" '^[[:space:]]*golang\.org/x/term v[0-9.]+$' go.mod
	forbid_grep "termui imports no internal package" 'mcp-skill-hub/internal' internal/delivery/cli/termui/*.go
	forbid_grep "termui emits no ANSI escapes" '\\x1b|\\033|\\u001b' $(non_test internal/delivery/cli/termui/*.go)
	local name
	for name in TestWrapKeepsLongTokensIntact TestWrapKeepsBacktickSpansIntact TestFieldsAlignLabels TestErrorBlockOrder TestCommandLinesAreNeverWrapped TestBytesHumanized TestTableAlignsColumns TestWidthClamping; do
		require_test "$name"
	done
	if TERMUI_SAMPLES_OUT="$TMP/samples.txt" go test -count=1 -run '^TestWriteSamples$' ./internal/delivery/cli/termui/ >/dev/null 2>&1 && cmp -s "$TMP/samples.txt" "$REPORTS/termui-samples.txt"; then
		pass "reports/termui-samples.txt is exactly what TestWriteSamples renders"
	else
		fail "reports/termui-samples.txt is missing or differs from a fresh TestWriteSamples render"
	fi
}
phase5() {
	echo "-- phase 5: CLI migration"
	require_approval 04 "$REPORTS/termui-samples.txt"
	local sources labelled
	sources="$(non_test internal/delivery/cli/*.go | grep -vE '/(root|help)\.go$')"
	labelled="$(non_test internal/delivery/cli/*.go | grep -v '/help\.go$')"
	forbid_grep "no direct fmt.Fprint/io.WriteString/Write([]byte) in CLI command files (root.go, help.go exempt)" 'fmt\.Fprint|io\.WriteString|\.Write\(\[\]byte|= fmt\.Fprint|os\.Std(out|err)\.Write|bufio\.NewWriter' $sources
	forbid_grep "no hand-written ERROR/WHY/FIX/WARNING labels in CLI files (help.go exempt)" '"(ERROR|WHY|FIX|WARNING): ' $labelled
	forbid_grep "Raw is not used to bypass wrapping of formatted prose" '\.Raw\(fmt\.Sprint' $sources
	if diff -q "$BASE_DIR/root_funcs.txt" <(collect_root_funcs) >/dev/null; then pass "root.go declares no new functions"; else fail "root.go function set changed (new helpers belong in output.go)"; fi
	local encoders
	encoders="$(cat $sources | grep -cE 'json\.(NewEncoder|Marshal)' || true)"
	if (( encoders <= 1 )); then pass "no new JSON encoding in CLI command files ($encoders, baseline 1 in evaluation.go)"; else fail "CLI command files encode JSON directly $encoders times (baseline 1)"; fi
	require_grep "shared result writer exists in internal/delivery/cli/output.go" '^func writeResult\(stdout, stderr io\.Writer, jsonOutput bool, value any, render func\(p \*termui\.Printer\)\) int' internal/delivery/cli/output.go
	require_test TestHumanOutputFitsWidth
}
phase6() {
	echo "-- phase 6: docs"
	require_approval 05 "$REPORTS/phase-05-report.md"
	require_grep "architecture doc names app.ClassifyError" 'ClassifyError' docs/design/01-system-architecture.md
	require_grep "architecture doc names termui" 'termui' docs/design/01-system-architecture.md
	forbid_grep "WebUI spec no longer claims these validation errors surface as internal_error" 'bị trả về `internal_error`' docs/use-cases/04-webui-user-flows-and-screen-specs.md
	forbid_grep "WebUI spec no longer points the HTTP adapter at safeToolError" 'safeToolError' docs/use-cases/04-webui-user-flows-and-screen-specs.md
	require_grep "WebUI spec names app.ClassifyError" 'app\.ClassifyError' docs/use-cases/04-webui-user-flows-and-screen-specs.md
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
