package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/migration"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

type UpstreamResolution struct {
	Path    string `json:"path"`
	Action  string `json:"action"` // "upstream", "local", "merged", "manual"
	Content string `json:"content,omitempty"`
}

type UpstreamUpdateInput struct {
	SkillID        string               `json:"skill_id"`
	TargetCommit   string               `json:"target_commit,omitempty"`
	Resolutions    []UpstreamResolution `json:"resolutions,omitempty"`
	IdempotencyKey string               `json:"idempotency_key,omitempty"`
}

type UpstreamFile struct {
	Path              string `json:"path"`
	Status            string `json:"status"` // "unchanged", "upstream_only", "local_only", "added_upstream", "added_local", "removed_upstream", "removed_local", "both_changed", "blocked"
	DefaultAction     string `json:"default_action"`
	Action            string `json:"action"`
	Conflicts         int    `json:"conflicts"`
	Mergeable         bool   `json:"mergeable"`
	UpstreamDiff      string `json:"upstream_diff,omitempty"`
	LocalDiff         string `json:"local_diff,omitempty"`
	ResultDiff        string `json:"result_diff,omitempty"`
	MergedWithMarkers string `json:"merged_with_markers,omitempty"`
}

type TrustImpact struct {
	ThirdParty               bool   `json:"third_party"`
	CurrentlyApproved        bool   `json:"currently_approved"`
	ReviewRequiredAfterApply bool   `json:"review_required_after_apply"`
	ReviewCommand            string `json:"review_command"`
}

type UpstreamUpdatePreview struct {
	Result
	SkillID           string             `json:"skill_id"`
	SourceID          string             `json:"source_id"`
	BaseCommit        string             `json:"base_commit"`
	TargetCommit      string             `json:"target_commit"`
	TargetCommittedAt string             `json:"target_committed_at,omitempty"`
	BaseAvailable     bool               `json:"base_available"`
	Files             []UpstreamFile     `json:"files"`
	UnchangedCount    int                `json:"unchanged_count"`
	Unresolved        []string           `json:"unresolved"`
	Diff              skill.DiffSummary  `json:"diff"`
	TrustImpact       TrustImpact        `json:"trust_impact"`
	Confirmation      ConfirmationPolicy `json:"confirmation"`
	planned           mutation.Proposal
	expiresAt         time.Time
}

type UpstreamUpdateResult struct {
	Result
	SkillID      string           `json:"skill_id"`
	OperationID  string           `json:"operation_id"`
	Receipt      mutation.Receipt `json:"receipt"`
	ChangedPaths []string         `json:"changed_paths"`
	TrustImpact  TrustImpact      `json:"trust_impact"`
}

type UpstreamService struct {
	Clock    Clock
	Adapters map[string]sourcepkg.Adapter
}

func (service UpstreamService) defaults(root string) UpstreamService {
	if service.Clock == nil {
		service.Clock = SystemClock{}
	}
	if service.Adapters == nil {
		service.Adapters = (SourceService{}).defaults(root).Adapters
	}
	return service
}

func init() {
	RegisterProposalConfirmer(skill.ProposalKindUpstreamUpdate, func(ctx context.Context, path string, p skill.Proposal, pins ConfirmationPins) (any, error) {
		return (UpstreamService{}).ConfirmUpdateProposal(ctx, path, p, pins)
	})
}

func updateSkillMetaYAML(origBytes []byte, newCommit, folderDigest, filesDigest string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(origBytes, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("invalid skill.meta.yaml root")
	}
	rootMap := doc.Content[0]
	sourcesNode := findMappingValueNode(rootMap, "sources")
	if sourcesNode != nil && sourcesNode.Kind == yaml.SequenceNode {
		for _, item := range sourcesNode.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			rolesNode := findMappingValueNode(item, "roles")
			isUpstream := false
			if rolesNode != nil && rolesNode.Kind == yaml.SequenceNode {
				for _, r := range rolesNode.Content {
					if r.Value == "upstream" {
						isUpstream = true
						break
					}
				}
			}
			if isUpstream {
				if newCommit != "" {
					setMappingKey(item, "commit", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: newCommit})
					setMappingKey(item, "synced", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: newCommit})
				}
				if folderDigest != "" {
					setMappingKey(item, "folder_digest", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: folderDigest})
				}
				if filesDigest != "" {
					setMappingKey(item, "files_digest", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: filesDigest})
				}
				break
			}
		}
	} else {
		provNode := findMappingValueNode(rootMap, "provenance")
		if provNode == nil {
			provNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			setMappingKey(rootMap, "provenance", provNode)
		}

		originNode := findMappingValueNode(provNode, "origin")
		if originNode == nil {
			originNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			setMappingKey(provNode, "origin", originNode)
		}

		if newCommit != "" {
			setMappingKey(originNode, "commit", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: newCommit})
		}
		if folderDigest != "" {
			setMappingKey(originNode, "folder_digest", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: folderDigest})
		}
		if filesDigest != "" {
			setMappingKey(originNode, "files_digest", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: filesDigest})
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

func findMappingValueNode(mapNode *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == key {
			return mapNode.Content[i+1]
		}
	}
	return nil
}

func setMappingKey(mapNode *yaml.Node, key string, valNode *yaml.Node) {
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Value == key {
			mapNode.Content[i+1] = valNode
			return
		}
	}
	mapNode.Content = append(mapNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		valNode,
	)
}

func readLocalSkillFiles(root, skillRelDir string) (map[string][]byte, error) {
	fullDir := filepath.Join(root, filepath.FromSlash(skillRelDir))
	files := make(map[string][]byte)
	err := filepath.WalkDir(fullDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		info, err := d.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(fullDir, p)
		if err != nil {
			return err
		}
		cleanRel := filepath.ToSlash(rel)
		if workspace.IsHubMeta(cleanRel) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[cleanRel] = data
		return nil
	})
	return files, err
}

func validateAndFilterUpstreamPaths(rawFiles map[string][]byte) (map[string][]byte, []Warning, error) {
	filtered := make(map[string][]byte, len(rawFiles))
	caseFolded := make(map[string]string)
	var warnings []Warning

	for p, b := range rawFiles {
		for _, r := range p {
			if !unicode.IsPrint(r) && r != '/' {
				return nil, nil, NewInvalidRequestError(
					fmt.Sprintf("upstream path %q rejected: contains non-printable character", p),
					"Resolve or remove invalid file path upstream.",
				)
			}
		}

		if workspace.IsHubMeta(p) {
			warnings = append(warnings, Warning{
				Code:    "upstream_meta_ignored",
				Summary: fmt.Sprintf("Ignored upstream metadata file %q.", p),
			})
			continue
		}

		lower := strings.ToLower(p)
		if existing, exists := caseFolded[lower]; exists {
			return nil, nil, NewInvalidRequestError(
				fmt.Sprintf("upstream path case-folding collision between %q and %q", existing, p),
				"Resolve colliding file paths upstream.",
			)
		}
		caseFolded[lower] = p
		filtered[p] = b
	}
	return filtered, warnings, nil
}

type fileClassifyContext struct {
	Path          string
	Base          []byte
	Local         []byte
	Upstream      []byte
	BaseOK        bool
	LocalOK       bool
	UpstreamOK    bool
	BaseAvailable bool
}

func classifyUpstreamFile(ctx context.Context, root string, fc fileClassifyContext) (UpstreamFile, bool, error) {
	p := fc.Path
	B, L, U := fc.Base, fc.Local, fc.Upstream
	bOK, lOK, uOK := fc.BaseOK, fc.LocalOK, fc.UpstreamOK

	if uOK && canonical.HasConflictMarker(string(U)) {
		return UpstreamFile{
			Path:          p,
			Status:        "blocked",
			DefaultAction: "local",
			Action:        "local",
			Mergeable:     false,
		}, false, nil
	}

	if lOK && uOK && bytes.Equal(L, U) {
		return UpstreamFile{Path: p, Status: "unchanged"}, true, nil
	}

	uf := UpstreamFile{Path: p, Mergeable: true}

	if fc.BaseAvailable {
		switch {
		case lOK && bOK && bytes.Equal(L, B) && uOK && !bytes.Equal(U, B):
			uf.Status, uf.DefaultAction, uf.Action = "upstream_only", "upstream", "upstream"
		case uOK && bOK && bytes.Equal(U, B) && lOK && !bytes.Equal(L, B):
			uf.Status, uf.DefaultAction, uf.Action = "local_only", "local", "local"
		case !bOK && uOK && !lOK:
			uf.Status, uf.DefaultAction, uf.Action = "added_upstream", "upstream", "upstream"
		case !bOK && lOK && !uOK:
			uf.Status, uf.DefaultAction, uf.Action = "added_local", "local", "local"
		case bOK && lOK && bytes.Equal(L, B) && !uOK:
			uf.Status, uf.DefaultAction, uf.Action = "removed_upstream", "upstream", "upstream"
		case bOK && lOK && !bytes.Equal(L, B) && !uOK:
			uf.Status = "removed_upstream"
		case bOK && uOK && bytes.Equal(U, B) && !lOK:
			uf.Status, uf.DefaultAction, uf.Action = "removed_local", "local", "local"
		case bOK && uOK && !bytes.Equal(U, B) && !lOK:
			uf.Status = "removed_local"
		default:
			uf.Status = "both_changed"
			if lOK && uOK && bOK && mergeableText(L) && mergeableText(U) && mergeableText(B) {
				merged, conflicts, mErr := gitMergeFile(ctx, root, B, L, U)
				if mErr != nil {
					return UpstreamFile{}, false, mErr
				}
				uf.Conflicts = conflicts
				if conflicts == 0 {
					uf.DefaultAction, uf.Action = "merged", "merged"
				} else {
					uf.MergedWithMarkers = string(merged)
				}
			} else {
				uf.Mergeable = false
			}
		}
	} else {
		uf.Status = "both_changed"
		uf.Mergeable = false
	}

	if bOK && uOK && mergeableText(B) && mergeableText(U) {
		uf.UpstreamDiff, _ = gitDiffNoIndex(ctx, root, p, B, U)
	}
	if bOK && lOK && mergeableText(B) && mergeableText(L) {
		uf.LocalDiff, _ = gitDiffNoIndex(ctx, root, p, B, L)
	}

	return uf, false, nil
}

type updatePreconditions struct {
	Tracked      TrackedSkill
	SourceRec    *sourcepkg.Record
	State        *sourcepkg.UpstreamState
	MetaBytes    []byte
	TargetCommit string
}

func validateUpdatePreconditions(ctx context.Context, root string, input UpstreamUpdateInput) (updatePreconditions, *Error, error) {
	skillID := strings.TrimSpace(input.SkillID)
	if skillID == "" {
		return updatePreconditions{}, nil, errors.New("skill_id is required")
	}

	_, skillRelDir, metaBytes, err := locateSkillDir(root, skillID)
	if err != nil {
		return updatePreconditions{}, nil, fmt.Errorf("locate skill %q: %w", skillID, err)
	}

	var metaDoc struct {
		Sources []struct {
			ID          string   `yaml:"id"`
			Roles       []string `yaml:"roles"`
			Kind        string   `yaml:"kind"`
			Repo        string   `yaml:"repo"`
			Repository  string   `yaml:"repository"`
			Ref         string   `yaml:"ref"`
			Commit      string   `yaml:"commit"`
			Path        string   `yaml:"path"`
			FilesDigest string   `yaml:"files_digest"`
		} `yaml:"sources"`
		Provenance struct {
			SourceID string      `yaml:"source_id"`
			Origin   SkillOrigin `yaml:"origin"`
		} `yaml:"provenance"`
	}
	if err := yaml.Unmarshal(metaBytes, &metaDoc); err != nil {
		return updatePreconditions{}, nil, fmt.Errorf("parse skill metadata: %w", err)
	}

	sourceID := metaDoc.Provenance.SourceID
	origin := metaDoc.Provenance.Origin
	for _, s := range metaDoc.Sources {
		for _, r := range s.Roles {
			if r == "upstream" {
				sourceID = s.ID
				repo := s.Repository
				if repo == "" {
					repo = s.Repo
				}
				origin = SkillOrigin{
					Kind:        s.Kind,
					Repository:  repo,
					Ref:         s.Ref,
					Commit:      s.Commit,
					Path:        s.Path,
					FilesDigest: s.FilesDigest,
				}
				break
			}
		}
	}

	tracked := TrackedSkill{
		SkillID:     skillID,
		SkillRelDir: skillRelDir,
		SourceID:    sourceID,
		Origin:      origin,
	}
	if tracked.SourceID == "" || (tracked.Origin.Kind != "github" && tracked.Origin.Kind != "git") {
		return updatePreconditions{}, NewInvalidRequestError(
			fmt.Sprintf("skill %q is not tracked by an upstream repository", skillID),
			"Run `skillhub source backfill` to attach this skill to an upstream source.",
		), nil
	}

	store := sourcepkg.OperationalStore{Root: root}
	states, _ := store.ListUpstream(ctx)
	var state *sourcepkg.UpstreamState
	for i := range states {
		if states[i].SkillID == skillID {
			state = &states[i]
			break
		}
	}

	_, records, err := readSourceRecords(root)
	if err != nil {
		return updatePreconditions{}, nil, err
	}
	var sourceRec *sourcepkg.Record
	for i := range records {
		if records[i].ID == tracked.SourceID {
			sourceRec = &records[i]
			break
		}
	}

	localDigest := workingTreeSkillFilesDigest(root, skillRelDir)
	status, _, _ := deriveUpstreamStatus(tracked.Origin, tracked.SourceID, sourceRec, state, localDigest)

	if status != "update_available" && status != "diverged" {
		var why, fix string
		switch status {
		case "unknown", "unavailable":
			why = fmt.Sprintf("Upstream state for skill %q is %s.", skillID, status)
			fix = fmt.Sprintf("Run `skillhub source check %s`.", tracked.SourceID)
		case "up_to_date", "modified":
			why = "Upstream is unchanged."
			fix = "No update needed."
		case "upstream_removed":
			why = "The skill folder no longer exists upstream."
			fix = "Keep your local copy or archive the skill."
		case "pinned":
			why = "Skill is pinned to a commit SHA."
			fix = "Unpin the skill in its origin metadata to check updates."
		default:
			why = fmt.Sprintf("Skill %q cannot be updated (status %s).", skillID, status)
			fix = "Check upstream status."
		}
		return updatePreconditions{}, NewInvalidRequestError(why, fix), nil
	}

	targetCommit := state.CheckedCommit
	if input.TargetCommit != "" && input.TargetCommit != targetCommit {
		return updatePreconditions{}, NewStaleProposalError("upstream moved since you reviewed it", "Run `skillhub source check` to refresh."), nil
	}

	return updatePreconditions{
		Tracked:      tracked,
		SourceRec:    sourceRec,
		State:        state,
		MetaBytes:    metaBytes,
		TargetCommit: targetCommit,
	}, nil, nil
}

type fileClassificationResult struct {
	Files          []UpstreamFile
	Unresolved     []string
	UnchangedCount int
	ResolvedBytes  map[string][]byte
	IsDeleteFile   map[string]bool
}

type updateFileSets struct {
	Base          map[string][]byte
	Local         map[string][]byte
	Upstream      map[string][]byte
	BaseAvailable bool
}

func classifyAllFiles(
	ctx context.Context,
	root string,
	sets updateFileSets,
	resolutions []UpstreamResolution,
) (fileClassificationResult, *Error, error) {
	resMap := make(map[string]UpstreamResolution, len(resolutions))
	for _, r := range resolutions {
		resMap[r.Path] = r
	}

	allPathsMap := make(map[string]struct{}, len(sets.Base)+len(sets.Local)+len(sets.Upstream))
	for p := range sets.Base {
		allPathsMap[p] = struct{}{}
	}
	for p := range sets.Local {
		allPathsMap[p] = struct{}{}
	}
	for p := range sets.Upstream {
		allPathsMap[p] = struct{}{}
	}
	allPaths := make([]string, 0, len(allPathsMap))
	for p := range allPathsMap {
		allPaths = append(allPaths, p)
	}
	sort.Strings(allPaths)

	res := fileClassificationResult{
		ResolvedBytes: make(map[string][]byte),
		IsDeleteFile:  make(map[string]bool),
	}

	for _, p := range allPaths {
		B, bOK := sets.Base[p]
		L, lOK := sets.Local[p]
		U, uOK := sets.Upstream[p]

		fc := fileClassifyContext{
			Path:          p,
			Base:          B,
			Local:         L,
			Upstream:      U,
			BaseOK:        bOK,
			LocalOK:       lOK,
			UpstreamOK:    uOK,
			BaseAvailable: sets.BaseAvailable,
		}

		uf, isUnchanged, cErr := classifyUpstreamFile(ctx, root, fc)
		if cErr != nil {
			return fileClassificationResult{}, nil, cErr
		}
		if isUnchanged {
			res.UnchangedCount++
			continue
		}

		if r, hasRes := resMap[p]; hasRes {
			switch r.Action {
			case "upstream":
				if uf.Status == "blocked" {
					return fileClassificationResult{}, NewInvalidRequestError(fmt.Sprintf("file %q is blocked; only 'local' action is allowed", p), "Choose action 'local'."), nil
				}
				uf.Action = "upstream"
			case "local":
				uf.Action = "local"
			case "merged":
				if uf.Conflicts > 0 || !uf.Mergeable {
					return fileClassificationResult{}, NewInvalidRequestError(fmt.Sprintf("file %q cannot use action 'merged' with conflicts", p), "Resolve with 'manual', 'local', or 'upstream'."), nil
				}
				uf.Action = "merged"
			case "manual":
				if canonical.HasConflictMarker(r.Content) {
					return fileClassificationResult{}, NewInvalidRequestError(fmt.Sprintf("manual content for %q contains conflict markers", p), "Remove conflict markers."), nil
				}
				uf.Action = "manual"
			default:
				return fileClassificationResult{}, NewInvalidRequestError(fmt.Sprintf("invalid action %q for %q", r.Action, p), "Use 'upstream', 'local', 'merged', or 'manual'."), nil
			}
		}

		if uf.Action == "" {
			res.Unresolved = append(res.Unresolved, p)
		} else {
			switch uf.Action {
			case "upstream":
				if uOK {
					res.ResolvedBytes[p] = U
				} else {
					res.IsDeleteFile[p] = true
				}
			case "local":
				if lOK {
					res.ResolvedBytes[p] = L
				} else {
					res.IsDeleteFile[p] = true
				}
			case "merged":
				merged, _, _ := gitMergeFile(ctx, root, B, L, U)
				res.ResolvedBytes[p] = merged
			case "manual":
				res.ResolvedBytes[p] = []byte(resMap[p].Content)
			}
			if lOK && mergeableText(L) {
				if res.IsDeleteFile[p] {
					uf.ResultDiff, _ = gitDiffNoIndex(ctx, root, p, L, nil)
				} else if mergeableText(res.ResolvedBytes[p]) {
					uf.ResultDiff, _ = gitDiffNoIndex(ctx, root, p, L, res.ResolvedBytes[p])
				}
			}
		}

		res.Files = append(res.Files, uf)
	}

	return res, nil, nil
}

type updateWriteContext struct {
	Root           string
	SkillID        string
	SkillRelDir    string
	TargetCommit   string
	IdempotencyKey string
	MetaBytes      []byte
	Tracked        TrackedSkill
	SourceRec      *sourcepkg.Record
	Adapter        sourcepkg.Adapter
	LocalFiles     map[string][]byte
	ResolvedBytes  map[string][]byte
	UpstreamFiles  map[string][]byte
	IsDeleteFile   map[string]bool
	Now            time.Time
}

func buildUpstreamFileChanges(wc updateWriteContext) ([]mutation.Change, skill.DiffSummary, map[string][]byte) {
	var changes []mutation.Change
	diffAdded := []string{}
	diffModified := []string{}
	diffDeleted := []string{}

	resultingFiles := make(map[string][]byte, len(wc.LocalFiles))
	for p, b := range wc.LocalFiles {
		resultingFiles[p] = b
	}

	for p, finalData := range wc.ResolvedBytes {
		localData, lOK := wc.LocalFiles[p]
		fullPath := wc.SkillRelDir + "/" + p
		localDigest := ""
		if lOK {
			localDigest = sourcepkg.Digest(localData)
		}
		if !lOK || !bytes.Equal(localData, finalData) {
			changes = append(changes, mutation.Change{
				Path:         fullPath,
				BeforeDigest: localDigest,
				Contents:     finalData,
			})
			if !lOK {
				diffAdded = append(diffAdded, fullPath)
			} else {
				diffModified = append(diffModified, fullPath)
			}
		}
		resultingFiles[p] = finalData
	}

	for p := range wc.IsDeleteFile {
		localData, lOK := wc.LocalFiles[p]
		if lOK {
			fullPath := wc.SkillRelDir + "/" + p
			changes = append(changes, mutation.Change{
				Path:         fullPath,
				BeforeDigest: sourcepkg.Digest(localData),
				Delete:       true,
			})
			diffDeleted = append(diffDeleted, fullPath)
			delete(resultingFiles, p)
		}
	}

	return changes, skill.DiffSummary{
		Added:    diffAdded,
		Modified: diffModified,
		Deleted:  diffDeleted,
	}, resultingFiles
}

func buildAndPlanUpstreamWriteSet(ctx context.Context, wc updateWriteContext) (mutation.Proposal, skill.DiffSummary, ConfirmationPins, error) {
	changes, diffSummary, _ := buildUpstreamFileChanges(wc)

	var newFolderDigest string
	if revAdapter, ok := wc.Adapter.(revisionAtAdapter); ok {
		if rev, err := revAdapter.RevisionAt(ctx, sourcepkg.Source{ID: wc.SourceRec.ID, Locator: sourcepkg.Locator{Repository: wc.SourceRec.Locator.Repository, Ref: wc.SourceRec.Locator.Ref, Path: wc.Tracked.Origin.Path}}, wc.TargetCommit); err == nil {
			newFolderDigest = rev.ContentDigest
		}
	}
	newFilesDigest := filesDigestOf(wc.UpstreamFiles)

	newMetaBytes, err := updateSkillMetaYAML(wc.MetaBytes, wc.TargetCommit, newFolderDigest, newFilesDigest)
	if err != nil {
		return mutation.Proposal{}, skill.DiffSummary{}, ConfirmationPins{}, fmt.Errorf("update skill.meta.yaml: %w", err)
	}

	schemaVersion := 3
	if v, vErr := migration.DetectVersion(wc.Root); vErr == nil && v > 0 {
		schemaVersion = v
	}
	metaPath := wc.SkillRelDir + "/.meta/skill.yaml"
	if schemaVersion < 3 {
		metaPath = wc.SkillRelDir + "/skill.meta.yaml"
	}
	changes = append(changes, mutation.Change{
		Path:         metaPath,
		BeforeDigest: sourcepkg.Digest(wc.MetaBytes),
		Contents:     newMetaBytes,
	})
	diffSummary.Modified = append(diffSummary.Modified, metaPath)

	idempotencyKey := strings.TrimSpace(wc.IdempotencyKey)
	if idempotencyKey == "" {
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s", wc.SkillID, wc.TargetCommit, newFilesDigest)))
		idempotencyKey = fmt.Sprintf("upstream_update:%s:%s", wc.SkillID, hex.EncodeToString(hash[:16]))
	}

	expectedBeforeDigests := make(map[string]string, len(changes))
	for _, c := range changes {
		expectedBeforeDigests[c.Path] = c.BeforeDigest
	}

	writeSet := mutation.WriteSet{
		Command:        "skill_upstream_update",
		IdempotencyKey: idempotencyKey,
		RequestDigest:  sourcepkg.Digest([]byte(idempotencyKey)),
		Changes:        changes,
	}

	planned, err := mutation.PlanMutation(wc.Root, writeSet)
	if err != nil {
		return mutation.Proposal{}, skill.DiffSummary{}, ConfirmationPins{}, err
	}

	for _, pc := range planned.WriteSet.Changes {
		if exp, ok := expectedBeforeDigests[pc.Path]; ok && exp != pc.BeforeDigest {
			return mutation.Proposal{}, skill.DiffSummary{}, ConfirmationPins{}, NewStaleProposalError(fmt.Sprintf("file %s changed on disk before planning", pc.Path), "Refresh preview.")
		}
	}

	if err := storeProposalArtifact(wc.Root, storedProposalArtifact{
		Version:      1,
		Kind:         skill.ProposalKindUpstreamUpdate,
		CreatedAt:    wc.Now,
		ExpiresAt:    wc.Now.Add(24 * time.Hour),
		ID:           planned.ID,
		Digest:       planned.Digest,
		BaseSnapshot: planned.BaseCatalogSnapshot,
		SkillID:      wc.SkillID,
		Command:      "skill_upstream_update",
		Summary:      diffSummary,
		WriteSet:     planned.WriteSet,
	}); err != nil {
		return mutation.Proposal{}, skill.DiffSummary{}, ConfirmationPins{}, err
	}

	pins := ConfirmationPins{
		ProposalID:     planned.ID,
		ProposalDigest: planned.Digest,
		BaseVersion:    planned.BaseCatalogSnapshot,
	}

	return planned, diffSummary, pins, nil
}

func reconstructBaseAndUpstreamFiles(
	ctx context.Context,
	adapter sourcepkg.Adapter,
	sourceRec sourcepkg.Record,
	tracked TrackedSkill,
	targetCommit string,
) (updateFileSets, []Warning, *Error, error) {
	var warnings []Warning

	baseFiles, _, _, baseErr := reconstructSkillFilesAtCommit(ctx, adapter, sourceRec, tracked.Origin.Path, tracked.Origin.Commit, tracked.SkillID)
	baseAvailable := baseErr == nil
	if baseAvailable && tracked.Origin.FilesDigest != "" {
		if filesDigestOf(baseFiles) != tracked.Origin.FilesDigest {
			baseAvailable = false
			warnings = append(warnings, Warning{Code: "upstream_base_mismatch", Summary: "Reconstructed base files do not match recorded files digest."})
		}
	} else if !baseAvailable {
		warnings = append(warnings, Warning{Code: "upstream_base_unavailable", Summary: "Upstream base commit could not be retrieved."})
	}

	rawUpstreamFiles, upstreamWarnings, _, uErr := reconstructSkillFilesAtCommit(ctx, adapter, sourceRec, tracked.Origin.Path, targetCommit, tracked.SkillID)
	if uErr != nil {
		return updateFileSets{}, nil, nil, fmt.Errorf("reconstruct upstream files: %w", uErr)
	}
	warnings = append(warnings, upstreamWarnings...)
	upstreamFiles, pathWarnings, pathErr := validateAndFilterUpstreamPaths(rawUpstreamFiles)
	if pathErr != nil {
		var invErr *Error
		if errors.As(pathErr, &invErr) {
			return updateFileSets{}, nil, invErr, nil
		}
		return updateFileSets{}, nil, nil, pathErr
	}
	warnings = append(warnings, pathWarnings...)

	return updateFileSets{
		Base:          baseFiles,
		Upstream:      upstreamFiles,
		BaseAvailable: baseAvailable,
	}, warnings, nil, nil
}

func (service UpstreamService) PreviewUpdate(ctx context.Context, path string, input UpstreamUpdateInput) (UpstreamUpdatePreview, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return UpstreamUpdatePreview{}, err
	}
	service = service.defaults(root)

	pre, reqErr, err := validateUpdatePreconditions(ctx, root, input)
	if err != nil {
		return UpstreamUpdatePreview{}, err
	}
	if reqErr != nil {
		return UpstreamUpdatePreview{Result: ErrorResult(reqErr)}, nil
	}

	adapter, ok := service.Adapters[pre.SourceRec.Adapter]
	if !ok {
		return UpstreamUpdatePreview{}, fmt.Errorf("source adapter %q is not configured", pre.SourceRec.Adapter)
	}

	fileSets, warnings, invErr, err := reconstructBaseAndUpstreamFiles(ctx, adapter, *pre.SourceRec, pre.Tracked, pre.TargetCommit)
	if err != nil {
		return UpstreamUpdatePreview{}, err
	}
	if invErr != nil {
		return UpstreamUpdatePreview{Result: ErrorResult(invErr)}, nil
	}

	localFiles, err := readLocalSkillFiles(root, pre.Tracked.SkillRelDir)
	if err != nil {
		return UpstreamUpdatePreview{}, fmt.Errorf("read local skill files: %w", err)
	}
	fileSets.Local = localFiles

	classRes, classErr, err := classifyAllFiles(ctx, root, fileSets, input.Resolutions)
	if err != nil {
		return UpstreamUpdatePreview{}, err
	}
	if classErr != nil {
		return UpstreamUpdatePreview{Result: ErrorResult(classErr)}, nil
	}

	baseAvailable := fileSets.BaseAvailable

	reviewService := SkillService{}
	trust, _ := reviewService.ContentTrustFor(ctx, root, pre.Tracked.SkillID)
	targetCommittedAt := ""
	if !pre.State.CheckedCommitAt.IsZero() {
		targetCommittedAt = pre.State.CheckedCommitAt.UTC().Format(time.RFC3339)
	}

	if len(classRes.Unresolved) > 0 {
		return makeUnresolvedPreview(pre, classRes, warnings, trust.Approved, targetCommittedAt, baseAvailable), nil
	}

	res := NewResult(StatusActionRequired, "Upstream update preview ready.")
	res.Warnings = warnings

	preview := UpstreamUpdatePreview{
		Result:            res,
		SkillID:           pre.Tracked.SkillID,
		SourceID:          pre.Tracked.SourceID,
		BaseCommit:        pre.Tracked.Origin.Commit,
		TargetCommit:      pre.TargetCommit,
		TargetCommittedAt: targetCommittedAt,
		BaseAvailable:     baseAvailable,
		Files:             classRes.Files,
		UnchangedCount:    classRes.UnchangedCount,
		Unresolved:        classRes.Unresolved,
		TrustImpact: TrustImpact{
			ThirdParty:               true,
			CurrentlyApproved:        trust.Approved,
			ReviewRequiredAfterApply: true,
			ReviewCommand:            fmt.Sprintf("skillhub skill review %s", pre.Tracked.SkillID),
		},
	}

	now := service.Clock.Now().UTC()
	wc := updateWriteContext{
		Root:           root,
		SkillID:        pre.Tracked.SkillID,
		SkillRelDir:    pre.Tracked.SkillRelDir,
		TargetCommit:   pre.TargetCommit,
		IdempotencyKey: input.IdempotencyKey,
		MetaBytes:      pre.MetaBytes,
		Tracked:        pre.Tracked,
		SourceRec:      pre.SourceRec,
		Adapter:        adapter,
		LocalFiles:     localFiles,
		ResolvedBytes:  classRes.ResolvedBytes,
		UpstreamFiles:  fileSets.Upstream,
		IsDeleteFile:   classRes.IsDeleteFile,
		Now:            now,
	}

	planned, diffSummary, pins, err := buildAndPlanUpstreamWriteSet(ctx, wc)
	if err != nil {
		var staleErr *Error
		if errors.As(err, &staleErr) {
			return UpstreamUpdatePreview{Result: ErrorResult(staleErr)}, nil
		}
		return UpstreamUpdatePreview{}, err
	}

	preview.Diff = diffSummary
	preview.Confirmation = ConfirmationPolicy{
		PolicyRevision:     "policy_v1",
		ActionClass:        "semantic",
		ApplicationCommand: "skill_upstream_update",
		Confirmation: ConfirmationRequirement{
			Required: true,
			Mode:     "preview-and-approval",
			Pins:     pins,
		},
	}
	preview.planned = planned
	preview.expiresAt = now.Add(24 * time.Hour)

	return preview, nil
}

func makeUnresolvedPreview(
	pre updatePreconditions,
	classRes fileClassificationResult,
	warnings []Warning,
	approved bool,
	targetCommittedAt string,
	baseAvailable bool,
) UpstreamUpdatePreview {
	res := NewResult(StatusActionRequired, fmt.Sprintf("%d file(s) need a decision before this update can be applied.", len(classRes.Unresolved)))
	res.Warnings = warnings
	return UpstreamUpdatePreview{
		Result:            res,
		SkillID:           pre.Tracked.SkillID,
		SourceID:          pre.Tracked.SourceID,
		BaseCommit:        pre.Tracked.Origin.Commit,
		TargetCommit:      pre.TargetCommit,
		TargetCommittedAt: targetCommittedAt,
		BaseAvailable:     baseAvailable,
		Files:             classRes.Files,
		UnchangedCount:    classRes.UnchangedCount,
		Unresolved:        classRes.Unresolved,
		TrustImpact: TrustImpact{
			ThirdParty:               true,
			CurrentlyApproved:        approved,
			ReviewRequiredAfterApply: true,
			ReviewCommand:            fmt.Sprintf("skillhub skill review %s", pre.Tracked.SkillID),
		},
		Confirmation: ConfirmationPolicy{
			PolicyRevision:     "policy_v1",
			ActionClass:        "semantic",
			ApplicationCommand: "skill_upstream_update",
			Confirmation: ConfirmationRequirement{
				Required: true,
				Mode:     "preview-and-approval",
				Pins:     ConfirmationPins{},
			},
		},
	}
}

func (service UpstreamService) ConfirmUpdateProposal(ctx context.Context, path string, p skill.Proposal, pins ConfirmationPins) (UpstreamUpdateResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return UpstreamUpdateResult{}, err
	}
	service = service.defaults(root)

	preview := UpstreamUpdatePreview{
		SkillID:   p.SkillID,
		planned:   p.Planned(),
		expiresAt: p.ExpiresAt,
		Confirmation: ConfirmationPolicy{
			Confirmation: ConfirmationRequirement{
				Pins: ConfirmationPins{
					ProposalID:     p.ID,
					ProposalDigest: p.Digest,
					BaseVersion:    p.BaseSnapshot,
				},
			},
		},
	}

	return service.ConfirmUpdate(ctx, path, preview, pins)
}

func (service UpstreamService) ConfirmUpdate(ctx context.Context, path string, preview UpstreamUpdatePreview, pins ConfirmationPins) (UpstreamUpdateResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return UpstreamUpdateResult{}, err
	}
	service = service.defaults(root)

	now := service.Clock.Now().UTC()
	switch verifyProposalPins(now, preview.expiresAt, true, false, preview.Confirmation.Confirmation.Pins, pins) {
	case proposalExpired:
		return UpstreamUpdateResult{
			Result: ErrorResult(NewStaleProposalError("proposal artifact expired", "Regenerate the update proposal.")),
		}, nil
	case proposalDigestMismatch, proposalPinsMismatch:
		return UpstreamUpdateResult{
			Result: ErrorResult(NewStaleProposalError("confirmation pins do not match; nothing was applied", "Regenerate the update proposal.")),
		}, nil
	}

	receipt, err := confirmAndPublish(ctx, root, preview.planned)
	if errors.Is(err, mutation.ErrConflict) {
		return UpstreamUpdateResult{
			Result: ErrorResult(NewStaleProposalError("workspace state changed after preview; nothing was applied", "Regenerate the update proposal.")),
		}, nil
	}
	if err != nil {
		return UpstreamUpdateResult{}, err
	}

	skillID := preview.SkillID
	store := sourcepkg.OperationalStore{Root: root}
	states, _ := store.ListUpstream(ctx)
	for _, st := range states {
		if st.SkillID == skillID {
			targetCommit := preview.TargetCommit
			if targetCommit == "" {
				targetCommit = st.CheckedCommit
			}
			updatedState := st
			updatedState.BaseCommit = targetCommit
			updatedState.CheckedCommit = targetCommit
			updatedState.Upstream = "same"
			updatedState.ChangedFiles = []sourcepkg.Change{}
			confirmTime := now
			if !confirmTime.After(st.CheckedAt) {
				confirmTime = st.CheckedAt.Add(time.Millisecond)
			}
			updatedState.CheckedAt = confirmTime
			_ = store.RecordUpstream(ctx, []sourcepkg.UpstreamState{updatedState})
			break
		}
	}

	reviewService := SkillService{}
	trust, _ := reviewService.ContentTrustFor(ctx, root, skillID)
	thirdParty := true
	reviewRequired := thirdParty && !trust.Approved

	summary := fmt.Sprintf("Applied upstream update for %s. Review content trust with `skillhub skill review %s`.", skillID, skillID)
	result := UpstreamUpdateResult{
		Result:       NewResult(StatusOK, summary),
		SkillID:      skillID,
		OperationID:  receipt.OperationID,
		Receipt:      receipt,
		ChangedPaths: receipt.ChangedPaths,
		TrustImpact: TrustImpact{
			ThirdParty:               thirdParty,
			CurrentlyApproved:        trust.Approved,
			ReviewRequiredAfterApply: reviewRequired,
			ReviewCommand:            fmt.Sprintf("skillhub skill review %s", skillID),
		},
	}
	return result, nil
}
