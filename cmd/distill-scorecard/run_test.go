package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestMain lets the test binary stand in for `claude`: the runner calls it with `-p`.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "-p" {
		os.Exit(stubClaude(os.Stdin, os.Stdout))
	}
	os.Exit(m.Run())
}

var keyPattern = regexp.MustCompile(`"key": "([^"]+)"`)

// stubClaude answers every family with a canned, valid reply built from the keys in the prompt.
func stubClaude(in *os.File, out *os.File) int {
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(in)
	prompt := buf.String()
	var keys []string
	seen := map[string]bool{}
	for _, m := range keyPattern.FindAllStringSubmatch(prompt, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			keys = append(keys, m[1])
		}
	}
	var reply any
	switch {
	case strings.Contains(prompt, "Task family: F1"):
		labels := []map[string]string{}
		for _, k := range keys {
			labels = append(labels, map[string]string{"key": k, "label": "port"})
		}
		reply = map[string]any{"top5": keys[:5], "labels": labels}
	case strings.Contains(prompt, "Task family: F2"):
		ls := []map[string]any{}
		for _, k := range keys {
			if strings.Contains(prompt, "(missing: `contrast`") {
				ls = append(ls, map[string]any{"key": k, "contrast": "new"})
			} else {
				ls = append(ls, map[string]any{"key": k, "relevance": 2, "facts": []string{"a"}, "evidence": 1, "effort": 1})
			}
		}
		reply = map[string]any{"lessons": ls}
	case strings.Contains(prompt, "Task family: F3"):
		as := []map[string]any{}
		for _, k := range keys {
			as = append(as, map[string]any{"key": k, "no_change_needed": false, "change": "add a sentence"})
		}
		reply = map[string]any{"answers": as}
	default:
		ss := []map[string]any{}
		for _, k := range keys {
			ss = append(ss, map[string]any{"key": k, "mechanism": 2, "in_goal": 2, "not_in_skill": 2, "placement": 2})
		}
		reply = map[string]any{"scores": ss}
	}
	text, _ := json.Marshal(reply)
	env, _ := json.Marshal(map[string]any{"type": "result", "is_error": false, "result": string(text)})
	_, _ = out.Write(env)
	return 0
}

func makeSandbox(t *testing.T) string {
	t.Helper()
	skill := t.TempDir()
	for name, body := range map[string]string{"SKILL.md": "# test-audit\nskill text\n", "CAMPAIGN.md": "# campaign\n"} {
		if err := os.WriteFile(filepath.Join(skill, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sandbox := filepath.Join(t.TempDir(), "sandbox")
	var out bytes.Buffer
	err := run([]string{"variants", "--in", fixture, "--skill-dir", skill, "--out", sandbox,
		"--rubric", "../../internal/systemskills/distill-lab/SKILL.md"}, &out, &out)
	if err != nil {
		t.Fatalf("variants: %v\n%s", err, out.String())
	}
	return sandbox
}

func TestVariantsChangeOnlyTheFieldUnderTest(t *testing.T) {
	sandbox := makeSandbox(t)
	read := func(variant string) []map[string]any {
		b, err := os.ReadFile(filepath.Join(sandbox, "variants", variant, "rep1.json"))
		if err != nil {
			t.Fatal(err)
		}
		var ls []map[string]any
		if err := json.Unmarshal(b, &ls); err != nil {
			t.Fatal(err)
		}
		return ls
	}
	full := read("full")
	for variant, gone := range map[string][]string{"no-notable": {"notable"}, "no-contrast": {"contrast"}, "no-score": {"score", "final_score"}} {
		ls := read(variant)
		if len(ls) != len(full) {
			t.Fatalf("%s has %d lessons, want %d", variant, len(ls), len(full))
		}
		for i, l := range ls {
			for _, f := range gone {
				if _, has := l[f]; has {
					t.Errorf("%s still has %s", variant, f)
				}
			}
			if l["key"] != full[i]["key"] {
				t.Fatalf("%s shuffles differently from full at %d", variant, i)
			}
			if _, has := l["decision"]; has {
				t.Errorf("%s leaks the decision", variant)
			}
		}
	}
	// A different repetition shuffles differently; full-sorted keeps the stored order.
	b, _ := os.ReadFile(filepath.Join(sandbox, "variants", "full", "rep2.json"))
	var rep2 []map[string]any
	_ = json.Unmarshal(b, &rep2)
	if fmt.Sprint(rep2[0]["key"]) == fmt.Sprint(full[0]["key"]) && fmt.Sprint(rep2[1]["key"]) == fmt.Sprint(full[1]["key"]) {
		t.Error("repetitions share one order")
	}
}

func TestVariantsRefuseASandboxInsideAHub(t *testing.T) {
	hub := makeHub(t)
	var out bytes.Buffer
	err := run([]string{"variants", "--in", fixture, "--skill-dir", t.TempDir(), "--out", filepath.Join(hub, "sb")}, &out, &out)
	if err == nil {
		t.Fatal("expected a refusal")
	}
}

func TestRubricIsCopiedVerbatimFromDistillLab(t *testing.T) {
	sandbox := makeSandbox(t)
	src, _ := os.ReadFile("../../internal/systemskills/distill-lab/SKILL.md")
	rubric, _ := os.ReadFile(filepath.Join(sandbox, "inputs", "rubric.txt"))
	if !strings.Contains(string(src), strings.TrimSpace(string(rubric))) {
		t.Error("rubric is not a verbatim part of the distill-lab SKILL.md")
	}
	if !strings.Contains(string(rubric), "Relevance (0-3)") || !strings.Contains(string(rubric), "already-covered") {
		t.Error("rubric lacks the score weights or the contrast values")
	}
}

func TestDryRunPrintsEveryCommandWithTheRestrictedEnvironment(t *testing.T) {
	t.Setenv("SOME_SECRET_TOKEN", "must-not-leak")
	sandbox := makeSandbox(t)
	var out bytes.Buffer
	if err := run([]string{"run", "--sandbox", sandbox, "--dry-run"}, &out, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	n := strings.Count(out.String(), "(prompt ")
	if n != 75 {
		t.Errorf("dry run planned %d commands, want 75", n)
	}
	for _, want := range []string{"--tools ''", "--strict-mcp-config", "--no-session-persistence", "--json-schema", "--max-budget-usd 0.50"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("commands lack %s", want)
		}
	}
	if strings.Contains(out.String(), "must-not-leak") {
		t.Error("a host secret reached the output")
	}
	env := runOptions{sandbox: sandbox}.environment()
	for _, e := range env {
		if strings.HasPrefix(e, "SOME_SECRET") {
			t.Error("environment passes a host variable")
		}
	}
}

func TestEndToEndWithAStubProducesAScorecard(t *testing.T) {
	sandbox := makeSandbox(t)
	var out bytes.Buffer
	stub, _ := os.Executable()
	if err := run([]string{"run", "--sandbox", sandbox, "--claude", stub, "--model", "stub-model"}, &out, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	metas, _ := filepath.Glob(filepath.Join(sandbox, "runs", "*", "meta.json"))
	if len(metas) != 75 {
		t.Fatalf("finished %d runs, want 75", len(metas))
	}
	// A second invocation skips finished runs.
	out.Reset()
	if err := run([]string{"run", "--sandbox", sandbox, "--claude", "/nonexistent", "--model", "stub-model"}, &out, &out); err != nil {
		t.Fatalf("resume failed: %v\n%s", err, out.String())
	}

	statsFile := filepath.Join(t.TempDir(), "stats.json")
	if err := run([]string{"stats", "--in", fixture, "--json", "--out", statsFile}, &out, &out); err != nil {
		t.Fatal(err)
	}
	reportDir := filepath.Join(t.TempDir(), "report")
	if err := run([]string{"report", "--stats", statsFile, "--runs", sandbox, "--out", reportDir}, &out, &out); err != nil {
		t.Fatal(err)
	}
	md, _ := os.ReadFile(filepath.Join(reportDir, "scorecard.md"))
	for _, want := range []string{"| notable |", "| contrast |", "| Score weights as a block |", "Limits:", "Run manifest"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("scorecard lacks %q", want)
		}
	}
	var sum Summary
	b, _ := os.ReadFile(filepath.Join(reportDir, "inuse", "summary.json"))
	if err := json.Unmarshal(b, &sum); err != nil {
		t.Fatal(err)
	}
	// The stub gives every lesson the same answer, so triage cannot tell variants apart and the
	// judge score is a constant 2.
	if got := sum.Metrics["F3"]["judge_mean"]["full"].Mean; got != 2 {
		t.Errorf("judge mean = %v, want 2", got)
	}
	if got := sum.Metrics["F1"]["recall_at_5_planned"]["full"]; got == nil || len(got.ByRep) != 5 {
		t.Errorf("F1 recall cell = %#v", got)
	}
	if sum.Runs["F3"]["ok"] != 20 {
		t.Errorf("F3 ok runs = %d, want 20", sum.Runs["F3"]["ok"])
	}
}

func TestRunStopsAFamilyWhenTooManyRunsFail(t *testing.T) {
	sandbox := makeSandbox(t)
	var out bytes.Buffer
	err := run([]string{"run", "--sandbox", sandbox, "--claude", "/nonexistent", "--model", "m"}, &out, &out)
	if err == nil && !strings.Contains(out.String(), "stopping family F1") {
		t.Fatalf("expected the family to stop:\n%s", out.String())
	}
	metas, _ := filepath.Glob(filepath.Join(sandbox, "runs", "f1-*", "meta.json"))
	if len(metas) >= 25 {
		t.Errorf("all %d F1 runs were attempted", len(metas))
	}
}

func TestStructuredOutputAcceptsBothReplyShapes(t *testing.T) {
	a, err := structuredOutput([]byte(`{"is_error":false,"structured_output":{"x":1}}`))
	if err != nil || string(a) != `{"x":1}` {
		t.Errorf("structured_output: %s %v", a, err)
	}
	b, err := structuredOutput([]byte("{\"is_error\":false,\"result\":\"```json\\n{\\\"x\\\":2}\\n```\"}"))
	if err != nil || string(b) != `{"x":2}` {
		t.Errorf("result text: %s %v", b, err)
	}
	if _, err := structuredOutput([]byte(`{"is_error":true,"result":"boom"}`)); err == nil {
		t.Error("an error result must fail")
	}
	if _, err := structuredOutput([]byte(`not json`)); err == nil {
		t.Error("non-JSON stdout must fail")
	}
}

func TestValidateOutputRejectsIncompleteAnswers(t *testing.T) {
	r := plannedRun{Family: "F1", Expected: []string{"a", "b", "c", "d", "e", "f"}}
	bad := `{"top5":["a","b","c","d","x"],"labels":[]}`
	if err := validateOutput(r, json.RawMessage(bad)); err == nil {
		t.Error("an unknown top5 key must be rejected")
	}
	short := `{"top5":["a","b","c","d","e"],"labels":[{"key":"a","label":"port"}]}`
	if err := validateOutput(r, json.RawMessage(short)); err == nil {
		t.Error("labels covering one of six lessons must be rejected")
	}
	j := plannedRun{Family: "JUDGE", Judge: true, Expected: []string{"a"}}
	if err := validateOutput(j, json.RawMessage(`{"scores":[{"key":"a","mechanism":4,"in_goal":1,"not_in_skill":1,"placement":1}]}`)); err == nil {
		t.Error("a judge score above 3 must be rejected")
	}
}
