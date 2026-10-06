package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	distillpkg "github.com/vantt/mcp-skill-hub/internal/distill"
	insightpkg "github.com/vantt/mcp-skill-hub/internal/insight"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

// ErrInsightNotFound reports an insight ID with no stored insight.
var ErrInsightNotFound = errors.New("insight not found")

// InsightService owns review, application, provenance, and outcome semantics.
type InsightService struct {
	Clock           Clock
	IDs             IDGenerator
	MutationOptions mutation.Options
}

func (service InsightService) defaults() InsightService {
	if service.Clock == nil {
		service.Clock = SystemClock{}
	}
	if service.IDs == nil {
		service.IDs = RandomIDGenerator{}
	}
	return service
}

type InsightRank struct {
	Score            int    `json:"score"`
	EvidenceSources  int    `json:"evidence_sources"`
	EvidenceFindings int    `json:"evidence_findings"`
	Impact           string `json:"impact"`
	Stale            bool   `json:"stale"`
}
type InsightInboxItem struct {
	Insight distillpkg.Insight `json:"insight"`
	Rank    InsightRank        `json:"rank"`
}
type InsightInboxGroup struct {
	SkillID  string             `json:"skill_id"`
	Category string             `json:"category"`
	Items    []InsightInboxItem `json:"items"`
}
type InsightInboxResult struct {
	Result
	Groups []InsightInboxGroup `json:"groups"`
	Total  int                 `json:"total"`
}

type InsightDetailResult struct {
	Result
	Insight     distillpkg.Insight       `json:"insight"`
	Findings    []distillpkg.Observation `json:"findings"`
	Comparisons []distillpkg.Comparison  `json:"comparisons"`
}

type InsightDecisionInput struct {
	Decision       string `json:"decision"`
	Rationale      string `json:"rationale"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}
type InsightDecisionResult struct {
	Result
	Insight         distillpkg.Insight `json:"insight"`
	OperationID     string             `json:"operation_id"`
	ChangedPaths    []string           `json:"changed_paths"`
	CatalogSnapshot string             `json:"catalog_snapshot"`
	Generation      string             `json:"generation"`
}

type ApplicationChange struct {
	Path     string `json:"path"`
	Contents string `json:"contents"`
}
type ApplicationMapping struct {
	ObservationID string `json:"observation_id"`
	ArtifactPath  string `json:"artifact_path"`
	Concept       string `json:"concept"`
}
type PreviewInsightInput struct {
	Changes        []ApplicationChange  `json:"changes"`
	Mappings       []ApplicationMapping `json:"mappings"`
	IdempotencyKey string               `json:"idempotency_key,omitempty"`
}
type InsightApplicationPreview struct {
	Result
	ProposalID         string               `json:"proposal_id"`
	ProposalDigest     string               `json:"proposal_digest"`
	BaseCatalogVersion string               `json:"base_catalog_version"`
	InsightID          string               `json:"insight_id"`
	SkillID            string               `json:"skill_id"`
	PathPins           []insightpkg.PathPin `json:"path_pins"`
	Diff               string               `json:"diff"`
	Confirmation       ConfirmationPolicy   `json:"confirmation"`
}
type InsightApplicationResult struct {
	Result
	Confirmation    ConfirmationPolicy `json:"confirmation"`
	InsightID       string             `json:"insight_id"`
	IncorporationID string             `json:"incorporation_id"`
	OperationID     string             `json:"operation_id"`
	ChangedPaths    []string           `json:"changed_paths"`
	CatalogSnapshot string             `json:"catalog_snapshot"`
	Generation      string             `json:"generation"`
	ActiveLocally   bool               `json:"active_locally"`
	GitDirty        bool               `json:"git_dirty"`
}
type OutcomeInput struct {
	State          string   `json:"state"`
	Evidence       []string `json:"evidence"`
	Note           string   `json:"note"`
	Supersedes     string   `json:"supersedes,omitempty"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
}
type OutcomeResult struct {
	Result
	Outcome      insightpkg.Outcome `json:"outcome"`
	OperationID  string             `json:"operation_id"`
	ChangedPaths []string           `json:"changed_paths"`
}

type ProvenanceFinding struct {
	Observation    distillpkg.Observation      `json:"observation"`
	SourceRevision distillpkg.RevisionIdentity `json:"source_revision"`
}
type ProvenanceChain struct {
	Insight       distillpkg.Insight       `json:"insight"`
	Incorporation insightpkg.Incorporation `json:"incorporation"`
	Findings      []ProvenanceFinding      `json:"findings"`
}
type ProvenanceResult struct {
	Result
	ArtifactPath      string            `json:"artifact_path,omitempty"`
	ObservationID     string            `json:"observation_id,omitempty"`
	Chains            []ProvenanceChain `json:"chains"`
	AffectedArtifacts []string          `json:"affected_artifacts"`
}
type OperationChange struct {
	Path                string `json:"path"`
	BeforeDigest        string `json:"before_digest"`
	AfterDigest         string `json:"after_digest"`
	Before              string `json:"before,omitempty"`
	After               string `json:"after,omitempty"`
	Diff                string `json:"diff,omitempty"`
	DiffAvailable       bool   `json:"diff_available"`
	DigestOnlyMetadata  bool   `json:"digest_only_metadata"`
	CurrentMatchesAfter bool   `json:"current_matches_after"`
}
type OperationDiffResult struct {
	Result
	OperationID     string            `json:"operation_id"`
	Changes         []OperationChange `json:"changes"`
	ReviewCommand   string            `json:"review_command"`
	RestoreGuidance []string          `json:"restore_guidance"`
	Warning         string            `json:"warning"`
}

func (InsightService) GetInsightInbox(ctx context.Context, path string) (InsightInboxResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return InsightInboxResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return InsightInboxResult{}, err
	}
	insights, err := readInsights(root)
	if err != nil {
		return InsightInboxResult{}, err
	}
	observations, _, err := readObservations(root)
	if err != nil {
		return InsightInboxResult{}, err
	}
	comparisons, _, err := readComparisons(root)
	if err != nil {
		return InsightInboxResult{}, err
	}
	obsByID := map[string]distillpkg.Observation{}
	for _, item := range observations {
		obsByID[item.ID] = item
	}
	comparisonByID := map[string]distillpkg.Comparison{}
	for _, item := range comparisons {
		comparisonByID[item.ID] = item
	}
	groups := map[string]*InsightInboxGroup{}
	for _, item := range insights {
		if item.Status != "pending" && item.Status != "planned" {
			continue
		}
		rank := rankInsightEvidence(item, obsByID, comparisonByID)
		key := item.SkillID + "\x00" + item.Category
		group := groups[key]
		if group == nil {
			group = &InsightInboxGroup{SkillID: item.SkillID, Category: item.Category}
			groups[key] = group
		}
		group.Items = append(group.Items, InsightInboxItem{Insight: item, Rank: rank})
	}
	result := InsightInboxResult{Result: NewResult(StatusOK, "Insight inbox is empty."), Groups: []InsightInboxGroup{}}
	for _, group := range groups {
		sort.Slice(group.Items, func(i, j int) bool {
			if group.Items[i].Rank.Score == group.Items[j].Rank.Score {
				return group.Items[i].Insight.ID < group.Items[j].Insight.ID
			}
			return group.Items[i].Rank.Score > group.Items[j].Rank.Score
		})
		result.Groups = append(result.Groups, *group)
		result.Total += len(group.Items)
	}
	sort.Slice(result.Groups, func(i, j int) bool {
		if result.Groups[i].SkillID == result.Groups[j].SkillID {
			return result.Groups[i].Category < result.Groups[j].Category
		}
		return result.Groups[i].SkillID < result.Groups[j].SkillID
	})
	if result.Total > 0 {
		result.Summary = fmt.Sprintf("%d insight(s) are grouped and ranked for review; none were auto-adopted.", result.Total)
		result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Review the insight recommendation", Command: "GetInsightDetail"})
		if result.Total == 1 && !result.Groups[0].Items[0].Rank.Stale && (result.Groups[0].Items[0].Rank.Impact == "high" || result.Groups[0].Items[0].Rank.Impact == "critical") {
			result.Summary = "One high-value insight is ready for review."
		}
	}
	return result, nil
}

func rankInsightEvidence(item distillpkg.Insight, observations map[string]distillpkg.Observation, comparisons map[string]distillpkg.Comparison) InsightRank {
	sources := map[string]bool{}
	findings := map[string]bool{}
	stale := false
	visit := func(id string) {
		findings[id] = true
		observation, ok := observations[id]
		if !ok || observation.Status != "active" {
			stale = true
			return
		}
		sources[observation.SourceID] = true
	}
	for _, id := range item.ObservationIDs {
		visit(id)
	}
	for _, id := range item.ComparisonIDs {
		comparison, ok := comparisons[id]
		if !ok || comparison.Stale {
			stale = true
			continue
		}
		for _, observationID := range comparison.ObservationIDs {
			visit(observationID)
		}
	}
	weight := map[string]int{"low": 10, "medium": 30, "high": 60, "critical": 90}[item.Priority]
	rank := InsightRank{Score: weight + len(sources)*15 + len(findings)*3, EvidenceSources: len(sources), EvidenceFindings: len(findings), Impact: item.Priority, Stale: stale}
	if stale {
		rank.Score -= 50
	}
	return rank
}

func (InsightService) GetInsightDetail(ctx context.Context, path, id string) (InsightDetailResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return InsightDetailResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return InsightDetailResult{}, err
	}
	item, _, observations, err := loadInsightContext(root, id)
	if err != nil {
		return InsightDetailResult{}, err
	}
	result := InsightDetailResult{Result: NewResult(StatusOK, "Insight evidence loaded on demand."), Insight: item}
	for _, observationID := range item.ObservationIDs {
		result.Findings = append(result.Findings, observations[observationID])
	}
	allComparisons, _, err := readComparisons(root)
	if err != nil {
		return InsightDetailResult{}, err
	}
	wanted := map[string]bool{}
	for _, comparisonID := range item.ComparisonIDs {
		wanted[comparisonID] = true
	}
	for _, comparison := range allComparisons {
		if wanted[comparison.ID] {
			result.Comparisons = append(result.Comparisons, comparison)
		}
	}
	return result, nil
}

func (service InsightService) DecideInsight(ctx context.Context, path, id string, input InsightDecisionInput) (InsightDecisionResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return InsightDecisionResult{}, err
	}
	service = service.defaults()
	item, original, observations, err := loadInsightContext(root, id)
	if err != nil {
		return InsightDecisionResult{}, err
	}
	reason := strings.TrimSpace(input.Rationale)
	if reason == "" {
		return InsightDecisionResult{}, NewInvalidRequestError("insight decision requires a rationale", "Provide a non-empty rationale for the decision.")
	}
	comparisons, _, err := readComparisons(root)
	if err != nil {
		return InsightDecisionResult{}, err
	}
	comparisonByID := make(map[string]distillpkg.Comparison, len(comparisons))
	for _, comparison := range comparisons {
		comparisonByID[comparison.ID] = comparison
	}
	currentDigest := distillpkg.InsightEvidenceDigest(item.ObservationIDs, item.ComparisonIDs, observations, comparisonByID)
	if item.EvidenceDigest == "" {
		item.EvidenceDigest = currentDigest
	}
	target := input.Decision
	requestDigest, _ := normalizedIntentDigest("insight_decide", struct{ ID, Decision, Rationale, Evidence string }{id, target, reason, currentDigest})
	key := firstNonEmpty(strings.TrimSpace(input.IdempotencyKey), "insight:decide:"+id+":"+target+":"+strings.TrimPrefix(requestDigest, "sha256:"))
	if receipt, found, lookupErr := mutation.LookupOperation(root, mutation.WriteSet{Command: "insight_decide", IdempotencyKey: key, RequestDigest: requestDigest}); lookupErr != nil {
		return InsightDecisionResult{}, lookupErr
	} else if found {
		current, _, _, loadErr := loadInsightContext(root, id)
		if loadErr != nil {
			return InsightDecisionResult{}, loadErr
		}
		return insightDecisionResult("Original insight decision recovered idempotently.", current, receipt), nil
	}
	switch target {
	case "plan":
		if item.Status != "pending" {
			return InsightDecisionResult{}, NewInvalidRequestError("only a pending insight can be planned", "Plan only pending insights; reload the insight state first.")
		}
		item.Status = "planned"
	case "reject":
		if item.Status != "pending" && item.Status != "planned" {
			return InsightDecisionResult{}, fmt.Errorf("insight in state %s cannot be rejected", item.Status)
		}
		item.Status = "rejected"
		item.RejectedEvidenceDigest = currentDigest
	case "obsolete":
		if item.Status == "incorporated" {
			return InsightDecisionResult{}, errors.New("an incorporated insight cannot be made obsolete; record an outcome or adjustment")
		}
		item.Status = "obsolete"
	case "reopen":
		if item.Status != "rejected" {
			return InsightDecisionResult{}, NewInvalidRequestError("only a rejected insight can be reopened", "Reopen only rejected insights; reload the insight state first.")
		}
		if item.RejectedEvidenceDigest == "" || currentDigest == item.RejectedEvidenceDigest {
			return InsightDecisionResult{}, errors.New("rejected insight cannot reopen without materially new evidence")
		}
		item.Status = "pending"
	default:
		return InsightDecisionResult{}, errors.New("decision must be plan, reject, obsolete, or reopen")
	}
	item.EvidenceDigest = currentDigest
	item.DecisionRationale = reason
	item.DecisionHistory = append(item.DecisionHistory, insightpkg.Decision{State: item.Status, Rationale: reason, EvidenceDigest: currentDigest, DecidedAt: service.Clock.Now().UTC().Format(time.RFC3339Nano)})
	if err := distillpkg.ValidateInsight(item); err != nil {
		return InsightDecisionResult{}, err
	}
	after, _ := distillpkg.Marshal(item)
	set := mutation.WriteSet{Command: "insight_decide", IdempotencyKey: key, RequestDigest: requestDigest, Changes: []mutation.Change{{Path: insightPath(item.SkillID, item.ID), BeforeDigest: sourcepkg.Digest(original), Contents: after}}}
	planned, err := mutation.PlanMutation(root, set)
	if err != nil {
		return InsightDecisionResult{}, err
	}
	receipt, err := confirmAndPublishWithOptions(ctx, root, planned, service.MutationOptions)
	if err != nil {
		return InsightDecisionResult{}, err
	}
	return insightDecisionResult("Insight decision recorded explicitly.", item, receipt), nil
}

func (service InsightService) PreviewInsightApplication(ctx context.Context, path, id string, input PreviewInsightInput) (InsightApplicationPreview, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return InsightApplicationPreview{}, err
	}
	service = service.defaults()
	item, originalInsight, observations, err := loadInsightContext(root, id)
	if err != nil {
		return InsightApplicationPreview{}, err
	}
	if item.Status != "pending" && item.Status != "planned" {
		return InsightApplicationPreview{}, fmt.Errorf("insight in state %s cannot be applied", item.Status)
	}
	if len(input.Changes) == 0 {
		return InsightApplicationPreview{}, NewInvalidRequestError("application preview requires at least one changed skill path", "Include at least one changed skill file in changes.")
	}
	skillPrefix, err := skillDirectoryForID(root, item.SkillID)
	if err != nil {
		return InsightApplicationPreview{}, err
	}
	changes := make([]mutation.Change, 0, len(input.Changes)+3)
	before := map[string][]byte{}
	targets := []string{}
	seenPaths := map[string]bool{}
	for _, requested := range input.Changes {
		path := filepath.ToSlash(strings.TrimSpace(requested.Path))
		if !strings.HasPrefix(path, skillPrefix+"/") || seenPaths[path] {
			return InsightApplicationPreview{}, errors.New("application changes must be unique paths owned by the insight target skill")
		}
		seenPaths[path] = true
		prior, readErr := readWorkspaceFile(root, path)
		if readErr != nil {
			return InsightApplicationPreview{}, fmt.Errorf("read target %s: %w", path, readErr)
		}
		contents := []byte(requested.Contents)
		if !strings.HasSuffix(requested.Contents, "\n") {
			contents = append(contents, '\n')
		}
		if sourcepkg.Digest(prior) == sourcepkg.Digest(contents) {
			return InsightApplicationPreview{}, NewInvalidRequestError(fmt.Sprintf("application path %s is unchanged", path), "Change the file contents before previewing the application.")
		}
		before[path] = prior
		targets = append(targets, path)
		changes = append(changes, mutation.Change{Path: path, BeforeDigest: sourcepkg.Digest(prior), Contents: contents})
	}
	sort.Strings(targets)
	comparisons, _, err := readComparisons(root)
	if err != nil {
		return InsightApplicationPreview{}, err
	}
	comparisonByID := make(map[string]distillpkg.Comparison, len(comparisons))
	for _, comparison := range comparisons {
		comparisonByID[comparison.ID] = comparison
	}
	mappings, err := validateApplicationMappings(item, comparisonByID, input.Mappings, targets)
	if err != nil {
		return InsightApplicationPreview{}, err
	}
	random, err := service.IDs.New()
	if err != nil || random == "" {
		return InsightApplicationPreview{}, errors.New("application ID generation failed")
	}
	upper := strings.ToUpper(random)
	operationID := "OP-" + upper
	proposalID := "APP-" + upper
	incorporationID := "INC-" + upper
	now := service.Clock.Now().UTC()
	// Plan once to obtain exact base/path pins before constructing the durable
	// audit records. The final plan rechecks the same pins and all virtual state.
	basePlan, err := mutation.PlanMutation(root, mutation.WriteSet{OperationID: operationID, Command: "insight_apply", Changes: changes})
	if err != nil {
		return InsightApplicationPreview{}, err
	}
	pins := make([]insightpkg.PathPin, 0, len(changes))
	for _, change := range basePlan.WriteSet.Changes {
		pins = append(pins, insightpkg.PathPin{Path: change.Path, Before: change.BeforeDigest, After: sourcepkg.Digest(change.Contents)})
	}
	sort.Slice(pins, func(i, j int) bool { return pins[i].Path < pins[j].Path })
	digest := insightpkg.ProposalDigest(item.ID, basePlan.BaseCatalogSnapshot, operationID, pins, mappings)
	proposalRecord := insightpkg.ApplicationProposal{SchemaVersion: 1, ID: proposalID, InsightID: item.ID, BaseCatalogVersion: basePlan.BaseCatalogSnapshot, Digest: digest, Status: "approved", ChangedFiles: targets, PathPins: pins, CreatedAt: now.Format(time.RFC3339Nano)}
	incorporation := insightpkg.Incorporation{SchemaVersion: 1, ID: incorporationID, InsightID: item.ID, ProposalID: proposalID, OperationID: operationID, State: "incorporated", Targets: targets, SourceToLocal: mappings, IncorporatedAt: now.Format(time.RFC3339Nano)}
	item.Status = "incorporated"
	item.EvidenceDigest = distillpkg.InsightEvidenceDigest(item.ObservationIDs, item.ComparisonIDs, observations, comparisonByID)
	item.DecisionRationale = "Applied after explicit approval of proposal " + proposalID
	item.DecisionHistory = append(item.DecisionHistory, insightpkg.Decision{State: "incorporated", Rationale: item.DecisionRationale, EvidenceDigest: item.EvidenceDigest, DecidedAt: now.Format(time.RFC3339Nano)})
	proposalBytes, _ := insightpkg.Marshal(proposalRecord)
	incorporationBytes, _ := insightpkg.Marshal(incorporation)
	insightBytes, _ := distillpkg.Marshal(item)
	changes = append(changes,
		mutation.Change{Path: insightPath(item.SkillID, item.ID), BeforeDigest: sourcepkg.Digest(originalInsight), Contents: insightBytes},
		mutation.Change{Path: proposalPath(item.SkillID, proposalID), Contents: proposalBytes},
		mutation.Change{Path: incorporationPath(item.SkillID, incorporationID), Contents: incorporationBytes},
	)
	requestDigest, _ := normalizedIntentDigest("insight_apply", struct{ InsightID, Digest string }{item.ID, digest})
	key := firstNonEmpty(strings.TrimSpace(input.IdempotencyKey), "insight:apply:"+item.ID+":"+strings.TrimPrefix(digest, "sha256:"))
	planned, err := mutation.PlanMutation(root, mutation.WriteSet{OperationID: operationID, Command: "insight_apply", IdempotencyKey: key, RequestDigest: requestDigest, BaseCatalogSnapshot: basePlan.BaseCatalogSnapshot, Changes: changes})
	if err != nil {
		return InsightApplicationPreview{}, err
	}
	created, expires := insightpkg.NewRuntimeProposal(now)
	plannedBytes, err := json.Marshal(planned)
	if err != nil {
		return InsightApplicationPreview{}, err
	}
	runtimeProposal := insightpkg.RuntimeProposal{Version: 1, CreatedAt: created, ExpiresAt: expires, ID: proposalID, Digest: digest, InsightID: item.ID, SkillID: item.SkillID, BaseSnapshot: planned.BaseCatalogSnapshot, PathPins: pins, Mappings: mappings, FullDiff: renderApplicationDiff(changes, before), MutationProposal: plannedBytes}
	if err := insightpkg.StoreRuntimeProposal(root, runtimeProposal); err != nil {
		return InsightApplicationPreview{}, err
	}
	confirmation := ConfirmationPolicy{PolicyRevision: "policy_v1", ActionClass: "semantic", ApplicationCommand: "ConfirmInsightApplication", Confirmation: ConfirmationRequirement{Required: true, Mode: "preview-and-approval", Pins: ConfirmationPins{ProposalID: proposalID, ProposalDigest: digest, BaseVersion: planned.BaseCatalogSnapshot}}}
	result := InsightApplicationPreview{Result: NewResult(StatusActionRequired, "Insight application is ready for explicit review; no canonical files changed."), ProposalID: proposalID, ProposalDigest: digest, BaseCatalogVersion: planned.BaseCatalogSnapshot, InsightID: item.ID, SkillID: item.SkillID, PathPins: pins, Diff: runtimeProposal.FullDiff, Confirmation: confirmation}
	confirmCommand := fmt.Sprintf("skillhub insight confirm --workspace %s --proposal %s --proposal-digest %s --base-version %s", shellQuote(root), shellQuote(proposalID), shellQuote(digest), shellQuote(planned.BaseCatalogSnapshot))
	result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Confirm the exact reviewed proposal", Command: confirmCommand, RequiresConfirmation: true})
	return result, nil
}

func (service InsightService) ConfirmInsightApplication(ctx context.Context, path, proposalID, digest, base string) (InsightApplicationResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return InsightApplicationResult{}, err
	}
	service = service.defaults()
	proposal, err := insightpkg.LoadRuntimeProposal(root, proposalID, service.Clock.Now())
	if err != nil {
		return InsightApplicationResult{}, err
	}
	expectedPins := ConfirmationPins{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseVersion: proposal.BaseSnapshot}
	suppliedPins := ConfirmationPins{ProposalID: proposalID, ProposalDigest: digest, BaseVersion: base}
	switch verifyProposalPins(time.Time{}, time.Time{}, false, false, expectedPins, suppliedPins) {
	case proposalDigestMismatch:
		confirmation := ConfirmationPolicy{PolicyRevision: "policy_v1", ActionClass: "semantic", ApplicationCommand: "ConfirmInsightApplication", Confirmation: ConfirmationRequirement{Required: true, Mode: "preview-and-approval", Pins: ConfirmationPins{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseVersion: proposal.BaseSnapshot}}}
		result := InsightApplicationResult{Result: NewResult(StatusError, "Proposal digest does not match the preview; nothing was applied."), Confirmation: confirmation, InsightID: proposal.InsightID}
		result.Items = append(result.Items, Item{ID: proposal.ID, Summary: "Proposal digest does not match the preview", Impact: "Active content remains unchanged."})
		result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Pass the matching proposal digest", Command: "ConfirmInsightApplication"})
		result.Error = &Error{Code: ErrorStaleProposal, Render: ErrorRender{Error: "The proposal can no longer be confirmed.", Why: "Proposal digest does not match the preview.", Fix: "Pass the exact proposal digest printed by the preview command."}}
		return result, nil
	case proposalPinsMismatch:
		return staleInsightApplication(proposal), nil
	}
	var planned mutation.Proposal
	if err := json.Unmarshal(proposal.MutationProposal, &planned); err != nil {
		return InsightApplicationResult{}, fmt.Errorf("decode stored mutation proposal: %w", err)
	}
	if planned.WriteSet.OperationID == "" || planned.BaseCatalogSnapshot != proposal.BaseSnapshot || planned.WriteSet.BaseCatalogSnapshot != proposal.BaseSnapshot || proposal.Digest != insightpkg.ProposalDigest(proposal.InsightID, proposal.BaseSnapshot, planned.WriteSet.OperationID, proposal.PathPins, proposal.Mappings) {
		return InsightApplicationResult{}, errors.New("persisted application proposal integrity check failed")
	}
	receipt, err := mutation.ConfirmMutationWithOptions(root, planned, mutation.Confirmation{ProposalID: planned.ID, ProposalDigest: planned.Digest, BaseCatalogSnapshot: planned.BaseCatalogSnapshot}, mutation.Options{Fault: service.MutationOptions.Fault, PostCanonical: func(expected string) (mutation.Publication, error) {
		built, buildErr := catalog.BuildCatalogGenerationWhileLocked(ctx, root, expected, catalog.BuildOptions{})
		if buildErr != nil {
			return mutation.Publication{}, buildErr
		}
		return mutation.Publication{CatalogSnapshot: built.Pointer.CatalogSnapshot, Generation: built.Pointer.Generation}, nil
	}})
	if errors.Is(err, mutation.ErrConflict) {
		return staleInsightApplication(proposal), nil
	}
	if err != nil {
		return InsightApplicationResult{}, err
	}
	incorporationID := ""
	for _, changed := range receipt.ChangedPaths {
		if strings.Contains(changed, "/incorporations/") {
			incorporationID = strings.TrimSuffix(filepath.Base(changed), ".yaml")
		}
	}
	confirmation := ConfirmationPolicy{PolicyRevision: "policy_v1", ActionClass: "semantic", ApplicationCommand: "ConfirmInsightApplication", Confirmation: ConfirmationRequirement{Required: true, Mode: "preview-and-approval", Pins: ConfirmationPins{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseVersion: proposal.BaseSnapshot}}}
	summary := "Insight proposal was applied atomically and is active locally."
	if receipt.GitDirty {
		summary = "Insight was applied locally; canonical changes are not committed."
	}
	result := InsightApplicationResult{Result: NewResult(StatusApplied, summary), Confirmation: confirmation, InsightID: proposal.InsightID, IncorporationID: incorporationID, OperationID: receipt.OperationID, ChangedPaths: receipt.ChangedPaths, CatalogSnapshot: receipt.CatalogSnapshot, Generation: receipt.Generation, ActiveLocally: true, GitDirty: receipt.GitDirty}
	impact := fmt.Sprintf("%d canonical files changed.", len(proposal.PathPins)+2)
	if len(proposal.PathPins)+2 == 3 {
		impact = "Three canonical files changed."
	}
	result.Items = append(result.Items, Item{ID: proposal.InsightID, Summary: "Insight is active locally", Impact: impact})
	result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Review the curation diff", Command: "GetOperationDiff"})
	if receipt.GitDirty {
		result.Warnings = append(result.Warnings, Warning{Code: "git_dirty", Summary: "Canonical changes are active locally but remain uncommitted; review the operation diff before committing."})
	}
	result.Warnings = append(result.Warnings, Warning{Code: "outcome_unknown", Summary: "No outcome was inferred from apply or use; record one only when explicit evidence exists."})
	return result, nil
}

func (service InsightService) RecordIncorporationOutcome(ctx context.Context, path, incorporationID string, input OutcomeInput) (OutcomeResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return OutcomeResult{}, err
	}
	service = service.defaults()
	incorporation, _, err := readIncorporation(root, incorporationID)
	if err != nil {
		return OutcomeResult{}, err
	}
	state, evidence, note, supersedes := strings.TrimSpace(input.State), cleanNonEmpty(input.Evidence), strings.TrimSpace(input.Note), strings.TrimSpace(input.Supersedes)
	requestDigest, _ := normalizedIntentDigest("outcome_record", struct {
		IncorporationID, State, Note, Supersedes string
		Evidence                                 []string
	}{incorporation.ID, state, note, supersedes, evidence})
	key := firstNonEmpty(strings.TrimSpace(input.IdempotencyKey), "outcome:"+incorporation.ID+":"+strings.TrimPrefix(requestDigest, "sha256:"))
	if receipt, found, lookupErr := mutation.LookupOperation(root, mutation.WriteSet{Command: "outcome_record", IdempotencyKey: key, RequestDigest: requestDigest}); lookupErr != nil {
		return OutcomeResult{}, lookupErr
	} else if found {
		for _, changed := range receipt.ChangedPaths {
			if strings.Contains(changed, "/outcomes/") {
				existing, _, loadErr := readOutcome(root, strings.TrimSuffix(filepath.Base(changed), ".yaml"))
				if loadErr != nil {
					return OutcomeResult{}, loadErr
				}
				return OutcomeResult{Result: NewResult(StatusApplied, "Original explicit outcome recovered idempotently."), Outcome: existing, OperationID: receipt.OperationID, ChangedPaths: receipt.ChangedPaths}, nil
			}
		}
		return OutcomeResult{}, errors.New("idempotent outcome receipt does not identify an outcome")
	}
	random, err := service.IDs.New()
	if err != nil || random == "" {
		return OutcomeResult{}, errors.New("outcome ID generation failed")
	}
	outcome := insightpkg.Outcome{SchemaVersion: 1, ID: "OUT-" + strings.ToUpper(random), IncorporationID: incorporation.ID, State: state, Evidence: evidence, Note: note, RecordedAt: service.Clock.Now().UTC().Format(time.RFC3339Nano), Supersedes: supersedes}
	if err := insightpkg.ValidateOutcome(outcome); err != nil {
		return OutcomeResult{}, err
	}
	if outcome.Supersedes != "" {
		prior, _, priorErr := readOutcome(root, outcome.Supersedes)
		if priorErr != nil || prior.IncorporationID != incorporation.ID {
			return OutcomeResult{}, errors.New("superseded outcome must exist for the same incorporation")
		}
	}
	data, _ := insightpkg.Marshal(outcome)
	set := mutation.WriteSet{Command: "outcome_record", IdempotencyKey: key, RequestDigest: requestDigest, Changes: []mutation.Change{{Path: outcomePathForIncorporation(root, incorporation, outcome.ID), Contents: data}}}
	planned, err := mutation.PlanMutation(root, set)
	if err != nil {
		return OutcomeResult{}, err
	}
	receipt, err := confirmAndPublishWithOptions(ctx, root, planned, service.MutationOptions)
	if err != nil {
		return OutcomeResult{}, err
	}
	return OutcomeResult{Result: NewResult(StatusApplied, "Explicit incorporation outcome recorded; no outcome was inferred."), Outcome: outcome, OperationID: receipt.OperationID, ChangedPaths: receipt.ChangedPaths}, nil
}

func (InsightService) QueryArtifactProvenance(ctx context.Context, path, artifactPath string) (ProvenanceResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return ProvenanceResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ProvenanceResult{}, err
	}
	artifactPath = filepath.ToSlash(strings.TrimSpace(artifactPath))
	if artifactPath == "" || strings.HasPrefix(artifactPath, "/") || strings.HasPrefix(artifactPath, "../") {
		return ProvenanceResult{}, errors.New("artifact path must be workspace-relative")
	}
	return buildProvenance(root, artifactPath, "")
}
func (InsightService) QueryFindingImpact(ctx context.Context, path, observationID string) (ProvenanceResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return ProvenanceResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return ProvenanceResult{}, err
	}
	if !distillpkg.ValidEntityID(observationID) {
		return ProvenanceResult{}, NewInvalidRequestError("invalid observation ID", "Pass an observation ID returned by the workspace.")
	}
	return buildProvenance(root, "", observationID)
}

func (InsightService) GetOperationDiff(ctx context.Context, path, operationID string) (OperationDiffResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return OperationDiffResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return OperationDiffResult{}, err
	}
	if !distillpkg.ValidEntityID(operationID) {
		return OperationDiffResult{}, NewInvalidRequestError("invalid operation ID", "Pass an operation ID returned by the workspace.")
	}
	type receiptChange struct {
		Path             string `yaml:"path"`
		BeforeDigest     string `yaml:"before"`
		AfterDigest      string `yaml:"after"`
		BeforeContent    string `yaml:"before_content"`
		AfterContent     string `yaml:"after_content"`
		ContentAvailable bool   `yaml:"content_available"`
	}
	var document struct {
		ID      string          `yaml:"id"`
		Changes []receiptChange `yaml:"changes"`
	}
	found := false
	err = walkYAML(root, "history/operations", func(_ string, data []byte) error {
		var header struct {
			ID string `yaml:"id"`
		}
		if yaml.Unmarshal(data, &header) != nil || header.ID != operationID {
			return nil
		}
		if found {
			return errors.New("duplicate operation ID")
		}
		found = true
		return yaml.Unmarshal(data, &document)
	})
	if err != nil {
		return OperationDiffResult{}, err
	}
	if !found {
		return OperationDiffResult{}, errors.New("operation not found")
	}
	paths := []string{}
	changes := make([]OperationChange, 0, len(document.Changes))
	for _, stored := range document.Changes {
		paths = append(paths, stored.Path)
		currentDigest, digestErr := workspacePathDigest(root, stored.Path)
		if digestErr != nil {
			return OperationDiffResult{}, digestErr
		}
		change := OperationChange{Path: stored.Path, BeforeDigest: stored.BeforeDigest, AfterDigest: stored.AfterDigest, CurrentMatchesAfter: currentDigest == stored.AfterDigest}
		if stored.ContentAvailable {
			change.Before, change.After = stored.BeforeContent, stored.AfterContent
			change.Diff = renderBoundedOperationDiff(stored.Path, stored.BeforeContent, stored.AfterContent)
			change.DiffAvailable = true
		} else {
			change.DigestOnlyMetadata = true
		}
		changes = append(changes, change)
	}
	sort.Strings(paths)
	quoted := make([]string, len(paths))
	for index, value := range paths {
		quoted[index] = shellQuote(value)
	}
	result := OperationDiffResult{Result: NewResult(StatusOK, "Operation history loaded; retained content is shown as a bounded before/after diff and older entries are labeled digest-only metadata."), OperationID: operationID, Changes: changes, ReviewCommand: "git diff -- " + strings.Join(quoted, " "), Warning: "Skill Hub only inspects Git and never runs restore, remove, reset, checkout, or revert. Recheck current digests immediately before any manual undo."}
	for _, change := range changes {
		tracked, headDigest := inspectGitPath(root, change.Path)
		switch {
		case !change.CurrentMatchesAfter:
			result.RestoreGuidance = append(result.RestoreGuidance, fmt.Sprintf("%s: no automatic guidance; current bytes no longer match operation after-digest %s.", change.Path, change.AfterDigest))
		case tracked && change.BeforeDigest != "" && headDigest == change.BeforeDigest:
			result.RestoreGuidance = append(result.RestoreGuidance, fmt.Sprintf("%s: tracked restore candidate; current after-digest and HEAD before-digest were verified. Recheck both before manually restoring this path.", change.Path))
		case !tracked && change.BeforeDigest == "":
			result.RestoreGuidance = append(result.RestoreGuidance, fmt.Sprintf("%s: untracked creation removal candidate; current after-digest was verified. Recheck it before manually removing only this path.", change.Path))
		default:
			result.RestoreGuidance = append(result.RestoreGuidance, fmt.Sprintf("%s: use a reviewed compensating edit from retained content or Git history; no safe direct restore/removal was established.", change.Path))
		}
	}
	return result, nil
}

func loadInsightContext(root, id string) (distillpkg.Insight, []byte, map[string]distillpkg.Observation, error) {
	insights, bytesByID, err := readInsightsWithBytes(root)
	if err != nil {
		return distillpkg.Insight{}, nil, nil, err
	}
	var selected distillpkg.Insight
	found := false
	for _, item := range insights {
		if item.ID == id {
			selected = item
			found = true
			break
		}
	}
	if !found {
		return selected, nil, nil, ErrInsightNotFound
	}
	observations, _, err := readObservations(root)
	if err != nil {
		return selected, nil, nil, err
	}
	byID := map[string]distillpkg.Observation{}
	for _, item := range observations {
		byID[item.ID] = item
	}
	for _, observationID := range selected.ObservationIDs {
		if _, ok := byID[observationID]; !ok {
			return selected, nil, nil, fmt.Errorf("insight references missing observation %s", observationID)
		}
	}
	return selected, bytesByID[id], byID, nil
}
func validateApplicationMappings(item distillpkg.Insight, comparisons map[string]distillpkg.Comparison, input []ApplicationMapping, targets []string) ([]insightpkg.SourceToLocalMapping, error) {
	targetSet := map[string]bool{}
	for _, target := range targets {
		targetSet[target] = true
	}
	wanted := map[string]bool{}
	for _, id := range item.ObservationIDs {
		wanted[id] = true
	}
	for _, comparisonID := range item.ComparisonIDs {
		comparison, ok := comparisons[comparisonID]
		if !ok {
			return nil, fmt.Errorf("insight comparison %s does not exist", comparisonID)
		}
		for _, id := range comparison.ObservationIDs {
			wanted[id] = true
		}
	}
	seen := map[string]bool{}
	result := make([]insightpkg.SourceToLocalMapping, 0, len(input))
	for _, mapping := range input {
		concept := strings.TrimSpace(mapping.Concept)
		key := mapping.ObservationID + "\x00" + mapping.ArtifactPath + "\x00" + concept
		if !wanted[mapping.ObservationID] || seen[key] || !targetSet[mapping.ArtifactPath] || concept == "" {
			return nil, errors.New("mappings must be unique, cover supporting observations, and target changed skill paths")
		}
		seen[key] = true
		result = append(result, insightpkg.SourceToLocalMapping{ObservationID: mapping.ObservationID, ArtifactPath: mapping.ArtifactPath, Concept: concept})
	}
	covered := map[string]bool{}
	mappedTargets := map[string]bool{}
	for _, mapping := range result {
		covered[mapping.ObservationID] = true
		mappedTargets[mapping.ArtifactPath] = true
	}
	if len(covered) != len(wanted) {
		return nil, errors.New("application proposal must map every direct and comparison-member observation")
	}
	if len(mappedTargets) != len(targetSet) {
		return nil, errors.New("application proposal must map every changed skill path")
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ObservationID != result[j].ObservationID {
			return result[i].ObservationID < result[j].ObservationID
		}
		if result[i].ArtifactPath != result[j].ArtifactPath {
			return result[i].ArtifactPath < result[j].ArtifactPath
		}
		return result[i].Concept < result[j].Concept
	})
	return result, nil
}
func skillDirectoryForID(root, id string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(root, "skills", "*", id, "skill.meta.yaml"))
	if err != nil || len(matches) != 1 {
		return "", errors.New("target skill not found or ambiguous")
	}
	rel, err := filepath.Rel(root, filepath.Dir(matches[0]))
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}
func renderApplicationDiff(changes []mutation.Change, before map[string][]byte) string {
	var out strings.Builder
	copyChanges := append([]mutation.Change(nil), changes...)
	sort.Slice(copyChanges, func(i, j int) bool { return copyChanges[i].Path < copyChanges[j].Path })
	for _, change := range copyChanges {
		prior, ok := before[change.Path]
		if !ok {
			continue
		}
		fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", change.Path, change.Path)
		for _, line := range strings.Split(strings.TrimSuffix(string(prior), "\n"), "\n") {
			out.WriteString("-" + line + "\n")
		}
		for _, line := range strings.Split(strings.TrimSuffix(string(change.Contents), "\n"), "\n") {
			out.WriteString("+" + line + "\n")
		}
	}
	return out.String()
}
func staleInsightApplication(proposal insightpkg.RuntimeProposal) InsightApplicationResult {
	confirmation := ConfirmationPolicy{PolicyRevision: "policy_v1", ActionClass: "semantic", ApplicationCommand: "ConfirmInsightApplication", Confirmation: ConfirmationRequirement{Required: true, Mode: "preview-and-approval", Pins: ConfirmationPins{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseVersion: proposal.BaseSnapshot}}}
	result := InsightApplicationResult{Result: NewResult(StatusError, "Proposal is stale; nothing was applied."), Confirmation: confirmation, InsightID: proposal.InsightID}
	result.Items = append(result.Items, Item{ID: proposal.ID, Summary: "Reviewed proposal no longer matches the target", Impact: "Active content remains unchanged."})
	result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Regenerate the proposal", Command: "PreviewInsightApplication"})
	result.Error = &Error{Code: ErrorStaleProposal, Render: ErrorRender{Error: "The proposal can no longer be confirmed.", Why: "The target changed after the preview was created.", Fix: "Regenerate the proposal and review the updated diff."}}
	return result
}
func insightDecisionResult(summary string, item distillpkg.Insight, receipt mutation.Receipt) InsightDecisionResult {
	return InsightDecisionResult{Result: NewResult(StatusApplied, summary), Insight: item, OperationID: receipt.OperationID, ChangedPaths: receipt.ChangedPaths, CatalogSnapshot: receipt.CatalogSnapshot, Generation: receipt.Generation}
}
func proposalPath(skillID, id string) string {
	return "distill/skills/" + skillID + "/proposals/" + id + ".yaml"
}
func incorporationPath(skillID, id string) string {
	return "distill/skills/" + skillID + "/incorporations/" + id + ".yaml"
}
func outcomePath(skillID, id string) string {
	return "distill/skills/" + skillID + "/outcomes/" + id + ".yaml"
}
func readIncorporation(root, id string) (insightpkg.Incorporation, []byte, error) {
	var selected insightpkg.Incorporation
	var data []byte
	found := false
	err := walkYAML(root, "distill/skills", func(path string, value []byte) error {
		if !strings.Contains(path, "/incorporations/") {
			return nil
		}
		item, parseErr := insightpkg.ParseIncorporation(value)
		if parseErr != nil {
			return parseErr
		}
		if item.ID == id {
			if found {
				return errors.New("duplicate incorporation ID")
			}
			selected, data, found = item, value, true
		}
		return nil
	})
	if err != nil {
		return selected, nil, err
	}
	if !found {
		return selected, nil, errors.New("incorporation not found")
	}
	return selected, data, nil
}
func readOutcome(root, id string) (insightpkg.Outcome, []byte, error) {
	var selected insightpkg.Outcome
	var data []byte
	found := false
	err := walkYAML(root, "distill/skills", func(path string, value []byte) error {
		if !strings.Contains(path, "/outcomes/") {
			return nil
		}
		item, parseErr := insightpkg.ParseOutcome(value)
		if parseErr != nil {
			return parseErr
		}
		if item.ID == id {
			selected, data, found = item, value, true
		}
		return nil
	})
	if err != nil {
		return selected, nil, err
	}
	if !found {
		return selected, nil, errors.New("outcome not found")
	}
	return selected, data, nil
}
func outcomePathForIncorporation(root string, incorporation insightpkg.Incorporation, id string) string {
	insights, _ := readInsights(root)
	for _, item := range insights {
		if item.ID == incorporation.InsightID {
			return outcomePath(item.SkillID, id)
		}
	}
	return "distill/skills/unknown/outcomes/" + id + ".yaml"
}
func buildProvenance(root, artifactPath, observationID string) (ProvenanceResult, error) {
	insights, err := readInsights(root)
	if err != nil {
		return ProvenanceResult{}, err
	}
	observations, _, err := readObservations(root)
	if err != nil {
		return ProvenanceResult{}, err
	}
	comparisons, _, err := readComparisons(root)
	if err != nil {
		return ProvenanceResult{}, err
	}
	obsByID := map[string]distillpkg.Observation{}
	for _, item := range observations {
		obsByID[item.ID] = item
	}
	insightByID := map[string]distillpkg.Insight{}
	for _, item := range insights {
		insightByID[item.ID] = item
	}
	comparisonByID := map[string]distillpkg.Comparison{}
	for _, item := range comparisons {
		comparisonByID[item.ID] = item
	}
	result := ProvenanceResult{Result: NewResult(StatusOK, "No matching provenance was found."), ArtifactPath: artifactPath, ObservationID: observationID, Chains: []ProvenanceChain{}, AffectedArtifacts: []string{}}
	affected := map[string]bool{}
	err = walkYAML(root, "distill/skills", func(path string, data []byte) error {
		if !strings.Contains(path, "/incorporations/") {
			return nil
		}
		incorporation, parseErr := insightpkg.ParseIncorporation(data)
		if parseErr != nil {
			return parseErr
		}
		matched := false
		for _, mapping := range incorporation.SourceToLocal {
			if artifactPath != "" && mapping.ArtifactPath == artifactPath {
				matched = true
			}
			if observationID != "" && mapping.ObservationID == observationID {
				matched = true
				affected[mapping.ArtifactPath] = true
			}
		}
		if !matched {
			return nil
		}
		item, ok := insightByID[incorporation.InsightID]
		if !ok {
			return fmt.Errorf("incorporation %s references missing insight", incorporation.ID)
		}
		chain := ProvenanceChain{Insight: item, Incorporation: incorporation}
		findingIDs := map[string]bool{}
		for _, id := range item.ObservationIDs {
			findingIDs[id] = true
		}
		for _, comparisonID := range item.ComparisonIDs {
			comparison, ok := comparisonByID[comparisonID]
			if !ok {
				return fmt.Errorf("insight %s references missing comparison", item.ID)
			}
			for _, id := range comparison.ObservationIDs {
				findingIDs[id] = true
			}
		}
		ids := make([]string, 0, len(findingIDs))
		for id := range findingIDs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			obs, ok := obsByID[id]
			if !ok {
				return fmt.Errorf("insight %s references missing observation", item.ID)
			}
			chain.Findings = append(chain.Findings, ProvenanceFinding{Observation: obs, SourceRevision: obs.LastSeen})
		}
		result.Chains = append(result.Chains, chain)
		return nil
	})
	if err != nil {
		return result, err
	}
	for path := range affected {
		result.AffectedArtifacts = append(result.AffectedArtifacts, path)
	}
	sort.Strings(result.AffectedArtifacts)
	sort.Slice(result.Chains, func(i, j int) bool { return result.Chains[i].Incorporation.ID < result.Chains[j].Incorporation.ID })
	if len(result.Chains) > 0 {
		result.Summary = fmt.Sprintf("Loaded %d complete local-to-source provenance chain(s).", len(result.Chains))
	}
	return result, nil
}
func cleanNonEmpty(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
func workspacePathDigest(root, path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read current operation path %s: %w", path, err)
	}
	return sourcepkg.Digest(data), nil
}

func renderBoundedOperationDiff(path, before, after string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "--- before/%s\n+++ after/%s\n", path, path)
	for _, line := range strings.Split(strings.TrimSuffix(before, "\n"), "\n") {
		if before != "" {
			out.WriteString("-" + line + "\n")
		}
	}
	for _, line := range strings.Split(strings.TrimSuffix(after, "\n"), "\n") {
		if after != "" {
			out.WriteString("+" + line + "\n")
		}
	}
	return out.String()
}

// inspectGitPath performs read-only inspection. Failure means no safe tracked
// restore can be established; callers deliberately emit no mutating command.
func inspectGitPath(root, path string) (bool, string) {
	tracked := exec.Command("git", "-C", root, "ls-files", "--error-unmatch", "--", path)
	if tracked.Run() != nil {
		return false, ""
	}
	show := exec.Command("git", "-C", root, "show", "HEAD:"+path)
	data, err := show.Output()
	if err != nil {
		return true, ""
	}
	return true, sourcepkg.Digest(data)
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
