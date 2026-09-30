package app

import (
	"context"
	"fmt"

	catalogpkg "github.com/vantt/mcp-skill-hub/internal/catalog"
	migrationpkg "github.com/vantt/mcp-skill-hub/internal/migration"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// MigrationReceipt is the public, structured result of an applied migration.
type MigrationReceipt struct {
	OperationID         string   `json:"operation_id"`
	ChangedPaths        []string `json:"changed_paths"`
	CatalogSnapshot     string   `json:"catalog_snapshot"`
	Generation          string   `json:"generation"`
	GitDirty            bool     `json:"git_dirty"`
	SourceSchemaVersion int      `json:"source_schema_version"`
	TargetSchemaVersion int      `json:"target_schema_version"`
}

// MigrationResult is shared by human and JSON delivery adapters.
type MigrationResult struct {
	Result
	SourceSchemaVersion int                     `json:"source_schema_version"`
	TargetSchemaVersion int                     `json:"target_schema_version"`
	ProposalID          string                  `json:"proposal_id,omitempty"`
	ProposalDigest      string                  `json:"proposal_digest,omitempty"`
	BaseCatalogSnapshot string                  `json:"base_catalog_snapshot,omitempty"`
	Changes             []migrationpkg.FileDiff `json:"changes"`
	Receipt             *MigrationReceipt       `json:"receipt,omitempty"`
}

// MigrationService owns explicit canonical migration preview and confirmation.
type MigrationService struct{ Registry migrationpkg.Registry }

// Migrate previews by default. When confirmed, it applies the exact proposal
// produced by the same fresh inspection through the canonical mutation WAL.
func (service MigrationService) Migrate(ctx context.Context, path string, target int, yes bool) (MigrationResult, error) {
	if err := ctx.Err(); err != nil {
		return MigrationResult{}, err
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return MigrationResult{}, err
	}
	if target == 0 {
		target = migrationpkg.CurrentVersion
	}
	registry := service.Registry
	if registry.Empty() {
		registry = migrationpkg.DefaultRegistry()
	}
	source, err := migrationpkg.DetectVersion(root)
	if err != nil {
		return MigrationResult{}, err
	}
	if source == target {
		if yes {
			prior, found, lookupErr := lookupAppliedMigration(ctx, root, target)
			if lookupErr != nil {
				return MigrationResult{}, lookupErr
			}
			if found {
				return migrationRetryResult(prior), nil
			}
		}
		return MigrationResult{
			Result:              NewResult(StatusReady, fmt.Sprintf("Canonical schema is already at version %d; no migration was applied.", target)),
			SourceSchemaVersion: source, TargetSchemaVersion: target, Changes: []migrationpkg.FileDiff{},
		}, nil
	}
	proposal, err := registry.Preview(root, target)
	if err != nil {
		return MigrationResult{}, err
	}
	result := MigrationResult{
		Result:              NewResult(StatusActionRequired, fmt.Sprintf("Preview only: canonical schema migration %d to %d was not applied without --yes.", proposal.SourceVersion, proposal.TargetVersion)),
		SourceSchemaVersion: proposal.SourceVersion, TargetSchemaVersion: proposal.TargetVersion,
		ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseSnapshot,
		Changes: proposal.Changes,
	}
	result.Items = append(result.Items, Item{ID: "canonical_migration", Summary: fmt.Sprintf("Canonical schema %d -> %d (%d file change(s)).", proposal.SourceVersion, proposal.TargetVersion, len(proposal.Changes)), Impact: "Only the previewed canonical paths will change after explicit confirmation."})
	result.SuggestedActions = []Action{{Label: "Apply this migration after reviewing the fresh preview", Command: fmt.Sprintf("skillhub migrate --workspace %s --to %d --yes", root, target), RequiresConfirmation: true}}
	if !yes {
		return result, nil
	}
	confirmed, err := mutation.ConfirmMutationWithOptions(root, proposal.Mutation, mutation.Confirmation{
		ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseSnapshot,
	}, mutation.Options{PostCanonical: func(expected string) (mutation.Publication, error) {
		built, err := catalogpkg.BuildCatalogGenerationWhileLocked(ctx, root, expected, catalogpkg.BuildOptions{})
		if err != nil {
			return mutation.Publication{}, err
		}
		return mutation.Publication{CatalogSnapshot: built.Pointer.CatalogSnapshot, Generation: built.Pointer.Generation}, nil
	}})
	if err != nil {
		return MigrationResult{}, err
	}
	if confirmed.OperationID != proposal.Mutation.WriteSet.OperationID {
		// A recorded receipt was returned instead of a fresh write. It is only
		// truthful when the workspace still sits at the receipt's target version.
		current, detectErr := migrationpkg.DetectVersion(root)
		if detectErr != nil {
			return MigrationResult{}, detectErr
		}
		if current != proposal.TargetVersion {
			return MigrationResult{}, fmt.Errorf("%w: migration %d to %d was recorded by operation %s but the workspace is now at schema version %d", mutation.ErrConflict, proposal.SourceVersion, proposal.TargetVersion, confirmed.OperationID, current)
		}
		if confirmed.Generation == "" {
			confirmed.Generation, err = catalogpkg.FindGenerationForOperation(ctx, root, confirmed.OperationID, confirmed.CatalogSnapshot)
			if err != nil {
				return MigrationResult{}, fmt.Errorf("published generation for applied migration %s is unavailable: %w", confirmed.OperationID, err)
			}
		}
	}
	result.Status = StatusApplied
	result.Summary = fmt.Sprintf("Canonical schema migration %d to %d was applied and the catalog was rebuilt.", proposal.SourceVersion, proposal.TargetVersion)
	result.SuggestedActions = []Action{}
	result.Receipt = migrationReceipt(confirmed)
	return result, nil
}

func lookupAppliedMigration(ctx context.Context, root string, target int) (mutation.Receipt, bool, error) {
	// V1 has one explicit canonical transition. Reconstructing its normalized
	// request lets a confirmed retry find the immutable receipt even though the
	// successful marker update means a fresh preview is now a no-op.
	if target != 1 {
		return mutation.Receipt{}, false, nil
	}
	source := 0
	set := mutation.WriteSet{
		Command: "canonical_migration", IdempotencyKey: fmt.Sprintf("canonical-migration:%d:%d", source, target),
		SourceSchemaVersion: &source, TargetSchemaVersion: &target,
		Changes: []mutation.Change{{Path: ".skillhub/schema-version", Contents: []byte(fmt.Sprintf("%d\n", target))}},
	}
	digest, err := mutation.DigestRequest(set)
	if err != nil {
		return mutation.Receipt{}, false, err
	}
	set.RequestDigest = digest
	receipt, found, err := mutation.LookupOperation(root, set)
	if err != nil || !found {
		return mutation.Receipt{}, found, err
	}
	if receipt.SourceSchemaVersion == nil || receipt.TargetSchemaVersion == nil ||
		*receipt.SourceSchemaVersion != source || *receipt.TargetSchemaVersion != target {
		return mutation.Receipt{}, false, fmt.Errorf("applied migration receipt has invalid schema version context")
	}
	if receipt.Generation == "" {
		receipt.Generation, err = catalogpkg.FindGenerationForOperation(ctx, root, receipt.OperationID, receipt.CatalogSnapshot)
		if err != nil {
			return mutation.Receipt{}, false, fmt.Errorf("published generation for applied migration %s is unavailable: %w", receipt.OperationID, err)
		}
	}
	return receipt, true, nil
}

func migrationRetryResult(receipt mutation.Receipt) MigrationResult {
	source, target := *receipt.SourceSchemaVersion, *receipt.TargetSchemaVersion
	return MigrationResult{
		Result:              NewResult(StatusApplied, fmt.Sprintf("Canonical schema migration %d to %d was already applied; returning its prior receipt.", source, target)),
		SourceSchemaVersion: source, TargetSchemaVersion: target, Changes: []migrationpkg.FileDiff{},
		Receipt: migrationReceipt(receipt),
	}
}

func migrationReceipt(receipt mutation.Receipt) *MigrationReceipt {
	source, target := 0, 0
	if receipt.SourceSchemaVersion != nil {
		source = *receipt.SourceSchemaVersion
	}
	if receipt.TargetSchemaVersion != nil {
		target = *receipt.TargetSchemaVersion
	}
	return &MigrationReceipt{
		OperationID: receipt.OperationID, ChangedPaths: receipt.ChangedPaths,
		CatalogSnapshot: receipt.CatalogSnapshot, Generation: receipt.Generation, GitDirty: receipt.GitDirty,
		SourceSchemaVersion: source, TargetSchemaVersion: target,
	}
}
