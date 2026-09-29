package distill

import (
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/source"
)

func TestCoverageSupportsCompleteConsultedAndRuledOutLedger(t *testing.T) {
	changed := []ChangedResource{{Path: "consulted.md", Status: "modified"}, {Path: "ruled.md", Status: "modified"}}
	coverage := []CoverageEntry{{Resource: "consulted.md", Status: "consulted", Reason: "Read for context; it made no claim."}, {Resource: "ruled.md", Status: "ruled_out_with_reason", Reason: "Generated content is outside source policy."}}
	if err := ValidateCoverage(changed, coverage); err != nil {
		t.Fatal(err)
	}
	coverage[1].Reason = ""
	if err := ValidateCoverage(changed, coverage); err == nil {
		t.Fatal("ruled-out coverage without a reason was accepted")
	}
}

func TestInsightEvidenceDigestBindsCompleteComparisonSemantics(t *testing.T) {
	revision := RevisionIdentity{Kind: "git-commit", Value: "abc", ContentDigest: source.Digest([]byte("revision"))}
	observation := Observation{ID: "OBS-source--retry", SourceID: "source", Status: "active", What: "Use bounded retry.", LastSeen: revision, Vocabulary: []string{"retry"}, Evidence: []Evidence{{RunID: "RUN-one", PackageDigest: source.Digest([]byte("package")), Path: "SKILL.md", Locator: "SKILL.md", Digest: source.Digest([]byte("bytes")), Revision: revision}}}
	observations := map[string]Observation{observation.ID: observation}
	comparison := Comparison{ID: "CMP-retry", Subject: "retry", ObservationIDs: []string{observation.ID}, Verdict: "convergent", Tradeoffs: "bounded cost", BasedOn: map[string]RevisionIdentity{observation.ID: revision}}
	comparisons := map[string]Comparison{comparison.ID: comparison}
	base := InsightEvidenceDigest(nil, []string{comparison.ID}, observations, comparisons)
	if base != InsightEvidenceDigest(nil, []string{comparison.ID}, observations, comparisons) {
		t.Fatal("digest is not deterministic")
	}
	changed := comparison
	changed.Tradeoffs = "different operational cost"
	comparisons[comparison.ID] = changed
	if base == InsightEvidenceDigest(nil, []string{comparison.ID}, observations, comparisons) {
		t.Fatal("materially changed comparison semantics did not change evidence digest")
	}
	comparisons[comparison.ID] = comparison
	observation.What = "Use jittered bounded retry."
	observations[observation.ID] = observation
	if base == InsightEvidenceDigest(nil, []string{comparison.ID}, observations, comparisons) {
		t.Fatal("changed comparison-member observation did not change evidence digest")
	}
}

func TestComparisonRequiresExactUniqueObservationBasis(t *testing.T) {
	revision := RevisionIdentity{Kind: "git-commit", Value: "abc", ContentDigest: source.Digest([]byte("abc"))}
	base := Comparison{SchemaVersion: 1, ID: "CMP-retry", RunID: "RUN-one", Subject: "retry", ObservationIDs: []string{"OBS-source--retry"}, Verdict: "convergent", BasedOn: map[string]RevisionIdentity{"OBS-source--retry": revision}}
	if err := ValidateComparison(base); err != nil {
		t.Fatal(err)
	}
	duplicate := base
	duplicate.ObservationIDs = []string{"OBS-source--retry", "OBS-source--retry"}
	if err := ValidateComparison(duplicate); err == nil {
		t.Fatal("duplicate observation identity was accepted")
	}
	extra := base
	extra.BasedOn = map[string]RevisionIdentity{"OBS-source--retry": revision, "OBS-source--other": revision}
	if err := ValidateComparison(extra); err == nil {
		t.Fatal("non-corresponding based_on entry was accepted")
	}
}
