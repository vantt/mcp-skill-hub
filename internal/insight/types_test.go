package insight

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeApplicationProposalIsPersistedRestrictivelyAndExactly(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)
	proposal := RuntimeProposal{Version: 1, CreatedAt: now, ExpiresAt: now.Add(time.Hour), ID: "APP-fixture", Digest: "sha256:" + repeat("a", 64), InsightID: "INS-fixture", SkillID: "skill", BaseSnapshot: "sha256:" + repeat("b", 64), PathPins: []PathPin{{Path: "skills/core/skill/SKILL.md", Before: "sha256:" + repeat("c", 64), After: "sha256:" + repeat("d", 64)}}, Mappings: []SourceToLocalMapping{{ObservationID: "OBS-source--finding", ArtifactPath: "skills/core/skill/SKILL.md", Concept: "review-step"}}, FullDiff: "exact diff", MutationProposal: json.RawMessage(`{"exact":true}`)}
	if err := StoreRuntimeProposal(root, proposal); err != nil {
		t.Fatal(err)
	}
	if err := StoreRuntimeProposal(root, proposal); err == nil {
		t.Fatal("immutable proposal was overwritten")
	}
	path := filepath.Join(root, "runtime", "insight-proposals", "APP-fixture.json")
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, %v", info, err)
	}
	loaded, err := LoadRuntimeProposal(root, proposal.ID, now.Add(time.Minute))
	if err != nil || loaded.FullDiff != "exact diff" || string(loaded.MutationProposal) != `{"exact":true}` {
		t.Fatalf("loaded = %#v, %v", loaded, err)
	}
}

func TestOutcomeRequiresExplicitEvidence(t *testing.T) {
	value := Outcome{SchemaVersion: 1, ID: "OUT-1", IncorporationID: "INC-1", State: "confirmed", Note: "reviewed", RecordedAt: "2026-09-29T06:00:00Z"}
	if err := ValidateOutcome(value); err == nil {
		t.Fatal("outcome without explicit evidence was accepted")
	}
	value.Evidence = []string{"review:42"}
	if err := ValidateOutcome(value); err != nil {
		t.Fatal(err)
	}
}

func repeat(value string, count int) string {
	result := ""
	for index := 0; index < count; index++ {
		result += value
	}
	return result
}
