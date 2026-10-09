package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/distill"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"gopkg.in/yaml.v3"
)

func TestSourceLinks(t *testing.T) {
	root := newSourceWorkspace(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	// Create test skill
	skillService := SkillService{}
	createPrev, err := skillService.PreviewCreate(ctx, root, skill.CreateInput{
		ID:          "test-skill",
		Collection:  "default",
		Name:        "Test Skill",
		Description: "A skill for testing links.",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = skillService.ConfirmSkillMutation(ctx, root, createPrev, createPrev.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatal(err)
	}

	metaPath := filepath.Join(root, "skills", "default", "test-skill", ".meta", "skill.yaml")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		metaPath = filepath.Join(root, "skills", "default", "test-skill", "skill.meta.yaml")
	}
	metaBytesBefore, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}

	adapter := &fakeImportAdapter{
		fakeSourceAdapter: fakeSourceAdapter{
			revisions: map[string]sourcepkg.Revision{
				"skills":       revision("one"),
				"custom-watch": revision("one"),
			},
		},
	}
	service := SourceService{
		Clock:    sourceClock{now: now},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	// 1. Attach by locator creates source + link and leaves skill's meta byte-identical
	attachPrev, err := service.PreviewAttach(ctx, root, SourceAttachInput{
		SkillID: "test-skill",
		Locator: testGitHubURL,
	})
	if err != nil || attachPrev.Error != nil {
		t.Fatalf("attach preview failed: %v, %#v", err, attachPrev.Error)
	}
	if attachPrev.WriteCommand() != "source_attach" {
		t.Fatalf("expected WriteCommand 'source_attach', got %q", attachPrev.WriteCommand())
	}
	sourceID := attachPrev.Source.ID

	confirmRes, err := service.ConfirmSourceProposal(ctx, root, attachPrev, attachPrev.Confirmation.Confirmation.Pins)
	if err != nil || confirmRes.Error != nil {
		t.Fatalf("confirm attach failed: %v, %#v", err, confirmRes.Error)
	}

	// Source record and link file created
	sourceRecordPath := filepath.Join(root, "sources", "catalog", sourceID+".yaml")
	if _, err := os.Stat(sourceRecordPath); err != nil {
		t.Fatalf("source record missing: %v", err)
	}
	linkPath := filepath.Join(root, "sources", "skills", "LINK-test-skill--"+sourceID+".yaml")
	if _, err := os.Stat(linkPath); err != nil {
		t.Fatalf("link file missing: %v", err)
	}

	// Skill meta byte-identical (no source_id)
	metaBytesAfter, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(metaBytesBefore, metaBytesAfter) {
		t.Fatalf("skill meta was modified by attach")
	}

	// canonical.Validate returns no issues
	issues, err := canonical.Validate(root)
	if err != nil || len(issues) != 0 {
		t.Fatalf("canonical validate issues after attach: %v, %v", err, issues)
	}

	// 2. Attach again -> "already linked"
	attachAgain, err := service.PreviewAttach(ctx, root, SourceAttachInput{
		SkillID: "test-skill",
		Locator: testGitHubURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if attachAgain.Status != StatusOK || attachAgain.Confirmation.Confirmation.Pins.ProposalID != "" {
		t.Fatalf("expected already linked StatusOK without pins, got %#v", attachAgain)
	}

	// 3. Detach when an observation references the source keeps it with monitoring off and warning source_kept_referenced
	// Update source record to have DistilledRevision so it can be prepared
	fromRev := revision("one")
	toRev := revision("two")
	adapter.revisions[sourceID] = toRev
	recData, _ := os.ReadFile(sourceRecordPath)
	var recObj sourcepkg.Record
	_ = yaml.Unmarshal(recData, &recObj)
	recObj.DistilledRevision = &fromRev
	recObj.CurrentRevision = &toRev
	recObj.Status = "changed"
	recBytes, _ := sourcepkg.MarshalCanonical(recObj)
	_ = os.WriteFile(sourceRecordPath, recBytes, 0o644)
	commitWorkspace(t, root)

	skillContent := []byte("# Target\nTest skill content.\n")
	adapter.files = map[string][]byte{
		"SKILL.md": skillContent,
	}
	adapter.resources = []sourcepkg.Resource{
		{Path: "SKILL.md", Size: int64(len(skillContent))},
	}

	distillService := DistillService{
		Clock:    distillClock{value: now},
		IDs:      fixedSourceID("distillrun000001"),
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}
	run := prepareAndStart(t, root, distillService, sourceID)
	toIdentity := distill.IdentityOf(run.ToRevision)

	obsItem := distill.Observation{
		SchemaVersion: 1,
		ID:            distill.ObservationID(sourceID, "feature"),
		SourceID:      sourceID,
		RunID:         run.ID,
		StableKey:     "feature",
		Status:        "active",
		What:          "Important finding",
		FirstSeen:     toIdentity,
		LastSeen:      toIdentity,
		Vocabulary:    []string{"feature"},
		Evidence: []distill.Evidence{{
			Revision:      toIdentity,
			RunID:         run.ID,
			PackageDigest: run.PackageDigest,
			Path:          "SKILL.md",
			Locator:       "SKILL.md",
			Digest:        sourcepkg.Digest(skillContent),
		}},
	}
	obsBytes, _ := yaml.Marshal(obsItem)
	obsDir := filepath.Join(root, "distill", "sources", sourceID, "observations")
	_ = os.MkdirAll(obsDir, 0o755)
	run.FindingIDs = []string{obsItem.ID}
	runBytes, _ := yaml.Marshal(run)
	_ = os.WriteFile(filepath.Join(root, "distill", "sources", sourceID, "runs", run.ID+".yaml"), runBytes, 0o644)
	_ = os.WriteFile(filepath.Join(obsDir, obsItem.ID+".yaml"), obsBytes, 0o644)
	commitWorkspace(t, root)
	_, _ = catalog.BuildCatalogGeneration(ctx, root, catalog.BuildOptions{})

	detachPrevRef, err := service.PreviewDetach(ctx, root, "test-skill", sourceID)
	if err != nil || detachPrevRef.Error != nil {
		t.Fatalf("preview detach referenced failed: %v, %#v", err, detachPrevRef.Error)
	}
	hasWarning := false
	for _, w := range detachPrevRef.Warnings {
		if w.Code == "source_kept_referenced" {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Fatalf("expected warning source_kept_referenced, got: %#v", detachPrevRef.Warnings)
	}

	confirmDetachRef, err := service.ConfirmSourceProposal(ctx, root, detachPrevRef, detachPrevRef.Confirmation.Confirmation.Pins)
	if err != nil || confirmDetachRef.Error != nil {
		t.Fatalf("confirm detach referenced failed: %v, %#v", err, confirmDetachRef.Error)
	}
	// Source record is kept on disk with monitoring disabled
	srcData, err := os.ReadFile(sourceRecordPath)
	if err != nil {
		t.Fatalf("source record should have been kept: %v", err)
	}
	rec, err := sourcepkg.ParseRecord(srcData)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Monitoring.Enabled || rec.Monitoring.Cadence != "manual" {
		t.Fatalf("expected monitoring disabled, got %#v", rec.Monitoring)
	}
	// Link file is deleted
	if _, err := os.Stat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("link file should have been deleted")
	}

	issues, err = canonical.Validate(root)
	if err != nil || len(issues) != 0 {
		t.Fatalf("canonical validate issues after detach referenced: %v, %v", err, issues)
	}

	// 4. Detach of the only link deletes the source (for a source without external references)
	// Create another source and link
	secAttach, err := service.PreviewAttach(ctx, root, SourceAttachInput{
		SkillID:  "test-skill",
		SourceID: sourceID,
	})
	if err != nil || secAttach.Error != nil {
		t.Fatalf("re-attach failed: %v, %#v", err, secAttach.Error)
	}
	_, err = service.ConfirmSourceProposal(ctx, root, secAttach, secAttach.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatal(err)
	}
	// Remove observation reference so source is completely unreferenced outside the link
	_ = os.RemoveAll(filepath.Join(root, "distill"))
	commitWorkspace(t, root)
	_, _ = catalog.BuildCatalogGeneration(ctx, root, catalog.BuildOptions{})

	detachDeletePrev, err := service.PreviewDetach(ctx, root, "test-skill", sourceID)
	if err != nil || detachDeletePrev.Error != nil {
		t.Fatalf("preview detach delete failed: %v, %#v", err, detachDeletePrev.Error)
	}
	_, err = service.ConfirmSourceProposal(ctx, root, detachDeletePrev, detachDeletePrev.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatal(err)
	}
	// Both source and link are deleted
	if _, err := os.Stat(sourceRecordPath); !os.IsNotExist(err) {
		t.Fatalf("expected source record deleted on disk")
	}
	if _, err := os.Stat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("expected link file deleted on disk")
	}

	issues, err = canonical.Validate(root)
	if err != nil || len(issues) != 0 {
		t.Fatalf("canonical validate issues after delete detach: %v, %v", err, issues)
	}

	// 5. Unwatch of a source with upstream skills turns monitoring off and keeps the record
	// Add an upstream tracked skill
	addService := SkillAddService{
		Clock:    sourceClock{now: now},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}
	// Create an upstream source manually for unwatch test
	upSourceRec := sourcepkg.Record{
		SchemaVersion: 1,
		ID:            "src-with-upstream-skill",
		Purpose:       "upstream",
		Adapter:       "git",
		Locator:       sourcepkg.Locator{Repository: "https://github.com/example/upstream.git", Ref: "main"},
		Status:        "watching",
		Identity:      sourcepkg.Identity{Name: "upstream", Canonical: "https://github.com/example/upstream.git"},
		Limits:        sourcepkg.Limits{TimeoutSeconds: 20, MaxBytes: sourcepkg.DefaultMaxBytes, MaxFiles: sourcepkg.DefaultMaxFiles, MaxFileBytes: sourcepkg.DefaultMaxFileSize},
		Monitoring:    sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
	}
	upBytes, _ := sourcepkg.MarshalCanonical(upSourceRec)
	_ = os.WriteFile(filepath.Join(root, "sources", "catalog", upSourceRec.ID+".yaml"), upBytes, 0o644)

	// Link test-skill to this source via provenance.source_id
	metaData, _ := os.ReadFile(metaPath)
	var doc map[string]any
	if err := yaml.Unmarshal(metaData, &doc); err != nil {
		t.Fatal(err)
	}
	prov, ok := doc["provenance"].(map[string]any)
	if !ok || prov == nil {
		prov = make(map[string]any)
	}
	prov["source_id"] = "src-with-upstream-skill"
	doc["provenance"] = prov
	newMeta, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(metaPath, newMeta, 0o644)
	commitWorkspace(t, root)
	_, _ = catalog.BuildCatalogGeneration(ctx, root, catalog.BuildOptions{})

	unwatchPrev, err := service.PreviewUnwatch(ctx, root, upSourceRec.ID)
	if err != nil || unwatchPrev.Error != nil {
		t.Fatalf("preview unwatch failed: %v, %#v", err, unwatchPrev.Error)
	}
	unwatchRes, err := service.ConfirmSourceProposal(ctx, root, unwatchPrev, unwatchPrev.Confirmation.Confirmation.Pins)
	if err != nil || unwatchRes.Error != nil {
		t.Fatalf("confirm unwatch failed: %v, %#v", err, unwatchRes.Error)
	}
	if unwatchRes.Summary != "Stopped watching src-with-upstream-skill." {
		t.Fatalf("expected summary 'Stopped watching src-with-upstream-skill.', got %q", unwatchRes.Summary)
	}

	// Record kept on disk with monitoring disabled
	upDataAfter, err := os.ReadFile(filepath.Join(root, "sources", "catalog", upSourceRec.ID+".yaml"))
	if err != nil {
		t.Fatalf("upstream source record should be kept: %v", err)
	}
	recAfter, _ := sourcepkg.ParseRecord(upDataAfter)
	if recAfter.Monitoring.Enabled || recAfter.Monitoring.Cadence != "manual" {
		t.Fatalf("expected monitoring disabled, got %#v", recAfter.Monitoring)
	}

	issues, err = canonical.Validate(root)
	if err != nil || len(issues) != 0 {
		t.Fatalf("canonical validate issues after unwatch: %v, %v", err, issues)
	}
	_ = addService
}
