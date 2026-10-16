package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/mutation"
	"github.com/vantt/skillhub-mcp/internal/skill"
	"github.com/vantt/skillhub-mcp/internal/skillruntime"
	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
	"github.com/vantt/skillhub-mcp/internal/workspace"
	"gopkg.in/yaml.v3"
)

// SkillAddApplyOptions carries the person's answers to the add questions.
type SkillAddApplyOptions struct {
	// Approve approves the content of a third-party skill in the same change.
	Approve bool
	// Activate activates the skill right after it is added, in the same commit.
	Activate bool
}

// skillAddExtras is what a person needs to decide on an add, beyond the plan:
// every file, the routing that will be written, its lint, and the trust line.
type skillAddExtras struct {
	files              []SkillCommitFile
	routing            *skill.RoutingInput
	lint               []SkillCommitLint
	trust              *SkillCommitTrust
	activationReady    bool
	activationMissing  []string
	approvedChanges    []mutation.Change
	fullDiffByAddedKey map[string]treeFile
}

// skillAddTree returns the content files and the hub metadata change of one
// skill in a set of add changes. Files are keyed relative to the skill folder.
func skillAddTree(changes []mutation.Change, collection, id string) (map[string]treeFile, string) {
	prefix := fmt.Sprintf("skills/%s/%s/", collection, id)
	tree := map[string]treeFile{}
	metaPath := ""
	for _, change := range changes {
		rel, ok := strings.CutPrefix(change.Path, prefix)
		if !ok {
			continue
		}
		if workspace.IsHubMeta(change.Path) {
			if strings.HasSuffix(change.Path, "/.meta/skill.yaml") {
				metaPath = change.Path
			}
			continue
		}
		tree[rel] = treeFile{Data: change.Contents, Executable: change.Executable}
	}
	return tree, metaPath
}

// applySkillAddRouting writes suggested routing, with the person's own fields on
// top, into the metadata of each added skill. It returns the routing of the
// first skill, which the preview shows.
func applySkillAddRouting(changes []mutation.Change, collection string, selected []DiscoveredSkillItem, override *skill.RoutingInput) (*skill.RoutingInput, error) {
	var first *skill.RoutingInput
	for _, item := range selected {
		tree, metaPath := skillAddTree(changes, collection, item.TargetID)
		skillMD := string(tree["SKILL.md"].Data)
		routing := skill.SuggestRouting(item.Description, skillMD)
		if override != nil {
			routing = overrideRouting(routing, *override)
		}
		if first == nil {
			copied := routing
			first = &copied
		}
		for index := range changes {
			if changes[index].Path != metaPath {
				continue
			}
			meta, err := setMetaRouting(changes[index].Contents, routing)
			if err != nil {
				return nil, err
			}
			changes[index].Contents = meta
		}
	}
	return first, nil
}

// buildSkillAddExtras describes the first added skill the way a person decides
// on it, and prepares the same write set with the content approved.
func buildSkillAddExtras(ctx context.Context, root string, planCtx skillAddPlanContext, routing *skill.RoutingInput) (*skillAddExtras, error) {
	primary := planCtx.selected[0]
	tree, metaPath := skillAddTree(planCtx.changes, planCtx.collection, primary.TargetID)
	var meta []byte
	for _, change := range planCtx.changes {
		if change.Path == metaPath {
			meta = change.Contents
		}
	}
	extras := &skillAddExtras{routing: routing}
	multiple := len(planCtx.selected) > 1
	extras.fullDiffByAddedKey = map[string]treeFile{}
	for _, item := range planCtx.selected {
		itemTree, _ := skillAddTree(planCtx.changes, planCtx.collection, item.TargetID)
		for rel, file := range itemTree {
			key := rel
			if multiple {
				key = item.TargetID + "/" + rel
			}
			extras.fullDiffByAddedKey[key] = file
		}
	}
	extras.files = describeFiles(nil, tree, nil)

	var document contentTrustDocument
	_ = yaml.Unmarshal(meta, &document)
	extras.trust = &SkillCommitTrust{
		ThirdParty:    skillruntime.IsThirdParty(document.provenance()),
		Origin:        strings.TrimSpace(planCtx.origin.Repository),
		ContentDigest: proposedContentDigest(tree, meta),
	}

	plan := &commitPlan{root: root, id: primary.TargetID, final: tree, metaAfter: meta}
	extras.lint = commitLint(ctx, plan)
	metaDoc := parseSkillReviewMeta(meta)
	metaDoc.Name, metaDoc.Description = catalog.DeriveSkillNameAndDesc(tree["SKILL.md"].Data, metaDoc.Name, metaDoc.Description, primary.TargetID)
	readiness, _, missing := checkActivationReadiness(tree["SKILL.md"].Data, metaDoc, true)
	extras.activationReady, extras.activationMissing = readiness.Ready, missing

	if extras.trust.ThirdParty && !multiple {
		approvedMeta, err := setMetaApproval(meta, extras.trust.ContentDigest)
		if err != nil {
			return nil, err
		}
		extras.approvedChanges = make([]mutation.Change, 0, len(planCtx.changes))
		for _, change := range planCtx.changes {
			if change.Path == metaPath {
				change.Contents = approvedMeta
			}
			extras.approvedChanges = append(extras.approvedChanges, change)
		}
	}
	return extras, nil
}

// addFullDiff renders every added content file as an added-file diff.
func addFullDiff(ctx context.Context, root string, files map[string]treeFile) string {
	return commitFullDiff(ctx, root, nil, files)
}

// ConfirmSkillAddWith applies a previewed add. With options it chains the
// approval of third-party content and the activation, so the draft, the
// approval and the activation land as one Git commit. Several skills added at
// once are drafts only.
func (service SkillAddService) ConfirmSkillAddWith(ctx context.Context, path string, preview SkillAddProposal, pins ConfirmationPins, options SkillAddApplyOptions) (SkillAddResult, error) {
	if preview.Error != nil || preview.Replayed {
		return service.ConfirmSkillAdd(ctx, path, preview, pins)
	}
	if len(preview.SkillIDs) > 1 {
		options = SkillAddApplyOptions{}
	}
	approve := options.Approve && preview.plannedApproved != nil
	activate := options.Activate
	message := skillAddCommitMessage(preview, approve, activate)
	if !approve && !activate {
		return service.ConfirmSkillAdd(withCommitMessage(ctx, message), path, preview, pins)
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return SkillAddResult{}, err
	}
	touched := make([]string, 0, len(preview.planned.WriteSet.Changes))
	for _, change := range preview.planned.WriteSet.Changes {
		touched = append(touched, change.Path)
	}
	chain, err := beginCommitChain(ctx, root, message, touched)
	if err != nil {
		return SkillAddResult{}, err
	}
	working := preview
	if approve {
		working.planned = *preview.plannedApproved
	}
	result, err := service.ConfirmSkillAdd(chain.Context(), path, working, pins)
	if err != nil || result.Error != nil || result.Replayed {
		return result, err
	}
	chain.Record(result.Receipt)
	result.Approved = approve
	if activate {
		activated, note := activateInChain(chain, path, result.SkillID)
		result.Activated = activated
		if !activated {
			chain.SetMessage(skillAddCommitMessage(preview, approve, false))
			result.Warnings = append(result.Warnings, Warning{Code: "activation_failed", Summary: note})
		}
	}
	if outcome := chain.Finish(); outcome != nil {
		noteAgentReach(ctx, root, outcome, result.SkillID)
		if outcome.Status == workspace.CommitCommitted {
			dirty, dirtyErr := workspace.DirtyPaths(ctx, root, touched)
			result.GitDirty = dirtyErr != nil || len(dirty) > 0
		}
		result.Commit = outcome
		result.Items = append(result.Items, commitItem(*outcome, result.GitDirty))
	}
	switch {
	case result.Activated:
		result.Summary = fmt.Sprintf("Added %s and activated it.", result.SkillID)
	case approve:
		result.Summary = fmt.Sprintf("Added draft %s and approved its content.", result.SkillID)
	}
	return result, nil
}

// skillAddCommitMessage is the subject of the Git commit of an add.
func skillAddCommitMessage(preview SkillAddProposal, approved, activated bool) string {
	message := "Add " + strings.Join(skillAddIDs(preview), ", ")
	if repo := skillAddRepositoryName(preview.Origin.Repository); repo != "" {
		message += " from " + repo
	}
	if approved {
		message += ", approve content"
	}
	if activated {
		message += ", activate"
	}
	return message
}

func skillAddIDs(preview SkillAddProposal) []string {
	if len(preview.SkillIDs) > 0 {
		return preview.SkillIDs
	}
	return []string{preview.SkillID}
}

// skillAddRepositoryName is a repository URL without its scheme and ".git".
func skillAddRepositoryName(repository string) string {
	name := strings.TrimSuffix(strings.TrimSpace(repository), ".git")
	if _, rest, found := strings.Cut(name, "://"); found {
		name = rest
	}
	return name
}

// planApprovedSkillAdd plans the add whose metadata carries the approval of the
// content. It is applied instead of the plain plan when the person approves.
func planApprovedSkillAdd(root string, planCtx skillAddPlanContext, extras *skillAddExtras) (*mutation.Proposal, error) {
	if extras == nil || extras.approvedChanges == nil {
		return nil, nil
	}
	key := planCtx.idempotencyKey + ":approved"
	planned, err := mutation.PlanMutation(root, mutation.WriteSet{
		Command:        "skill_add",
		IdempotencyKey: key,
		RequestDigest:  sourcepkg.Digest([]byte(planCtx.requestDigest + "|approved:" + extras.trust.ContentDigest)),
		Changes:        extras.approvedChanges,
	})
	if err != nil {
		return nil, err
	}
	return &planned, nil
}

// SourceLabel names where the add comes from, as "host/owner/repo@ref (path)".
func (p SkillAddProposal) SourceLabel() string {
	label := skillAddRepositoryName(p.Origin.Repository)
	if label == "" {
		return p.Origin.Kind
	}
	if p.Origin.Ref != "" {
		label += "@" + p.Origin.Ref
	}
	if p.Origin.Path != "" {
		label += " (" + p.Origin.Path + ")"
	}
	return label
}
