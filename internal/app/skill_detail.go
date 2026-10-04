package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

// SkillDetail is the shared read model for MCP skill_get, CLI, and WebUI.
type SkillDetail struct {
	SkillID          string             `json:"skill_id"`
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	Status           string             `json:"status"`
	Path             string             `json:"path"`
	CatalogSnapshot  string             `json:"catalog_snapshot"`
	Content          string             `json:"content"`
	ContentDigest    string             `json:"content_digest"`
	StateBasis       string             `json:"state_basis"`
	LifecycleState   string             `json:"lifecycle_state"`
	RoutingEligible  bool               `json:"routing_eligible"`
	Diverged         bool               `json:"diverged"`
	ChangedResources []string           `json:"changed_resources,omitempty"`
	MissingResources []string           `json:"missing_resources,omitempty"`
	Routing          skill.RoutingInput `json:"routing"`
	Rationale        string             `json:"rationale,omitempty"`
	Resources        []skill.Resource   `json:"resources"`
}

// ContentTrustFor evaluates the content trust of a skill in any lifecycle
// state from its canonical files: a third-party skill is trusted only when the
// recorded approval matches the current content digest. Callers that hand
// skill content to an agent use it to withhold unreviewed third-party content.
func (SkillService) ContentTrustFor(ctx context.Context, path, id string) (ContentTrust, error) {
	if err := ctx.Err(); err != nil {
		return ContentTrust{}, err
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return ContentTrust{}, err
	}
	_, skillRelDir, skillMetaBytes, err := locateSkillDir(root, id)
	if err != nil {
		return ContentTrust{}, err
	}
	entrypointRelPath, entrypointDigest, _ := inspectCanonicalEntrypoint(root, skillRelDir)
	_, resources, _ := inventorySkillResources(root, filepath.Join(root, filepath.FromSlash(skillRelDir)), entrypointRelPath, entrypointDigest)
	return reviewContentTrust(id, skillRelDir, resources, skillMetaBytes), nil
}

// GetSkillDetail returns the full detail and read model for a single skill.
// It is the shared read model for MCP skill_get, CLI, and WebUI.
func (service SkillService) GetSkillDetail(ctx context.Context, path, id string) (SkillDetail, error) {
	skillResult, err := service.ReadSkill(ctx, path, id)
	if err != nil {
		return SkillDetail{}, err
	}

	routing, err := service.ReadSkillRouting(ctx, path, id)
	if err != nil {
		return SkillDetail{}, err
	}

	rationale, rationaleErr := service.ReadSkillRationale(ctx, path, id)
	if rationaleErr != nil {
		// An unreadable rationale is treated as absent.
		rationale = ""
	}

	entrypoint := ""
	for _, res := range skillResult.Manifest.Resources {
		if strings.HasSuffix(res.Path, "/SKILL.md") {
			entrypoint = res.Path
			break
		}
	}

	sum := sha256.Sum256([]byte(skillResult.Content))
	contentDigest := "sha256:" + hex.EncodeToString(sum[:])

	assessment, assessErr := catalog.AssessSkillState(ctx, path, id)
	if assessErr != nil {
		// An assessment error falls back to empty state.
		assessment = catalog.SkillStateAssessment{}
	}

	lifecycleState := skillResult.Manifest.Status
	if lifecycleState == "" {
		lifecycleState = assessment.Canonical.Status
	}
	routingEligible := (lifecycleState == "active") && (!assessment.Served.Known || assessment.Served.Servable)

	return SkillDetail{
		SkillID:          skillResult.Manifest.SkillID,
		Name:             skillResult.Manifest.Name,
		Description:      skillResult.Manifest.Description,
		Status:           skillResult.Manifest.Status,
		Path:             entrypoint,
		CatalogSnapshot:  skillResult.Manifest.CatalogSnapshot,
		Content:          skillResult.Content,
		ContentDigest:    contentDigest,
		StateBasis:       string(catalog.BasisCanonical),
		LifecycleState:   lifecycleState,
		RoutingEligible:  routingEligible,
		Diverged:         assessment.Diverged,
		ChangedResources: assessment.ChangedResources,
		MissingResources: assessment.MissingResources,
		Routing:          routing,
		Rationale:        rationale,
		Resources:        skillResult.Manifest.Resources,
	}, nil
}
