package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestSkillReviewHumanAndJSON(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	contentFile := filepath.Join(t.TempDir(), "content.md")
	_ = os.WriteFile(contentFile, []byte("---\nname: rev-test\ndescription: Review test skill.\n---\n\n# Review Test\n\nInstructions.\n"), 0o600)

	code, _, stderr := runCLI(t, "skill", "create", "rev-test", "--workspace", root,
		"--collection", "core", "--name", "Review Test", "--description", "Review test skill.",
		"--trigger", "review it", "--not-for", "unrelated", "--min-scope", "single_step",
		"--content-file", contentFile, "--yes")
	if code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}

	// 1. Human review output
	code, stdout, stderr := runCLI(t, "skill", "review", "rev-test", "--workspace", root)
	if code != 0 {
		t.Fatalf("review failed (exit %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "Review:") || !strings.Contains(stdout, "Review Test (rev-test)") {
		t.Errorf("review missing title line: %s", stdout)
	}
	if !strings.Contains(stdout, "State:") || !strings.Contains(stdout, "draft") {
		t.Errorf("review missing draft state: %s", stdout)
	}
	if !strings.Contains(stdout, "Files:") {
		t.Errorf("review missing files line: %s", stdout)
	}
	if !strings.Contains(stdout, "Origin:") || !strings.Contains(stdout, "local authoring") {
		t.Errorf("review missing local authoring origin: %s", stdout)
	}
	if !strings.Contains(stdout, "Next:") {
		t.Errorf("review missing Next action: %s", stdout)
	}

	// 2. Verbose review output
	code, stdout, stderr = runCLI(t, "skill", "review", "rev-test", "--workspace", root, "--verbose")
	if code != 0 {
		t.Fatalf("verbose review failed (exit %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "Catalog facts") {
		t.Errorf("verbose review missing catalog facts: %s", stdout)
	}
	if !strings.Contains(stdout, "Canonical entrypoint:") {
		t.Errorf("verbose review missing canonical entrypoint: %s", stdout)
	}

	// 3. JSON review output
	var jsonBuf, jsonErrBuf bytes.Buffer
	code = Run([]string{"skill", "review", "rev-test", "--workspace", root, "--json"}, &jsonBuf, &jsonErrBuf)
	if code != 0 {
		t.Fatalf("json review failed (exit %d): %s", code, jsonErrBuf.String())
	}
	var result app.SkillReviewResult
	if err := json.Unmarshal(jsonBuf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal review JSON: %v", err)
	}
	if result.SkillID != "rev-test" || result.Collection != "core" || result.LifecycleState != "draft" {
		t.Errorf("unexpected review JSON content: %#v", result)
	}

	// 4. Missing skill review returns error exit 2
	code, _, stderr = runCLI(t, "skill", "review", "non-existent", "--workspace", root)
	if code != 2 {
		t.Fatalf("expected exit 2 on missing skill review, got %d", code)
	}
	if !strings.Contains(stderr, "skill not found") {
		t.Errorf("expected skill not found in stderr, got: %s", stderr)
	}
}

func TestSkillReviewPrintsScriptApprovalCommandForThirdPartyScripts(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	contentFile := filepath.Join(t.TempDir(), "content.md")
	_ = os.WriteFile(contentFile, []byte("---\nname: trust-test\ndescription: Trust test skill.\n---\n\n# Trust Test\n"), 0o600)
	code, _, stderr := runCLI(t, "skill", "create", "trust-test", "--workspace", root,
		"--collection", "core", "--name", "Trust Test", "--description", "Trust test skill.",
		"--trigger", "trust it", "--not-for", "unrelated", "--min-scope", "single_step",
		"--content-file", contentFile, "--yes")
	if code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}
	skillDir := filepath.Join(root, "skills", "core", "trust-test")
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "run.sh"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(skillDir, "skill.meta.yaml")
	meta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(meta), "provenance:\n    created_by: skillhub\n", "provenance:\n    created_by: skillhub\n    source_id: upstream-source\n", 1)
	if updated == string(meta) {
		t.Fatalf("fixture metadata has an unexpected provenance block:\n%s", meta)
	}
	if err := os.WriteFile(metaPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	var jsonBuf, jsonErrBuf bytes.Buffer
	if code := Run([]string{"skill", "review", "trust-test", "--workspace", root, "--json"}, &jsonBuf, &jsonErrBuf); code != 0 {
		t.Fatalf("json review failed (exit %d): %s", code, jsonErrBuf.String())
	}
	var result app.SkillReviewResult
	if err := json.Unmarshal(jsonBuf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	trust := result.ContentTrust
	if !trust.ThirdParty || trust.Approved || !trust.RequiresReview() || !strings.HasPrefix(trust.ContentDigest, "sha256:") {
		t.Fatalf("content trust = %#v", trust)
	}

	code, stdout, stderr := runCLI(t, "skill", "review", "trust-test", "--workspace", root)
	if code != 0 {
		t.Fatalf("review failed (exit %d): %s", code, stderr)
	}
	want := "skillhub skill edit trust-test --approve-content " + trust.ContentDigest
	if !strings.Contains(stdout, "Content trust") || !strings.Contains(stdout, want) {
		t.Fatalf("review output missing approval command %q:\n%s", want, stdout)
	}
	if !strings.Contains(stdout, "Never approved") {
		t.Fatalf("review output must say the skill was never approved:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Runtime") || !strings.Contains(stdout, "Interpreters") || !strings.Contains(stdout, "sh") {
		t.Fatalf("review output missing runtime hints:\n%s", stdout)
	}
	if !result.RuntimeHints.MissingRuntimeBlock || !strings.Contains(stdout, "runtime block") {
		t.Fatalf("review output missing runtime block suggestion:\n%s", stdout)
	}
}

func TestSkillReviewSummarizesChangesSinceApproval(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	contentFile := filepath.Join(t.TempDir(), "content.md")
	_ = os.WriteFile(contentFile, []byte("---\nname: delta-test\ndescription: Delta test skill.\n---\n\n# Delta Test\n"), 0o600)
	if code, _, stderr := runCLI(t, "skill", "create", "delta-test", "--workspace", root,
		"--collection", "core", "--name", "Delta Test", "--description", "Delta test skill.",
		"--trigger", "delta it", "--not-for", "unrelated", "--min-scope", "single_step",
		"--content-file", contentFile, "--yes"); code != 0 {
		t.Fatalf("create failed (exit %d): %s", code, stderr)
	}
	skillDir := filepath.Join(root, "skills", "core", "delta-test")
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(skillDir, "scripts", "run.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(skillDir, "skill.meta.yaml")
	meta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	thirdParty := strings.Replace(string(meta), "provenance:\n    created_by: skillhub\n", "provenance:\n    created_by: skillhub\n    source_id: upstream-source\n", 1)
	if thirdParty == string(meta) {
		t.Fatalf("fixture metadata has an unexpected provenance block:\n%s", meta)
	}
	if err := os.WriteFile(metaPath, []byte(thirdParty), 0o644); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", root, "-c", "user.email=t@example.com", "-c", "user.name=T"}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	git("init", "-q")
	var jsonBuf bytes.Buffer
	if code := Run([]string{"skill", "review", "delta-test", "--workspace", root, "--json"}, &jsonBuf, &bytes.Buffer{}); code != 0 {
		t.Fatalf("json review failed (exit %d)", code)
	}
	var result app.SkillReviewResult
	if err := json.Unmarshal(jsonBuf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	approved := strings.Replace(thirdParty, "quality:\n", "quality:\n    content_reviewed_digest: "+result.ContentTrust.ContentDigest+"\n", 1)
	if approved == thirdParty {
		t.Fatalf("fixture metadata has no quality block:\n%s", thirdParty)
	}
	if err := os.WriteFile(metaPath, []byte(approved), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "approve")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runCLI(t, "skill", "review", "delta-test", "--workspace", root)
	if code != 0 {
		t.Fatalf("review failed (exit %d): %s", code, stderr)
	}
	for _, want := range []string{"Changes since the last approval", "scripts changed", "scripts/run.sh", "git diff ", "-- skills/core/delta-test", "--approve-content"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("review output missing %q:\n%s", want, stdout)
		}
	}
	if strings.Index(stdout, "Changes since the last approval") > strings.Index(stdout, "--approve-content") {
		t.Fatalf("the change summary must precede the approve command:\n%s", stdout)
	}
}
