package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"gopkg.in/yaml.v3"
)

// ActivationReadiness describes whether a skill can transition to active.
type ActivationReadiness struct {
	Ready             bool     `json:"ready"`
	UntouchedScaffold bool     `json:"untouched_scaffold"`
	MissingFields     []string `json:"missing_fields,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
}

// ResourceItem describes one skill file found in canonical storage.
type ResourceItem struct {
	Path      string `json:"path"`
	Digest    string `json:"digest"`
	SizeBytes int64  `json:"size_bytes"`
}

// SkillResourceStatus details canonical files on disk.
type SkillResourceStatus struct {
	EntrypointPath   string         `json:"entrypoint_path"`
	EntrypointDigest string         `json:"entrypoint_digest,omitempty"`
	ResourceCount    int            `json:"resource_count"`
	TotalBytes       int64          `json:"total_bytes"`
	Resources        []ResourceItem `json:"resources,omitempty"`
}

// SkillProvenance captures origin and creation metadata from skill.meta.yaml.
type SkillProvenance struct {
	CreatedBy      string `json:"created_by,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	SourceID       string `json:"source_id,omitempty"`
	SourceLocator  string `json:"source_locator,omitempty"`
	SourceRevision string `json:"source_revision,omitempty"`
	UpstreamPath   string `json:"upstream_path,omitempty"`
}

// SkillGitSummary details Git status for files in this skill's directory.
type SkillGitSummary struct {
	Configured bool     `json:"configured"`
	Dirty      bool     `json:"dirty"`
	Staged     []string `json:"staged,omitempty"`
	Unstaged   []string `json:"unstaged,omitempty"`
	Untracked  []string `json:"untracked,omitempty"`
	Unmerged   []string `json:"unmerged,omitempty"`
}

// SkillReviewResult is the comprehensive, offline, read-only diagnostic review for one skill.
type SkillReviewResult struct {
	Result
	SkillID             string                      `json:"skill_id"`
	Collection          string                      `json:"collection"`
	Name                string                      `json:"name"`
	Description         string                      `json:"description"`
	LifecycleState      string                      `json:"lifecycle_state"`
	ActiveLocally       bool                        `json:"active_locally"` // Deprecated compatibility alias for lifecycle_state == "active"
	RoutingEligible     bool                        `json:"routing_eligible"`
	Valid               bool                        `json:"valid"`
	CanonicalIssues     []string                    `json:"canonical_issues,omitempty"`
	ActivationReadiness ActivationReadiness         `json:"activation_readiness"`
	ResourceStatus      SkillResourceStatus         `json:"resource_status"`
	CanonicalFacts      catalog.CanonicalSkillFacts `json:"canonical_facts"`
	ServedFacts         catalog.ServedSkillFacts    `json:"served_facts"`
	Diverged            bool                        `json:"diverged"`
	ChangedResources    []string                    `json:"changed_resources,omitempty"`
	MissingResources    []string                    `json:"missing_resources,omitempty"`
	Provenance          *SkillProvenance            `json:"provenance,omitempty"`
	Git                 SkillGitSummary             `json:"git"`
	ContentTrust        ContentTrust                `json:"content_trust"`
	RuntimeHints        skillruntime.Hints          `json:"runtime_hints"`
	NextAction          string                      `json:"next_action"`
}

// ContentTrust tells a human whether a skill's content may be served and run
// and, when review is required, which content digest to approve and how.
type ContentTrust struct {
	ThirdParty     bool     `json:"third_party"`
	Approved       bool     `json:"approved"`
	ContentDigest  string   `json:"content_digest"`
	ReasonCodes    []string `json:"reason_codes,omitempty"`
	ApproveCommand string   `json:"approve_command,omitempty"`
	// ChangesSinceApproval is present when a previous approval is recorded and
	// the content no longer matches it. It is absent for a skill never approved.
	ChangesSinceApproval *ChangesSinceApproval `json:"changes_since_approval,omitempty"`
}

// RequiresReview reports whether the content is withheld from agents until a
// human approves the content digest.
func (trust ContentTrust) RequiresReview() bool {
	return len(trust.ReasonCodes) > 0
}

// ReviewSkill executes a read-only, offline review of one skill directly from canonical files
// and published generation facts without modifying workspace files or rebuilding the catalog.
func (SkillService) ReviewSkill(ctx context.Context, path, id string) (SkillReviewResult, error) {
	if err := ctx.Err(); err != nil {
		return SkillReviewResult{}, err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillReviewResult{}, err
	}

	foundCollection, skillRelDir, skillMetaBytes, err := locateSkillDir(root, id)
	if err != nil {
		return SkillReviewResult{}, err
	}

	metaDoc := parseSkillReviewMeta(skillMetaBytes)
	entrypointRelPath, entrypointDigest, entrypointBytes := inspectCanonicalEntrypoint(root, skillRelDir)
	metaDoc.Name, metaDoc.Description = catalog.DeriveSkillNameAndDesc(entrypointBytes, metaDoc.Name, metaDoc.Description, id)
	canonicalIssues, valid := checkCanonicalIssues(root, skillRelDir)
	readiness, isScaffold, missingFields := checkActivationReadiness(entrypointBytes, metaDoc, valid)
	if handle, err := catalog.OpenCurrentLocked(ctx, root); err == nil {
		defer handle.Close()
		if sqliteCat, catErr := resolverpkg.NewSQLiteCatalog(handle.DB, handle.Pointer.CatalogSnapshot); catErr == nil {
			if allSkills, skillsErr := sqliteCat.Skills(ctx); skillsErr == nil {
				var activeSkills []resolverpkg.Skill
				for _, s := range allSkills {
					if s.Status == "active" && s.ID != "system-curator" {
						activeSkills = append(activeSkills, s)
					}
				}
				if targetSkill, decErr := resolverpkg.DecodeSkillDocument(id, skillMetaBytes); decErr == nil {
					findings := resolverpkg.LintTargetSkill(targetSkill, activeSkills)
					for _, f := range findings {
						readiness.Warnings = append(readiness.Warnings, fmt.Sprintf("%s: %s Fix: %s", f.Code, f.Message, f.Fix))
					}
				}
			}
		}
	}
	fullSkillDir := filepath.Join(root, filepath.FromSlash(skillRelDir))
	resourceStatus, resources, totalBytes := inventorySkillResources(root, fullSkillDir, entrypointRelPath, entrypointDigest)

	canonicalFacts := catalog.CanonicalSkillFacts{
		Known:          true,
		Collection:     foundCollection,
		Path:           skillRelDir,
		Status:         metaDoc.Status,
		Valid:          valid,
		Issues:         canonicalIssues,
		EntrypointPath: entrypointRelPath,
	}
	servedFacts, diverged, changedResources, missingResources := assessServedSkillFacts(ctx, root, id, &canonicalFacts)
	prov := extractSkillProvenance(metaDoc)
	gitSummary := getSkillGitSummary(ctx, root, skillRelDir+"/")
	contentTrust := reviewContentTrust(id, skillRelDir, resources, skillMetaBytes)
	contentTrust.ChangesSinceApproval = reviewChangesSinceApproval(ctx, approvalDiffInput{Root: root, SkillRelDir: skillRelDir, SkillMetaBytes: skillMetaBytes, Resources: resources, Trust: contentTrust, HistoryLimit: approvalHistoryLimit})
	runtimeHints, _ := canonicalRuntimeHints(root, id)

	nextAction := computeNextAction(id, skillRelDir, metaDoc.Status, valid, canonicalIssues, readiness, isScaffold, missingFields, diverged, changedResources, missingResources, servedFacts, gitSummary)

	activeLocally := (metaDoc.Status == "active")
	routingEligible := activeLocally && (!servedFacts.Known || servedFacts.Servable)

	result := SkillReviewResult{
		Result:              NewResult(StatusOK, fmt.Sprintf("Review for skill %s: %s", id, nextAction)),
		SkillID:             id,
		Collection:          foundCollection,
		Name:                metaDoc.Name,
		Description:         metaDoc.Description,
		LifecycleState:      metaDoc.Status,
		ActiveLocally:       activeLocally,
		RoutingEligible:     routingEligible,
		Valid:               valid,
		CanonicalIssues:     canonicalIssues,
		ActivationReadiness: readiness,
		ResourceStatus:      resourceStatus,
		CanonicalFacts:      canonicalFacts,
		ServedFacts:         servedFacts,
		Diverged:            diverged,
		ChangedResources:    changedResources,
		MissingResources:    missingResources,
		Provenance:          prov,
		Git:                 gitSummary,
		ContentTrust:        contentTrust,
		RuntimeHints:        runtimeHints,
		NextAction:          nextAction,
	}

	appendSkillReviewItems(&result, len(resources), totalBytes, missingFields)
	return result, nil
}

type skillReviewMeta struct {
	Name        string `yaml:"name"`
	Status      string `yaml:"status"`
	Description string `yaml:"description"`
	Routing     struct {
		Operations      []string `yaml:"operations"`
		Triggers        []string `yaml:"triggers"`
		NotFor          []string `yaml:"not_for"`
		MinScope        string   `yaml:"min_scope"`
		Examples        []string `yaml:"examples"`
		CounterExamples []string `yaml:"counter_examples"`
	} `yaml:"routing"`
	Sources []struct {
		ID              string   `yaml:"id"`
		Roles           []string `yaml:"roles"`
		Kind            string   `yaml:"kind"`
		Repo            string   `yaml:"repo"`
		Repository      string   `yaml:"repository"`
		Ref             string   `yaml:"ref"`
		Commit          string   `yaml:"commit"`
		Path            string   `yaml:"path"`
		FilesDigest     string   `yaml:"files_digest"`
		Transformations []string `yaml:"transformations"`
		Synced          string   `yaml:"synced"`
	} `yaml:"sources"`
	Quality struct {
		Reviewed               bool   `yaml:"reviewed"`
		RoutingReviewRationale string `yaml:"routing_review_rationale"`
		ContentReviewedDigest  string `yaml:"content_reviewed_digest"`
	} `yaml:"quality"`
	Provenance struct {
		CreatedBy      string `yaml:"created_by"`
		CreatedAt      string `yaml:"created_at"`
		SourceID       string `yaml:"source_id"`
		SourceLocator  string `yaml:"source_locator"`
		SourceRevision string `yaml:"source_revision"`
		UpstreamPath   string `yaml:"upstream_path"`
		Origin         struct {
			Kind       string `yaml:"kind"`
			Repository string `yaml:"repository"`
			Ref        string `yaml:"ref"`
			Commit     string `yaml:"commit"`
			Path       string `yaml:"path"`
		} `yaml:"origin"`
	} `yaml:"provenance"`
}

func locateSkillDir(root, id string) (string, string, []byte, error) {
	skillsDir := filepath.Join(root, "skills")
	collections, err := os.ReadDir(skillsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", nil, skill.ErrNotFound
		}
		return "", "", nil, err
	}
	var foundCollection string
	var skillMetaBytes []byte
	for _, coll := range collections {
		if !coll.IsDir() {
			continue
		}
		metaFile := filepath.Join(skillsDir, coll.Name(), id, "skill.meta.yaml")
		data, readErr := os.ReadFile(metaFile)
		if readErr != nil {
			metaFile = filepath.Join(skillsDir, coll.Name(), id, ".meta", "skill.yaml")
			data, readErr = os.ReadFile(metaFile)
		}
		if readErr == nil {
			foundCollection = coll.Name()
			skillMetaBytes = data
			break
		}
	}
	if foundCollection == "" {
		return "", "", nil, skill.ErrNotFound
	}
	skillRelDir := filepath.ToSlash(filepath.Join("skills", foundCollection, id))
	return foundCollection, skillRelDir, skillMetaBytes, nil
}

func parseSkillReviewMeta(skillMetaBytes []byte) skillReviewMeta {
	var metaDoc skillReviewMeta
	_ = yaml.Unmarshal(skillMetaBytes, &metaDoc)
	if metaDoc.Status == "" {
		metaDoc.Status = "draft"
	}
	return metaDoc
}

func inspectCanonicalEntrypoint(root, skillRelDir string) (string, string, []byte) {
	entrypointRelPath := skillRelDir + "/SKILL.md"
	fullEntrypointPath := filepath.Join(root, filepath.FromSlash(entrypointRelPath))
	entrypointBytes, _ := os.ReadFile(fullEntrypointPath)
	entrypointDigest := ""
	if entrypointBytes != nil {
		sum := sha256.Sum256(entrypointBytes)
		entrypointDigest = "sha256:" + hex.EncodeToString(sum[:])
	}
	return entrypointRelPath, entrypointDigest, entrypointBytes
}

func checkCanonicalIssues(root, skillRelDir string) ([]string, bool) {
	allIssues, _ := canonical.Validate(root)
	var canonicalIssues []string
	prefix := skillRelDir + "/"
	for _, issue := range allIssues {
		if strings.HasPrefix(issue.Path, prefix) {
			canonicalIssues = append(canonicalIssues, fmt.Sprintf("%s: %s", issue.Path, issue.Message))
		}
	}
	return canonicalIssues, len(canonicalIssues) == 0
}

func checkActivationReadiness(entrypointBytes []byte, metaDoc skillReviewMeta, valid bool) (ActivationReadiness, bool, []string) {
	isScaffold := skill.IsUntouchedScaffold(entrypointBytes)
	var missingFields []string
	if isScaffold {
		missingFields = append(missingFields, "content (replace untouched scaffold instructions)")
	}
	if len(metaDoc.Routing.Triggers) == 0 {
		missingFields = append(missingFields, "trigger")
	}
	if len(metaDoc.Routing.NotFor) == 0 && strings.TrimSpace(metaDoc.Quality.RoutingReviewRationale) == "" {
		missingFields = append(missingFields, "not_for or rationale (quality.routing_review_rationale)")
	}
	if strings.TrimSpace(metaDoc.Routing.MinScope) == "" {
		missingFields = append(missingFields, "min_scope")
	}

	readiness := ActivationReadiness{
		Ready:             len(missingFields) == 0 && valid,
		UntouchedScaffold: isScaffold,
		MissingFields:     missingFields,
	}
	if metaDoc.Status == "active" && !readiness.Ready {
		readiness.Warnings = append(readiness.Warnings, "skill is marked active but has unmet activation requirements")
	}
	return readiness, isScaffold, missingFields
}

func inventorySkillResources(root, fullSkillDir, entrypointRelPath, entrypointDigest string) (SkillResourceStatus, []ResourceItem, int64) {
	var resources []ResourceItem
	var totalBytes int64
	_ = filepath.WalkDir(fullSkillDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return nil
		}
		cleanRel := filepath.ToSlash(rel)
		resources = append(resources, ResourceItem{
			Path:      cleanRel,
			Digest:    "sha256:" + hex.EncodeToString(sum[:]),
			SizeBytes: info.Size(),
		})
		totalBytes += info.Size()
		return nil
	})
	sort.Slice(resources, func(i, j int) bool { return resources[i].Path < resources[j].Path })

	resourceStatus := SkillResourceStatus{
		EntrypointPath:   entrypointRelPath,
		EntrypointDigest: entrypointDigest,
		ResourceCount:    len(resources),
		TotalBytes:       totalBytes,
		Resources:        resources,
	}
	return resourceStatus, resources, totalBytes
}

// reviewContentTrust evaluates content trust from the canonical files with the
// same rules activation applies to the catalog, so the digest shown here is the
// one a human approves with `skill edit --approve-content`.
func reviewContentTrust(skillID, skillRelDir string, resources []ResourceItem, skillMetaBytes []byte) ContentTrust {
	var document contentTrustDocument
	_ = yaml.Unmarshal(skillMetaBytes, &document)
	spec, hasSpec := reviewRuntimeSpec(skillMetaBytes)
	prefix := skillRelDir + "/"
	files := make([]skillruntime.ResourceDigest, 0, len(resources))
	for _, resource := range resources {
		if relative := strings.TrimPrefix(resource.Path, prefix); relative != resource.Path {
			files = append(files, skillruntime.ResourceDigest{Path: relative, Digest: resource.Digest})
		}
	}
	verdict := document.verdict(files, spec, hasSpec)
	result := ContentTrust{
		ThirdParty:    skillruntime.IsThirdParty(document.provenance()),
		ContentDigest: verdict.ContentDigest,
		Approved:      document.Quality.ContentReviewedDigest != "" && document.Quality.ContentReviewedDigest == verdict.ContentDigest,
		ReasonCodes:   verdict.ReasonCodes,
	}
	if verdict.RequiresReview {
		result.ApproveCommand = fmt.Sprintf("skillhub skill edit %s --approve-content %s", skillID, verdict.ContentDigest)
	}
	return result
}

// hintReadLimit bounds how much of each file the runtime hints inspect.
const hintReadLimit = 256 << 10

// reviewRuntimeHints derives static runtime hints from the canonical skill
// files. Unreadable files are skipped: the hints are advisory.
func reviewRuntimeHints(root, skillRelDir string, resources []ResourceItem, hasRuntimeBlock bool) skillruntime.Hints {
	prefix := skillRelDir + "/"
	files := make([]skillruntime.HintFile, 0, len(resources))
	for _, resource := range resources {
		relative := strings.TrimPrefix(resource.Path, prefix)
		if relative == resource.Path {
			continue
		}
		files = append(files, skillruntime.HintFile{Path: relative, Content: readLeadingBytes(filepath.Join(root, filepath.FromSlash(resource.Path)), hintReadLimit)})
	}
	return skillruntime.AnalyzeHints(files, hasRuntimeBlock)
}

func readLeadingBytes(path string, limit int64) []byte {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return nil
	}
	return content
}

// reviewRuntimeSpec parses the runtime block from skill.meta.yaml. An invalid
// block is already reported as a canonical issue, so it is treated as absent.
func reviewRuntimeSpec(skillMetaBytes []byte) (skillruntime.Spec, bool) {
	var document map[string]any
	if err := yaml.Unmarshal(skillMetaBytes, &document); err != nil {
		return skillruntime.Spec{}, false
	}
	encoded, err := json.Marshal(map[string]any{"runtime": document["runtime"]})
	if err != nil {
		return skillruntime.Spec{}, false
	}
	spec, hasSpec, err := skillruntime.ParseSpec(encoded)
	if err != nil {
		return skillruntime.Spec{}, false
	}
	return spec, hasSpec
}

func assessServedSkillFacts(ctx context.Context, root, id string, canonicalFacts *catalog.CanonicalSkillFacts) (catalog.ServedSkillFacts, bool, []string, []string) {
	assessment, assessErr := catalog.AssessSkillState(ctx, root, id)
	if assessErr != nil {
		return catalog.ServedSkillFacts{Known: false}, false, nil, nil
	}
	if len(canonicalFacts.Issues) == 0 && len(assessment.Canonical.Issues) > 0 {
		canonicalFacts.Issues = assessment.Canonical.Issues
		canonicalFacts.Valid = assessment.Canonical.Valid
	}
	return assessment.Served, assessment.Diverged, assessment.ChangedResources, assessment.MissingResources
}

func extractSkillProvenance(metaDoc skillReviewMeta) *SkillProvenance {
	if len(metaDoc.Sources) > 0 {
		for i := range metaDoc.Sources {
			for _, r := range metaDoc.Sources[i].Roles {
				if r == "upstream" {
					src := &metaDoc.Sources[i]
					repo := src.Repository
					if repo == "" {
						repo = src.Repo
					}
					return &SkillProvenance{
						SourceID:       src.ID,
						SourceLocator:  repo,
						SourceRevision: src.Commit,
						UpstreamPath:   src.Path,
					}
				}
			}
		}
		return nil
	}
	if metaDoc.Provenance.CreatedBy == "" && metaDoc.Provenance.SourceID == "" && metaDoc.Provenance.SourceLocator == "" && metaDoc.Provenance.Origin.Repository == "" {
		return nil
	}
	sourceLocator := metaDoc.Provenance.SourceLocator
	if sourceLocator == "" && metaDoc.Provenance.Origin.Repository != "" {
		sourceLocator = metaDoc.Provenance.Origin.Repository
	}
	sourceRevision := metaDoc.Provenance.SourceRevision
	if sourceRevision == "" && metaDoc.Provenance.Origin.Commit != "" {
		sourceRevision = metaDoc.Provenance.Origin.Commit
	}
	upstreamPath := metaDoc.Provenance.UpstreamPath
	if upstreamPath == "" && metaDoc.Provenance.Origin.Path != "" {
		upstreamPath = metaDoc.Provenance.Origin.Path
	}
	return &SkillProvenance{
		CreatedBy:      metaDoc.Provenance.CreatedBy,
		CreatedAt:      metaDoc.Provenance.CreatedAt,
		SourceID:       metaDoc.Provenance.SourceID,
		SourceLocator:  sourceLocator,
		SourceRevision: sourceRevision,
		UpstreamPath:   upstreamPath,
	}
}

func getSkillGitSummary(ctx context.Context, root, prefix string) SkillGitSummary {
	var gitSummary SkillGitSummary
	pathSummary, gitErr := (WorkspaceService{}).GetGitPathSummary(ctx, root)
	if gitErr == nil && pathSummary.Configured {
		gitSummary.Configured = true
		for _, f := range pathSummary.Staged {
			if strings.HasPrefix(f.Path, prefix) {
				gitSummary.Staged = append(gitSummary.Staged, f.Path)
			}
		}
		for _, f := range pathSummary.Unstaged {
			if strings.HasPrefix(f.Path, prefix) {
				gitSummary.Unstaged = append(gitSummary.Unstaged, f.Path)
			}
		}
		for _, f := range pathSummary.Untracked {
			if strings.HasPrefix(f.Path, prefix) {
				gitSummary.Untracked = append(gitSummary.Untracked, f.Path)
			}
		}
		for _, f := range pathSummary.Unmerged {
			if strings.HasPrefix(f.Path, prefix) {
				gitSummary.Unmerged = append(gitSummary.Unmerged, f.Path)
			}
		}
		gitSummary.Dirty = len(gitSummary.Staged)+len(gitSummary.Unstaged)+len(gitSummary.Untracked)+len(gitSummary.Unmerged) > 0
	}
	return gitSummary
}

func appendSkillReviewItems(result *SkillReviewResult, resCount int, totalBytes int64, missingFields []string) {
	statusImpact := fmt.Sprintf("Lifecycle: %s.", result.LifecycleState)
	if result.ServedFacts.Known {
		statusImpact += fmt.Sprintf(" Served generation: %s.", result.ServedFacts.Generation)
	} else {
		statusImpact += " Not currently served in published generation."
	}
	result.Items = append(result.Items, Item{ID: "status", Summary: result.LifecycleState, Impact: statusImpact})

	validSummary := "Canonical files valid."
	if !result.Valid {
		validSummary = fmt.Sprintf("%d canonical issue(s) detected.", len(result.CanonicalIssues))
	}
	result.Items = append(result.Items, Item{ID: "validity", Summary: validSummary, Impact: "Validated offline against canonical schema."})

	readinessSummary := "Ready for activation."
	if !result.ActivationReadiness.Ready {
		readinessSummary = fmt.Sprintf("Not ready for activation: missing %s.", strings.Join(missingFields, ", "))
	}
	result.Items = append(result.Items, Item{ID: "readiness", Summary: readinessSummary, Impact: "Activation requirements check."})

	resSummary := fmt.Sprintf("%d canonical file(s), %d bytes.", resCount, totalBytes)
	if result.Diverged {
		resSummary += fmt.Sprintf(" Diverged from served generation (%d modified, %d missing).", len(result.ChangedResources), len(result.MissingResources))
	}
	result.Items = append(result.Items, Item{ID: "resources", Summary: resSummary, Impact: "Disk resource inventory."})

	if result.Git.Configured {
		gitText := "Git clean."
		if result.Git.Dirty {
			gitText = fmt.Sprintf("Git changes: %d staged, %d unstaged, %d untracked.", len(result.Git.Staged), len(result.Git.Unstaged), len(result.Git.Untracked))
		}
		result.Items = append(result.Items, Item{ID: "git", Summary: gitText, Impact: "Repository change tracking."})
	}

	result.Items = append(result.Items, Item{ID: "next_action", Summary: result.NextAction, Impact: "Recommended next step."})
}

func computeNextAction(id, relDir, status string, valid bool, canonicalIssues []string, readiness ActivationReadiness, isScaffold bool, missingFields []string, diverged bool, changed, missing []string, served catalog.ServedSkillFacts, git SkillGitSummary) string {
	if !valid {
		return fmt.Sprintf("Fix %d canonical validation error(s) in %s.", len(canonicalIssues), relDir)
	}
	switch status {
	case "draft":
		if isScaffold {
			return fmt.Sprintf("Edit instructions with `skillhub skill edit %s --editor` to replace the untouched scaffold template.", id)
		}
		if len(missingFields) > 0 {
			return fmt.Sprintf("Complete required activation fields (%s) with `skillhub skill edit %s`.", strings.Join(missingFields, ", "), id)
		}
		return fmt.Sprintf("Activate skill with `skillhub skill activate %s --yes`.", id)
	case "active":
		if diverged {
			return fmt.Sprintf("Rebuild catalog with `skillhub rebuild` to publish %d modified canonical file(s).", len(changed)+len(missing))
		}
		if served.Known && !served.Servable {
			return fmt.Sprintf("Resolve servability issue: %s.", served.ServableReason)
		}
		if git.Dirty {
			return "Commit canonical changes with `git -C <ws> commit`."
		}
		return "Skill is active and available for agent routing."
	case "deprecated":
		return fmt.Sprintf("Skill is deprecated; archive with `skillhub skill archive %s --yes` when no longer needed.", id)
	case "archived":
		return "Skill is archived."
	default:
		return "Review skill configuration."
	}
}
