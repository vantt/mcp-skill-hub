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

	"github.com/vantt/mcp-skill-hub/internal/mutation"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

type BackfillInput struct {
	SkillID  string `json:"skill_id,omitempty"`
	RepoPath string `json:"repo_path,omitempty"`
}

type BackfillCandidateKind string

const (
	BackfillKindOriginWithoutSource BackfillCandidateKind = "origin_without_source"
	BackfillKindSourceWithoutOrigin BackfillCandidateKind = "source_without_origin"
)

type BackfillCandidateItem struct {
	SkillID      string                `json:"skill_id"`
	Kind         BackfillCandidateKind `json:"kind"`
	SourceID     string                `json:"source_id"`
	RepoPath     string                `json:"repo_path"`
	CreateSource bool                  `json:"create_source"`
}

type BackfillPreview struct {
	Result
	Candidates []BackfillCandidateItem `json:"candidates"`
	Diff       SourceDiff              `json:"diff"`
	changes    []mutation.Change
	planned    mutation.Proposal
}

type BackfillResult struct {
	Result
	UpdatedSkills  []string `json:"updated_skills"`
	CreatedSources []string `json:"created_sources"`
}

type BackfillService struct {
	Clock    Clock
	Adapters map[string]sourcepkg.Adapter
}

func (service BackfillService) defaults() BackfillService {
	if service.Clock == nil {
		service.Clock = SystemClock{}
	}
	return service
}

func (service SourceService) PreviewBackfill(ctx context.Context, path string, input BackfillInput) (BackfillPreview, error) {
	return (BackfillService{Clock: service.Clock, Adapters: service.Adapters}).PreviewBackfill(ctx, path, input)
}

func (service SourceService) ApplyBackfill(ctx context.Context, path string, preview BackfillPreview) (BackfillResult, error) {
	return (BackfillService{Clock: service.Clock, Adapters: service.Adapters}).ApplyBackfill(ctx, path, preview)
}

type backfillContext struct {
	targetSkillID     string
	repoPathOverride  string
	now               time.Time
	existingSources   []sourcepkg.Record
	sourcesByID       map[string]sourcepkg.Record
	createdSourcesMap map[string]sourcepkg.Record
}

func (service BackfillService) PreviewBackfill(ctx context.Context, path string, input BackfillInput) (BackfillPreview, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return BackfillPreview{}, err
	}
	service = service.defaults()

	_, existingSources, err := readSourceRecords(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return BackfillPreview{}, err
	}

	skillIDs, err := listAllSkillIDs(root)
	if err != nil {
		return BackfillPreview{}, err
	}

	targetSkillID := strings.TrimSpace(input.SkillID)
	var notFound *BackfillPreview
	skillIDs, notFound = filterBackfillSkillIDs(skillIDs, targetSkillID)
	if notFound != nil {
		return *notFound, nil
	}

	sourcesByID := make(map[string]sourcepkg.Record)
	for _, s := range existingSources {
		sourcesByID[s.ID] = s
	}

	bCtx := backfillContext{
		targetSkillID:     targetSkillID,
		repoPathOverride:  input.RepoPath,
		now:               service.Clock.Now().UTC(),
		existingSources:   existingSources,
		sourcesByID:       sourcesByID,
		createdSourcesMap: make(map[string]sourcepkg.Record),
	}

	var candidates []BackfillCandidateItem
	var changes []mutation.Change
	diff := SourceDiff{Added: []string{}, Modified: []string{}, Deleted: []string{}}

	for _, id := range skillIDs {
		_, relDir, metaBytes, err := locateSkillDir(root, id)
		if err != nil || len(metaBytes) == 0 {
			continue
		}

		var doc map[string]any
		if err := yaml.Unmarshal(metaBytes, &doc); err != nil {
			continue
		}

		prov, _ := doc["provenance"].(map[string]any)
		if prov == nil {
			prov = make(map[string]any)
		}
		sourceID, _ := prov["source_id"].(string)
		createdBy, _ := prov["created_by"].(string)
		origin, _ := prov["origin"].(map[string]any)

		if sourceID == "" && origin != nil {
			cand, chgs, added, modded, aErr := bCtx.processCandidateA(id, relDir, metaBytes, doc, prov, origin)
			if aErr != nil {
				return BackfillPreview{}, aErr
			}
			if cand != nil {
				candidates = append(candidates, *cand)
				changes = append(changes, chgs...)
				diff.Added = append(diff.Added, added...)
				diff.Modified = append(diff.Modified, modded...)
				continue
			}
		}

		if sourceID != "" && createdBy == "source_import" && origin == nil {
			cand, chgs, modded, bErr := bCtx.processCandidateB(id, relDir, sourceID, metaBytes, doc, prov)
			if bErr != nil {
				return BackfillPreview{}, bErr
			}
			if cand != nil {
				candidates = append(candidates, *cand)
				changes = append(changes, chgs...)
				diff.Modified = append(diff.Modified, modded...)
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].SkillID < candidates[j].SkillID
	})

	var planned mutation.Proposal
	if len(changes) > 0 {
		set := mutation.WriteSet{
			Command:        "source_backfill",
			RequestDigest:  sourcepkg.Digest([]byte(fmt.Sprintf("backfill:%d", bCtx.now.UnixNano()))),
			IdempotencyKey: fmt.Sprintf("backfill:%d", bCtx.now.UnixNano()),
			Changes:        changes,
		}
		var planErr error
		planned, planErr = mutation.PlanMutation(root, set)
		if planErr != nil {
			return BackfillPreview{}, planErr
		}
	}

	return BackfillPreview{
		Result:     NewResult(StatusOK, fmt.Sprintf("Found %d backfill candidate(s).", len(candidates))),
		Candidates: candidates,
		Diff:       diff,
		changes:    changes,
		planned:    planned,
	}, nil
}

func filterBackfillSkillIDs(skillIDs []string, target string) ([]string, *BackfillPreview) {
	if target == "" {
		return skillIDs, nil
	}
	for _, id := range skillIDs {
		if id == target {
			return []string{target}, nil
		}
	}
	return nil, &BackfillPreview{
		Result: ErrorResult(NewInvalidRequestError(fmt.Sprintf("skill %q not found", target), "Check skill ID.")),
	}
}

func (bc *backfillContext) processCandidateA(id, relDir string, metaBytes []byte, doc, prov, origin map[string]any) (*BackfillCandidateItem, []mutation.Change, []string, []string, error) {
	kind, _ := origin["kind"].(string)
	if kind != "github" && kind != "git" {
		return nil, nil, nil, nil, nil
	}
	repo, _ := origin["repository"].(string)
	ref, _ := origin["ref"].(string)
	if ref == "" {
		ref = "main"
	}
	originPath, _ := origin["path"].(string)
	if bc.targetSkillID != "" && strings.TrimSpace(bc.repoPathOverride) != "" {
		originPath = strings.TrimSpace(bc.repoPathOverride)
	}
	commit, _ := origin["commit"].(string)

	var matchedSourceID string
	for _, s := range bc.existingSources {
		if s.Adapter == "git" && sameRepository(s.Locator.Repository, repo) && (s.Locator.Ref == ref || s.Locator.Ref == "") {
			matchedSourceID = s.ID
			break
		}
	}
	if matchedSourceID == "" {
		for sID, s := range bc.createdSourcesMap {
			if sameRepository(s.Locator.Repository, repo) && s.Locator.Ref == ref {
				matchedSourceID = sID
				break
			}
		}
	}

	createSource := false
	var changes []mutation.Change
	var added, modified []string

	if matchedSourceID == "" {
		createSource = true
		matchedSourceID = deriveBackfillSourceID(repo, originPath)
		baseID := matchedSourceID
		counter := 1
		for {
			_, existsInExisting := bc.sourcesByID[matchedSourceID]
			_, existsInCreated := bc.createdSourcesMap[matchedSourceID]
			if !existsInExisting && !existsInCreated {
				break
			}
			matchedSourceID = fmt.Sprintf("%s-%d", baseID, counter)
			counter++
		}

		var rev *sourcepkg.Revision
		if commit != "" {
			digest := sourcepkg.Digest([]byte(commit))
			rev = &sourcepkg.Revision{
				Kind:          "git-commit",
				Value:         commit,
				ContentDigest: digest,
				ObservedAt:    bc.now,
			}
		}

		newRec := sourcepkg.Record{
			SchemaVersion:   1,
			ID:              matchedSourceID,
			Adapter:         "git",
			Locator:         sourcepkg.Locator{Repository: repo, Ref: ref},
			Status:          "watching",
			Identity:        sourcepkg.Identity{Name: matchedSourceID, Canonical: repo, DefaultBranch: ref},
			Monitoring:      sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
			Limits:          sourcepkg.Limits{TimeoutSeconds: 30, MaxBytes: sourcepkg.DefaultMaxBytes, MaxFiles: sourcepkg.DefaultMaxFiles, MaxFileBytes: sourcepkg.DefaultMaxFileSize},
			CurrentRevision: rev,
		}
		bc.createdSourcesMap[matchedSourceID] = newRec
		recBytes, _ := sourcepkg.MarshalCanonical(newRec)
		sourcePath := fmt.Sprintf("sources/catalog/%s.yaml", matchedSourceID)
		changes = append(changes, mutation.Change{Path: sourcePath, Contents: recBytes})
		added = append(added, sourcePath)
	}

	prov["source_id"] = matchedSourceID
	if originPath != "" {
		origin["path"] = originPath
	}
	prov["origin"] = origin
	doc["provenance"] = prov

	updatedMetaBytes, err := yaml.Marshal(doc)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	skillMetaPath := filepath.ToSlash(filepath.Join(relDir, "skill.meta.yaml"))
	changes = append(changes, mutation.Change{
		Path:         skillMetaPath,
		BeforeDigest: sourcepkg.Digest(metaBytes),
		Contents:     updatedMetaBytes,
	})
	modified = append(modified, skillMetaPath)

	return &BackfillCandidateItem{
		SkillID:      id,
		Kind:         BackfillKindOriginWithoutSource,
		SourceID:     matchedSourceID,
		RepoPath:     originPath,
		CreateSource: createSource,
	}, changes, added, modified, nil
}

func (bc *backfillContext) processCandidateB(id, relDir, sourceID string, metaBytes []byte, doc, prov map[string]any) (*BackfillCandidateItem, []mutation.Change, []string, error) {
	srcRec, found := bc.sourcesByID[sourceID]
	if !found {
		srcRec, found = bc.createdSourcesMap[sourceID]
	}
	if !found || srcRec.Adapter != "git" {
		return nil, nil, nil, nil
	}

	originKind := "git"
	if strings.Contains(strings.ToLower(srcRec.Locator.Repository), "github.com") {
		originKind = "github"
	}
	ref := srcRec.Locator.Ref
	if ref == "" {
		ref = "main"
	}

	repoPath := srcRec.Locator.Path
	if bc.targetSkillID != "" && strings.TrimSpace(bc.repoPathOverride) != "" {
		repoPath = strings.TrimSpace(bc.repoPathOverride)
	} else if p, _ := prov["path"].(string); p != "" {
		repoPath = p
	}

	var commit string
	if srcRec.CurrentRevision != nil {
		commit = srcRec.CurrentRevision.Value
	}

	newOrigin := map[string]any{
		"kind":       originKind,
		"repository": srcRec.Locator.Repository,
		"ref":        ref,
	}
	if repoPath != "" {
		newOrigin["path"] = repoPath
	}
	if commit != "" {
		newOrigin["commit"] = commit
	}
	prov["origin"] = newOrigin
	doc["provenance"] = prov

	updatedMetaBytes, err := yaml.Marshal(doc)
	if err != nil {
		return nil, nil, nil, err
	}
	skillMetaPath := filepath.ToSlash(filepath.Join(relDir, "skill.meta.yaml"))
	change := mutation.Change{
		Path:         skillMetaPath,
		BeforeDigest: sourcepkg.Digest(metaBytes),
		Contents:     updatedMetaBytes,
	}
	return &BackfillCandidateItem{
		SkillID:      id,
		Kind:         BackfillKindSourceWithoutOrigin,
		SourceID:     sourceID,
		RepoPath:     repoPath,
		CreateSource: false,
	}, []mutation.Change{change}, []string{skillMetaPath}, nil
}

func (service BackfillService) ApplyBackfill(ctx context.Context, path string, preview BackfillPreview) (BackfillResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return BackfillResult{}, err
	}

	if len(preview.changes) == 0 {
		return BackfillResult{
			Result: NewResult(StatusOK, "No backfill changes to apply."),
		}, nil
	}

	if _, err := confirmAndPublish(ctx, root, preview.planned); err != nil {
		return BackfillResult{}, err
	}

	var updatedSkills []string
	var createdSources []string
	for _, cand := range preview.Candidates {
		updatedSkills = append(updatedSkills, cand.SkillID)
		if cand.CreateSource {
			createdSources = append(createdSources, cand.SourceID)
		}
	}
	sort.Strings(updatedSkills)
	sort.Strings(createdSources)

	return BackfillResult{
		Result:         NewResult(StatusOK, fmt.Sprintf("Backfilled %d skill(s) and created %d source(s).", len(updatedSkills), len(createdSources))),
		UpdatedSkills:  updatedSkills,
		CreatedSources: createdSources,
	}, nil
}

func deriveBackfillSourceID(repoURL, path string) string {
	clean := strings.TrimSuffix(repoURL, ".git")
	clean = strings.TrimRight(clean, "/")
	parts := strings.Split(clean, "/")
	name := "source"
	if len(parts) > 0 && parts[len(parts)-1] != "" {
		name = parts[len(parts)-1]
	}
	name = strings.ToLower(name)
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	s := strings.Trim(sb.String(), "-")
	if s == "" {
		return "source"
	}
	return s
}
