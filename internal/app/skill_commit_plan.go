package app

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/mutation"
	"github.com/vantt/skillhub-mcp/internal/skill"
	"github.com/vantt/skillhub-mcp/internal/skillruntime"
	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
	"gopkg.in/yaml.v3"
)

// mergeWithHub settles a checkout whose hub copy changed since: a three-way
// merge of the files the checkout started from, the hub's files today and the
// project's files. The merge engine is the one skill updates use; its
// upstream-centred words stay inside it and are translated for the person.
func (service SkillCommitService) mergeWithHub(ctx context.Context, preview *SkillCommitPreview, input SkillCommitInput, projectTree map[string]treeFile) (*Error, error) {
	plan := preview.plan
	hub := plan.hub
	resolutions, refusal := mergeResolutions(input.Resolutions)
	if refusal != nil {
		return refusal, nil
	}
	base, baseFound := findCheckoutBase(ctx, plan.root, hub.relDir, plan.id, *plan.marker)
	// The hub plays "local" and the project plays "upstream" in the engine.
	result, appErr, err := classifyAllFiles(ctx, plan.root, updateFileSets{Base: base, Local: treeBytes(hub.tree), Upstream: treeBytes(projectTree), BaseAvailable: baseFound}, resolutions)
	if appErr != nil || err != nil {
		return appErr, err
	}
	differing := map[string]bool{}
	for _, file := range result.Files {
		differing[file.Path] = true
	}
	for _, resolution := range input.Resolutions {
		if !differing[resolution.Path] {
			return NewInvalidRequestError(fmt.Sprintf("%s has no difference to decide", resolution.Path), "Run `skillhub skill commit` without --accept to see which files need a decision."), nil
		}
	}
	final := make(map[string]treeFile, len(hub.tree))
	for rel, file := range hub.tree {
		final[rel] = file
	}
	for rel, data := range result.ResolvedBytes {
		executable := hub.tree[rel].Executable
		if project, ok := projectTree[rel]; ok {
			executable = project.Executable
		}
		final[rel] = treeFile{Data: data, Executable: executable}
	}
	for rel := range result.IsDeleteFile {
		delete(final, rel)
	}
	plan.final = final
	preview.Conflicts = result.Unresolved
	preview.MergeFiles = result.Files
	preview.Merge = mergeEntries(result.Files)
	preview.ProjectChanged, preview.HubChanged = countSides(result.Files)
	return nil, nil
}

// mergeResolutions translates the person's choices into the engine's actions.
func mergeResolutions(choices []SkillCommitResolution) ([]UpstreamResolution, *Error) {
	var resolutions []UpstreamResolution
	for _, choice := range choices {
		switch choice.Choice {
		case "hub":
			resolutions = append(resolutions, UpstreamResolution{Path: choice.Path, Action: "local"})
		case "project":
			resolutions = append(resolutions, UpstreamResolution{Path: choice.Path, Action: "upstream"})
		case "manual":
			resolutions = append(resolutions, UpstreamResolution{Path: choice.Path, Action: "manual", Content: string(choice.Content)})
		default:
			return nil, NewInvalidRequestError(fmt.Sprintf("%q is not a choice for %s", choice.Choice, choice.Path), "Choose hub, project, or a file with --manual.")
		}
	}
	return resolutions, nil
}

// mergeStatusWords says, in project terms, whose change a merge status is.
func mergeStatusWords(file UpstreamFile) string {
	decided := file.Action != ""
	switch file.Status {
	case "upstream_only":
		return "changed in the project"
	case "local_only":
		return "changed on the hub, kept"
	case "added_upstream":
		return "added in the project"
	case "added_local":
		return "added on the hub, kept"
	case "removed_upstream":
		if decided {
			return "removed in the project"
		}
	case "removed_local":
		if decided {
			return "removed on the hub, kept removed"
		}
	case "both_changed":
		switch file.Action {
		case "merged":
			return "changed in both, merged"
		case "upstream":
			return "changed in both, project version used"
		case "local":
			return "changed in both, hub version used"
		case "manual":
			return "changed in both, edited by hand"
		}
	}
	return "needs a decision"
}

func mergeActionWords(action string) string {
	switch action {
	case "upstream":
		return "project"
	case "local":
		return "hub"
	}
	return action
}

func mergeEntries(files []UpstreamFile) []SkillCommitMerge {
	entries := make([]SkillCommitMerge, 0, len(files))
	for _, file := range files {
		entries = append(entries, SkillCommitMerge{Path: file.Path, Status: mergeStatusWords(file), Action: mergeActionWords(file.Action)})
	}
	return entries
}

// countSides counts the files each side changed since the checkout.
func countSides(files []UpstreamFile) (project, hub int) {
	for _, file := range files {
		undecidedRemoval := file.DefaultAction == "" && (file.Status == "removed_upstream" || file.Status == "removed_local")
		switch file.Status {
		case "upstream_only", "added_upstream", "blocked":
			project++
		case "local_only", "added_local":
			hub++
		case "both_changed":
			project++
			hub++
		case "removed_upstream", "removed_local":
			if undecidedRemoval {
				project++
				hub++
			} else if file.Status == "removed_upstream" {
				project++
			} else {
				hub++
			}
		}
	}
	return project, hub
}

// planNewSkill builds the draft a folder with no hub skill becomes, with the
// same change builder skill add uses, so a folder and a repository land alike.
func (service SkillCommitService) planNewSkill(preview *SkillCommitPreview, input SkillCommitInput, projectTree map[string]treeFile) (*Error, error) {
	plan := preview.plan
	collection := strings.TrimSpace(input.Collection)
	if collection == "" {
		collection = skill.DefaultCollection
	}
	if !skillTargetIDPattern.MatchString(collection) {
		return NewInvalidRequestError("invalid collection name", "Specify a valid lowercase collection name."), nil
	}
	plan.caseName, plan.collection, plan.final = commitCaseNew, collection, projectTree
	plan.relDir = "skills/" + collection + "/" + plan.id
	plan.metaPath = plan.relDir + "/.meta/skill.yaml"
	preview.Case, preview.Collection = commitCaseNew, collection

	item := discoveredFromTree(plan.id, projectTree)
	captured := skillAddCapture{origin: SkillOrigin{Kind: "local", Name: plan.id, FolderDigest: skillBaseVersion(projectTree)}}
	changes, _, _, rejected, err := buildSkillAddChanges([]DiscoveredSkillItem{item}, collection, captured, service.Clock.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"))
	if err != nil {
		return nil, err
	}
	if rejected != nil {
		return rejected.Error, nil
	}
	// The hub keeps the normalized SKILL.md the builder produces, so the folder
	// is synced to that, and the next commit finds nothing to change.
	for _, change := range changes {
		if change.Path == plan.relDir+"/SKILL.md" {
			entry := projectTree["SKILL.md"]
			entry.Data = change.Contents
			projectTree["SKILL.md"] = entry
		}
	}
	skillMD := projectTree["SKILL.md"].Data
	routing := skill.SuggestRouting(item.Description, string(skillMD))
	if input.Routing != nil {
		routing = overrideRouting(routing, *input.Routing)
	}
	preview.RoutingSuggestions = &routing
	for index := range changes {
		if changes[index].Path != plan.metaPath {
			continue
		}
		meta, err := setMetaRouting(changes[index].Contents, routing)
		if err != nil {
			return nil, err
		}
		if input.Runtime != nil {
			document, err := decodeMeta(meta)
			if err != nil {
				return nil, err
			}
			if err := skill.ApplyRuntimeBlock(document, input.Runtime); err != nil {
				return NewInvalidRequestError(err.Error(), "Fix the runtime file: requires.bins, requires.env, requires.platforms and setup.check/command are the known keys."), nil
			}
			if meta, err = encodeMeta(document); err != nil {
				return nil, err
			}
		}
		changes[index].Contents, plan.metaAfter = meta, meta
	}
	plan.changes = changes
	metaDoc := parseSkillReviewMeta(plan.metaAfter)
	metaDoc.Name, metaDoc.Description = catalog.DeriveSkillNameAndDesc(skillMD, metaDoc.Name, metaDoc.Description, plan.id)
	readiness, _, missing := checkActivationReadiness(skillMD, metaDoc, true)
	preview.ActivationReady, preview.ActivationMissing = readiness.Ready, missing
	return nil, nil
}

// discoveredFromTree describes a project folder the way discovery describes a
// skill found in a source, so the add change builder can take it.
func discoveredFromTree(id string, tree map[string]treeFile) DiscoveredSkillItem {
	skillMD := tree["SKILL.md"].Data
	name, description, _ := parseSkillMDFrontmatter(skillMD)
	if name == "" {
		name = id
	}
	item := DiscoveredSkillItem{Name: name, TargetID: id, Description: description, SkillMDBytes: skillMD, CompanionBytes: map[string][]byte{}, TotalBytes: int64(len(skillMD)), FileCount: len(tree)}
	paths := make([]string, 0, len(tree))
	for rel := range tree {
		if rel != "SKILL.md" {
			paths = append(paths, rel)
		}
	}
	sort.Strings(paths)
	for _, rel := range paths {
		file := tree[rel]
		item.Companions = append(item.Companions, DiscoveredCompanion{Path: rel, FullPath: rel, Bytes: int64(len(file.Data)), Digest: sourcepkg.Digest(file.Data), Executable: file.Executable})
		item.CompanionBytes[rel] = file.Data
		item.TotalBytes += int64(len(file.Data))
	}
	return item
}

// finishPreview fills in everything a person reviews: the files, the trust
// line, the routing lint and the full diff, and checks the hub would accept it.
func (service SkillCommitService) finishPreview(ctx context.Context, preview SkillCommitPreview, input SkillCommitInput) (SkillCommitPreview, error) {
	plan := preview.plan
	var hubTree map[string]treeFile
	if plan.hub != nil {
		hubTree = plan.hub.tree
	}
	preview.Files = describeFiles(hubTree, plan.final, preview.Merge)
	for _, file := range plan.final {
		preview.TotalBytes += int64(len(file.Data))
	}
	plan.finalVersion = skillBaseVersion(plan.final)
	preview.NothingToCommit = len(plan.changes) == 0 && len(preview.Conflicts) == 0
	if plan.hub != nil {
		preview.Trust = trustOfProposal(plan, plan.final, plan.metaAfter)
		plan.thirdParty, plan.origin, plan.contentDigest = preview.Trust.ThirdParty, preview.Trust.Origin, preview.Trust.ContentDigest
	}
	if plan.submission != nil {
		// Content from another machine is untrusted until a person approves
		// it, whatever the skill's own origin is.
		if preview.Trust == nil {
			preview.Trust = &SkillCommitTrust{ContentDigest: proposedContentDigest(plan.final, plan.metaAfter)}
		}
		preview.Trust.ThirdParty, preview.Trust.Origin = true, "submitted from "+plan.submission.ClientLabel
		plan.thirdParty, plan.origin, plan.contentDigest = true, preview.Trust.Origin, preview.Trust.ContentDigest
	}
	warnings := preview.Warnings
	preview.Result = NewResult(StatusActionRequired, commitSummary(preview))
	preview.Warnings = warnings
	switch {
	case preview.NothingToCommit:
		preview.Result = NewResult(StatusOK, fmt.Sprintf("Nothing to commit: %s matches the hub.", plan.id))
	case len(preview.Conflicts) > 0:
		preview.Warnings = append(preview.Warnings, Warning{Code: "needs_decision", Summary: fmt.Sprintf("%s need a decision before this can be applied.", pluralize(len(preview.Conflicts), "file"))})
	}
	if !preview.NothingToCommit && len(plan.changes) > 0 {
		preview.RoutingLint = commitLint(ctx, plan)
		if err := validateCommitPlan(plan); err != nil {
			return commitPreviewError(NewInvalidRequestError(err.Error(), "Fix what is reported in the folder and run commit again.")), nil
		}
	}
	if input.FullDiff {
		preview.FullDiff = commitFullDiff(ctx, plan.root, hubTree, plan.final)
	}
	return preview, nil
}

func commitSummary(preview SkillCommitPreview) string {
	id := preview.SkillID
	switch preview.Case {
	case commitCaseNew:
		return fmt.Sprintf("%s is not on the hub: this adds a new skill (collection: %s).", id, preview.Collection)
	case commitCaseReplace:
		return fmt.Sprintf("%s was not checked out from the hub: this replaces the hub copy.", id)
	case commitCaseMerge:
		return fmt.Sprintf("%s: %s in the project; the hub changed %s since checkout", id, pluralize(preview.ProjectChanged, "file")+" changed", pluralize(preview.HubChanged, "file"))
	}
	return fmt.Sprintf("%s: %s changed since checkout (the hub did not change)", id, pluralize(len(preview.Files), "file"))
}

// describeFiles lists the content files a commit adds, modifies or deletes,
// with the lines each gains and loses.
func describeFiles(hub, final map[string]treeFile, merge []SkillCommitMerge) []SkillCommitFile {
	status := map[string]string{}
	for _, entry := range merge {
		status[entry.Path] = entry.Status
	}
	seen := map[string]bool{}
	var paths []string
	for rel := range final {
		seen[rel] = true
		paths = append(paths, rel)
	}
	for rel := range hub {
		if !seen[rel] {
			paths = append(paths, rel)
		}
	}
	sort.Strings(paths)
	files := []SkillCommitFile{}
	for _, rel := range paths {
		before, hadBefore := hub[rel]
		after, hasAfter := final[rel]
		if hadBefore && hasAfter && bytes.Equal(before.Data, after.Data) {
			continue
		}
		file := SkillCommitFile{Path: rel, Status: status[rel], Size: int64(len(after.Data))}
		switch {
		case !hadBefore:
			file.Change = "A"
		case !hasAfter:
			file.Change, file.Size = "D", int64(len(before.Data))
		default:
			file.Change = "M"
		}
		if mergeableText(before.Data) && mergeableText(after.Data) {
			file.Added, file.Removed = skill.CountLineChanges(before.Data, after.Data)
		}
		files = append(files, file)
	}
	return files
}

// proposedContentDigest is the digest an approval binds to: the proposed
// content files and the runtime block of the metadata.
func proposedContentDigest(final map[string]treeFile, meta []byte) string {
	files := make([]skillruntime.ResourceDigest, 0, len(final))
	for rel, file := range final {
		files = append(files, skillruntime.ResourceDigest{Path: rel, Digest: sourcepkg.Digest(file.Data)})
	}
	spec, hasSpec := reviewRuntimeSpec(meta)
	return skillruntime.ContentDigest(files, spec, hasSpec)
}

// trustOfProposal says whether the proposed content needs an approval and
// whether today's hub content has one.
func trustOfProposal(plan *commitPlan, final map[string]treeFile, meta []byte) *SkillCommitTrust {
	var document contentTrustDocument
	_ = yaml.Unmarshal(plan.metaBefore, &document)
	trust := &SkillCommitTrust{
		ThirdParty:    skillruntime.IsThirdParty(document.provenance()),
		ContentDigest: proposedContentDigest(final, meta),
	}
	approved := document.Quality.ContentReviewedDigest
	trust.Approved = approved != "" && approved == proposedContentDigest(plan.hub.tree, plan.metaBefore)
	if trust.ThirdParty {
		provenance := extractSkillProvenance(parseSkillReviewMeta(plan.metaBefore))
		if provenance != nil {
			trust.Origin = strings.TrimSpace(provenance.SourceLocator)
		}
		if trust.Origin == "" {
			trust.Origin = thirdPartyOrigin(plan.metaBefore)
		}
	}
	return trust
}

// thirdPartyOrigin names where a third-party skill came from.
func thirdPartyOrigin(meta []byte) string {
	for _, source := range parseSkillReviewMeta(meta).Sources {
		for _, role := range source.Roles {
			if role != "upstream" {
				continue
			}
			if source.Repo != "" {
				return source.Repo
			}
			if source.Repository != "" {
				return source.Repository
			}
			return source.ID
		}
	}
	return "an upstream source"
}

// commitLint lints the proposed skill the way review does, through the same
// decoding the catalog applies, against the skills that are active now.
func commitLint(ctx context.Context, plan *commitPlan) []SkillCommitLint {
	findings := []SkillCommitLint{}
	meta := plan.metaAfter
	if meta == nil {
		meta = plan.metaBefore
	}
	lint, ok, err := lintProposedSkill(ctx, plan.root, plan.id, meta, plan.final["SKILL.md"].Data)
	if !ok {
		return findings
	}
	if err != nil {
		return append(findings, SkillCommitLint{Code: "lint_skipped", Message: "Routing lint skipped: " + err.Error()})
	}
	for _, finding := range lint {
		findings = append(findings, SkillCommitLint{Code: finding.Code, Message: finding.Message, Fix: finding.Fix})
	}
	return findings
}

// validateCommitPlan asks the mutation layer whether the hub would accept the
// proposed files, so a refusal shows in the preview and not after a "y".
func validateCommitPlan(plan *commitPlan) error {
	_, err := mutation.PlanMutation(plan.root, commitWriteSet(plan, plan.changes, "preview"))
	return err
}

// commitWriteSet is the write set of a commit. The key names the content and
// the hub version it was made against, so repeating the same commit replays and
// a different one never collides.
func commitWriteSet(plan *commitPlan, changes []mutation.Change, variant string) mutation.WriteSet {
	key := fmt.Sprintf("skill_commit:%s:%s", plan.id, strings.TrimPrefix(sourcepkg.Digest([]byte(strings.Join([]string{plan.id, plan.caseName, plan.hubVersion, plan.finalVersion, variant, digestOf(plan.metaAfter), plan.nonce}, "|"))), "sha256:")[:32])
	return mutation.WriteSet{Command: "skill_commit", IdempotencyKey: key, RequestDigest: sourcepkg.Digest([]byte(key)), Changes: changes}
}

// commitFullDiff renders a unified diff of every changed content file.
func commitFullDiff(ctx context.Context, root string, hub, final map[string]treeFile) string {
	var out strings.Builder
	for _, file := range describeFiles(hub, final, nil) {
		before, after := hub[file.Path].Data, final[file.Path].Data
		if !mergeableText(before) || !mergeableText(after) {
			fmt.Fprintf(&out, "Binary file %s differs (%s)\n", file.Path, file.Change)
			continue
		}
		if text, err := gitDiffNoIndex(ctx, root, file.Path, before, after); err == nil && text != "" {
			out.WriteString(text)
			if !strings.HasSuffix(text, "\n") {
				out.WriteString("\n")
			}
		}
	}
	return out.String()
}
