package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"gopkg.in/yaml.v3"
)

func TestSkillCLIEndToEndPreviewConfirmActivateShowAndArchive(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init = %d: %s", code, stderr.String())
	}
	contentFile := filepath.Join(t.TempDir(), "consumer-review.md")
	if err := os.WriteFile(contentFile, []byte("---\nname: consumer-review\ndescription: Review consumers\n---\n\n# Consumer Review\n\nReview consumer workflows and verify error handling.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := []string{"skill", "create", "--workspace", root, "--id", "consumer-review", "--collection", "software", "--name", "Consumer Review", "--description", "Review consumers", "--trigger", "review consumers", "--not-for", "design brokers", "--min-scope", "multi_step", "--content-file", contentFile, "--full-diff", "--json"}
	stdout.Reset()
	stderr.Reset()
	if code := Run(base, &stdout, &stderr); code != 0 {
		t.Fatalf("preview = %d: %s", code, stderr.String())
	}
	var preview app.SkillProposal
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Status != app.StatusActionRequired || preview.FullDiff == "" {
		t.Fatalf("preview = %#v", preview)
	}
	if _, err := os.Stat(filepath.Join(root, "skills", "software", "consumer-review", ".meta", "skill.yaml")); !os.IsNotExist(err) {
		t.Fatalf("preview changed canonical state: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run(append(base, "--yes"), &stdout, &stderr); code != 0 {
		t.Fatalf("create = %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var created app.SkillMutationResult
	if err := json.Unmarshal(stdout.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.OperationID == "" || created.CatalogSnapshot == "" || created.Generation == "" || created.ActiveLocally || !created.GitDirty {
		t.Fatalf("create result = %#v", created)
	}

	for _, command := range []string{"activate", "deprecate", "archive"} {
		stdout.Reset()
		stderr.Reset()
		if code := Run([]string{"skill", command, "consumer-review", "--workspace", root, "--yes", "--json"}, &stdout, &stderr); code != 0 {
			t.Fatalf("%s = %d: stdout=%s stderr=%s", command, code, stdout.String(), stderr.String())
		}
		if command == "activate" {
			stdout.Reset()
			stderr.Reset()
			if code := Run([]string{"skill", "show", "consumer-review", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
				t.Fatalf("show = %d: %s", code, stderr.String())
			}
			var read app.SkillReadResult
			if err := json.Unmarshal(stdout.Bytes(), &read); err != nil || read.Manifest.Status != "active" || !strings.Contains(read.Content, "Consumer Review") {
				t.Fatalf("show result = %#v, %v", read, err)
			}
		}
	}
}

func TestSkillCLIConfirmsStoredExactProposalAndRejectsInterveningEdit(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	create := []string{"skill", "create", "--workspace", root, "--id", "stored-proposal", "--collection", "software", "--name", "Stored Proposal", "--description", "Confirm exact bytes", "--trigger", "confirm proposal", "--not-for", "other", "--min-scope", "single_step", "--json"}
	stdout.Reset()
	stderr.Reset()
	if code := Run(create, &stdout, &stderr); code != 0 {
		t.Fatalf("preview = %d: %s", code, stderr.String())
	}
	var preview app.SkillProposal
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	pins := preview.Confirmation.Confirmation.Pins
	artifact := filepath.Join(root, "runtime", "proposals", pins.ProposalID+".json")
	if info, err := os.Stat(artifact); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		t.Fatalf("proposal artifact mode = %v, %v", info, err)
	}
	confirm := []string{"skill", "confirm", "--workspace", root, "--proposal", pins.ProposalID, "--proposal-digest", pins.ProposalDigest, "--base-version", pins.BaseVersion, "--json"}
	stdout.Reset()
	stderr.Reset()
	if code := Run(confirm, &stdout, &stderr); code != 0 {
		t.Fatalf("confirm = %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "skills", "software", "stored-proposal", ".meta", "skill.yaml")); err != nil {
		t.Fatal(err)
	}

	description := "Pinned edit"
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"skill", "edit", "stored-proposal", "--workspace", root, "--description", description, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("edit preview = %d: %s", code, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	pins = preview.Confirmation.Confirmation.Pins
	if err := os.WriteFile(filepath.Join(root, "skills", "software", "stored-proposal", "SKILL.md"), []byte("# external valid edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	code := Run([]string{"skill", "confirm", "--workspace", root, "--proposal", pins.ProposalID, "--proposal-digest", pins.ProposalDigest, "--base-version", pins.BaseVersion, "--json"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("stale confirm = %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var stale app.SkillMutationResult
	if err := json.Unmarshal(stdout.Bytes(), &stale); err != nil || stale.Error == nil || stale.Error.Code != app.ErrorStaleProposal {
		t.Fatalf("stale result = %#v, %v", stale, err)
	}
	if len(stale.SuggestedActions) != 1 || stale.SuggestedActions[0].CLI == "" {
		t.Fatalf("stale proposal has no runnable recovery: %#v", stale.SuggestedActions)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run(splitCLI(stale.SuggestedActions[0].CLI)[1:], &stdout, &stderr); code != 0 {
		t.Fatalf("stale proposal recovery = %d: %s %s", code, stdout.String(), stderr.String())
	}
	var review app.SkillReviewResult
	if err := json.Unmarshal(stdout.Bytes(), &review); err != nil || review.SkillID != "stored-proposal" {
		t.Fatalf("stale proposal recovery did not inspect the affected skill: %s, %v", stdout.String(), err)
	}
}

func TestSkillCLIExternalEditorUsesValidatedMutationFlow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	create := []string{"skill", "create", "--workspace", root, "--id", "editor-skill", "--collection", "software", "--name", "Editor Skill", "--description", "Editor flow", "--trigger", "edit skill", "--not-for", "other work", "--min-scope", "single_step", "--yes"}
	stdout.Reset()
	stderr.Reset()
	if code := Run(create, &stdout, &stderr); code != 0 {
		t.Fatalf("create = %d: %s", code, stderr.String())
	}
	editor := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nprintf '\\nEdited externally.\\n' >> \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", editor)
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"skill", "edit", "editor-skill", "--workspace", root, "--editor", "--yes", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("edit = %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	contents, err := os.ReadFile(filepath.Join(root, "skills", "software", "editor-skill", "SKILL.md"))
	if err != nil || !strings.Contains(string(contents), "Edited externally") {
		t.Fatalf("edited contents = %q, %v", contents, err)
	}
	var result app.SkillMutationResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Generation == "" || result.OperationID == "" {
		t.Fatalf("edit result = %#v, %v", result, err)
	}
}

func TestSkillCLIStoredEditorProposalDoesNotReopenEditorAtConfirm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	if code := Run([]string{"skill", "create", "--workspace", root, "--id", "editor-confirm", "--collection", "software", "--name", "Editor Confirm", "--description", "Editor proposal", "--trigger", "edit", "--not-for", "other", "--min-scope", "single_step", "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	counter := filepath.Join(t.TempDir(), "count")
	editor := filepath.Join(t.TempDir(), "editor.sh")
	script := "#!/bin/sh\nprintf x >> " + counter + "\nprintf '\\nEdited once.\\n' >> \"$1\"\n"
	if err := os.WriteFile(editor, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", editor)
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"skill", "edit", "editor-confirm", "--workspace", root, "--editor", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("preview = %d: %s", code, stderr.String())
	}
	var preview app.SkillProposal
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	pins := preview.Confirmation.Confirmation.Pins
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"skill", "confirm", "--workspace", root, "--proposal", pins.ProposalID, "--proposal-digest", pins.ProposalDigest, "--base-version", pins.BaseVersion, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("confirm = %d: %s", code, stderr.String())
	}
	calls, err := os.ReadFile(counter)
	if err != nil || string(calls) != "x" {
		t.Fatalf("editor calls = %q, %v", calls, err)
	}
}

func TestSkillCLIRoutingExamplesAndScriptsApproval(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	run := func(args ...string) int {
		stdout.Reset()
		stderr.Reset()
		return Run(args, &stdout, &stderr)
	}
	if code := run("init", root, "--yes"); code != 0 {
		t.Fatalf("init = %d: %s", code, stderr.String())
	}
	readMetadata := func() map[string]any {
		t.Helper()
		metaPath := filepath.Join(root, "skills", "software", "pr-review", ".meta", "skill.yaml")
		if _, err := os.Stat(metaPath); os.IsNotExist(err) {
			metaPath = filepath.Join(root, "skills", "software", "pr-review", "skill.meta.yaml")
		}
		contents, err := os.ReadFile(metaPath)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := yaml.Unmarshal(contents, &document); err != nil {
			t.Fatal(err)
		}
		return document
	}
	section := func(document map[string]any, key string) map[string]any {
		value, _ := document[key].(map[string]any)
		return value
	}

	if code := run("skill", "create", "pr-review", "--workspace", root, "--collection", "software", "--name", "PR Review", "--description", "Review pull requests",
		"--trigger", "review pull request", "--min-scope", "single_step",
		"--example", "review my pull request", "--example", "check this diff before merge",
		"--counter-example", "write release notes", "--yes", "--json"); code != 0 {
		t.Fatalf("create = %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	routing := section(readMetadata(), "routing")
	if !reflect.DeepEqual(routing["examples"], []any{"review my pull request", "check this diff before merge"}) || !reflect.DeepEqual(routing["counter_examples"], []any{"write release notes"}) {
		t.Fatalf("created routing = %#v", routing)
	}

	if code := run("skill", "edit", "pr-review", "--workspace", root, "--trigger", "review a pull request", "--yes", "--json"); code != 0 {
		t.Fatalf("edit triggers = %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	routing = section(readMetadata(), "routing")
	if !reflect.DeepEqual(routing["triggers"], []any{"review a pull request"}) || !reflect.DeepEqual(routing["examples"], []any{"review my pull request", "check this diff before merge"}) {
		t.Fatalf("trigger edit must keep examples: %#v", routing)
	}

	if code := run("skill", "edit", "pr-review", "--workspace", root, "--counter-example", "draft a changelog", "--yes", "--json"); code != 0 {
		t.Fatalf("edit counter examples = %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	routing = section(readMetadata(), "routing")
	if !reflect.DeepEqual(routing["counter_examples"], []any{"draft a changelog"}) || !reflect.DeepEqual(routing["triggers"], []any{"review a pull request"}) || len(routing["examples"].([]any)) != 2 {
		t.Fatalf("counter-example edit = %#v", routing)
	}

	digest := "sha256:" + strings.Repeat("c", 64)
	if code := run("skill", "edit", "pr-review", "--workspace", root, "--approve-content", digest, "--yes", "--json"); code != 0 {
		t.Fatalf("approve scripts = %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	document := readMetadata()
	if got := section(document, "quality")["content_reviewed_digest"]; got != digest {
		t.Fatalf("quality.content_reviewed_digest = %#v", got)
	}
	if routing := section(document, "routing"); !reflect.DeepEqual(routing["counter_examples"], []any{"draft a changelog"}) {
		t.Fatalf("scripts approval must not touch routing: %#v", routing)
	}
}

func TestParseSkillFlagsValidatesExamplesAndScriptsApproval(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init = %d: %s", code, stderr.String())
	}
	valid := "sha256:" + strings.Repeat("a", 64)
	accepted, err := parseSkillFlags("edit", []string{"pr-review", "--workspace", root, "--example", "a", "--example", "b", "--counter-example", "c"})
	if err != nil {
		t.Fatalf("example-only edit rejected: %v", err)
	}
	if !reflect.DeepEqual(accepted.examples, []string{"a", "b"}) || !reflect.DeepEqual(accepted.counterExamples, []string{"c"}) {
		t.Fatalf("parsed examples = %#v / %#v", accepted.examples, accepted.counterExamples)
	}
	if _, err := parseSkillFlags("edit", []string{"pr-review", "--workspace", root, "--approve-content", valid}); err != nil {
		t.Fatalf("approval-only edit rejected: %v", err)
	}
	for name, tc := range map[string]struct {
		subcommand string
		args       []string
		want       string
	}{
		"malformed digest":     {"edit", []string{"pr-review", "--approve-content", "sha256:ABC"}, "--approve-content requires a digest"},
		"digest without value": {"edit", []string{"pr-review", "--approve-content"}, "--approve-content requires a value"},
		"create approval":      {"create", []string{"pr-review", "--collection", "software", "--name", "n", "--description", "d", "--approve-content", valid}, "--approve-content is available only for skill edit"},
		"activate example":     {"activate", []string{"pr-review", "--example", "a"}, "activate accepts only"},
		"activate approval":    {"activate", []string{"pr-review", "--approve-content", valid}, "activate accepts only"},
		"list counter example": {"list", []string{"--counter-example", "a"}, "list accepts only"},
		"confirm example":      {"confirm", []string{"PROP-1", "--example", "a"}, "confirm accepts only"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseSkillFlags(tc.subcommand, append(tc.args, "--workspace", root))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSkillEditRuntimeFileSetsAndRemovesRuntimeBlock(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	run := func(args ...string) int {
		stdout.Reset()
		stderr.Reset()
		return Run(args, &stdout, &stderr)
	}
	if code := run("init", root, "--yes"); code != 0 {
		t.Fatalf("init = %d: %s", code, stderr.String())
	}
	if code := run("skill", "create", "rt-edit", "--workspace", root, "--collection", "software", "--name", "RT Edit", "--description", "Runtime edit fixture", "--yes", "--json"); code != 0 {
		t.Fatalf("create = %d: %s %s", code, stdout.String(), stderr.String())
	}
	runtimeBlock := func() any {
		t.Helper()
		metaPath := filepath.Join(root, "skills", "software", "rt-edit", ".meta", "skill.yaml")
		if _, err := os.Stat(metaPath); os.IsNotExist(err) {
			metaPath = filepath.Join(root, "skills", "software", "rt-edit", "skill.meta.yaml")
		}
		contents, err := os.ReadFile(metaPath)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := yaml.Unmarshal(contents, &document); err != nil {
			t.Fatal(err)
		}
		return document["runtime"]
	}
	writeRuntimeFile := func(contents string) string {
		path := filepath.Join(t.TempDir(), "runtime.yaml")
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	set := writeRuntimeFile("requires:\n  bins: [python3]\n  env: [API_TOKEN]\nsetup:\n  check: python3 --version\n")
	if code := run("skill", "edit", "rt-edit", "--workspace", root, "--runtime-file", set, "--yes", "--json"); code != 0 {
		t.Fatalf("set runtime = %d: %s %s", code, stdout.String(), stderr.String())
	}
	block, _ := runtimeBlock().(map[string]any)
	if setup, _ := block["setup"].(map[string]any); setup["check"] != "python3 --version" {
		t.Fatalf("runtime block = %#v", block)
	}

	invalid := writeRuntimeFile("requires:\n  bins: [\"not a bin\"]\n")
	if code := run("skill", "edit", "rt-edit", "--workspace", root, "--runtime-file", invalid, "--yes", "--json"); code == 0 {
		t.Fatal("an invalid runtime block must be rejected")
	}
	if block, _ := runtimeBlock().(map[string]any); block["setup"] == nil {
		t.Fatalf("a rejected edit must keep the block: %#v", block)
	}

	if code := run("skill", "edit", "rt-edit", "--workspace", root, "--runtime-file", writeRuntimeFile("{}\n"), "--yes", "--json"); code != 0 {
		t.Fatalf("remove runtime = %d: %s %s", code, stdout.String(), stderr.String())
	}
	if got := runtimeBlock(); got != nil {
		t.Fatalf("runtime block must be removed: %#v", got)
	}

	if _, err := parseSkillFlags("show", []string{"rt-edit", "--workspace", root, "--runtime-file", set}); err == nil || !strings.Contains(err.Error(), "--runtime-file is available only for skill edit") {
		t.Fatalf("err = %v", err)
	}
	if code := run("skill", "edit", "rt-edit", "--workspace", root, "--runtime-file", writeRuntimeFile("# nothing\n"), "--yes"); code == 0 {
		t.Fatal("an empty runtime file must be rejected")
	}
}
