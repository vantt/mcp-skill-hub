package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
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
	if !strings.Contains(stdout, "Apply: skillhub skill add ") || !strings.Contains(stdout, " --yes\n") {
		t.Errorf("preview must end with the command that applies it: %s", stdout)
	}
	if !strings.Contains(stdout, "Show the diff: skillhub skill add ") || !strings.Contains(stdout, " --diff\n") {
		t.Errorf("preview must say how to see the diff: %s", stdout)
	}
	if !strings.Contains(stdout, "Nothing written yet.") {
		t.Errorf("preview should note that nothing was written: %s", stdout)
	}

	// Verify no canonical files changed yet
	targetMeta := filepath.Join(root, "skills", "default", "pdf-tools", ".meta", "skill.yaml")
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
	writeSkillAddPreview(p, preview, "skillhub skill add x", false, false)
	out := buf.String()
	wantPreviewLine := "Upstream: tracked by new source anthropics-skills (checked weekly by `skillhub source check`; nothing runs in the background)."
	if !strings.Contains(flatText(out), wantPreviewLine) {
		t.Fatalf("preview output missing upstream line %q, got:\n%s", wantPreviewLine, out)
	}
	if strings.Contains(out, "Watching: off") {
		t.Fatalf("preview output should not contain 'Watching: off', got:\n%s", out)
	}

	// 2. Preview with existing upstream source
	buf.Reset()
	preview.UpstreamSource.Created = false
	writeSkillAddPreview(p, preview, "skillhub skill add x", false, false)
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

func writeCLIAddFixture(t *testing.T, dir, rel, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestSkillAddPreviewListsEveryFileAndWhatWasLeftOut(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	dir := filepath.Join(t.TempDir(), "team-release")
	writeCLIAddFixture(t, dir, "SKILL.md", "---\nname: team-release\ndescription: Release the team.\n---\n\n# Release\n", 0o644)
	writeCLIAddFixture(t, dir, "scripts/bump.sh", "#!/bin/sh\n", 0o755)
	writeCLIAddFixture(t, dir, ".env", "TOKEN=abc\n", 0o644)

	code, stdout, stderr := runCLI(t, "skill", "add", dir, "--workspace", root)
	if code != 0 {
		t.Fatalf("preview exit %d: %s", code, stderr)
	}
	out := flatText(stdout)
	for _, want := range []string{"Files (2,", "SKILL.md", "scripts/bump.sh", "(executable)", "Left out", ".env"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(out, "TOKEN=abc") {
		t.Errorf("preview leaked file contents:\n%s", stdout)
	}
}

func TestSkillAddAgainExplainsNothingChanged(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	dir := filepath.Join(t.TempDir(), "team-release")
	writeCLIAddFixture(t, dir, "SKILL.md", "---\nname: team-release\ndescription: Release the team.\n---\n\n# Release\n", 0o644)

	if code, _, stderr := runCLI(t, "skill", "add", dir, "--workspace", root, "--yes"); code != 0 {
		t.Fatalf("first add exit %d: %s", code, stderr)
	}
	if code, _, stderr := runCLI(t, "skill", "edit", "team-release", "--trigger", "release day", "--not-for", "hotfixes", "--min-scope", "single_step", "--workspace", root, "--yes"); code != 0 {
		t.Fatalf("edit exit %d: %s", code, stderr)
	}
	if code, _, stderr := runCLI(t, "skill", "activate", "team-release", "--workspace", root, "--yes"); code != 0 {
		t.Fatalf("activate exit %d: %s", code, stderr)
	}
	for _, args := range [][]string{
		{"skill", "add", dir, "--workspace", root},
		{"skill", "add", dir, "--workspace", root, "--yes"},
	} {
		code, stdout, stderr := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v exit %d: %s", args, code, stderr)
		}
		out := flatText(stdout)
		if !strings.Contains(out, "already added") || !strings.Contains(out, "active") {
			t.Errorf("%v: want an already-added message with the state:\n%s", args, stdout)
		}
		if strings.Contains(out, "--proposal ''") || strings.Contains(out, "skill confirm") {
			t.Errorf("%v: must not offer a confirm command:\n%s", args, stdout)
		}
		if strings.Contains(out, "Draft team-release added") {
			t.Errorf("%v: must not claim a new draft:\n%s", args, stdout)
		}
		if !strings.Contains(out, "skillhub skill review team-release") {
			t.Errorf("%v: missing review next step:\n%s", args, stdout)
		}
	}
}

func TestSkillAddErrorPreviewKeepsItsOwnCauseUnderYes(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	missing := filepath.Join(t.TempDir(), "nope")
	for _, args := range [][]string{
		{"skill", "add", missing, "--workspace", root},
		{"skill", "add", missing, "--workspace", root, "--yes"},
	} {
		code, _, stderr := runCLI(t, args...)
		if code != 2 {
			t.Errorf("%v exit = %d, want 2", args, code)
		}
		if !strings.Contains(flatText(stderr), "cannot access local directory") || strings.Contains(strings.ToLower(stderr), "stale") {
			t.Errorf("%v stderr should carry the real cause:\n%s", args, stderr)
		}
	}
}

func TestSkillAddPreviewShowsLicenseWarningOnlyWhenItMatters(t *testing.T) {
	t.Parallel()
	root := initTestWorkspace(t)
	temp := t.TempDir()
	proprietary := filepath.Join(temp, "prop-skill")
	writeCLIAddFixture(t, proprietary, "SKILL.md", "---\nname: prop-skill\ndescription: Proprietary tool\nlicense: Proprietary terms apply\n---\n\n# Prop\n", 0o644)
	own := filepath.Join(temp, "own-skill")
	writeCLIAddFixture(t, own, "SKILL.md", "---\nname: own-skill\ndescription: My own tool\n---\n\n# Own\n", 0o644)

	code, stdout, stderr := runCLI(t, "skill", "add", proprietary, "--workspace", root)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(flatText(stdout)), "proprietary") {
		t.Errorf("preview must warn about the proprietary license:\n%s", stdout)
	}
	code, stdout, stderr = runCLI(t, "skill", "add", own, "--workspace", root)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	preview, _, _ := strings.Cut(stdout, "Nothing written yet.")
	if strings.Contains(strings.ToLower(preview), "license") {
		t.Errorf("a folder of your own needs no license line:\n%s", stdout)
	}
}
