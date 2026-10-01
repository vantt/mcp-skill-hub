package app

import (
	"context"
	"fmt"
	"sort"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// CatalogService owns all application entry points that build derived catalog state.
type CatalogService struct{}

// ProgressEvent is a transport-neutral long-operation update.
type ProgressEvent struct {
	Stage     string `json:"stage"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Message   string `json:"message"`
}

// ProgressSink receives ordered rebuild updates. Callbacks must return quickly.
type ProgressSink func(ProgressEvent)

// BuildCatalogGeneration performs the same offline full rebuild used by init and doctor --fix.
func (service CatalogService) BuildCatalogGeneration(ctx context.Context, path string) (Result, error) {
	return service.BuildCatalogGenerationWithProgress(ctx, path, nil)
}

// BuildCatalogGenerationWithProgress exposes shared progress semantics to CLI and tool adapters.
func (CatalogService) BuildCatalogGenerationWithProgress(ctx context.Context, path string, progress ProgressSink) (Result, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return Result{}, err
	}
	built, err := catalog.BuildCatalogGeneration(ctx, root, catalog.BuildOptions{Progress: func(event catalog.ProgressEvent) {
		if progress != nil {
			progress(ProgressEvent{Stage: event.Stage, Completed: event.Completed, Total: event.Total, Message: event.Message})
		}
	}})
	if err != nil {
		return Result{}, err
	}
	result := NewResult(StatusApplied, "Catalog generation rebuilt and published.")
	result.Items = append(result.Items,
		Item{ID: "generation", Summary: built.Pointer.Generation, Impact: "Published immutable catalog generation."},
		Item{ID: "catalog_snapshot", Summary: built.Pointer.CatalogSnapshot, Impact: "Canonical routing catalog identity."},
		Item{ID: "projection_input_digest", Summary: built.Pointer.ProjectionInputDigest, Impact: "Identity of every projected canonical input."},
	)
	tables := make([]string, 0, len(built.RowCounts))
	for table := range built.RowCounts {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		result.Items = append(result.Items, Item{ID: "rows_" + table, Summary: fmt.Sprintf("%s: %d row(s)", table, built.RowCounts[table]), Impact: "Verified projection row count."})
	}
	for _, warning := range built.Warnings {
		result.Warnings = append(result.Warnings, Warning{Code: "runtime_maintenance_warning", Summary: warning})
	}
	if built.Freshness != catalog.FreshnessCurrent {
		result.Status = StatusActionRequired
		result.Summary = "Catalog generation was published, but post-publish freshness is " + string(built.Freshness) + "."
		result.Warnings = append(result.Warnings, Warning{Code: "catalog_" + string(built.Freshness), Summary: built.FreshnessDetail})
	}
	return result, nil
}

// EnsureCatalog rebuilds only when the current generation is absent, stale, corrupt, or incompatible.
// It is suitable for future server/workspace startup paths because it never modifies canonical files.
func (CatalogService) EnsureCatalog(ctx context.Context, path string) (Result, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return Result{}, err
	}
	status, err := catalog.Inspect(ctx, root)
	if err != nil {
		return Result{}, err
	}
	if status.State == catalog.StateHealthy {
		result := NewResult(StatusReady, "Catalog generation is current.")
		result.Items = append(result.Items, Item{ID: "generation", Summary: status.Pointer.Generation, Impact: "Current immutable catalog generation."})
		return result, nil
	}
	return CatalogService{}.BuildCatalogGeneration(ctx, root)
}

// InspectCatalog reports the current catalog status without modifying state.
func (CatalogService) InspectCatalog(ctx context.Context, path string) (catalog.Status, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return catalog.Status{}, err
	}
	return catalog.Inspect(ctx, root)
}

// AssessSkill evaluates a skill's basis-aware state comparing canonical and served facts.
func (CatalogService) AssessSkill(ctx context.Context, path, id string) (catalog.SkillStateAssessment, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return catalog.SkillStateAssessment{}, err
	}
	return catalog.AssessSkillState(ctx, root, id)
}
