package resolver

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

type memoryCatalog struct {
	skills   []Skill
	snapshot string
}

func (catalog memoryCatalog) Snapshot() string { return catalog.snapshot }
func (catalog memoryCatalog) Skills(context.Context) ([]Skill, error) {
	return append([]Skill(nil), catalog.skills...), nil
}
func (catalog memoryCatalog) Search(_ context.Context, query string, limit int) ([]SearchHit, error) {
	queryTokens := tokenize(query)
	var hits []SearchHit
	for _, skill := range catalog.skills {
		rank := overlap(queryTokens, tokenize(skill.Name+" "+skill.Description+" "+join(skill.Triggers)))
		if rank > 0 {
			hits = append(hits, SearchHit{SkillID: skill.ID, Rank: rank})
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
func join(values []string) string {
	result := ""
	for _, value := range values {
		result += " " + value
	}
	return result
}

func TestNormalizeRequestRejectsUnsafeInputWithoutTaxonomyRequirement(t *testing.T) {
	request := Request{SchemaVersion: "1", RequestID: "req-1", Task: Task{Description: "  Review   retries  "}, Operation: "review", Context: RequestContext{ActiveArtifact: &Artifact{PathHint: "consumer/retry.go"}}}
	normalized, err := NormalizeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Task.Description != "Review retries" {
		t.Fatalf("description=%q", normalized.Task.Description)
	}
	request.Context.ActiveArtifact.PathHint = "../../secret"
	if _, err := NormalizeRequest(request); err == nil {
		t.Fatal("path traversal accepted")
	}
	request.Context.ActiveArtifact.PathHint = `C:\\Users\\secret.txt`
	if _, err := NormalizeRequest(request); err == nil {
		t.Fatal("Windows absolute path accepted")
	}
	request.Context.ActiveArtifact.PathHint = "consumer.go"
	request.Context.Execution.Capabilities = []string{"run-tests"}
	request.Context.Execution.UnavailableCapabilities = []string{"run-tests"}
	if _, err := NormalizeRequest(request); err == nil {
		t.Fatal("contradictory capabilities accepted")
	}
}

func TestRequirementAndExclusionEvidenceUsesTriState(t *testing.T) {
	requirements := Requirements{FactsAll: []Requirement{{Key: "dependency", Value: "kafka"}}, CapabilitiesAll: []string{"run-tests"}}
	request := Request{Context: RequestContext{Facts: []Fact{{Key: "dependency", Value: "kafka", Basis: "tool"}}, Execution: Execution{Capabilities: []string{"run-tests"}}}}
	if state := evaluateRequirements(requirements, request).State; state != Satisfied {
		t.Fatalf("state=%s", state)
	}
	request.Context.Facts = nil
	if state := evaluateRequirements(requirements, request).State; state != Unknown {
		t.Fatalf("unknown fact became %s", state)
	}
	request.Context.Facts = []Fact{{Key: "dependency", Value: "rabbitmq", Basis: "tool"}}
	if state := evaluateRequirements(requirements, request).State; state != Violated {
		t.Fatalf("contradiction became %s", state)
	}
	request.Context.Facts = []Fact{{Key: "dependency", Value: "rabbitmq", Basis: "agent-inference"}}
	if state := evaluateRequirements(requirements, request).State; state != Unknown {
		t.Fatalf("inference hard-filtered as %s", state)
	}
}

func TestResolverHardCompatibilityClarificationAndNoActivation(t *testing.T) {
	skill := Skill{ID: "consumer-review", CollectionID: "core", Name: "Consumer Review", Description: "Review Kafka retries", Status: "active", Digest: "sha256:" + repeat("a", 64), Operations: []string{"review"}, Triggers: []string{"review kafka consumer retries"}, NotFor: []string{"design broker topology"}, MinScope: "multi_step", Requirements: Requirements{CapabilitiesAll: []string{"read-files"}}, Reviewed: true}
	engine, _ := New(memoryCatalog{[]Skill{skill}, "sha256:" + repeat("b", 64)}, DefaultPolicy(), NewCache(8))
	request := Request{SchemaVersion: "1", RequestID: "req", Task: Task{Description: "review kafka consumer retries", Scope: "multi_step"}, Operation: "review"}
	response, err := engine.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != StatusNeedsContext || response.Question == nil {
		t.Fatalf("response=%#v", response)
	}
	request.RequestID = "req-clarified"
	request.Prior = &Prior{ResolutionID: response.ResolutionID, ContextRevision: response.ContextRevision, Kind: "clarification", QuestionID: response.Question.ID, Answer: "unknown"}
	afterUnknown, err := engine.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if afterUnknown.Status != StatusNoSkill || afterUnknown.Question != nil {
		t.Fatalf("clarification loop was not bounded: %#v", afterUnknown)
	}
	request.Prior = nil
	request.RequestID = "req-2"
	request.Context.Execution.UnavailableCapabilities = []string{"read-files"}
	response, err = engine.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != StatusNoSkill || response.NoSkill.ReasonCode != "capability_unavailable" {
		t.Fatalf("response=%#v", response)
	}
	if response.Primary != nil {
		t.Fatal("resolver activated or selected an incompatible primary")
	}
}

func TestClarificationContinuityRejectsForgedStaleAndMismatchedPrior(t *testing.T) {
	skill := Skill{ID: "reader", CollectionID: "core", Name: "Reader", Description: "inspect source files", Status: "active", Digest: "v1", Operations: []string{"review"}, Triggers: []string{"inspect source files"}, MinScope: "multi_step", Requirements: Requirements{CapabilitiesAll: []string{"read-files"}}, Reviewed: true}
	catalog := memoryCatalog{[]Skill{skill}, "sha256:" + repeat("1", 64)}
	engine, _ := New(catalog, DefaultPolicy(), NewCache(16))
	base := Request{SchemaVersion: "1", RequestID: "continuity", Task: Task{Description: "inspect source files", Scope: "multi_step"}, Operation: "review"}
	issued, err := engine.Resolve(context.Background(), base)
	if err != nil || issued.Status != StatusNeedsContext {
		t.Fatalf("issued=%#v err=%v", issued, err)
	}
	for _, answer := range issued.Question.Choices {
		request := base
		request.RequestID = "answer-" + answer
		request.Prior = &Prior{ResolutionID: issued.ResolutionID, ContextRevision: issued.ContextRevision, Kind: "clarification", QuestionID: issued.Question.ID, Answer: answer}
		response, err := engine.Resolve(context.Background(), request)
		if err != nil {
			t.Fatalf("answer %s: %v", answer, err)
		}
		if answer == "available" && response.Status != StatusResolved {
			t.Fatalf("available=%#v", response)
		}
		if answer != "available" && response.Status != StatusNoSkill {
			t.Fatalf("%s=%#v", answer, response)
		}
	}
	for name, mutate := range map[string]func(*Request){
		"forged":   func(request *Request) { request.Prior.ResolutionID = "res_forged" },
		"stale":    func(request *Request) { request.Prior.ContextRevision++ },
		"question": func(request *Request) { request.Prior.QuestionID = "other" },
		"request":  func(request *Request) { request.Task.Description = "different task" },
		"choice":   func(request *Request) { request.Prior.Answer = "invented" },
	} {
		t.Run(name, func(t *testing.T) {
			request := base
			request.Prior = &Prior{ResolutionID: issued.ResolutionID, ContextRevision: issued.ContextRevision, Kind: "clarification", QuestionID: issued.Question.ID, Answer: "available"}
			mutate(&request)
			if _, err := engine.Resolve(context.Background(), request); err == nil {
				t.Fatal("invalid prior accepted")
			}
		})
	}
}

func TestUnknownScopeNeverSatisfiesMinimumScope(t *testing.T) {
	skill := Skill{ID: "project-work", CollectionID: "core", Name: "Project Work", Description: "coordinate project work", Status: "active", Digest: "v1", Operations: []string{"implement"}, Triggers: []string{"coordinate project work"}, MinScope: "project", Reviewed: true}
	engine, _ := New(memoryCatalog{[]Skill{skill}, "sha256:" + repeat("2", 64)}, DefaultPolicy(), NewCache(8))
	request := Request{SchemaVersion: "1", RequestID: "scope", Task: Task{Description: "coordinate project work"}, Operation: "implement"}
	issued, err := engine.Resolve(context.Background(), request)
	if err != nil || issued.Status != StatusNeedsContext || issued.Question.Field != "task.scope" {
		t.Fatalf("issued=%#v err=%v", issued, err)
	}
	decisions := map[Status]bool{}
	for _, answer := range issued.Question.Choices {
		answered := request
		answered.RequestID = "scope-" + answer
		answered.Prior = &Prior{ResolutionID: issued.ResolutionID, ContextRevision: issued.ContextRevision, Kind: "clarification", QuestionID: issued.Question.ID, Answer: answer}
		response, err := engine.Resolve(context.Background(), answered)
		if err != nil {
			t.Fatalf("scope %s: %v", answer, err)
		}
		decisions[response.Status] = true
		if answer == "project" && response.Status != StatusResolved {
			t.Fatalf("project scope=%#v", response)
		}
		if answer == "unknown" && (response.Status != StatusNoSkill || response.NoSkill.ReasonCode != "missing_scope") {
			t.Fatalf("unknown scope=%#v", response)
		}
	}
	if len(decisions) < 2 {
		t.Fatal("scope question does not change the decision")
	}
}

func TestStructuredAndCompoundNaturalExclusionsAreHardAndClauseSafe(t *testing.T) {
	skill := Skill{ID: "code-review", CollectionID: "core", Name: "Code Review", Description: "review code changes", Status: "active", Digest: "v1", Operations: []string{"review"}, Triggers: []string{"review code changes"}, MinScope: "multi_step", Reviewed: true}
	engine, _ := New(memoryCatalog{[]Skill{skill}, "sha256:" + repeat("3", 64)}, DefaultPolicy(), NewCache(8))
	base := Request{SchemaVersion: "1", RequestID: "exclude", Task: Task{Description: "review code changes", Scope: "multi_step"}, Operation: "review"}
	structured := base
	structured.Task.Exclusions.SkillIDs = []string{"code-review"}
	response, _ := engine.Resolve(context.Background(), structured)
	if response.Status != StatusNoSkill || response.NoSkill.ReasonCode != "constraint_conflict" {
		t.Fatalf("structured=%#v", response)
	}
	natural := base
	natural.RequestID = "natural"
	natural.Task.Constraints = []string{"read-only; do not use Code Review"}
	response, _ = engine.Resolve(context.Background(), natural)
	if response.Status != StatusNoSkill {
		t.Fatalf("compound exclusion=%#v", response)
	}
	safe := base
	safe.RequestID = "safe"
	safe.Task.Constraints = []string{"read-only; preserve existing behavior"}
	response, _ = engine.Resolve(context.Background(), safe)
	if response.Status != StatusResolved {
		t.Fatalf("unrelated words became exclusion: %#v", response)
	}
}

func TestCuratedEquivalencePreferenceOverridesLexicalIDOrder(t *testing.T) {
	first := Skill{ID: "aaa-legacy", CollectionID: "core", Name: "Review", Description: "review changes", Status: "active", Digest: "v1", Operations: []string{"review"}, Triggers: []string{"review changes"}, MinScope: "multi_step", Reviewed: true, Equivalence: []EquivalenceRelation{{SkillID: "zzz-curated", Preference: "target", VersionPolicy: "latest-reviewed"}}}
	second := first
	second.ID, second.Name, second.Digest, second.Equivalence = "zzz-curated", "Review Curated", "v2", []EquivalenceRelation{{SkillID: "aaa-legacy", Preference: "self", VersionPolicy: "latest-reviewed"}}
	engine, _ := New(memoryCatalog{[]Skill{first, second}, "sha256:" + repeat("4", 64)}, DefaultPolicy(), NewCache(8))
	response, err := engine.Resolve(context.Background(), Request{SchemaVersion: "1", RequestID: "equivalent", Task: Task{Description: "review changes", Scope: "multi_step"}, Operation: "review"})
	if err != nil || response.Status != StatusResolved || response.Primary.ID != "zzz-curated" {
		t.Fatalf("response=%#v err=%v", response, err)
	}
	first.Equivalence[0].VersionPolicy = "exact"
	second.Equivalence[0].VersionPolicy = "exact"
	engine, _ = New(memoryCatalog{[]Skill{first, second}, "sha256:" + repeat("5", 64)}, DefaultPolicy(), NewCache(8))
	response, err = engine.Resolve(context.Background(), Request{SchemaVersion: "1", RequestID: "exact", Task: Task{Description: "review changes", Scope: "multi_step"}, Operation: "review"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status == StatusResolved && response.Primary.ID == "zzz-curated" {
		t.Fatal("mismatched exact versions were canonicalized")
	}
}

func TestSemanticAndLLMExtensionsRemainDisabled(t *testing.T) {
	if _, err := (DisabledSemanticRetriever{}).Search(context.Background(), "query", "snapshot", 1); err != ErrExtensionDisabled {
		t.Fatalf("semantic extension error=%v", err)
	}
	if _, err := (DisabledLLMReranker{}).Rerank(context.Background(), Request{}, nil); err != ErrExtensionDisabled {
		t.Fatalf("LLM extension error=%v", err)
	}
	policy := DefaultPolicy()
	policy.VectorEnabled = true
	if err := policy.Validate(); err == nil {
		t.Fatal("enabled vector policy accepted")
	}
}

func TestCacheSeparatesFactAndActivationSnapshotsWhilePreservingCorrelation(t *testing.T) {
	skill := Skill{ID: "review", CollectionID: "core", Name: "Review", Description: "review kafka retries", Status: "active", Digest: "v1", Triggers: []string{"review kafka retries"}, NotFor: []string{"broker design"}, MinScope: "multi_step", Requirements: Requirements{FactsAll: []Requirement{{Key: "dependency", Value: "kafka"}}}, Reviewed: true}
	engine, _ := New(memoryCatalog{[]Skill{skill}, "sha256:" + repeat("e", 64)}, DefaultPolicy(), NewCache(8))
	request := Request{SchemaVersion: "1", RequestID: "first", Task: Task{Description: "review kafka retries", Scope: "multi_step"}, Context: RequestContext{Facts: []Fact{{Key: "dependency", Value: "kafka", Basis: "tool", Scope: "active-component"}}}}
	first, err := engine.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.RequestID = "second"
	cached, err := engine.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if cached.RequestID != "second" || cached.ResolutionID != first.ResolutionID {
		t.Fatalf("cached correlation=%#v first=%#v", cached, first)
	}
	request.Context.Facts[0].Value = "rabbitmq"
	changed, err := engine.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Status != StatusNoSkill || changed.ResolutionID == first.ResolutionID {
		t.Fatalf("fact snapshot did not change cache identity: %#v", changed)
	}
	request.Context.Facts[0].Value = "kafka"
	request.ActivationContext = &ActivationContext{Mode: "supplement-only"}
	blocked, err := engine.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status != StatusNoSkill || blocked.ResolutionID == first.ResolutionID {
		t.Fatalf("activation snapshot did not change cache identity: %#v", blocked)
	}
}

func TestNormalizationMakesUnorderedEvidenceAndCoverageInvariant(t *testing.T) {
	first := Request{SchemaVersion: "1", RequestID: "order", Task: Task{Description: "review code", Constraints: []string{"z", "a"}, Exclusions: Exclusions{SkillIDs: []string{"z", "a"}}, Scope: "multi_step"}, Context: RequestContext{Facts: []Fact{{Key: "b", Value: "2", Basis: "tool"}, {Key: "a", Value: "1", Basis: "user"}}, Execution: Execution{Capabilities: []string{"write", "read"}}}, ActivationContext: &ActivationContext{Mode: "allow-primary", ActiveProcedures: []ActiveProcedure{{ID: "z", Role: "supporting", Scope: "review code", StateBasis: "host-native"}, {ID: "a", Role: "primary", Scope: "review code", Summary: "review code", StateBasis: "host-native"}}}}
	second := first
	second.RequestID = "order-2"
	second.Task.Constraints = []string{"a", "z"}
	second.Task.Exclusions.SkillIDs = []string{"a", "z"}
	second.Context.Facts = []Fact{first.Context.Facts[1], first.Context.Facts[0]}
	second.Context.Execution.Capabilities = []string{"read", "write"}
	second.ActivationContext.ActiveProcedures = []ActiveProcedure{first.ActivationContext.ActiveProcedures[1], first.ActivationContext.ActiveProcedures[0]}
	normalizedFirst, err := NormalizeRequest(first)
	if err != nil {
		t.Fatal(err)
	}
	normalizedSecond, err := NormalizeRequest(second)
	if err != nil {
		t.Fatal(err)
	}
	normalizedFirst.RequestID, normalizedSecond.RequestID = "", ""
	if fingerprint(normalizedFirst) != fingerprint(normalizedSecond) || activeCoverage(normalizedFirst).ID != activeCoverage(normalizedSecond).ID {
		t.Fatal("unordered semantic inputs changed fingerprint or coverage")
	}
}

func TestDeterministicConcurrentResolutionAndCandidateOrder(t *testing.T) {
	skills := []Skill{{ID: "beta", CollectionID: "core", Name: "Beta", Description: "review beta code", Status: "active", Digest: "b", Triggers: []string{"review beta code"}, NotFor: []string{"write prose"}, MinScope: "multi_step", Reviewed: true}, {ID: "alpha", CollectionID: "core", Name: "Alpha", Description: "review alpha code", Status: "active", Digest: "a", Triggers: []string{"review alpha code"}, NotFor: []string{"write prose"}, MinScope: "multi_step", Reviewed: true}}
	engine, _ := New(memoryCatalog{skills, "sha256:" + repeat("c", 64)}, DefaultPolicy(), NewCache(32))
	request := Request{SchemaVersion: "1", RequestID: "req-deterministic", Task: Task{Description: "review alpha code", Scope: "multi_step"}, Operation: "review"}
	const workers = 64
	results := make([]Response, workers)
	errs := make([]error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(i int) { defer wait.Done(); results[i], errs[i] = engine.Resolve(context.Background(), request) }(index)
	}
	wait.Wait()
	for index := range results {
		if errs[index] != nil {
			t.Fatal(errs[index])
		}
		if !reflect.DeepEqual(results[0], results[index]) {
			t.Fatalf("result %d differs", index)
		}
	}
	reversed := []Skill{skills[1], skills[0]}
	other, _ := New(memoryCatalog{reversed, "sha256:" + repeat("c", 64)}, DefaultPolicy(), NewCache(4))
	response, err := other.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(results[0], response) {
		t.Fatal("candidate input order changed resolution")
	}
}

type goldenCorpus struct {
	SchemaVersion int          `json:"schema_version"`
	ID            string       `json:"id"`
	Sanitization  string       `json:"sanitization"`
	Skills        []Skill      `json:"skills"`
	Cases         []goldenCase `json:"cases"`
}
type evaluationPolicy struct {
	CalibrationGrid struct {
		ApplicabilityFloor []float64 `json:"applicability_floor"`
		MinimumMargin      []float64 `json:"minimum_margin"`
	} `json:"calibration_grid"`
	Selected struct {
		ApplicabilityFloor float64 `json:"applicability_floor"`
		MinimumMargin      float64 `json:"minimum_margin"`
	} `json:"selected"`
	HeldOutGates struct {
		ResolvedPrecision float64 `json:"resolved_precision_min"`
		NoSkillRecall     float64 `json:"no_skill_abstention_recall_min"`
		AmbiguityRecall   float64 `json:"ambiguity_recall_min"`
		OverallAccuracy   float64 `json:"overall_accuracy_min"`
	} `json:"held_out_gates"`
}

type goldenCase struct {
	ID       string   `json:"id"`
	Split    string   `json:"split"`
	Tags     []string `json:"tags"`
	Request  Request  `json:"request"`
	Expected struct {
		Status            Status   `json:"status"`
		AcceptablePrimary []string `json:"acceptable_primary"`
		Supporting        []string `json:"supporting"`
	} `json:"expected"`
	Provenance map[string]any `json:"provenance"`
}
type metrics struct{ total, correct, predictedResolved, correctResolved, expectedNoSkill, correctNoSkill, expectedAmbiguity, correctAmbiguity int }

func (m metrics) precision() float64 {
	if m.predictedResolved == 0 {
		return 0
	}
	return float64(m.correctResolved) / float64(m.predictedResolved)
}
func (m metrics) abstentionRecall() float64 {
	if m.expectedNoSkill == 0 {
		return 0
	}
	return float64(m.correctNoSkill) / float64(m.expectedNoSkill)
}
func (m metrics) ambiguityRecall() float64 {
	if m.expectedAmbiguity == 0 {
		return 0
	}
	return float64(m.correctAmbiguity) / float64(m.expectedAmbiguity)
}

func TestGoldenCorpusCalibratesOnlyOnTrainingAndMeetsHeldOutGates(t *testing.T) {
	corpus := loadGolden(t)
	evaluation := loadEvaluationPolicy(t)
	if len(corpus.Skills) < 30 || len(corpus.Skills) > 50 || len(corpus.Cases) != 150 {
		t.Fatalf("corpus size skills=%d cases=%d", len(corpus.Skills), len(corpus.Cases))
	}
	var calibration, heldOut []goldenCase
	semanticSplits := map[string]string{}
	groupSplits := map[string]string{}
	heldOutExactTriggers := 0
	heldOutAmbiguity := 0
	triggerSet := map[string]bool{}
	for _, skill := range corpus.Skills {
		for _, trigger := range skill.Triggers {
			triggerSet[normalizeText(trigger)] = true
		}
	}
	for _, test := range corpus.Cases {
		semantic := test.Request
		semantic.RequestID = ""
		semanticKey := fingerprint(semantic)
		if split, exists := semanticSplits[semanticKey]; exists && split != test.Split {
			t.Fatalf("semantic request leakage between %s and %s", split, test.Split)
		}
		semanticSplits[semanticKey] = test.Split
		group, _ := test.Provenance["intent_template_group"].(string)
		if group == "" {
			t.Fatalf("%s has no pre-authored intent/template group", test.ID)
		}
		if split, exists := groupSplits[group]; exists && split != test.Split {
			t.Fatalf("intent/template group %s leaks across splits", group)
		}
		groupSplits[group] = test.Split
		if test.Split == "calibration" {
			calibration = append(calibration, test)
		} else if test.Split == "held_out" {
			heldOut = append(heldOut, test)
			if triggerSet[normalizeText(test.Request.Task.Description)] {
				heldOutExactTriggers++
			}
			if contains(test.Tags, "ambiguity") {
				heldOutAmbiguity++
			}
		} else {
			t.Fatalf("unknown split %q", test.Split)
		}
	}
	if heldOutExactTriggers > 5 {
		t.Fatalf("held-out exact-trigger cases=%d exceeds cap 5", heldOutExactTriggers)
	}
	if heldOutAmbiguity < 3 {
		t.Fatalf("held-out ambiguity scenarios=%d", heldOutAmbiguity)
	}
	policy := calibrate(t, corpus.Skills, calibration, evaluation.CalibrationGrid.ApplicabilityFloor, evaluation.CalibrationGrid.MinimumMargin)
	baseline := DefaultPolicy()
	if policy.ApplicabilityFloor != evaluation.Selected.ApplicabilityFloor || policy.MinimumMargin != evaluation.Selected.MinimumMargin {
		t.Fatalf("calibration selection changed: got %.2f/%.2f want %.2f/%.2f", policy.ApplicabilityFloor, policy.MinimumMargin, evaluation.Selected.ApplicabilityFloor, evaluation.Selected.MinimumMargin)
	}
	if policy.ApplicabilityFloor != baseline.ApplicabilityFloor || policy.MinimumMargin != baseline.MinimumMargin {
		t.Fatalf("production thresholds are not calibration-derived: calibrated floor/margin %.2f/%.2f, production %.2f/%.2f", policy.ApplicabilityFloor, policy.MinimumMargin, baseline.ApplicabilityFloor, baseline.MinimumMargin)
	}
	paraphrases := 0
	byID := map[string]Skill{}
	for _, skill := range corpus.Skills {
		byID[skill.ID] = skill
	}
	for _, test := range heldOut {
		if len(test.Expected.AcceptablePrimary) == 0 {
			continue
		}
		skill := byID[test.Expected.AcceptablePrimary[0]]
		description := normalizeText(test.Request.Task.Description)
		if description != normalizeText(skill.Description) && !contains(skill.Triggers, description) {
			paraphrases++
		}
	}
	if paraphrases < 20 {
		t.Fatalf("held-out set has only %d non-tautological resolved paraphrases", paraphrases)
	}
	held := evaluate(t, corpus.Skills, heldOut, policy, true)
	t.Logf("held-out n=%d accuracy=%.3f precision=%.3f no-skill-recall=%.3f ambiguity-recall=%.3f floor=%.2f margin=%.2f", held.total, float64(held.correct)/float64(held.total), held.precision(), held.abstentionRecall(), held.ambiguityRecall(), policy.ApplicabilityFloor, policy.MinimumMargin)
	if held.precision() < evaluation.HeldOutGates.ResolvedPrecision {
		t.Errorf("held-out resolved precision %.3f < %.3f", held.precision(), evaluation.HeldOutGates.ResolvedPrecision)
	}
	if held.abstentionRecall() < evaluation.HeldOutGates.NoSkillRecall {
		t.Errorf("held-out no-skill abstention recall %.3f < %.3f", held.abstentionRecall(), evaluation.HeldOutGates.NoSkillRecall)
	}
	if held.ambiguityRecall() < evaluation.HeldOutGates.AmbiguityRecall {
		t.Errorf("held-out ambiguity recall %.3f < %.3f", held.ambiguityRecall(), evaluation.HeldOutGates.AmbiguityRecall)
	}
	if accuracy := float64(held.correct) / float64(held.total); accuracy < evaluation.HeldOutGates.OverallAccuracy {
		t.Errorf("held-out overall accuracy %.3f < %.3f", accuracy, evaluation.HeldOutGates.OverallAccuracy)
	}
}

func TestGoldenDistractorGrowthPreservesDominanceAndMetrics(t *testing.T) {
	corpus := loadGolden(t)
	var heldOut []goldenCase
	for _, test := range corpus.Cases {
		if test.Split == "held_out" {
			heldOut = append(heldOut, test)
		}
	}
	policy := DefaultPolicy()
	baseline := evaluate(t, corpus.Skills, heldOut, policy, false)
	for _, multiplier := range []int{2, 5} {
		expanded := append([]Skill(nil), corpus.Skills...)
		for index := len(expanded); index < len(corpus.Skills)*multiplier; index++ {
			topics := []string{"satellite orbit telemetry", "museum collection preservation", "semiconductor fabrication yield", "marine habitat surveys", "legal archive retention"}
			topic := topics[index%len(topics)]
			expanded = append(expanded, Skill{ID: fmt.Sprintf("distractor-%03d", index), CollectionID: "growth", Name: "Specialist " + topic, Description: "Analyze " + topic + " with domain-specific evidence", Status: "active", Digest: fmt.Sprintf("growth-%d", index), Operations: []string{"research"}, Triggers: []string{"analyze " + topic}, NotFor: []string{"software delivery workflows"}, MinScope: "multi_step", Reviewed: true})
		}
		grown := evaluate(t, expanded, heldOut, policy, false)
		accuracyDelta := float64(baseline.correct)/float64(baseline.total) - float64(grown.correct)/float64(grown.total)
		precisionDelta := baseline.precision() - grown.precision()
		t.Logf("distractor x%d accuracy_delta=%.3f precision_delta=%.3f", multiplier, accuracyDelta, precisionDelta)
		if accuracyDelta > .02 || precisionDelta > .02 || grown.correctResolved < baseline.correctResolved {
			t.Fatalf("distractor x%d displaced dominant recommendations: baseline=%#v grown=%#v", multiplier, baseline, grown)
		}
	}
}

func calibrate(t *testing.T, skills []Skill, cases []goldenCase, floors, margins []float64) Policy {
	t.Helper()
	best := DefaultPolicy()
	bestScore := -1.0
	for _, floor := range floors {
		for _, margin := range margins {
			candidate := DefaultPolicy()
			candidate.ApplicabilityFloor = floor
			candidate.MinimumMargin = margin
			candidate.AmbiguityWindow = margin
			sum := sha256.Sum256([]byte(candidate.Revision + string(rune(int(floor*100))) + string(rune(int(margin*100)))))
			candidate.Revision = "sha256:" + hex.EncodeToString(sum[:])
			m := evaluate(t, skills, cases, candidate, false)
			score := 10*float64(m.correct)/float64(m.total) + m.precision() + m.abstentionRecall() + m.ambiguityRecall()
			if score > bestScore {
				bestScore = score
				best = candidate
			}
		}
	}
	return best
}

func evaluate(t *testing.T, skills []Skill, cases []goldenCase, policy Policy, verifyContracts bool) metrics {
	t.Helper()
	catalog, closeCatalog := goldenSQLiteCatalog(t, skills)
	defer closeCatalog()
	engine, err := New(catalog, policy, NewCache(len(cases)+1))
	if err != nil {
		t.Fatal(err)
	}
	result := metrics{}
	for _, test := range cases {
		response, err := engine.Resolve(context.Background(), test.Request)
		if err != nil {
			t.Fatalf("%s: %v", test.ID, err)
		}
		if err := validateResponseContract(response); err != nil {
			t.Fatalf("%s: invalid response contract: %v", test.ID, err)
		}
		if verifyContracts && response.Status == StatusNeedsContext {
			verifyQuestionAnswersChangeDecision(t, engine, test.Request, response)
		}
		result.total++
		if response.Status == StatusResolved {
			result.predictedResolved++
		}
		correct := response.Status == test.Expected.Status
		if correct && response.Status == StatusResolved {
			correct = contains(test.Expected.AcceptablePrimary, response.Primary.ID)
			if correct && len(test.Expected.Supporting) > 0 {
				ids := []string{}
				for _, support := range response.Supporting {
					ids = append(ids, support.ID)
				}
				for _, expected := range test.Expected.Supporting {
					correct = correct && contains(ids, expected)
				}
			}
			if correct {
				result.correctResolved++
			}
		}
		if test.Expected.Status == StatusNoSkill {
			result.expectedNoSkill++
			if response.Status == StatusNoSkill {
				result.correctNoSkill++
			}
		}
		if test.Expected.Status == StatusNeedsContext {
			result.expectedAmbiguity++
			if response.Status == StatusNeedsContext {
				result.correctAmbiguity++
			}
		}
		if correct {
			result.correct++
		} else if verifyContracts {
			primary := ""
			if response.Primary != nil {
				primary = response.Primary.ID
			}
			t.Logf("miss %s expected=%s primary=%v got=%s/%s", test.ID, test.Expected.Status, test.Expected.AcceptablePrimary, response.Status, primary)
		}
	}
	return result
}

func validateResponseContract(response Response) error {
	if response.SchemaVersion != SchemaVersion || response.ResolutionID == "" || response.RequestID == "" || response.ContextRevision < 1 || response.CatalogSnapshot == "" || response.PolicyRevision == "" || response.ValidFor.ScopeFingerprint == "" {
		return fmt.Errorf("missing common fields")
	}
	switch response.Status {
	case StatusResolved:
		if response.Primary == nil || response.Question != nil || response.NoSkill != nil || response.CoveredBy != "" {
			return fmt.Errorf("resolved payload mismatch")
		}
		for _, support := range response.Supporting {
			if !supportRoles[support.Role] || !supportActivations[support.Activation] {
				return fmt.Errorf("invalid supporting metadata")
			}
		}
	case StatusNeedsContext:
		if response.Question == nil || len(response.Question.Choices) < 2 || response.Primary != nil || response.NoSkill != nil {
			return fmt.Errorf("needs-context payload mismatch")
		}
	case StatusNoSkill:
		if response.NoSkill == nil || response.Primary != nil || response.Question != nil {
			return fmt.Errorf("no-skill payload mismatch")
		}
	case StatusAlreadyCovered:
		if response.CoveredBy == "" || response.CoverageBasis == "" || response.Primary != nil || response.Question != nil || response.NoSkill != nil {
			return fmt.Errorf("covered payload mismatch")
		}
	default:
		return fmt.Errorf("unknown status")
	}
	return nil
}

func verifyQuestionAnswersChangeDecision(t *testing.T, engine *Resolver, request Request, issued Response) {
	t.Helper()
	decisions := map[string]bool{}
	for index, answer := range issued.Question.Choices {
		answered := request
		answered.RequestID = fmt.Sprintf("%s-answer-%d", request.RequestID, index)
		answered.Prior = &Prior{ResolutionID: issued.ResolutionID, ContextRevision: issued.ContextRevision, Kind: "clarification", QuestionID: issued.Question.ID, Answer: answer}
		response, err := engine.Resolve(context.Background(), answered)
		if err != nil {
			t.Fatalf("answer %q rejected: %v", answer, err)
		}
		if err := validateResponseContract(response); err != nil {
			t.Fatalf("answer %q response: %v", answer, err)
		}
		primary := ""
		if response.Primary != nil {
			primary = response.Primary.ID
		}
		decisions[string(response.Status)+"/"+primary] = true
	}
	if len(decisions) < 2 {
		t.Fatalf("question %s does not change a decision across choices: %#v", issued.Question.ID, decisions)
	}
}

func goldenSQLiteCatalog(t *testing.T, skills []Skill) (*SQLiteCatalog, func()) {
	t.Helper()
	database, err := sql.Open("sqlite", fmt.Sprintf("file:golden-%p?mode=memory&cache=shared", t))
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	for _, statement := range []string{
		`CREATE TABLE skills(id TEXT PRIMARY KEY,collection_id TEXT,name TEXT,status TEXT,description TEXT,digest TEXT)`,
		`CREATE TABLE canonical_entities(id TEXT PRIMARY KEY,content_json TEXT)`,
		`CREATE VIRTUAL TABLE skill_fts USING fts5(skill_id UNINDEXED,name,aliases,description,triggers)`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for _, skill := range skills {
		document := map[string]any{"aliases": skill.Aliases, "quality": map[string]any{"reviewed": skill.Reviewed}, "routing": map[string]any{
			"operations": skill.Operations, "triggers": skill.Triggers, "not_for": skill.NotFor, "min_scope": skill.MinScope,
			"requirements":     map[string]any{"facts": map[string]any{"all": skill.Requirements.FactsAll, "any": skill.Requirements.FactsAny}, "capabilities": map[string]any{"all": skill.Requirements.CapabilitiesAll, "any": skill.Requirements.CapabilitiesAny}},
			"distinguish_from": goldenDiscriminators(skill.DistinguishFrom), "supporting": goldenSupports(skill.Supporting), "equivalent_to": goldenEquivalence(skill.Equivalence),
		}}
		encoded, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`INSERT INTO skills VALUES(?,?,?,?,?,?)`, skill.ID, skill.CollectionID, skill.Name, skill.Status, skill.Description, skill.Digest); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`INSERT INTO canonical_entities VALUES(?,?)`, skill.ID, string(encoded)); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Exec(`INSERT INTO skill_fts VALUES(?,?,?,?,?)`, skill.ID, skill.Name, join(skill.Aliases), skill.Description, join(skill.Triggers)); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := NewSQLiteCatalog(database, "sha256:"+repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	return catalog, func() { _ = database.Close() }
}

func goldenDiscriminators(values []Discriminator) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		result = append(result, map[string]any{"skill": value.SkillID, "discriminator": map[string]any{"field": value.Field, "question": value.Question, "choices": value.Choices}})
	}
	return result
}
func goldenSupports(values []SupportRelation) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		result = append(result, map[string]any{"skill": value.SkillID, "when": map[string]any{"operation": value.Operation}, "role": value.Role, "activation": value.Activation})
	}
	return result
}
func goldenEquivalence(values []EquivalenceRelation) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		result = append(result, map[string]any{"skill": value.SkillID, "preference": value.Preference, "version_policy": value.VersionPolicy})
	}
	return result
}

func loadEvaluationPolicy(t *testing.T) evaluationPolicy {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "resolver", "evaluation-policy-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var policy evaluationPolicy
	if err := json.Unmarshal(contents, &policy); err != nil {
		t.Fatal(err)
	}
	if len(policy.CalibrationGrid.ApplicabilityFloor) == 0 || len(policy.CalibrationGrid.MinimumMargin) == 0 {
		t.Fatal("evaluation calibration grid is empty")
	}
	return policy
}

func loadGolden(t *testing.T) goldenCorpus {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "resolver", "golden-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus goldenCorpus
	if err := json.Unmarshal(contents, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Sanitization == "" {
		t.Fatal("corpus sanitization declaration missing")
	}
	return corpus
}
func repeat(value string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += value
	}
	return result
}

func TestClarificationPriorIsValidatedWithoutSharedState(t *testing.T) {
	skill := Skill{ID: "reader", CollectionID: "core", Name: "Reader", Description: "inspect source files", Status: "active", Digest: "v1", Operations: []string{"review"}, Triggers: []string{"inspect source files"}, MinScope: "multi_step", Requirements: Requirements{CapabilitiesAll: []string{"read-files"}}, Reviewed: true}
	catalog := memoryCatalog{[]Skill{skill}, "sha256:" + repeat("3", 64)}
	first, _ := New(catalog, DefaultPolicy(), NewCache(4))
	base := Request{SchemaVersion: "1", RequestID: "stateless", Task: Task{Description: "inspect source files", Scope: "multi_step"}, Operation: "review"}
	issued, err := first.Resolve(context.Background(), base)
	if err != nil || issued.Status != StatusNeedsContext {
		t.Fatalf("issued=%#v err=%v", issued, err)
	}
	request := base
	request.Prior = &Prior{ResolutionID: issued.ResolutionID, ContextRevision: issued.ContextRevision, Kind: "clarification", QuestionID: issued.Question.ID, Answer: "available"}
	second, _ := New(catalog, DefaultPolicy(), NewCache(4))
	response, err := second.Resolve(context.Background(), request)
	if err != nil || response.Status != StatusResolved {
		t.Fatalf("independent resolver rejected a valid prior: %#v err=%v", response, err)
	}
	changed, _ := New(memoryCatalog{[]Skill{skill}, "sha256:" + repeat("4", 64)}, DefaultPolicy(), NewCache(4))
	if _, err := changed.Resolve(context.Background(), request); err == nil {
		t.Fatal("prior from a different catalog snapshot was accepted")
	}
}

func TestExampleMatchResolvesSkill(t *testing.T) {
	t.Parallel()
	skill := Skill{
		ID:           "k8s-deployer",
		CollectionID: "core",
		Name:         "K8s Deployer",
		Description:  "Deploys containerized applications",
		Status:       "active",
		Digest:       "v1",
		Triggers:     []string{"unrelated trigger phrase"},
		Examples:     []string{"deploy kubernetes cluster to aws"},
		MinScope:     "single_step",
		Reviewed:     true,
	}
	req := Request{
		SchemaVersion: "1",
		RequestID:     "req-ex-match",
		Task: Task{
			Description: "deploy kubernetes cluster to aws",
			Scope:       "single_step",
		},
	}
	policy := DefaultPolicy()
	scored := scoreSkill(skill, req, policy)
	if scored.Features.Trigger < 0.89 || scored.Features.Trigger > 0.91 {
		t.Fatalf("expected trigger feature ~0.9 from 1.0 * ExampleOverlapWeight, got %f", scored.Features.Trigger)
	}
	foundExample := false
	foundTrigger := false
	for _, r := range scored.Reasons {
		if r == "example_match" {
			foundExample = true
		}
		if r == "trigger_match" {
			foundTrigger = true
		}
	}
	if !foundExample {
		t.Fatalf("expected example_match reason, got reasons: %v", scored.Reasons)
	}
	if foundTrigger {
		t.Fatalf("did not expect trigger_match reason when example won, got reasons: %v", scored.Reasons)
	}

	resolver, err := New(memoryCatalog{[]Skill{skill}, "sha256:" + repeat("1", 64)}, policy, NewCache(4))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := resolver.Resolve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != StatusResolved {
		t.Fatalf("expected StatusResolved, got %s", resp.Status)
	}
	if resp.Primary == nil || resp.Primary.ID != "k8s-deployer" {
		t.Fatalf("expected k8s-deployer as primary, got %#v", resp.Primary)
	}
}

func TestCounterExamplePenalizesOrExcludesSkill(t *testing.T) {
	t.Parallel()
	skill := Skill{
		ID:              "unit-tester",
		CollectionID:    "core",
		Name:            "Unit Tester",
		Description:     "Run unit tests",
		Status:          "active",
		Digest:          "v1",
		Triggers:        []string{"run tests"},
		CounterExamples: []string{"deploy kubernetes cluster"},
		MinScope:        "single_step",
		Reviewed:        true,
	}
	// Near-exact counter-example -> hard exclusion (>= 0.72)
	hardReq := Request{
		SchemaVersion: "1",
		RequestID:     "req-counter-hard",
		Task: Task{
			Description: "deploy kubernetes cluster",
			Scope:       "single_step",
		},
	}
	scoredHard := scoreSkill(skill, hardReq, DefaultPolicy())
	if scoredHard.Exclusion != Violated || scoredHard.HardReason != "not_for_match" {
		t.Fatalf("expected Violated with not_for_match, got %v (%s)", scoredHard.Exclusion, scoredHard.HardReason)
	}

	// Partial counter-example -> soft penalty (>= 0.35, < 0.72)
	softReq := Request{
		SchemaVersion: "1",
		RequestID:     "req-counter-soft",
		Task: Task{
			Description: "deploy something else without kubernetes cluster",
			Scope:       "single_step",
		},
	}
	scoredSoft := scoreSkill(skill, softReq, DefaultPolicy())
	if scoredSoft.Features.NotFor < 0.35 || scoredSoft.Features.NotFor >= 0.72 {
		t.Fatalf("expected soft penalty in [.35, .72), got %f", scoredSoft.Features.NotFor)
	}
	if scoredSoft.Exclusion != Unknown {
		t.Fatalf("expected Unknown exclusion for soft penalty, got %v", scoredSoft.Exclusion)
	}
	foundPenalty := false
	for _, r := range scoredSoft.Reasons {
		if r == "not_for_penalty" {
			foundPenalty = true
		}
	}
	if !foundPenalty {
		t.Fatalf("expected not_for_penalty in reasons: %v", scoredSoft.Reasons)
	}
}

func TestTechnologyMatchLiftsScore(t *testing.T) {
	t.Parallel()
	skill := Skill{
		ID:           "go-linter",
		CollectionID: "core",
		Name:         "Go Linter",
		Description:  "Lints code files",
		Status:       "active",
		Digest:       "v1",
		Triggers:     []string{"lint files"},
		Technologies: []string{"go"},
		MinScope:     "single_step",
		Reviewed:     true,
	}
	baseReq := Request{
		SchemaVersion: "1",
		RequestID:     "req-base",
		Task: Task{
			Description: "lint files",
			Scope:       "single_step",
		},
	}
	policy := DefaultPolicy()
	skillWithoutTech := skill
	skillWithoutTech.Technologies = nil

	// Tech match via ActiveArtifact.Language
	artifactReq := baseReq
	artifactReq.Context.ActiveArtifact = &Artifact{
		Kind:     "file",
		Language: "go",
		PathHint: "main.go",
	}
	scoredWithoutTech := scoreSkill(skillWithoutTech, artifactReq, policy)
	scoredArtifact := scoreSkill(skill, artifactReq, policy)
	if scoredArtifact.Features.Artifact < 1.0 {
		t.Fatalf("expected Artifact feature >= 1.0, got %f", scoredArtifact.Features.Artifact)
	}
	if scoredArtifact.Score <= scoredWithoutTech.Score {
		t.Fatalf("expected score lift from technology match: %f <= %f", scoredArtifact.Score, scoredWithoutTech.Score)
	}
	foundTech := false
	for _, r := range scoredArtifact.Reasons {
		if r == "technology_match" {
			foundTech = true
		}
	}
	if !foundTech {
		t.Fatalf("expected technology_match reason, got %v", scoredArtifact.Reasons)
	}

	// Tech match via Fact with case-insensitive equalEvidence
	factReq := baseReq
	factReq.Context.Facts = []Fact{{Key: "framework", Value: "Go"}}
	scoredBase := scoreSkill(skill, baseReq, policy)
	scoredFact := scoreSkill(skill, factReq, policy)
	if scoredFact.Features.Artifact < 1.0 {
		t.Fatalf("expected Artifact feature >= 1.0, got %f", scoredFact.Features.Artifact)
	}
	if scoredFact.Score <= scoredBase.Score {
		t.Fatalf("expected score lift: %f <= %f", scoredFact.Score, scoredBase.Score)
	}
}

func TestGoldenV1FeatureVectorIdenticalBeforeAndAfter(t *testing.T) {
	t.Parallel()
	corpus := loadGolden(t)
	policy := DefaultPolicy()
	for _, c := range corpus.Cases {
		for _, skill := range corpus.Skills {
			scored := scoreSkill(skill, c.Request, policy)
			// Verify baseline calculations without new features
			query := tokenize(positiveQuery(c.Request))
			metaTokens := tokenize(strings.Join(append([]string{skill.Name, skill.Description}, skill.Aliases...), " "))
			baselineLexical := overlap(query, metaTokens)
			baselineTrigger := bestOverlap(query, skill.Triggers)
			baselineNotFor := bestOverlap(query, skill.NotFor)

			if scored.Features.Lexical != baselineLexical {
				t.Fatalf("case %s, skill %s: Lexical feature changed from %f to %f", c.ID, skill.ID, baselineLexical, scored.Features.Lexical)
			}
			if scored.Features.Trigger != baselineTrigger {
				t.Fatalf("case %s, skill %s: Trigger feature changed from %f to %f", c.ID, skill.ID, baselineTrigger, scored.Features.Trigger)
			}
			if scored.Features.NotFor != baselineNotFor {
				t.Fatalf("case %s, skill %s: NotFor feature changed from %f to %f", c.ID, skill.ID, baselineNotFor, scored.Features.NotFor)
			}
		}
	}
}
