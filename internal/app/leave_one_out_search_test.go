package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// leaveOneOutSkill is one skill of a small library built for these tests.
type leaveOneOutSkill struct {
	id       string
	trigger  string
	examples []string
}

// The ledger skill's first example shares no word with anything else the
// library indexes, so only that example can make a search for it find the skill.
var leaveOneOutLibrary = []leaveOneOutSkill{
	{id: "ledger-audit", trigger: "audit quarterly ledgers", examples: []string{
		"reconcile zymurgy brewery invoices",
		"audit the quarterly ledgers before closing",
		"check ledgers for duplicate postings",
	}},
	{id: "container-deploy", trigger: "deploy containers", examples: []string{
		"deploy the containers to staging",
		"roll out new container images",
		"ship containers to production",
	}},
}

const leakingPhrase = "reconcile zymurgy brewery invoices"

func buildLeaveOneOutLibrary(t *testing.T, skills []leaveOneOutSkill) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	for _, skill := range skills {
		var examples strings.Builder
		for _, example := range skill.examples {
			fmt.Fprintf(&examples, "    - %s\n", example)
		}
		meta := fmt.Sprintf("schema_version: 1\nid: %s\nname: %s\nstatus: active\ndescription: Help with %s.\nquality:\n  reviewed: true\nrouting:\n  triggers: [%s]\n  not_for: [write marketing prose]\n  min_scope: single_step\n  examples:\n%s  counter_examples:\n    - write marketing prose\n", skill.id, skill.id, skill.trigger, skill.trigger, examples.String())
		writeResolveBenchmarkFile(t, root, "skills/core/"+skill.id+"/skill.meta.yaml", meta)
		writeResolveBenchmarkFile(t, root, "skills/core/"+skill.id+"/SKILL.md", fmt.Sprintf("---\nname: %s\ndescription: Help with %s.\n---\n\n# %s\n", skill.id, skill.trigger, skill.id))
	}
	if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	return root
}

// openServedCatalog opens the current generation of root as the self-check
// sees it, with a leave-one-out index released when the test ends.
func openServedCatalog(t *testing.T, root string) (*catalog.View, resolverpkg.Catalog) {
	t.Helper()
	view, err := catalog.ObserveCurrentLocked(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { view.Close() })
	served, release, err := selfCheckCatalog(t.Context(), view)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return view, served
}

func caseOf(t *testing.T, base resolverpkg.Catalog, skillID, phrase string) selfResolutionCase {
	t.Helper()
	skills, err := base.Skills(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range skills {
		if skill.ID != skillID {
			continue
		}
		for _, c := range ownCases(skill) {
			if c.Kind == selfCaseExample && c.Phrase == phrase {
				return c
			}
		}
	}
	t.Fatalf("no example %q of %s", phrase, skillID)
	return selfResolutionCase{}
}

func searchIDs(t *testing.T, c resolverpkg.Catalog, query string) []string {
	t.Helper()
	hits, err := c.Search(t.Context(), query, 40)
	if err != nil {
		t.Fatal(err)
	}
	return hitIDs(hits)
}

// A phrase left out of its skill must not find that skill through the
// full-text index, whichever catalog answers the search for the owner.
func TestLeaveOneOutSearchDoesNotFindTheOwnerThroughTheLeftOutExample(t *testing.T) {
	_, served := openServedCatalog(t, buildLeaveOneOutLibrary(t, leaveOneOutLibrary))
	if got := searchIDs(t, served, leakingPhrase); !slices.Equal(got, []string{"ledger-audit"}) {
		t.Fatalf("served search for the phrase = %v, want only its owner", got)
	}
	c := caseOf(t, served, "ledger-audit", leakingPhrase)
	ledger := func(catalog resolverpkg.Catalog) resolverpkg.Skill {
		skills, err := catalog.Skills(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for _, skill := range skills {
			if skill.ID == "ledger-audit" {
				return skill
			}
		}
		t.Fatal("ledger-audit missing")
		return resolverpkg.Skill{}
	}(served)

	catalogs := map[string]resolverpkg.Catalog{
		"served index":                         served,
		"overlay that leaves the owner served": newOverlayCatalog(served, nil, nil),
		"overlay that replaces the owner":      newOverlayCatalog(served, map[string]resolverpkg.Skill{ledger.ID: ledger}, nil),
	}
	for name, base := range catalogs {
		t.Run(name, func(t *testing.T) {
			if got := searchIDs(t, c.withoutPhrase(base), leakingPhrase); len(got) != 0 {
				t.Fatalf("leave-one-out search = %v, want no hit", got)
			}
			// The rest of the owner's index still finds it.
			if got := searchIDs(t, c.withoutPhrase(base), "duplicate postings"); !slices.Equal(got, []string{"ledger-audit"}) {
				t.Fatalf("search by another example = %v, want the owner", got)
			}
			// The catalog itself still serves the example.
			if got := searchIDs(t, base, leakingPhrase); !slices.Equal(got, []string{"ledger-audit"}) {
				t.Fatalf("search after leave-one-out = %v, want the owner again", got)
			}
		})
	}
}

// Leave-one-out search must return what the served index would return for a
// library built without that example: the same hits, order and ranks.
func TestLeaveOneOutSearchMatchesALibraryBuiltWithoutTheExample(t *testing.T) {
	_, served := openServedCatalog(t, buildLeaveOneOutLibrary(t, leaveOneOutLibrary))
	without := slices.Clone(leaveOneOutLibrary)
	without[0].examples = without[0].examples[1:]
	_, rebuilt := openServedCatalog(t, buildLeaveOneOutLibrary(t, without))

	c := caseOf(t, served, "ledger-audit", leakingPhrase)
	for _, query := range []string{leakingPhrase, "audit ledgers and deploy containers", "quarterly brewery containers", "postings"} {
		got, err := c.withoutPhrase(served).Search(t.Context(), query, 40)
		if err != nil {
			t.Fatal(err)
		}
		want, err := rebuilt.Search(t.Context(), query, 40)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("query %q: leave-one-out hits %v, rebuilt library hits %v", query, got, want)
		}
	}
}

// The routing evaluation must replay a phrase with no full-text candidate when
// only the phrase itself could have led the resolver to its skill.
func TestRoutingEvalReplaysAPhraseWithoutItsOwnIndexEntry(t *testing.T) {
	root := buildLeaveOneOutLibrary(t, leaveOneOutLibrary)
	view, served := openServedCatalog(t, root)
	c := caseOf(t, served, "ledger-audit", leakingPhrase)
	skills, err := served.Skills(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	owner := skills[slices.IndexFunc(skills, func(s resolverpkg.Skill) bool { return s.ID == "ledger-audit" })]
	policy, err := generationPolicy(t.Context(), view)
	if err != nil {
		t.Fatal(err)
	}
	index := newExampleIndex(view)
	t.Cleanup(index.Close)
	response, err := selfCaseReplay{root: root, view: view, svc: ResolverService{Policy: policy}, index: index}.resolve(t.Context(), c, owner)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(response.Channels, "fts") || response.RetrievalCandidateCount != 0 {
		t.Fatalf("channels %v with %d candidates, want none: the phrase found its own skill", response.Channels, response.RetrievalCandidateCount)
	}
	if response.Primary != nil {
		t.Fatalf("primary = %s, want none", response.Primary.ID)
	}
}
