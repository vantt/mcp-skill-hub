package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
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

func (a *fakeImportAdapter) Diff(_ context.Context, _ sourcepkg.Source, from, to sourcepkg.Revision) (sourcepkg.ChangeSet, error) {
	var changes []sourcepkg.Change
	for p := range a.files {
		changes = append(changes, sourcepkg.Change{Path: p, Status: "modified"})
	}
	return sourcepkg.ChangeSet{From: from, To: to, Changes: changes}, nil
}

func TestSourceImportPreviewAndConfirmWithConflictSkipping(t *testing.T) {
	t.Parallel()
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
		CurrentRevision: new(revision("commit-123")),
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

		// Verify no provenance link exists on disk
		linkFile := filepath.Join(root, "sources", "skills", "LINK-"+id+"--gh-source.yaml")
		if _, err := os.Stat(linkFile); !os.IsNotExist(err) {
			t.Fatalf("expected no provenance link %s, but file exists", linkFile)
		}
		expectedPath := "skills/" + id
		// Verify skill metadata
		metaFile := filepath.Join(root, "skills", "default", id, ".meta", "skill.yaml")
		if _, err := os.Stat(metaFile); os.IsNotExist(err) {
			metaFile = filepath.Join(root, "skills", "default", id, "skill.meta.yaml")
		}
		metaData, err := os.ReadFile(metaFile)
		if err != nil {
			t.Fatalf("read meta failed: %v", err)
		}
		var meta struct {
			Sources []struct {
				ID     string   `yaml:"id"`
				Roles  []string `yaml:"roles"`
				Commit string   `yaml:"commit"`
			} `yaml:"sources"`
			Provenance struct {
				CreatedBy string `yaml:"created_by"`
				SourceID  string `yaml:"source_id"`
				Origin    struct {
					Commit string `yaml:"commit"`
					Path   string `yaml:"path"`
				} `yaml:"origin"`
			} `yaml:"provenance"`
		}
		if err := yaml.Unmarshal(metaData, &meta); err != nil {
			t.Fatalf("unmarshal meta failed: %v", err)
		}
		if len(meta.Sources) > 0 {
			if meta.Sources[0].ID != "gh-source" {
				t.Fatalf("expected source_id gh-source, got %q", meta.Sources[0].ID)
			}
			if meta.Sources[0].Commit != sourceRec.CurrentRevision.Value {
				t.Fatalf("expected commit %q, got %q", sourceRec.CurrentRevision.Value, meta.Sources[0].Commit)
			}
		} else {
			if meta.Provenance.SourceID != "gh-source" {
				t.Fatalf("expected source_id gh-source, got %q", meta.Provenance.SourceID)
			}
			if meta.Provenance.Origin.Commit != sourceRec.CurrentRevision.Value {
				t.Fatalf("expected commit %q, got %q", sourceRec.CurrentRevision.Value, meta.Provenance.Origin.Commit)
			}
			if meta.Provenance.Origin.Path != expectedPath {
				t.Fatalf("expected origin.path %q, got %q", expectedPath, meta.Provenance.Origin.Path)
			}
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
	t.Parallel()
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
		CurrentRevision: new(revision("commit-huge")),
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
	t.Parallel()
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
func TestSourceImportFolderScopedPreservesCompanionsBUG04(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	sourceRec := sourcepkg.Record{
		SchemaVersion: 1,
		ID:            "ap",
		Locator: sourcepkg.Locator{
			Repository: "https://github.com/anthropics/skills.git",
		},
		Adapter:         "git",
		Status:          "watching",
		Identity:        sourcepkg.Identity{Name: "skills", Canonical: "https://github.com/anthropics/skills.git"},
		Limits:          sourcepkg.Limits{TimeoutSeconds: 20, MaxBytes: sourcepkg.DefaultMaxBytes, MaxFiles: sourcepkg.DefaultMaxFiles, MaxFileBytes: sourcepkg.DefaultMaxFileSize},
		Monitoring:      sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		CurrentRevision: new(revision("commit-8a1541c")),
	}
	srcBytes, _ := sourcepkg.MarshalCanonical(sourceRec)
	_ = os.WriteFile(filepath.Join(root, "sources", "catalog", "ap.yaml"), srcBytes, 0o644)
	_, _ = catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{})

	binaryBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x01}
	fakeFiles := map[string][]byte{
		"skills/pdf/SKILL.md":           []byte("---\nname: pdf\ndescription: PDF Processing\nlicense: Proprietary. LICENSE.txt has complete terms\n---\n\n# PDF Tool\n"),
		"skills/pdf/LICENSE.txt":        []byte("Commercial License Terms\n"),
		"skills/pdf/forms.md":           []byte("# Forms Markdown\n"),
		"skills/pdf/reference.md":       []byte("# Reference Markdown\n"),
		"skills/pdf/scripts/extract.py": []byte("print('extracting')\n"),
		"skills/pdf/empty.txt":          []byte(""),
		"skills/pdf/assets/icon.png":    binaryBytes,
	}

	resources := []sourcepkg.Resource{
		{Path: "skills/pdf/SKILL.md", Size: int64(len(fakeFiles["skills/pdf/SKILL.md"]))},
		{Path: "skills/pdf/LICENSE.txt", Size: int64(len(fakeFiles["skills/pdf/LICENSE.txt"]))},
		{Path: "skills/pdf/forms.md", Size: int64(len(fakeFiles["skills/pdf/forms.md"]))},
		{Path: "skills/pdf/reference.md", Size: int64(len(fakeFiles["skills/pdf/reference.md"]))},
		{Path: "skills/pdf/scripts/extract.py", Size: int64(len(fakeFiles["skills/pdf/scripts/extract.py"]))},
		{Path: "skills/pdf/empty.txt", Size: 0},
		{Path: "skills/pdf/assets/icon.png", Size: int64(len(binaryBytes))},
	}

	adapter := &fakeImportAdapter{
		resources: resources,
		files:     fakeFiles,
	}

	importService := SourceImportService{
		Clock: sourceClock{now: now},
		Adapters: map[string]sourcepkg.Adapter{
			"git": adapter,
		},
	}

	// Folder-scoped import using Path: "skills/pdf"
	preview, err := importService.PreviewSourceImport(context.Background(), root, SourceImportPreviewInput{
		SourceID: "ap",
		Path:     "skills/pdf",
	})
	if err != nil {
		t.Fatalf("PreviewSourceImport failed: %v", err)
	}

	if len(preview.Importable) != 1 || preview.Importable[0].TargetID != "pdf" {
		t.Fatalf("expected 1 importable skill 'pdf', got %#v", preview.Importable)
	}

	// Verify BUG-16: warning generated for proprietary license
	hasLicenseWarning := false
	for _, w := range preview.Warnings {
		if strings.Contains(w.Summary, "proprietary") {
			hasLicenseWarning = true
			break
		}
	}
	if !hasLicenseWarning {
		t.Errorf("expected warning for proprietary license in preview, got %v", preview.Warnings)
	}

	// Verify all 7 files + metadata + link are in diff.Added (9 files total)
	expectedAdded := []string{
		"skills/default/pdf/SKILL.md",
		"skills/default/pdf/.meta/skill.yaml",
		"skills/default/pdf/LICENSE.txt",
		"skills/default/pdf/forms.md",
		"skills/default/pdf/reference.md",
		"skills/default/pdf/scripts/extract.py",
		"skills/default/pdf/empty.txt",
		"skills/default/pdf/assets/icon.png",
	}

	addedMap := make(map[string]bool)
	for _, a := range preview.Diff.Added {
		if strings.HasPrefix(a, "sources/skills/LINK-") {
			t.Errorf("unexpected link file in preview diff: %s", a)
		}
		addedMap[a] = true
	}
	for _, exp := range expectedAdded {
		if !addedMap[exp] {
			t.Errorf("expected added path %s missing from preview diff: %v", exp, preview.Diff.Added)
		}
	}

	// Confirm the import
	pins := preview.Confirmation.Confirmation.Pins
	result, err := importService.ConfirmSourceImport(context.Background(), root, preview, pins)
	if err != nil {
		t.Fatalf("ConfirmSourceImport failed: %v", err)
	}

	if result.ImportedCount != 1 || len(result.ImportedIDs) != 1 || result.ImportedIDs[0] != "pdf" {
		t.Fatalf("unexpected import result: %#v", result)
	}

	// Verify no link file on disk
	if _, err := os.Stat(filepath.Join(root, "sources", "skills", "LINK-pdf--ap.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected no link file on disk, but found it: %v", err)
	}

	// Verify files on disk
	for _, rel := range expectedAdded {
		filePath := filepath.Join(root, filepath.FromSlash(rel))
		if _, statErr := os.Stat(filePath); statErr != nil {
			t.Errorf("expected imported file %s does not exist: %v", rel, statErr)
		}
	}

	// Verify metadata
	pdfMetaPath := filepath.Join(root, "skills", "default", "pdf", ".meta", "skill.yaml")
	if _, err := os.Stat(pdfMetaPath); os.IsNotExist(err) {
		pdfMetaPath = filepath.Join(root, "skills", "default", "pdf", "skill.meta.yaml")
	}
	pdfMetaData, err := os.ReadFile(pdfMetaPath)
	if err != nil {
		t.Fatalf("read pdf meta failed: %v", err)
	}
	var pdfMeta struct {
		Sources []struct {
			ID    string   `yaml:"id"`
			Roles []string `yaml:"roles"`
			Path  string   `yaml:"path"`
		} `yaml:"sources"`
		Provenance struct {
			CreatedBy string `yaml:"created_by"`
			SourceID  string `yaml:"source_id"`
			Origin    struct {
				Commit string `yaml:"commit"`
				Path   string `yaml:"path"`
			} `yaml:"origin"`
		} `yaml:"provenance"`
	}
	if err := yaml.Unmarshal(pdfMetaData, &pdfMeta); err != nil {
		t.Fatalf("unmarshal pdf meta failed: %v", err)
	}
	if len(pdfMeta.Sources) > 0 {
		if pdfMeta.Sources[0].ID != "ap" {
			t.Fatalf("expected source_id ap, got %q", pdfMeta.Sources[0].ID)
		}
	} else {
		if pdfMeta.Provenance.SourceID != "ap" {
			t.Fatalf("expected source_id ap, got %q", pdfMeta.Provenance.SourceID)
		}
	}
	// Verify empty file was preserved
	emptyData, _ := os.ReadFile(filepath.Join(root, "skills", "default", "pdf", "empty.txt"))
	if len(emptyData) != 0 {
		t.Errorf("expected empty file, got %d bytes", len(emptyData))
	}

	// Verify binary file was preserved
	binData, _ := os.ReadFile(filepath.Join(root, "skills", "default", "pdf", "assets", "icon.png"))
	if string(binData) != string(binaryBytes) {
		t.Errorf("binary content mismatch: got %v, want %v", binData, binaryBytes)
	}
}

func TestSourceImportMoreDiscoversNewAndSkipsImported(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	// Create source record
	currentRev := revision("rev-more")
	sourceRec := sourcepkg.Record{
		SchemaVersion:   1,
		ID:              "src-more",
		Adapter:         "git",
		Locator:         sourcepkg.Locator{Repository: "https://github.com/example/more.git", Ref: "main"},
		Status:          "watching",
		Identity:        sourcepkg.Identity{Name: "more", Canonical: "https://github.com/example/more.git", DefaultBranch: "main"},
		Monitoring:      sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		CurrentRevision: &currentRev,
		Limits:          sourcepkg.Limits{TimeoutSeconds: 30, MaxBytes: 1024 * 1024, MaxFiles: 100, MaxFileBytes: 1024 * 1024},
	}
	sourceData, _ := sourcepkg.MarshalCanonical(sourceRec)
	_ = os.WriteFile(filepath.Join(root, "sources", "catalog", "src-more.yaml"), sourceData, 0o644)

	// Pre-vendor skill-one with provenance pointing to src-more and origin.path: skills/skill-one
	skillService := SkillService{}
	prev, err := skillService.PreviewCreate(context.Background(), root, skill.CreateInput{
		ID:          "skill-one",
		Collection:  "default",
		Name:        "skill-one",
		Description: "Existing skill one",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skillService.ConfirmSkillMutation(context.Background(), root, prev, prev.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(root, "skills", "default", "skill-one", ".meta", "skill.yaml")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		metaPath = filepath.Join(root, "skills", "default", "skill-one", "skill.meta.yaml")
	}
	metaBytes, _ := os.ReadFile(metaPath)
	var doc map[string]any
	_ = yaml.Unmarshal(metaBytes, &doc)
	if doc == nil {
		doc = make(map[string]any)
	}
	doc["sources"] = []any{
		map[string]any{
			"id":         "src-more",
			"roles":      []string{"upstream"},
			"kind":       "github",
			"path":       "skills/skill-one",
			"commit":     currentRev.Value,
			"repository": "https://github.com/example/repo",
		},
	}
	doc["provenance"] = map[string]any{
		"source_id": "src-more",
		"origin": map[string]any{
			"kind":   "github",
			"path":   "skills/skill-one",
			"commit": currentRev.Value,
		},
	}
	newMeta, _ := yaml.Marshal(doc)
	_ = os.WriteFile(metaPath, newMeta, 0o644)
	commitWorkspace(t, root)

	// Fake adapter with two skills: skills/skill-one and skills/skill-two
	skill1MD := "---\nname: skill-one\ndescription: Skill One\n---\n# Skill One\n"
	skill2MD := "---\nname: skill-two\ndescription: Skill Two\n---\n# Skill Two\n"
	adapter := &fakeImportAdapter{
		resources: []sourcepkg.Resource{
			{Path: "skills/skill-one/SKILL.md", Size: int64(len(skill1MD))},
			{Path: "skills/skill-two/SKILL.md", Size: int64(len(skill2MD))},
		},
		files: map[string][]byte{
			"skills/skill-one/SKILL.md": []byte(skill1MD),
			"skills/skill-two/SKILL.md": []byte(skill2MD),
		},
	}
	importService := SourceImportService{
		Clock:    sourceClock{now: now},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	// 1. Preview import with empty Path -> should derive commonParentDir "skills",
	// discover skill-one as imported/skipped, and skill-two as importable
	prop, err := importService.PreviewSourceImport(context.Background(), root, SourceImportPreviewInput{
		SourceID: "src-more",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(prop.Importable) != 1 || prop.Importable[0].TargetID != "skill-two" {
		t.Fatalf("expected 1 importable (skill-two), got %#v", prop.Importable)
	}
	if len(prop.Skipped) != 1 || prop.Skipped[0].TargetID != "skill-one" {
		t.Fatalf("expected 1 skipped (skill-one), got %#v", prop.Skipped)
	}
	if !prop.Skipped[0].Imported {
		t.Fatalf("expected skill-one to have Imported: true, got %#v", prop.Skipped[0])
	}

	// 2. Confirm import of skill-two
	res, err := importService.ConfirmSourceImport(context.Background(), root, prop, prop.Confirmation.Confirmation.Pins)
	if err != nil || res.Error != nil {
		t.Fatalf("confirm failed: %v, %#v", err, res.Error)
	}
	if res.ImportedCount != 1 || res.ImportedIDs[0] != "skill-two" {
		t.Fatalf("imported count/ids mismatch: %#v", res)
	}

	// 3. Preview again -> both should now be skipped, zero importable, status OK
	prop2, err := importService.PreviewSourceImport(context.Background(), root, SourceImportPreviewInput{
		SourceID: "src-more",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(prop2.Importable) != 0 {
		t.Fatalf("expected 0 importable on re-preview, got %d", len(prop2.Importable))
	}
	if prop2.Status != StatusOK {
		t.Fatalf("expected StatusOK on zero importable, got %v", prop2.Status)
	}
	if len(prop2.Diff.Added) != 0 {
		t.Fatalf("expected zero diff added, got %v", prop2.Diff.Added)
	}
}

func TestSourceImportInV3CreatesMetaSkillYAMLAndMatchesSkillAddTrust(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}

	repoDir := t.TempDir()
	runGitInDir(t, repoDir, "init", "-b", "main")
	runGitInDir(t, repoDir, "config", "user.name", "Test")
	runGitInDir(t, repoDir, "config", "user.email", "test@example.com")
	runGitInDir(t, repoDir, "config", "uploadpack.allowReachableSHA1InWant", "true")

	skillDir := filepath.Join(repoDir, "skills", "imported-skill")
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: imported-skill\ndescription: Skill imported from git repo\n---\n# Imported Skill\n\nInstructions.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "run.sh"), []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitInDir(t, repoDir, "add", ".")
	runGitInDir(t, repoDir, "commit", "-m", "initial commit")

	adapter := sourcepkg.GitRepositoryAdapter{
		CacheRoot:         filepath.Join(root, "runtime", "sources", "git"),
		AllowFileProtocol: true,
	}
	fileURL := "file://" + filepath.ToSlash(repoDir)

	headCommit := runGitInDir(t, repoDir, "rev-parse", "HEAD")
	now := time.Now().UTC()
	rev := sourcepkg.Revision{
		Kind:          "git-commit",
		Value:         headCommit,
		ContentDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		ObservedAt:    now,
	}
	sourceRec := sourcepkg.Record{
		SchemaVersion:   1,
		ID:              "src-git",
		Adapter:         "git",
		Locator:         sourcepkg.Locator{Repository: fileURL, Ref: "main", Path: "skills"},
		Status:          "watching",
		Identity:        sourcepkg.Identity{Name: "src-git", Canonical: fileURL},
		Limits:          sourcepkg.Limits{TimeoutSeconds: 20, MaxBytes: sourcepkg.DefaultMaxBytes, MaxFiles: sourcepkg.DefaultMaxFiles, MaxFileBytes: sourcepkg.DefaultMaxFileSize},
		Monitoring:      sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		CurrentRevision: &rev,
	}
	sourceBytes, err := sourcepkg.MarshalCanonical(sourceRec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources", "catalog", "src-git.yaml"), sourceBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	importService := SourceImportService{
		Clock:    sourceClock{now: now},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	preview, err := importService.PreviewSourceImport(context.Background(), root, SourceImportPreviewInput{
		SourceID: "src-git",
	})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview import failed: %v, %#v", err, preview.Error)
	}

	res, err := importService.ConfirmSourceImport(context.Background(), root, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil || res.Error != nil {
		t.Fatalf("confirm import failed: %v, %#v", err, res.Error)
	}

	// 1. In v3 workspace: .meta/skill.yaml is created, skill.meta.yaml is NOT created
	localMetaPath := filepath.Join(root, "skills", "default", "imported-skill", ".meta", "skill.yaml")
	metaBytes, err := os.ReadFile(localMetaPath)
	if err != nil {
		t.Fatalf("expected .meta/skill.yaml to exist: %v", err)
	}
	legacyMetaPath := filepath.Join(root, "skills", "default", "imported-skill", "skill.meta.yaml")
	if _, err := os.Stat(legacyMetaPath); !os.IsNotExist(err) {
		t.Fatalf("skill.meta.yaml must not exist in v3 workspace")
	}

	// Verify .meta/skill.yaml content: has repo, no repository, no content_digest, no provenance
	metaStr := string(metaBytes)
	if !strings.Contains(metaStr, "repo: ") {
		t.Fatalf("expected .meta/skill.yaml to contain 'repo: ', got:\n%s", metaStr)
	}
	if strings.Contains(metaStr, "repository:") {
		t.Fatalf(".meta/skill.yaml must not contain 'repository:', got:\n%s", metaStr)
	}
	if strings.Contains(metaStr, "content_digest:") {
		t.Fatalf(".meta/skill.yaml must not contain 'content_digest:', got:\n%s", metaStr)
	}
	if strings.Contains(metaStr, "provenance:") {
		t.Fatalf(".meta/skill.yaml must not contain 'provenance:', got:\n%s", metaStr)
	}

	// 2. canonical.Validate passes with 0 issues
	issues, err := canonical.Validate(root)
	if err != nil || len(issues) != 0 {
		t.Fatalf("canonical.Validate failed on imported skill: err=%v, issues=%#v", err, issues)
	}

	// 3. Compare trust verdict with skill_add for the same upstream
	addRoot := filepath.Join(t.TempDir(), "add-workspace")
	if _, err := workspace.Apply(addRoot); err != nil {
		t.Fatal(err)
	}
	addAdapter := sourcepkg.GitRepositoryAdapter{
		CacheRoot:         filepath.Join(addRoot, "runtime", "sources", "git"),
		AllowFileProtocol: true,
	}
	addService := SkillAddService{
		Clock:    sourceClock{now: time.Now().UTC()},
		Adapters: map[string]sourcepkg.Adapter{"git": addAdapter},
	}
	addPrev, err := addService.PreviewSkillAdd(context.Background(), addRoot, SkillAddInput{
		Locator:   fileURL,
		Selection: "imported-skill",
	})
	if err != nil || addPrev.Error != nil {
		t.Fatalf("preview add failed: %v, %#v", err, addPrev.Error)
	}
	_, err = addService.ConfirmSkillAdd(context.Background(), addRoot, addPrev, addPrev.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatalf("confirm add failed: %v", err)
	}

	skillService := SkillService{}
	importTrust, err := skillService.ContentTrustFor(context.Background(), root, "imported-skill")
	if err != nil {
		t.Fatalf("ContentTrustFor imported: %v", err)
	}
	addTrust, err := skillService.ContentTrustFor(context.Background(), addRoot, "imported-skill")
	if err != nil {
		t.Fatalf("ContentTrustFor added: %v", err)
	}

	if importTrust.ThirdParty != addTrust.ThirdParty {
		t.Fatalf("ThirdParty mismatch: import=%v, add=%v", importTrust.ThirdParty, addTrust.ThirdParty)
	}
	if importTrust.Approved != addTrust.Approved {
		t.Fatalf("Approved mismatch: import=%v, add=%v", importTrust.Approved, addTrust.Approved)
	}
	if importTrust.RequiresReview() != addTrust.RequiresReview() {
		t.Fatalf("RequiresReview mismatch: import=%v, add=%v", importTrust.RequiresReview(), addTrust.RequiresReview())
	}
	if importTrust.ContentDigest != addTrust.ContentDigest {
		t.Fatalf("ContentDigest mismatch: import=%q, add=%q", importTrust.ContentDigest, addTrust.ContentDigest)
	}
	if !reflect.DeepEqual(importTrust.ReasonCodes, addTrust.ReasonCodes) {
		t.Fatalf("ReasonCodes mismatch: import=%v, add=%v", importTrust.ReasonCodes, addTrust.ReasonCodes)
	}
}
