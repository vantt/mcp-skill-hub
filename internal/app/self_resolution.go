package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"

	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
)

const (
	selfCaseExample = "example"
	selfCaseCounter = "counter_example"
)

// maxSelfResolutionWork bounds one self-resolution run, counted in
// resolves x active skills. One resolve costs about 0.75 ms plus 9 us per
// active skill, measured with SKILLHUB_PERF=1 on the dev machine, with every
// search going through the leave-one-out index (the costliest path):
//
//	golden corpus, 42 skills:    1.13 ms per resolve, 27.0 us per resolve x skill
//	synthetic corpus, 256 skills: 3.04 ms per resolve, 11.9 us per resolve x skill
//
// At 60,000 a run takes about 1.6 s at 42 skills and 0.7 s at 256, so it stays
// under the 2 s target.
const maxSelfResolutionWork = 60000

// selfResolutionCase is one routing phrase of a skill, replayed against the
// library to see which skill the resolver picks for it.
type selfResolutionCase struct {
	SkillID string
	Kind    string // selfCaseExample or selfCaseCounter
	Index   int
	Phrase  string
}

// ownCases lists a skill's examples, then its counter-examples, in index order.
func ownCases(skill resolverpkg.Skill) []selfResolutionCase {
	cases := make([]selfResolutionCase, 0, len(skill.Examples)+len(skill.CounterExamples))
	for index, phrase := range skill.Examples {
		cases = append(cases, selfResolutionCase{SkillID: skill.ID, Kind: selfCaseExample, Index: index, Phrase: phrase})
	}
	for index, phrase := range skill.CounterExamples {
		cases = append(cases, selfResolutionCase{SkillID: skill.ID, Kind: selfCaseCounter, Index: index, Phrase: phrase})
	}
	return cases
}

// selfResolutionRequest builds the resolve request for one case of its owner:
// the owner's scope, first operation and the facts and capabilities it needs.
func selfResolutionRequest(c selfResolutionCase, owner resolverpkg.Skill) resolverpkg.Request {
	operation := ""
	if len(owner.Operations) > 0 {
		operation = owner.Operations[0]
	}
	tag := "ex"
	if c.Kind == selfCaseCounter {
		tag = "cx"
	}
	return resolverpkg.Request{
		SchemaVersion: "1",
		RequestID:     fmt.Sprintf("routing-eval-%s-%s-%d", c.SkillID, tag, c.Index),
		Task:          resolverpkg.Task{Description: c.Phrase, Scope: owner.MinScope},
		Operation:     operation,
		Context: resolverpkg.RequestContext{
			Facts:     buildPreconditionFacts(owner),
			Execution: resolverpkg.Execution{Capabilities: buildPreconditionCapabilities(owner)},
		},
	}
}

// withoutPhrase hides the case's own phrase from its owner, so the phrase is
// judged as a new input instead of being matched against itself.
func (c selfResolutionCase) withoutPhrase(base resolverpkg.Catalog) resolverpkg.Catalog {
	return leaveOneOutCatalog{Catalog: base, targetSkillID: c.SkillID, removeKind: c.Kind, removeIndex: c.Index}
}

// engineResolver resolves through the resolver engine directly, with a fresh
// cache per call. It writes no telemetry and builds no manifest, so it also
// resolves skills that are not servable yet.
func engineResolver(catalog resolverpkg.Catalog, policy resolverpkg.Policy) func(context.Context, resolverpkg.Request) (resolverpkg.Response, error) {
	return func(ctx context.Context, request resolverpkg.Request) (resolverpkg.Response, error) {
		engine, err := resolverpkg.New(catalog, policy, resolverpkg.NewCache(1))
		if err != nil {
			return resolverpkg.Response{}, err
		}
		return engine.Resolve(ctx, request)
	}
}

// overlayCatalog is the served library with some skills swapped in: replaced
// ones carry their changed version, added ones are new, removed ones are gone.
// A skill id in replace that the base lacks is an addition.
type overlayCatalog struct {
	resolverpkg.Catalog
	replace map[string]resolverpkg.Skill
	remove  map[string]struct{}

	once    sync.Once
	skills  []resolverpkg.Skill
	loadErr error
	terms   map[string]map[string]struct{} // overlay skill id -> indexed terms, active only
}

func newOverlayCatalog(base resolverpkg.Catalog, replace map[string]resolverpkg.Skill, remove map[string]struct{}) *overlayCatalog {
	return &overlayCatalog{Catalog: base, replace: replace, remove: remove}
}

// Skills decodes the base once per overlay. The result is shared: callers must
// not modify it.
func (c *overlayCatalog) Skills(ctx context.Context) ([]resolverpkg.Skill, error) {
	c.once.Do(func() { c.skills, c.loadErr = c.load(ctx) })
	return c.skills, c.loadErr
}

func (c *overlayCatalog) load(ctx context.Context) ([]resolverpkg.Skill, error) {
	base, err := c.Catalog.Skills(ctx)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(base))
	skills := make([]resolverpkg.Skill, 0, len(base)+len(c.replace))
	for _, skill := range base {
		known[skill.ID] = true
		if _, gone := c.remove[skill.ID]; gone {
			continue
		}
		if replacement, ok := c.replace[skill.ID]; ok {
			skill = replacement
		}
		skills = append(skills, skill)
	}
	for id, skill := range c.replace {
		if known[id] {
			continue
		}
		if _, gone := c.remove[id]; gone {
			continue
		}
		skill.Status = "active"
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].ID < skills[j].ID })
	c.terms = make(map[string]map[string]struct{}, len(c.replace))
	for _, skill := range skills {
		if _, overlaid := c.replace[skill.ID]; overlaid && skill.Status == "active" {
			c.terms[skill.ID] = indexedTerms(skill)
		}
	}
	return skills, nil
}

// Search returns the base hits without replaced or removed skills, then every
// overlay skill that shares a token with the query. The overlay side has no
// BM25 order and no truncation: it over-approximates the full-text index, and
// the resolver only uses hits as a candidate set.
func (c *overlayCatalog) Search(ctx context.Context, query string, limit int) ([]resolverpkg.SearchHit, error) {
	baseHits, err := c.Catalog.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	return c.withOverlayHits(ctx, baseHits, query, "", nil)
}

// searchOmitting searches as Search does with one example of a skill left out
// of whichever index holds the skill: the overlay's own terms for a replaced or
// added skill, the base index for any other.
func (c *overlayCatalog) searchOmitting(ctx context.Context, query string, limit int, skillID, phrase string) ([]resolverpkg.SearchHit, error) {
	if _, err := c.Skills(ctx); err != nil {
		return nil, err
	}
	replacement, overlaid := c.replace[skillID]
	if !overlaid {
		omitter, ok := c.Catalog.(exampleOmitter)
		if !ok {
			return nil, fmt.Errorf("search without an example of %s: %w", skillID, errCannotOmit)
		}
		baseHits, err := omitter.searchOmitting(ctx, query, limit, skillID, phrase)
		if err != nil {
			return nil, err
		}
		return c.withOverlayHits(ctx, baseHits, query, "", nil)
	}
	baseHits, err := c.Catalog.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	if _, indexed := c.terms[skillID]; !indexed {
		return c.withOverlayHits(ctx, baseHits, query, "", nil)
	}
	remaining, found := withoutExample(replacement.Examples, phrase)
	if !found {
		return nil, fmt.Errorf("example %q is not an example of %s", phrase, skillID)
	}
	replacement.Examples = remaining
	return c.withOverlayHits(ctx, baseHits, query, skillID, indexedTerms(replacement))
}

// withOverlayHits drops replaced and removed skills from the base hits and
// adds the overlay skills that match the query. ownerTerms, when set, stands in
// for the indexed terms of the skill ownerID.
func (c *overlayCatalog) withOverlayHits(ctx context.Context, baseHits []resolverpkg.SearchHit, query, ownerID string, ownerTerms map[string]struct{}) ([]resolverpkg.SearchHit, error) {
	if _, err := c.Skills(ctx); err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(baseHits))
	hits := make([]resolverpkg.SearchHit, 0, len(baseHits))
	for _, hit := range baseHits {
		if _, replaced := c.replace[hit.SkillID]; replaced {
			continue
		}
		if _, gone := c.remove[hit.SkillID]; gone {
			continue
		}
		if !seen[hit.SkillID] {
			seen[hit.SkillID] = true
			hits = append(hits, hit)
		}
	}
	var queryTerms [][]string
	for _, token := range resolverpkg.Tokenize(query) {
		if terms := splitTerms(token); len(terms) > 0 {
			queryTerms = append(queryTerms, terms)
		}
	}
	ids := make([]string, 0, len(c.terms))
	for id := range c.terms {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		terms := c.terms[id]
		if id == ownerID {
			terms = ownerTerms
		}
		if matchesAnyToken(terms, queryTerms) {
			seen[id] = true
			hits = append(hits, resolverpkg.SearchHit{SkillID: id})
		}
	}
	return hits, nil
}

func matchesAnyToken(indexed map[string]struct{}, queryTerms [][]string) bool {
	for _, terms := range queryTerms {
		all := true
		for _, term := range terms {
			if _, ok := indexed[term]; !ok {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// indexedTerms mirrors the skill_fts columns: name, aliases, description,
// triggers, examples, and topics plus technologies.
func indexedTerms(skill resolverpkg.Skill) map[string]struct{} {
	terms := map[string]struct{}{}
	add := func(values ...string) {
		for _, value := range values {
			for _, term := range splitTerms(value) {
				terms[term] = struct{}{}
			}
		}
	}
	add(skill.Name, skill.Description)
	add(skill.Aliases...)
	add(skill.Triggers...)
	add(skill.Examples...)
	add(skill.Topics...)
	add(skill.Technologies...)
	return terms
}

// splitTerms lowercases text and splits it on non-alphanumerics, as the
// unicode61 tokenizer of the full-text index does.
func splitTerms(value string) []string {
	return strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
