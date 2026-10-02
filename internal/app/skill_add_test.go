package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/vantt/mcp-skill-hub/internal/canonical"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func TestSkillAddLocalDirectHappyPathBUG11(t *testing.T) {
	root := newSourceWorkspace(t)
	temp := t.TempDir()
	sourceDir := filepath.Join(temp, "my-external-skill")
	if err := os.MkdirAll(filepath.Join(sourceDir, "references"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceDir, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceDir, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}

	skillMDContent := []byte(`---
name: External-Skill
description: An external skill from local drive
license: MIT
custom_meta: preserved-value
---

# External Skill Guide

Step 1: Do something useful.
`)
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), skillMDContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "LICENSE.txt"), []byte("MIT License terms\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "references", "guide.md"), []byte("# Reference Guide\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "scripts", "run.py"), []byte("print('running')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "empty.txt"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	binaryData := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x01}
	if err := os.WriteFile(filepath.Join(sourceDir, "assets", "icon.png"), binaryData, 0o600); err != nil {
		t.Fatal(err)
	}

	service := SkillAddService{
		Clock: sourceClock{now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)},
	}

	// 1. Preview
	preview, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{
		Locator: sourceDir,
	})
	if err != nil {
		t.Fatalf("PreviewSkillAdd failed: %v", err)
	}
	if preview.Error != nil {
		t.Fatalf("preview returned error: %s: %s", preview.Error.Code, preview.Error.Render.Why)
	}

	if preview.SkillID != "external-skill" {
		t.Errorf("expected SkillID 'external-skill', got %q", preview.SkillID)
	}
	if preview.Collection != "default" {
		t.Errorf("expected Collection 'default', got %q", preview.Collection)
	}
	if preview.Origin.Kind != "local" {
		t.Errorf("expected origin kind 'local', got %q", preview.Origin.Kind)
	}
	if preview.Origin.FolderDigest == "" {
		t.Error("expected non-empty origin FolderDigest")
	}
	if preview.License.Declared != "MIT" || preview.License.LicenseFile != "LICENSE.txt" {
		t.Errorf("unexpected license: %#v", preview.License)
	}
	if len(preview.Resources) != 5 {
		t.Errorf("expected 5 companion resources, got %d", len(preview.Resources))
	}

	pins := preview.Confirmation.Confirmation.Pins
	if pins.ProposalID == "" || pins.ProposalDigest == "" || pins.BaseVersion == "" {
		t.Fatalf("incomplete confirmation pins: %#v", pins)
	}

	// 2. Confirm
	result, err := service.ConfirmSkillAdd(context.Background(), root, preview, pins)
	if err != nil {
		t.Fatalf("ConfirmSkillAdd failed: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("confirm returned error: %s: %s", result.Error.Code, result.Error.Render.Why)
	}
	if result.SkillID != "external-skill" {
		t.Errorf("result SkillID = %q, want 'external-skill'", result.SkillID)
	}
	if result.Replayed {
		t.Error("expected replayed=false on initial confirm")
	}
	if result.Assessment.Canonical.Status != "draft" {
		t.Errorf("expected canonical status draft, got %q", result.Assessment.Canonical.Status)
	}

	// 3. Verify canonical files on disk
	skillPath := filepath.Join(root, "skills", "default", "external-skill")
	for _, rel := range []string{
		"SKILL.md",
		"skill.meta.yaml",
		"LICENSE.txt",
		"references/guide.md",
		"scripts/run.py",
		"empty.txt",
		"assets/icon.png",
	} {
		p := filepath.Join(skillPath, filepath.FromSlash(rel))
		if _, statErr := os.Stat(p); statErr != nil {
			t.Errorf("expected skill file %s does not exist: %v", rel, statErr)
		}
	}

	// Verify empty file preserved
	emptyBytes, _ := os.ReadFile(filepath.Join(skillPath, "empty.txt"))
	if len(emptyBytes) != 0 {
		t.Errorf("expected empty file, got %d bytes", len(emptyBytes))
	}

	// Verify binary file preserved
	binBytes, _ := os.ReadFile(filepath.Join(skillPath, "assets", "icon.png"))
	if !bytes.Equal(binBytes, binaryData) {
		t.Errorf("binary content mismatch")
	}

	// 4. Verify privacy: NO persistence of host path
	sourceDirBytes := []byte(sourceDir)
	_ = filepath.Walk(root, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		// Skip binary files
		if strings.HasSuffix(p, ".png") || strings.HasSuffix(p, ".db") {
			return nil
		}
		data, readErr := os.ReadFile(p)
		if readErr == nil && bytes.Contains(data, sourceDirBytes) {
			t.Errorf("workspace file %s leaks host sourceDir %s", p, sourceDir)
		}
		return nil
	})

	// 5. Verify NO source candidate, source record, or source link files created
	for _, dir := range []string{
		filepath.Join(root, "sources", "intake"),
		filepath.Join(root, "sources", "catalog"),
		filepath.Join(root, "sources", "skills"),
	} {
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Errorf("expected 0 files in %s, got %d", dir, len(entries))
		}
	}

	// 6. Canonical validation passes with 0 issues
	issues, valErr := canonical.Validate(root)
	if valErr != nil {
		t.Fatalf("canonical.Validate error: %v", valErr)
	}
	if len(issues) != 0 {
		var msgs []string
		for _, iss := range issues {
			msgs = append(msgs, iss.Path+": "+iss.Message)
		}
		t.Fatalf("expected 0 canonical issues, got:\n%s", strings.Join(msgs, "\n"))
	}
}

func TestSkillAddImmutableSnapshotAndOriginalDeletion(t *testing.T) {
	root := newSourceWorkspace(t)
	temp := t.TempDir()
	sourceDir := filepath.Join(temp, "ephemeral-skill")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: ephemeral\ndescription: test\n---\n# Ephemeral\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "companion.txt"), []byte("captured before delete\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	service := SkillAddService{
		Clock: sourceClock{now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)},
	}

	preview, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{
		Locator: sourceDir,
	})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview failed: %v, %#v", err, preview.Error)
	}

	// Completely delete the original source directory from host!
	if err := os.RemoveAll(sourceDir); err != nil {
		t.Fatal(err)
	}

	// Confirm from the captured immutable preview
	pins := preview.Confirmation.Confirmation.Pins
	result, err := service.ConfirmSkillAdd(context.Background(), root, preview, pins)
	if err != nil {
		t.Fatalf("ConfirmSkillAdd failed after source deletion: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("confirm returned error: %#v", result.Error)
	}

	compData, err := os.ReadFile(filepath.Join(root, "skills", "default", "ephemeral", "companion.txt"))
	if err != nil || string(compData) != "captured before delete\n" {
		t.Fatalf("expected companion data to be preserved, got %q, err %v", string(compData), err)
	}
}

func TestSkillAddDeterministicIdempotencyReplayAndCacheLoss(t *testing.T) {
	root := newSourceWorkspace(t)
	temp := t.TempDir()
	sourceDir := filepath.Join(temp, "replay-skill")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: replay-skill\ndescription: replay test\n---\n# Content\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	service := SkillAddService{Clock: sourceClock{now: now}}

	input := SkillAddInput{
		Locator:        sourceDir,
		IdempotencyKey: "test-idempotent-key-1",
	}

	// First run: preview and confirm
	preview1, err := service.PreviewSkillAdd(context.Background(), root, input)
	if err != nil || preview1.Error != nil {
		t.Fatalf("preview1 failed: %v, %#v", err, preview1.Error)
	}
	res1, err := service.ConfirmSkillAdd(context.Background(), root, preview1, preview1.Confirmation.Confirmation.Pins)
	if err != nil || res1.Error != nil {
		t.Fatalf("confirm1 failed: %v, %#v", err, res1.Error)
	}
	if res1.Replayed {
		t.Error("expected first confirm replayed=false")
	}

	// Simulate disposable cache and proposal loss: delete runtime/proposals and runtime/cache
	_ = os.RemoveAll(filepath.Join(root, "runtime", "proposals"))
	_ = os.RemoveAll(filepath.Join(root, "runtime", "cache"))

	// Second run with identical input: exact replay lookup precedes target ID conflict!
	preview2, err := service.PreviewSkillAdd(context.Background(), root, input)
	if err != nil || preview2.Error != nil {
		t.Fatalf("preview2 replay failed: %v, %#v", err, preview2.Error)
	}
	if !preview2.Replayed {
		t.Error("expected preview2 to be recognized as replayed")
	}

	res2, err := service.ConfirmSkillAdd(context.Background(), root, preview2, preview2.Confirmation.Confirmation.Pins)
	if err != nil || res2.Error != nil {
		t.Fatalf("confirm2 failed: %v, %#v", err, res2.Error)
	}
	if !res2.Replayed {
		t.Error("expected confirm2 replayed=true")
	}
	if res2.OperationID != res1.OperationID {
		t.Errorf("operation ID changed on replay: %s vs %s", res2.OperationID, res1.OperationID)
	}

	// Third run: change payload with the same idempotency key -> returns conflict error!
	conflictInput := SkillAddInput{
		Locator:        sourceDir,
		TargetID:       "different-id",
		IdempotencyKey: "test-idempotent-key-1", // reused key with different target ID
	}
	preview3, err := service.PreviewSkillAdd(context.Background(), root, conflictInput)
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if preview3.Error == nil || preview3.Error.Code != ErrorSkillConflict {
		t.Fatalf("expected ErrorSkillConflict for reused idempotency key with different payload, got: %#v", preview3.Error)
	}
}

func TestSkillAddTargetIDConflictDetectionAndRename(t *testing.T) {
	root := newSourceWorkspace(t)
	temp := t.TempDir()

	// Pre-create an existing skill in the workspace
	createActiveDistributionSkill(t, root, "conflict-skill", "Existing Conflict Skill")

	// Attempt to add a skill with the same name without an explicit rename
	sourceDir := filepath.Join(temp, "conflict-source")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: conflict-skill\ndescription: new attempt\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	service := SkillAddService{
		Clock: sourceClock{now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)},
	}

	preview, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{
		Locator: sourceDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview.Error == nil || preview.Error.Code != ErrorSkillConflict {
		t.Fatalf("expected ErrorSkillConflict, got: %#v", preview.Error)
	}
	if !strings.Contains(preview.Error.Render.Fix, "--id") {
		t.Errorf("expected fix guidance to mention --id, got %q", preview.Error.Render.Fix)
	}

	// Rename using TargetID
	previewRename, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{
		Locator:  sourceDir,
		TargetID: "renamed-unique-skill",
	})
	if err != nil || previewRename.Error != nil {
		t.Fatalf("preview with TargetID rename failed: %v, %#v", err, previewRename.Error)
	}
	if previewRename.SkillID != "renamed-unique-skill" {
		t.Errorf("expected SkillID 'renamed-unique-skill', got %q", previewRename.SkillID)
	}

	res, err := service.ConfirmSkillAdd(context.Background(), root, previewRename, previewRename.Confirmation.Confirmation.Pins)
	if err != nil || res.Error != nil {
		t.Fatalf("confirm rename failed: %v, %#v", err, res.Error)
	}
	if _, statErr := os.Stat(filepath.Join(root, "skills", "default", "renamed-unique-skill", "SKILL.md")); statErr != nil {
		t.Errorf("renamed skill file does not exist: %v", statErr)
	}
}

func TestSkillAddSelectionSemantics(t *testing.T) {
	root := newSourceWorkspace(t)
	temp := t.TempDir()

	service := SkillAddService{
		Clock: sourceClock{now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)},
	}

	// 1. Zero skills found
	emptyDir := filepath.Join(temp, "no-skills")
	_ = os.MkdirAll(emptyDir, 0o700)
	preview0, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{Locator: emptyDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if preview0.Error == nil || !strings.Contains(preview0.Error.Render.Why, "no skills found") {
		t.Fatalf("expected no_skills error, got: %#v", preview0.Error)
	}

	// 2. Multiple skills found: selection required
	multiDir := filepath.Join(temp, "multi-skills")
	_ = os.MkdirAll(filepath.Join(multiDir, "skill-a"), 0o700)
	_ = os.MkdirAll(filepath.Join(multiDir, "skill-b"), 0o700)
	_ = os.WriteFile(filepath.Join(multiDir, "skill-a", "SKILL.md"), []byte("---\nname: skill-a\ndescription: A\n---\n"), 0o600)
	_ = os.WriteFile(filepath.Join(multiDir, "skill-b", "SKILL.md"), []byte("---\nname: skill-b\ndescription: B\n---\n"), 0o600)

	previewMulti, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{Locator: multiDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if previewMulti.Error == nil || previewMulti.Error.Code != ErrorSkillSelectionRequired {
		t.Fatalf("expected ErrorSkillSelectionRequired, got: %#v", previewMulti.Error)
	}

	// 3. Selection with --skill
	previewSel, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{
		Locator:   multiDir,
		Selection: "skill-b",
	})
	if err != nil || previewSel.Error != nil {
		t.Fatalf("preview with selection failed: %v, %#v", err, previewSel.Error)
	}
	if previewSel.SkillID != "skill-b" {
		t.Errorf("expected SkillID 'skill-b', got %q", previewSel.SkillID)
	}

	// 4. Import all with --all
	previewAll, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{
		Locator: multiDir,
		All:     true,
	})
	if err != nil || previewAll.Error != nil {
		t.Fatalf("preview with All=true failed: %v, %#v", err, previewAll.Error)
	}
	if len(previewAll.SkillIDs) != 2 {
		t.Errorf("expected 2 skills in previewAll, got %d (%v)", len(previewAll.SkillIDs), previewAll.SkillIDs)
	}
}

func TestSkillAddLicenseWarningsBUG16(t *testing.T) {
	root := newSourceWorkspace(t)
	temp := t.TempDir()

	service := SkillAddService{
		Clock: sourceClock{now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)},
	}

	// Proprietary license in frontmatter
	propDir := filepath.Join(temp, "prop-skill")
	_ = os.MkdirAll(propDir, 0o700)
	_ = os.WriteFile(filepath.Join(propDir, "SKILL.md"), []byte("---\nname: prop\ndescription: prop tool\nlicense: Proprietary terms apply\n---\n"), 0o600)

	preview, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{Locator: propDir})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview failed: %v, %#v", err, preview.Error)
	}
	if !preview.License.IsProprietary {
		t.Error("expected IsProprietary=true")
	}
	hasPropWarning := false
	for _, w := range preview.Warnings {
		if strings.Contains(w.Summary, "proprietary") {
			hasPropWarning = true
			break
		}
	}
	if !hasPropWarning {
		t.Errorf("expected proprietary warning in preview.Warnings: %v", preview.Warnings)
	}
}

func TestSkillAddProposalDispatchIntegration(t *testing.T) {
	root := newSourceWorkspace(t)
	temp := t.TempDir()
	sourceDir := filepath.Join(temp, "dispatch-skill")
	_ = os.MkdirAll(sourceDir, 0o700)
	_ = os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: dispatch-skill\ndescription: dispatch test\n---\n"), 0o600)

	skillService := SkillService{}
	preview, err := skillService.PreviewSkillAdd(context.Background(), root, SkillAddInput{Locator: sourceDir})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview failed: %v, %#v", err, preview.Error)
	}

	pins := preview.Confirmation.Confirmation.Pins
	// Dispatch confirmation through Phase 3 ProposalDispatcher / DispatchConfirmProposal
	dispatchedResult, err := skillService.DispatchConfirmProposal(context.Background(), root, preview.ProposalID(), &pins)
	if err != nil {
		t.Fatalf("DispatchConfirmProposal failed: %v", err)
	}

	addResult, ok := dispatchedResult.(SkillAddResult)
	if !ok {
		t.Fatalf("expected SkillAddResult from dispatcher, got %T", dispatchedResult)
	}
	if addResult.SkillID != "dispatch-skill" {
		t.Errorf("dispatched result SkillID = %q, want 'dispatch-skill'", addResult.SkillID)
	}
}

func TestSkillAddConfirmationPinsMismatchRejected(t *testing.T) {
	root := newSourceWorkspace(t)
	temp := t.TempDir()
	sourceDir := filepath.Join(temp, "pins-skill")
	_ = os.MkdirAll(sourceDir, 0o700)
	_ = os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("---\nname: pins-skill\ndescription: pins test\n---\n"), 0o600)

	service := SkillAddService{
		Clock: sourceClock{now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)},
	}

	preview, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{Locator: sourceDir})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview failed: %v, %#v", err, preview.Error)
	}

	// Mismatched proposal digest
	badPins := preview.Confirmation.Confirmation.Pins
	badPins.ProposalDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

	result, err := service.ConfirmSkillAdd(context.Background(), root, preview, badPins)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == nil || result.Error.Code != ErrorStaleProposal {
		t.Fatalf("expected ErrorStaleProposal for bad pins, got %#v", result.Error)
	}
}

func TestSkillAddSmokeMutateOriginalAfterPreview(t *testing.T) {
	root := newSourceWorkspace(t)
	temp := t.TempDir()
	sourceDir := filepath.Join(temp, "smoke-skill")
	if err := os.MkdirAll(filepath.Join(sourceDir, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}

	originalSkillMD := []byte("---\nname: smoke-skill\ndescription: smoke test\n---\n# Original Body\n")
	originalLicense := []byte("Original License Terms\n")
	originalForms := []byte("# Original Forms\n")
	originalScript := []byte("print('original script')\n")

	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), originalSkillMD, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "LICENSE.txt"), originalLicense, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "forms.md"), originalForms, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "scripts", "run.py"), originalScript, 0o600); err != nil {
		t.Fatal(err)
	}

	service := SkillAddService{
		Clock: sourceClock{now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)},
	}

	// 1. Preview
	preview, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{
		Locator: sourceDir,
	})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview failed: %v, %#v", err, preview.Error)
	}

	if len(preview.Resources) != 3 {
		t.Fatalf("expected 3 companion resources in preview, got %d", len(preview.Resources))
	}

	// 2. Mutate original source files and add new extraneous file
	if err := os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte("# Mutated After Preview\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "LICENSE.txt"), []byte("MUTATED LICENSE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "forms.md"), []byte("# MUTATED FORMS\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "scripts", "run.py"), []byte("print('mutated')\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "extraneous.txt"), []byte("should not be imported\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 3. Confirm
	pins := preview.Confirmation.Confirmation.Pins
	result, err := service.ConfirmSkillAdd(context.Background(), root, preview, pins)
	if err != nil || result.Error != nil {
		t.Fatalf("confirm failed: %v, %#v", err, result.Error)
	}

	// 4. Verify canonical tree: must match PREVIEW bytes, NOT mutated bytes!
	canonicalRoot := filepath.Join(root, "skills", "default", "smoke-skill")

	// Extraneous file must NOT exist
	if _, statErr := os.Stat(filepath.Join(canonicalRoot, "extraneous.txt")); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("extraneous file created after preview was unexpectedly imported")
	}

	// Check files match original bytes
	gotLicense, _ := os.ReadFile(filepath.Join(canonicalRoot, "LICENSE.txt"))
	if !bytes.Equal(gotLicense, originalLicense) {
		t.Errorf("LICENSE.txt changed after preview mutation: got %q, want %q", string(gotLicense), string(originalLicense))
	}

	gotForms, _ := os.ReadFile(filepath.Join(canonicalRoot, "forms.md"))
	if !bytes.Equal(gotForms, originalForms) {
		t.Errorf("forms.md changed after preview mutation: got %q, want %q", string(gotForms), string(originalForms))
	}

	gotScript, _ := os.ReadFile(filepath.Join(canonicalRoot, "scripts", "run.py"))
	if !bytes.Equal(gotScript, originalScript) {
		t.Errorf("run.py changed after preview mutation: got %q, want %q", string(gotScript), string(originalScript))
	}

	gotSkillMD, _ := os.ReadFile(filepath.Join(canonicalRoot, "SKILL.md"))
	if !strings.Contains(string(gotSkillMD), "Original Body") {
		t.Errorf("SKILL.md changed after preview mutation: %s", string(gotSkillMD))
	}

	// Canonical validation passes
	issues, valErr := canonical.Validate(root)
	if valErr != nil || len(issues) != 0 {
		t.Fatalf("canonical validation issues: %v, %v", valErr, issues)
	}
}
func TestSkillAddRemoteGitRealAdapter(t *testing.T) {
	root := newSourceWorkspace(t)
	repoDir := t.TempDir()
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(repoDir, "my-skill")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\ndescription: Test git skill\n---\nBody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatal(err)
	}
	_, err = wt.Commit("initial", &git.CommitOptions{
		Author: &object.Signature{Name: "Tester", Email: "tester@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}

	adapter := sourcepkg.GitRepositoryAdapter{
		CacheRoot:         filepath.Join(root, "runtime", "sources", "git"),
		AllowFileProtocol: true,
	}
	service := SkillAddService{
		Clock:    sourceClock{now: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	fileURL := "file://" + filepath.ToSlash(repoDir)
	preview, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{
		Locator: fileURL,
	})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview failed: %v, %#v", err, preview.Error)
	}
	if preview.SkillID != "my-skill" {
		t.Fatalf("expected skill ID my-skill, got %s", preview.SkillID)
	}

	pins := preview.Confirmation.Confirmation.Pins
	result, err := service.ConfirmSkillAdd(context.Background(), root, preview, pins)
	if err != nil || result.Error != nil {
		t.Fatalf("confirm failed: %v, %#v", err, result.Error)
	}

	reviewService := SkillService{}
	reviewResult, err := reviewService.ReviewSkill(context.Background(), root, "my-skill")
	if err != nil || reviewResult.Error != nil {
		t.Fatalf("review failed: %v, %#v", err, reviewResult.Error)
	}
	if reviewResult.Provenance == nil || reviewResult.Provenance.SourceLocator == "" {
		t.Fatalf("expected provenance locator, got %#v", reviewResult.Provenance)
	}
}
