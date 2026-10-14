package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
	"github.com/vantt/skillhub-mcp/internal/skill"
)

// RoutingSuggestionProposal is an ordinary routing edit preview for one
// suggestion, with the suggestion's reason and evidence beside it.
type RoutingSuggestionProposal struct {
	SkillProposal
	Suggestion RoutingSuggestion `json:"suggestion"`
}

// PreviewRoutingSuggestion turns a current suggestion into a routing edit
// preview. It appends the suggested text to its field and goes through
// PreviewSkillUpdate, so pins, expiry, confirmation and the hub commit are the
// ones of any skill edit. The proposal always carries the full diff, because
// the exact text a person confirms is what ends up in Git. Nothing is written
// to the skill; the only write is the stored proposal.
func (service SkillService) PreviewRoutingSuggestion(ctx context.Context, path, suggestionID string) (RoutingSuggestionProposal, error) {
	notCurrent := NewInvalidRequestError(
		fmt.Sprintf("Suggestion %s is not current: its evidence expired, it was applied or dismissed, or the skill changed.", suggestionID),
		"Run `skillhub telemetry cases suggest`.")
	if !suggestionIDPattern.MatchString(suggestionID) {
		return RoutingSuggestionProposal{}, notCurrent
	}
	report, err := RoutingSuggestionService{}.Suggest(ctx, path, RoutingSuggestionQuery{})
	if err != nil {
		return RoutingSuggestionProposal{}, err
	}
	var suggestion *RoutingSuggestion
	for i := range report.Suggestions {
		if report.Suggestions[i].ID == suggestionID {
			suggestion = &report.Suggestions[i]
		}
	}
	if suggestion == nil {
		return RoutingSuggestionProposal{}, notCurrent
	}
	routing, err := service.ReadSkillRouting(ctx, path, suggestion.SkillID)
	if err != nil {
		return RoutingSuggestionProposal{}, err
	}
	switch suggestion.Field {
	case "examples":
		routing.Examples = append(routing.Examples, suggestion.Text)
	case "counter_examples":
		routing.CounterExamples = append(routing.CounterExamples, suggestion.Text)
	case "not_for":
		routing.NotFor = append(routing.NotFor, suggestion.Text)
	default:
		return RoutingSuggestionProposal{}, fmt.Errorf("suggestion %s names the unsupported field %q", suggestion.ID, suggestion.Field)
	}
	editor := service
	editor.Manager.RoutingHook = routingCheckHook{field: suggestion.Field, phrase: suggestion.Text}
	proposal, err := editor.PreviewSkillUpdate(ctx, path, suggestion.SkillID, skill.UpdateInput{Routing: &routing}, true)
	if err != nil {
		return RoutingSuggestionProposal{}, err
	}
	return RoutingSuggestionProposal{SkillProposal: proposal, Suggestion: *suggestion}, nil
}

// maxRoutingCheckWarnings bounds the warning lines of one preview.
const maxRoutingCheckWarnings = 10

// routingCheckHook fills a preview's routing impact from the shared findability
// lint: the proposed skill is replayed against the generation agents are served
// now. It is read-only, records nothing and rebuilds nothing.
type routingCheckHook struct {
	field  string
	phrase string
}

func (h routingCheckHook) Evaluate(ctx context.Context, root, skillID, baseSnapshot string, _, afterMetadata []byte) (skill.RoutingImpact, error) {
	skipped := func(reason string) (skill.RoutingImpact, error) {
		return skill.RoutingImpact{Summary: "Routing check skipped: " + reason + ". Run `skillhub eval routing` after confirming."}, nil
	}
	view, err := catalog.ObservePublishedLocked(ctx, root)
	if err != nil {
		return skipped("the catalog generation is unavailable")
	}
	current := view.Pointer.CatalogSnapshot
	view.Close()
	if current != baseSnapshot {
		return skipped("the catalog changed since this proposal was based on it")
	}
	_, relDir, _, err := locateSkillDir(root, skillID)
	if err != nil {
		return skill.RoutingImpact{}, err
	}
	entrypoint, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relDir), "SKILL.md"))
	if err != nil {
		return skill.RoutingImpact{}, fmt.Errorf("read the skill entrypoint: %w", err)
	}
	findings, _, err := lintProposedSkill(ctx, root, skillID, afterMetadata, entrypoint)
	if err != nil {
		return skill.RoutingImpact{}, err
	}
	return h.impact(findings), nil
}

// impact renders the findability findings of a suggestion's routing change. A
// finding about the suggested phrase says whether it routes as intended; every
// other example finding is a regression the change causes.
func (h routingCheckHook) impact(findings []resolverpkg.LintFinding) skill.RoutingImpact {
	var notIntended, regressions []resolverpkg.LintFinding
	for _, finding := range findings {
		switch finding.Code {
		case "lint_skipped", "self_resolution_skipped":
			return skill.RoutingImpact{Summary: fmt.Sprintf("Routing check skipped: %s. %s. Run `skillhub eval routing` after confirming.",
				strings.TrimPrefix(strings.TrimPrefix(finding.Message, "example check skipped: "), "Routing lint skipped: "), finding.Fix)}
		case "example_resolves_elsewhere", "counter_example_resolves_to_skill":
			if finding.Value == h.phrase {
				notIntended = append(notIntended, finding)
				continue
			}
			regressions = append(regressions, finding)
		case "example_taken_by":
			regressions = append(regressions, finding)
		}
	}
	outcome := "the phrase routes as intended"
	switch {
	case len(notIntended) > 0:
		outcome = "the phrase does not route as intended"
	case h.field == "not_for":
		outcome = "the not_for entry itself is not replayed, only the examples it can affect"
	}
	summary := fmt.Sprintf("Routing check against the served catalog: %s; %s.", outcome,
		plural(len(regressions), "regression", "regressions"))
	var warnings []string
	for _, finding := range append(notIntended, regressions...) {
		if len(warnings) == maxRoutingCheckWarnings {
			warnings = append(warnings, fmt.Sprintf("and %d more", len(notIntended)+len(regressions)-maxRoutingCheckWarnings))
			break
		}
		warnings = append(warnings, finding.Message+". "+finding.Fix+".")
	}
	return skill.RoutingImpact{Summary: summary, Warnings: warnings}
}

var _ skill.RoutingImpactHook = routingCheckHook{}
