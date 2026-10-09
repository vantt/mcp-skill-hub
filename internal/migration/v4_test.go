package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/distill"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestConvertPhase0DistillYAML(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/phase0-distill.yaml")
	if err != nil {
		t.Fatalf("read testdata/phase0-distill.yaml: %v", err)
	}

	doc, err := ConvertSkillDistillYAML(data, "test-audit")
	if err != nil {
		t.Fatalf("convert phase-0 distill yaml failed: %v", err)
	}

	// 1. Goal converted to string purpose
	if doc.Goal == "" {
		t.Fatal("expected non-empty goal")
	}
	if len(doc.Goal) < 20 {
		t.Fatalf("expected meaningful goal, got %q", doc.Goal)
	}

	// 2. Cursors converted to []distill.Cursor with 40-hex SHAs
	if len(doc.Cursors) != 2 {
		t.Fatalf("expected 2 cursors, got %d", len(doc.Cursors))
	}
	for _, c := range doc.Cursors {
		if len(c.Commit) != 40 {
			t.Errorf("cursor %s commit is not 40 hex chars: %s", c.SourceID, c.Commit)
		}
	}

	// 3. Coverage converted, coverage gaps survive
	if len(doc.Coverage) != 32 {
		t.Fatalf("expected 32 coverage items (21 analyzed + 11 skipped), got %d", len(doc.Coverage))
	}
	skippedCount := 0
	for _, cov := range doc.Coverage {
		if cov.Status == "skipped" {
			skippedCount++
			if cov.Reason == "" {
				t.Errorf("coverage gap %s lost reason", cov.Resource)
			}
		}
	}
	if skippedCount != 11 {
		t.Fatalf("expected 11 coverage gaps, got %d", skippedCount)
	}

	// 4. Lessons converted, all 42 lessons present
	if len(doc.Lessons) != 42 {
		t.Fatalf("expected 42 lessons, got %d", len(doc.Lessons))
	}

	lessonMap := make(map[string]distill.Lesson)
	for _, l := range doc.Lessons {
		lessonMap[l.Key] = l
	}

	// 5. Human decisions survive
	plannedKeys := []string{"ratchet-over-doctrine", "mock-isolation-must-be-declared", "ban-ships-with-replacement"}
	for _, k := range plannedKeys {
		l, ok := lessonMap[k]
		if !ok {
			t.Fatalf("missing lesson %s", k)
		}
		if l.Decision.Status != "planned" {
			t.Errorf("lesson %s status = %q, want planned", k, l.Decision.Status)
		}
		if len(l.Decision.SeenWhere) == 0 {
			t.Errorf("lesson %s lost seen_where", k)
		}
		if len(l.Decision.SeenWhere) != len(l.Where) {
			t.Errorf("lesson %s seen_where length %d != where length %d", k, len(l.Decision.SeenWhere), len(l.Where))
		}
	}

	// Candidate lessons remain candidate
	candLesson, ok := lessonMap["mocks-mirror-the-real-shape"]
	if !ok || candLesson.Decision.Status != "candidate" {
		t.Fatalf("expected candidate lesson for mocks-mirror-the-real-shape, got %#v", candLesson)
	}

	// 6. Reopen on new evidence
	rod := lessonMap["ratchet-over-doctrine"]
	if rod.Decision.Status != "planned" {
		t.Fatalf("expected planned status, got %s", rod.Decision.Status)
	}

	// Same evidence -> stays planned
	err = doc.ApplyLesson(rod, time.Now().UTC())
	if err != nil {
		t.Fatalf("re-applying same lesson failed: %v", err)
	}
	for _, l := range doc.Lessons {
		if l.Key == "ratchet-over-doctrine" && l.Decision.Status != "planned" {
			t.Fatalf("same evidence altered decision: got %s, want planned", l.Decision.Status)
		}
	}

	// New evidence -> reopens to candidate
	rodWithNew := rod
	rodWithNew.Where = append([]string{}, rod.Where...)
	rodWithNew.Where = append(rodWithNew.Where, "usage:cs_reopen_test")
	err = doc.ApplyLesson(rodWithNew, time.Now().UTC())
	if err != nil {
		t.Fatalf("applying lesson with new evidence failed: %v", err)
	}
	for _, l := range doc.Lessons {
		if l.Key == "ratchet-over-doctrine" {
			if l.Decision.Status != "candidate" {
				t.Fatalf("new evidence did not reopen lesson: got %s, want candidate", l.Decision.Status)
			}
			if len(l.Where) != len(rod.Where)+1 {
				t.Fatalf("expected %d where entries, got %d", len(rod.Where)+1, len(l.Where))
			}
		}
	}

	// 7. Validate full document
	if err := distill.ValidateDocument(doc); err != nil {
		t.Fatalf("ValidateDocument failed on converted document: %v", err)
	}
}

func TestPrePhase3FixtureMigrationV3ToV4(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")

	// 1. Create a schema 3 workspace
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	// Write schema-version 3
	_ = os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("3\n"), 0o644)

	// Create skill
	skillDir := filepath.Join(root, "skills", "default", "sample-skill")
	_ = os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Sample Skill\n\nSample description.\n"), 0o644)
	_ = os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte("schema_version: 1\nid: sample-skill\nstatus: active\nrouting:\n  triggers: [sample]\n  not_for: [other]\n  min_scope: single_step\n"), 0o644)

	// 2. Add legacy distill/ and sources/ artifacts
	// a. sources/intake/
	_ = os.MkdirAll(filepath.Join(root, "sources", "intake"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "sources", "intake", "intake-1.yaml"), []byte("url: https://example.com/intake\n"), 0o644)

	// b. sources/skills/LINK-*.yaml
	_ = os.MkdirAll(filepath.Join(root, "sources", "skills"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "sources", "skills", "LINK-sample-skill--src-1.yaml"), []byte("skill_id: sample-skill\nsource_id: src-1\n"), 0o644)

	// c. distill/runs/
	_ = os.MkdirAll(filepath.Join(root, "distill", "runs"), 0o755)
	runContent := `id: run-01
coverage:
  - resource: docs/analyzed.md
    status: analyzed
  - resource: docs/gap.md
    status: skipped
    reason: "not relevant to current scope"
  - resource: docs/unreadable.bin
    status: unreadable
    reason: "binary file"
`
	_ = os.WriteFile(filepath.Join(root, "distill", "runs", "run-01.yaml"), []byte(runContent), 0o644)

	// d. distill/sources/src-1/findings/ (observations, including tombstone)
	_ = os.MkdirAll(filepath.Join(root, "distill", "sources", "src-1", "findings"), 0o755)
	obsActive := `id: OBS-src-1--active-finding
source_id: src-1
stable_key: active-finding
status: active
what: Active knowledge pattern
evidence:
  - path: docs/analyzed.md
    revision:
      value: 1234567890123456789012345678901234567890
`
	obsTombstone := `id: OBS-src-1--removed-finding
source_id: src-1
stable_key: removed-finding
status: removed
what: Obsolete removed knowledge pattern
evidence:
  - path: docs/old.md
    revision:
      value: 1234567890123456789012345678901234567890
`
	_ = os.WriteFile(filepath.Join(root, "distill", "sources", "src-1", "findings", "obs-01.yaml"), []byte(obsActive), 0o644)
	_ = os.WriteFile(filepath.Join(root, "distill", "sources", "src-1", "findings", "obs-02.yaml"), []byte(obsTombstone), 0o644)

	// e. distill/insights/
	_ = os.MkdirAll(filepath.Join(root, "distill", "insights"), 0o755)
	insAccepted := `id: INS-sample-skill--accepted-item
skill_id: sample-skill
stable_key: accepted-item
status: accepted
recommendation: Adopt robust timeout pattern
rationale: Prevents hangs in distributed workers
`
	insRejected := `id: INS-sample-skill--rejected-item
skill_id: sample-skill
stable_key: rejected-item
status: rejected
recommendation: Naive sleep retry
rationale: Flaky pattern
decision_rationale: Rejected by curator because sleep creates flake
`
	_ = os.WriteFile(filepath.Join(root, "distill", "insights", "ins-01.yaml"), []byte(insAccepted), 0o644)
	_ = os.WriteFile(filepath.Join(root, "distill", "insights", "ins-02.yaml"), []byte(insRejected), 0o644)

	// 3. Run migration v3 to v4
	reg := DefaultRegistry()
	proposal, err := reg.Preview(root, CurrentVersion)
	if err != nil {
		t.Fatalf("registry preview failed: %v", err)
	}
	if proposal.SourceVersion != 3 || proposal.TargetVersion != 4 {
		t.Fatalf("unexpected versions: source=%d, target=%d", proposal.SourceVersion, proposal.TargetVersion)
	}
	receipt, err := mutation.ConfirmMutation(root, proposal.Mutation, mutation.Confirmation{
		ProposalID:          proposal.ID,
		ProposalDigest:      proposal.Digest,
		BaseCatalogSnapshot: proposal.BaseSnapshot,
	})
	if err != nil {
		t.Fatalf("mutation confirm failed: %v", err)
	}
	if receipt.OperationID == "" {
		t.Fatal("expected non-empty operation ID")
	}
	// 4. Verify version marker on disk
	newVer, err := DetectVersion(root)
	if err != nil || newVer != 4 {
		t.Fatalf("expected detected version 4, got %d, %v", newVer, err)
	}

	// 5. Verify legacy directories deleted
	legacyPaths := []string{
		filepath.Join(root, "sources", "intake", "intake-1.yaml"),
		filepath.Join(root, "sources", "skills", "LINK-sample-skill--src-1.yaml"),
		filepath.Join(root, "distill", "runs", "run-01.yaml"),
		filepath.Join(root, "distill", "insights", "ins-01.yaml"),
		filepath.Join(root, "distill", "insights", "ins-02.yaml"),
		filepath.Join(root, "distill", "sources", "src-1", "findings", "obs-01.yaml"),
		filepath.Join(root, "distill", "sources", "src-1", "findings", "obs-02.yaml"),
	}
	for _, lp := range legacyPaths {
		if _, statErr := os.Stat(lp); !os.IsNotExist(statErr) {
			t.Errorf("legacy file was not deleted: %s", lp)
		}
	}

	// 6. Verify .meta/distill.yaml created for sample-skill
	distillPath := filepath.Join(skillDir, ".meta", "distill.yaml")
	doc, err := distill.LoadDocument(distillPath)
	if err != nil {
		t.Fatalf("load migrated distill.yaml failed: %v", err)
	}

	// 7. Verify human decisions survive
	lessonMap := make(map[string]distill.Lesson)
	for _, l := range doc.Lessons {
		lessonMap[l.Key] = l
	}

	acc, ok := lessonMap["accepted-item"]
	if !ok || acc.Decision.Status != "planned" {
		t.Fatalf("accepted insight did not survive as planned lesson: %#v", acc)
	}
	if len(acc.Decision.SeenWhere) == 0 {
		t.Fatalf("accepted insight lost seen_where: %#v", acc)
	}

	rej, ok := lessonMap["rejected-item"]
	if !ok || rej.Decision.Status != "rejected" {
		t.Fatalf("rejected insight did not survive as rejected lesson: %#v", rej)
	}
	if rej.Decision.Reason != "Rejected by curator because sleep creates flake" {
		t.Fatalf("rejected insight lost decision rationale: %q", rej.Decision.Reason)
	}

	// 8. Verify tombstone survives
	tomb, ok := lessonMap["removed-finding"]
	if !ok || tomb.Decision.Status != "rejected" {
		t.Fatalf("tombstoned observation did not survive as rejected lesson: %#v", tomb)
	}
	if tomb.Decision.Reason != "tombstoned: upstream removed or superseded this finding" {
		t.Fatalf("tombstone lost reason: %q", tomb.Decision.Reason)
	}

	// 9. Verify coverage gaps survive
	var foundGap, foundUnreadable bool
	for _, c := range doc.Coverage {
		if c.Resource == "docs/gap.md" && c.Status == "skipped" && c.Reason == "not relevant to current scope" {
			foundGap = true
		}
		if c.Resource == "docs/unreadable.bin" && c.Status == "skipped" && c.Reason == "binary file" {
			foundUnreadable = true
		}
	}
	if !foundGap {
		t.Error("coverage gap docs/gap.md did not survive")
	}
	if !foundUnreadable {
		t.Error("coverage gap docs/unreadable.bin did not survive")
	}

	// 10. Verify reopen on new evidence
	if err := doc.ApplyLesson(acc, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	for _, l := range doc.Lessons {
		if l.Key == "accepted-item" && l.Decision.Status != "planned" {
			t.Fatalf("re-applying without new evidence changed status: %s", l.Decision.Status)
		}
	}

	accNew := acc
	accNew.Where = append([]string{}, acc.Where...)
	accNew.Where = append(accNew.Where, "usage:cs_new_case")
	if err := doc.ApplyLesson(accNew, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	for _, l := range doc.Lessons {
		if l.Key == "accepted-item" && l.Decision.Status != "candidate" {
			t.Fatalf("re-applying with new evidence did not reopen to candidate: %s", l.Decision.Status)
		}
	}
}

func TestGarbledInsightFileFailsClosedV4Preview(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, ".skillhub", "schema-version"), []byte("3\n"), 0o644)

	skillDir := filepath.Join(root, "skills", "default", "sample-skill")
	_ = os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Sample Skill\n\nSample description.\n"), 0o644)
	_ = os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte("schema_version: 1\nid: sample-skill\nstatus: active\nrouting:\n  triggers: [sample]\n  not_for: [other]\n  min_scope: single_step\n"), 0o644)

	// Create garbled insight file
	insightsDir := filepath.Join(root, "distill", "insights")
	_ = os.MkdirAll(insightsDir, 0o755)
	badFile := filepath.Join(insightsDir, "garbled-insight.yaml")
	_ = os.WriteFile(badFile, []byte("[unterminated yaml: {oops\n"), 0o644)

	reg := DefaultRegistry()
	_, err := reg.Preview(root, CurrentVersion)
	if err == nil {
		t.Fatal("expected error previewing v4 migration with garbled insight file, got nil")
	}
	if !strings.Contains(err.Error(), "garbled-insight.yaml") {
		t.Fatalf("expected error to name garbled-insight.yaml, got %v", err)
	}

	// Verify workspace is unchanged
	ver, err := DetectVersion(root)
	if err != nil || ver != 3 {
		t.Fatalf("workspace version changed: ver=%d, err=%v", ver, err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, ".meta", "distill.yaml")); !os.IsNotExist(err) {
		t.Fatal("workspace was modified: distill.yaml was created despite preview failure")
	}
}
