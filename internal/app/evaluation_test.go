package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/evaluation"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/version"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestEvaluationServiceGenerateManifestRunsImmediatelyWithoutCanonicalMutation(t *testing.T) {
	root, current := evaluationCurrentWorkspace(t)
	suite, suitePath := evaluationSuiteFixture(t)
	before, err := canonical.Scan(root)
	if err != nil {
		t.Fatal(err)
	}

	manifest, err := (EvaluationService{}).GenerateManifest(t.Context(), root, suitePath, "generated-experiment", "production-deterministic")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := evaluation.DigestSuite(suite)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ExperimentID != "generated-experiment" || manifest.Variant != "production-deterministic" || manifest.SuiteDigest != digest || manifest.CatalogSnapshot != current {
		t.Fatalf("manifest = %#v", manifest)
	}
	if manifest.Seed != EvaluationDefaultSeed || manifest.Model != nil || manifest.Binary.Version == "" || manifest.Binary.Commit == "" {
		t.Fatalf("generated replay identities = %#v", manifest)
	}
	if _, err := (EvaluationService{}).RunSuite(t.Context(), root, suitePath, manifest, evaluation.RunOptions{BootstrapSamples: 100}); err != nil {
		t.Fatalf("run generated manifest: %v", err)
	}
	after, err := canonical.Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("manifest generation or run changed canonical state:\nbefore=%#v\nafter=%#v", before, after)
	}

	if err := os.RemoveAll(filepath.Join(root, "runtime", "catalog", "generations")); err != nil {
		t.Fatal(err)
	}
	if _, err := (EvaluationService{}).RunSuite(t.Context(), root, suitePath, manifest, evaluation.RunOptions{BootstrapSamples: 100}); !errors.Is(err, evaluation.ErrPinnedArtifactUnavailable) {
		t.Fatalf("run after pin removal error = %v, want pinned artifact unavailable", err)
	}
}

func TestEvaluationServiceGenerateManifestUsesExplicitSeedAndStrictSuite(t *testing.T) {
	root, _ := evaluationCurrentWorkspace(t)
	_, suitePath := evaluationSuiteFixture(t)
	seed := int64(0)
	manifest, err := (EvaluationService{Seed: &seed}).GenerateManifest(t.Context(), root, suitePath, "seeded", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Seed != 0 {
		t.Fatalf("seed = %d, want explicit zero", manifest.Seed)
	}

	invalidPath := filepath.Join(t.TempDir(), "suite.json")
	if err := os.WriteFile(invalidPath, []byte(`{"schema_version":1,"id":"strict","cases":[],"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (EvaluationService{}).GenerateManifest(t.Context(), root, invalidPath, "strict", "candidate"); err == nil {
		t.Fatal("unknown suite field was accepted")
	}
}

func TestEvaluationServiceReplaysCurrentAndHistoricalSnapshotsExactly(t *testing.T) {
	root, historical, current := evaluationWorkspace(t)
	suite, suitePath := evaluationSuiteFixture(t)

	service := EvaluationService{}
	for _, snapshot := range []string{historical, current} {
		manifest := evaluationManifest(t, root, snapshot, suite, nil)
		report, err := service.RunSuite(t.Context(), root, suitePath, manifest, evaluation.RunOptions{
			Partitions:       []evaluation.Partition{evaluation.PartitionHeldOut},
			BootstrapSamples: 100,
		})
		if err != nil {
			t.Fatalf("run snapshot %s: %v", snapshot, err)
		}
		if report.Manifest.CatalogSnapshot != snapshot || report.Samples != 1 || len(report.Outcomes) != 1 || report.Outcomes[0].Partition != evaluation.PartitionHeldOut {
			t.Fatalf("snapshot %s report = %#v", snapshot, report)
		}
	}
}

func TestEvaluationServiceReplaysStrictCaseFromPinnedSnapshot(t *testing.T) {
	root, current := evaluationCurrentWorkspace(t)
	test := evaluationCase("case-replay", evaluation.PartitionDevelopment)
	casePath := filepath.Join(t.TempDir(), "case.json")
	writeEvaluationJSON(t, casePath, test)
	digest, err := evaluation.DigestCase(test)
	if err != nil {
		t.Fatal(err)
	}
	manifest := evaluationManifestBase(t, root, current)
	manifest.CaseDigest = digest

	report, err := (EvaluationService{}).ReplayCase(t.Context(), root, casePath, manifest, evaluation.RunOptions{BootstrapSamples: 100})
	if err != nil {
		t.Fatal(err)
	}
	if report.Samples != 1 || report.Outcomes[0].CaseID != test.ID || report.Outcomes[0].Status != resolverpkg.StatusNoSkill {
		t.Fatalf("report = %#v", report)
	}
}

func TestEvaluationServiceFailsClosedWhenSnapshotIsUnavailable(t *testing.T) {
	root, current := evaluationCurrentWorkspace(t)
	suite, suitePath := evaluationSuiteFixture(t)
	manifest := evaluationManifest(t, root, current, suite, nil)
	manifest.CatalogSnapshot = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	_, err := (EvaluationService{}).RunSuite(t.Context(), root, suitePath, manifest, evaluation.RunOptions{BootstrapSamples: 100})
	if !errors.Is(err, evaluation.ErrPinnedArtifactUnavailable) {
		t.Fatalf("error = %v, want pinned artifact unavailable", err)
	}
}

func TestEvaluationServiceFailsClosedOnPolicyAndEnvironmentMismatch(t *testing.T) {
	root, current := evaluationCurrentWorkspace(t)
	suite, suitePath := evaluationSuiteFixture(t)
	base := evaluationManifest(t, root, current, suite, nil)

	tests := []struct {
		name   string
		mutate func(*evaluation.Manifest)
	}{
		{name: "policy", mutate: func(manifest *evaluation.Manifest) {
			manifest.PolicyRevision = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}},
		{name: "protocol", mutate: func(manifest *evaluation.Manifest) { manifest.ProtocolSchema = "2" }},
		{name: "normalization", mutate: func(manifest *evaluation.Manifest) { manifest.NormalizationVersion = "latest" }},
		{name: "index", mutate: func(manifest *evaluation.Manifest) { manifest.IndexVersion = "latest" }},
		{name: "fact-provider", mutate: func(manifest *evaluation.Manifest) { manifest.FactProviderVersion = "latest" }},
		{name: "binary", mutate: func(manifest *evaluation.Manifest) { manifest.Binary.Commit = "other" }},
		{name: "unsupported-model", mutate: func(manifest *evaluation.Manifest) {
			manifest.Model = &evaluation.ModelIdentity{Provider: "recorded", Model: "ranker", Configuration: "v1"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := base
			test.mutate(&manifest)
			_, err := (EvaluationService{}).RunSuite(t.Context(), root, suitePath, manifest, evaluation.RunOptions{BootstrapSamples: 100})
			if !errors.Is(err, evaluation.ErrPinnedArtifactUnavailable) {
				t.Fatalf("error = %v, want pinned artifact unavailable", err)
			}
		})
	}

	model := evaluation.ModelIdentity{Provider: "recorded", Model: "ranker", Configuration: "v1"}
	withModel := base
	withModel.Model = &model
	service := EvaluationService{Models: map[string]evaluation.ModelIdentity{"fixture": model}}
	if _, err := service.RunSuite(t.Context(), root, suitePath, withModel, evaluation.RunOptions{BootstrapSamples: 100}); err != nil {
		t.Fatalf("explicitly supplied model rejected: %v", err)
	}
}

func TestEvaluationServiceReportBytesAreDeterministic(t *testing.T) {
	root, current := evaluationCurrentWorkspace(t)
	suite, suitePath := evaluationSuiteFixture(t)
	manifest := evaluationManifest(t, root, current, suite, nil)
	options := evaluation.RunOptions{Partitions: []evaluation.Partition{evaluation.PartitionHeldOut}, BootstrapSamples: 200}

	first, err := (EvaluationService{}).RunSuite(t.Context(), root, suitePath, manifest, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := (EvaluationService{}).RunSuite(t.Context(), root, suitePath, manifest, options)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := evaluation.MarshalReport(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := evaluation.MarshalReport(second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstBytes, secondBytes) {
		t.Fatalf("reports differ:\n%s\n%s", firstBytes, secondBytes)
	}
}

func TestEvaluationServiceDoesNotMutateCanonicalState(t *testing.T) {
	root, current := evaluationCurrentWorkspace(t)
	suite, suitePath := evaluationSuiteFixture(t)
	manifest := evaluationManifest(t, root, current, suite, nil)
	before, err := canonical.Scan(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := (EvaluationService{}).RunSuite(t.Context(), root, suitePath, manifest, evaluation.RunOptions{BootstrapSamples: 100}); err != nil {
		t.Fatal(err)
	}
	after, err := canonical.Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("canonical state changed:\nbefore=%#v\nafter=%#v", before, after)
	}
}

func evaluationCurrentWorkspace(t *testing.T) (root, current string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	result, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return root, result.Pointer.CatalogSnapshot
}

func evaluationWorkspace(t *testing.T) (root, historical, current string) {
	t.Helper()
	root, historical = evaluationCurrentWorkspace(t)

	policyPath := filepath.Join(root, "config", "recommendation.yaml")
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	policy := `schema_version: 1
policy_version: calibrated-v1
normalization_version: fts-rules-v1
weights:
  lexical: 0.28
  trigger: 0.38
  artifact: 0.08
  fact: 0.10
  operation: 0.15
  quality: 0.03
  not_for_penalty: 0.42
  constraint_penalty: 0.48
  scope_penalty: 0.25
thresholds:
  applicability_floor: 0.16
  high_confidence: 0.68
  minimum_margin: 0.04
  ambiguity_window: 0.04
  supporting_floor: 0.08
extensions:
  vector: disabled
  llm: disabled
`
	if err := os.WriteFile(policyPath, []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	current = result.Pointer.CatalogSnapshot
	if historical == current {
		t.Fatal("test workspace did not create distinct catalog snapshots")
	}
	return root, historical, current
}

func evaluationSuiteFixture(t *testing.T) (evaluation.Suite, string) {
	t.Helper()
	suite := evaluation.Suite{
		SchemaVersion: evaluation.SchemaVersion,
		ID:            "strict-suite",
		Cases: []evaluation.Case{
			evaluationCase("development-case", evaluation.PartitionDevelopment),
			evaluationCase("held-out-case", evaluation.PartitionHeldOut),
		},
	}
	path := filepath.Join(t.TempDir(), "suite.json")
	writeEvaluationJSON(t, path, suite)
	return suite, path
}

func evaluationCase(id string, partition evaluation.Partition) evaluation.Case {
	return evaluation.Case{
		SchemaVersion: evaluation.SchemaVersion,
		ID:            id,
		Partition:     partition,
		Request: resolverpkg.Request{
			SchemaVersion: resolverpkg.SchemaVersion,
			RequestID:     id,
			Task:          resolverpkg.Task{Description: "find a nonexistent quantum gardening procedure"},
		},
		Expected: evaluation.Expected{
			AcceptableStatuses: []resolverpkg.Status{resolverpkg.StatusNoSkill},
			NoSkill:            true,
		},
	}
}

func evaluationManifest(t *testing.T, root, snapshot string, suite evaluation.Suite, model *evaluation.ModelIdentity) evaluation.Manifest {
	t.Helper()
	manifest := evaluationManifestBase(t, root, snapshot)
	digest, err := evaluation.DigestSuite(suite)
	if err != nil {
		t.Fatal(err)
	}
	manifest.SuiteDigest = digest
	manifest.Model = model
	return manifest
}

func evaluationManifestBase(t *testing.T, root, snapshot string) evaluation.Manifest {
	t.Helper()
	handle, err := catalog.OpenSnapshot(context.Background(), root, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	policy, policyErr := resolverpkg.LoadPolicy(context.Background(), handle.DB)
	closeErr := handle.Close()
	if policyErr != nil {
		t.Fatal(policyErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	build := version.Current()
	return evaluation.Manifest{
		SchemaVersion:        evaluation.SchemaVersion,
		ExperimentID:         "strict-replay",
		CatalogSnapshot:      snapshot,
		PolicyRevision:       policy.Revision,
		ProtocolSchema:       resolverpkg.SchemaVersion,
		NormalizationVersion: EvaluationNormalizationVersion,
		IndexVersion:         EvaluationIndexVersion,
		FactProviderFixture:  EvaluationFactProviderFixture,
		FactProviderVersion:  EvaluationFactProviderVersion,
		Seed:                 42,
		Binary:               evaluation.BinaryIdentity{Version: build.Version, Commit: build.Commit},
		Variant:              "production-deterministic",
	}
}

func writeEvaluationJSON(t *testing.T, path string, value interface{}) {
	t.Helper()
	contents, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(contents, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
