package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
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
	NextAction          string                      `json:"next_action"`
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

	// 1. Locate the skill directory directly from canonical storage
	skillsDir := filepath.Join(root, "skills")
	collections, err := os.ReadDir(skillsDir)
	if err != nil {
		return SkillReviewResult{}, err
	}

	var foundCollection string
	var skillMetaBytes []byte
	for _, coll := range collections {
		if !coll.IsDir() {
			continue
		}
		metaFile := filepath.Join(skillsDir, coll.Name(), id, "skill.meta.yaml")
		data, readErr := os.ReadFile(metaFile)
		if readErr == nil {
			foundCollection = coll.Name()
			skillMetaBytes = data
			break
		}
	}
	if foundCollection == "" {
		return SkillReviewResult{}, skill.ErrNotFound
	}

	skillRelDir := filepath.ToSlash(filepath.Join("skills", foundCollection, id))
	fullSkillDir := filepath.Join(root, filepath.FromSlash(skillRelDir))

	// 2. Parse skill.meta.yaml
	var metaDoc struct {
		Name        string `yaml:"name"`
		Status      string `yaml:"status"`
		Description string `yaml:"description"`
		Routing     struct {
			Operations []string `yaml:"operations"`
			Triggers   []string `yaml:"triggers"`
			NotFor     []string `yaml:"not_for"`
			MinScope   string   `yaml:"min_scope"`
		} `yaml:"routing"`
		Quality struct {
			Reviewed               bool   `yaml:"reviewed"`
			RoutingReviewRationale string `yaml:"routing_review_rationale"`
		} `yaml:"quality"`
		Provenance struct {
			CreatedBy      string `yaml:"created_by"`
			CreatedAt      string `yaml:"created_at"`
			SourceID       string `yaml:"source_id"`
			SourceLocator  string `yaml:"source_locator"`
			SourceRevision string `yaml:"source_revision"`
			UpstreamPath   string `yaml:"upstream_path"`
		} `yaml:"provenance"`
	}
	_ = yaml.Unmarshal(skillMetaBytes, &metaDoc)
	if metaDoc.Status == "" {
		metaDoc.Status = "draft"
	}

	// 3. Read canonical entrypoint (SKILL.md)
	entrypointRelPath := skillRelDir + "/SKILL.md"
	fullEntrypointPath := filepath.Join(root, filepath.FromSlash(entrypointRelPath))
	entrypointBytes, _ := os.ReadFile(fullEntrypointPath)

	entrypointDigest := ""
	if entrypointBytes != nil {
		sum := sha256.Sum256(entrypointBytes)
		entrypointDigest = "sha256:" + hex.EncodeToString(sum[:])
	}

	// 4. Validate canonical rules offline
	allIssues, _ := canonical.Validate(root)
	var canonicalIssues []string
	prefix := skillRelDir + "/"
	for _, issue := range allIssues {
		if strings.HasPrefix(issue.Path, prefix) {
			canonicalIssues = append(canonicalIssues, fmt.Sprintf("%s: %s", issue.Path, issue.Message))
		}
	}
	valid := len(canonicalIssues) == 0

	// 5. Activation readiness
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

	// 6. Canonical resource inventory
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

	// 7. Served generation facts & assessment (tolerant of broken/unavailable catalog)
	assessment, assessErr := catalog.AssessSkillState(ctx, root, id)
	canonicalFacts := catalog.CanonicalSkillFacts{
		Known:          true,
		Collection:     foundCollection,
		Path:           skillRelDir,
		Status:         metaDoc.Status,
		Valid:          valid,
		Issues:         canonicalIssues,
		EntrypointPath: entrypointRelPath,
	}
	var servedFacts catalog.ServedSkillFacts
	diverged := false
	var changedResources []string
	var missingResources []string

	if assessErr == nil {
		servedFacts = assessment.Served
		diverged = assessment.Diverged
		changedResources = assessment.ChangedResources
		missingResources = assessment.MissingResources
		if len(canonicalFacts.Issues) == 0 && len(assessment.Canonical.Issues) > 0 {
			canonicalFacts.Issues = assessment.Canonical.Issues
			canonicalFacts.Valid = assessment.Canonical.Valid
		}
	} else {
		// Tolerant fallback: catalog may be unavailable or corrupt
		servedFacts = catalog.ServedSkillFacts{Known: false}
	}

	// 8. Provenance
	var prov *SkillProvenance
	if metaDoc.Provenance.CreatedBy != "" || metaDoc.Provenance.SourceID != "" || metaDoc.Provenance.SourceLocator != "" {
		prov = &SkillProvenance{
			CreatedBy:      metaDoc.Provenance.CreatedBy,
			CreatedAt:      metaDoc.Provenance.CreatedAt,
			SourceID:       metaDoc.Provenance.SourceID,
			SourceLocator:  metaDoc.Provenance.SourceLocator,
			SourceRevision: metaDoc.Provenance.SourceRevision,
			UpstreamPath:   metaDoc.Provenance.UpstreamPath,
		}
	}

	// 9. Git status
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

	// 10. Deterministic next action
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
		NextAction:          nextAction,
	}

	// Build human-readable progressive disclosure items
	statusImpact := fmt.Sprintf("Lifecycle: %s.", metaDoc.Status)
	if servedFacts.Known {
		statusImpact += fmt.Sprintf(" Served generation: %s.", servedFacts.Generation)
	} else {
		statusImpact += " Not currently served in published generation."
	}
	result.Items = append(result.Items, Item{ID: "status", Summary: metaDoc.Status, Impact: statusImpact})

	validSummary := "Canonical files valid."
	if !valid {
		validSummary = fmt.Sprintf("%d canonical issue(s) detected.", len(canonicalIssues))
	}
	result.Items = append(result.Items, Item{ID: "validity", Summary: validSummary, Impact: "Validated offline against canonical schema."})

	readinessSummary := "Ready for activation."
	if !readiness.Ready {
		readinessSummary = fmt.Sprintf("Not ready for activation: missing %s.", strings.Join(missingFields, ", "))
	}
	result.Items = append(result.Items, Item{ID: "readiness", Summary: readinessSummary, Impact: "Activation requirements check."})

	resSummary := fmt.Sprintf("%d canonical file(s), %d bytes.", len(resources), totalBytes)
	if diverged {
		resSummary += fmt.Sprintf(" Diverged from served generation (%d modified, %d missing).", len(changedResources), len(missingResources))
	}
	result.Items = append(result.Items, Item{ID: "resources", Summary: resSummary, Impact: "Disk resource inventory."})

	if gitSummary.Configured {
		gitText := "Git clean."
		if gitSummary.Dirty {
			gitText = fmt.Sprintf("Git changes: %d staged, %d unstaged, %d untracked.", len(gitSummary.Staged), len(gitSummary.Unstaged), len(gitSummary.Untracked))
		}
		result.Items = append(result.Items, Item{ID: "git", Summary: gitText, Impact: "Repository change tracking."})
	}

	result.Items = append(result.Items, Item{ID: "next_action", Summary: nextAction, Impact: "Recommended next step."})
	return result, nil
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
