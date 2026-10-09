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
	CandidateID    string `json:"-"`                         // Optional candidate ID when invoked from triage import
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
	FilesDigest     string   `json:"files_digest,omitempty" yaml:"files_digest,omitempty"`
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
	UpstreamSource  *UpstreamSourceRef           `json:"upstream_source,omitempty"`
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
	UpstreamSource  *UpstreamSourceRef           `json:"upstream_source,omitempty"`
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

	collection, rawLocator, errProp := validateSkillAddInput(input)
	if errProp != nil {
		return *errProp, nil
	}

	captured, capProp, err := resolveSkillAddSource(ctx, service, root, rawLocator)
	if err != nil {
		return SkillAddProposal{}, err
	}
	if capProp != nil {
		return *capProp, nil
	}

	discovered, discErr := DiscoverSkillsFromResources(ctx, captured.reader, captured.resources, captured.scopePrefix)
	if discErr != nil {
		return SkillAddProposal{}, discErr
	}

	selected, selProp := selectSkillAddCandidates(input, discovered)
	if selProp != nil {
		return *selProp, nil
	}
	var upstreamRef *UpstreamSourceRef
	if captured.origin.Kind != "local" {
		ref := captured.origin.Ref
		if ref == "" {
			ref = captured.origin.Commit
		}
		commit := captured.origin.Commit
		gitAdapter := service.Adapters["git"]
		sourceRec, sourceChange, sourceCreated, srcErr := ensureUpstreamSource(ctx, root, gitAdapter, captured.origin.Repository, ref, commit)
		if srcErr != nil {
			return SkillAddProposal{}, srcErr
		}
		if sourceRec != nil {
			upstreamRef = &UpstreamSourceRef{
				SourceID: sourceRec.ID,
				Created:  sourceCreated,
			}
			captured.sourceID = sourceRec.ID
			captured.sourceChange = sourceChange
		}
	}

	now := service.Clock.Now().UTC()
	changes, diffAdded, allTransforms, changeProp, err := buildSkillAddChanges(selected, collection, captured, now.Format(time.RFC3339Nano))
	if err != nil {
		return SkillAddProposal{}, err
	}
	if changeProp != nil {
		return *changeProp, nil
	}

	primaryTargetID := selected[0].TargetID
	primaryContentDigest := "sha256:" + hex.EncodeToString(func() []byte { s := sha256.Sum256(selected[0].SkillMDBytes); return s[:] }())
	requestDigest := skillAddRequestDigest(captured.origin.Kind, captured.origin.FolderDigest, primaryContentDigest, primaryTargetID, input.Selection, collection, allTransforms)

	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("skill_add:%s:%s", primaryTargetID, strings.TrimPrefix(requestDigest, "sha256:")[:32])
	}

	if input.CandidateID != "" {
		cand, candBytes, cErr := findCandidate(root, input.CandidateID)
		if cErr == nil {
			cand.Status = "accepted"
			candAfter, _ := sourcepkg.MarshalCanonical(cand)
			candPath := "sources/intake/" + cand.ID + ".yaml"
			changes = append(changes, mutation.Change{
				Path:         candPath,
				BeforeDigest: sourcepkg.Digest(candBytes),
				Contents:     candAfter,
			})
			diffAdded = append(diffAdded, candPath)
		}
	}

	planCtx := skillAddPlanContext{
		root:           root,
		collection:     collection,
		selected:       selected,
		origin:         captured.origin,
		upstreamSource: upstreamRef,
		changes:        changes,
		diffAdded:      diffAdded,
		transforms:     allTransforms,
		idempotencyKey: idempotencyKey,
		requestDigest:  requestDigest,
		now:            now,
		fullDiff:       input.FullDiff,
	}

	replayProp, found, err := lookupReplayedSkillAdd(ctx, planCtx)
	if err != nil {
		return SkillAddProposal{}, err
	}
	if found {
		return *replayProp, nil
	}

	if conflictProp, err := checkSkillAddConflicts(root, selected); err != nil {
		return SkillAddProposal{}, err
	} else if conflictProp != nil {
		return *conflictProp, nil
	}

	return planSkillAddProposal(ctx, planCtx)
}

func validateSkillAddInput(input SkillAddInput) (string, string, *SkillAddProposal) {
	collection := strings.TrimSpace(input.Collection)
	if collection == "" {
		collection = "default"
	}
	if !skillTargetIDPattern.MatchString(collection) {
		prop := SkillAddProposal{
			Result: ErrorResult(NewInvalidRequestError("invalid collection name", "Specify a valid lowercase collection name.")),
		}
		return "", "", &prop
	}
	rawLocator := strings.TrimSpace(input.Locator)
	if rawLocator == "" {
		prop := SkillAddProposal{
			Result: ErrorResult(NewInvalidRequestError("locator is required", "Provide a local directory path or repository URL pointing to a skill.")),
		}
		return "", "", &prop
	}
	return collection, rawLocator, nil
}

type skillAddCapture struct {
	origin       SkillOrigin
	reader       ResourceReader
	resources    []sourcepkg.Resource
	scopePrefix  string
	scopePath    string
	revisionAt   func(path string) (sourcepkg.Revision, error)
	sourceID     string
	sourceChange *mutation.Change
}

func resolveSkillAddSource(ctx context.Context, service SkillAddService, root, rawLocator string) (skillAddCapture, *SkillAddProposal, error) {
	isRemote := strings.Contains(rawLocator, "://") || strings.HasPrefix(rawLocator, "git@") || strings.HasSuffix(rawLocator, ".git")
	if !isRemote {
		return captureLocalSkillAddSource(ctx, root, rawLocator)
	}
	return captureRemoteSkillAddSource(ctx, service, rawLocator)
}

func captureLocalSkillAddSource(ctx context.Context, root, rawLocator string) (skillAddCapture, *SkillAddProposal, error) {
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
		prop := SkillAddProposal{
			Result: ErrorResult(NewInvalidRequestError(
				fmt.Sprintf("cannot access local directory %q: %v", rawLocator, authErr),
				"Verify the directory exists and has readable permissions.",
			)),
		}
		return skillAddCapture{}, &prop, nil
	}

	cacheRoot := filepath.Join(root, "runtime", "cache", "snapshots")
	fsAdapter := sourcepkg.FilesystemAdapter{CacheRoot: cacheRoot}
	manifest, capErr := fsAdapter.CaptureAuthorizedRoot(ctx, authRoot, "")
	if capErr != nil {
		if errors.Is(capErr, sourcepkg.ErrUnsafeFile) {
			prop := SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError(capErr.Error(), "Ensure source folder does not contain unsafe symlinks or escape paths.")),
			}
			return skillAddCapture{}, &prop, nil
		}
		return skillAddCapture{}, nil, fmt.Errorf("capture local snapshot: %w", capErr)
	}

	baseName := filepath.Base(authRoot.Path())
	return skillAddCapture{
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
	}, nil, nil
}

func captureRemoteSkillAddSource(ctx context.Context, service SkillAddService, rawLocator string) (skillAddCapture, *SkillAddProposal, error) {
	gitAdapter, ok := service.Adapters["git"]
	if !ok {
		return skillAddCapture{}, nil, errors.New("git source adapter is not configured")
	}

	allowFile := false
	if ga, isGit := gitAdapter.(sourcepkg.GitRepositoryAdapter); isGit {
		allowFile = ga.AllowFileProtocol
	}

	route, parseErr := sourcepkg.ParseGitHubLocatorWithOptions(rawLocator, "", "", sourcepkg.GitHubLocatorOptions{AllowFile: allowFile})
	if parseErr != nil {
		prop := SkillAddProposal{
			Result: ErrorResult(NewInvalidRequestError(parseErr.Error(), "Provide a valid GitHub repository URL.")),
		}
		return skillAddCapture{}, &prop, nil
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
				prop := SkillAddProposal{
					Result: ErrorResult(NewAmbiguousRefError(ambErr.Error(), "Specify an unambiguous reference.")),
				}
				return skillAddCapture{}, &prop, nil
			}
			prop := SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError(resErr.Error(), "Verify repository exists and is accessible.")),
			}
			return skillAddCapture{}, &prop, nil
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
			prop := SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError("failed to determine repository revision: "+revErr.Error(), "Verify repository and ref exist.")),
			}
			return skillAddCapture{}, &prop, nil
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
		return skillAddCapture{}, nil, fmt.Errorf("list repository resources: %w", listErr)
	}

	name := filepath.Base(resolved.Repository)
	name = strings.TrimSuffix(name, ".git")
	if resolved.Path != "" {
		name = filepath.Base(resolved.Path)
	}

	return skillAddCapture{
		origin: SkillOrigin{
			Kind:         "github",
			Repository:   resolved.Repository,
			Ref:          resolved.Ref,
			Commit:       rev.Value,
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
		scopePath:   resolved.Path,
		revisionAt:  makeRevisionAtCallback(ctx, gitAdapter, src, rev.Value),
	}, nil, nil
}

func selectSkillAddCandidates(input SkillAddInput, discovered []DiscoveredSkillItem) ([]DiscoveredSkillItem, *SkillAddProposal) {
	if len(discovered) == 0 {
		prop := SkillAddProposal{
			Result: ErrorResult(NewInvalidRequestError(
				"no skills found at locator",
				"Ensure the locator points to a skill directory containing SKILL.md.",
			)),
		}
		return nil, &prop
	}

	if len(discovered) > 1 && !input.All && strings.TrimSpace(input.Selection) == "" {
		names := make([]string, 0, len(discovered))
		for _, d := range discovered {
			names = append(names, d.TargetID)
		}
		prop := SkillAddProposal{
			Result: ErrorResult(NewSkillSelectionRequiredError(
				fmt.Sprintf("found %d skills at locator: %s; explicit selection is required", len(discovered), strings.Join(names, ", ")),
				"Specify --skill <name> to select one, or --all to import all discovered skills.",
			)),
		}
		return nil, &prop
	}

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
			prop := SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError(
					fmt.Sprintf("skill %q not found among discovered skills", input.Selection),
					"Run without --skill to inspect available skills.",
				)),
			}
			return nil, &prop
		}
	} else {
		selected = []DiscoveredSkillItem{discovered[0]}
	}

	if len(selected) > 1 && strings.TrimSpace(input.TargetID) != "" {
		prop := SkillAddProposal{
			Result: ErrorResult(NewInvalidRequestError(
				"cannot specify --id when adding multiple skills",
				"Use --skill to add one skill with a custom ID, or omit --id when using --all.",
			)),
		}
		return nil, &prop
	}

	if len(selected) == 1 && strings.TrimSpace(input.TargetID) != "" {
		customID := sanitizeSkillID(input.TargetID)
		if customID == "" || !skillTargetIDPattern.MatchString(customID) {
			prop := SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError(
					fmt.Sprintf("invalid target ID %q", input.TargetID),
					"Skill ID must contain only lowercase alphanumeric characters and hyphens.",
				)),
			}
			return nil, &prop
		}
		selected[0].TargetID = customID
	}

	return selected, nil
}

func buildSkillAddChanges(selected []DiscoveredSkillItem, collection string, captured skillAddCapture, nowISO string) ([]mutation.Change, []string, []string, *SkillAddProposal, error) {
	var changes []mutation.Change
	diffAdded := []string{}
	var allTransforms []string

	if captured.sourceChange != nil {
		changes = append(changes, *captured.sourceChange)
		diffAdded = append(diffAdded, captured.sourceChange.Path)
	}

	for _, item := range selected {
		targetID := item.TargetID

		if item.Error != "" {
			prop := SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError(
					item.Error,
					"Fix the resource error in the source before adding.",
				)),
			}
			return nil, nil, nil, &prop, nil
		}

		files, transforms, normErr := importedSkillFiles(item, targetID)
		if normErr != nil {
			prop := SkillAddProposal{
				Result: ErrorResult(NewInvalidRequestError(normErr.Error(), "Ensure SKILL.md contains valid frontmatter.")),
			}
			return nil, nil, nil, &prop, nil
		}
		allTransforms = append(allTransforms, transforms...)
		normMD := files["SKILL.md"]
		skillMDTarget := fmt.Sprintf("skills/%s/%s/SKILL.md", collection, targetID)
		changes = append(changes, mutation.Change{Path: skillMDTarget, Contents: normMD})
		diffAdded = append(diffAdded, skillMDTarget)

		var skillOrigin SkillOrigin
		if captured.origin.Kind == "local" {
			contentDigest := "sha256:" + hex.EncodeToString(func() []byte { s := sha256.Sum256(normMD); return s[:] }())
			skillOrigin = captured.origin
			skillOrigin.ContentDigest = contentDigest
			if skillOrigin.Name == "" {
				skillOrigin.Name = targetID
			}
			if item.SkillDir != "" && isSafeRelativeSkillPath(item.SkillDir) {
				skillOrigin.Path = item.SkillDir
			}
			skillOrigin.Transformations = transforms
			skillOrigin.AddedAt = nowISO
		} else {
			capturedOrigin := captured.origin
			capturedOrigin.AddedAt = nowISO
			skillOrigin = buildGitOrigin(capturedOrigin, captured.scopePath, item.SkillDir, files, transforms, captured.revisionAt)
			if skillOrigin.Name == "" {
				skillOrigin.Name = targetID
			}
		}

		srcID := captured.sourceID
		if srcID == "" {
			srcID = targetID
		}
		src := map[string]any{
			"id":    srcID,
			"roles": []string{"upstream"},
		}
		if skillOrigin.Kind != "" {
			src["kind"] = skillOrigin.Kind
		}
		if skillOrigin.Repository != "" {
			src["repo"] = skillOrigin.Repository
		}
		if skillOrigin.Ref != "" {
			src["ref"] = skillOrigin.Ref
		}
		if skillOrigin.Commit != "" {
			src["commit"] = skillOrigin.Commit
			src["synced"] = skillOrigin.Commit
		}
		if skillOrigin.Path != "" {
			src["path"] = skillOrigin.Path
		}
		if skillOrigin.FilesDigest != "" {
			src["files_digest"] = skillOrigin.FilesDigest
		}
		if skillOrigin.FolderDigest != "" {
			src["folder_digest"] = skillOrigin.FolderDigest
		}
		if len(skillOrigin.Transformations) > 0 {
			src["transformations"] = skillOrigin.Transformations
		}
		if skillOrigin.AddedAt != "" {
			src["added_at"] = skillOrigin.AddedAt
		}

		metaBytes, yErr := buildInitialSkillMetaYAML(targetID, item.Name, item.Description, normMD, src)
		if yErr != nil {
			return nil, nil, nil, nil, yErr
		}
		metaTarget := fmt.Sprintf("skills/%s/%s/.meta/skill.yaml", collection, targetID)
		changes = append(changes, mutation.Change{Path: metaTarget, Contents: metaBytes})
		diffAdded = append(diffAdded, metaTarget)

		for _, comp := range item.Companions {
			compData := item.CompanionBytes[comp.Path]
			compTarget := fmt.Sprintf("skills/%s/%s/%s", collection, targetID, comp.Path)
			changes = append(changes, mutation.Change{Path: compTarget, Contents: compData})
			diffAdded = append(diffAdded, compTarget)
		}
	}

	return changes, diffAdded, allTransforms, nil, nil
}

type skillAddPlanContext struct {
	root           string
	collection     string
	selected       []DiscoveredSkillItem
	origin         SkillOrigin
	upstreamSource *UpstreamSourceRef
	changes        []mutation.Change
	diffAdded      []string
	transforms     []string
	idempotencyKey string
	requestDigest  string
	now            time.Time
	fullDiff       bool
}

func lookupReplayedSkillAdd(ctx context.Context, planCtx skillAddPlanContext) (*SkillAddProposal, bool, error) {
	priorReceipt, found, lookupErr := mutation.LookupOperation(planCtx.root, mutation.WriteSet{
		Command:        "skill_add",
		IdempotencyKey: planCtx.idempotencyKey,
		RequestDigest:  planCtx.requestDigest,
	})
	if lookupErr != nil {
		if errors.Is(lookupErr, mutation.ErrIdempotencyConflict) || errors.Is(lookupErr, mutation.ErrConflict) {
			prop := SkillAddProposal{
				Result: ErrorResult(NewSkillConflictError(
					"idempotency key reused with a different payload",
					"Retry with the identical add request or specify a new --idempotency-key.",
				)),
			}
			return &prop, true, nil
		}
		return nil, false, lookupErr
	}

	if found {
		primaryItem := planCtx.selected[0]
		primaryTargetID := primaryItem.TargetID
		skillIDs := make([]string, 0, len(planCtx.selected))
		for _, it := range planCtx.selected {
			skillIDs = append(skillIDs, it.TargetID)
		}
		assessment, _ := catalog.AssessSkillState(ctx, planCtx.root, primaryTargetID)
		proposal := SkillAddProposal{
			Result:         NewResult(StatusOK, fmt.Sprintf("Skill %q was already added (replayed).", primaryTargetID)),
			SkillID:        primaryTargetID,
			SkillIDs:       skillIDs,
			Collection:     planCtx.collection,
			Name:           primaryItem.Name,
			Description:    primaryItem.Description,
			Origin:         planCtx.origin,
			License:        primaryItem.License,
			UpstreamSource: planCtx.upstreamSource,
			Resources:      primaryItem.Companions,
			TotalBytes:     primaryItem.TotalBytes,
			Diff:           skill.DiffSummary{},
			Assessment:     assessment,
			Replayed:       true,
			alreadyApplied: &priorReceipt,
			expiresAt:      planCtx.now.Add(24 * time.Hour),
		}
		return &proposal, true, nil
	}
	return nil, false, nil
}

func checkSkillAddConflicts(root string, selected []DiscoveredSkillItem) (*SkillAddProposal, error) {
	existingSkills, err := listWorkspaceSkillIDs(root)
	if err != nil {
		return nil, err
	}
	for _, item := range selected {
		if existingSkills[item.TargetID] {
			prop := SkillAddProposal{
				Result: ErrorResult(NewSkillConflictError(
					fmt.Sprintf("skill %q already exists in workspace", item.TargetID),
					fmt.Sprintf("Specify --id <new-id> to rename the skill, or edit the existing skill with `skillhub skill edit %s`.", item.TargetID),
				)),
			}
			return &prop, nil
		}
	}
	return nil, nil
}

func planSkillAddProposal(ctx context.Context, planCtx skillAddPlanContext) (SkillAddProposal, error) {
	sort.Strings(planCtx.diffAdded)
	writeSet := mutation.WriteSet{
		Command:        "skill_add",
		IdempotencyKey: planCtx.idempotencyKey,
		RequestDigest:  planCtx.requestDigest,
		Changes:        planCtx.changes,
	}

	planned, err := mutation.PlanMutation(planCtx.root, writeSet)
	if err != nil {
		return SkillAddProposal{}, err
	}

	expiresAt := planCtx.now.Add(24 * time.Hour)
	diffSummary := skill.DiffSummary{Added: planCtx.diffAdded}
	fullDiffText := ""
	if planCtx.fullDiff {
		var diffBuf bytes.Buffer
		for _, c := range planCtx.changes {
			fmt.Fprintf(&diffBuf, "--- /dev/null\n+++ %s\n@@ -0,0 +1 @@\n+[added %d bytes]\n", c.Path, len(c.Contents))
		}
		fullDiffText = diffBuf.String()
	}

	primaryItem := planCtx.selected[0]
	primaryTargetID := primaryItem.TargetID
	if err := storeProposalArtifact(planCtx.root, storedProposalArtifact{
		Version:      1,
		Kind:         skill.ProposalKindAdd,
		CreatedAt:    planCtx.now,
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

	assessment, _ := catalog.AssessSkillState(ctx, planCtx.root, primaryTargetID)
	skillIDs := make([]string, 0, len(planCtx.selected))
	for _, it := range planCtx.selected {
		skillIDs = append(skillIDs, it.TargetID)
	}

	proposal := SkillAddProposal{
		Result:          NewResult(StatusActionRequired, fmt.Sprintf("Draft skill %q ready to add. Next: confirm with `skillhub skill confirm %s --yes`.", primaryTargetID, planned.ID)),
		SkillID:         primaryTargetID,
		SkillIDs:        skillIDs,
		Collection:      planCtx.collection,
		Name:            primaryItem.Name,
		Description:     primaryItem.Description,
		Origin:          planCtx.origin,
		License:         primaryItem.License,
		UpstreamSource:  planCtx.upstreamSource,
		Resources:       primaryItem.Companions,
		TotalBytes:      primaryItem.TotalBytes,
		Transformations: planCtx.transforms,
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

	if primaryItem.License.Warning != "" {
		proposal.Warnings = append(proposal.Warnings, Warning{
			Code:    "license_warning",
			Summary: primaryItem.License.Warning,
		})
	}
	for _, item := range planCtx.selected {
		proposal.Warnings = append(proposal.Warnings, item.Warnings...)
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

	switch verifyProposalPins(now, preview.expiresAt, true, false, preview.Confirmation.Confirmation.Pins, pins) {
	case proposalExpired:
		return SkillAddResult{
			Result: ErrorResult(NewStaleProposalError("proposal expired; nothing was applied", "Regenerate the add proposal.")),
		}, nil
	case proposalDigestMismatch:
		return SkillAddResult{
			Result: ErrorResult(NewStaleProposalError("proposal digest does not match preview; nothing was applied", "Pass the exact proposal digest printed by the preview, or re-run with --yes.")),
		}, nil
	case proposalPinsMismatch:
		return SkillAddResult{
			Result: ErrorResult(NewStaleProposalError("confirmation pins do not match; nothing was applied", "Load or regenerate the add proposal.")),
		}, nil
	}

	upstreamSource := preview.UpstreamSource
	if upstreamSource == nil {
		upstreamSource = deriveUpstreamSourceFromChanges(preview.planned.WriteSet.Changes)
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
			UpstreamSource:  upstreamSource,
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
		UpstreamSource:  upstreamSource,
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
		SkillID:        p.SkillID,
		Collection:     "default",
		UpstreamSource: deriveUpstreamSourceFromChanges(p.WriteSet().Changes),
		planned:        p.Planned(),
		expiresAt:      p.ExpiresAt,
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
		UpstreamSource: deriveUpstreamSourceFromChanges(stored.WriteSet().Changes),
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

func deriveUpstreamSourceFromChanges(changes []mutation.Change) *UpstreamSourceRef {
	for _, c := range changes {
		if workspace.IsHubMeta(c.Path) && (strings.HasSuffix(c.Path, "skill.meta.yaml") || strings.HasSuffix(c.Path, "skill.yaml")) {
			var m struct {
				Sources []struct {
					ID    string   `yaml:"id"`
					Roles []string `yaml:"roles"`
				} `yaml:"sources"`
				Provenance struct {
					SourceID string `yaml:"source_id"`
				} `yaml:"provenance"`
			}
			if yaml.Unmarshal(c.Contents, &m) == nil {
				sourceID := m.Provenance.SourceID
				if sourceID == "" && len(m.Sources) > 0 {
					for _, s := range m.Sources {
						for _, r := range s.Roles {
							if r == "upstream" {
								sourceID = s.ID
								break
							}
						}
						if sourceID != "" {
							break
						}
					}
				}
				if sourceID != "" {
					created := false
					for _, sc := range changes {
						if sc.Path == "sources/catalog/"+sourceID+".yaml" {
							created = true
							break
						}
					}
					return &UpstreamSourceRef{SourceID: sourceID, Created: created}
				}
			}
		}
	}
	return nil
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

func storeProposalArtifact(root string, artifact storedProposalArtifact) error {
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
