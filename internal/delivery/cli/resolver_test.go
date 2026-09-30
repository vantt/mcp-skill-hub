package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
)

func TestResolveCLIExposesStructuredApplicationContractWithoutActivating(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init=%d: %s", code, stderr.String())
	}
	writeResolverFixture(t, root, "skills/core/code-review/skill.meta.yaml", "schema_version: 1\nid: code-review\nname: Code Review\nstatus: active\ndescription: Review code changes for defects.\naliases: [patch inspection]\nrouting:\n  operations: [review]\n  triggers: [review code changes pull requests]\n  not_for: [write marketing prose]\n  min_scope: multi_step\nquality:\n  reviewed: true\n")
	writeResolverFixture(t, root, "skills/core/code-review/SKILL.md", "---\nname: code-review\ndescription: Review code changes for defects.\n---\n\n# Code Review\nInspect evidence.\n")
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"rebuild", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("rebuild=%d: %s %s", code, stdout.String(), stderr.String())
	}
	requestPath := filepath.Join(t.TempDir(), "request.json")
	request := []byte(`{"schema_version":"1","request_id":"req-cli","task":{"description":"inspect a patch for code defects","scope":"multi_step"},"operation":"review"}`)
	if err := os.WriteFile(requestPath, request, 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"resolve", "--workspace", root, "--request", requestPath, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("resolve=%d: %s %s", code, stdout.String(), stderr.String())
	}
	var response resolverpkg.Response
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != resolverpkg.StatusResolved || response.Primary == nil || response.Primary.ID != "code-review" {
		t.Fatalf("response=%#v", response)
	}
	if response.Primary.Applicability == "" || response.CatalogSnapshot == "" || response.PolicyRevision == "" || response.Primary.Version == "" || response.Primary.URI == "" {
		t.Fatalf("response lacks MCP-ready evidence and distribution pins: %#v", response)
	}
	entry, err := (app.DistributionService{}).GetSkill(t.Context(), root, response.Primary.URI)
	if err != nil || entry.Version != response.Primary.Version {
		t.Fatalf("CLI resolution cannot be loaded through skills/get semantics: entry=%#v err=%v", entry, err)
	}
}

func TestResolveCLIRejectsUnknownRequestFields(t *testing.T) {
	requestPath := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(requestPath, []byte(`{"schema_version":"1","request_id":"req","task":{"description":"review code"},"domain":"client-taxonomy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"resolve", "--workspace", t.TempDir(), "--request", requestPath, "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"code":"invalid_request"`)) {
		t.Fatalf("unexpected error: %s", stdout.String())
	}
}

func writeResolverFixture(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveCLIClarificationRoundTripAndErrorClassification(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init=%d: %s", code, stderr.String())
	}
	writeResolverFixture(t, root, "skills/core/code-review/skill.meta.yaml", "schema_version: 1\nid: code-review\nname: Code Review\nstatus: active\ndescription: Review code changes for defects.\nrouting:\n  operations: [review]\n  triggers: [review code changes pull requests]\n  not_for: [write marketing prose]\n  min_scope: multi_step\nquality:\n  reviewed: true\n")
	writeResolverFixture(t, root, "skills/core/code-review/SKILL.md", "---\nname: code-review\ndescription: Review code changes for defects.\n---\n\n# Code Review\n")
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"rebuild", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("rebuild=%d: %s %s", code, stdout.String(), stderr.String())
	}
	resolve := func(request string) (int, string) {
		path := filepath.Join(t.TempDir(), "request.json")
		if err := os.WriteFile(path, []byte(request), 0o600); err != nil {
			t.Fatal(err)
		}
		stdout.Reset()
		stderr.Reset()
		code := Run([]string{"resolve", "--workspace", root, "--request", path, "--json"}, &stdout, &stderr)
		return code, stdout.String()
	}

	code, output := resolve(`{"schema_version":"1","request_id":"req-1","task":{"description":"review code changes for pull requests"},"operation":"review"}`)
	if code != 0 {
		t.Fatalf("first resolve=%d: %s", code, output)
	}
	var issued resolverpkg.Response
	if err := json.Unmarshal([]byte(output), &issued); err != nil || issued.Status != resolverpkg.StatusNeedsContext || issued.Question == nil {
		t.Fatalf("first response=%s err=%v", output, err)
	}
	prior := func(resolutionID string) string {
		encoded, _ := json.Marshal(map[string]any{"resolution_id": resolutionID, "context_revision": issued.ContextRevision, "kind": "clarification", "question_id": issued.Question.ID, "answer": "multi_step"})
		return `{"schema_version":"1","request_id":"req-2","task":{"description":"review code changes for pull requests"},"operation":"review","prior":` + string(encoded) + `}`
	}

	code, output = resolve(prior(issued.ResolutionID))
	var answered resolverpkg.Response
	if code != 0 || json.Unmarshal([]byte(output), &answered) != nil || answered.Status != resolverpkg.StatusResolved || answered.Primary == nil || answered.Primary.ID != "code-review" {
		t.Fatalf("answered resolve=%d: %s", code, output)
	}

	code, output = resolve(prior("res_forged"))
	if code != 2 || !bytes.Contains([]byte(output), []byte(`"code":"unknown_resolution"`)) {
		t.Fatalf("forged prior resolve=%d: %s", code, output)
	}

	writeResolverFixture(t, root, "skills/core/code-review/SKILL.md", "---\nname: code-review\ndescription: Review code changes for defects.\n---\n\n# Code Review\nEdited.\n")
	code, output = resolve(prior(issued.ResolutionID))
	if code != 2 || !bytes.Contains([]byte(output), []byte(`"code":"index_stale"`)) {
		t.Fatalf("stale catalog resolve=%d: %s", code, output)
	}
}
