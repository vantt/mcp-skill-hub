package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
)

func TestEnsureCatalogKeepsHealthyGenerationAndRebuildsMissingPointer(t *testing.T) {
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
