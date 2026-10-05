package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

type SourceAttachInput struct {
	SkillID           string `json:"skill_id"`
	SourceID          string `json:"source_id,omitempty"`
	Locator           string `json:"locator,omitempty"`
	Ref               string `json:"ref,omitempty"`
	Path              string `json:"path,omitempty"`
	Cadence           string `json:"cadence,omitempty"`
	MonitoringEnabled *bool  `json:"monitoring_enabled,omitempty"`
	Trust             string `json:"trust,omitempty"`
	License           string `json:"license,omitempty"`
	IdempotencyKey    string `json:"idempotency_key,omitempty"`
}

func readSkillSourceLinks(root string) ([]sourcepkg.Link, error) {
	dir := filepath.Join(root, "sources", "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var links []sourcepkg.Link
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var link sourcepkg.Link
		if err := yaml.Unmarshal(data, &link); err != nil {
			continue
		}
		links = append(links, link)
	}
	return links, nil
}

func learningSourceIDs(links []sourcepkg.Link) map[string]bool {
	set := make(map[string]bool)
	for _, link := range links {
		if link.Role == "learning-source" || link.Role == "inspiration" {
			set[link.SourceID] = true
		}
	}
	return set
}

func linkedSkills(root, sourceID string) ([]string, error) {
	linked := make(map[string]bool)

	allSkills, err := listAllSkillIDs(root)
	if err == nil {
		for _, id := range allSkills {
			_, _, metaBytes, err := locateSkillDir(root, id)
			if err != nil || len(metaBytes) == 0 {
				continue
			}
			var meta struct {
				Provenance struct {
					SourceID string `yaml:"source_id"`
				} `yaml:"provenance"`
			}
			if yaml.Unmarshal(metaBytes, &meta) == nil && meta.Provenance.SourceID == sourceID {
				linked[id] = true
			}
		}
	}

	links, err := readSkillSourceLinks(root)
	if err == nil {
		for _, l := range links {
			if l.SourceID == sourceID && l.SkillID != "" {
				linked[l.SkillID] = true
			}
		}
	}

	result := make([]string, 0, len(linked))
	for id := range linked {
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

func sourceHasExternalReferences(root, sourceID string) bool {
	distillSourceDir := filepath.Join(root, "distill", "sources", sourceID)
	for _, sub := range []string{"observations", "runs"} {
		if entries, err := os.ReadDir(filepath.Join(distillSourceDir, sub)); err == nil && len(entries) > 0 {
			return true
		}
	}

	handle, err := catalog.OpenCurrent(context.Background(), root)
	if err != nil {
		return false
	}
	defer handle.Close()

	var count int
	pattern := fmt.Sprintf(`%%"source_id":"%s"%%`, sourceID)
	err = handle.DB.QueryRow(`SELECT count(*) FROM canonical_entities WHERE kind NOT IN ('skill', 'skill_source_link') AND (json_extract(content_json, '$.source_id') = ? OR content_json LIKE ?)`, sourceID, pattern).Scan(&count)
	return err == nil && count > 0
}

func (service SourceService) PreviewAttach(ctx context.Context, path string, input SourceAttachInput) (SourceProposal, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceProposal{}, err
	}
	service = service.defaults(root)

	skillID := strings.TrimSpace(input.SkillID)
	if skillID == "" {
		return SourceProposal{Result: ErrorResult(NewInvalidRequestError("skill_id is required", "Provide a target skill ID."))}, nil
	}
	if _, _, _, err := locateSkillDir(root, skillID); err != nil {
		return SourceProposal{Result: ErrorResult(NewInvalidRequestError(fmt.Sprintf("skill %q not found", skillID), "Create the skill before attaching a source."))}, nil
	}

	hasSourceID := strings.TrimSpace(input.SourceID) != ""
	hasLocator := strings.TrimSpace(input.Locator) != ""
	if !hasSourceID && !hasLocator {
		return SourceProposal{Result: ErrorResult(NewInvalidRequestError("specify either source_id or locator", "Pass either source_id to link an existing source or locator to watch a new source."))}, nil
	}

	_, records, err := readSourceRecords(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return SourceProposal{}, err
	}

	sourceRecord, newSource, prop, err := service.resolveAttachSource(ctx, input, records)
	if err != nil {
		return SourceProposal{}, err
	}
	if prop != nil {
		return *prop, nil
	}

	linkID := fmt.Sprintf("LINK-%s--%s", skillID, sourceRecord.ID)
	linkPath := fmt.Sprintf("sources/skills/%s.yaml", linkID)
	if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(linkPath))); statErr == nil {
		return SourceProposal{
			Result: NewResult(StatusOK, fmt.Sprintf("Skill %q is already linked to %q.", skillID, sourceRecord.ID)),
			Source: sourceRecord,
		}, nil
	}

	var changes []mutation.Change
	diff := SourceDiff{Added: []string{}, Modified: []string{}, Deleted: []string{}}

	if newSource {
		sourceBytes, _ := sourcepkg.MarshalCanonical(sourceRecord)
		sourcePath := "sources/catalog/" + sourceRecord.ID + ".yaml"
		changes = append(changes, mutation.Change{Path: sourcePath, Contents: sourceBytes})
		diff.Added = append(diff.Added, sourcePath)
	}

	linkDoc := sourcepkg.Link{
		SchemaVersion: 1,
		ID:            linkID,
		SkillID:       skillID,
		SourceID:      sourceRecord.ID,
		Role:          "learning-source",
	}
	linkBytes, err := sourcepkg.MarshalCanonical(linkDoc)
	if err != nil {
		return SourceProposal{}, err
	}
	changes = append(changes, mutation.Change{Path: linkPath, Contents: linkBytes})
	diff.Added = append(diff.Added, linkPath)

	set := mutation.WriteSet{
		Command:        "source_attach",
		IdempotencyKey: input.IdempotencyKey,
		Changes:        changes,
	}
	planned, err := mutation.PlanMutation(root, set)
	if err != nil {
		return SourceProposal{}, err
	}

	pins := ConfirmationPins{
		ProposalID:     planned.ID,
		ProposalDigest: planned.Digest,
		BaseVersion:    planned.BaseCatalogSnapshot,
	}
	expiresAt := service.Clock.Now().UTC().Add(24 * time.Hour)
	proposal := SourceProposal{
		Result: NewResult(StatusActionRequired, fmt.Sprintf("Attach proposal for %s to %s is ready for review.", sourceRecord.ID, skillID)),
		Source: sourceRecord,
		Diff:   diff,
		Confirmation: ConfirmationPolicy{
			PolicyRevision:     "policy_v1",
			ActionClass:        "semantic",
			ApplicationCommand: "source_watch_confirm",
			Confirmation: ConfirmationRequirement{
				Required: true,
				Mode:     "preview-and-approval",
				Pins:     pins,
			},
		},
		planned:   planned,
		expiresAt: expiresAt,
	}

	if err := storeSourceProposal(root, proposal, service.Clock.Now().UTC()); err != nil {
		return SourceProposal{}, err
	}
	return proposal, nil
}

func (service SourceService) resolveAttachSource(ctx context.Context, input SourceAttachInput, records []sourcepkg.Record) (sourcepkg.Record, bool, *SourceProposal, error) {
	if strings.TrimSpace(input.Locator) != "" {
		if prop := validateSourceWatchLocator(input.Locator); prop != nil {
			return sourcepkg.Record{}, false, prop, nil
		}
		adapter, ok := service.Adapters["git"]
		if !ok {
			return sourcepkg.Record{}, false, nil, fmt.Errorf("git adapter is not configured")
		}
		route, rProp := resolveSourceWatchRoute(ctx, adapter, input.Locator, input.Ref, input.Path)
		if rProp != nil {
			return sourcepkg.Record{}, false, rProp, nil
		}
		watchInput := SourceWatchInput{
			Locator:           input.Locator,
			SourceID:          input.SourceID,
			Ref:               input.Ref,
			Path:              input.Path,
			Cadence:           input.Cadence,
			MonitoringEnabled: input.MonitoringEnabled,
			Trust:             input.Trust,
			License:           input.License,
		}
		cfg, cProp := deriveSourceWatchConfig(watchInput, route)
		if cProp != nil {
			return sourcepkg.Record{}, false, cProp, nil
		}
		if collisionProp := checkExistingSourceCollisions(records, cfg); collisionProp != nil {
			if collisionProp.Error != nil {
				return sourcepkg.Record{}, false, collisionProp, nil
			}
			return collisionProp.Source, false, nil, nil
		}
		record, _, recProp, bErr := buildSourceWatchRecord(ctx, adapter, cfg, "")
		if bErr != nil {
			return sourcepkg.Record{}, false, nil, bErr
		}
		if recProp != nil {
			return sourcepkg.Record{}, false, recProp, nil
		}
		return record, true, nil, nil
	}

	for i := range records {
		if records[i].ID == input.SourceID {
			return records[i], false, nil, nil
		}
	}
	prop := SourceProposal{Result: ErrorResult(NewInvalidRequestError(fmt.Sprintf("source %q not found", input.SourceID), "Check configured sources with skillhub source list."))}
	return sourcepkg.Record{}, false, &prop, nil
}

func (service SourceService) PreviewDetach(ctx context.Context, path, skillID, sourceID string) (SourceProposal, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceProposal{}, err
	}
	service = service.defaults(root)

	linkID := fmt.Sprintf("LINK-%s--%s", skillID, sourceID)
	linkPath := fmt.Sprintf("sources/skills/%s.yaml", linkID)
	linkData, err := readWorkspaceFile(root, linkPath)
	if err != nil {
		return SourceProposal{Result: ErrorResult(NewInvalidRequestError(fmt.Sprintf("link %s does not exist", linkID), "Verify link exists."))}, nil
	}

	changes := []mutation.Change{
		{Path: linkPath, BeforeDigest: sourcepkg.Digest(linkData), Delete: true},
	}
	diff := SourceDiff{Added: []string{}, Modified: []string{}, Deleted: []string{linkPath}}

	linked, _ := linkedSkills(root, sourceID)
	remaining := make([]string, 0, len(linked))
	for _, id := range linked {
		if id != skillID {
			remaining = append(remaining, id)
		}
	}

	sourcePath := "sources/catalog/" + sourceID + ".yaml"
	sourceData, sErr := readWorkspaceFile(root, sourcePath)
	var warnings []Warning

	if len(remaining) == 0 && sErr == nil {
		if !sourceHasExternalReferences(root, sourceID) {
			changes = append(changes, mutation.Change{
				Path:         sourcePath,
				BeforeDigest: sourcepkg.Digest(sourceData),
				Delete:       true,
			})
			diff.Deleted = append(diff.Deleted, sourcePath)
		} else {
			var rec sourcepkg.Record
			if err := yaml.Unmarshal(sourceData, &rec); err == nil {
				rec.Monitoring.Enabled = false
				rec.Monitoring.Cadence = "manual"
				updatedBytes, _ := sourcepkg.MarshalCanonical(rec)
				changes = append(changes, mutation.Change{
					Path:         sourcePath,
					BeforeDigest: sourcepkg.Digest(sourceData),
					Contents:     updatedBytes,
				})
				diff.Modified = append(diff.Modified, sourcePath)
				warnings = append(warnings, Warning{
					Code:    "source_kept_referenced",
					Summary: fmt.Sprintf("Source %s is referenced by other canonical entities; retained with monitoring disabled.", sourceID),
				})
			}
		}
	}

	set := mutation.WriteSet{
		Command: "source_detach",
		Changes: changes,
	}
	planned, err := mutation.PlanMutation(root, set)
	if err != nil {
		return SourceProposal{}, err
	}

	pins := ConfirmationPins{
		ProposalID:     planned.ID,
		ProposalDigest: planned.Digest,
		BaseVersion:    planned.BaseCatalogSnapshot,
	}
	expiresAt := service.Clock.Now().UTC().Add(24 * time.Hour)
	proposal := SourceProposal{
		Result: NewResult(StatusActionRequired, fmt.Sprintf("Detach proposal for %s from %s is ready for review.", sourceID, skillID)),
		Source: sourcepkg.Record{ID: sourceID},
		Diff:   diff,
		Confirmation: ConfirmationPolicy{
			PolicyRevision:     "policy_v1",
			ActionClass:        "semantic",
			ApplicationCommand: "source_watch_confirm",
			Confirmation: ConfirmationRequirement{
				Required: true,
				Mode:     "preview-and-approval",
				Pins:     pins,
			},
		},
		planned:   planned,
		expiresAt: expiresAt,
	}
	proposal.Warnings = warnings

	if err := storeSourceProposal(root, proposal, service.Clock.Now().UTC()); err != nil {
		return SourceProposal{}, err
	}
	return proposal, nil
}

func (service SourceService) PreviewUnwatch(ctx context.Context, path, sourceID string) (SourceProposal, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceProposal{}, err
	}
	service = service.defaults(root)

	sourcePath := "sources/catalog/" + sourceID + ".yaml"
	sourceData, err := readWorkspaceFile(root, sourcePath)
	if err != nil {
		return SourceProposal{Result: ErrorResult(NewInvalidRequestError(fmt.Sprintf("source %q not found", sourceID), "Verify source exists."))}, nil
	}

	linked, _ := linkedSkills(root, sourceID)
	hasReferences := sourceHasExternalReferences(root, sourceID)

	var changes []mutation.Change
	diff := SourceDiff{Added: []string{}, Modified: []string{}, Deleted: []string{}}

	if len(linked) == 0 && !hasReferences {
		changes = append(changes, mutation.Change{
			Path:         sourcePath,
			BeforeDigest: sourcepkg.Digest(sourceData),
			Delete:       true,
		})
		diff.Deleted = append(diff.Deleted, sourcePath)
	} else {
		var rec sourcepkg.Record
		if err := yaml.Unmarshal(sourceData, &rec); err != nil {
			return SourceProposal{}, err
		}
		rec.Monitoring.Enabled = false
		rec.Monitoring.Cadence = "manual"
		updatedBytes, _ := sourcepkg.MarshalCanonical(rec)
		changes = append(changes, mutation.Change{
			Path:         sourcePath,
			BeforeDigest: sourcepkg.Digest(sourceData),
			Contents:     updatedBytes,
		})
		diff.Modified = append(diff.Modified, sourcePath)
	}

	set := mutation.WriteSet{
		Command: "source_unwatch",
		Changes: changes,
	}
	planned, err := mutation.PlanMutation(root, set)
	if err != nil {
		return SourceProposal{}, err
	}

	pins := ConfirmationPins{
		ProposalID:     planned.ID,
		ProposalDigest: planned.Digest,
		BaseVersion:    planned.BaseCatalogSnapshot,
	}
	expiresAt := service.Clock.Now().UTC().Add(24 * time.Hour)
	proposal := SourceProposal{
		Result: NewResult(StatusActionRequired, fmt.Sprintf("Unwatch proposal for %s is ready for review.", sourceID)),
		Source: sourcepkg.Record{ID: sourceID},
		Diff:   diff,
		Confirmation: ConfirmationPolicy{
			PolicyRevision:     "policy_v1",
			ActionClass:        "semantic",
			ApplicationCommand: "source_watch_confirm",
			Confirmation: ConfirmationRequirement{
				Required: true,
				Mode:     "preview-and-approval",
				Pins:     pins,
			},
		},
		planned:   planned,
		expiresAt: expiresAt,
	}

	if err := storeSourceProposal(root, proposal, service.Clock.Now().UTC()); err != nil {
		return SourceProposal{}, err
	}
	return proposal, nil
}
