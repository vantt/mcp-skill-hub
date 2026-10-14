package app

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
)

func goldenSkill(t *testing.T, base resolverpkg.Catalog, id string) resolverpkg.Skill {
	t.Helper()
	skills, err := base.Skills(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range skills {
		if skill.ID == id {
			return skill
		}
	}
	t.Fatalf("golden corpus has no skill %q", id)
	return resolverpkg.Skill{}
}

func lintFindingFor(findings []resolverpkg.LintFinding, code, skillID, other string) *resolverpkg.LintFinding {
	for i := range findings {
		if findings[i].Code == code && findings[i].SkillID == skillID && findings[i].OtherSkillID == other {
			return &findings[i]
		}
	}
	return nil
}

func treeNames(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		names = append(names, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func TestAffectedExamplesOwnMisses(t *testing.T) {
	view, base, policy, _ := openGoldenCatalog(t)
	old := goldenSkill(t, base, "browser-testing")
	stolen := goldenSkill(t, base, "backup-recovery").Examples[0]
	changed := old
	changed.Examples = append(append([]string(nil), old.Examples...), stolen)

	findings := checkSkillChanges(t.Context(), view, policy, []skillChange{{ID: old.ID, Old: &old, New: &changed}}, selfCheckChanges)
	miss := lintFindingFor(findings, "example_resolves_elsewhere", "browser-testing", "backup-recovery")
	if miss == nil || miss.Value != stolen {
		t.Fatalf("no miss for %q resolving to backup-recovery in %+v", stolen, findings)
	}
	if !strings.Contains(miss.Fix, "not_for entry on backup-recovery") {
		t.Fatalf("fix %q does not name the winner", miss.Fix)
	}
}

// A trigger added to one skill steals another skill's example: the finding
// names the changed skill, the owner and says the change did it.
func TestAffectedExamplesStealCase(t *testing.T) {
	view, base, policy, _ := openGoldenCatalog(t)
	old := goldenSkill(t, base, "backup-recovery")
	owner := goldenSkill(t, base, "browser-testing")
	phrase := owner.Examples[0]
	changed := old
	changed.Triggers = append(append([]string(nil), old.Triggers...), phrase)

	findings := checkSkillChanges(t.Context(), view, policy, []skillChange{{ID: old.ID, Old: &old, New: &changed}}, selfCheckChanges)
	taken := lintFindingFor(findings, "example_taken_by", "backup-recovery", "browser-testing")
	if taken == nil || taken.Value != phrase || taken.Field != "routing.examples" {
		t.Fatalf("no example_taken_by for %q in %+v", phrase, findings)
	}
	if !strings.Contains(taken.Message, "now resolves to \"backup-recovery\"") {
		t.Fatalf("message = %q", taken.Message)
	}
	if !strings.Contains(taken.Fix, "distinguish_from between backup-recovery and browser-testing") {
		t.Fatalf("fix = %q", taken.Fix)
	}

	// The unchanged skill is not blamed again: Old equal to New reports the
	// same cases as already taken.
	again := checkSkillChanges(t.Context(), view, policy, []skillChange{{ID: changed.ID, Old: &changed, New: &changed}}, selfCheckChanges)
	taken = lintFindingFor(again, "example_taken_by", "backup-recovery", "browser-testing")
	if taken == nil || !strings.Contains(taken.Message, "already resolves") {
		t.Fatalf("a change that holds before and after must say already: %+v", taken)
	}
}

func TestAffectedExamplesAmbiguityWording(t *testing.T) {
	view, base, policy, _ := openGoldenCatalog(t)
	old := goldenSkill(t, base, "api-design")
	phrase := goldenSkill(t, base, "authentication").Examples[0]
	changed := old
	changed.Triggers = append(append([]string(nil), old.Triggers...), phrase)

	findings := checkSkillChanges(t.Context(), view, policy, []skillChange{{ID: old.ID, Old: &old, New: &changed}}, selfCheckChanges)
	taken := lintFindingFor(findings, "example_taken_by", "api-design", "authentication")
	if taken == nil || !strings.Contains(taken.Message, "now needs a choice between \"api-design\" and \"authentication\"") {
		t.Fatalf("finding = %+v", taken)
	}
}

func TestAffectedExamplesRemovedSkill(t *testing.T) {
	view, base, policy, _ := openGoldenCatalog(t)
	old := goldenSkill(t, base, "backup-recovery")
	findings := checkSkillChanges(t.Context(), view, policy, []skillChange{{ID: old.ID, Old: &old}}, selfCheckChanges)
	if len(findings) != 0 {
		t.Fatalf("removing a skill resolves nothing of its own and reports %+v", findings)
	}
}

func TestAffectedExamplesAddedDraft(t *testing.T) {
	view, base, policy, _ := openGoldenCatalog(t)
	owner := goldenSkill(t, base, "browser-testing")
	draft := owner
	draft.ID, draft.Name, draft.Status = "zz-new-skill", "zz-new-skill", "draft"
	draft.Triggers = []string{owner.Examples[0]}
	draft.Examples = []string{"polish quartz countertops with diamond pads", "sharpen ceramic kitchen knives", "season a cast iron skillet"}
	draft.CounterExamples = nil

	findings := checkSkillChanges(t.Context(), view, policy, []skillChange{{ID: draft.ID, New: &draft}}, selfCheckChanges)
	own := lintFindingFor(findings, "example_resolves_elsewhere", "zz-new-skill", "browser-testing")
	if own == nil {
		t.Fatalf("the draft's own examples miss and must be reported: %+v", findings)
	}
	taken := lintFindingFor(findings, "example_taken_by", "zz-new-skill", "browser-testing")
	if taken == nil || strings.Contains(taken.Message, "already") {
		t.Fatalf("a draft added now cannot have held the example before: %+v", taken)
	}
}

func TestAffectedExamplesBound(t *testing.T) {
	view, base, policy, _ := openGoldenCatalog(t)
	old := goldenSkill(t, base, "browser-testing")
	changed := old
	for i := 0; i < 1500; i++ {
		changed.Examples = append(changed.Examples, "bulk example phrase number "+strings.Repeat("x", i%7))
	}
	findings := checkSkillChanges(t.Context(), view, policy, []skillChange{{ID: old.ID, Old: &old, New: &changed}}, selfCheckChanges)
	if len(findings) != 1 || findings[0].Code != "self_resolution_skipped" || findings[0].SkillID != "browser-testing" {
		t.Fatalf("an over-bound run must report one skipped finding: %+v", findings)
	}
}

func TestAffectedExamplesDeterministicAndSilent(t *testing.T) {
	view, base, policy, root := openGoldenCatalog(t)
	oldA := goldenSkill(t, base, "backup-recovery")
	oldB := goldenSkill(t, base, "api-design")
	newA, newB := oldA, oldB
	newA.Triggers = append(append([]string(nil), oldA.Triggers...), goldenSkill(t, base, "browser-testing").Examples[0])
	newB.Triggers = append(append([]string(nil), oldB.Triggers...), goldenSkill(t, base, "authentication").Examples[0])

	before := treeNames(t, root)
	forward := []skillChange{{ID: oldA.ID, Old: &oldA, New: &newA}, {ID: oldB.ID, Old: &oldB, New: &newB}}
	backward := []skillChange{forward[1], forward[0]}
	first := checkSkillChanges(t.Context(), view, policy, forward, selfCheckChanges)
	second := checkSkillChanges(t.Context(), view, policy, backward, selfCheckChanges)
	if len(first) == 0 || !reflect.DeepEqual(first, second) {
		t.Fatalf("runs differ or are empty:\n%+v\n%+v", first, second)
	}
	if after := treeNames(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("the check wrote files: before %d entries, after %d", len(before), len(after))
	}
}

// The full check over the golden library reports exactly the own-case misses
// the routing evaluation reports.
func TestAffectedExamplesFullMatchesRoutingEval(t *testing.T) {
	view, _, policy, _ := openGoldenCatalog(t)
	findings := checkSkillChanges(t.Context(), view, policy, nil, selfCheckFull)

	contents, err := os.ReadFile(filepath.Join(routingRepoRoot(t), "testdata", "routing", "eval-report-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report RoutingEvalReport
	if err := json.Unmarshal(contents, &report); err != nil {
		t.Fatal(err)
	}
	want := 0
	for _, failure := range report.Failures {
		if failure.Kind == "positive" || failure.Kind == "counter" {
			want++
		}
	}
	if len(findings) != want {
		t.Fatalf("full check reports %d findings, the routing evaluation %d failures", len(findings), want)
	}
	for _, finding := range findings {
		if finding.Code != "example_resolves_elsewhere" && finding.Code != "counter_example_resolves_to_skill" {
			t.Fatalf("full mode reports only own-case misses, got %+v", finding)
		}
	}
}
