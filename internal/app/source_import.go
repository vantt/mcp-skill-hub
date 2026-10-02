package app

import (
	"context"
	"encoding/json"
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

type SourceImportService struct {
	Clock    Clock
	Adapters map[string]sourcepkg.Adapter
}

type SourceImportPreviewInput struct {
	SourceID       string   `json:"source_id"`
	Path           string   `json:"path,omitempty"`
	Skills         []string `json:"skills,omitempty"`
	IdempotencyKey string   `json:"idempotency_key,omitempty"`
}

type DiscoveredSkill struct {
	Name        string `json:"name"`
	TargetID    string `json:"target_id"`
	Collection  string `json:"collection"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Conflict    bool   `json:"conflict"`
	SkipReason  string `json:"skip_reason,omitempty"`
}

type SourceImportProposal struct {
	Result
	SourceID     string             `json:"source_id"`
	Revision     string             `json:"revision"`
	Discovered   []DiscoveredSkill  `json:"discovered"`
	Importable   []DiscoveredSkill  `json:"importable"`
	Skipped      []DiscoveredSkill  `json:"skipped"`
	Diff         SourceDiff         `json:"diff"`
	Confirmation ConfirmationPolicy `json:"confirmation"`
	planned      mutation.Proposal
	expiresAt    time.Time
}

type SourceImportResult struct {
	Result
	SourceID        string   `json:"source_id"`
	ImportedCount   int      `json:"imported_count"`
	SkippedCount    int      `json:"skipped_count"`
	ImportedIDs     []string `json:"imported_ids"`
	SkippedIDs      []string `json:"skipped_ids"`
	OperationID     string   `json:"operation_id"`
	ChangedPaths    []string `json:"changed_paths"`
	CatalogSnapshot string   `json:"catalog_snapshot"`
	Generation      string   `json:"generation"`
	GitDirty        bool     `json:"git_dirty"`
}

func (service SourceImportService) defaults(root string) SourceImportService {
	if service.Clock == nil {
		service.Clock = SystemClock{}
	}
	if service.Adapters == nil {
		service.Adapters = (SourceService{}).defaults(root).Adapters
	}
	return service
}

func (service SourceImportService) PreviewSourceImport(ctx context.Context, path string, input SourceImportPreviewInput) (SourceImportProposal, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceImportProposal{}, err
	}
	service = service.defaults(root)

	sourceID := strings.TrimSpace(input.SourceID)
	if sourceID == "" {
		return SourceImportProposal{}, errors.New("source_id is required")
	}

	_, records, err := readSourceRecords(root)
	if err != nil {
		return SourceImportProposal{}, err
	}
	var record *sourcepkg.Record
	for i := range records {
		if records[i].ID == sourceID {
			record = &records[i]
			break
		}
	}
	if record == nil {
		return SourceImportProposal{}, fmt.Errorf("source %q not found", sourceID)
	}
	if record.CurrentRevision == nil {
		return SourceImportProposal{}, fmt.Errorf("source %q has no current revision; run skillhub check %s first", sourceID, sourceID)
	}

	adapter, ok := service.Adapters[record.Adapter]
	if !ok {
		return SourceImportProposal{}, fmt.Errorf("source adapter %q is not configured", record.Adapter)
	}

	src := sourcepkg.Source{ID: record.ID, Locator: record.Locator, Limits: record.Limits}
	opCtx, cancel := context.WithTimeout(ctx, time.Duration(record.Limits.TimeoutSeconds)*time.Second)
	defer cancel()

	scopePrefix := input.Path

	resources, err := adapter.List(opCtx, src, *record.CurrentRevision, sourcepkg.Scope{Prefix: scopePrefix})
	if err != nil {
		return SourceImportProposal{}, err
	}

	existingSkills, err := listWorkspaceSkillIDs(root)
	if err != nil {
		return SourceImportProposal{}, err
	}

	// Filter requested skills map
	wanted := make(map[string]bool, len(input.Skills))
	for _, s := range input.Skills {
		wanted[strings.ToLower(strings.TrimSpace(s))] = true
	}

	discoveredItems, err := DiscoverSkillsFromResources(opCtx, AdapterResourceReader{Adapter: adapter, Source: src, Revision: *record.CurrentRevision}, resources, scopePrefix)
	if err != nil {
		return SourceImportProposal{}, err
	}

	var discovered []DiscoveredSkill
	var importable []DiscoveredSkill
	var skipped []DiscoveredSkill
	var pendingImports []DiscoveredSkillItem
	var warnings []Warning

	for _, item := range discoveredItems {
		// Check filter
		if len(wanted) > 0 && !wanted[strings.ToLower(item.Name)] && !wanted[item.TargetID] {
			continue
		}

		discItem := DiscoveredSkill{
			Name:        item.Name,
			TargetID:    item.TargetID,
			Collection:  "default",
			Description: item.Description,
			Path:        item.SkillDir,
		}

		if item.Error != "" {
			discItem.Conflict = true
			discItem.SkipReason = item.Error
			skipped = append(skipped, discItem)
		} else if existingSkills[item.TargetID] {
			discItem.Conflict = true
			discItem.SkipReason = fmt.Sprintf("Skill %q already exists in workspace", item.TargetID)
			skipped = append(skipped, discItem)
		} else {
			importable = append(importable, discItem)
			pendingImports = append(pendingImports, item)
			existingSkills[item.TargetID] = true
		}
		discovered = append(discovered, discItem)

		// License warning (BUG-16)
		if item.License.Warning != "" {
			warnings = append(warnings, Warning{
				Code:    "license_warning",
				Summary: fmt.Sprintf("Skill %s: %s", item.TargetID, item.License.Warning),
			})
		}
	}

	sort.Slice(discovered, func(i, j int) bool { return discovered[i].TargetID < discovered[j].TargetID })
	sort.Slice(importable, func(i, j int) bool { return importable[i].TargetID < importable[j].TargetID })
	sort.Slice(skipped, func(i, j int) bool { return skipped[i].TargetID < skipped[j].TargetID })
	now := service.Clock.Now().UTC()
	nowISO := now.Format(time.RFC3339Nano)

	var changes []mutation.Change
	diff := SourceDiff{Added: []string{}, Modified: []string{}, Deleted: []string{}}

	for _, pi := range pendingImports {
		targetID := pi.TargetID
		// 1. Normalized SKILL.md
		normMD, _, normErr := ensureImportedSkillFrontmatter(pi.SkillMDBytes, targetID, pi.Description)
		if normErr != nil {
			return SourceImportProposal{}, normErr
		}
		skillMDTarget := "skills/default/" + targetID + "/SKILL.md"
		changes = append(changes, mutation.Change{Path: skillMDTarget, Contents: normMD})
		diff.Added = append(diff.Added, skillMDTarget)

		// 2. skill.meta.yaml
		metaDoc := map[string]any{
			"schema_version": 1,
			"id":             targetID,
			"name":           pi.Name,
			"status":         "draft",
			"description":    pi.Description,
			"routing": map[string]any{
				"triggers":  []string{},
				"not_for":   []string{},
				"min_scope": "",
			},
			"quality": map[string]any{
				"reviewed": false,
			},
			"provenance": map[string]any{
				"created_by": "source_import",
				"source_id":  record.ID,
				"revision":   record.CurrentRevision.Value,
				"path":       pi.SkillDir,
			},
			"history": []any{
				map[string]any{
					"state":       "draft",
					"occurred_at": nowISO,
				},
			},
			"created_at": nowISO,
			"updated_at": nowISO,
		}
		metaBytes, err := yaml.Marshal(metaDoc)
		if err != nil {
			return SourceImportProposal{}, err
		}
		metaTarget := "skills/default/" + targetID + "/skill.meta.yaml"
		changes = append(changes, mutation.Change{Path: metaTarget, Contents: metaBytes})
		diff.Added = append(diff.Added, metaTarget)

		// 3. Provenance link
		linkID := "LINK-" + targetID + "--" + record.ID
		linkDoc := sourcepkg.Link{
			SchemaVersion: 1,
			ID:            linkID,
			SkillID:       targetID,
			SourceID:      record.ID,
			Role:          "origin",
		}
		linkBytes, err := sourcepkg.MarshalCanonical(linkDoc)
		if err != nil {
			return SourceImportProposal{}, err
		}
		linkTarget := "sources/skills/" + linkID + ".yaml"
		changes = append(changes, mutation.Change{Path: linkTarget, Contents: linkBytes})
		diff.Added = append(diff.Added, linkTarget)

		// 4. Companion files (BUG-04: byte-for-byte copy, no empty/binary skips)
		for _, comp := range pi.Companions {
			compData := pi.CompanionBytes[comp.Path]
			compTarget := "skills/default/" + targetID + "/" + comp.Path
			changes = append(changes, mutation.Change{Path: compTarget, Contents: compData})
			diff.Added = append(diff.Added, compTarget)
		}
	}

	var planned mutation.Proposal
	var pins ConfirmationPins
	expiresAt := now.Add(24 * time.Hour)
	status := StatusActionRequired
	if len(importable) == 0 {
		status = StatusOK
	}
	if len(changes) > 0 {
		set := mutation.WriteSet{Command: "source_import", IdempotencyKey: input.IdempotencyKey, Changes: changes}
		var err error
		planned, err = mutation.PlanMutation(root, set)
		if err != nil {
			return SourceImportProposal{}, err
		}
		pins = ConfirmationPins{ProposalID: planned.ID, ProposalDigest: planned.Digest, BaseVersion: planned.BaseCatalogSnapshot}
	}

	proposal := SourceImportProposal{
		Result:       NewResult(status, fmt.Sprintf("Found %d skill(s) (%d importable, %d skipped).", len(discovered), len(importable), len(skipped))),
		SourceID:     record.ID,
		Revision:     record.CurrentRevision.Value,
		Discovered:   discovered,
		Importable:   importable,
		Skipped:      skipped,
		Diff:         diff,
		Confirmation: ConfirmationPolicy{PolicyRevision: "policy_v1", ActionClass: "semantic", ApplicationCommand: "SourceImport", Confirmation: ConfirmationRequirement{Required: len(changes) > 0, Mode: "preview-and-approval", Pins: pins}},
		planned:      planned,
		expiresAt:    expiresAt,
	}
	proposal.Warnings = append(proposal.Warnings, warnings...)

	for _, item := range importable {
		proposal.Items = append(proposal.Items, Item{ID: item.TargetID, Summary: item.Name, Impact: "Draft skill to be created with provenance."})
	}
	for _, item := range skipped {
		proposal.Items = append(proposal.Items, Item{ID: item.TargetID, Summary: item.Name, Impact: item.SkipReason})
	}

	if len(changes) > 0 {
		if err := storeSourceImportProposal(root, proposal, now); err != nil {
			return SourceImportProposal{}, err
		}
	}

	return proposal, nil
}

func (service SourceImportService) LoadSourceImportProposal(ctx context.Context, path, id string) (SourceImportProposal, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceImportProposal{}, err
	}
	if err := ctx.Err(); err != nil {
		return SourceImportProposal{}, err
	}
	service = service.defaults(root)
	return loadSourceImportProposal(root, id, service.Clock.Now())
}

func (service SourceImportService) ConfirmSourceImport(ctx context.Context, path string, preview SourceImportProposal, pins ConfirmationPins) (SourceImportResult, error) {
	if service.Clock == nil {
		service.Clock = SystemClock{}
	}
	now := service.Clock.Now().UTC()
	switch verifyProposalPins(now, preview.expiresAt, true, true, preview.Confirmation.Confirmation.Pins, pins) {
	case proposalExpired:
		result := SourceImportResult{Result: NewResult(StatusError, "Proposal expired; nothing was applied.")}
		result.Error = &Error{Code: ErrorStaleProposal, Render: ErrorRender{Error: "The source import proposal can no longer be confirmed.", Why: "The proposal expired.", Fix: "Regenerate the import proposal."}}
		return result, nil
	case proposalDigestMismatch:
		result := SourceImportResult{Result: NewResult(StatusError, "Proposal digest does not match the preview; nothing was applied.")}
		result.Error = &Error{Code: ErrorStaleProposal, Render: ErrorRender{Error: "The source import proposal cannot be confirmed.", Why: "Proposal digest does not match the preview.", Fix: "Pass the exact proposal digest printed by the preview, or re-run with --yes."}}
		return result, nil
	case proposalPinsMismatch:
		result := SourceImportResult{Result: NewResult(StatusError, "Proposal is stale; nothing was applied.")}
		result.Error = &Error{Code: ErrorStaleProposal, Render: ErrorRender{Error: "The source import proposal can no longer be confirmed.", Why: "Confirmation pins do not match.", Fix: "Load or regenerate the import proposal."}}
		return result, nil
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceImportResult{}, err
	}
	receipt, err := confirmAndPublish(ctx, root, preview.planned)
	if errors.Is(err, mutation.ErrConflict) {
		result := SourceImportResult{Result: NewResult(StatusError, "Proposal is stale; nothing was applied.")}
		result.Error = &Error{Code: ErrorStaleProposal, Render: ErrorRender{Error: "The source import proposal can no longer be confirmed.", Why: "Canonical workspace state changed after preview.", Fix: "Regenerate the import proposal."}}
		return result, nil
	}
	if err != nil {
		return SourceImportResult{}, err
	}

	importedIDs := make([]string, 0, len(preview.Importable))
	for _, item := range preview.Importable {
		importedIDs = append(importedIDs, item.TargetID)
	}
	skippedIDs := make([]string, 0, len(preview.Skipped))
	for _, item := range preview.Skipped {
		skippedIDs = append(skippedIDs, item.TargetID)
	}

	summary := fmt.Sprintf("Imported %d draft skill(s). Next: review with `skillhub skill show <id>`, then `skillhub skill activate <id> --yes` (or ask your agent).", len(importedIDs))
	if len(importedIDs) == 1 {
		summary = fmt.Sprintf("Imported 1 draft skill (%s). Next: review with `skillhub skill show %s`, then `skillhub skill activate %s --yes` (or ask your agent).", importedIDs[0], importedIDs[0], importedIDs[0])
	} else if len(importedIDs) == 0 {
		summary = "No draft skills were imported."
	}

	result := SourceImportResult{
		Result:          NewResult(StatusApplied, summary),
		SourceID:        preview.SourceID,
		ImportedCount:   len(importedIDs),
		SkippedCount:    len(skippedIDs),
		ImportedIDs:     importedIDs,
		SkippedIDs:      skippedIDs,
		OperationID:     receipt.OperationID,
		ChangedPaths:    receipt.ChangedPaths,
		CatalogSnapshot: receipt.CatalogSnapshot,
		Generation:      receipt.Generation,
		GitDirty:        true,
	}
	for _, id := range importedIDs {
		result.Items = append(result.Items, Item{ID: id, Summary: "Imported draft skill", Impact: "Saved in draft state; review and activate before use."})
	}
	for _, id := range skippedIDs {
		result.Items = append(result.Items, Item{ID: id, Summary: "Skipped existing skill", Impact: "Skill already exists; was not modified."})
	}

	return result, nil
}

type sourceImportProposalArtifact struct {
	Version    int
	CreatedAt  time.Time
	ExpiresAt  time.Time
	SourceID   string
	Revision   string
	Discovered []DiscoveredSkill
	Importable []DiscoveredSkill
	Skipped    []DiscoveredSkill
	Diff       SourceDiff
	Planned    mutation.Proposal
}

func storeSourceImportProposal(root string, proposal SourceImportProposal, now time.Time) error {
	if !validSourceProposalID(proposal.planned.ID) {
		return errors.New("invalid proposal ID")
	}
	expiresAt := proposal.expiresAt
	if expiresAt.IsZero() {
		expiresAt = now.UTC().Add(24 * time.Hour)
	}
	artifact := sourceImportProposalArtifact{
		Version:    1,
		CreatedAt:  now.UTC(),
		ExpiresAt:  expiresAt,
		SourceID:   proposal.SourceID,
		Revision:   proposal.Revision,
		Discovered: proposal.Discovered,
		Importable: proposal.Importable,
		Skipped:    proposal.Skipped,
		Diff:       proposal.Diff,
		Planned:    proposal.planned,
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer handle.Close()
	if err := handle.MkdirAll("runtime/source-import-proposals", 0o700); err != nil {
		return err
	}
	path := "runtime/source-import-proposals/" + proposal.planned.ID + ".json"
	temporary := path + ".tmp"
	_ = handle.Remove(temporary)
	file, err := handle.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(data, '\n')); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = handle.Remove(temporary)
		return err
	}
	return handle.Rename(temporary, path)
}

func loadSourceImportProposal(root, id string, now time.Time) (SourceImportProposal, error) {
	if !validSourceProposalID(id) {
		return SourceImportProposal{}, errors.New("invalid proposal ID")
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return SourceImportProposal{}, err
	}
	defer handle.Close()
	path := "runtime/source-import-proposals/" + id + ".json"
	data, err := handle.ReadFile(path)
	if err != nil {
		return SourceImportProposal{}, err
	}
	var artifact sourceImportProposalArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return SourceImportProposal{}, err
	}
	if artifact.Version != 1 || artifact.Planned.ID != id || !now.UTC().Before(artifact.ExpiresAt) {
		return SourceImportProposal{}, errors.New("source import proposal is invalid or expired")
	}
	pins := ConfirmationPins{
		ProposalID:     artifact.Planned.ID,
		ProposalDigest: artifact.Planned.Digest,
		BaseVersion:    artifact.Planned.BaseCatalogSnapshot,
	}
	return SourceImportProposal{
		Result:       NewResult(StatusActionRequired, "Stored source import proposal is ready for confirmation."),
		SourceID:     artifact.SourceID,
		Revision:     artifact.Revision,
		Discovered:   artifact.Discovered,
		Importable:   artifact.Importable,
		Skipped:      artifact.Skipped,
		Diff:         artifact.Diff,
		Confirmation: ConfirmationPolicy{PolicyRevision: "policy_v1", ActionClass: "semantic", ApplicationCommand: "SourceImport", Confirmation: ConfirmationRequirement{Required: true, Mode: "preview-and-approval", Pins: pins}},
		planned:      artifact.Planned,
		expiresAt:    artifact.ExpiresAt,
	}, nil
}

func listWorkspaceSkillIDs(root string) (map[string]bool, error) {
	ids := map[string]bool{}
	// Check skills/ directory
	skillsDir := filepath.Join(root, "skills")
	collections, err := os.ReadDir(skillsDir)
	if err == nil {
		for _, col := range collections {
			if !col.IsDir() {
				continue
			}
			entries, err := os.ReadDir(filepath.Join(skillsDir, col.Name()))
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() {
					ids[entry.Name()] = true
				}
			}
		}
	}
	// Also check catalog database if available
	handle, err := catalog.OpenCurrent(context.Background(), root)
	if err == nil {
		defer handle.Close()
		rows, qErr := handle.DB.Query(`SELECT id FROM skills`)
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err == nil {
					ids[id] = true
				}
			}
		}
	}
	return ids, nil
}
