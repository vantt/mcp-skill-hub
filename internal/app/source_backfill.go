package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	if targetSkillID != "" {
		found := false
		for _, id := range skillIDs {
			if id == targetSkillID {
				found = true
				break
			}
		}
		if !found {
			return BackfillPreview{
				Result: ErrorResult(NewInvalidRequestError(fmt.Sprintf("skill %q not found", targetSkillID), "Check skill ID.")),
			}, nil
		}
		skillIDs = []string{targetSkillID}
	}

	sourcesByID := make(map[string]sourcepkg.Record)
	for _, s := range existingSources {
		sourcesByID[s.ID] = s
	}

	createdSourcesMap := make(map[string]sourcepkg.Record)
	var candidates []BackfillCandidateItem
	var changes []mutation.Change
	diff := SourceDiff{Added: []string{}, Modified: []string{}, Deleted: []string{}}
	now := service.Clock.Now().UTC()

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

		// Candidate (A): origin.kind in ["github", "git"] without provenance.source_id
		if sourceID == "" && origin != nil {
			kind, _ := origin["kind"].(string)
			if kind == "github" || kind == "git" {
				repo, _ := origin["repository"].(string)
				ref, _ := origin["ref"].(string)
				if ref == "" {
					ref = "main"
				}
				originPath, _ := origin["path"].(string)
				if targetSkillID != "" && strings.TrimSpace(input.RepoPath) != "" {
					originPath = strings.TrimSpace(input.RepoPath)
				}
				commit, _ := origin["commit"].(string)

				// Find or create matching source
				var matchedSourceID string
				for _, s := range existingSources {
					if s.Adapter == "git" && sameRepository(s.Locator.Repository, repo) && (s.Locator.Ref == ref || s.Locator.Ref == "") {
						matchedSourceID = s.ID
						break
					}
				}
				if matchedSourceID == "" {
					for sID, s := range createdSourcesMap {
						if sameRepository(s.Locator.Repository, repo) && s.Locator.Ref == ref {
							matchedSourceID = sID
							break
						}
					}
				}

				createSource := false
				if matchedSourceID == "" {
					createSource = true
					matchedSourceID = deriveBackfillSourceID(repo, originPath)
					// Ensure unique ID
					baseID := matchedSourceID
					counter := 1
					for {
						_, existsInExisting := sourcesByID[matchedSourceID]
						_, existsInCreated := createdSourcesMap[matchedSourceID]
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
							ObservedAt:    now,
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
					createdSourcesMap[matchedSourceID] = newRec
					recBytes, _ := sourcepkg.MarshalCanonical(newRec)
					sourcePath := fmt.Sprintf("sources/catalog/%s.yaml", matchedSourceID)
					changes = append(changes, mutation.Change{
						Path:     sourcePath,
						Contents: recBytes,
					})
					diff.Added = append(diff.Added, sourcePath)
				}

				// Update skill provenance
				prov["source_id"] = matchedSourceID
				if originPath != "" {
					origin["path"] = originPath
				}
				prov["origin"] = origin
				doc["provenance"] = prov

				updatedMetaBytes, err := yaml.Marshal(doc)
				if err != nil {
					return BackfillPreview{}, err
				}
				skillMetaPath := filepath.ToSlash(filepath.Join(relDir, "skill.meta.yaml"))
				changes = append(changes, mutation.Change{
					Path:         skillMetaPath,
					BeforeDigest: sourcepkg.Digest(metaBytes),
					Contents:     updatedMetaBytes,
				})
				diff.Modified = append(diff.Modified, skillMetaPath)

				candidates = append(candidates, BackfillCandidateItem{
					SkillID:      id,
					Kind:         BackfillKindOriginWithoutSource,
					SourceID:     matchedSourceID,
					RepoPath:     originPath,
					CreateSource: createSource,
				})
				continue
			}
		}

		// Candidate (B): provenance.source_id set, created_by == "source_import", no origin block, source has adapter == "git"
		if sourceID != "" && createdBy == "source_import" && origin == nil {
			srcRec, found := sourcesByID[sourceID]
			if !found {
				srcRec, found = createdSourcesMap[sourceID]
			}
			if !found || srcRec.Adapter != "git" {
				// Exclude filesystem and HTTP source imports
				continue
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
			if targetSkillID != "" && strings.TrimSpace(input.RepoPath) != "" {
				repoPath = strings.TrimSpace(input.RepoPath)
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
				return BackfillPreview{}, err
			}
			skillMetaPath := filepath.ToSlash(filepath.Join(relDir, "skill.meta.yaml"))
			changes = append(changes, mutation.Change{
				Path:         skillMetaPath,
				BeforeDigest: sourcepkg.Digest(metaBytes),
				Contents:     updatedMetaBytes,
			})
			diff.Modified = append(diff.Modified, skillMetaPath)

			candidates = append(candidates, BackfillCandidateItem{
				SkillID:      id,
				Kind:         BackfillKindSourceWithoutOrigin,
				SourceID:     sourceID,
				RepoPath:     repoPath,
				CreateSource: false,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].SkillID < candidates[j].SkillID
	})

	var planned mutation.Proposal
	if len(changes) > 0 {
		set := mutation.WriteSet{
			Command:        "source_backfill",
			RequestDigest:  sourcepkg.Digest([]byte(fmt.Sprintf("backfill:%d", now.UnixNano()))),
			IdempotencyKey: fmt.Sprintf("backfill:%d", now.UnixNano()),
			Changes:        changes,
		}
		var planErr error
		planned, planErr = mutation.PlanMutation(root, set)
		if planErr != nil {
			return BackfillPreview{}, planErr
		}
	}

	preview := BackfillPreview{
		Result:     NewResult(StatusOK, fmt.Sprintf("Found %d backfill candidate(s).", len(candidates))),
		Candidates: candidates,
		Diff:       diff,
		changes:    changes,
		planned:    planned,
	}
	return preview, nil
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
