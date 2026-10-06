package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
)

func TestSkillAddLocalPreviewAndConfirm(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	temp := t.TempDir()
	sourceDir := filepath.Join(temp, "pdf-tools")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	skillContent := "---\nname: pdf-tools\ndescription: Extract and convert PDF files.\n---\n\n# PDF Tools\n\nInstructions.\n"
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte(skillContent), 0o600); err != nil {
		t.Fatal(err)
	}

	// 1. Preview
	code, stdout, stderr := runCLI(t, "skill", "add", sourceDir, "--workspace", root)
	if code != 0 {
		t.Fatalf("skill add preview failed (exit %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "Add pdf-tools as a draft") {
		t.Errorf("preview missing summary: %s", stdout)
	}
	if !strings.Contains(stdout, "skillhub skill confirm") {
		t.Errorf("preview missing confirm guidance: %s", stdout)
	}
	if !strings.Contains(stdout, "No collection files changed.") {
		t.Errorf("preview should note no files changed: %s", stdout)
	}

	// Verify no canonical files changed yet
	targetMeta := filepath.Join(root, "skills", "default", "pdf-tools", "skill.meta.yaml")
	if _, err := os.Stat(targetMeta); !os.IsNotExist(err) {
		t.Fatalf("preview wrote canonical files before confirmation")
	}

	// 2. Add with --yes
	code, stdout, stderr = runCLI(t, "skill", "add", sourceDir, "--workspace", root, "--yes")
	if code != 0 {
		t.Fatalf("skill add --yes failed (exit %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "Draft pdf-tools added.") {
		t.Errorf("applied output missing confirmation line: %s", stdout)
	}
	if !strings.Contains(stdout, "skillhub skill review pdf-tools") {
		t.Errorf("applied output missing review next step: %s", stdout)
	}

	// Verify canonical files were created
	if _, err := os.Stat(targetMeta); err != nil {
		t.Fatalf("canonical files not found after confirm: %v", err)
	}
}

func TestSkillAddMultiSelectionAndAll(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	temp := t.TempDir()
	sourceDir := filepath.Join(temp, "bundle")
	dirA := filepath.Join(sourceDir, "tool-a")
	dirB := filepath.Join(sourceDir, "tool-b")
	_ = os.MkdirAll(dirA, 0o700)
	_ = os.MkdirAll(dirB, 0o700)
	_ = os.WriteFile(filepath.Join(dirA, "SKILL.md"), []byte("---\nname: tool-a\ndescription: Tool A\n---\n\n# Tool A\n"), 0o600)
	_ = os.WriteFile(filepath.Join(dirB, "SKILL.md"), []byte("---\nname: tool-b\ndescription: Tool B\n---\n\n# Tool B\n"), 0o600)

	// 1. Adding without selection must fail with selection required
	code, _, stderr := runCLI(t, "skill", "add", sourceDir, "--workspace", root)
	if code != 2 {
		t.Fatalf("expected exit 2 on multi-skill without selection, got %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "explicit selection is required") {
		t.Errorf("expected explicit selection error, got: %s", stderr)
	}

	// 2. Add all preview with --all
	code, stdout, stderr := runCLI(t, "skill", "add", sourceDir, "--all", "--workspace", root)
	if code != 0 {
		t.Fatalf("skill add --all preview failed (exit %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "Add tool-a, tool-b as a draft") && !strings.Contains(stdout, "tool-a") {
		t.Errorf("preview missing multi-skill line: %s", stdout)
	}

	// 3. Add specific with --skill and --yes
	code, stdout, stderr = runCLI(t, "skill", "add", sourceDir, "--skill", "tool-b", "--workspace", root, "--yes")
	if code != 0 {
		t.Fatalf("skill add --skill failed (exit %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "Draft tool-b added.") {
		t.Errorf("applied output missing tool-b: %s", stdout)
	}
}

func TestSkillAddShortConfirmDispatched(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	temp := t.TempDir()
	sourceDir := filepath.Join(temp, "short-confirm-skill")
	_ = os.MkdirAll(sourceDir, 0o700)
	_ = os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: short-confirm\ndescription: Short confirm test.\n---\n\n# Short Confirm\n"), 0o600)

	// Preview with JSON to get proposal ID
	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "add", sourceDir, "--workspace", root, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("preview exit %d: %s", code, stderr.String())
	}
	var preview app.SkillAddProposal
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatalf("unmarshal preview: %v", err)
	}
	proposalID := preview.Confirmation.Confirmation.Pins.ProposalID
	if proposalID == "" {
		t.Fatal("missing proposal ID in preview JSON")
	}

	// Confirm with short form: skillhub skill confirm <proposal_id>
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"skill", "confirm", proposalID, "--workspace", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("short confirm exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Draft short-confirm added.") {
		t.Errorf("confirm output missing draft added summary: %s", stdout.String())
	}
}

func TestSkillAddPreviewAndResultRenderUpstream(t *testing.T) {
	var buf bytes.Buffer
	p := termui.NewWithWidth(&buf, termui.MaxWidth)

	// 1. Preview with new upstream source
	preview := app.SkillAddProposal{
		SkillID: "pdf",
		Origin: app.SkillOrigin{
			Kind:       "github",
			Repository: "https://github.com/anthropics/skills",
			Path:       "skills/pdf",
			Commit:     "3f9c2a1b4d5e",
		},
		Resources:  []app.DiscoveredCompanion{{Path: "f1"}, {Path: "f2"}},
		TotalBytes: 49152,
		Confirmation: app.ConfirmationPolicy{
			Confirmation: app.ConfirmationRequirement{
				Pins: app.ConfirmationPins{ProposalID: "PROP-12345"},
			},
		},
		UpstreamSource: &app.UpstreamSourceRef{
			SourceID: "anthropics-skills",
			Created:  true,
		},
	}
	writeSkillAddPreview(p, preview)
	out := buf.String()
	wantPreviewLine := "Upstream: tracked by new source anthropics-skills (checked weekly by `skillhub check`; nothing runs in the background)."
	if !strings.Contains(out, wantPreviewLine) {
		t.Fatalf("preview output missing upstream line %q, got:\n%s", wantPreviewLine, out)
	}
	if strings.Contains(out, "Watching: off") {
		t.Fatalf("preview output should not contain 'Watching: off', got:\n%s", out)
	}

	// 2. Preview with existing upstream source
	buf.Reset()
	preview.UpstreamSource.Created = false
	writeSkillAddPreview(p, preview)
	outExisting := buf.String()
	wantExistingLine := "Upstream: tracked by source anthropics-skills."
	if !strings.Contains(outExisting, wantExistingLine) {
		t.Fatalf("preview output missing existing upstream line %q, got:\n%s", wantExistingLine, outExisting)
	}

	// 3. Result with upstream source
	buf.Reset()
	result := app.SkillAddResult{
		SkillID: "pdf",
		UpstreamSource: &app.UpstreamSourceRef{
			SourceID: "anthropics-skills",
		},
	}
	writeSkillAddResult(p, false, result)
	outResult := buf.String()
	wantResultLine := "Draft pdf added. Agent use: off. Upstream: anthropics-skills. Changes are not committed."
	if !strings.Contains(outResult, wantResultLine) {
		t.Fatalf("result output missing upstream line %q, got:\n%s", wantResultLine, outResult)
	}
	if strings.Contains(outResult, "Watching: off") {
		t.Fatalf("result output should not contain 'Watching: off', got:\n%s", outResult)
	}

	// 4. Result without upstream source (local add)
	buf.Reset()
	resultLocal := app.SkillAddResult{
		SkillID: "pdf",
	}
	writeSkillAddResult(p, false, resultLocal)
	outLocal := buf.String()
	wantLocalLine := "Draft pdf added. Agent use: off. Changes are not committed."
	if !strings.Contains(outLocal, wantLocalLine) {
		t.Fatalf("local result output missing expected line %q, got:\n%s", wantLocalLine, outLocal)
	}
	if strings.Contains(outLocal, "Watching: off") {
		t.Fatalf("local result output should not contain 'Watching: off', got:\n%s", outLocal)
	}
}
