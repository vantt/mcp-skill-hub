package evaluation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/vantt/mcp-skill-hub/internal/resolver"
)

const testDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const policyDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// goldenReportOutputDirEnv is the opt-in artifact boundary used by CI. When it
// is unset, committed-corpus tests perform no writes outside testing.TempDir.
const goldenReportOutputDirEnv = "SKILLHUB_EVALUATION_REPORT_DIR"

type fixtureResolver struct{ variant string }

func (engine fixtureResolver) Resolve(_ context.Context, request resolver.Request) (resolver.Response, error) {
	response := resolver.Response{SchemaVersion: "1", RequestID: request.RequestID, ResolutionID: "res-" + request.RequestID, ContextRevision: 1, CatalogSnapshot: testDigest, PolicyRevision: policyDigest, Supporting: []resolver.Supporting{}, ReasonCodes: []string{}, ValidFor: resolver.ValidFor{ScopeFingerprint: "scope"}}
	if request.Prior != nil {
		response.ContextRevision = 2
		switch request.Prior.Answer {
		case "yes":
			response.Status = resolver.StatusResolved
			response.Primary = &resolver.Recommendation{ID: "alpha"}
		case "no":
			response.Status = resolver.StatusNoSkill
			response.NoSkill = &resolver.NoSkill{ReasonCode: "catalog_gap"}
		default:
			return resolver.Response{}, errors.New("unexpected branch")
		}
		return response, nil
	}
	switch request.RequestID {
	case "multi":
		response.Status = resolver.StatusResolved
		if engine.variant == "candidate" {
			response.Primary = &resolver.Recommendation{ID: "beta"}
		} else {
			response.Primary = &resolver.Recommendation{ID: "wrong"}
		}
	case "none":
		response.Status = resolver.StatusNoSkill
		response.NoSkill = &resolver.NoSkill{ReasonCode: "catalog_gap"}
	case "clarify":
		response.Status = resolver.StatusNeedsContext
		response.Question = &resolver.Question{ID: "fact-kind", Field: "context.facts.kind", Choices: []string{"yes", "no"}}
	case "held":
		response.Status = resolver.StatusResolved
		response.Primary = &resolver.Recommendation{ID: "held-skill"}
	case "supporting":
		response.Status = resolver.StatusResolved
		response.Primary = &resolver.Recommendation{ID: "alpha"}
		response.Supporting = []resolver.Supporting{{ID: "tests"}, {ID: "security"}}
	default:
		return resolver.Response{}, errors.New("unknown request")
	}
	return response, nil
}

type stepClock struct{ current time.Time }

func (clock *stepClock) Now() time.Time {
	value := clock.current
	clock.current = clock.current.Add(time.Millisecond)
	return value
}

type fixtureEvidence struct{}
type evidenceFunc func(context.Context, Case, resolver.Response) (Measurements, error)

func (fn evidenceFunc) Measurements(ctx context.Context, test Case, response resolver.Response) (Measurements, error) {
	return fn(ctx, test, response)
}

func (fixtureEvidence) Measurements(_ context.Context, test Case, _ resolver.Response) (Measurements, error) {
	latency := 1.0
	measurements := Measurements{LatencyMS: &latency}
	if test.ID == "multi" {
		measurements.Counters = map[string]float64{"curation.turns_to_next_action": 2, "invocation.correct_reuse": 1}
	}
	return measurements, nil
}

type corpusCatalog struct {
	skills   []resolver.Skill
	snapshot string
}

func (catalog corpusCatalog) Snapshot() string { return catalog.snapshot }
func (catalog corpusCatalog) Skills(context.Context) ([]resolver.Skill, error) {
	return append([]resolver.Skill(nil), catalog.skills...), nil
}
func (catalog corpusCatalog) Search(_ context.Context, query string, limit int) ([]resolver.SearchHit, error) {
	queryTokens := corpusTokens(query)
	hits := make([]resolver.SearchHit, 0, len(catalog.skills))
	for _, skill := range catalog.skills {
		rank := corpusOverlap(queryTokens, corpusTokens(skill.Name+" "+skill.Description+" "+strings.Join(skill.Triggers, " ")))
		if rank > 0 {
			hits = append(hits, resolver.SearchHit{SkillID: skill.ID, Rank: rank})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Rank != hits[j].Rank {
			return hits[i].Rank > hits[j].Rank
		}
		return hits[i].SkillID < hits[j].SkillID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

var corpusStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "in": true, "into": true,
	"is": true, "it": true, "of": true, "on": true, "or": true, "the": true,
	"this": true, "to": true, "with": true, "without": true, "please": true,
	"current": true, "task": true,
}

func corpusTokens(value string) []string {
	words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '+' || r == '#')
	})
	seen := map[string]bool{}
	result := make([]string, 0, len(words))
	for _, word := range words {
		if len([]rune(word)) < 2 || corpusStopWords[word] || seen[word] {
			continue
		}
		seen[word] = true
		result = append(result, word)
	}
	sort.Strings(result)
	return result
}

func corpusOverlap(left, right []string) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	values := make(map[string]bool, len(left))
	for _, value := range left {
		values[value] = true
	}
	common := 0
	for _, value := range right {
		if values[value] {
			common++
		}
	}
	denominator := len(left)
	if len(right) > denominator {
		denominator = len(right)
	}
	return float64(common) / float64(denominator)
}

func loadGoldenEvaluationSuite(t *testing.T) Suite {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "resolver", "golden-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	suite, err := ParseSuite(contents, FormatJSON)
	if err != nil {
		t.Fatalf("strict golden suite validation: %v", err)
	}
	if len(suite.Cases) != 150 {
		t.Fatalf("golden suite cases=%d, want 150", len(suite.Cases))
	}
	return suite
}

func goldenManifest(t *testing.T, suite Suite, snapshot, policyRevision string) Manifest {
	t.Helper()
	digest, err := DigestSuite(suite)
	if err != nil {
		t.Fatal(err)
	}
	return Manifest{
		SchemaVersion:        SchemaVersion,
		ExperimentID:         "golden-v1-production-resolver",
		SuiteDigest:          digest,
		CatalogSnapshot:      snapshot,
		PolicyRevision:       policyRevision,
		ProtocolSchema:       resolver.SchemaVersion,
		NormalizationVersion: "resolver-normalization-v1",
		IndexVersion:         "in-memory-lexical-v1",
		FactProviderFixture:  "golden-v1",
		FactProviderVersion:  digest,
		Seed:                 260929,
		Binary:               BinaryIdentity{Version: "test", Commit: "committed-corpus"},
		Variant:              "production-default-policy",
	}
}

func runGoldenPartition(t *testing.T, suite Suite, manifest Manifest, policy resolver.Policy, partition Partition) Report {
	t.Helper()
	engine, err := resolver.New(corpusCatalog{skills: suite.Skills, snapshot: manifest.CatalogSnapshot}, policy, resolver.NewCache(len(suite.Cases)*2))
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(engine, StaticPinValidator{Available: manifest})
	if err != nil {
		t.Fatal(err)
	}
	report, err := runner.RunSuite(context.Background(), suite, manifest, RunOptions{
		Partitions:       []Partition{partition},
		Clock:            &stepClock{current: time.Unix(0, 0)},
		BootstrapSamples: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func writeGoldenReportsIfRequested(t *testing.T, calibration, heldOut Report) {
	t.Helper()
	outputDir := os.Getenv(goldenReportOutputDirEnv)
	if outputDir == "" {
		return
	}
	if strings.IndexByte(outputDir, 0) >= 0 {
		t.Fatalf("%s contains a NUL byte", goldenReportOutputDirEnv)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("create %s: %v", goldenReportOutputDirEnv, err)
	}
	for name, report := range map[string]Report{
		"calibration-report.json": calibration,
		"held-out-report.json":    heldOut,
	} {
		contents, err := MarshalReport(report)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(outputDir, name), contents, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func baseCase(id string, partition Partition) Case {
	return Case{SchemaVersion: 1, ID: id, Partition: partition, Request: resolver.Request{SchemaVersion: "1", RequestID: id, Task: resolver.Task{Description: "evaluate " + id}}, Expected: Expected{}}
}
func testSuite() Suite {
	multi := baseCase("multi", PartitionCalibration)
	multi.Tags = []string{"multiple", "routing"}
	multi.Expected = Expected{AcceptableStatuses: []resolver.Status{resolver.StatusResolved}, AcceptablePrimary: []string{"alpha", "beta"}}
	multi.Counters = map[string]float64{"curation.turns_to_next_action": 2, "invocation.correct_reuse": 1}
	none := baseCase("none", PartitionCalibration)
	none.Tags = []string{"no-skill"}
	none.Expected = Expected{AcceptableStatuses: []resolver.Status{resolver.StatusNoSkill}, NoSkill: true}
	clarify := baseCase("clarify", PartitionCalibration)
	clarify.Tags = []string{"clarification"}
	clarify.Expected = Expected{AcceptableStatuses: []resolver.Status{resolver.StatusNeedsContext}, Question: &ExpectedQuestion{Field: "context.facts.kind"}, Branches: map[string]Branch{"yes": {AcceptableStatuses: []resolver.Status{resolver.StatusResolved}, AcceptablePrimary: []string{"alpha"}}, "no": {AcceptableStatuses: []resolver.Status{resolver.StatusNoSkill}, NoSkill: true}}}
	held := baseCase("held", PartitionHeldOut)
	held.Tags = []string{"held"}
	held.Expected = Expected{AcceptableStatuses: []resolver.Status{resolver.StatusResolved}, AcceptablePrimary: []string{"held-skill"}}
	excluded := baseCase("excluded", PartitionCalibration)
	excluded.Exclude = "fixture unavailable"
	excluded.Expected = Expected{AcceptableStatuses: []resolver.Status{resolver.StatusNoSkill}, NoSkill: true}
	return Suite{SchemaVersion: 1, ID: "core", Cases: []Case{multi, none, clarify, held, excluded}}
}
func manifestFor(t *testing.T, suite Suite, variant string) Manifest {
	t.Helper()
	digest, err := DigestSuite(suite)
	if err != nil {
		t.Fatal(err)
	}
	return Manifest{SchemaVersion: 1, ExperimentID: "experiment", SuiteDigest: digest, CatalogSnapshot: testDigest, PolicyRevision: policyDigest, ProtocolSchema: "1", NormalizationVersion: "normalize-v1", IndexVersion: "index-v1", FactProviderFixture: "facts", FactProviderVersion: "v1", Seed: 42, Binary: BinaryIdentity{Version: "1.0.0", Commit: "abc"}, Variant: variant}
}

func TestStrictJSONAndYAMLParsing(t *testing.T) {
	yamlCase := []byte(`schema_version: 1
id: yaml-case
partition: development
request:
  schema_version: "1"
  request_id: yaml
  task:
    description: review code
expected:
  acceptable_statuses: [resolved]
  acceptable_primary: [review-a, review-b]
`)
	parsed, err := ParseCase(yamlCase, FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Expected.AcceptablePrimary) != 2 {
		t.Fatal("multiple acceptable skills were not parsed")
	}
	if _, err := ParseCase(append(yamlCase, []byte("unknown: true\n")...), FormatYAML); err == nil {
		t.Fatal("unknown YAML field accepted")
	}
	if _, err := ParseCase([]byte(`{"schema_version":1,"id":"x","partition":"development","request":{"schema_version":"1","request_id":"x","task":{"description":"x"}},"expected":{"acceptable_statuses":["no_skill"],"no_skill":true},"extra":1}`), FormatJSON); err == nil {
		t.Fatal("unknown JSON field accepted")
	}
	if _, err := ParseCase([]byte("id: x\nid: y\n"), FormatYAML); err == nil {
		t.Fatal("duplicate YAML key accepted")
	}
}

func TestExistingEvaluationPolicyParsesStrictly(t *testing.T) {
	policy, err := LoadCalibrationPolicy(filepath.Join("..", "..", "testdata", "resolver", "evaluation-policy-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if policy.HeldOutSplit != "held_out" || len(policy.CalibrationGrid.ApplicabilityFloor) == 0 {
		t.Fatalf("policy=%#v", policy)
	}
}

func TestRunnerBranchesMetricsPartitionsAndDeterministicReport(t *testing.T) {
	suite := testSuite()
	manifest := manifestFor(t, suite, "candidate")
	pins := StaticPinValidator{Available: manifest}
	runner, err := NewRunner(fixtureResolver{variant: "candidate"}, pins)
	if err != nil {
		t.Fatal(err)
	}
	options := RunOptions{Partitions: []Partition{PartitionCalibration}, Evidence: fixtureEvidence{}, BootstrapSamples: 200}
	first, err := runner.RunSuite(context.Background(), suite, manifest, options)
	if err != nil {
		t.Fatal(err)
	}
	if first.Samples != 3 || len(first.Exclusions) != 1 {
		t.Fatalf("samples=%d exclusions=%v", first.Samples, first.Exclusions)
	}
	if first.Metrics.AcceptableTop1.Value == nil || *first.Metrics.AcceptableTop1.Value != 1 || first.Metrics.NoSkillPrecision.Value == nil || *first.Metrics.NoSkillPrecision.Value != 1 || first.Metrics.NoSkillRecall.Value == nil || *first.Metrics.NoSkillRecall.Value != 1 || first.Metrics.NoSkillFalsePositive.Value == nil || *first.Metrics.NoSkillFalsePositive.Value != 0 || first.Metrics.ClarificationValidity.Value == nil || *first.Metrics.ClarificationValidity.Value != 1 {
		t.Fatalf("metrics=%#v", first.Metrics)
	}
	if first.Metrics.Counters["curation.turns_to_next_action"].Mean != 2 || first.Metrics.Latency.P95MS == nil || *first.Metrics.Latency.P95MS != 1 {
		t.Fatalf("supplied/latency metrics=%#v", first.Metrics)
	}
	if first.Metrics.NoSkillPrecision.CI95 == nil || first.Metrics.NoSkillPrecision.CI95.Samples <= 0 || first.Metrics.NoSkillPrecision.CI95.Samples >= options.BootstrapSamples {
		t.Fatalf("undefined bootstrap replicates were not excluded: %#v", first.Metrics.NoSkillPrecision)
	}
	for _, outcome := range first.Outcomes {
		if outcome.Partition == PartitionHeldOut {
			t.Fatal("held-out case leaked into calibration run")
		}
	}
	secondOptions := options
	second, err := runner.RunSuite(context.Background(), suite, manifest, secondOptions)
	if err != nil {
		t.Fatal(err)
	}
	left, _ := MarshalReport(first)
	right, _ := MarshalReport(second)
	if !reflect.DeepEqual(left, right) {
		t.Fatal("replay report or confidence intervals are nondeterministic")
	}
}

func TestCommittedGoldenCorpusEvaluationRegression(t *testing.T) {
	suite := loadGoldenEvaluationSuite(t)
	policy := resolver.DefaultPolicy()
	manifest := goldenManifest(t, suite, testDigest, policy.Revision)

	calibration := runGoldenPartition(t, suite, manifest, policy, PartitionCalibration)
	heldOut := runGoldenPartition(t, suite, manifest, policy, PartitionHeldOut)
	writeGoldenReportsIfRequested(t, calibration, heldOut)
	if calibration.Samples != 81 || heldOut.Samples != 69 {
		t.Fatalf("partition samples calibration=%d held_out=%d", calibration.Samples, heldOut.Samples)
	}
	if len(calibration.Exclusions) != 0 || len(heldOut.Exclusions) != 0 {
		t.Fatalf("unexpected exclusions calibration=%v held_out=%v", calibration.Exclusions, heldOut.Exclusions)
	}

	calibrationIDs := make(map[string]bool, calibration.Samples)
	for _, outcome := range calibration.Outcomes {
		if outcome.Partition != PartitionCalibration {
			t.Fatalf("non-calibration outcome in calibration report: %#v", outcome)
		}
		calibrationIDs[outcome.CaseID] = true
	}
	for _, outcome := range heldOut.Outcomes {
		if outcome.Partition != PartitionHeldOut {
			t.Fatalf("non-held-out outcome in held-out report: %#v", outcome)
		}
		if calibrationIDs[outcome.CaseID] {
			t.Fatalf("case %q leaked across evaluation partitions", outcome.CaseID)
		}
	}

	if calibration.Metrics.AcceptableTop1.Denominator != 48 || calibration.Metrics.NoSkillRecall.Denominator != 20 || calibration.Metrics.ClarificationValidity.Denominator != 13 {
		t.Fatalf("calibration metric populations=%#v", calibration.Metrics)
	}
	if heldOut.Metrics.AcceptableTop1.Denominator != 46 || heldOut.Metrics.NoSkillRecall.Denominator != 16 || heldOut.Metrics.ClarificationValidity.Denominator != 7 {
		t.Fatalf("held-out metric populations=%#v", heldOut.Metrics)
	}
	if calibration.Metrics.AcceptableTop1.Numerator != 46 || calibration.Metrics.NoSkillPrecision.Numerator != 20 || calibration.Metrics.NoSkillRecall.Numerator != 20 || calibration.Metrics.NoSkillFalsePositive.Numerator != 0 || calibration.Metrics.ClarificationValidity.Numerator != 13 {
		t.Fatalf("calibration regression metrics=%#v", calibration.Metrics)
	}
	if heldOut.Metrics.AcceptableTop1.Numerator != 45 || heldOut.Metrics.NoSkillPrecision.Numerator != 16 || heldOut.Metrics.NoSkillRecall.Numerator != 16 || heldOut.Metrics.NoSkillFalsePositive.Numerator != 0 || heldOut.Metrics.ClarificationValidity.Numerator != 6 {
		t.Fatalf("held-out regression metrics=%#v", heldOut.Metrics)
	}

	evaluationPolicy, err := LoadCalibrationPolicy(filepath.Join("..", "..", "testdata", "resolver", "evaluation-policy-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if policy.ApplicabilityFloor != evaluationPolicy.Selected.ApplicabilityFloor || policy.MinimumMargin != evaluationPolicy.Selected.MinimumMargin {
		t.Fatalf("production policy %.2f/%.2f differs from calibrated selection %.2f/%.2f", policy.ApplicabilityFloor, policy.MinimumMargin, evaluationPolicy.Selected.ApplicabilityFloor, evaluationPolicy.Selected.MinimumMargin)
	}
	resolvedPredictions, correctResolved, correctTotal := 0, 0, 0
	for _, outcome := range heldOut.Outcomes {
		if outcome.Status == resolver.StatusResolved {
			resolvedPredictions++
			if outcome.Correct {
				correctResolved++
			}
		}
		if outcome.Correct {
			correctTotal++
		}
	}
	resolvedPrecision := float64(correctResolved) / float64(resolvedPredictions)
	overallAccuracy := float64(correctTotal) / float64(heldOut.Samples)
	if resolvedPrecision < evaluationPolicy.HeldOutGates.ResolvedPrecision || heldOut.Metrics.NoSkillRecall.Value == nil || *heldOut.Metrics.NoSkillRecall.Value < evaluationPolicy.HeldOutGates.NoSkillRecall || heldOut.Metrics.ClarificationValidity.Value == nil || *heldOut.Metrics.ClarificationValidity.Value < evaluationPolicy.HeldOutGates.AmbiguityRecall || overallAccuracy < evaluationPolicy.HeldOutGates.OverallAccuracy {
		t.Fatalf("held-out gates failed: resolved_precision=%.3f no_skill_recall=%v ambiguity_recall=%v overall_accuracy=%.3f gates=%#v", resolvedPrecision, heldOut.Metrics.NoSkillRecall.Value, heldOut.Metrics.ClarificationValidity.Value, overallAccuracy, evaluationPolicy.HeldOutGates)
	}

	for _, run := range []struct {
		name      string
		partition Partition
		first     Report
	}{{"calibration", PartitionCalibration, calibration}, {"held_out", PartitionHeldOut, heldOut}} {
		t.Run(run.name+"-deterministic-report", func(t *testing.T) {
			replayed := runGoldenPartition(t, suite, manifest, policy, run.partition)
			first, err := MarshalReport(run.first)
			if err != nil {
				t.Fatal(err)
			}
			second, err := MarshalReport(replayed)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("serialized committed-corpus report is nondeterministic")
			}
		})
	}
	t.Logf("calibration metrics=%#v", calibration.Metrics)
	t.Logf("held-out metrics=%#v", heldOut.Metrics)
}

func TestUnavailableAndMismatchedPinsFailClosed(t *testing.T) {
	suite := testSuite()
	manifest := manifestFor(t, suite, "candidate")
	available := manifest
	available.PolicyRevision = testDigest
	runner, _ := NewRunner(fixtureResolver{variant: "candidate"}, StaticPinValidator{Available: available})
	if _, err := runner.RunSuite(context.Background(), suite, manifest, RunOptions{BootstrapSamples: 100}); !errors.Is(err, ErrPinnedArtifactUnavailable) {
		t.Fatalf("missing policy error=%v", err)
	}
	wrong := manifest
	wrong.SuiteDigest = policyDigest
	runner, _ = NewRunner(fixtureResolver{variant: "candidate"}, StaticPinValidator{Available: manifest})
	if _, err := runner.RunSuite(context.Background(), suite, wrong, RunOptions{BootstrapSamples: 100}); !errors.Is(err, ErrPinnedArtifactUnavailable) {
		t.Fatalf("mismatched suite error=%v", err)
	}
}

func TestCaseReplayPinsCaseDigest(t *testing.T) {
	test := baseCase("none", PartitionDevelopment)
	test.Expected = Expected{AcceptableStatuses: []resolver.Status{resolver.StatusNoSkill}, NoSkill: true}
	digest, err := DigestCase(test)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{SchemaVersion: 1, ExperimentID: "single", CaseDigest: digest, CatalogSnapshot: testDigest, PolicyRevision: policyDigest, ProtocolSchema: "1", NormalizationVersion: "n", IndexVersion: "i", FactProviderFixture: "f", FactProviderVersion: "1", Binary: BinaryIdentity{Version: "1", Commit: "c"}, Variant: "v"}
	runner, _ := NewRunner(fixtureResolver{}, StaticPinValidator{Available: manifest})
	report, err := runner.RunCase(context.Background(), test, manifest, RunOptions{Clock: &stepClock{}, BootstrapSamples: 100})
	if err != nil {
		t.Fatal(err)
	}
	if report.Samples != 1 || report.Outcomes[0].Status != resolver.StatusNoSkill {
		t.Fatalf("report=%#v", report)
	}
}

func TestBoundedParsingAndRuntimeValidationParity(t *testing.T) {
	valid := `{"schema_version":1,"id":"x","partition":"development","request":{"schema_version":"1","request_id":"x","task":{"description":"x"}},"expected":{"acceptable_statuses":["resolved"],"acceptable_primary":["alpha"]}}`
	if _, err := ParseCase(append([]byte(valid), make([]byte, MaxDocumentBytes)...), FormatJSON); err == nil {
		t.Fatal("oversized document accepted")
	}
	oversizedPath := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(oversizedPath, make([]byte, MaxDocumentBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCase(oversizedPath); err == nil {
		t.Fatal("oversized file accepted")
	}
	if _, err := ParseCase([]byte(strings.Replace(valid, `"description":"x"`, `"description":"`+strings.Repeat("x", MaxStringRunes+1)+`"`, 1)), FormatJSON); err == nil {
		t.Fatal("oversized string accepted")
	}
	if _, err := ParseCase([]byte(strings.Replace(valid, `"acceptable_statuses":["resolved"]`, `"status":"resolved","acceptable_statuses":["resolved"]`, 1)), FormatJSON); err == nil {
		t.Fatal("conflicting status forms accepted")
	}

	test := baseCase("limits", PartitionDevelopment)
	test.Expected = Expected{AcceptableStatuses: []resolver.Status{resolver.StatusResolved}, AcceptablePrimary: []string{"alpha", "alpha"}}
	if err := validateCase(test); err == nil {
		t.Fatal("duplicate acceptable primary accepted")
	}
	test.Expected = Expected{AcceptableStatuses: []resolver.Status{resolver.StatusNeedsContext}, Question: &ExpectedQuestion{Field: "kind"}, Branches: map[string]Branch{"bad": {AcceptableStatuses: []resolver.Status{"invalid"}}}}
	if err := validateCase(test); err == nil {
		t.Fatal("invalid branch status accepted")
	}
	test.Expected.Branches = map[string]Branch{"bad": {AcceptableStatuses: []resolver.Status{resolver.StatusResolved}, AcceptablePrimary: []string{"alpha"}, NoSkill: true}}
	if err := validateCase(test); err == nil {
		t.Fatal("inconsistent branch no_skill accepted")
	}

	suite := Suite{SchemaVersion: 1, ID: "large", Cases: make([]Case, MaxSuiteCases+1)}
	if err := validateSuite(suite); err == nil {
		t.Fatal("oversized suite accepted")
	}
}

func TestSupportingExpectationsAndObservedMeasurements(t *testing.T) {
	test := baseCase("supporting", PartitionDevelopment)
	test.Expected = Expected{AcceptableStatuses: []resolver.Status{resolver.StatusResolved}, AcceptablePrimary: []string{"alpha"}, Supporting: []string{"security", "tests"}}
	digest, err := DigestCase(test)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{SchemaVersion: 1, ExperimentID: "supporting", CaseDigest: digest, CatalogSnapshot: testDigest, PolicyRevision: policyDigest, ProtocolSchema: "1", NormalizationVersion: "n", IndexVersion: "i", FactProviderFixture: "f", FactProviderVersion: "1", Binary: BinaryIdentity{Version: "1", Commit: "c"}, Variant: "v"}
	runner, _ := NewRunner(fixtureResolver{}, StaticPinValidator{Available: manifest})
	report, err := runner.RunCase(context.Background(), test, manifest, RunOptions{BootstrapSamples: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Outcomes[0].Correct || !reflect.DeepEqual(report.Outcomes[0].Supporting, []string{"security", "tests"}) {
		t.Fatalf("supporting expectation not evaluated: %#v", report.Outcomes[0])
	}
	if report.Metrics.Latency.Samples != 0 || report.Metrics.Latency.MeanMS != nil || len(report.Metrics.Counters) != 0 {
		t.Fatalf("unobserved fixture metrics leaked into report: %#v", report.Metrics)
	}
	if report.Metrics.NoSkillPrecision.Value != nil || report.Metrics.NoSkillPrecision.CI95 != nil {
		t.Fatalf("undefined metric reported as measured: %#v", report.Metrics.NoSkillPrecision)
	}
	serialized, err := MarshalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(serialized), `"value": null`) || !strings.Contains(string(serialized), `"ci95": null`) {
		t.Fatalf("undefined metrics are not explicit in report: %s", serialized)
	}
	if report.Metrics.AcceptableTop1.CI95 == nil || report.Metrics.AcceptableTop1.CI95.Samples != 100 {
		t.Fatalf("bootstrap sample basis missing: %#v", report.Metrics.AcceptableTop1)
	}

	test.Expected.Supporting = []string{"different"}
	if matchesExpected(resolver.Response{Status: resolver.StatusResolved, Primary: &resolver.Recommendation{ID: "alpha"}, Supporting: []resolver.Supporting{{ID: "tests"}, {ID: "security"}}}, test.Expected) {
		t.Fatal("incorrect supporting set accepted")
	}

	test.Expected.Supporting = []string{"security", "tests"}
	negative := -1.0
	_, err = runner.RunCase(context.Background(), test, manifest, RunOptions{BootstrapSamples: 100, Evidence: evidenceFunc(func(context.Context, Case, resolver.Response) (Measurements, error) {
		return Measurements{LatencyMS: &negative}, nil
	})})
	if err == nil || !strings.Contains(err.Error(), "latency_ms") {
		t.Fatalf("invalid observed measurement error=%v", err)
	}
}

func TestFixtureCountersAreAssertionsNotObservedMetrics(t *testing.T) {
	suite := testSuite()
	manifest := manifestFor(t, suite, "candidate")
	runner, _ := NewRunner(fixtureResolver{variant: "candidate"}, StaticPinValidator{Available: manifest})
	report, err := runner.RunSuite(context.Background(), suite, manifest, RunOptions{Partitions: []Partition{PartitionCalibration}, BootstrapSamples: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Metrics.Counters) != 0 {
		t.Fatalf("fixture counters were aggregated: %#v", report.Metrics.Counters)
	}
	for _, outcome := range report.Outcomes {
		if outcome.CaseID == "multi" && (outcome.Correct || outcome.MeasurementAssertionsValid == nil || *outcome.MeasurementAssertionsValid) {
			t.Fatalf("missing observed counter evidence passed fixture assertion: %#v", outcome)
		}
	}
}

func TestPairedVariantComparisonIsDeterministicAndReportsVariance(t *testing.T) {
	suite := testSuite()
	baseManifest := manifestFor(t, suite, "baseline")
	candidateManifest := manifestFor(t, suite, "candidate")
	baseRunner, _ := NewRunner(fixtureResolver{variant: "baseline"}, StaticPinValidator{Available: baseManifest})
	candidateRunner, _ := NewRunner(fixtureResolver{variant: "candidate"}, StaticPinValidator{Available: candidateManifest})
	options := RunOptions{Partitions: []Partition{PartitionCalibration}, Evidence: fixtureEvidence{}, BootstrapSamples: 100}
	baseline, err := baseRunner.RunSuite(context.Background(), suite, baseManifest, options)
	if err != nil {
		t.Fatal(err)
	}
	options.Clock = &stepClock{}
	candidate, err := candidateRunner.RunSuite(context.Background(), suite, candidateManifest, options)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ComparePaired(baseline, candidate, 99, 500)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := ComparePaired(baseline, candidate, 99, 500)
	if !reflect.DeepEqual(first, second) || first.CorrectnessDelta == nil || *first.CorrectnessDelta <= 0 || first.PairedSamples != 3 {
		t.Fatalf("comparison=%#v", first)
	}
	if first.BaselineCorrectnessVariance == nil || *first.BaselineCorrectnessVariance <= 0 || first.CandidateCorrectnessVariance == nil || *first.CandidateCorrectnessVariance != 0 {
		t.Fatalf("variance fields=%#v", first)
	}
}

func TestMultiStatusCasesDoNotSkewNoSkillMetricsOrTopOne(t *testing.T) {
	mixed := Expected{AcceptableStatuses: []resolver.Status{resolver.StatusResolved, resolver.StatusNoSkill}, AcceptablePrimary: []string{"beta"}}
	only := Expected{AcceptableStatuses: []resolver.Status{resolver.StatusNoSkill}}
	want := Expected{AcceptableStatuses: []resolver.Status{resolver.StatusResolved}, AcceptablePrimary: []string{"beta"}}
	records := []record{
		{outcome: Outcome{Status: resolver.StatusResolved, Primary: "beta", Correct: true}, test: Case{Expected: mixed}},
		{outcome: Outcome{Status: resolver.StatusNoSkill, Correct: true}, test: Case{Expected: mixed}},
		{outcome: Outcome{Status: resolver.StatusNoSkill, Correct: true}, test: Case{Expected: only}},
		{outcome: Outcome{Status: resolver.StatusResolved, Primary: "beta", Correct: false}, test: Case{Expected: only}},
		{outcome: Outcome{Status: resolver.StatusNoSkill}, test: Case{Expected: want}},
	}
	m := calculatePointMetrics(records)
	// Top-1 covers: mixed/resolved (hit), want/no_skill (miss); mixed/no_skill is skipped.
	if m.AcceptableTop1.Numerator != 1 || m.AcceptableTop1.Denominator != 2 {
		t.Fatalf("top1 = %d/%d, want 1/2", m.AcceptableTop1.Numerator, m.AcceptableTop1.Denominator)
	}
	// Only the sole-no_skill case is ground truth: 1 recalled of 2 expected, 1 false positive.
	if m.NoSkillRecall.Numerator != 1 || m.NoSkillRecall.Denominator != 2 {
		t.Fatalf("recall = %d/%d, want 1/2", m.NoSkillRecall.Numerator, m.NoSkillRecall.Denominator)
	}
	if m.NoSkillFalsePositive.Numerator != 1 || m.NoSkillFalsePositive.Denominator != 2 {
		t.Fatalf("false positive = %d/%d, want 1/2", m.NoSkillFalsePositive.Numerator, m.NoSkillFalsePositive.Denominator)
	}
}

func TestNormalizationMarksNoSkillOnlyWhenSoleAcceptableStatus(t *testing.T) {
	mixed := Case{Expected: Expected{AcceptableStatuses: []resolver.Status{resolver.StatusResolved, resolver.StatusNoSkill}}}
	normalizeCase(&mixed, 1)
	if mixed.Expected.NoSkill {
		t.Fatal("multi-status case must not be flagged no_skill")
	}
	sole := Case{Expected: Expected{AcceptableStatuses: []resolver.Status{resolver.StatusNoSkill}}}
	normalizeCase(&sole, 1)
	if !sole.Expected.NoSkill {
		t.Fatal("sole no_skill case must be flagged")
	}
}
