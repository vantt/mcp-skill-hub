package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// SourceWatchInput specifies the desired watch policy for a public GitHub source.
type SourceWatchInput struct {
	Locator           string `json:"locator"`
	SkillID           string `json:"skill_id,omitempty"`
	SourceID          string `json:"source_id,omitempty"`
	Ref               string `json:"ref,omitempty"`
	Path              string `json:"path,omitempty"`
	Cadence           string `json:"cadence,omitempty"`
	MonitoringEnabled *bool  `json:"monitoring_enabled,omitempty"`
	Trust             string `json:"trust,omitempty"`
	License           string `json:"license,omitempty"`
	IdempotencyKey    string `json:"idempotency_key,omitempty"`
}

// PreviewSourceWatch creates an immutable, reviewed proposal to watch a public GitHub repository.
// Local paths return ErrorLocalWatchUnsupported.
func (service SourceService) PreviewSourceWatch(ctx context.Context, path string, input SourceWatchInput) (SourceProposal, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceProposal{}, err
	}
	service = service.defaults(root)

	rawLocator := strings.TrimSpace(input.Locator)
	if valProp := validateSourceWatchLocator(rawLocator); valProp != nil {
		return *valProp, nil
	}

	skillID := strings.TrimSpace(input.SkillID)
	if skillID == "" {
		prop := SourceProposal{
			Result: ErrorResult(NewInvalidRequestError(
				"A watched source must belong to a skill.",
				"Pass --skill-id <id> to use it as a learning reference, run `skillhub skill add <locator>` to vendor its skills, or `skillhub source capture <locator> --reason <text>` to save it for later.",
			)),
		}
		return prop, nil
	}

	return service.PreviewAttach(ctx, path, SourceAttachInput{
		SkillID:           skillID,
		SourceID:          input.SourceID,
		Locator:           input.Locator,
		Ref:               input.Ref,
		Path:              input.Path,
		Cadence:           input.Cadence,
		MonitoringEnabled: input.MonitoringEnabled,
		Trust:             input.Trust,
		License:           input.License,
		IdempotencyKey:    input.IdempotencyKey,
	})
}

func validateSourceWatchLocator(rawLocator string) *SourceProposal {
	if rawLocator == "" {
		prop := SourceProposal{
			Result: ErrorResult(NewInvalidRequestError("source locator is required", "Provide a public GitHub repository URL.")),
		}
		return &prop
	}
	if !strings.Contains(rawLocator, "://") || strings.HasPrefix(rawLocator, "./") || strings.HasPrefix(rawLocator, "../") || strings.HasPrefix(rawLocator, "~/") || filepath.IsAbs(rawLocator) {
		prop := SourceProposal{
			Result: ErrorResult(NewLocalWatchUnsupportedError(
				"local folders are machine-specific; watch is supported for public GitHub repositories only",
				"re-run skill add after updating",
			)),
		}
		return &prop
	}
	return nil
}

func resolveSourceWatchRoute(inspectionContext context.Context, adapter sourcepkg.Adapter, rawLocator, refInput, pathInput string) (*sourcepkg.ResolvedGitHubRoute, *SourceProposal) {
	allowFile := false
	if ga, isGit := adapter.(sourcepkg.GitRepositoryAdapter); isGit {
		allowFile = ga.AllowFileProtocol
	}

	route, err := sourcepkg.ParseGitHubLocatorWithOptions(rawLocator, refInput, pathInput, sourcepkg.GitHubLocatorOptions{AllowFile: allowFile})
	if err != nil {
		prop := SourceProposal{
			Result: ErrorResult(NewInvalidRequestError(err.Error(), "Provide a valid public GitHub URL (e.g. https://github.com/owner/repo).")),
		}
		return nil, &prop
	}

	if ga, isGit := adapter.(sourcepkg.GitRepositoryAdapter); isGit {
		resolved, resolveErr := sourcepkg.ResolveGitHubRoute(inspectionContext, ga, route)
		if resolveErr != nil {
			var ambErr *sourcepkg.AmbiguousRefError
			if errors.As(resolveErr, &ambErr) {
				prop := SourceProposal{
					Result: ErrorResult(NewAmbiguousRefError(
						ambErr.Error(),
						"Specify an unambiguous --ref or --path flag.",
					)),
				}
				return nil, &prop
			}
			prop := SourceProposal{
				Result: ErrorResult(NewInvalidRequestError(
					resolveErr.Error(),
					"Verify the GitHub repository and ref exist, or specify --ref explicitly.",
				)),
			}
			return nil, &prop
		}
		return resolved, nil
	}

	ref := route.Ref
	if ref == "" {
		ref = "main"
	}
	path := route.Path
	if path == "" && route.Rest != "" {
		path = route.Rest
	}
	return &sourcepkg.ResolvedGitHubRoute{
		Repository: route.Repository,
		Ref:        ref,
		Path:       path,
		Commit:     "0123456789abcdef0123456789abcdef01234567",
	}, nil
}

type sourceWatchConfig struct {
	locator           sourcepkg.Locator
	sourceID          string
	monitoringEnabled bool
	cadence           string
	trust             string
}

func deriveSourceWatchConfig(input SourceWatchInput, resolved *sourcepkg.ResolvedGitHubRoute) (sourceWatchConfig, *SourceProposal) {
	monitoringEnabled := true
	if input.MonitoringEnabled != nil {
		monitoringEnabled = *input.MonitoringEnabled
	}
	cadence := input.Cadence
	if cadence == "" {
		if !monitoringEnabled {
			cadence = "manual"
		} else {
			cadence = "weekly"
		}
	} else if !monitoringEnabled && cadence != "manual" {
		cadence = "manual"
	}
	trust := input.Trust
	if trust == "" {
		trust = "community"
	}

	locator := sourcepkg.Locator{
		Repository: resolved.Repository,
		Ref:        resolved.Ref,
		Path:       resolved.Path,
	}

	sourceID := strings.TrimSpace(input.SourceID)
	if sourceID != "" {
		if !safeSourceID(sourceID) {
			prop := SourceProposal{
				Result: ErrorResult(NewInvalidRequestError(
					"invalid source ID: "+sourceID,
					"Use lowercase letters, numbers, hyphens, and underscores (max 128 characters).",
				)),
			}
			return sourceWatchConfig{}, &prop
		}
	} else {
		sourceID = deriveSourceID(resolved.Repository, resolved.Path)
	}

	return sourceWatchConfig{
		locator:           locator,
		sourceID:          sourceID,
		monitoringEnabled: monitoringEnabled,
		cadence:           cadence,
		trust:             trust,
	}, nil
}

func checkExistingSourceCollisions(records []sourcepkg.Record, cfg sourceWatchConfig) *SourceProposal {
	for _, rec := range records {
		sameID := (rec.ID == cfg.sourceID)
		sameLocator := (sameRepository(rec.Locator.Repository, cfg.locator.Repository) && rec.Locator.Ref == cfg.locator.Ref && rec.Locator.Path == cfg.locator.Path)
		exactPolicy := (rec.Monitoring.Enabled == cfg.monitoringEnabled && rec.Monitoring.Cadence == cfg.cadence)

		if sameID {
			if sameLocator && exactPolicy {
				prop := SourceProposal{
					Result: NewResult(StatusOK, fmt.Sprintf("Source %q is already being watched with identical configuration.", cfg.sourceID)),
					Source: rec,
					Diff:   SourceDiff{Added: []string{}, Modified: []string{}, Deleted: []string{}},
					Confirmation: ConfirmationPolicy{
						PolicyRevision:     "policy_v1",
						ActionClass:        "informational",
						ApplicationCommand: "PreviewSourceWatch",
						Confirmation:       ConfirmationRequirement{Required: false},
					},
				}
				return &prop
			}
			prop := SourceProposal{
				Result: ErrorResult(NewSourceConflictError(
					fmt.Sprintf("source %q already exists with different configuration", cfg.sourceID),
					"Specify a different --source-id with `--source-id <id>`, or re-run with matching policy.",
				)),
			}
			return &prop
		}

		if sameLocator {
			if exactPolicy {
				prop := SourceProposal{
					Result: NewResult(StatusOK, fmt.Sprintf("Source locator is already being watched under ID %q.", rec.ID)),
					Source: rec,
					Diff:   SourceDiff{Added: []string{}, Modified: []string{}, Deleted: []string{}},
					Confirmation: ConfirmationPolicy{
						PolicyRevision:     "policy_v1",
						ActionClass:        "informational",
						ApplicationCommand: "PreviewSourceWatch",
						Confirmation:       ConfirmationRequirement{Required: false},
					},
				}
				return &prop
			}
			prop := SourceProposal{
				Result: ErrorResult(NewSourceConflictError(
					fmt.Sprintf("source locator is already configured under ID %q with different policy", rec.ID),
					fmt.Sprintf("Update the existing source or pass matching policy for %s.", rec.ID),
				)),
			}
			return &prop
		}
	}
	return nil
}

func buildSourceWatchRecord(inspectionContext context.Context, adapter sourcepkg.Adapter, cfg sourceWatchConfig, licenseInput string) (sourcepkg.Record, sourcepkg.Revision, *SourceProposal, error) {
	identity, err := adapter.Identify(inspectionContext, cfg.locator)
	if err != nil {
		prop := SourceProposal{
			Result: ErrorResult(NewInvalidRequestError("failed to identify repository: "+err.Error(), "Verify repository is accessible.")),
		}
		return sourcepkg.Record{}, sourcepkg.Revision{}, &prop, nil
	}
	revision, err := adapter.CurrentRevision(inspectionContext, sourcepkg.Source{ID: cfg.sourceID, Locator: cfg.locator})
	if err != nil {
		prop := SourceProposal{
			Result: ErrorResult(NewInvalidRequestError("failed to determine revision: "+err.Error(), "Verify repository and ref exist.")),
		}
		return sourcepkg.Record{}, sourcepkg.Revision{}, &prop, nil
	}

	license := licenseInput
	if license == "" {
		license = identity.License
	}

	record := sourcepkg.Record{
		SchemaVersion: 1,
		ID:            cfg.sourceID,
		Adapter:       "git",
		Locator:       cfg.locator,
		Status:        "watching",
		Identity:      identity,
		License:       license,
		Trust:         sourcepkg.Trust{Source: cfg.trust},
		Monitoring:    sourcepkg.Monitoring{Enabled: cfg.monitoringEnabled, Cadence: cfg.cadence},
		Limits: sourcepkg.Limits{
			TimeoutSeconds: int(sourcepkg.DefaultTimeout / time.Second),
			MaxBytes:       sourcepkg.DefaultMaxBytes,
			MaxFiles:       sourcepkg.DefaultMaxFiles,
			MaxFileBytes:   sourcepkg.DefaultMaxFileSize,
		},
		CurrentRevision: &revision,
	}

	if _, err := sourcepkg.ParseRecord(mustYAML(record)); err != nil {
		return sourcepkg.Record{}, sourcepkg.Revision{}, nil, err
	}
	return record, revision, nil, nil
}

// ConfirmSourceWatch confirms a reviewed source watch proposal and commits the catalog source record.
func (service SourceService) ConfirmSourceWatch(ctx context.Context, path string, preview SourceProposal, pins ConfirmationPins) (SourceMutationResult, error) {
	if service.Clock == nil {
		service.Clock = SystemClock{}
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return SourceMutationResult{}, err
	}

	switch verifyProposalPins(service.Clock.Now().UTC(), preview.expiresAt, true, true, preview.Confirmation.Confirmation.Pins, pins) {
	case proposalExpired:
		return SourceMutationResult{
			Result: ErrorResult(NewStaleProposalError("The reviewed source watch proposal expired.", "Regenerate the watch proposal.")),
		}, nil
	case proposalDigestMismatch:
		return SourceMutationResult{
			Result: ErrorResult(NewStaleProposalError("Proposal digest does not match the preview.", "Pass the exact proposal digest printed by preview.")),
		}, nil
	case proposalPinsMismatch:
		return SourceMutationResult{
			Result: ErrorResult(NewStaleProposalError("Confirmation pins do not match the reviewed proposal.", "Confirm with exact proposal ID, digest, and base version.")),
		}, nil
	}

	// Verify proposal exists in storage
	stored, err := loadSourceProposal(root, preview.planned.ID, service.Clock.Now().UTC())
	if err != nil {
		return SourceMutationResult{
			Result: ErrorResult(NewStaleProposalError("Stored source watch proposal was not found or has expired.", "Regenerate the watch proposal.")),
		}, nil
	}
	if stored.planned.Digest != preview.planned.Digest {
		return SourceMutationResult{
			Result: ErrorResult(NewStaleProposalError("Stored proposal digest does not match the preview.", "Regenerate the watch proposal.")),
		}, nil
	}

	// Check catalog for idempotency or conflict
	_, records, err := readSourceRecords(root)
	if err != nil {
		return SourceMutationResult{}, err
	}
	for _, rec := range records {
		if rec.ID == preview.Source.ID {
			sameLocator := (rec.Locator.Repository == preview.Source.Locator.Repository && rec.Locator.Ref == preview.Source.Locator.Ref && rec.Locator.Path == preview.Source.Locator.Path)
			exactPolicy := (rec.Monitoring == preview.Source.Monitoring)
			if sameLocator && exactPolicy {
				return SourceMutationResult{
					Result:   NewResult(StatusOK, fmt.Sprintf("Source %s is already being watched with identical configuration.", preview.Source.ID)),
					SourceID: preview.Source.ID,
				}, nil
			}
			return SourceMutationResult{
				Result: ErrorResult(NewSourceConflictError(
					fmt.Sprintf("source %s already exists with different configuration", preview.Source.ID),
					"Specify a different source ID.",
				)),
			}, nil
		}
	}

	receipt, err := confirmAndPublish(ctx, root, preview.planned)
	if err != nil {
		if errors.Is(err, mutation.ErrConflict) {
			return SourceMutationResult{
				Result: ErrorResult(NewStaleProposalError("Canonical source state changed after preview.", "Regenerate and review the watch proposal.")),
			}, nil
		}
		return SourceMutationResult{}, err
	}

	return sourceMutationResult("Watching "+preview.Source.ID+".", preview.Source.ID, receipt), nil
}

func deriveSourceID(repoURL, subpath string) string {
	u, err := url.Parse(repoURL)
	base := ""
	if err == nil && u.Path != "" {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 {
			base = strings.TrimSuffix(parts[1], ".git")
		} else if len(parts) == 1 {
			base = strings.TrimSuffix(parts[0], ".git")
		}
	}
	if base == "" {
		base = strings.TrimSuffix(filepath.Base(repoURL), ".git")
	}
	base = sanitizeSourceID(base)
	if subpath != "" && subpath != "." {
		cleanSub := sanitizeSourceID(filepath.Base(subpath))
		if cleanSub != "" && cleanSub != base {
			base = base + "-" + cleanSub
		}
	}
	if len(base) > 64 {
		base = base[:64]
	}
	if base == "" {
		base = "source"
	}
	return base
}

func sanitizeSourceID(value string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	return strings.Trim(sb.String(), "-")
}
