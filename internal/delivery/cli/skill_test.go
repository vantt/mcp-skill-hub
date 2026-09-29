package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestSkillCLIEndToEndPreviewConfirmActivateShowAndArchive(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init = %d: %s", code, stderr.String())
	}
	base := []string{"skill", "create", "--workspace", root, "--id", "consumer-review", "--collection", "software", "--name", "Consumer Review", "--description", "Review consumers", "--trigger", "review consumers", "--not-for", "design brokers", "--min-scope", "multi_step", "--full-diff", "--json"}
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
	if _, err := os.Stat(filepath.Join(root, "skills", "software", "consumer-review", "skill.meta.yaml")); !os.IsNotExist(err) {
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
	if created.OperationID == "" || created.CatalogSnapshot == "" || created.Generation == "" || !created.ActiveLocally || !created.GitDirty {
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
	if info, err := os.Stat(artifact); err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("proposal artifact mode = %v, %v", info, err)
	}
	confirm := []string{"skill", "confirm", "--workspace", root, "--proposal", pins.ProposalID, "--proposal-digest", pins.ProposalDigest, "--base-version", pins.BaseVersion, "--json"}
	stdout.Reset()
	stderr.Reset()
	if code := Run(confirm, &stdout, &stderr); code != 0 {
		t.Fatalf("confirm = %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "skills", "software", "stored-proposal", "skill.meta.yaml")); err != nil {
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
