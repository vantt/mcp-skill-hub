package systemskills_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/distill"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

// The distill-lab script tests run the embedded copy of scripts/distill.mjs with Node. Golden files
// under testdata/distill were produced once by the previous implementation of the script, so these
// tests pin its output; header lines name distill.mjs.
//
// Without Node on PATH the tests skip, unless SKILLHUB_REQUIRE_NODE=1 (set in CI), which makes a
// missing Node fail. On Windows the round trip runs one of the two 42-lesson fixtures.

const (
	jsYAMLSHA256        = "cea276c7e15f409a1adbe5d177aba7824398a474f7cf702bd57962c7d570636f"
	jsYAMLLicenseSHA256 = "a07bc24468b9654ce76a547d47a2db282d07733b715db4c73a98bd63961f9550"
	distillHeader       = "# Written by distill-lab/scripts/distill.mjs. Edit freely, then run `distill.mjs format`.\n"
)

var fixtureNames = []string{"phase0", "test-audit"}

func roundTripFixtures() []string {
	if runtime.GOOS == "windows" {
		return fixtureNames[:1]
	}
	return fixtureNames
}

func nodePath(t *testing.T) string {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		if os.Getenv("SKILLHUB_REQUIRE_NODE") == "1" {
			t.Fatal("node not found on PATH and SKILLHUB_REQUIRE_NODE=1; install Node 18 or newer")
		}
		t.Skip("node not found on PATH; install Node 18 or newer to run the distill-lab script tests")
	}
	return node
}

func scriptPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("distill-lab", "scripts", "distill.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func testdataPath(t *testing.T, parts ...string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join(append([]string{"testdata", "distill"}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "migration", "testdata", name+"-distill.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

type scriptResult struct {
	stdout string
	stderr string
	code   int
}

// isolatedEnv keeps git and the script away from the developer's own configuration.
func isolatedEnv(t *testing.T, extra ...string) []string {
	t.Helper()
	home := t.TempDir()
	globalConfig := filepath.Join(home, "gitconfig")
	if err := os.WriteFile(globalConfig, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	env := []string{}
	for _, kv := range os.Environ() {
		name := strings.ToUpper(strings.SplitN(kv, "=", 2)[0])
		if strings.HasPrefix(name, "GIT_") || name == "HOME" || name == "USERPROFILE" ||
			strings.HasPrefix(name, "XDG_") || name == "SKILLHUB_WORKSPACE" {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "HOME="+home, "USERPROFILE="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"GIT_CONFIG_GLOBAL="+globalConfig, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	return append(env, extra...)
}

func runNode(t *testing.T, dir string, env []string, args ...string) scriptResult {
	t.Helper()
	cmd := exec.Command(nodePath(t), args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run node %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return scriptResult{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func runScript(t *testing.T, dir string, env []string, args ...string) scriptResult {
	t.Helper()
	return runNode(t, dir, env, append([]string{scriptPath(t)}, args...)...)
}

func assertNoStackTrace(t *testing.T, r scriptResult) {
	t.Helper()
	if strings.Contains(r.stderr, "\n    at ") || strings.Contains(r.stderr, "Error: ") && !strings.Contains(r.stderr, "ERROR: ") {
		t.Fatalf("stderr shows a stack trace instead of an ERROR/WHY/FIX message:\n%s", r.stderr)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newHub makes a folder that carries the hub marker and returns the path of its distill.yaml.
func newHub(t *testing.T, content string) (hub, file string) {
	t.Helper()
	hub = t.TempDir()
	writeFile(t, filepath.Join(hub, ".skillhub", "schema-version"), "1\n")
	file = filepath.Join(hub, "skills", "default", "demo", ".meta", "distill.yaml")
	writeFile(t, file, content)
	return hub, file
}

func withoutFirstLine(s string) string {
	_, rest, _ := strings.Cut(s, "\n")
	return rest
}

func TestDistillScriptVendoredYAMLDigests(t *testing.T) {
	t.Parallel()

	for _, tree := range []string{
		filepath.Join("distill-lab", "scripts", "lib"),
		filepath.Join("..", "..", ".agents", "skills", "distill-lab", "scripts", "lib"),
	} {
		for name, want := range map[string]string{"js-yaml.mjs": jsYAMLSHA256, "js-yaml.LICENSE": jsYAMLLicenseSHA256} {
			sum := sha256.Sum256([]byte(readFile(t, filepath.Join(tree, name))))
			if got := hex.EncodeToString(sum[:]); got != want {
				t.Errorf("%s: sha256 %s, want %s (the vendored file must stay byte-identical to the pinned release)",
					filepath.Join(tree, name), got, want)
			}
		}
	}
}

func TestDistillScriptHelpAndUsageErrors(t *testing.T) {
	t.Parallel()
	env := isolatedEnv(t)
	dir := t.TempDir()

	help := runScript(t, dir, env, "--help")
	if help.code != 0 || !strings.HasPrefix(help.stdout, "usage: distill.mjs [-h] {check,format,list,decide} ...") {
		t.Fatalf("--help: code %d\nstdout:\n%s\nstderr:\n%s", help.code, help.stdout, help.stderr)
	}
	for _, args := range [][]string{{}, {"frob"}, {"list"}, {"list", "x.yaml", "--top", "x"}, {"decide", "x.yaml", "1"}} {
		r := runScript(t, dir, env, args...)
		if r.code != 2 || !strings.Contains(r.stderr, "error: ") {
			t.Errorf("%v: code %d, want 2 with a usage error\nstderr:\n%s", args, r.code, r.stderr)
		}
	}
}

// Loaded values must match the previous loader exactly (SHAs stay strings, dates keep their fields),
// for every committed distill file and every edge-case input.
func TestDistillScriptLoaderMatchesReference(t *testing.T) {
	t.Parallel()
	env := isolatedEnv(t)
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	files := map[string]bool{}
	for _, name := range roundTripFixtures() {
		files["internal/migration/testdata/"+name+"-distill.yaml"] = true
	}
	inputs, err := filepath.Glob(testdataPath(t, "inputs", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range inputs {
		rel, _ := filepath.Rel(repoRoot, input)
		files[filepath.ToSlash(rel)] = true
	}
	if runtime.GOOS != "windows" {
		if out, err := exec.Command("git", "-C", repoRoot, "ls-files", "*/.meta/distill.yaml",
			"internal/**/testdata/*distill*.yaml").Output(); err == nil {
			for _, line := range strings.Fields(string(out)) {
				files[line] = true
			}
		}
	}

	for rel := range files {
		golden := testdataPath(t, "loaded", strings.ReplaceAll(strings.TrimSuffix(rel, ".yaml"), "/", "__")+".json")
		if _, err := os.Stat(golden); err != nil {
			t.Errorf("%s has no golden loaded values at %s; produce them with the previous loader", rel, golden)
			continue
		}
		r := runNode(t, repoRoot, env, testdataPath(t, "load-harness.mjs"), scriptPath(t), "load", filepath.FromSlash(rel))
		if r.code != 0 {
			t.Errorf("%s: load failed (code %d):\n%s", rel, r.code, r.stderr)
			continue
		}
		var got, want any
		if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
			t.Fatalf("%s: harness output: %v", rel, err)
		}
		if err := json.Unmarshal([]byte(readFile(t, golden)), &want); err != nil {
			t.Fatalf("%s: golden: %v", rel, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: loaded values differ from the previous loader\n got: %s", rel, r.stdout)
		}
		if strings.HasSuffix(rel, "-distill.yaml") {
			assertShasAreStrings(t, rel, got)
		}
	}
}

// assertShasAreStrings checks the typed values of a distill document: every cursor and every
// decision.at is a plain string, never an int or float.
func assertShasAreStrings(t *testing.T, rel string, doc any) {
	t.Helper()
	top := mapPairs(doc)
	for _, pair := range top {
		switch pair[0] {
		case "cursors":
			for _, cursor := range mapPairs(pair[1]) {
				if _, ok := cursor[1].(string); !ok {
					t.Errorf("%s: cursor %v loaded as %v, want a string", rel, cursor[0], cursor[1])
				}
			}
		case "lessons":
			lessons, _ := pair[1].([]any)
			for _, lesson := range lessons {
				for _, field := range mapPairs(lesson) {
					if field[0] != "decision" {
						continue
					}
					for _, d := range mapPairs(field[1]) {
						if _, ok := d[1].(string); d[0] == "at" && !ok {
							t.Errorf("%s: decision.at loaded as %v, want a string", rel, d[1])
						}
					}
				}
			}
		}
	}
}

func mapPairs(v any) [][2]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	raw, _ := m["map"].([]any)
	out := make([][2]any, 0, len(raw))
	for _, item := range raw {
		pair, _ := item.([]any)
		if len(pair) == 2 {
			out = append(out, [2]any{pair[0], pair[1]})
		}
	}
	return out
}

func TestDistillScriptRoundsHalfToEven(t *testing.T) {
	t.Parallel()
	// Expected values are round(x, 2) of the previous implementation's runtime.
	cases := []struct{ in, want string }{
		{"0.125", "0.12"}, {"0.375", "0.38"}, {"0.625", "0.62"}, {"0.875", "0.88"}, {"2.675", "2.67"},
		{"1.005", "1.0"}, {"22.5", "22.5"}, {"0.6666666666666666", "0.67"}, {"1.3333333333333333", "1.33"},
		{"-0.125", "-0.12"}, {"-0.001", "-0.0"}, {"45", "45.0"},
	}
	args := []string{testdataPath(t, "load-harness.mjs"), scriptPath(t), "round"}
	for _, c := range cases {
		args = append(args, c.in)
	}
	r := runNode(t, t.TempDir(), isolatedEnv(t), args...)
	if r.code != 0 {
		t.Fatalf("round harness failed: %s", r.stderr)
	}
	got := strings.Fields(r.stdout)
	for i, c := range cases {
		if i >= len(got) || got[i] != c.want {
			t.Errorf("round(%s, 2) = %v, want %s", c.in, got, c.want)
		}
	}
}

func TestDistillScriptFormatRoundTripsFixtures(t *testing.T) {
	t.Parallel()
	env := isolatedEnv(t)
	for _, name := range roundTripFixtures() {
		fixture := readFile(t, fixturePath(t, name))
		_, file := newHub(t, fixture)

		r := runScript(t, t.TempDir(), env, "format", file, "--no-where")
		if r.code != 0 || r.stdout != "OK: wrote 42 lessons in priority order\n" {
			t.Fatalf("%s: format: code %d\nstdout:\n%s\nstderr:\n%s", name, r.code, r.stdout, r.stderr)
		}
		got := readFile(t, file)
		if !strings.HasPrefix(got, distillHeader) {
			t.Errorf("%s: header line is %q", name, strings.SplitN(got, "\n", 2)[0])
		}
		if withoutFirstLine(got) != withoutFirstLine(fixture) {
			t.Errorf("%s: format changed the body of a file written by the previous implementation", name)
		}
		doc, err := distill.UnmarshalDocument([]byte(got))
		if err != nil {
			t.Fatalf("%s: Go reader: %v", name, err)
		}
		if err := distill.ValidateDocument(doc); err != nil {
			t.Fatalf("%s: Go validator: %v", name, err)
		}
		if r := runScript(t, t.TempDir(), env, "format", file, "--no-where"); r.code != 0 || readFile(t, file) != got {
			t.Errorf("%s: a second format is not a no-op (code %d)", name, r.code)
		}
		if r := runScript(t, t.TempDir(), env, "check", file, "--no-where"); r.code != 0 || r.stdout != "OK: 42 lessons, goal confirmed\n" {
			t.Errorf("%s: check after format: code %d stdout %q stderr %q", name, r.code, r.stdout, r.stderr)
		}
	}
}

func TestDistillScriptFormatMatchesReferenceOnEdgeCases(t *testing.T) {
	t.Parallel()
	env := isolatedEnv(t)
	for _, name := range []string{"quoting", "wrapping", "numbers-and-order", "anchors-and-timestamps"} {
		file := filepath.Join(t.TempDir(), "distill.yaml")
		writeFile(t, file, readFile(t, testdataPath(t, "inputs", name+".yaml")))
		r := runScript(t, t.TempDir(), env, "format", file, "--no-where")
		if r.code != 0 {
			t.Errorf("%s: format: code %d\n%s", name, r.code, r.stderr)
			continue
		}
		if got, want := readFile(t, file), readFile(t, testdataPath(t, "format", name+".yaml")); got != want {
			t.Errorf("%s: output differs from the golden file\n got:\n%s", name, got)
		}
	}
}

func TestDistillScriptListMatchesReference(t *testing.T) {
	t.Parallel()
	env := isolatedEnv(t)
	for _, name := range roundTripFixtures() {
		for suffix, args := range map[string][]string{"top5": {"--top", "5"}, "all": {"--all"}} {
			r := runScript(t, t.TempDir(), env, append([]string{"list", fixturePath(t, name)}, args...)...)
			want := readFile(t, testdataPath(t, "list", name+"-"+suffix+".txt"))
			if r.code != 0 || r.stdout != want {
				t.Errorf("%s list %v: code %d\n got:\n%s\nwant:\n%s", name, args, r.code, r.stdout, want)
			}
		}
	}
}

func TestDistillScriptDecideRecordsDecisions(t *testing.T) {
	t.Parallel()
	env := isolatedEnv(t)
	fixture := readFile(t, fixturePath(t, "phase0"))
	cases := []struct {
		golden string
		args   []string
	}{
		{"decide-by-number", []string{"1", "3", "--state", "planned"}},
		{"decide-by-key", []string{"wait-on-owned-signal", "--state", "rejected", "--reason", "not now"}},
	}
	for _, c := range cases {
		_, file := newHub(t, fixture)
		r := runScript(t, t.TempDir(), env, append([]string{"decide", file}, c.args...)...)
		if want := readFile(t, testdataPath(t, "format", c.golden+".out")); r.code != 0 || r.stdout != want {
			t.Errorf("%s: code %d stdout %q want %q stderr %q", c.golden, r.code, r.stdout, want, r.stderr)
		}
		got := readFile(t, file)
		if want := readFile(t, testdataPath(t, "format", c.golden+".yaml")); got != want {
			t.Errorf("%s: file differs from the golden file", c.golden)
		}
		if !regexp.MustCompile(`at: [0-9a-f]{12}\}`).MatchString(got) {
			t.Errorf("%s: no decision with a 12-character at", c.golden)
		}
	}

	_, file := newHub(t, fixture)
	r := runScript(t, t.TempDir(), env, "decide", file, "2", "--state", "rejected")
	if r.code != 1 || !strings.Contains(r.stderr, "  - --reason is required for rejected\n") {
		t.Errorf("rejected without --reason: code %d stderr %q", r.code, r.stderr)
	}
	if readFile(t, file) != fixture {
		t.Error("a failed decide changed the file")
	}
}

func TestDistillScriptCheckReportsValidationErrors(t *testing.T) {
	t.Parallel()
	r := runScript(t, t.TempDir(), isolatedEnv(t), "check", testdataPath(t, "inputs", "invalid.yaml"), "--no-where")
	if want := readFile(t, testdataPath(t, "check", "invalid.txt")); r.code != 1 || r.stderr != want {
		t.Errorf("code %d\n got:\n%s\nwant:\n%s", r.code, r.stderr, want)
	}
}

// git runs git for test setup with the isolated environment.
func git(t *testing.T, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// requireGit skips when git is missing or older than the 2.45.0 the evidence check needs.
func requireGit(t *testing.T) {
	t.Helper()
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		t.Skip("git not found; the evidence check tests need Git 2.45.0 or newer")
	}
	m := regexp.MustCompile(`(\d+)\.(\d+)`).FindStringSubmatch(string(out))
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < 2 || major == 2 && minor < 45 {
		t.Skipf("%s is older than 2.45.0, which the evidence check needs", strings.TrimSpace(string(out)))
	}
}

// sourceRepo makes a repository with docs/a.md (3 lines), docs/crlf.md (\r\n, no final newline)
// and docs/seps.md (line separators beyond \n), and returns its path and HEAD.
func sourceRepo(t *testing.T, env []string) (string, string) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "src")
	git(t, env, "init", "-q", src)
	writeFile(t, filepath.Join(src, "docs", "a.md"), "one\ntwo\nthree\n")
	writeFile(t, filepath.Join(src, "docs", "crlf.md"), "one\r\ntwo\r\nthree")
	writeFile(t, filepath.Join(src, "docs", "seps.md"), "one two\vthree\ffour\x1cfive\nsix")
	git(t, env, "-C", src, "add", "-A")
	git(t, env, "-C", src, "commit", "-qm", "init")
	return src, git(t, env, "-C", src, "rev-parse", "HEAD")
}

func lessonDoc(cursors map[string]string, where ...string) string {
	var b strings.Builder
	b.WriteString("goal: {status: draft, purpose: p, in_scope: [i], out_of_scope: [o], failures_it_prevents: [f]}\ncursors:\n")
	for _, id := range []string{"src", "bare", "gone", "part", "link", "kept"} {
		if c, ok := cursors[id]; ok {
			b.WriteString("  " + id + ": " + c + "\n")
		}
	}
	b.WriteString("lessons:\n  - key: refs\n    layer: content\n    what: w\n    notable: n\n    where:\n")
	for _, w := range where {
		b.WriteString("      - '" + w + "'\n")
	}
	b.WriteString("    contrast: new\n    score: {relevance: 1, facts: [a], evidence: 1, effort: 1, why: w}\n    decision: {state: candidate}\n")
	return b.String()
}

func mirrorsDir(hub string) string {
	return filepath.Join(hub, "runtime", "distill-lab", "mirrors")
}

func TestDistillScriptEvidenceCheckMatchesReference(t *testing.T) {
	t.Parallel()
	requireGit(t)
	env := isolatedEnv(t)
	src, c := sourceRepo(t, env)
	short, long := c[:7], c[:12]
	doc := lessonDoc(map[string]string{"src": c, "bare": c, "gone": c},
		"src@"+short, "src@deadbeef",
		"src@"+long+":docs/a.md", "src@"+long+":docs/a.md#L2-L3", "src@"+long+":docs/a.md#L4",
		"src@"+long+":docs/a.md#L3-L2", "src@"+long+":docs/missing.md",
		"src@"+long+":docs/crlf.md#L3", "src@"+long+":docs/crlf.md#L4",
		"src@"+long+":docs/seps.md#L6", "src@"+long+":docs/seps.md#L7",
		"bare@"+short, "bare@"+long+":docs/a.md#L3", "bare@"+long+":docs/a.md#L9",
		"gone@"+short, "gone@"+long+":docs/a.md")
	hub, file := newHub(t, doc)
	git(t, env, "clone", "-q", src, filepath.Join(mirrorsDir(hub), "src"))
	git(t, env, "clone", "-q", "--bare", src, filepath.Join(mirrorsDir(hub), "bare"))

	// Run from an unrelated folder, with SKILLHUB_WORKSPACE naming another hub: the hub is found from
	// the file's own path only.
	other, _ := newHub(t, doc)
	r := runScript(t, t.TempDir(), append(env, "SKILLHUB_WORKSPACE="+other), "check", file)
	want := "ERROR: distill.yaml is invalid; nothing was written\n" +
		"  - refs: src@deadbeef is not a commit in mirror src\n" +
		"  - refs: src@" + long + ":docs/a.md#L4 line range exceeds 3 lines\n" +
		"  - refs: src@" + long + ":docs/a.md#L3-L2 line range is reversed\n" +
		"  - refs: src@" + long + ":docs/missing.md does not resolve in mirror src\n" +
		"  - refs: src@" + long + ":docs/crlf.md#L4 line range exceeds 3 lines\n" +
		"  - refs: src@" + long + ":docs/seps.md#L7 line range exceeds 6 lines\n" +
		"  - refs: bare@" + long + ":docs/a.md#L9 line range exceeds 3 lines\n" +
		"  - refs: gone@" + short + " is not a commit in mirror gone\n" +
		"  - refs: gone@" + long + ":docs/a.md does not resolve in mirror gone\n"
	if r.code != 1 || r.stderr != want {
		t.Fatalf("code %d\n got:\n%s\nwant:\n%s", r.code, r.stderr, want)
	}

	good := lessonDoc(map[string]string{"src": c, "bare": c}, "src@"+short, "src@"+long+":docs/a.md#L1-L3", "bare@"+long+":docs/crlf.md#L3")
	writeFile(t, file, good)
	if r := runScript(t, t.TempDir(), env, "check", file, "--hub", hub); r.code != 0 || !strings.HasSuffix(r.stdout, "OK: 1 lessons, goal draft\n") {
		t.Errorf("valid refs with a matching --hub: code %d stdout %q stderr %q", r.code, r.stdout, r.stderr)
	}
	r = runScript(t, t.TempDir(), env, "check", file, "--hub", other)
	if r.code != 1 || !strings.HasPrefix(r.stderr, "ERROR: --hub "+other+" is not the hub that holds ") {
		t.Errorf("--hub naming another hub: code %d stderr %q", r.code, r.stderr)
	}
}

func TestDistillScriptEvidenceCheckNeverFetchesOrRunsMirrorConfig(t *testing.T) {
	t.Parallel()
	requireGit(t)
	env := isolatedEnv(t)
	src, c := sourceRepo(t, env)
	git(t, env, "-C", src, "config", "uploadpack.allowFilter", "true")
	hub, file := newHub(t, lessonDoc(map[string]string{"part": c},
		"part@"+c[:7], "part@"+c[:12]+":docs/a.md#L1", "part@"+c[:12]+":docs/nope.md"))
	mirror := filepath.Join(mirrorsDir(hub), "part")
	url := "file://" + filepath.ToSlash(src)
	if runtime.GOOS == "windows" {
		url = "file:///" + filepath.ToSlash(src)
	}
	git(t, env, "clone", "-q", "--bare", "--filter=blob:none", url, mirror)

	// A planted config: any lazy fetch would go to a remote whose ssh command creates the marker.
	marker := filepath.Join(t.TempDir(), "marker")
	git(t, env, "-C", mirror, "config", "remote.origin.url", "ssh://example.invalid/x")
	git(t, env, "-C", mirror, "config", "core.sshCommand", "touch '"+filepath.ToSlash(marker)+"'")
	git(t, env, "-C", mirror, "config", "core.fsmonitor", "touch '"+filepath.ToSlash(marker)+"'")
	if err := os.RemoveAll(src); err != nil {
		t.Fatal(err)
	}

	r := runScript(t, t.TempDir(), env, "check", file)
	want := "ERROR: distill.yaml is invalid; nothing was written\n" +
		"  - refs: part@" + c[:12] + ":docs/a.md#L1: blob not fetched; run git fetch in the mirror\n" +
		"  - refs: part@" + c[:12] + ":docs/nope.md does not resolve in mirror part\n"
	if r.code != 1 || r.stderr != want {
		t.Errorf("code %d\n got:\n%s\nwant:\n%s", r.code, r.stderr, want)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the evidence check ran a command from the mirror's config")
	}
}

func TestDistillScriptEvidenceCheckRefusesLinkedOrTrackedMirrors(t *testing.T) {
	t.Parallel()
	requireGit(t)
	env := isolatedEnv(t)
	src, c := sourceRepo(t, env)

	t.Run("symlinked mirror", func(t *testing.T) {
		hub, file := newHub(t, lessonDoc(map[string]string{"link": c}, "link@"+c[:7]))
		real := filepath.Join(t.TempDir(), "real")
		git(t, env, "clone", "-q", "--bare", src, real)
		if err := os.MkdirAll(mirrorsDir(hub), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(real, filepath.Join(mirrorsDir(hub), "link")); err != nil {
			t.Skipf("cannot create symlinks here: %v", err)
		}
		r := runScript(t, t.TempDir(), env, "check", file)
		if r.code != 1 || !strings.Contains(r.stderr, "is not a commit in mirror link (refused: runtime/distill-lab/mirrors/link is a symbolic link") {
			t.Errorf("code %d stderr %q", r.code, r.stderr)
		}
	})

	t.Run("mirror tracked by the hub", func(t *testing.T) {
		hub, file := newHub(t, lessonDoc(map[string]string{"kept": c}, "kept@"+c[:7]))
		git(t, env, "clone", "-q", "--bare", src, filepath.Join(mirrorsDir(hub), "kept"))
		git(t, env, "init", "-q", hub)
		git(t, env, "-C", hub, "add", "-f", "runtime")
		r := runScript(t, t.TempDir(), env, "check", file)
		if r.code != 1 || !strings.Contains(r.stderr, "is not a commit in mirror kept (refused: the hub repository tracks it") {
			t.Errorf("code %d stderr %q", r.code, r.stderr)
		}
	})
}

func TestDistillScriptWriteRefusesSymlinks(t *testing.T) {
	t.Parallel()
	env := isolatedEnv(t)
	fixture := readFile(t, fixturePath(t, "phase0"))

	tempFiles := func(dir string) []string {
		matches, _ := filepath.Glob(filepath.Join(dir, ".distill.yaml.*.tmp"))
		return matches
	}

	t.Run("old fixed temp name is not followed", func(t *testing.T) {
		_, file := newHub(t, fixture)
		outside := filepath.Join(t.TempDir(), "outside.txt")
		writeFile(t, outside, "untouched\n")
		if err := os.Symlink(outside, filepath.Join(filepath.Dir(file), "distill.yaml.tmp")); err != nil {
			t.Skipf("cannot create symlinks here: %v", err)
		}
		if r := runScript(t, t.TempDir(), env, "format", file, "--no-where"); r.code != 0 {
			t.Fatalf("format: code %d stderr %q", r.code, r.stderr)
		}
		if readFile(t, outside) != "untouched\n" {
			t.Fatal("format wrote through a symlink next to distill.yaml")
		}
		if left := tempFiles(filepath.Dir(file)); len(left) != 0 {
			t.Errorf("temp files left: %v", left)
		}
	})

	t.Run("symlinked distill.yaml", func(t *testing.T) {
		_, file := newHub(t, "")
		target := filepath.Join(t.TempDir(), "target.yaml")
		writeFile(t, target, fixture)
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, file); err != nil {
			t.Skipf("cannot create symlinks here: %v", err)
		}
		r := runScript(t, t.TempDir(), env, "format", file, "--no-where")
		if r.code != 1 || !strings.Contains(r.stderr, "is a symbolic link; nothing was written") {
			t.Errorf("code %d stderr %q", r.code, r.stderr)
		}
		if readFile(t, target) != fixture || len(tempFiles(filepath.Dir(file))) != 0 {
			t.Error("a refused write changed the target or left a temp file")
		}
	})

	t.Run("symlinked .meta", func(t *testing.T) {
		hub, _ := newHub(t, fixture)
		realMeta := filepath.Join(t.TempDir(), "meta")
		writeFile(t, filepath.Join(realMeta, "distill.yaml"), fixture)
		skill := filepath.Join(hub, "skills", "default", "linked")
		if err := os.MkdirAll(skill, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realMeta, filepath.Join(skill, ".meta")); err != nil {
			t.Skipf("cannot create symlinks here: %v", err)
		}
		r := runScript(t, t.TempDir(), env, "decide", filepath.Join(skill, ".meta", "distill.yaml"), "1", "--state", "planned")
		if r.code != 1 || !strings.Contains(r.stderr, "is a symbolic link; nothing was written") {
			t.Errorf("code %d stderr %q", r.code, r.stderr)
		}
		if readFile(t, filepath.Join(realMeta, "distill.yaml")) != fixture || len(tempFiles(realMeta)) != 0 {
			t.Error("a refused write changed the file or left a temp file")
		}
	})
}

func TestDistillScriptNeedsAHubOnlyForTheEvidenceCheck(t *testing.T) {
	t.Parallel()
	env := isolatedEnv(t)
	file := filepath.Join(t.TempDir(), "distill.yaml")
	writeFile(t, file, readFile(t, fixturePath(t, "phase0")))

	r := runScript(t, t.TempDir(), env, "check", file)
	if r.code != 1 || !strings.HasPrefix(r.stderr, "ERROR: "+file+" is not inside a Skill Hub folder\nWHY: ") ||
		!strings.Contains(r.stderr, "\nFIX: run it on the distill.yaml inside the hub, or add --no-where\n") {
		t.Errorf("check outside a hub: code %d stderr %q", r.code, r.stderr)
	}
	assertNoStackTrace(t, r)
	for _, args := range [][]string{{"check", file, "--no-where"}, {"list", file}, {"format", file, "--no-where"}, {"decide", file, "1", "--state", "planned"}} {
		if r := runScript(t, t.TempDir(), env, args...); r.code != 0 {
			t.Errorf("%v outside a hub: code %d stderr %q", args, r.code, r.stderr)
		}
	}
}

// The embedded distill-lab folder copied alone, outside any hub, runs with Node only.
func TestDistillScriptBundleRunsOnItsOwn(t *testing.T) {
	t.Parallel()
	env := isolatedEnv(t)
	var bundle *systemskills.SystemSkill
	for _, skill := range systemskills.SystemSkills() {
		if skill.ID == systemskills.DistillSkillID {
			bundle = &skill
		}
	}
	if bundle == nil {
		t.Fatal("distill-lab bundle missing")
	}
	dir := filepath.Join(t.TempDir(), "distill-lab")
	for _, f := range bundle.Files {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(f.Path)), string(f.Content))
	}
	fixture := filepath.Join(t.TempDir(), "distill.yaml")
	writeFile(t, fixture, readFile(t, fixturePath(t, "phase0")))

	if r := runNode(t, dir, env, filepath.Join("scripts", "distill.mjs"), "--help"); r.code != 0 {
		t.Errorf("--help: code %d stderr %q", r.code, r.stderr)
	}
	if r := runNode(t, dir, env, filepath.Join("scripts", "distill.mjs"), "list", fixture); r.code != 0 || r.stdout == "" {
		t.Errorf("list: code %d stderr %q", r.code, r.stderr)
	}
	r := runNode(t, dir, env, filepath.Join("scripts", "distill.mjs"), "check", fixture)
	if r.code != 1 || !strings.Contains(r.stderr, "ERROR: ") || !strings.Contains(r.stderr, "\nWHY: ") || !strings.Contains(r.stderr, "\nFIX: ") {
		t.Errorf("check outside a hub: code %d stderr %q", r.code, r.stderr)
	}
	assertNoStackTrace(t, r)
}
