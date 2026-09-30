package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/evaluation"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/version"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

const (
	// EvaluationDefaultSeed is used for generated manifests unless the caller
	// explicitly supplies EvaluationService.Seed.
	EvaluationDefaultSeed int64 = 42
	// EvaluationNormalizationVersion identifies the request and routing normalization
	// implemented by the deterministic production resolver.
	EvaluationNormalizationVersion = "fts-rules-v1"
	// EvaluationIndexVersion identifies the SQLite FTS projection consumed by the
	// production resolver. It changes when behavior-affecting index semantics change.
	EvaluationIndexVersion = "sqlite-fts5-v2"
	// EvaluationFactProviderFixture identifies facts embedded in committed evaluation
	// requests. No live fact provider is consulted during replay.
	EvaluationFactProviderFixture = "request-fixture"
	EvaluationFactProviderVersion = "v1"
)

// EvaluationService runs strict evaluation artifacts against an exact retained
// catalog generation. Its zero value supports the deterministic production
// resolver and request-embedded fact fixtures.
type EvaluationService struct {
	// Seed overrides EvaluationDefaultSeed when generating a manifest. A pointer
	// preserves zero as an explicit deterministic seed.
	Seed *int64
	// Binary overrides the current process identity, primarily for a packaged
	// binary that also pins a digest. Nil uses version.Current().
	Binary *evaluation.BinaryIdentity
	// FactProviderFixtures replaces the built-in request-fixture identity when
	// non-nil. Values are exact fixture-name to version mappings.
	FactProviderFixtures map[string]string
	// Models lists explicitly available recorded model artifacts. The production
	// deterministic resolver uses none, so a nil map rejects model-pinned runs.
	Models map[string]evaluation.ModelIdentity
}

// GenerateManifest strictly loads a suite and pins every behavior-affecting
// identity needed to replay it against the current healthy catalog. The
// deterministic production resolver has no model identity, so Model is nil.
func (service EvaluationService) GenerateManifest(ctx context.Context, path, suitePath, experimentID, variant string) (evaluation.Manifest, error) {
	suite, err := evaluation.LoadSuite(suitePath)
	if err != nil {
		return evaluation.Manifest{}, err
	}
	digest, err := evaluation.DigestSuite(suite)
	if err != nil {
		return evaluation.Manifest{}, fmt.Errorf("digest evaluation suite: %w", err)
	}
	return service.generateManifest(ctx, path, experimentID, variant, evaluation.Manifest{SuiteDigest: digest})
}

// GenerateCaseManifest pins a single case digest so the manifest can drive an
// exact case replay.
func (service EvaluationService) GenerateCaseManifest(ctx context.Context, path, casePath, experimentID, variant string) (evaluation.Manifest, error) {
	test, err := evaluation.LoadCase(casePath)
	if err != nil {
		return evaluation.Manifest{}, err
	}
	digest, err := evaluation.DigestCase(test)
	if err != nil {
		return evaluation.Manifest{}, fmt.Errorf("digest evaluation case: %w", err)
	}
	return service.generateManifest(ctx, path, experimentID, variant, evaluation.Manifest{CaseDigest: digest})
}

func (service EvaluationService) generateManifest(ctx context.Context, path, experimentID, variant string, subject evaluation.Manifest) (manifest evaluation.Manifest, resultErr error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return evaluation.Manifest{}, err
	}
	handle, err := catalog.OpenCurrent(ctx, root)
	if err != nil {
		return evaluation.Manifest{}, fmt.Errorf("open current healthy catalog: %w", err)
	}
	defer func() {
		if closeErr := handle.Close(); resultErr == nil && closeErr != nil {
			resultErr = fmt.Errorf("close current catalog: %w", closeErr)
		}
	}()

	policy, err := resolverpkg.LoadPolicy(ctx, handle.DB)
	if err != nil {
		return evaluation.Manifest{}, fmt.Errorf("load current recommendation policy: %w", err)
	}
	seed := EvaluationDefaultSeed
	if service.Seed != nil {
		seed = *service.Seed
	}
	manifest = evaluation.Manifest{
		SchemaVersion:        evaluation.SchemaVersion,
		ExperimentID:         experimentID,
		CaseDigest:           subject.CaseDigest,
		SuiteDigest:          subject.SuiteDigest,
		CatalogSnapshot:      handle.Pointer.CatalogSnapshot,
		PolicyRevision:       policy.Revision,
		ProtocolSchema:       resolverpkg.SchemaVersion,
		NormalizationVersion: EvaluationNormalizationVersion,
		IndexVersion:         EvaluationIndexVersion,
		FactProviderFixture:  EvaluationFactProviderFixture,
		FactProviderVersion:  EvaluationFactProviderVersion,
		Seed:                 seed,
		Binary:               service.binaryIdentity(),
		Variant:              variant,
	}
	// Round-trip through the strict parser so all caller-provided identities and
	// bounds are validated at this application boundary.
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return evaluation.Manifest{}, fmt.Errorf("marshal generated manifest: %w", err)
	}
	validated, err := evaluation.ParseManifest(encoded, evaluation.FormatJSON)
	if err != nil {
		return evaluation.Manifest{}, fmt.Errorf("validate generated manifest: %w", err)
	}
	return validated, nil
}

// RunSuite loads a strictly decoded suite and evaluates the selected partitions
// with the production resolver over the manifest's exact catalog generation.
func (service EvaluationService) RunSuite(ctx context.Context, path, suitePath string, manifest evaluation.Manifest, options evaluation.RunOptions) (evaluation.Report, error) {
	suite, err := evaluation.LoadSuite(suitePath)
	if err != nil {
		return evaluation.Report{}, err
	}
	return service.run(ctx, path, manifest, func(runner *evaluation.Runner) (evaluation.Report, error) {
		return runner.RunSuite(ctx, suite, manifest, deterministicRunOptions(options))
	})
}

// ReplayCase loads a strictly decoded case and replays it with the production
// resolver over the manifest's exact catalog generation.
func (service EvaluationService) ReplayCase(ctx context.Context, path, casePath string, manifest evaluation.Manifest, options evaluation.RunOptions) (evaluation.Report, error) {
	test, err := evaluation.LoadCase(casePath)
	if err != nil {
		return evaluation.Report{}, err
	}
	return service.run(ctx, path, manifest, func(runner *evaluation.Runner) (evaluation.Report, error) {
		return runner.RunCase(ctx, test, manifest, deterministicRunOptions(options))
	})
}

func (service EvaluationService) run(ctx context.Context, path string, manifest evaluation.Manifest, execute func(*evaluation.Runner) (evaluation.Report, error)) (report evaluation.Report, resultErr error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return evaluation.Report{}, err
	}
	handle, err := catalog.OpenSnapshot(ctx, root, manifest.CatalogSnapshot)
	if err != nil {
		return evaluation.Report{}, fmt.Errorf("%w: catalog %s: %v", evaluation.ErrPinnedArtifactUnavailable, manifest.CatalogSnapshot, err)
	}
	defer func() {
		if closeErr := handle.Close(); resultErr == nil && closeErr != nil {
			resultErr = fmt.Errorf("close pinned catalog: %w", closeErr)
		}
	}()

	policy, err := resolverpkg.LoadPolicy(ctx, handle.DB)
	if err != nil {
		return evaluation.Report{}, fmt.Errorf("load pinned recommendation policy: %w", err)
	}
	if policy.Revision != manifest.PolicyRevision {
		return evaluation.Report{}, fmt.Errorf("%w: policy revision %s", evaluation.ErrPinnedArtifactUnavailable, manifest.PolicyRevision)
	}
	if err := service.validateEnvironment(manifest); err != nil {
		return evaluation.Report{}, err
	}
	view, err := resolverpkg.NewSQLiteCatalog(handle.DB, handle.Pointer.CatalogSnapshot)
	if err != nil {
		return evaluation.Report{}, fmt.Errorf("create pinned catalog view: %w", err)
	}
	engine, err := resolverpkg.New(view, policy, resolverpkg.NewCache(256))
	if err != nil {
		return evaluation.Report{}, fmt.Errorf("create deterministic resolver: %w", err)
	}
	// The exact generation and all environment identities were verified above.
	// Static validation lets Runner recheck the complete manifest without opening
	// a second generation or introducing a current/latest fallback path.
	runner, err := evaluation.NewRunner(engine, evaluation.StaticPinValidator{Available: manifest})
	if err != nil {
		return evaluation.Report{}, err
	}
	return execute(runner)
}

func (service EvaluationService) validateEnvironment(manifest evaluation.Manifest) error {
	if manifest.ProtocolSchema != resolverpkg.SchemaVersion ||
		manifest.NormalizationVersion != EvaluationNormalizationVersion ||
		manifest.IndexVersion != EvaluationIndexVersion {
		return fmt.Errorf("%w: protocol, normalization, or index identity", evaluation.ErrPinnedArtifactUnavailable)
	}
	providers := service.FactProviderFixtures
	if providers == nil {
		providers = map[string]string{EvaluationFactProviderFixture: EvaluationFactProviderVersion}
	}
	if providers[manifest.FactProviderFixture] != manifest.FactProviderVersion {
		return fmt.Errorf("%w: fact-provider fixture %s@%s", evaluation.ErrPinnedArtifactUnavailable, manifest.FactProviderFixture, manifest.FactProviderVersion)
	}
	if service.binaryIdentity() != manifest.Binary {
		return fmt.Errorf("%w: binary identity", evaluation.ErrPinnedArtifactUnavailable)
	}
	if manifest.Model != nil && !containsModel(service.Models, *manifest.Model) {
		return fmt.Errorf("%w: model identity", evaluation.ErrPinnedArtifactUnavailable)
	}
	return nil
}

func (service EvaluationService) binaryIdentity() evaluation.BinaryIdentity {
	if service.Binary != nil {
		return *service.Binary
	}
	current := version.Current()
	return evaluation.BinaryIdentity{Version: current.Version, Commit: current.Commit}
}

func containsModel(models map[string]evaluation.ModelIdentity, wanted evaluation.ModelIdentity) bool {
	for _, model := range models {
		if model == wanted {
			return true
		}
	}
	return false
}

type deterministicEvaluationClock struct{}

func (deterministicEvaluationClock) Now() time.Time { return time.Unix(0, 0).UTC() }

func deterministicRunOptions(options evaluation.RunOptions) evaluation.RunOptions {
	// Wall-clock latency makes serialized reports differ across exact replays.
	// Application evaluation therefore measures logical outcomes deterministically;
	// transport-level benchmarks remain responsible for real latency measurements.
	options.Clock = deterministicEvaluationClock{}
	return options
}
