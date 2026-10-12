package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

// SkillCore is the runtime read model of one skill, as MCP skill_get serves
// it: identity, lifecycle, the verified SKILL.md, routing and the resource
// manifest, all read from one pinned catalog generation. It carries no
// curation judgement, so assembling it never inspects canonical state.
type SkillCore struct {
	SkillID         string             `json:"skill_id"`
	Name            string             `json:"name"`
	Description     string             `json:"description"`
	Status          string             `json:"status"`
	Path            string             `json:"path"`
	CatalogSnapshot string             `json:"catalog_snapshot"`
	Content         string             `json:"content"`
	ContentDigest   string             `json:"content_digest"`
	LifecycleState  string             `json:"lifecycle_state"`
	Routing         skill.RoutingInput `json:"routing"`
	Rationale       string             `json:"rationale,omitempty"`
	Resources       []skill.Resource   `json:"resources"`
}

// SkillDetail is the curation read model for the CLI and WebUI: the runtime
// core plus the assessment of canonical state against the served catalog.
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

// ReadSkillCore reads a skill in any lifecycle state from the pinned catalog
// generation: one catalog open, one manifest query, and the entrypoint read
// once and verified against its pinned digest. Routing and rationale come from
// the catalog's copy of the skill metadata, which the same generation
// published.
func (SkillService) ReadSkillCore(ctx context.Context, path, id string) (core SkillCore, resultErr error) {
	if !snapshotSkillIDPattern.MatchString(id) {
		return SkillCore{}, errors.New("skill id must be a lowercase kebab-case identifier")
	}
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillCore{}, err
	}
	view, err := catalog.ObserveWithFallbackLocked(ctx, root)
	if err != nil {
		return SkillCore{}, fmt.Errorf("%w: %v", skill.ErrSnapshotExpired, err)
	}
	defer closeDistributionHandle(view, &resultErr)

	row, err := loadSkillRow(ctx, view, id)
	if err != nil {
		return SkillCore{}, err
	}
	core = SkillCore{SkillID: id, Name: row.Name, Description: row.Description, Status: row.Status, LifecycleState: row.Status, CatalogSnapshot: view.Pointer.CatalogSnapshot, Resources: slices.Clone(row.Resources)}
	if row.Manifest == nil {
		return SkillCore{}, fmt.Errorf("%w: skill %s has no catalog manifest", skill.ErrSnapshotExpired, id)
	}
	routing, rationale, err := row.routingAndRationale()
	if err != nil {
		return SkillCore{}, fmt.Errorf("decode skill %s manifest: %w", id, err)
	}
	core.Routing, core.Rationale = routing, rationale
	var entrypoint *skill.Resource
	for index := range core.Resources {
		if strings.HasSuffix(core.Resources[index].Path, "/SKILL.md") {
			entrypoint = &core.Resources[index]
			break
		}
	}
	if entrypoint == nil {
		return SkillCore{}, fmt.Errorf("%w: skill has no SKILL.md manifest entry", skill.ErrSnapshotExpired)
	}
	contents, err := readPinnedResource(root, entrypoint.Path, entrypoint.Digest)
	if err != nil {
		if errors.Is(err, skill.ErrResourceDigestMismatch) {
			why := fmt.Sprintf("Current canonical content for %s no longer matches published manifest digest %s.", entrypoint.Path, entrypoint.Digest)
			fix := fmt.Sprintf("Run `skillhub maintain rebuild` to publish current canonical files, or inspect differences with `skillhub skill review %s`.", id)
			return SkillCore{}, NewResourceContentUnavailableError(why, fix)
		}
		return SkillCore{}, err
	}
	core.Path = entrypoint.Path
	core.Content = string(contents)
	core.ContentDigest = entrypoint.Digest
	return core, nil
}

// GetSkillDetail returns the full detail and read model for a single skill:
// the runtime core plus the curation assessment of its canonical state. It
// serves the CLI and WebUI; MCP skill_get serves the core alone.
func (service SkillService) GetSkillDetail(ctx context.Context, path, id string) (SkillDetail, error) {
	core, err := service.ReadSkillCore(ctx, path, id)
	if err != nil {
		return SkillDetail{}, err
	}
	assessment, assessErr := catalog.AssessSkillState(ctx, path, id)
	if assessErr != nil {
		// An assessment error falls back to empty state.
		assessment = catalog.SkillStateAssessment{}
	}
	lifecycleState := core.LifecycleState
	if lifecycleState == "" {
		lifecycleState = assessment.Canonical.Status
	}
	routingEligible := (lifecycleState == "active") && (!assessment.Served.Known || assessment.Served.Servable)

	return SkillDetail{
		SkillID:          core.SkillID,
		Name:             core.Name,
		Description:      core.Description,
		Status:           core.Status,
		Path:             core.Path,
		CatalogSnapshot:  core.CatalogSnapshot,
		Content:          core.Content,
		ContentDigest:    core.ContentDigest,
		StateBasis:       string(catalog.BasisCanonical),
		LifecycleState:   lifecycleState,
		RoutingEligible:  routingEligible,
		Diverged:         assessment.Diverged,
		ChangedResources: assessment.ChangedResources,
		MissingResources: assessment.MissingResources,
		Routing:          core.Routing,
		Rationale:        core.Rationale,
		Resources:        core.Resources,
	}, nil
}
