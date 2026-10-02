package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	distillpkg "github.com/vantt/mcp-skill-hub/internal/distill"
	insightpkg "github.com/vantt/mcp-skill-hub/internal/insight"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"gopkg.in/yaml.v3"
)

func TestInsightRankingTraversesCompleteComparisonGraphAndCountsUniqueSources(t *testing.T) {
	t.Parallel()
	item := distillpkg.Insight{Priority: "high", ObservationIDs: []string{"OBS-a"}, ComparisonIDs: []string{"CMP-all"}}
	observations := map[string]distillpkg.Observation{
		"OBS-a": {ID: "OBS-a", SourceID: "source-a", Status: "active"},
		"OBS-b": {ID: "OBS-b", SourceID: "source-b", Status: "active"},
		"OBS-c": {ID: "OBS-c", SourceID: "source-b", Status: "active"},
	}
	comparisons := map[string]distillpkg.Comparison{"CMP-all": {ObservationIDs: []string{"OBS-a", "OBS-b", "OBS-c"}}}
	rank := rankInsightEvidence(item, observations, comparisons)
	if rank.Stale || rank.EvidenceSources != 2 || rank.EvidenceFindings != 3 {
		t.Fatalf("complete comparison rank = %#v", rank)
	}
}

func TestApplicationMappingsCoverDirectAndComparisonObservationsWithoutDuplicates(t *testing.T) {
	t.Parallel()
	item := distillpkg.Insight{ObservationIDs: []string{"OBS-direct"}, ComparisonIDs: []string{"CMP-one"}}
	comparisons := map[string]distillpkg.Comparison{"CMP-one": {ObservationIDs: []string{"OBS-direct", "OBS-member"}}}
	targets := []string{"skills/software/example/SKILL.md"}
	mappings, err := validateApplicationMappings(item, comparisons, []ApplicationMapping{
		{ObservationID: "OBS-direct", ArtifactPath: targets[0], Concept: "direct"},
		{ObservationID: "OBS-member", ArtifactPath: targets[0], Concept: "comparison-member"},
	}, targets)
	if err != nil || len(mappings) != 2 {
		t.Fatalf("expanded mappings = %#v, %v", mappings, err)
	}
	_, err = validateApplicationMappings(item, comparisons, []ApplicationMapping{{ObservationID: "OBS-direct", ArtifactPath: targets[0], Concept: "direct"}}, targets)
	if err == nil {
		t.Fatal("comparison-member observation could be omitted")
	}
	_, err = validateApplicationMappings(item, comparisons, []ApplicationMapping{
		{ObservationID: "OBS-direct", ArtifactPath: targets[0], Concept: "direct"},
		{ObservationID: "OBS-member", ArtifactPath: targets[0], Concept: "member"},
		{ObservationID: "OBS-member", ArtifactPath: targets[0], Concept: "member"},
	}, targets)
	if err == nil {
		t.Fatal("duplicate mapping was accepted")
	}
}

func TestInsightApplyIsAtomicTraceableIdempotentAndOutcomeExplicit(t *testing.T) {
	t.Parallel()
	root, distillService, adapter, item := workspaceWithPendingInsight(t)
	service := InsightService{Clock: distillClock{value: time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)}, IDs: fixedSourceID("application000001")}
	inbox, err := service.GetInsightInbox(context.Background(), root)
	if err != nil || inbox.Total != 1 || len(inbox.Groups) != 1 || inbox.Groups[0].Items[0].Rank.EvidenceSources != 1 {
		t.Fatalf("inbox = %#v, %v", inbox, err)
	}
	inboxFixture := loadInsightUXFixture(t, "high-value-insight-review.yaml")
	assertInsightFixtureResult(t, inboxFixture, inbox.Result, ConfirmationPolicy{PolicyRevision: "policy_v1", ActionClass: "read-only", ApplicationCommand: "GetInsightInbox", Confirmation: ConfirmationRequirement{Required: false, Mode: "none"}})
	path := "skills/software/consumer-review/SKILL.md"
	preview, err := service.PreviewInsightApplication(context.Background(), root, item.ID, PreviewInsightInput{
		Changes:        []ApplicationChange{{Path: path, Contents: "# Consumer Review\n\nReview retry failure modes.\n"}},
		Mappings:       []ApplicationMapping{{ObservationID: item.ObservationIDs[0], ArtifactPath: path, Concept: "retry-failure-review"}},
		IdempotencyKey: "apply-lost-response",
	})
	if err != nil || preview.ProposalID == "" || preview.ProposalDigest == "" || !strings.Contains(preview.Diff, "retry failure modes") {
		t.Fatalf("preview = %#v, %v", preview, err)
	}
	if contents, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))); strings.Contains(string(contents), "failure modes") {
		t.Fatal("preview modified active skill")
	}
	applied, err := service.ConfirmInsightApplication(context.Background(), root, preview.ProposalID, preview.ProposalDigest, preview.BaseCatalogVersion)
	if err != nil || applied.Status != StatusApplied || applied.OperationID == "" || applied.IncorporationID == "" || !applied.ActiveLocally {
		t.Fatalf("apply = %#v, %v", applied, err)
	}
	appliedFixture := loadInsightUXFixture(t, "git-dirty-after-apply.yaml")
	assertInsightFixtureResult(t, appliedFixture, applied.Result, applied.Confirmation)
	if !applied.GitDirty || len(applied.Warnings) == 0 || applied.Warnings[0].Code != "git_dirty" || len(applied.Items) != 1 || applied.Items[0].Summary != appliedFixture.Expect.ProgressiveDisclosure.L1.Items[0].Summary || applied.Items[0].Impact != appliedFixture.Expect.ProgressiveDisclosure.L1.Items[0].Impact {
		t.Fatalf("applied UX contract = %#v", applied)
	}
	retried, err := service.ConfirmInsightApplication(context.Background(), root, preview.ProposalID, preview.ProposalDigest, preview.BaseCatalogVersion)
	if err != nil || retried.OperationID != applied.OperationID || retried.IncorporationID != applied.IncorporationID {
		t.Fatalf("lost-response retry = %#v, %v", retried, err)
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "distill/skills/consumer-review/outcomes/*.yaml")); len(matches) != 0 {
		t.Fatal("apply inferred an outcome")
	}
	local, err := service.QueryArtifactProvenance(context.Background(), root, path)
	if err != nil || len(local.Chains) != 1 || len(local.Chains[0].Findings) != 1 || local.Chains[0].Findings[0].SourceRevision.Value != "r2" {
		t.Fatalf("local provenance = %#v, %v", local, err)
	}
	impact, err := service.QueryFindingImpact(context.Background(), root, item.ObservationIDs[0])
	if err != nil || len(impact.AffectedArtifacts) != 1 || impact.AffectedArtifacts[0] != path {
		t.Fatalf("finding impact = %#v, %v", impact, err)
	}
	outcome, err := service.RecordIncorporationOutcome(context.Background(), root, applied.IncorporationID, OutcomeInput{State: "confirmed", Evidence: []string{"review:task-42"}, Note: "Reviewer confirmed the new step prevented a missed retry risk.", IdempotencyKey: "outcome-lost-response"})
	if err != nil || outcome.Outcome.State != "confirmed" {
		t.Fatalf("outcome = %#v, %v", outcome, err)
	}
	outcomeRetry, err := service.RecordIncorporationOutcome(context.Background(), root, applied.IncorporationID, OutcomeInput{State: "confirmed", Evidence: []string{"review:task-42"}, Note: "Reviewer confirmed the new step prevented a missed retry risk.", IdempotencyKey: "outcome-lost-response"})
	if err != nil || outcomeRetry.OperationID != outcome.OperationID {
		t.Fatalf("outcome retry = %#v, %v", outcomeRetry, err)
	}
	operation, err := service.GetOperationDiff(context.Background(), root, applied.OperationID)
	if err != nil || len(operation.Changes) < 4 || !strings.Contains(operation.Warning, "never runs") || !strings.Contains(operation.ReviewCommand, "git diff") || !operation.Changes[0].DiffAvailable || operation.Changes[0].DigestOnlyMetadata {
		t.Fatalf("operation diff = %#v, %v", operation, err)
	}
	for _, guidance := range operation.RestoreGuidance {
		if strings.Contains(guidance, "git restore") || strings.Contains(guidance, "rm --") || strings.Contains(guidance, "git revert") {
			t.Fatalf("operation view emitted an unguarded mutating command: %q", guidance)
		}
	}
	_ = distillService
	_ = adapter
}

func TestInsightStalePathAppliesNothing(t *testing.T) {
	t.Parallel()
	root, _, _, item := workspaceWithPendingInsight(t)
	service := InsightService{Clock: distillClock{value: time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)}, IDs: fixedSourceID("staleproposal001")}
	path := "skills/software/consumer-review/SKILL.md"
	preview, err := service.PreviewInsightApplication(context.Background(), root, item.ID, PreviewInsightInput{Changes: []ApplicationChange{{Path: path, Contents: "# Changed by proposal\n"}}, Mappings: []ApplicationMapping{{ObservationID: item.ObservationIDs[0], ArtifactPath: path, Concept: "retry-review"}}})
	if err != nil {
		t.Fatal(err)
	}
	external := []byte("# External edit after preview\n")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), external, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := service.ConfirmInsightApplication(context.Background(), root, preview.ProposalID, preview.ProposalDigest, preview.BaseCatalogVersion)
	if err != nil || result.Status != StatusError || result.Error == nil || result.Error.Code != ErrorStaleProposal {
		t.Fatalf("stale = %#v, %v", result, err)
	}
	staleFixture := loadInsightUXFixture(t, "stale-proposal.yaml")
	assertInsightFixtureResult(t, staleFixture, result.Result, result.Confirmation)
	if result.Error.Render.Error != staleFixture.Expect.Error.Error || result.Error.Render.Why != staleFixture.Expect.Error.Why || result.Error.Render.Fix != staleFixture.Expect.Error.Fix {
		t.Fatalf("stale error UX = %#v, want %#v", result.Error.Render, staleFixture.Expect.Error)
	}
	contents, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if string(contents) != string(external) {
		t.Fatal("stale confirmation changed target")
	}
	loaded, _, _, err := loadInsightContext(root, item.ID)
	if err != nil || loaded.Status != "pending" {
		t.Fatalf("insight changed on stale apply: %#v %v", loaded, err)
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "distill/skills/consumer-review/incorporations/*.yaml")); len(matches) != 0 {
		t.Fatal("stale apply created incorporation")
	}
}

func TestRejectedInsightRequiresMaterialEvidenceAndExplicitReopen(t *testing.T) {
	t.Parallel()
	root, distillService, adapter, item := workspaceWithPendingInsight(t)
	service := InsightService{Clock: distillClock{value: time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)}, IDs: fixedSourceID("decision0000001")}
	rejected, err := service.DecideInsight(context.Background(), root, item.ID, InsightDecisionInput{Decision: "reject", Rationale: "Existing workflow already covers this evidence."})
	if err != nil || rejected.Insight.Status != "rejected" || rejected.Insight.RejectedEvidenceDigest == "" {
		t.Fatalf("reject = %#v, %v", rejected, err)
	}
	recovered, err := service.DecideInsight(context.Background(), root, item.ID, InsightDecisionInput{Decision: "reject", Rationale: "Existing workflow already covers this evidence."})
	if err != nil || recovered.OperationID != rejected.OperationID {
		t.Fatalf("decision retry = %#v, %v", recovered, err)
	}
	if _, err := service.DecideInsight(context.Background(), root, item.ID, InsightDecisionInput{Decision: "reopen", Rationale: "Try again."}); err == nil {
		t.Fatal("same evidence reopened rejected insight")
	}
	distillService.IDs = fixedSourceID("distillrun000002")
	distillService.Clock = distillClock{value: time.Date(2026, 9, 29, 7, 0, 0, 0, time.UTC)}
	adapter.files["r3"] = map[string][]byte{"SKILL.md": []byte("# New\nretry review with bounded backoff\n")}
	_, records, err := readSourceRecords(root)
	if err != nil {
		t.Fatal(err)
	}
	records[0].CurrentRevision = ptrRevision(revisionForDistill("r3", adapter.files["r3"]))
	records[0].Status = "changed"
	recordBytes, _ := sourcepkg.MarshalCanonical(records[0])
	if err := os.WriteFile(filepath.Join(root, "sources/catalog/source-a.yaml"), recordBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	run := prepareAndStart(t, root, distillService, "source-a")
	submission := DistillSubmission{Coverage: []distillpkg.CoverageEntry{{Resource: "SKILL.md", Status: "analyzed", Reason: "Read changed target."}}, Findings: []FindingSubmission{{StableKey: "retry-review", Status: "active", What: "The source now requires bounded retry backoff.", Vocabulary: []string{"bounded backoff"}, Evidence: []distillpkg.Evidence{evidenceFor(run, "SKILL.md", "SKILL.md", adapter.files["r3"]["SKILL.md"])}}}, Insights: []InsightSubmission{{StableKey: "retry-review", SkillID: "consumer-review", Recommendation: "Add bounded retry review.", ObservationIDs: []string{"OBS-source-a--retry-review"}, Category: "reliability", Priority: "high", Rationale: "The changed source adds a materially new bounded-backoff requirement."}}}
	if _, err := distillService.SubmitDistillRun(context.Background(), root, run.ID, submission); err != nil {
		t.Fatal(err)
	}
	updated, _, _, err := loadInsightContext(root, item.ID)
	if err != nil || updated.Status != "rejected" || updated.EvidenceDigest == updated.RejectedEvidenceDigest {
		t.Fatalf("material evidence did not remain rejected pending explicit reopen: %#v %v", updated, err)
	}
	reopened, err := service.DecideInsight(context.Background(), root, item.ID, InsightDecisionInput{Decision: "reopen", Rationale: "Bounded-backoff evidence materially changes the rejected recommendation."})
	if err != nil || reopened.Insight.Status != "pending" {
		t.Fatalf("reopen = %#v, %v", reopened, err)
	}
}

func TestWorkspaceValidationRejectsDanglingIncorporationEdits(t *testing.T) {
	t.Parallel()
	root, _, _, item := workspaceWithPendingInsight(t)
	service := InsightService{Clock: distillClock{value: time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)}, IDs: fixedSourceID("validation000001")}
	path := "skills/software/consumer-review/SKILL.md"
	preview, err := service.PreviewInsightApplication(context.Background(), root, item.ID, PreviewInsightInput{Changes: []ApplicationChange{{Path: path, Contents: "# Validated proposal\n"}}, Mappings: []ApplicationMapping{{ObservationID: item.ObservationIDs[0], ArtifactPath: path, Concept: "validated"}}})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := service.ConfirmInsightApplication(context.Background(), root, preview.ProposalID, preview.ProposalDigest, preview.BaseCatalogVersion)
	if err != nil {
		t.Fatal(err)
	}
	incorporation, _, err := readIncorporation(root, applied.IncorporationID)
	if err != nil {
		t.Fatal(err)
	}
	incorporation.SourceToLocal[0].ObservationID = "OBS-dangling"
	data, err := insightpkg.Marshal(incorporation)
	if err != nil {
		t.Fatal(err)
	}
	incorporationFile := filepath.Join(root, "distill", "skills", "consumer-review", "incorporations", incorporation.ID+".yaml")
	if err := os.WriteFile(incorporationFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := distillpkg.ValidateWorkspace(root); err == nil || !strings.Contains(err.Error(), "dangling") {
		t.Fatalf("dangling external edit validation error = %v", err)
	}
}

func TestInterruptedInsightApplyLeavesRecoverableLifecycleTransaction(t *testing.T) {
	t.Parallel()
	root, _, _, item := workspaceWithPendingInsight(t)
	service := InsightService{Clock: distillClock{value: time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)}, IDs: fixedSourceID("atomicfailure001")}
	path := "skills/software/consumer-review/SKILL.md"
	preview, err := service.PreviewInsightApplication(context.Background(), root, item.ID, PreviewInsightInput{Changes: []ApplicationChange{{Path: path, Contents: "# Atomic proposal\n"}}, Mappings: []ApplicationMapping{{ObservationID: item.ObservationIDs[0], ArtifactPath: path, Concept: "atomic-review"}}})
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("simulated interruption")
	service.MutationOptions.Fault = func(point mutation.FaultPoint) error {
		if point == mutation.FaultFirstCanonicalReplace {
			return injected
		}
		return nil
	}
	if _, err := service.ConfirmInsightApplication(context.Background(), root, preview.ProposalID, preview.ProposalDigest, preview.BaseCatalogVersion); !errors.Is(err, injected) {
		t.Fatalf("confirm error = %v", err)
	}
	recoveries, err := mutation.InspectRecovery(root)
	if err != nil || len(recoveries) != 1 || recoveries[0].Action != mutation.RecoveryRollForward {
		t.Fatalf("recovery = %#v, %v", recoveries, err)
	}
	if err := mutation.RollForwardWithOptions(root, mutation.Options{PostCanonical: func(expected string) (mutation.Publication, error) {
		built, buildErr := catalog.BuildCatalogGenerationWhileLocked(context.Background(), root, expected, catalog.BuildOptions{})
		if buildErr != nil {
			return mutation.Publication{}, buildErr
		}
		return mutation.Publication{CatalogSnapshot: built.Pointer.CatalogSnapshot, Generation: built.Pointer.Generation}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	loaded, _, _, err := loadInsightContext(root, item.ID)
	if err != nil || loaded.Status != "incorporated" {
		t.Fatalf("recovered insight = %#v, %v", loaded, err)
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "distill/skills/consumer-review/incorporations/*.yaml")); len(matches) != 1 {
		t.Fatalf("recovered incorporations = %v", matches)
	}
	contents, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if string(contents) != "# Atomic proposal\n" {
		t.Fatalf("recovered skill = %q", contents)
	}
}

func workspaceWithPendingInsight(t *testing.T) (string, DistillService, revisionAdapter, distillpkg.Insight) {
	t.Helper()
	root, service, adapter := newDistillWorkspace(t, "source-a", false)
	run := prepareAndStart(t, root, service, "source-a")
	submission := validSubmission(run, adapter)
	submission.Insights = []InsightSubmission{{StableKey: "retry-review", SkillID: "consumer-review", Recommendation: "Add retry failure review.", ObservationIDs: []string{"OBS-source-a--retry-review"}, Category: "reliability", Priority: "high", Rationale: "Pinned evidence adds a missing review step."}}
	if _, err := service.SubmitDistillRun(context.Background(), root, run.ID, submission); err != nil {
		t.Fatal(err)
	}
	items, err := readInsights(root)
	if err != nil || len(items) != 1 {
		t.Fatalf("insights = %#v, %v", items, err)
	}
	return root, service, adapter, items[0]
}

type insightUXFixture struct {
	Expect struct {
		Status           Status   `yaml:"status"`
		Summary          string   `yaml:"summary"`
		SuggestedActions []string `yaml:"suggested_actions"`
		Confirmation     struct {
			PolicyRevision     string `yaml:"policy_revision"`
			ActionClass        string `yaml:"action_class"`
			ApplicationCommand string `yaml:"application_command"`
			Confirmation       struct {
				Required bool   `yaml:"required"`
				Mode     string `yaml:"mode"`
				Pins     struct {
					ProposalID     string `yaml:"proposal_id"`
					ProposalDigest string `yaml:"proposal_digest"`
					BaseVersion    string `yaml:"base_version"`
				} `yaml:"pins"`
			} `yaml:"confirmation"`
		} `yaml:"confirmation"`
		ProgressiveDisclosure struct {
			L1 struct {
				Items []Item `yaml:"items"`
			} `yaml:"L1"`
		} `yaml:"progressive_disclosure"`
		Error *struct {
			Error string `yaml:"ERROR"`
			Why   string `yaml:"WHY"`
			Fix   string `yaml:"FIX"`
		} `yaml:"error"`
	} `yaml:"expect"`
}

func loadInsightUXFixture(t *testing.T, name string) insightUXFixture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ux", "insight-apply", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture insightUXFixture
	if err := yaml.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func assertInsightFixtureResult(t *testing.T, fixture insightUXFixture, result Result, confirmation ConfirmationPolicy) {
	t.Helper()
	if result.Status != fixture.Expect.Status || result.Summary != fixture.Expect.Summary {
		t.Fatalf("fixture status/summary = %q/%q, got %q/%q", fixture.Expect.Status, fixture.Expect.Summary, result.Status, result.Summary)
	}
	if len(fixture.Expect.SuggestedActions) != 1 || len(result.SuggestedActions) != 1 || result.SuggestedActions[0].Label != fixture.Expect.SuggestedActions[0] {
		t.Fatalf("fixture actions = %#v, got %#v", fixture.Expect.SuggestedActions, result.SuggestedActions)
	}
	want := fixture.Expect.Confirmation
	if confirmation.PolicyRevision != want.PolicyRevision || confirmation.ActionClass != want.ActionClass || confirmation.ApplicationCommand != want.ApplicationCommand || confirmation.Confirmation.Required != want.Confirmation.Required || confirmation.Confirmation.Mode != want.Confirmation.Mode {
		t.Fatalf("fixture confirmation = %#v, got %#v", want, confirmation)
	}
	if want.Confirmation.Pins.ProposalID != "" && (confirmation.Confirmation.Pins.ProposalID == "" || confirmation.Confirmation.Pins.ProposalDigest == "" || confirmation.Confirmation.Pins.BaseVersion == "") {
		t.Fatalf("fixture requires exact confirmation pins, got %#v", confirmation.Confirmation.Pins)
	}
}

func ptrRevision(value sourcepkg.Revision) *sourcepkg.Revision { return &value }
