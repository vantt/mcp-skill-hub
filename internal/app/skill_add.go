package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

var skillTargetIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

func init() {
	RegisterProposalConfirmer(skill.ProposalKindAdd, func(ctx context.Context, path string, p skill.Proposal, pins ConfirmationPins) (any, error) {
		return (SkillAddService{}).ConfirmSkillAddProposal(ctx, path, p, pins)
	})
}

// SkillAddInput contains options for adding one or more skills from a locator.
type SkillAddInput struct {
	Locator        string `json:"locator"`                   // Raw or normalized locator (e.g. local directory, GitHub URL)
	Selection      string `json:"selection,omitempty"`       // Selected skill name or relative path when multiple skills exist (--skill)
	All            bool   `json:"all,omitempty"`             // Import all discovered skills when multiple exist (--all)
	TargetID       string `json:"target_id,omitempty"`       // Optional explicit target ID for single skill (--id)
	Collection     string `json:"collection,omitempty"`      // Target collection, defaults to "default"
	IdempotencyKey string `json:"idempotency_key,omitempty"` // Optional idempotency key
	FullDiff       bool   `json:"full_diff,omitempty"`       // Whether to include full diff
}

// SkillOrigin contains privacy-safe origin metadata matching the schema.
type SkillOrigin struct {
	Kind            string   `json:"kind" yaml:"kind"` // "local", "git", "github"
	Repository      string   `json:"repository,omitempty" yaml:"repository,omitempty"`
	Ref             string   `json:"ref,omitempty" yaml:"ref,omitempty"`
	Commit          string   `json:"commit,omitempty" yaml:"commit,omitempty"`
	Path            string   `json:"path,omitempty" yaml:"path,omitempty"`
	Name            string   `json:"name,omitempty" yaml:"name,omitempty"`
	FolderDigest    string   `json:"folder_digest,omitempty" yaml:"folder_digest,omitempty"`
	ContentDigest   string   `json:"content_digest,omitempty" yaml:"content_digest,omitempty"`
	Transformations []string `json:"transformations,omitempty" yaml:"transformations,omitempty"`
	AddedAt         string   `json:"added_at,omitempty" yaml:"added_at,omitempty"`
}

// SkillAddProposal is the preview returned before confirming a skill add.
type SkillAddProposal struct {
	Result
	SkillID         string                       `json:"skill_id"`
	SkillIDs        []string                     `json:"skill_ids,omitempty"`
	Collection      string                       `json:"collection"`
	Name            string                       `json:"name"`
	Description     string                       `json:"description"`
	Origin          SkillOrigin                  `json:"origin"`
	License         SkillLicenseInfo             `json:"license"`
	Resources       []DiscoveredCompanion        `json:"resources"`
	TotalBytes      int64                        `json:"total_bytes"`
	Transformations []string                     `json:"transformations,omitempty"`
	Diff            skill.DiffSummary            `json:"diff"`
	FullDiff        string                       `json:"full_diff,omitempty"`
	Confirmation    ConfirmationPolicy           `json:"confirmation"`
	Assessment      catalog.SkillStateAssessment `json:"assessment"`
	Replayed        bool                         `json:"replayed"`
	planned         mutation.Proposal
	alreadyApplied  *mutation.Receipt
	expiresAt       time.Time
}

// Planned returns the underlying planned mutation proposal.
func (p SkillAddProposal) Planned() mutation.Proposal {
	return p.planned
}

// ProposalID returns the immutable proposal ID.
func (p SkillAddProposal) ProposalID() string {
	return p.planned.ID
}

// Digest returns the immutable proposal digest.
func (p SkillAddProposal) Digest() string {
	return p.planned.Digest
}

// BaseSnapshot returns the pinned base catalog snapshot.
func (p SkillAddProposal) BaseSnapshot() string {
	return p.planned.BaseCatalogSnapshot
}

// SkillAddResult reports the outcome of confirming a skill add mutation.
type SkillAddResult struct {
	Result
	SkillID         string                       `json:"skill_id"`
	SkillIDs        []string                     `json:"skill_ids,omitempty"`
	Collection      string                       `json:"collection"`
	OperationID     string                       `json:"operation_id"`
	Receipt         mutation.Receipt             `json:"receipt"`
	Replayed        bool                         `json:"replayed"`
	ChangedPaths    []string                     `json:"changed_paths"`
	CatalogSnapshot string                       `json:"catalog_snapshot"`
	Generation      string                       `json:"generation"`
	GitDirty        bool                         `json:"git_dirty"`
	Assessment      catalog.SkillStateAssessment `json:"assessment"`
	Origin          SkillOrigin                  `json:"origin"`
	License         SkillLicenseInfo             `json:"license"`
	Resources       []DiscoveredCompanion        `json:"resources,omitempty"`
}

// SkillAddService coordinates preview and confirmation for direct skill additions.
type SkillAddService struct {
	Clock    Clock
	Adapters map[string]sourcepkg.Adapter
}

func (service SkillAddService) defaults(root string) SkillAddService {
	if service.Clock == nil {
		service.Clock = SystemClock{}
	}
	if service.Adapters == nil {
		service.Adapters = (SourceService{}).defaults(root).Adapters
	}
	return service
}

// skillAddRequestDigest deterministically computes the request payload digest
// across the normalized origin, folder digest, target ID, selection, collection, and transformations.
func skillAddRequestDigest(kind, folderDigest, contentDigest, targetID, selection, collection string, transformations []string) string {
	sortedTransforms := append([]string(nil), transformations...)
	sort.Strings(sortedTransforms)
	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s",
		kind, folderDigest, contentDigest, targetID, selection, collection, strings.Join(sortedTransforms, ","))
	sum := sha256.Sum256([]byte(payload))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// PreviewSkillAdd plans an atomic draft skill addition from a local or remote locator.
func (service SkillAddService) PreviewSkillAdd(ctx context.Context, path string, input SkillAddInput) (SkillAddProposal, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SkillAddProposal{}, err
	}
	service = service.defaults(root)

	// Validate collection
	collection := strings.TrimSpace(input.Collection)
	if collection == "" {
		collection = "default"
	}
	if !skillTargetIDPattern.MatchString(collection) {
		return SkillAddProposal{
			Result: ErrorResult(NewInvalidRequestError("invalid collection name", "Specify a valid lowercase collection name.")),
		}, nil
	}

	// Validate locator
	rawLocator := strings.TrimSpace(input.Locator)
	if rawLocator == "" {
		return SkillAddProposal{
			Result: ErrorResult(NewInvalidRequestError("locator is required", "Provide a local directory path or repository URL pointing to a skill.")),
		}, nil
	}

	// 1. Resolve locator and capture snapshot
	type captureResult struct {
		origin      SkillOrigin
		reader      ResourceReader
		resources   []sourcepkg.Resource
		scopePrefix string
	}

	var captured captureResult
	isRemote := strings.Contains(rawLocator, "://") || strings.HasPrefix(rawLocator, "git@") || strings.HasSuffix(rawLocator, ".git")

	if !isRemote {
		// Local folder resolution
		localPath := rawLocator
		if strings.HasPrefix(localPath, "~/") {
			home, hErr := os.UserHomeDir()
			if hErr == nil {
				localPath = filepath.Join(home, localPath[2:])
			}
		}
		if !filepath.IsAbs(localPath) {
			abs, aErr := filepath.Abs(localPath)
			if aErr == nil {
				localPath = abs
			}
		}

		authRoot, authErr := sourcepkg.NewAuthorizedLocalRoot(localPath)
		if authErr != nil {
			return SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError(
					fmt.Sprintf("cannot access local directory %q: %v", rawLocator, authErr),
					"Verify the directory exists and has readable permissions.",
				)),
			}, nil
		}

		// Capture snapshot into workspace cache
		cacheRoot := filepath.Join(root, "runtime", "cache", "snapshots")
		fsAdapter := sourcepkg.FilesystemAdapter{CacheRoot: cacheRoot}
		manifest, capErr := fsAdapter.CaptureAuthorizedRoot(ctx, authRoot, "")
		if capErr != nil {
			if errors.Is(capErr, sourcepkg.ErrUnsafeFile) {
				return SkillAddProposal{
					Result: ErrorResult(NewInvalidRequestError(capErr.Error(), "Ensure source folder does not contain unsafe symlinks or escape paths.")),
				}, nil
			}
			return SkillAddProposal{}, fmt.Errorf("capture local snapshot: %w", capErr)
		}

		// authRoot is discarded here and never stored in canonical files!
		baseName := filepath.Base(authRoot.Path())
		captured = captureResult{
			origin: SkillOrigin{
				Kind:         "local",
				Name:         sanitizeSkillID(baseName),
				FolderDigest: manifest.Digest,
			},
			reader: AdapterResourceReader{
				Adapter: fsAdapter,
				Source:  sourcepkg.Source{Locator: sourcepkg.Locator{SnapshotDigest: manifest.Digest}},
				Revision: sourcepkg.Revision{
					Kind:          "filesystem-snapshot",
					Value:         manifest.Digest,
					ContentDigest: manifest.Digest,
				},
			},
			resources: manifest.Resources,
		}
	} else {
		// Remote Git / GitHub resolution
		gitAdapter, ok := service.Adapters["git"]
		if !ok {
			return SkillAddProposal{}, errors.New("git source adapter is not configured")
		}

		allowFile := false
		if ga, isGit := gitAdapter.(sourcepkg.GitRepositoryAdapter); isGit {
			allowFile = ga.AllowFileProtocol
		}

		route, parseErr := sourcepkg.ParseGitHubLocatorWithOptions(rawLocator, "", "", sourcepkg.GitHubLocatorOptions{AllowFile: allowFile})
		if parseErr != nil {
			return SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError(parseErr.Error(), "Provide a valid GitHub repository URL.")),
			}, nil
		}

		inspectCtx, cancelInspect := context.WithTimeout(ctx, sourcepkg.DefaultTimeout)
		defer cancelInspect()

		var resolved *sourcepkg.ResolvedGitHubRoute
		if ga, isGit := gitAdapter.(sourcepkg.GitRepositoryAdapter); isGit {
			var resErr error
			resolved, resErr = sourcepkg.ResolveGitHubRoute(inspectCtx, ga, route)
			if resErr != nil {
				var ambErr *sourcepkg.AmbiguousRefError
				if errors.As(resErr, &ambErr) {
					return SkillAddProposal{
						Result: ErrorResult(NewAmbiguousRefError(ambErr.Error(), "Specify an unambiguous reference.")),
					}, nil
				}
				return SkillAddProposal{
					Result: ErrorResult(NewInvalidRequestError(resErr.Error(), "Verify repository exists and is accessible.")),
				}, nil
			}
		} else {
			ref := route.Ref
			if ref == "" {
				ref = "main"
			}
			resolved = &sourcepkg.ResolvedGitHubRoute{
				Repository: route.Repository,
				Ref:        ref,
				Path:       route.Path,
				Commit:     "0123456789abcdef0123456789abcdef01234567",
			}
		}

		ref := resolved.Ref
		if ref == "" {
			ref = resolved.Commit
		}
		src := sourcepkg.Source{
			Locator: sourcepkg.Locator{
				Repository: resolved.Repository,
				Ref:        ref,
				Path:       resolved.Path,
			},
			Limits: sourcepkg.Limits{TimeoutSeconds: 180, MaxBytes: sourcepkg.DefaultMaxBytes, MaxFiles: sourcepkg.DefaultMaxFiles, MaxFileBytes: sourcepkg.DefaultMaxFileSize},
		}

		var rev sourcepkg.Revision
		if _, isGit := gitAdapter.(sourcepkg.GitRepositoryAdapter); isGit {
			var revErr error
			rev, revErr = gitAdapter.CurrentRevision(ctx, src)
			if revErr != nil {
				return SkillAddProposal{
					Result: ErrorResult(NewInvalidRequestError("failed to determine repository revision: "+revErr.Error(), "Verify repository and ref exist.")),
				}, nil
			}
		} else {
			rev = sourcepkg.Revision{
				Kind:          "git-commit",
				Value:         resolved.Commit,
				ContentDigest: "sha256:" + resolved.Commit + strings.Repeat("0", 64-len(resolved.Commit)),
			}
			if r, err := gitAdapter.CurrentRevision(ctx, src); err == nil && r.Value != "" {
				rev = r
			}
		}

		resources, listErr := gitAdapter.List(ctx, src, rev, sourcepkg.Scope{})
		if listErr != nil {
			return SkillAddProposal{}, fmt.Errorf("list repository resources: %w", listErr)
		}

		name := filepath.Base(resolved.Repository)
		name = strings.TrimSuffix(name, ".git")
		if resolved.Path != "" {
			name = filepath.Base(resolved.Path)
		}

		captured = captureResult{
			origin: SkillOrigin{
				Kind:         "github",
				Repository:   resolved.Repository,
				Ref:          resolved.Ref,
				Commit:       resolved.Commit,
				Path:         resolved.Path,
				Name:         sanitizeSkillID(name),
				FolderDigest: rev.ContentDigest,
			},
			reader: AdapterResourceReader{
				Adapter:  gitAdapter,
				Source:   src,
				Revision: rev,
			},
			resources:   resources,
			scopePrefix: "",
		}
	}

	// 2. Discover skills and inventory companion files
	discovered, discErr := DiscoverSkillsFromResources(ctx, captured.reader, captured.resources, captured.scopePrefix)
	if discErr != nil {
		return SkillAddProposal{}, discErr
	}

	// Zero skills found: return no_skills
	if len(discovered) == 0 {
		return SkillAddProposal{
			Result: ErrorResult(NewInvalidRequestError(
				"no skills found at locator",
				"Ensure the locator points to a skill directory containing SKILL.md.",
			)),
		}, nil
	}

	// Multiple skills found: selection required unless explicit
	if len(discovered) > 1 && !input.All && strings.TrimSpace(input.Selection) == "" {
		names := make([]string, 0, len(discovered))
		for _, d := range discovered {
			names = append(names, d.TargetID)
		}
		return SkillAddProposal{
			Result: ErrorResult(NewSkillSelectionRequiredError(
				fmt.Sprintf("found %d skills at locator: %s; explicit selection is required", len(discovered), strings.Join(names, ", ")),
				"Specify --skill <name> to select one, or --all to import all discovered skills.",
			)),
		}, nil
	}

	// Filter / select skills
	var selected []DiscoveredSkillItem
	if input.All {
		selected = discovered
	} else if strings.TrimSpace(input.Selection) != "" {
		sel := strings.ToLower(strings.TrimSpace(input.Selection))
		for _, d := range discovered {
			if strings.ToLower(d.Name) == sel || strings.ToLower(d.TargetID) == sel || strings.ToLower(d.SkillDir) == sel {
				selected = append(selected, d)
				break
			}
		}
		if len(selected) == 0 {
			return SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError(
					fmt.Sprintf("skill %q not found among discovered skills", input.Selection),
					"Run without --skill to inspect available skills.",
				)),
			}, nil
		}
	} else {
		// Exactly 1 skill discovered
		selected = []DiscoveredSkillItem{discovered[0]}
	}

	// Check explicit TargetID
	if len(selected) > 1 && strings.TrimSpace(input.TargetID) != "" {
		return SkillAddProposal{
			Result: ErrorResult(NewInvalidRequestError(
				"cannot specify --id when adding multiple skills",
				"Use --skill to add one skill with a custom ID, or omit --id when using --all.",
			)),
		}, nil
	}

	if len(selected) == 1 && strings.TrimSpace(input.TargetID) != "" {
		customID := sanitizeSkillID(input.TargetID)
		if customID == "" || !skillTargetIDPattern.MatchString(customID) {
			return SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError(
					fmt.Sprintf("invalid target ID %q", input.TargetID),
					"Skill ID must contain only lowercase alphanumeric characters and hyphens.",
				)),
			}, nil
		}
		selected[0].TargetID = customID
	}

	// 3. Prepare planned changes and compute deterministic digests
	now := service.Clock.Now().UTC()
	nowISO := now.Format(time.RFC3339Nano)

	var changes []mutation.Change
	diffAdded := []string{}
	var allTransforms []string
	var skillIDs []string
	var primaryItem DiscoveredSkillItem
	if len(selected) > 0 {
		primaryItem = selected[0]
	}

	for _, item := range selected {
		targetID := item.TargetID
		skillIDs = append(skillIDs, targetID)

		if item.Error != "" {
			return SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError(
					item.Error,
					"Fix the resource error in the source before adding.",
				)),
			}, nil
		}

		// 3a. Normalized SKILL.md
		normMD, transforms, normErr := ensureImportedSkillFrontmatter(item.SkillMDBytes, targetID, item.Description)
		if normErr != nil {
			return SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError(normErr.Error(), "Ensure SKILL.md contains valid frontmatter.")),
			}, nil
		}
		allTransforms = append(allTransforms, transforms...)
		skillMDTarget := fmt.Sprintf("skills/%s/%s/SKILL.md", collection, targetID)
		changes = append(changes, mutation.Change{Path: skillMDTarget, Contents: normMD})
		diffAdded = append(diffAdded, skillMDTarget)

		// 3b. skill.meta.yaml with privacy-safe origin
		contentDigest := "sha256:" + hex.EncodeToString(func() []byte { s := sha256.Sum256(normMD); return s[:] }())

		skillOrigin := captured.origin
		skillOrigin.ContentDigest = contentDigest
		if skillOrigin.Name == "" {
			skillOrigin.Name = targetID
		}
		if item.SkillDir != "" && isSafeRelativeSkillPath(item.SkillDir) {
			skillOrigin.Path = item.SkillDir
		}
		skillOrigin.Transformations = transforms
		skillOrigin.AddedAt = nowISO

		metaDoc := map[string]any{
			"schema_version": 1,
			"id":             targetID,
			"name":           item.Name,
			"status":         "draft",
			"description":    item.Description,
			"collection":     collection,
			"routing": map[string]any{
				"triggers":  []string{},
				"not_for":   []string{},
				"min_scope": "",
			},
			"quality": map[string]any{
				"reviewed": false,
			},
			"provenance": map[string]any{
				"created_by": "skill_add",
				"origin":     skillOriginToMap(skillOrigin),
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

		metaBytes, yErr := yaml.Marshal(metaDoc)
		if yErr != nil {
			return SkillAddProposal{}, yErr
		}
		metaTarget := fmt.Sprintf("skills/%s/%s/skill.meta.yaml", collection, targetID)
		changes = append(changes, mutation.Change{Path: metaTarget, Contents: metaBytes})
		diffAdded = append(diffAdded, metaTarget)

		// 3c. Companion files (BUG-04: byte-for-byte, preserving empty and binary files)
		for _, comp := range item.Companions {
			compData := item.CompanionBytes[comp.Path]
			compTarget := fmt.Sprintf("skills/%s/%s/%s", collection, targetID, comp.Path)
			changes = append(changes, mutation.Change{Path: compTarget, Contents: compData})
			diffAdded = append(diffAdded, compTarget)
		}
	}

	// 4. Derive deterministic idempotency key and request digest
	primaryTargetID := selected[0].TargetID
	primaryContentDigest := "sha256:" + hex.EncodeToString(func() []byte { s := sha256.Sum256(selected[0].SkillMDBytes); return s[:] }())
	requestDigest := skillAddRequestDigest(captured.origin.Kind, captured.origin.FolderDigest, primaryContentDigest, primaryTargetID, input.Selection, collection, allTransforms)

	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("skill_add:%s:%s", primaryTargetID, strings.TrimPrefix(requestDigest, "sha256:")[:32])
	}

	// 5. Replay lookup precedes target ID conflict detection
	priorReceipt, found, lookupErr := mutation.LookupOperation(root, mutation.WriteSet{
		Command:        "skill_add",
		IdempotencyKey: idempotencyKey,
		RequestDigest:  requestDigest,
	})
	if lookupErr != nil {
		if errors.Is(lookupErr, mutation.ErrIdempotencyConflict) || errors.Is(lookupErr, mutation.ErrConflict) {
			return SkillAddProposal{
				Result: ErrorResult(NewSkillConflictError(
					"idempotency key reused with a different payload",
					"Retry with the identical add request or specify a new --idempotency-key.",
				)),
			}, nil
		}
		return SkillAddProposal{}, lookupErr
	}

	if found {
		// Replayed proposal: return prior receipt without checking ID conflicts
		assessment, _ := catalog.AssessSkillState(ctx, root, primaryTargetID)
		proposal := SkillAddProposal{
			Result:         NewResult(StatusOK, fmt.Sprintf("Skill %q was already added (replayed).", primaryTargetID)),
			SkillID:        primaryTargetID,
			SkillIDs:       skillIDs,
			Collection:     collection,
			Name:           primaryItem.Name,
			Description:    primaryItem.Description,
			Origin:         captured.origin,
			License:        primaryItem.License,
			Resources:      primaryItem.Companions,
			TotalBytes:     primaryItem.TotalBytes,
			Diff:           skill.DiffSummary{},
			Assessment:     assessment,
			Replayed:       true,
			alreadyApplied: &priorReceipt,
			expiresAt:      now.Add(24 * time.Hour),
		}
		return proposal, nil
	}

	// 6. Non-replay: Target ID conflict detection
	existingSkills, err := listWorkspaceSkillIDs(root)
	if err != nil {
		return SkillAddProposal{}, err
	}
	for _, item := range selected {
		if existingSkills[item.TargetID] {
			return SkillAddProposal{
				Result: ErrorResult(NewSkillConflictError(
					fmt.Sprintf("skill %q already exists in workspace", item.TargetID),
					fmt.Sprintf("Specify --id <new-id> to rename the skill, or edit the existing skill with `skillhub skill edit %s`.", item.TargetID),
				)),
			}, nil
		}
	}

	// 7. Plan mutation
	sort.Strings(diffAdded)
	writeSet := mutation.WriteSet{
		Command:        "skill_add",
		IdempotencyKey: idempotencyKey,
		RequestDigest:  requestDigest,
		Changes:        changes,
	}

	planned, err := mutation.PlanMutation(root, writeSet)
	if err != nil {
		return SkillAddProposal{}, err
	}

	// 8. Store in Phase 3 kind-tagged proposal store
	expiresAt := now.Add(24 * time.Hour)
	diffSummary := skill.DiffSummary{Added: diffAdded}
	fullDiffText := ""
	if input.FullDiff {
		var diffBuf bytes.Buffer
		for _, c := range changes {
			fmt.Fprintf(&diffBuf, "--- /dev/null\n+++ %s\n@@ -0,0 +1 @@\n+[added %d bytes]\n", c.Path, len(c.Contents))
		}
		fullDiffText = diffBuf.String()
	}

	if err := storeSkillAddProposal(root, storedProposalArtifact{
		Version:      1,
		Kind:         skill.ProposalKindAdd,
		CreatedAt:    now,
		ExpiresAt:    expiresAt,
		ID:           planned.ID,
		Digest:       planned.Digest,
		BaseSnapshot: planned.BaseCatalogSnapshot,
		SkillID:      primaryTargetID,
		Command:      "skill_add",
		Summary:      diffSummary,
		WriteSet:     planned.WriteSet,
	}); err != nil {
		return SkillAddProposal{}, err
	}

	pins := ConfirmationPins{
		ProposalID:     planned.ID,
		ProposalDigest: planned.Digest,
		BaseVersion:    planned.BaseCatalogSnapshot,
	}

	assessment, _ := catalog.AssessSkillState(ctx, root, primaryTargetID)

	proposal := SkillAddProposal{
		Result:          NewResult(StatusActionRequired, fmt.Sprintf("Draft skill %q ready to add. Next: confirm with `skillhub skill confirm %s --yes`.", primaryTargetID, planned.ID)),
		SkillID:         primaryTargetID,
		SkillIDs:        skillIDs,
		Collection:      collection,
		Name:            primaryItem.Name,
		Description:     primaryItem.Description,
		Origin:          captured.origin,
		License:         primaryItem.License,
		Resources:       primaryItem.Companions,
		TotalBytes:      primaryItem.TotalBytes,
		Transformations: allTransforms,
		Diff:            diffSummary,
		FullDiff:        fullDiffText,
		Confirmation: ConfirmationPolicy{
			PolicyRevision:     "policy_v1",
			ActionClass:        "semantic",
			ApplicationCommand: "skill_add",
			Confirmation: ConfirmationRequirement{
				Required: true,
				Mode:     "preview-and-approval",
				Pins:     pins,
			},
		},
		Assessment: assessment,
		planned:    planned,
		expiresAt:  expiresAt,
	}

	// Add license warning if any (BUG-16)
	if primaryItem.License.Warning != "" {
		proposal.Warnings = append(proposal.Warnings, Warning{
			Code:    "license_warning",
			Summary: primaryItem.License.Warning,
		})
	}

	return proposal, nil
}

// ConfirmSkillAdd applies an approved skill add proposal.
func (service SkillAddService) ConfirmSkillAdd(ctx context.Context, path string, preview SkillAddProposal, pins ConfirmationPins) (SkillAddResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SkillAddResult{}, err
	}
	service = service.defaults(root)
	now := service.Clock.Now().UTC()

	// Check expiration
	if !preview.expiresAt.IsZero() && !now.Before(preview.expiresAt) {
		return SkillAddResult{
			Result: ErrorResult(NewStaleProposalError("proposal expired; nothing was applied", "Regenerate the add proposal.")),
		}, nil
	}

	// Check confirmation pins
	expectedPins := preview.Confirmation.Confirmation.Pins
	if pins != expectedPins {
		if pins.ProposalDigest != expectedPins.ProposalDigest {
			return SkillAddResult{
				Result: ErrorResult(NewStaleProposalError("proposal digest does not match preview; nothing was applied", "Pass the exact proposal digest printed by the preview, or re-run with --yes.")),
			}, nil
		}
		return SkillAddResult{
			Result: ErrorResult(NewStaleProposalError("confirmation pins do not match; nothing was applied", "Load or regenerate the add proposal.")),
		}, nil
	}

	// Replayed proposal handling
	if preview.alreadyApplied != nil {
		assessment, _ := catalog.AssessSkillState(ctx, root, preview.SkillID)
		receipt := *preview.alreadyApplied
		return SkillAddResult{
			Result:          NewResult(StatusOK, fmt.Sprintf("Skill %q was already added (replayed).", preview.SkillID)),
			SkillID:         preview.SkillID,
			SkillIDs:        preview.SkillIDs,
			Collection:      preview.Collection,
			OperationID:     receipt.OperationID,
			Receipt:         receipt,
			Replayed:        true,
			ChangedPaths:    receipt.ChangedPaths,
			CatalogSnapshot: receipt.CatalogSnapshot,
			Generation:      receipt.Generation,
			GitDirty:        receipt.GitDirty,
			Assessment:      assessment,
			Origin:          preview.Origin,
			License:         preview.License,
			Resources:       preview.Resources,
		}, nil
	}

	// Apply mutation and publish
	receipt, err := confirmAndPublish(ctx, root, preview.planned)
	if errors.Is(err, mutation.ErrConflict) {
		return SkillAddResult{
			Result: ErrorResult(NewStaleProposalError("workspace state changed after preview; nothing was applied", "Regenerate the add proposal.")),
		}, nil
	}
	if err != nil {
		return SkillAddResult{}, err
	}

	assessment, _ := catalog.AssessSkillState(ctx, root, preview.SkillID)

	return SkillAddResult{
		Result:          NewResult(StatusOK, fmt.Sprintf("Added draft skill %q. Next: review with `skillhub skill show %s`, then activate with `skillhub skill activate %s --yes`.", preview.SkillID, preview.SkillID, preview.SkillID)),
		SkillID:         preview.SkillID,
		SkillIDs:        preview.SkillIDs,
		Collection:      preview.Collection,
		OperationID:     receipt.OperationID,
		Receipt:         receipt,
		Replayed:        false,
		ChangedPaths:    receipt.ChangedPaths,
		CatalogSnapshot: receipt.CatalogSnapshot,
		Generation:      receipt.Generation,
		GitDirty:        receipt.GitDirty,
		Assessment:      assessment,
		Origin:          preview.Origin,
		License:         preview.License,
		Resources:       preview.Resources,
	}, nil
}

// ConfirmSkillAddProposal adapts a loaded Phase 3 domain proposal for confirmation.
func (service SkillAddService) ConfirmSkillAddProposal(ctx context.Context, path string, p skill.Proposal, pins ConfirmationPins) (SkillAddResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SkillAddResult{}, err
	}
	service = service.defaults(root)

	preview := SkillAddProposal{
		SkillID:    p.SkillID,
		Collection: "default",
		planned:    p.Planned(),
		expiresAt:  p.ExpiresAt,
		Confirmation: ConfirmationPolicy{
			PolicyRevision:     "policy_v1",
			ActionClass:        "semantic",
			ApplicationCommand: "skill_add",
			Confirmation: ConfirmationRequirement{
				Required: true,
				Mode:     "preview-and-approval",
				Pins: ConfirmationPins{
					ProposalID:     p.ID,
					ProposalDigest: p.Digest,
					BaseVersion:    p.BaseSnapshot,
				},
			},
		},
		alreadyApplied: p.AlreadyApplied(),
	}

	return service.ConfirmSkillAdd(ctx, path, preview, pins)
}

// LoadSkillAddProposal loads a previously persisted skill_add proposal from the store.
func (service SkillAddService) LoadSkillAddProposal(ctx context.Context, path, proposalID string) (SkillAddProposal, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SkillAddProposal{}, err
	}
	service = service.defaults(root)

	stored, err := skill.LoadProposal(root, proposalID, service.Clock.Now())
	if err != nil {
		return SkillAddProposal{}, err
	}
	if stored.Kind != skill.ProposalKindAdd {
		return SkillAddProposal{}, fmt.Errorf("proposal %s has kind %q, expected %q", proposalID, stored.Kind, skill.ProposalKindAdd)
	}

	assessment, _ := catalog.AssessSkillState(ctx, root, stored.SkillID)

	return SkillAddProposal{
		Result:         NewResult(StatusActionRequired, "Stored add proposal ready for confirmation."),
		SkillID:        stored.SkillID,
		Collection:     "default",
		Diff:           stored.Summary,
		FullDiff:       stored.FullDiff,
		Assessment:     assessment,
		planned:        stored.Planned(),
		expiresAt:      stored.ExpiresAt,
		Replayed:       stored.AlreadyApplied() != nil,
		alreadyApplied: stored.AlreadyApplied(),
		Confirmation: ConfirmationPolicy{
			PolicyRevision:     "policy_v1",
			ActionClass:        "semantic",
			ApplicationCommand: "skill_add",
			Confirmation: ConfirmationRequirement{
				Required: true,
				Mode:     "preview-and-approval",
				Pins: ConfirmationPins{
					ProposalID:     stored.ID,
					ProposalDigest: stored.Digest,
					BaseVersion:    stored.BaseSnapshot,
				},
			},
		},
	}, nil
}

// Helper to convert SkillOrigin to schema-compliant map for yaml.Marshal.
func skillOriginToMap(origin SkillOrigin) map[string]any {
	m := map[string]any{
		"kind": origin.Kind,
	}
	if origin.Repository != "" {
		m["repository"] = origin.Repository
	}
	if origin.Ref != "" {
		m["ref"] = origin.Ref
	}
	if origin.Commit != "" {
		m["commit"] = origin.Commit
	}
	if origin.Path != "" {
		m["path"] = origin.Path
	}
	if origin.Name != "" {
		m["name"] = origin.Name
	}
	if origin.FolderDigest != "" {
		m["folder_digest"] = origin.FolderDigest
	}
	if origin.ContentDigest != "" {
		m["content_digest"] = origin.ContentDigest
	}
	if len(origin.Transformations) > 0 {
		m["transformations"] = origin.Transformations
	}
	if origin.AddedAt != "" {
		m["added_at"] = origin.AddedAt
	}
	return m
}

func isSafeRelativeSkillPath(p string) bool {
	p = filepath.ToSlash(p)
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || strings.Contains(p, "..") {
		return false
	}
	matched, _ := regexp.MatchString(`^([a-zA-Z0-9_.-]*[a-zA-Z0-9_-][a-zA-Z0-9_.-]*)(/[a-zA-Z0-9_.-]*[a-zA-Z0-9_-][a-zA-Z0-9_.-]*)*$`, p)
	return matched
}

type storedProposalArtifact struct {
	Version        int                  `json:"version"`
	Kind           skill.ProposalKind   `json:"kind,omitempty"`
	CreatedAt      time.Time            `json:"created_at"`
	ExpiresAt      time.Time            `json:"expires_at"`
	ID             string               `json:"id"`
	Digest         string               `json:"digest"`
	BaseSnapshot   string               `json:"base_snapshot"`
	SkillID        string               `json:"skill_id"`
	Command        string               `json:"command"`
	Summary        skill.DiffSummary    `json:"summary"`
	RoutingImpact  *skill.RoutingImpact `json:"routing_impact,omitempty"`
	WriteSet       mutation.WriteSet    `json:"write_set"`
	AlreadyApplied *mutation.Receipt    `json:"already_applied,omitempty"`
	RecoveryID     string               `json:"recovery_id,omitempty"`
}

func storeSkillAddProposal(root string, artifact storedProposalArtifact) error {
	data, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rootHandle.Close()
	if err := rootHandle.MkdirAll("runtime/proposals", 0o700); err != nil {
		return fmt.Errorf("create proposal runtime directory: %w", err)
	}
	path := filepath.ToSlash(filepath.Join("runtime/proposals", artifact.ID+".json"))
	temporary := path + ".tmp"
	_ = rootHandle.Remove(temporary)
	file, err := rootHandle.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
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
		_ = rootHandle.Remove(temporary)
		return err
	}
	return rootHandle.Rename(temporary, path)
}

// PreviewSkillAdd exposes skill adding on the SkillService facade.
func (service SkillService) PreviewSkillAdd(ctx context.Context, path string, input SkillAddInput) (SkillAddProposal, error) {
	return (SkillAddService{}).PreviewSkillAdd(ctx, path, input)
}

// ConfirmSkillAdd exposes skill add confirmation on the SkillService facade.
func (service SkillService) ConfirmSkillAdd(ctx context.Context, path string, preview SkillAddProposal, pins ConfirmationPins) (SkillAddResult, error) {
	return (SkillAddService{}).ConfirmSkillAdd(ctx, path, preview, pins)
}
