package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
)

func TestEnsureCatalogKeepsHealthyGenerationAndRebuildsMissingPointer(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	before, err := catalog.Inspect(context.Background(), root)
	if err != nil || before.State != catalog.StateHealthy || before.Pointer == nil {
		t.Fatalf("initial catalog = %#v, %v", before, err)
	}
	result, err := (CatalogService{}).EnsureCatalog(context.Background(), root)
	if err != nil || result.Status != StatusReady {
		t.Fatalf("healthy ensure = %#v, %v", result, err)
	}
	after, err := catalog.Inspect(context.Background(), root)
	if err != nil || after.Pointer == nil || after.Pointer.Generation != before.Pointer.Generation {
		t.Fatalf("healthy ensure rebuilt generation: %#v, %v", after, err)
	}
	if err := os.Remove(filepath.Join(root, "runtime", "catalog", "current.json")); err != nil {
		t.Fatal(err)
	}
	result, err = (CatalogService{}).EnsureCatalog(context.Background(), root)
	if err != nil || result.Status != StatusApplied {
		t.Fatalf("missing ensure = %#v, %v", result, err)
	}
	after, err = catalog.Inspect(context.Background(), root)
	if err != nil || after.State != catalog.StateHealthy || after.Pointer == nil || after.Pointer.Generation == before.Pointer.Generation {
		t.Fatalf("missing ensure did not rebuild: %#v, %v", after, err)
	}
}

func TestCatalogServiceInspectAndAssessSkill(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}

	service := CatalogService{}
	status, err := service.InspectCatalog(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if status.ServingMode != catalog.ServingCurrent {
		t.Fatalf("serving mode = %v, want %v", status.ServingMode, catalog.ServingCurrent)
	}

	// Add an active skill
	path := filepath.Join(root, "skills", "core", "my-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nname: my-skill\ndescription: Test skill.\n---\n\n# My Skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := filepath.Join(filepath.Dir(path), "skill.meta.yaml")
	if err := os.WriteFile(meta, []byte("schema_version: 1\nid: my-skill\nname: my-skill\nstatus: active\ndescription: Test skill.\nrouting:\n  triggers: [test]\n  not_for: [none]\n  min_scope: single_step\nquality:\n  reviewed: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}

	assessment, err := service.AssessSkill(t.Context(), root, "my-skill")
	if err != nil {
		t.Fatalf("AssessSkill failed: %v", err)
	}
	if !assessment.Canonical.Known || !assessment.Served.Known || !assessment.ResourcesMatch {
		t.Fatalf("unexpected assessment: %#v", assessment)
	}
}
