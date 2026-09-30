package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

type fakeImportAdapter struct {
	fakeSourceAdapter
	resources []sourcepkg.Resource
	files     map[string][]byte
	listErr   error
}

func (a *fakeImportAdapter) List(_ context.Context, _ sourcepkg.Source, _ sourcepkg.Revision, scope sourcepkg.Scope) ([]sourcepkg.Resource, error) {
	if a.listErr != nil {
		return nil, a.listErr
	}
	var res []sourcepkg.Resource
	for _, r := range a.resources {
		if scope.Prefix != "" && !strings.HasPrefix(r.Path, scope.Prefix) {
			continue
		}
		res = append(res, r)
	}
	return res, nil
}

func (a *fakeImportAdapter) Read(_ context.Context, _ sourcepkg.Source, _ sourcepkg.Revision, path string) ([]byte, error) {
	if data, ok := a.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func TestSourceImportPreviewAndConfirmWithConflictSkipping(t *testing.T) {
	root := newSourceWorkspace(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	// Create an existing draft skill
	skillService := SkillService{}
	existingPreview, err := skillService.PreviewCreate(context.Background(), root, skill.CreateInput{
		ID:          "existing-skill",
		Collection:  "default",
		Name:        "Existing Skill",
		Description: "Already in workspace.",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skillService.ConfirmSkillMutation(context.Background(), root, existingPreview, existingPreview.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}

	// Create a monitored source
	sourceService := SourceService{Clock: sourceClock{now: now}}
	sourceRec := sourcepkg.Record{
		SchemaVersion:   1,
		ID:              "gh-source",
		Adapter:         "git",
		Locator:         sourcepkg.Locator{Repository: "https://github.com/example/skills.git"},
		Status:          "watching",
		Identity:        sourcepkg.Identity{Name: "skills", Canonical: "https://github.com/example/skills.git"},
		Limits:          sourcepkg.Limits{TimeoutSeconds: 20, MaxBytes: sourcepkg.DefaultMaxBytes, MaxFiles: sourcepkg.DefaultMaxFiles, MaxFileBytes: sourcepkg.DefaultMaxFileSize},
		Monitoring:      sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		CurrentRevision: ptrRevision(revision("commit-123")),
	}
	srcBytes, err := sourcepkg.MarshalCanonical(sourceRec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources", "catalog", "gh-source.yaml"), srcBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}

	adapter := &fakeImportAdapter{
		fakeSourceAdapter: fakeSourceAdapter{
			revisions: map[string]sourcepkg.Revision{"gh-source": *sourceRec.CurrentRevision},
		},
		resources: []sourcepkg.Resource{
			{Path: "skills/existing-skill/SKILL.md", Size: 100},
			{Path: "skills/new-skill-a/SKILL.md", Size: 120},
			{Path: "skills/new-skill-b/SKILL.md", Size: 150},
			{Path: "skills/new-skill-b/references/guide.md", Size: 80},
		},
		files: map[string][]byte{
			"skills/existing-skill/SKILL.md":         []byte("---\nname: existing-skill\ndescription: From source\n---\n# Existing Skill\n"),
			"skills/new-skill-a/SKILL.md":            []byte("---\nname: new-skill-a\ndescription: Skill A description\n---\n# New Skill A\n"),
			"skills/new-skill-b/SKILL.md":            []byte("---\nname: new-skill-b\ndescription: Skill B description\n---\n# New Skill B\n"),
			"skills/new-skill-b/references/guide.md": []byte("# Companion Guide\nSome useful notes.\n"),
		},
	}

	importService := SourceImportService{
		Clock:    sourceClock{now: now},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	// 1. Preview
	proposal, err := importService.PreviewSourceImport(context.Background(), root, SourceImportPreviewInput{
		SourceID: "gh-source",
	})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if len(proposal.Discovered) != 3 {
		t.Fatalf("discovered count = %d, want 3", len(proposal.Discovered))
	}
	if len(proposal.Importable) != 2 {
		t.Fatalf("importable count = %d, want 2", len(proposal.Importable))
	}
	if len(proposal.Skipped) != 1 {
		t.Fatalf("skipped count = %d, want 1", len(proposal.Skipped))
	}
	if proposal.Skipped[0].TargetID != "existing-skill" || !proposal.Skipped[0].Conflict {
		t.Fatalf("expected existing-skill to be skipped conflict, got: %#v", proposal.Skipped[0])
	}
	if proposal.Confirmation.Confirmation.Pins.ProposalID == "" {
		t.Fatal("missing proposal ID in confirmation pins")
	}

	// 2. Confirm
	result, err := importService.ConfirmSourceImport(context.Background(), root, proposal, proposal.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatalf("confirm failed: %v", err)
	}

	if result.Status != StatusApplied {
		t.Fatalf("status = %s, want %s", result.Status, StatusApplied)
	}
	if result.ImportedCount != 2 {
		t.Fatalf("imported count = %d, want 2", result.ImportedCount)
	}
	if result.SkippedCount != 1 {
		t.Fatalf("skipped count = %d, want 1", result.SkippedCount)
	}
	if !strings.Contains(result.Summary, "Imported 2 draft skill(s)") {
		t.Fatalf("unexpected summary: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, "skillhub skill activate") {
		t.Fatalf("summary does not mention skill activate: %s", result.Summary)
	}

	// Verify imported skills are drafts with provenance
	for _, id := range []string{"new-skill-a", "new-skill-b"} {
		readSkill, err := skillService.ReadSkill(context.Background(), root, id)
		if err != nil {
			t.Fatalf("read %s failed: %v", id, err)
		}
		if readSkill.Manifest.Status != "draft" {
			t.Fatalf("imported skill %s status = %q, want draft", id, readSkill.Manifest.Status)
		}

		// Verify provenance link exists
		linkFile := filepath.Join(root, "sources", "skills", "LINK-"+id+"--gh-source.yaml")
		if _, err := os.Stat(linkFile); err != nil {
			t.Fatalf("missing provenance link %s: %v", linkFile, err)
		}

		// Verify skill.meta.yaml provenance
		metaFile := filepath.Join(root, "skills", "default", id, "skill.meta.yaml")
		metaData, err := os.ReadFile(metaFile)
		if err != nil {
			t.Fatalf("read meta failed: %v", err)
		}
		metaStr := string(metaData)
		if !strings.Contains(metaStr, "source_id: gh-source") || !strings.Contains(metaStr, "revision: "+sourceRec.CurrentRevision.Value) || !strings.Contains(metaStr, "created_by: source_import") {
			t.Fatalf("metadata missing provenance fields: %s", metaStr)
		}
	}

	// Verify companion file was imported
	compPath := filepath.Join(root, "skills", "default", "new-skill-b", "references", "guide.md")
	if _, err := os.Stat(compPath); err != nil {
		t.Fatalf("companion file was not imported: %v", err)
	}

	// Verify existing skill was not modified
	readExisting, err := skillService.ReadSkill(context.Background(), root, "existing-skill")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readExisting.Content, "Already in workspace") {
		t.Fatalf("existing skill was overwritten: %s", readExisting.Content)
	}

	// 3. Re-run import is idempotent
	reProposal, err := importService.PreviewSourceImport(context.Background(), root, SourceImportPreviewInput{
		SourceID: "gh-source",
	})
	if err != nil {
		t.Fatalf("re-preview failed: %v", err)
	}
	if len(reProposal.Importable) != 0 {
		t.Fatalf("re-run importable count = %d, want 0", len(reProposal.Importable))
	}
	if len(reProposal.Skipped) != 3 {
		t.Fatalf("re-run skipped count = %d, want 3", len(reProposal.Skipped))
	}

	_ = sourceService
}

func TestSourceImportOversizeLimitError(t *testing.T) {
	root := newSourceWorkspace(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	sourceRec := sourcepkg.Record{
		SchemaVersion:   1,
		ID:              "huge-source",
		Adapter:         "git",
		Locator:         sourcepkg.Locator{Repository: "https://github.com/example/huge.git"},
		Status:          "watching",
		Monitoring:      sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		Identity:        sourcepkg.Identity{Name: "huge", Canonical: "https://github.com/example/huge.git"},
		Limits:          sourcepkg.Limits{TimeoutSeconds: 20, MaxBytes: sourcepkg.DefaultMaxBytes, MaxFiles: sourcepkg.DefaultMaxFiles, MaxFileBytes: sourcepkg.DefaultMaxFileSize},
		CurrentRevision: ptrRevision(revision("commit-huge")),
	}
	srcBytes, _ := sourcepkg.MarshalCanonical(sourceRec)
	_ = os.WriteFile(filepath.Join(root, "sources", "catalog", "huge-source.yaml"), srcBytes, 0o644)
	_, _ = catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{})

	limitErr := &sourcepkg.LimitExceededError{Limit: "files", Actual: 3500, Max: 2048}
	adapter := &fakeImportAdapter{
		fakeSourceAdapter: fakeSourceAdapter{
			revisions: map[string]sourcepkg.Revision{"huge-source": *sourceRec.CurrentRevision},
		},
		listErr: limitErr,
	}

	importService := SourceImportService{
		Clock:    sourceClock{now: now},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	_, err := importService.PreviewSourceImport(context.Background(), root, SourceImportPreviewInput{
		SourceID: "huge-source",
	})
	if err == nil {
		t.Fatal("expected limit error, got nil")
	}
	if !errors.Is(err, sourcepkg.ErrLimitExceeded) {
		t.Fatalf("expected ErrLimitExceeded, got %v", err)
	}
	var extracted *sourcepkg.LimitExceededError
	if !errors.As(err, &extracted) {
		t.Fatalf("expected LimitExceededError, got %T", err)
	}
	if extracted.Limit != "files" || extracted.Actual != 3500 || extracted.Max != 2048 {
		t.Fatalf("unexpected limit details: %#v", extracted)
	}
}

func TestSourceCandidateCaptureIdempotent(t *testing.T) {
	root := newSourceWorkspace(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	service := SourceService{
		Clock: sourceClock{now: now},
		IDs:   fixedSourceID("1122334455667788"),
	}

	// First capture
	res1, err := service.CaptureSourceCandidate(context.Background(), root, SourceCandidateInput{
		Locator: "https://github.com/example/repo.git",
		Reason:  "first capture",
	})
	if err != nil {
		t.Fatalf("first capture failed: %v", err)
	}
	if res1.Candidate.ID == "" {
		t.Fatal("first candidate ID is empty")
	}

	// Second capture of same locator
	res2, err := service.CaptureSourceCandidate(context.Background(), root, SourceCandidateInput{
		Locator: "https://github.com/example/repo.git",
		Reason:  "second capture attempt",
	})
	if err != nil {
		t.Fatalf("second capture failed: %v", err)
	}

	if res2.Candidate.ID != res1.Candidate.ID {
		t.Fatalf("idempotent capture returned different ID: %s vs %s", res2.Candidate.ID, res1.Candidate.ID)
	}
	if res2.Candidate.Reason != "first capture" {
		t.Fatalf("expected original candidate reason, got: %s", res2.Candidate.Reason)
	}

	// Verify only 1 file in sources/intake/
	entries, err := os.ReadDir(filepath.Join(root, "sources", "intake"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 intake file, found %d", len(entries))
	}
}
