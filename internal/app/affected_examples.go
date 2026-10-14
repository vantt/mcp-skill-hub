package app

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
)

// skillChange is one skill that differs between two states of the library. Old
// is nil for an added skill and New is nil for a removed one.
type skillChange struct {
	ID  string
	Old *resolverpkg.Skill
	New *resolverpkg.Skill
}

type selfCheckMode int

const (
	// selfCheckChanges checks the changed skills' own examples and the examples
	// of other skills that a changed skill may now take.
	selfCheckChanges selfCheckMode = iota
	// selfCheckFull checks the own examples of every active skill.
	selfCheckFull
)

// bundledOwnerID is excluded as a case owner, as in the routing evaluation.
const bundledOwnerID = "system-curator"

// checkSkillChanges replays routing examples against the served generation with
// the changes applied and reports the ones that no longer resolve as their
// author meant. It resolves in memory and records nothing. The result is
// sorted, so a rerun on the same snapshot gives the same findings. When the
// work is over the bound or the resolver rejects a skill, it returns one
// self_resolution_skipped finding instead.
func checkSkillChanges(ctx context.Context, view *catalog.View, policy resolverpkg.Policy, changes []skillChange, mode selfCheckMode) []resolverpkg.LintFinding {
	served, err := servedSkillCatalog(ctx, view)
	if err == nil {
		var findings []resolverpkg.LintFinding
		if findings, err = runSelfCheck(ctx, served, policy, changes, mode); err == nil {
			return findings
		}
	}
	id := ""
	if len(changes) == 1 {
		id = changes[0].ID
	}
	fix := "Fill in the required routing fields first"
	if errors.Is(err, errOverBound) {
		fix = "Change fewer skills at once, or run skillhub eval routing for the whole library"
	}
	return []resolverpkg.LintFinding{resolverpkg.SelfResolutionSkipped(id, err.Error(), fix)}
}

// errOverBound marks a run that would resolve more than the work bound allows.
var errOverBound = errors.New("too many examples to resolve at once")

func servedSkillCatalog(ctx context.Context, view *catalog.View) (resolverpkg.Catalog, error) {
	database, err := view.DB(ctx)
	if err != nil {
		return nil, err
	}
	sqlite, err := resolverpkg.NewSQLiteCatalog(database, view.Pointer.CatalogSnapshot)
	if err != nil {
		return nil, err
	}
	return memoSkillsCatalog{Catalog: sqlite, view: view}, nil
}

// selectedCase is a case to resolve with the skill that owns its phrase.
type selectedCase struct {
	selfResolutionCase
	owner resolverpkg.Skill
}

// taker is a changed skill whose new version may win other skills' examples.
type taker struct {
	skill  resolverpkg.Skill
	old    *resolverpkg.Skill
	linked bool
}

func runSelfCheck(ctx context.Context, served resolverpkg.Catalog, policy resolverpkg.Policy, changes []skillChange, mode selfCheckMode) ([]resolverpkg.LintFinding, error) {
	if mode == selfCheckFull {
		skills, err := served.Skills(ctx)
		if err != nil {
			return nil, err
		}
		changes = unchangedActive(skills)
	}
	changes = sortedActiveChanges(changes)
	after := newOverlayCatalog(served, versions(changes, func(c skillChange) *resolverpkg.Skill { return c.New }), gone(changes, func(c skillChange) *resolverpkg.Skill { return c.New }))
	afterSkills, err := after.Skills(ctx)
	if err != nil {
		return nil, err
	}
	takers := takersOf(changes, afterSkills)
	if mode == selfCheckFull {
		takers = nil
	}
	cases := selectCases(afterSkills, changes, takers, policy)

	active := 0
	for _, skill := range afterSkills {
		if skill.Status == "active" {
			active++
		}
	}
	if work := len(cases) * active; work > maxSelfResolutionWork {
		return nil, fmt.Errorf("%w: %d examples against %d active skills exceed the bound of %d resolves x skills", errOverBound, len(cases), active, maxSelfResolutionWork)
	}

	before := beforeState{newOverlayCatalog(served, versions(changes, func(c skillChange) *resolverpkg.Skill { return c.Old }), gone(changes, func(c skillChange) *resolverpkg.Skill { return c.Old })), policy}
	own := ownerIDs(changes)
	var findings []resolverpkg.LintFinding
	for _, c := range cases {
		request := selfResolutionRequest(c.selfResolutionCase, c.owner)
		response, err := engineResolver(c.withoutPhrase(after), policy)(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("resolve %q of %s: %w", c.Phrase, c.SkillID, err)
		}
		if _, changed := own[c.SkillID]; changed {
			findings = append(findings, ownCaseFindings(c, response)...)
			continue
		}
		if resolvedTo(response, c.SkillID) {
			continue
		}
		for _, t := range takers {
			wins, takes := takesCase(t.skill, response, request, policy)
			if !takes {
				continue
			}
			already, err := before.held(ctx, t, c, request, wins)
			if err != nil {
				return nil, err
			}
			findings = append(findings, resolverpkg.ExampleTakenBy(t.skill.ID, c.SkillID, c.Phrase, wins, already))
		}
	}
	sort.SliceStable(findings, func(i, j int) bool { return findingLess(findings[i], findings[j]) })
	return findings, nil
}

// beforeState is the served library with every change taken back.
type beforeState struct {
	catalog *overlayCatalog
	policy  resolverpkg.Policy
}

// held reports whether the taker already won (or already shared the
// choice for) the case before the change, by resolving it over the old library.
// An added skill held nothing before.
func (b beforeState) held(ctx context.Context, t taker, c selectedCase, request resolverpkg.Request, wins bool) (bool, error) {
	if t.old == nil {
		return false, nil
	}
	response, err := engineResolver(c.withoutPhrase(b.catalog), b.policy)(ctx, request)
	if err != nil {
		return false, fmt.Errorf("resolve %q of %s: %w", c.Phrase, c.SkillID, err)
	}
	beforeWins, beforeTakes := takesCase(*t.old, response, request, b.policy)
	if wins {
		return beforeWins, nil
	}
	return beforeTakes && !beforeWins, nil
}

// unchangedActive lists every active skill as a change with the served version
// on both sides.
func unchangedActive(skills []resolverpkg.Skill) []skillChange {
	var changes []skillChange
	for _, skill := range skills {
		if skill.Status != "active" {
			continue
		}
		skill := skill
		changes = append(changes, skillChange{ID: skill.ID, Old: &skill, New: &skill})
	}
	return changes
}

// sortedActiveChanges orders changes by id and treats both sides as active, so a
// draft is judged as the skill it would become.
func sortedActiveChanges(changes []skillChange) []skillChange {
	sorted := make([]skillChange, len(changes))
	for i, change := range changes {
		sorted[i] = skillChange{ID: change.ID, Old: asActive(change.Old), New: asActive(change.New)}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	return sorted
}

func asActive(skill *resolverpkg.Skill) *resolverpkg.Skill {
	if skill == nil {
		return nil
	}
	copied := *skill
	copied.Status = "active"
	return &copied
}

// versions maps skill ids to the side of the change that pick selects; a change
// whose side is nil has no version.
func versions(changes []skillChange, pick func(skillChange) *resolverpkg.Skill) map[string]resolverpkg.Skill {
	result := map[string]resolverpkg.Skill{}
	for _, change := range changes {
		if skill := pick(change); skill != nil {
			result[change.ID] = *skill
		}
	}
	return result
}

// gone lists the ids whose side of the change is nil.
func gone(changes []skillChange, pick func(skillChange) *resolverpkg.Skill) map[string]struct{} {
	result := map[string]struct{}{}
	for _, change := range changes {
		if pick(change) == nil {
			result[change.ID] = struct{}{}
		}
	}
	return result
}

func ownerIDs(changes []skillChange) map[string]struct{} {
	result := map[string]struct{}{}
	for _, change := range changes {
		if change.New != nil {
			result[change.ID] = struct{}{}
		}
	}
	return result
}

// takersOf lists the changed skills that have a new version, with whether the
// resolver links each to another skill through distinguish_from.
func takersOf(changes []skillChange, afterSkills []resolverpkg.Skill) []taker {
	var takers []taker
	for _, change := range changes {
		if change.New == nil {
			continue
		}
		linked := len(change.New.DistinguishFrom) > 0
		for _, other := range afterSkills {
			for _, discriminator := range other.DistinguishFrom {
				if other.Status == "active" && discriminator.SkillID == change.ID {
					linked = true
				}
			}
		}
		takers = append(takers, taker{skill: *change.New, old: change.Old, linked: linked})
	}
	return takers
}

// selectCases picks the cases to resolve: every case of each changed skill, and
// the examples of other active skills that some changed skill may influence.
// Counter-examples of other skills are not selected: a changed skill winning
// one is harmless for its owner, so no result of theirs is ever reported.
func selectCases(afterSkills []resolverpkg.Skill, changes []skillChange, takers []taker, policy resolverpkg.Policy) []selectedCase {
	own := ownerIDs(changes)
	var cases []selectedCase
	for _, skill := range afterSkills {
		if skill.Status != "active" || skill.ID == bundledOwnerID {
			continue
		}
		if _, changed := own[skill.ID]; changed {
			for _, c := range ownCases(skill) {
				cases = append(cases, selectedCase{c, skill})
			}
			continue
		}
		for _, c := range ownCases(skill) {
			if c.Kind != selfCaseExample {
				continue
			}
			request := selfResolutionRequest(c, skill)
			for _, t := range takers {
				if resolverpkg.Influences(t.skill, request, policy, t.linked) {
					cases = append(cases, selectedCase{c, skill})
					break
				}
			}
		}
	}
	sort.SliceStable(cases, func(i, j int) bool {
		a, b := cases[i], cases[j]
		if a.SkillID != b.SkillID {
			return a.SkillID < b.SkillID
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Index < b.Index
	})
	return cases
}

func resolvedTo(response resolverpkg.Response, skillID string) bool {
	return response.Status == resolverpkg.StatusResolved && response.Primary != nil && response.Primary.ID == skillID
}

// ownCaseFindings applies the rules for a skill's own cases: an example must
// resolve to its skill and a counter-example must not.
func ownCaseFindings(c selectedCase, response resolverpkg.Response) []resolverpkg.LintFinding {
	switch c.Kind {
	case selfCaseExample:
		if resolvedTo(response, c.SkillID) {
			return nil
		}
		winner := ""
		if response.Primary != nil {
			winner = response.Primary.ID
		}
		return []resolverpkg.LintFinding{resolverpkg.ExampleResolvesElsewhere(c.SkillID, c.Phrase, response.Status, winner)}
	default:
		if response.Primary != nil && response.Primary.ID == c.SkillID {
			return []resolverpkg.LintFinding{resolverpkg.CounterExampleResolvesToSkill(c.SkillID, c.Phrase)}
		}
		return nil
	}
}

// takesCase reports whether the skill takes part in a missed example. wins is
// true when it is the resolved primary; otherwise it shares the choice, by
// ranking in the top two or by excluding itself with relevance above the floor.
func takesCase(skill resolverpkg.Skill, response resolverpkg.Response, request resolverpkg.Request, policy resolverpkg.Policy) (wins, takes bool) {
	if resolvedTo(response, skill.ID) {
		return true, true
	}
	if response.Status == resolverpkg.StatusResolved {
		return false, false
	}
	for i, id := range response.TopKSkillIDs {
		if i < 2 && id == skill.ID {
			return false, true
		}
	}
	if response.NoSkill != nil && response.NoSkill.ReasonCode == "constraint_conflict" {
		return false, resolverpkg.ExcludedWithinReach(skill, request, policy)
	}
	return false, false
}

func findingLess(a, b resolverpkg.LintFinding) bool {
	switch {
	case a.SkillID != b.SkillID:
		return a.SkillID < b.SkillID
	case a.Code != b.Code:
		return a.Code < b.Code
	case a.Field != b.Field:
		return a.Field < b.Field
	case a.Value != b.Value:
		return a.Value < b.Value
	}
	return a.OtherSkillID < b.OtherSkillID
}
