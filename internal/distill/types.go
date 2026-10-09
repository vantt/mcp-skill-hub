// Package distill defines the durable, transport-neutral contracts for source learning.
package distill

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	insightpkg "github.com/vantt/mcp-skill-hub/internal/insight"
	"github.com/vantt/mcp-skill-hub/internal/source"
	"gopkg.in/yaml.v3"
)

const SchemaVersion = 1

var (
	stableKeyPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	entityIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	digestPattern    = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ChangedResource is the immutable scope selected by an adapter diff.
type ChangedResource struct {
	Path   string `yaml:"path" json:"path"`
	Status string `yaml:"status" json:"status"`
}

// CoverageEntry proves that every changed resource received an explicit disposition.
type CoverageEntry struct {
	Resource string `yaml:"resource" json:"resource"`
	Status   string `yaml:"status" json:"status"`
	Reason   string `yaml:"reason" json:"reason"`
	Blocking bool   `yaml:"blocking,omitempty" json:"blocking,omitempty"`
}

// RevisionIdentity is the stable portion of an adapter revision. ObservedAt is
// intentionally excluded because it is discovery metadata, not revision identity.
type RevisionIdentity struct {
	Kind          string `yaml:"kind" json:"kind"`
	Value         string `yaml:"value" json:"value"`
	ContentDigest string `yaml:"content_digest" json:"content_digest"`
}

func IdentityOf(value source.Revision) RevisionIdentity {
	return RevisionIdentity{Kind: value.Kind, Value: value.Value, ContentDigest: value.ContentDigest}
}

func SameRevision(left, right source.Revision) bool { return IdentityOf(left) == IdentityOf(right) }

// RevisionPackage is the legacy immutable input bundle, removed in Phase 3.
type RevisionPackage struct {
	Version          int                `json:"version"`
	RunID            string             `json:"run_id"`
	SourceID         string             `json:"source_id"`
	Digest           string             `json:"digest"`
	FromRevision     *source.Revision   `json:"from_revision,omitempty"`
	ToRevision       source.Revision    `json:"to_revision"`
	ChangedResources []ChangedResource  `json:"changed_resources"`
	Resources        []PackagedResource `json:"resources"`
	CreatedAt        time.Time          `json:"created_at"`
}

// PackagedResource is one pinned file inside a legacy RevisionPackage.
type PackagedResource struct {
	Revision RevisionIdentity `json:"revision"`
	Path     string           `json:"path"`
	Digest   string           `json:"digest"`
	Size     int64            `json:"size"`
	Side     string           `json:"side"`
}

func LoadRevisionPackage(runtimeRoot, runID string) (RevisionPackage, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeRoot, "distill", "sources", "*", "runs", runID+".yaml"))
	if err == nil && len(paths) > 0 {
		if data, readErr := os.ReadFile(paths[0]); readErr == nil {
			var run Run
			if yaml.Unmarshal(data, &run) == nil {
				var resources []PackagedResource
				for _, ch := range run.ChangedResources {
					revIdentity := IdentityOf(run.ToRevision)
					side := "to"
					if ch.Status == "deleted" && run.FromRevision != nil {
						revIdentity = IdentityOf(*run.FromRevision)
						side = "from"
					}
					resources = append(resources, PackagedResource{
						Path:     ch.Path,
						Side:     side,
						Revision: revIdentity,
						Digest:   "sha256:0000000000000000000000000000000000000000000000000000000000000000",
					})
				}
				return RevisionPackage{
					RunID:            runID,
					SourceID:         run.SourceID,
					Digest:           run.PackageDigest,
					ToRevision:       run.ToRevision,
					FromRevision:     run.FromRevision,
					ChangedResources: run.ChangedResources,
					Resources:        resources,
				}, nil
			}
		}
	}
	return RevisionPackage{RunID: runID}, nil
}

func ReadEvidence(runtimeRoot string, pkg RevisionPackage, revision RevisionIdentity, path string) ([]byte, error) {
	return nil, nil
}

// ProposedArtifacts preserves an awaiting-decision submission exactly as
// normalized, together with the artifact identities it would have produced.
type ProposedArtifacts struct {
	Digest        string   `yaml:"digest" json:"digest"`
	Payload       string   `yaml:"payload" json:"payload"`
	FindingIDs    []string `yaml:"findings,omitempty" json:"findings,omitempty"`
	ComparisonIDs []string `yaml:"comparisons,omitempty" json:"comparisons,omitempty"`
	InsightIDs    []string `yaml:"insights,omitempty" json:"insights,omitempty"`
}

type DecisionRecord struct {
	Decision string `yaml:"decision" json:"decision"`
	At       string `yaml:"at" json:"at"`
}

// Run is the canonical state machine and cursor-finalization record.
type Run struct {
	SchemaVersion        int                `yaml:"schema_version" json:"schema_version"`
	ID                   string             `yaml:"id" json:"id"`
	SourceID             string             `yaml:"source_id" json:"source_id"`
	State                string             `yaml:"state" json:"state"`
	FromRevision         *source.Revision   `yaml:"from_revision,omitempty" json:"from_revision,omitempty"`
	ToRevision           source.Revision    `yaml:"to_revision" json:"to_revision"`
	ChangedResources     []ChangedResource  `yaml:"changed_resources" json:"changed_resources"`
	PackageDigest        string             `yaml:"package_digest" json:"package_digest"`
	PreparedAt           string             `yaml:"prepared_at" json:"prepared_at"`
	StartedAt            string             `yaml:"started_at,omitempty" json:"started_at,omitempty"`
	FinalizedAt          string             `yaml:"finalized_at,omitempty" json:"finalized_at,omitempty"`
	CancelledAt          string             `yaml:"cancelled_at,omitempty" json:"cancelled_at,omitempty"`
	Failure              string             `yaml:"failure,omitempty" json:"failure,omitempty"`
	Attempt              int                `yaml:"attempt" json:"attempt"`
	Coverage             []CoverageEntry    `yaml:"coverage,omitempty" json:"coverage,omitempty"`
	FindingIDs           []string           `yaml:"finding_ids,omitempty" json:"finding_ids,omitempty"`
	ComparisonIDs        []string           `yaml:"comparison_ids,omitempty" json:"comparison_ids,omitempty"`
	InsightIDs           []string           `yaml:"insight_ids,omitempty" json:"insight_ids,omitempty"`
	OutstandingDecisions []OutstandingIssue `yaml:"outstanding_decisions,omitempty" json:"outstanding_decisions,omitempty"`
	ProposedArtifacts    *ProposedArtifacts `yaml:"proposed_artifacts,omitempty" json:"proposed_artifacts,omitempty"`
	DecisionHistory      []DecisionRecord   `yaml:"decision_history,omitempty" json:"decision_history,omitempty"`
}

// Evidence pins a claim to bytes in one immutable revision package.
type Evidence struct {
	Revision      RevisionIdentity `yaml:"revision" json:"revision"`
	RunID         string           `yaml:"run_id" json:"run_id"`
	PackageDigest string           `yaml:"package_digest" json:"package_digest"`
	Path          string           `yaml:"path" json:"path"`
	Locator       string           `yaml:"locator" json:"locator"`
	Digest        string           `yaml:"digest" json:"digest"`
}

// Observation records what a source says; it is never an adoption decision.
type Observation struct {
	SchemaVersion  int              `yaml:"schema_version" json:"schema_version"`
	ID             string           `yaml:"id" json:"id"`
	SourceID       string           `yaml:"source_id" json:"source_id"`
	RunID          string           `yaml:"run_id" json:"run_id"`
	StableKey      string           `yaml:"stable_key" json:"stable_key"`
	Status         string           `yaml:"status" json:"status"`
	FirstSeen      RevisionIdentity `yaml:"first_seen" json:"first_seen"`
	LastSeen       RevisionIdentity `yaml:"last_seen" json:"last_seen"`
	What           string           `yaml:"what" json:"what"`
	Vocabulary     []string         `yaml:"vocabulary" json:"vocabulary"`
	Evidence       []Evidence       `yaml:"evidence" json:"evidence"`
	SupersedesIDs  []string         `yaml:"supersedes_ids,omitempty" json:"supersedes_ids,omitempty"`
	SupersededByID string           `yaml:"superseded_by_id,omitempty" json:"superseded_by_id,omitempty"`
}

// Comparison captures optional cross-source synthesis and its pinned basis.
type Comparison struct {
	SchemaVersion  int                         `yaml:"schema_version" json:"schema_version"`
	ID             string                      `yaml:"id" json:"id"`
	RunID          string                      `yaml:"run_id" json:"run_id"`
	Subject        string                      `yaml:"subject" json:"subject"`
	ObservationIDs []string                    `yaml:"observation_ids" json:"observation_ids"`
	Verdict        string                      `yaml:"verdict" json:"verdict"`
	Tradeoffs      string                      `yaml:"tradeoffs" json:"tradeoffs"`
	BasedOn        map[string]RevisionIdentity `yaml:"based_on" json:"based_on"`
	Stale          bool                        `yaml:"stale" json:"stale"`
}

// Insight records what a curated skill should consider adopting.
type Insight struct {
	SchemaVersion          int                   `yaml:"schema_version" json:"schema_version"`
	ID                     string                `yaml:"id" json:"id"`
	RunID                  string                `yaml:"run_id" json:"run_id"`
	StableKey              string                `yaml:"stable_key" json:"stable_key"`
	SkillID                string                `yaml:"skill_id" json:"skill_id"`
	Status                 string                `yaml:"status" json:"status"`
	Recommendation         string                `yaml:"recommendation" json:"recommendation"`
	ObservationIDs         []string              `yaml:"observation_ids" json:"observation_ids"`
	ComparisonIDs          []string              `yaml:"comparison_ids,omitempty" json:"comparison_ids,omitempty"`
	Category               string                `yaml:"category" json:"category"`
	Priority               string                `yaml:"priority" json:"priority"`
	Rationale              string                `yaml:"rationale" json:"rationale"`
	EvidenceDigest         string                `yaml:"evidence_digest" json:"evidence_digest"`
	RejectedEvidenceDigest string                `yaml:"rejected_evidence_digest,omitempty" json:"rejected_evidence_digest,omitempty"`
	DecisionRationale      string                `yaml:"decision_rationale,omitempty" json:"decision_rationale,omitempty"`
	DecisionHistory        []insightpkg.Decision `yaml:"decision_history,omitempty" json:"decision_history,omitempty"`
}

// OutstandingIssue is only valid for a blocking ambiguity or coverage decision.
type OutstandingIssue struct {
	Kind     string `yaml:"kind" json:"kind"`
	Resource string `yaml:"resource,omitempty" json:"resource,omitempty"`
	Question string `yaml:"question" json:"question"`
}

func ObservationID(sourceID, stableKey string) string { return "OBS-" + sourceID + "--" + stableKey }
func InsightID(skillID, stableKey string) string      { return "INS-" + skillID + "--" + stableKey }
func ValidStableKey(value string) bool {
	return len(value) <= 96 && stableKeyPattern.MatchString(value)
}
func ValidEntityID(value string) bool {
	return len(value) <= 240 && entityIDPattern.MatchString(value)
}

func Marshal(value any) ([]byte, error) { return yaml.Marshal(value) }

func ParseRun(data []byte) (Run, error) {
	var value Run
	if err := strict(data, &value); err != nil {
		return value, err
	}
	if err := ValidateRun(value); err != nil {
		return value, err
	}
	return value, nil
}
func ParseObservation(data []byte) (Observation, error) {
	var value Observation
	if err := strict(data, &value); err != nil {
		return value, err
	}
	if err := ValidateObservation(value); err != nil {
		return value, err
	}
	return value, nil
}
func ParseComparison(data []byte) (Comparison, error) {
	var value Comparison
	if err := strict(data, &value); err != nil {
		return value, err
	}
	if err := ValidateComparison(value); err != nil {
		return value, err
	}
	return value, nil
}
func ParseInsight(data []byte) (Insight, error) {
	var value Insight
	if err := strict(data, &value); err != nil {
		return value, err
	}
	if err := ValidateInsight(value); err != nil {
		return value, err
	}
	return value, nil
}

func ValidateRun(value Run) error {
	if value.SchemaVersion != SchemaVersion || !ValidEntityID(value.ID) || !ValidEntityID(value.SourceID) || !digestPattern.MatchString(value.PackageDigest) || value.Attempt < 0 {
		return errors.New("invalid distill run identity")
	}
	if err := validateRevision(value.ToRevision); err != nil {
		return fmt.Errorf("to_revision: %w", err)
	}
	if value.FromRevision != nil {
		if err := validateRevision(*value.FromRevision); err != nil {
			return fmt.Errorf("from_revision: %w", err)
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, value.PreparedAt); err != nil {
		return errors.New("prepared_at must be RFC3339")
	}
	switch value.State {
	case "prepared", "in_progress", "awaiting_decision", "failed", "finalized", "cancelled":
	default:
		return errors.New("invalid distill run state")
	}
	seen := map[string]bool{}
	for _, item := range value.ChangedResources {
		if item.Path == "" || seen[item.Path] {
			return errors.New("changed resources must have unique paths")
		}
		seen[item.Path] = true
		switch item.Status {
		case "added", "modified", "deleted":
		default:
			return errors.New("invalid changed resource status")
		}
	}
	if value.State == "failed" && strings.TrimSpace(value.Failure) == "" {
		return errors.New("failed run requires a failure reason")
	}
	if value.State != "failed" && value.Failure != "" {
		return errors.New("failure reason is only valid for failed runs")
	}
	if value.State == "finalized" {
		if value.FinalizedAt == "" {
			return errors.New("finalized run requires finalized_at")
		}
		if value.ProposedArtifacts != nil || len(value.OutstandingDecisions) != 0 {
			return errors.New("finalized run cannot retain a pending proposal or blockers")
		}
		if err := ValidateCoverage(value.ChangedResources, value.Coverage); err != nil {
			return err
		}
	}
	if value.State == "awaiting_decision" {
		if value.ProposedArtifacts == nil || !digestPattern.MatchString(value.ProposedArtifacts.Digest) || value.ProposedArtifacts.Payload == "" || value.ProposedArtifacts.Digest != source.Digest([]byte(value.ProposedArtifacts.Payload)) || len(value.OutstandingDecisions) == 0 {
			return errors.New("awaiting-decision run requires an immutable proposal and blockers")
		}
	}
	for _, issue := range value.OutstandingDecisions {
		if issue.Kind != "ambiguity" && issue.Kind != "coverage" {
			return errors.New("outstanding decisions may only represent ambiguity or coverage")
		}
		if strings.TrimSpace(issue.Question) == "" {
			return errors.New("outstanding decision requires a question")
		}
	}
	return nil
}

func ValidateCoverage(changed []ChangedResource, coverage []CoverageEntry) error {
	want := make(map[string]bool, len(changed))
	for _, item := range changed {
		want[item.Path] = true
	}
	seen := make(map[string]bool, len(coverage))
	for _, item := range coverage {
		if !want[item.Resource] || seen[item.Resource] {
			return fmt.Errorf("coverage resource %q is missing, duplicate, or outside changed scope", item.Resource)
		}
		seen[item.Resource] = true
		switch item.Status {
		case "analyzed", "consulted", "ruled_out_with_reason", "deferred", "unreadable", "out_of_scope":
		default:
			return fmt.Errorf("invalid coverage status %q", item.Status)
		}
		if strings.TrimSpace(item.Reason) == "" {
			return fmt.Errorf("coverage resource %q requires a reason", item.Resource)
		}
	}
	if len(seen) != len(want) {
		return errors.New("coverage must classify every changed resource exactly once")
	}
	return nil
}

func ValidateObservation(value Observation) error {
	if value.SchemaVersion != SchemaVersion || !ValidEntityID(value.SourceID) || !ValidStableKey(value.StableKey) || value.ID != ObservationID(value.SourceID, value.StableKey) || !ValidEntityID(value.ID) {
		return errors.New("invalid observation stable identity")
	}
	switch value.Status {
	case "active", "removed", "superseded":
	default:
		return errors.New("invalid observation status")
	}
	if strings.TrimSpace(value.What) == "" || len(value.Vocabulary) == 0 || len(value.Evidence) == 0 {
		return errors.New("observation requires what, source vocabulary, and evidence")
	}
	for _, term := range value.Vocabulary {
		if strings.TrimSpace(term) == "" {
			return errors.New("observation vocabulary terms cannot be empty")
		}
	}
	if !ValidEntityID(value.RunID) || value.FirstSeen.Value == "" || value.LastSeen.Value == "" {
		return errors.New("observation requires run and revision identities")
	}
	for _, evidence := range value.Evidence {
		if evidence.Revision.Value == "" || evidence.Revision.Kind == "" || !digestPattern.MatchString(evidence.Revision.ContentDigest) || !ValidEntityID(evidence.RunID) || !digestPattern.MatchString(evidence.PackageDigest) || !safeEvidencePath(evidence.Path) || !(evidence.Locator == evidence.Path || strings.HasPrefix(evidence.Locator, evidence.Path+"#")) || !digestPattern.MatchString(evidence.Digest) {
			return errors.New("observation evidence requires revision and package identities, a contained locator, and SHA-256 digests")
		}
	}
	if value.Status == "superseded" && value.SupersededByID == "" {
		return errors.New("superseded observation requires superseded_by_id")
	}
	if value.Status != "superseded" && value.SupersededByID != "" {
		return errors.New("superseded_by_id is only valid for superseded observations")
	}
	return nil
}

func ValidateComparison(value Comparison) error {
	if value.SchemaVersion != SchemaVersion || !ValidEntityID(value.ID) || !ValidEntityID(value.RunID) || strings.TrimSpace(value.Subject) == "" || len(value.ObservationIDs) == 0 || len(value.BasedOn) == 0 {
		return errors.New("invalid comparison")
	}
	seen := map[string]bool{}
	for _, id := range value.ObservationIDs {
		if !ValidEntityID(id) || seen[id] {
			return errors.New("comparison observation identities must be valid and unique")
		}
		seen[id] = true
		if revision, ok := value.BasedOn[id]; !ok || revision.Value == "" || revision.Kind == "" || !digestPattern.MatchString(revision.ContentDigest) {
			return errors.New("comparison based_on must correspond exactly to observation_ids")
		}
	}
	if len(seen) != len(value.BasedOn) {
		return errors.New("comparison based_on must correspond exactly to observation_ids")
	}
	switch value.Verdict {
	case "convergent", "complementary", "conflicting", "same-mechanism-different-scope", "probable-duplicate", "insufficient-evidence":
	default:
		return errors.New("invalid comparison verdict")
	}
	return nil
}

func ValidateInsight(value Insight) error {
	if value.SchemaVersion != SchemaVersion || !ValidEntityID(value.RunID) || !ValidEntityID(value.SkillID) || !ValidStableKey(value.StableKey) || value.ID != InsightID(value.SkillID, value.StableKey) || !ValidEntityID(value.ID) {
		return errors.New("invalid insight stable identity")
	}
	switch value.Status {
	case "pending", "planned", "rejected", "obsolete", "withdrawn", "partially_incorporated", "incorporated":
	default:
		return errors.New("invalid insight status")
	}
	if strings.TrimSpace(value.Recommendation) == "" || len(value.ObservationIDs) == 0 || strings.TrimSpace(value.Category) == "" || strings.TrimSpace(value.Rationale) == "" || (value.EvidenceDigest != "" && !digestPattern.MatchString(value.EvidenceDigest)) {
		return errors.New("insight requires recommendation, evidence, category, rationale, and a valid evidence digest when present")
	}
	if value.Status == "rejected" && (strings.TrimSpace(value.DecisionRationale) == "" || !digestPattern.MatchString(value.RejectedEvidenceDigest)) {
		return errors.New("rejected insight requires rationale and rejected evidence digest")
	}
	if value.Status != "rejected" && value.RejectedEvidenceDigest != "" && !digestPattern.MatchString(value.RejectedEvidenceDigest) {
		return errors.New("rejected evidence digest is invalid")
	}
	for _, decision := range value.DecisionHistory {
		if strings.TrimSpace(decision.State) == "" || strings.TrimSpace(decision.Rationale) == "" || !digestPattern.MatchString(decision.EvidenceDigest) {
			return errors.New("insight decision history requires state, rationale, and evidence digest")
		}
		if _, err := time.Parse(time.RFC3339Nano, decision.DecidedAt); err != nil {
			return errors.New("insight decision timestamp must be RFC3339")
		}
	}
	switch value.Priority {
	case "low", "medium", "high", "critical":
	default:
		return errors.New("invalid insight priority")
	}
	return nil
}

// InsightEvidenceDigest binds every semantic part of the evidence graph. It is
// deterministic across collection ordering, but changes when an observation,
// comparison verdict, tradeoff, membership, or pinned revision changes.
func InsightEvidenceDigest(observationIDs, comparisonIDs []string, observations map[string]Observation, comparisons map[string]Comparison) string {
	observationSet := make(map[string]bool, len(observationIDs))
	for _, id := range observationIDs {
		observationSet[id] = true
	}
	comparisonIDs = uniqueSorted(comparisonIDs)
	for _, comparisonID := range comparisonIDs {
		for _, id := range comparisons[comparisonID].ObservationIDs {
			observationSet[id] = true
		}
	}
	observationIDs = make([]string, 0, len(observationSet))
	for id := range observationSet {
		observationIDs = append(observationIDs, id)
	}
	sort.Strings(observationIDs)

	var builder strings.Builder
	for _, id := range observationIDs {
		item := observations[id]
		fmt.Fprintf(&builder, "observation\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\n", id, item.SourceID, item.RunID, item.StableKey, item.Status, item.What, item.SupersededByID)
		fmt.Fprintf(&builder, "first-revision\x00%s\x00%s\x00%s\n", item.FirstSeen.Kind, item.FirstSeen.Value, item.FirstSeen.ContentDigest)
		fmt.Fprintf(&builder, "last-revision\x00%s\x00%s\x00%s\n", item.LastSeen.Kind, item.LastSeen.Value, item.LastSeen.ContentDigest)
		for _, superseded := range uniqueSorted(item.SupersedesIDs) {
			fmt.Fprintf(&builder, "supersedes\x00%s\n", superseded)
		}
		vocabulary := append([]string(nil), item.Vocabulary...)
		sort.Strings(vocabulary)
		for _, term := range vocabulary {
			fmt.Fprintf(&builder, "vocabulary\x00%s\n", term)
		}
		evidence := append([]Evidence(nil), item.Evidence...)
		sort.Slice(evidence, func(i, j int) bool {
			left := evidence[i].Path + "\x00" + evidence[i].Locator + "\x00" + evidence[i].Digest
			right := evidence[j].Path + "\x00" + evidence[j].Locator + "\x00" + evidence[j].Digest
			return left < right
		})
		for _, pin := range evidence {
			fmt.Fprintf(&builder, "evidence\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\n", pin.RunID, pin.PackageDigest, pin.Path, pin.Locator, pin.Digest, pin.Revision.Kind, pin.Revision.Value, pin.Revision.ContentDigest)
		}
	}
	for _, id := range comparisonIDs {
		item := comparisons[id]
		fmt.Fprintf(&builder, "comparison\x00%s\x00%s\x00%s\x00%s\x00%s\x00%t\n", id, item.RunID, item.Subject, item.Verdict, item.Tradeoffs, item.Stale)
		members := uniqueSorted(item.ObservationIDs)
		for _, member := range members {
			pin := item.BasedOn[member]
			fmt.Fprintf(&builder, "member\x00%s\x00%s\x00%s\x00%s\n", member, pin.Kind, pin.Value, pin.ContentDigest)
		}
	}
	return source.Digest([]byte(builder.String()))
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func ValidateCanonical(path string, data []byte) error {
	switch {
	case strings.Contains(path, "/runs/"):
		_, err := ParseRun(data)
		return err
	case strings.Contains(path, "/observations/"), strings.Contains(path, "/findings/"):
		_, err := ParseObservation(data)
		return err
	case strings.HasPrefix(path, "distill/comparisons/"):
		_, err := ParseComparison(data)
		return err
	case strings.Contains(path, "/insights/"):
		_, err := ParseInsight(data)
		return err
	default:
		return nil
	}
}

func SortRunCollections(run *Run) {
	sort.Slice(run.ChangedResources, func(i, j int) bool { return run.ChangedResources[i].Path < run.ChangedResources[j].Path })
	sort.Slice(run.Coverage, func(i, j int) bool { return run.Coverage[i].Resource < run.Coverage[j].Resource })
	sort.Strings(run.FindingIDs)
	sort.Strings(run.ComparisonIDs)
	sort.Strings(run.InsightIDs)
}

func strict(data []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("multiple YAML documents are not allowed")
	}
	return nil
}
func validateRevision(value source.Revision) error {
	if value.Value == "" || !digestPattern.MatchString(value.ContentDigest) || value.ObservedAt.IsZero() {
		return errors.New("incomplete revision")
	}
	switch value.Kind {
	case "git-commit", "git-tree", "filesystem-snapshot", "content-digest", "declared-version":
	default:
		return errors.New("unsupported revision kind")
	}
	return nil
}
func safeEvidencePath(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.HasPrefix(value, "../") && !strings.Contains(value, `\\`) && !strings.HasPrefix(value, "/")
}
