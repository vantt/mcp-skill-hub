package evaluation

import (
	"context"
	"errors"
	"fmt"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
)

var ErrPinnedArtifactUnavailable = errors.New("pinned replay artifact unavailable")

// ExactEnvironment describes the artifacts actually available to a runner.
// Every field is compared exactly; empty values are unavailable, not wildcards.
type ExactEnvironment struct {
	Root                 string
	PolicyRevisions      map[string]bool
	ProtocolSchema       string
	NormalizationVersion string
	IndexVersion         string
	FactProviderFixtures map[string]string
	Binary               BinaryIdentity
	Models               map[string]ModelIdentity
}

// ValidatePins opens the exact retained immutable catalog before comparing its
// identity. A missing, stale, or corrupt snapshot fails rather than falling
// back to the current generation.
func (environment ExactEnvironment) ValidatePins(ctx context.Context, manifest Manifest) error {
	if err := validateManifest(manifest); err != nil {
		return err
	}
	handle, err := catalog.OpenSnapshot(ctx, environment.Root, manifest.CatalogSnapshot)
	if err != nil {
		return fmt.Errorf("%w: catalog %s: %v", ErrPinnedArtifactUnavailable, manifest.CatalogSnapshot, err)
	}
	actualSnapshot := handle.Pointer.CatalogSnapshot
	if closeErr := handle.Close(); closeErr != nil {
		return fmt.Errorf("close pinned catalog: %w", closeErr)
	}
	if actualSnapshot != manifest.CatalogSnapshot {
		return fmt.Errorf("%w: catalog snapshot %s", ErrPinnedArtifactUnavailable, manifest.CatalogSnapshot)
	}
	if !environment.PolicyRevisions[manifest.PolicyRevision] {
		return fmt.Errorf("%w: policy revision %s", ErrPinnedArtifactUnavailable, manifest.PolicyRevision)
	}
	if environment.ProtocolSchema != manifest.ProtocolSchema || environment.NormalizationVersion != manifest.NormalizationVersion || environment.IndexVersion != manifest.IndexVersion {
		return fmt.Errorf("%w: protocol, normalization, or index identity", ErrPinnedArtifactUnavailable)
	}
	if environment.FactProviderFixtures[manifest.FactProviderFixture] != manifest.FactProviderVersion {
		return fmt.Errorf("%w: fact-provider fixture %s@%s", ErrPinnedArtifactUnavailable, manifest.FactProviderFixture, manifest.FactProviderVersion)
	}
	if environment.Binary != manifest.Binary {
		return fmt.Errorf("%w: binary identity", ErrPinnedArtifactUnavailable)
	}
	if manifest.Model != nil {
		actual, ok := environment.Models[modelKey(*manifest.Model)]
		if !ok || actual != *manifest.Model {
			return fmt.Errorf("%w: model identity", ErrPinnedArtifactUnavailable)
		}
	}
	return nil
}

func modelKey(model ModelIdentity) string {
	return model.Provider + "/" + model.Model + "/" + model.Configuration
}

// StaticPinValidator is useful for hermetic replay where artifacts are already
// materialized by the caller. It still validates every identity exactly.
type StaticPinValidator struct{ Available Manifest }

func (validator StaticPinValidator) ValidatePins(_ context.Context, manifest Manifest) error {
	if err := validateManifest(manifest); err != nil {
		return err
	}
	available := validator.Available
	if manifest.CatalogSnapshot != available.CatalogSnapshot || manifest.PolicyRevision != available.PolicyRevision || manifest.ProtocolSchema != available.ProtocolSchema || manifest.NormalizationVersion != available.NormalizationVersion || manifest.IndexVersion != available.IndexVersion || manifest.FactProviderFixture != available.FactProviderFixture || manifest.FactProviderVersion != available.FactProviderVersion || manifest.Binary != available.Binary {
		return ErrPinnedArtifactUnavailable
	}
	if (manifest.Model == nil) != (available.Model == nil) || manifest.Model != nil && *manifest.Model != *available.Model {
		return ErrPinnedArtifactUnavailable
	}
	return nil
}
