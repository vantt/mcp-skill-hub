package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

func routingRepoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

// TestRoutingEvalReportSnapshot pins the full leave-one-out report on the
// golden corpus. Set SKILLHUB_UPDATE_SNAPSHOT=1 to rewrite the expected file.
func TestRoutingEvalReportSnapshot(t *testing.T) {
	repoRoot := routingRepoRoot(t)
	root := materializeRoutingWorkspace(t, repoRoot)
	report, err := EvaluateRouting(t.Context(), RoutingEvalOptions{
		WorkspacePath: root,
		NoSkillFile:   filepath.Join(repoRoot, "testdata", "routing", "no-skill-v1.yaml"),
	})
	if err != nil {
		t.Fatalf("EvaluateRouting: %v", err)
	}
	got, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join(repoRoot, "testdata", "routing", "eval-report-v1.json")
	if os.Getenv("SKILLHUB_UPDATE_SNAPSHOT") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("routing eval report drifted from %s", path)
	}
}

// stubCatalog is an in-memory library whose search matches any skill that
// shares a term with the query.
type stubCatalog struct {
	skills []resolverpkg.Skill
	loads  int
}

func (c *stubCatalog) Snapshot() string { return "stub" }

func (c *stubCatalog) Skills(context.Context) ([]resolverpkg.Skill, error) {
	c.loads++
	return append([]resolverpkg.Skill(nil), c.skills...), nil
}

func (c *stubCatalog) Search(_ context.Context, query string, limit int) ([]resolverpkg.SearchHit, error) {
	var queryTerms [][]string
	for _, token := range resolverpkg.Tokenize(query) {
		queryTerms = append(queryTerms, splitTerms(token))
	}
	var hits []resolverpkg.SearchHit
	for _, skill := range c.skills {
		if skill.Status == "active" && matchesAnyToken(indexedTerms(skill), queryTerms) {
			hits = append(hits, resolverpkg.SearchHit{SkillID: skill.ID})
		}
	}
	return hits, nil
}

func stubSkill(id, status string, triggers ...string) resolverpkg.Skill {
	return resolverpkg.Skill{
		ID: id, CollectionID: "core", Name: id, Description: "Skill " + id, Status: status,
		Triggers: triggers, NotFor: []string{"unrelated tasks"}, MinScope: "single_step",
	}
}

func skillIDs(skills []resolverpkg.Skill) []string {
	ids := make([]string, 0, len(skills))
	for _, skill := range skills {
		ids = append(ids, skill.ID)
	}
	return ids
}

func hitIDs(hits []resolverpkg.SearchHit) []string {
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.SkillID)
	}
	return ids
}

func TestOverlayCatalogSkills(t *testing.T) {
	base := &stubCatalog{skills: []resolverpkg.Skill{
		stubSkill("alpha", "active", "alpha work"),
		stubSkill("bravo", "active", "bravo work"),
		stubSkill("charlie", "active", "charlie work"),
	}}
	overlay := newOverlayCatalog(base,
		map[string]resolverpkg.Skill{
			"bravo": stubSkill("bravo", "active", "bravo changed"),
			"aaa":   stubSkill("aaa", "draft", "draft work"),
		},
		map[string]struct{}{"charlie": {}},
	)
	skills, err := overlay.Skills(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := skillIDs(skills), []string{"aaa", "alpha", "bravo"}; !slices.Equal(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	if skills[0].Status != "active" {
		t.Fatalf("added skill status = %q, want active", skills[0].Status)
	}
	if skills[2].Triggers[0] != "bravo changed" {
		t.Fatalf("replaced skill kept old triggers: %v", skills[2].Triggers)
	}
	if _, err := overlay.Skills(t.Context()); err != nil {
		t.Fatal(err)
	}
	if base.loads != 1 {
		t.Fatalf("base skills decoded %d times, want 1", base.loads)
	}
}

func TestOverlayCatalogSearch(t *testing.T) {
	base := &stubCatalog{skills: []resolverpkg.Skill{
		stubSkill("alpha", "active", "alpha work"),
		stubSkill("bravo", "active", "bravo work"),
		stubSkill("charlie", "active", "charlie work"),
	}}
	renamed := stubSkill("bravo", "active", "zulu words")
	renamed.Name, renamed.Description = "papa", "Papa skill"
	overlay := newOverlayCatalog(base,
		map[string]resolverpkg.Skill{
			"bravo": renamed,
			"aaa":   stubSkill("aaa", "draft", "draft zulu"),
		},
		map[string]struct{}{"charlie": {}},
	)
	search := func(query string) []string {
		t.Helper()
		hits, err := overlay.Search(t.Context(), query, 10)
		if err != nil {
			t.Fatal(err)
		}
		return hitIDs(hits)
	}
	// The base index still lists the replaced skill by its old triggers; the
	// overlay drops it, and drops the removed skill.
	if got := search("bravo charlie"); len(got) != 0 {
		t.Fatalf("replaced and removed skills leaked into hits: %v", got)
	}
	// The overlay version is found by its new triggers, the added one too, and
	// the result is stable and free of duplicates.
	want := []string{"aaa", "bravo"}
	for range 3 {
		if got := search("zulu zulu"); !slices.Equal(got, want) {
			t.Fatalf("hits = %v, want %v", got, want)
		}
	}
	if got := search("alpha"); !slices.Equal(got, []string{"alpha"}) {
		t.Fatalf("base hit lost: %v", got)
	}
	if got := search("the"); len(got) != 0 {
		t.Fatalf("stop word matched: %v", got)
	}
}

func TestOverlayCatalogComposesUnderLeaveOneOut(t *testing.T) {
	draft := stubSkill("draft", "draft", "draft work")
	draft.Examples = []string{"first phrase", "second phrase"}
	overlay := newOverlayCatalog(&stubCatalog{skills: []resolverpkg.Skill{stubSkill("alpha", "active", "alpha work")}},
		map[string]resolverpkg.Skill{"draft": draft}, nil)
	c := selfResolutionCase{SkillID: "draft", Kind: selfCaseExample, Index: 0, Phrase: "first phrase"}
	skills, err := c.withoutPhrase(overlay).Skills(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range skills {
		if skill.ID == "draft" && !slices.Equal(skill.Examples, []string{"second phrase"}) {
			t.Fatalf("examples = %v, want only the second phrase", skill.Examples)
		}
	}
	hits, err := c.withoutPhrase(overlay).Search(t.Context(), "draft", 10)
	if err != nil || !slices.Equal(hitIDs(hits), []string{"draft"}) {
		t.Fatalf("search through leave-one-out = %v, %v", hits, err)
	}
	// The overlay itself is unchanged by the wrapper.
	again, _ := overlay.Skills(t.Context())
	for _, skill := range again {
		if skill.ID == "draft" && len(skill.Examples) != 2 {
			t.Fatalf("leave-one-out modified the shared skills: %v", skill.Examples)
		}
	}
}

func TestOwnCasesAndRequest(t *testing.T) {
	skill := stubSkill("alpha", "active", "alpha work")
	skill.Examples = []string{"e0", "e1"}
	skill.CounterExamples = []string{"c0"}
	skill.Operations = []string{"review", "write"}
	cases := ownCases(skill)
	if len(cases) != 3 || cases[0].Kind != selfCaseExample || cases[2].Kind != selfCaseCounter || cases[2].Index != 0 {
		t.Fatalf("cases = %+v", cases)
	}
	req := selfResolutionRequest(cases[1], skill)
	if req.RequestID != "routing-eval-alpha-ex-1" || req.Operation != "review" || req.Task.Description != "e1" || req.Task.Scope != "single_step" {
		t.Fatalf("request = %+v", req)
	}
	if got := selfResolutionRequest(cases[2], skill).RequestID; got != "routing-eval-alpha-cx-0" {
		t.Fatalf("counter request id = %q", got)
	}
}

// openGoldenCatalog opens the golden workspace's current generation.
func openGoldenCatalog(t *testing.T) (*catalog.View, resolverpkg.Catalog, resolverpkg.Policy, string) {
	t.Helper()
	root := materializeRoutingWorkspace(t, routingRepoRoot(t))
	view, err := catalog.ObserveCurrentLocked(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { view.Close() })
	database, err := view.DB(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	sqlite, err := resolverpkg.NewSQLiteCatalog(database, view.Pointer.CatalogSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := generationPolicy(t.Context(), view)
	if err != nil {
		t.Fatal(err)
	}
	return view, memoSkillsCatalog{Catalog: sqlite, view: view}, policy, root
}

// The engine path over an unchanged overlay must classify every own case like
// the served resolver path does.
func TestSelfResolutionEngineMatchesServedResolver(t *testing.T) {
	view, base, policy, root := openGoldenCatalog(t)
	skills, err := base.Skills(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	overlay := newOverlayCatalog(base, nil, nil)
	svc := ResolverService{Policy: policy}
	compared := 0
	for _, skill := range skills {
		for _, c := range ownCases(skill) {
			served, err := resolveSelfCase(t.Context(), root, view, svc, c, skill)
			if err != nil {
				t.Fatal(err)
			}
			got, err := engineResolver(c.withoutPhrase(overlay), policy)(t.Context(), selfResolutionRequest(c, skill))
			if err != nil {
				t.Fatal(err)
			}
			gotPrimary, wantPrimary := "", ""
			if got.Primary != nil {
				gotPrimary = got.Primary.ID
			}
			if served.Primary != nil {
				wantPrimary = served.Primary.ID
			}
			if got.Status != served.Status || gotPrimary != wantPrimary {
				t.Fatalf("%s %s #%d: engine %s/%s, served %s/%s", c.SkillID, c.Kind, c.Index, got.Status, gotPrimary, served.Status, wantPrimary)
			}
			compared++
		}
	}
	if compared < 100 {
		t.Fatalf("compared only %d cases", compared)
	}
}

func TestSelfResolutionOverlayChangesResolution(t *testing.T) {
	_, base, policy, _ := openGoldenCatalog(t)
	skills, err := base.Skills(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var owner resolverpkg.Skill
	var phrase string
	for _, skill := range skills {
		if len(skill.Examples) > 1 {
			owner, phrase = skill, skill.Examples[0]
			break
		}
	}
	if phrase == "" {
		t.Fatal("no golden skill with examples")
	}
	c := selfResolutionCase{SkillID: owner.ID, Kind: selfCaseExample, Index: 0, Phrase: phrase}
	request := selfResolutionRequest(c, owner)

	// Removing the owner makes its own example resolve elsewhere.
	removed := newOverlayCatalog(base, nil, map[string]struct{}{owner.ID: {}})
	response, err := engineResolver(removed, policy)(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Primary != nil && response.Primary.ID == owner.ID {
		t.Fatalf("removed skill %s still resolved", owner.ID)
	}

	// An added draft that restates the owner is a candidate for the phrase.
	draft := owner
	draft.ID, draft.Name, draft.Status = "zz-added-draft", "zz-added-draft", "draft"
	draft.Examples, draft.CounterExamples = nil, nil
	added := newOverlayCatalog(base, map[string]resolverpkg.Skill{draft.ID: draft}, nil)
	response, err = engineResolver(c.withoutPhrase(added), policy)(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(response.TopKSkillIDs, draft.ID) {
		t.Fatalf("top skills %v do not include the added draft", response.TopKSkillIDs)
	}
}

func TestSelfResolutionDeterministic(t *testing.T) {
	_, base, policy, _ := openGoldenCatalog(t)
	skills, _ := base.Skills(t.Context())
	overlay := newOverlayCatalog(base, nil, nil)
	run := func() []string {
		var out []string
		for _, skill := range skills[:6] {
			for _, c := range ownCases(skill) {
				response, err := engineResolver(c.withoutPhrase(overlay), policy)(t.Context(), selfResolutionRequest(c, skill))
				if err != nil {
					t.Fatal(err)
				}
				out = append(out, string(response.Status)+":"+strings.Join(response.TopKSkillIDs, ","))
			}
		}
		return out
	}
	if first, second := run(), run(); !slices.Equal(first, second) {
		t.Fatal("repeated runs differ")
	}
}

// selfResolutionRun resolves every own case of every active skill through the
// engine over an unchanged overlay and returns the resolve count and elapsed
// time.
func selfResolutionRun(t *testing.T, base resolverpkg.Catalog, policy resolverpkg.Policy) (int, time.Duration) {
	t.Helper()
	skills, err := base.Skills(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	overlay := newOverlayCatalog(base, nil, nil)
	resolves := 0
	started := time.Now()
	for _, skill := range skills {
		for _, c := range ownCases(skill) {
			if _, err := engineResolver(c.withoutPhrase(overlay), policy)(t.Context(), selfResolutionRequest(c, skill)); err != nil {
				t.Fatal(err)
			}
			resolves++
		}
	}
	return resolves, time.Since(started)
}

// TestSelfResolutionPerformance measures the cost of one resolve against the
// library size, which fixes maxSelfResolutionWork. It is wall-clock and machine
// dependent, so it runs only with SKILLHUB_PERF=1.
func TestSelfResolutionPerformance(t *testing.T) {
	if os.Getenv("SKILLHUB_PERF") != "1" {
		t.Skip("wall-clock measurements run only with SKILLHUB_PERF=1 on an otherwise idle machine")
	}
	if testing.Short() {
		t.Skip("performance measurements are excluded from short tests")
	}
	report := func(label string, skills int, base resolverpkg.Catalog, policy resolverpkg.Policy) {
		selfResolutionRun(t, base, policy) // warm the memoized skills and the page cache
		resolves, elapsed := selfResolutionRun(t, base, policy)
		perResolve := elapsed / time.Duration(resolves)
		t.Logf("%s: %d skills, %d resolves in %s: %s per resolve, %.1f us per resolve x skill, work %d",
			label, skills, resolves, elapsed, perResolve, float64(perResolve.Microseconds())/float64(skills), resolves*skills)
		if work := resolves * skills; work > maxSelfResolutionWork {
			t.Logf("%s: a full run exceeds maxSelfResolutionWork (%d)", label, maxSelfResolutionWork)
		}
	}

	_, golden, policy, _ := openGoldenCatalog(t)
	goldenSkills, _ := golden.Skills(t.Context())
	report("golden", len(goldenSkills), golden, policy)

	const synthetic = 256
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < synthetic; index++ {
		id := fmt.Sprintf("synthetic-skill-%03d", index)
		var examples strings.Builder
		for example := 0; example < 5; example++ {
			fmt.Fprintf(&examples, "    - review synthetic target %03d variant %d for the team\n", index, example)
		}
		meta := fmt.Sprintf("schema_version: 1\nid: %s\nname: Synthetic Skill %03d\nstatus: active\ndescription: Review synthetic target %03d.\nquality:\n  reviewed: true\nrouting:\n  triggers: [review synthetic target %03d]\n  operations: [review]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n  examples:\n%s  counter_examples:\n    - write marketing prose about target %03d\n    - deploy target %03d to production\n", id, index, index, index, examples.String(), index, index)
		writeResolveBenchmarkFile(t, root, fmt.Sprintf("skills/benchmark/%s/skill.meta.yaml", id), meta)
		writeResolveBenchmarkFile(t, root, fmt.Sprintf("skills/benchmark/%s/SKILL.md", id), fmt.Sprintf("---\nname: %s\ndescription: Review synthetic target %03d.\n---\n\n# Synthetic %03d\n", id, index, index))
	}
	if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	view, err := catalog.ObserveCurrentLocked(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Close()
	database, err := view.DB(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	sqlite, err := resolverpkg.NewSQLiteCatalog(database, view.Pointer.CatalogSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	syntheticPolicy, err := generationPolicy(t.Context(), view)
	if err != nil {
		t.Fatal(err)
	}
	report("synthetic", synthetic, memoSkillsCatalog{Catalog: sqlite, view: view}, syntheticPolicy)
}
