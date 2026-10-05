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
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

type SkillUpstreamFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type SkillUpstream struct {
	SkillID           string              `json:"skill_id"`
	SourceID          string              `json:"source_id"`
	Repository        string              `json:"repository"`
	Ref               string              `json:"ref"`
	Path              string              `json:"path"`
	BaseCommit        string              `json:"base_commit"`
	LatestCommit      string              `json:"latest_commit"`
	LatestCommittedAt string              `json:"latest_committed_at,omitempty"`
	ChangedFiles      int                 `json:"changed_files"`
	Files             []SkillUpstreamFile `json:"files,omitempty"`
	Local             string              `json:"local"`
	Status            string              `json:"status"`
	CheckedAt         string              `json:"checked_at"`
	Error             string              `json:"error,omitempty"`
	NextAction        string              `json:"next_action,omitempty"`
}

type TrackedSkill struct {
	SkillID     string
	SkillRelDir string
	SourceID    string
	Origin      SkillOrigin
}

type remoteRefCommitAdapter interface {
	RemoteRefCommit(ctx context.Context, repository string, ref string) (string, error)
}

type commitTimeAdapter interface {
	CommitTime(ctx context.Context, repository string, commit string) (time.Time, error)
}

var errSkillNotFoundInCommit = errors.New("skill not found in commit")

func is40Hex(s string) bool {
	if len(s) != 40 {
		return false
	}
	for i := range s {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func deriveLocalStatus(originFilesDigest, currentLocalDigest string) string {
	if originFilesDigest == "" {
		return "unknown"
	}
	if originFilesDigest == currentLocalDigest {
		return "clean"
	}
	return "modified"
}

func deriveNextAction(status, skillID, sourceID string) string {
	switch status {
	case "update_available", "diverged":
		return "skillhub skill update " + skillID
	case "upstream_removed":
		return "skillhub skill upstream " + skillID
	case "unknown", "unavailable":
		if sourceID != "" {
			return "skillhub source check " + sourceID
		}
		return ""
	case "untracked":
		return "skillhub source backfill"
	default:
		return ""
	}
}

func deriveUpstreamStatus(
	origin SkillOrigin,
	sourceID string,
	sourceRec *sourcepkg.Record,
	state *sourcepkg.UpstreamState,
	currentLocalDigest string,
) (status string, local string, errStr string) {
	local = deriveLocalStatus(origin.FilesDigest, currentLocalDigest)

	if (origin.Kind == "github" || origin.Kind == "git") && sourceID == "" {
		return "untracked", local, ""
	}

	if is40Hex(origin.Ref) {
		return "pinned", local, ""
	}

	if sourceRec == nil || !sameRepository(sourceRec.Locator.Repository, origin.Repository) || sourceRec.Locator.Ref != origin.Ref {
		return "unavailable", local, "source_origin_mismatch"
	}

	if state == nil || state.BaseCommit != origin.Commit || !sameRepository(state.Repository, origin.Repository) || state.Ref != origin.Ref || state.Path != origin.Path {
		return "unknown", local, ""
	}

	if state.Upstream == "unavailable" {
		return "unavailable", local, state.LastError
	}

	if state.Upstream == "removed" {
		return "upstream_removed", local, ""
	}

	if state.Upstream == "same" {
		if local == "modified" {
			return "modified", local, ""
		}
		return "up_to_date", local, ""
	}

	if state.Upstream == "changed" {
		if local == "modified" {
			return "diverged", local, ""
		}
		return "update_available", local, ""
	}

	if state.Upstream == "pinned" {
		return "pinned", local, ""
	}

	return "unknown", local, ""
}

func workingTreeSkillFilesDigest(root, skillRelDir string) string {
	fullSkillDir := filepath.Join(root, filepath.FromSlash(skillRelDir))
	entrypointRelPath, entrypointDigest, _ := inspectCanonicalEntrypoint(root, skillRelDir)
	_, resources, _ := inventorySkillResources(root, fullSkillDir, entrypointRelPath, entrypointDigest)
	prefix := skillRelDir + "/"
	files := make([]skillruntime.ResourceDigest, 0, len(resources))
	for _, resource := range resources {
		if relative := strings.TrimPrefix(resource.Path, prefix); relative != resource.Path {
			files = append(files, skillruntime.ResourceDigest{Path: relative, Digest: resource.Digest})
		}
	}
	return skillruntime.ContentDigest(files, skillruntime.Spec{}, false)
}

func listAllSkillIDs(root string) ([]string, error) {
	handle, err := catalog.OpenCurrent(context.Background(), root)
	if err == nil {
		defer handle.Close()
		rows, qErr := handle.DB.Query(`SELECT id FROM skills ORDER BY id ASC`)
		if qErr == nil {
			defer rows.Close()
			var ids []string
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err == nil {
					ids = append(ids, id)
				}
			}
			if len(ids) > 0 {
				return ids, nil
			}
		}
	}
	m, err := listWorkspaceSkillIDs(root)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func loadTrackedSkills(root string) ([]TrackedSkill, error) {
	skillIDs, err := listAllSkillIDs(root)
	if err != nil {
		return nil, err
	}
	var tracked []TrackedSkill
	for _, id := range skillIDs {
		_, skillRelDir, metaBytes, err := locateSkillDir(root, id)
		if err != nil || len(metaBytes) == 0 {
			continue
		}
		var meta struct {
			Provenance struct {
				SourceID string      `yaml:"source_id"`
				Origin   SkillOrigin `yaml:"origin"`
			} `yaml:"provenance"`
		}
		if err := yaml.Unmarshal(metaBytes, &meta); err != nil {
			continue
		}
		if meta.Provenance.SourceID != "" && (meta.Provenance.Origin.Kind == "github" || meta.Provenance.Origin.Kind == "git") {
			tracked = append(tracked, TrackedSkill{
				SkillID:     id,
				SkillRelDir: skillRelDir,
				SourceID:    meta.Provenance.SourceID,
				Origin:      meta.Provenance.Origin,
			})
		}
	}
	return tracked, nil
}

func loadTrackedSkillsBySource(root string) (map[string][]TrackedSkill, error) {
	tracked, err := loadTrackedSkills(root)
	if err != nil {
		return nil, err
	}
	bySource := make(map[string][]TrackedSkill)
	for _, s := range tracked {
		bySource[s.SourceID] = append(bySource[s.SourceID], s)
	}
	return bySource, nil
}

func reconstructSkillFilesAtCommit(
	ctx context.Context,
	adapter sourcepkg.Adapter,
	record sourcepkg.Record,
	originPath string,
	commit string,
	skillID string,
) (map[string][]byte, sourcepkg.Revision, error) {
	revAdapter, ok := adapter.(revisionAtAdapter)
	if !ok {
		return nil, sourcepkg.Revision{}, sourcepkg.ErrHistoryUnavailable
	}

	src := sourcepkg.Source{
		ID: record.ID,
		Locator: sourcepkg.Locator{
			Repository: record.Locator.Repository,
			Ref:        record.Locator.Ref,
			Path:       originPath,
		},
		Limits: record.Limits,
	}

	rev, err := revAdapter.RevisionAt(ctx, src, commit)
	if err != nil {
		return nil, sourcepkg.Revision{}, err
	}

	resources, err := adapter.List(ctx, src, rev, sourcepkg.Scope{})
	if err != nil {
		return nil, sourcepkg.Revision{}, err
	}

	reader := AdapterResourceReader{Adapter: adapter, Source: src, Revision: rev}
	discovered, err := DiscoverSkillsFromResources(ctx, reader, resources, "")
	if err != nil {
		return nil, sourcepkg.Revision{}, err
	}

	var rootItem *DiscoveredSkillItem
	for i := range discovered {
		if discovered[i].SkillDir == "" {
			rootItem = &discovered[i]
			break
		}
	}
	if rootItem == nil {
		return nil, sourcepkg.Revision{}, errSkillNotFoundInCommit
	}

	files, _, err := importedSkillFiles(*rootItem, skillID)
	if err != nil {
		return nil, sourcepkg.Revision{}, err
	}
	return files, rev, nil
}

func diffReconstructedFiles(baseFiles, headFiles map[string][]byte) []sourcepkg.Change {
	allPaths := make(map[string]struct{}, len(baseFiles)+len(headFiles))
	for p := range baseFiles {
		allPaths[p] = struct{}{}
	}
	for p := range headFiles {
		allPaths[p] = struct{}{}
	}
	names := make([]string, 0, len(allPaths))
	for p := range allPaths {
		names = append(names, p)
	}
	sort.Strings(names)

	var changes []sourcepkg.Change
	for _, p := range names {
		bData, bOK := baseFiles[p]
		hData, hOK := headFiles[p]
		switch {
		case !bOK && hOK:
			changes = append(changes, sourcepkg.Change{Path: p, Status: "added"})
		case bOK && !hOK:
			changes = append(changes, sourcepkg.Change{Path: p, Status: "deleted"})
		case bOK && hOK && sourcepkg.Digest(bData) != sourcepkg.Digest(hData):
			changes = append(changes, sourcepkg.Change{Path: p, Status: "modified"})
		}
	}
	return changes
}

func checkSourceUpstream(
	ctx context.Context,
	adapter sourcepkg.Adapter,
	record sourcepkg.Record,
	skills []TrackedSkill,
	existingStates map[string]sourcepkg.UpstreamState,
	now time.Time,
) ([]sourcepkg.UpstreamState, error) {
	allPinned := len(skills) > 0
	for _, sk := range skills {
		if !is40Hex(sk.Origin.Ref) {
			allPinned = false
			break
		}
	}
	if allPinned {
		states := make([]sourcepkg.UpstreamState, 0, len(skills))
		for _, sk := range skills {
			states = append(states, sourcepkg.UpstreamState{
				SkillID:        sk.SkillID,
				SourceID:       record.ID,
				Repository:     record.Locator.Repository,
				Ref:            record.Locator.Ref,
				Path:           sk.Origin.Path,
				BaseCommit:     sk.Origin.Commit,
				CheckedCommit:  sk.Origin.Commit,
				Upstream:       "pinned",
				UpstreamDigest: sk.Origin.FilesDigest,
				ChangedFiles:   nil,
				CheckedAt:      now,
			})
		}
		return states, nil
	}

	refCommitAdapter, okRef := adapter.(remoteRefCommitAdapter)
	_, okRev := adapter.(revisionAtAdapter)
	if !okRef || !okRev {
		return unavailableUpstreamStates(record, skills, now, errors.New("adapter does not support upstream check")), nil
	}

	head, err := refCommitAdapter.RemoteRefCommit(ctx, record.Locator.Repository, record.Locator.Ref)
	if err != nil {
		return unavailableUpstreamStates(record, skills, now, err), nil
	}

	allUpToDate := len(skills) > 0
	for _, sk := range skills {
		st, exists := existingStates[sk.SkillID]
		if !exists || st.CheckedCommit != head || st.BaseCommit != sk.Origin.Commit {
			allUpToDate = false
			break
		}
	}
	if allUpToDate {
		states := make([]sourcepkg.UpstreamState, 0, len(skills))
		for _, sk := range skills {
			st := existingStates[sk.SkillID]
			st.CheckedAt = now
			states = append(states, st)
		}
		return states, nil
	}

	_, syncErr := adapter.CurrentRevision(ctx, sourcepkg.Source{
		ID:      record.ID,
		Locator: sourcepkg.Locator{Repository: record.Locator.Repository, Ref: record.Locator.Ref, Path: ""},
		Limits:  record.Limits,
	})
	if syncErr != nil {
		return unavailableUpstreamStates(record, skills, now, syncErr), nil
	}

	var commitTime time.Time
	if ctAdapter, ok := adapter.(commitTimeAdapter); ok {
		if t, err := ctAdapter.CommitTime(ctx, record.Locator.Repository, head); err == nil {
			commitTime = t
		}
	}

	checkCtx := upstreamCheckContext{Head: head, CommitTime: commitTime, Now: now}
	states := make([]sourcepkg.UpstreamState, 0, len(skills))
	for _, sk := range skills {
		states = append(states, checkSkillUpstreamState(ctx, adapter, record, sk, checkCtx))
	}
	return states, nil
}

type upstreamCheckContext struct {
	Head       string
	CommitTime time.Time
	Now        time.Time
}

func unavailableUpstreamStates(record sourcepkg.Record, skills []TrackedSkill, now time.Time, err error) []sourcepkg.UpstreamState {
	states := make([]sourcepkg.UpstreamState, 0, len(skills))
	msg := sanitizeOperationalError(err)
	for _, sk := range skills {
		states = append(states, sourcepkg.UpstreamState{
			SkillID:       sk.SkillID,
			SourceID:      record.ID,
			Repository:    record.Locator.Repository,
			Ref:           record.Locator.Ref,
			Path:          sk.Origin.Path,
			BaseCommit:    sk.Origin.Commit,
			CheckedCommit: "",
			Upstream:      "unavailable",
			CheckedAt:     now,
			LastError:     msg,
		})
	}
	return states
}

func checkSkillUpstreamState(
	ctx context.Context,
	adapter sourcepkg.Adapter,
	record sourcepkg.Record,
	sk TrackedSkill,
	checkCtx upstreamCheckContext,
) sourcepkg.UpstreamState {
	if is40Hex(sk.Origin.Ref) {
		return sourcepkg.UpstreamState{
			SkillID:         sk.SkillID,
			SourceID:        record.ID,
			Repository:      record.Locator.Repository,
			Ref:             record.Locator.Ref,
			Path:            sk.Origin.Path,
			BaseCommit:      sk.Origin.Commit,
			CheckedCommit:   sk.Origin.Commit,
			CheckedCommitAt: checkCtx.CommitTime,
			Upstream:        "pinned",
			UpstreamDigest:  sk.Origin.FilesDigest,
			ChangedFiles:    nil,
			CheckedAt:       checkCtx.Now,
		}
	}

	if checkCtx.Head == sk.Origin.Commit {
		return sourcepkg.UpstreamState{
			SkillID:         sk.SkillID,
			SourceID:        record.ID,
			Repository:      record.Locator.Repository,
			Ref:             record.Locator.Ref,
			Path:            sk.Origin.Path,
			BaseCommit:      sk.Origin.Commit,
			CheckedCommit:   checkCtx.Head,
			CheckedCommitAt: checkCtx.CommitTime,
			Upstream:        "same",
			UpstreamDigest:  sk.Origin.FilesDigest,
			ChangedFiles:    []sourcepkg.Change{},
			CheckedAt:       checkCtx.Now,
		}
	}

	headFiles, headRev, err := reconstructSkillFilesAtCommit(ctx, adapter, record, sk.Origin.Path, checkCtx.Head, sk.SkillID)
	if err != nil {
		if errors.Is(err, sourcepkg.ErrPathNotFound) || errors.Is(err, errSkillNotFoundInCommit) {
			return sourcepkg.UpstreamState{
				SkillID:         sk.SkillID,
				SourceID:        record.ID,
				Repository:      record.Locator.Repository,
				Ref:             record.Locator.Ref,
				Path:            sk.Origin.Path,
				BaseCommit:      sk.Origin.Commit,
				CheckedCommit:   checkCtx.Head,
				CheckedCommitAt: checkCtx.CommitTime,
				Upstream:        "removed",
				UpstreamDigest:  "",
				ChangedFiles:    nil,
				CheckedAt:       checkCtx.Now,
			}
		}
		return sourcepkg.UpstreamState{
			SkillID:         sk.SkillID,
			SourceID:        record.ID,
			Repository:      record.Locator.Repository,
			Ref:             record.Locator.Ref,
			Path:            sk.Origin.Path,
			BaseCommit:      sk.Origin.Commit,
			CheckedCommit:   checkCtx.Head,
			CheckedCommitAt: checkCtx.CommitTime,
			Upstream:        "unavailable",
			CheckedAt:       checkCtx.Now,
			LastError:       sanitizeOperationalError(err),
		}
	}

	upstreamDigest := filesDigestOf(headFiles)
	upstreamStatus := "changed"
	if sk.Origin.FilesDigest != "" {
		if upstreamDigest == sk.Origin.FilesDigest {
			upstreamStatus = "same"
		}
	} else if sk.Origin.FolderDigest != "" && headRev.ContentDigest == sk.Origin.FolderDigest {
		upstreamStatus = "same"
	}

	baseFiles, _, baseErr := reconstructSkillFilesAtCommit(ctx, adapter, record, sk.Origin.Path, sk.Origin.Commit, sk.SkillID)
	var changedFiles []sourcepkg.Change
	if baseErr == nil {
		changedFiles = diffReconstructedFiles(baseFiles, headFiles)
	} else {
		changedFiles = nil
	}

	return sourcepkg.UpstreamState{
		SkillID:         sk.SkillID,
		SourceID:        record.ID,
		Repository:      record.Locator.Repository,
		Ref:             record.Locator.Ref,
		Path:            sk.Origin.Path,
		BaseCommit:      sk.Origin.Commit,
		CheckedCommit:   checkCtx.Head,
		CheckedCommitAt: checkCtx.CommitTime,
		Upstream:        upstreamStatus,
		UpstreamDigest:  upstreamDigest,
		ChangedFiles:    changedFiles,
		CheckedAt:       checkCtx.Now,
	}
}

func buildSkillUpstreamModel(
	sk TrackedSkill,
	sourceRec *sourcepkg.Record,
	state *sourcepkg.UpstreamState,
	localDigest string,
) SkillUpstream {
	status, local, errStr := deriveUpstreamStatus(sk.Origin, sk.SourceID, sourceRec, state, localDigest)

	latestCommit := ""
	latestCommittedAt := ""
	changedFilesCount := -1
	var files []SkillUpstreamFile

	if state != nil {
		latestCommit = state.CheckedCommit
		if !state.CheckedCommitAt.IsZero() {
			latestCommittedAt = state.CheckedCommitAt.UTC().Format(time.RFC3339)
		}
		if state.ChangedFiles != nil {
			changedFilesCount = len(state.ChangedFiles)
			files = make([]SkillUpstreamFile, 0, len(state.ChangedFiles))
			for _, f := range state.ChangedFiles {
				files = append(files, SkillUpstreamFile{Path: f.Path, Status: f.Status})
			}
		}
	}

	checkedAt := ""
	if state != nil && !state.CheckedAt.IsZero() {
		checkedAt = state.CheckedAt.UTC().Format(time.RFC3339)
	}

	return SkillUpstream{
		SkillID:           sk.SkillID,
		SourceID:          sk.SourceID,
		Repository:        sk.Origin.Repository,
		Ref:               sk.Origin.Ref,
		Path:              sk.Origin.Path,
		BaseCommit:        sk.Origin.Commit,
		LatestCommit:      latestCommit,
		LatestCommittedAt: latestCommittedAt,
		ChangedFiles:      changedFilesCount,
		Files:             files,
		Local:             local,
		Status:            status,
		CheckedAt:         checkedAt,
		Error:             errStr,
		NextAction:        deriveNextAction(status, sk.SkillID, sk.SourceID),
	}
}

func ListSkillUpstream(ctx context.Context, path string) ([]SkillUpstream, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return nil, err
	}
	tracked, err := loadTrackedSkills(root)
	if err != nil {
		return nil, err
	}
	_, records, err := readSourceRecords(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	recordsMap := make(map[string]*sourcepkg.Record, len(records))
	for i := range records {
		recordsMap[records[i].ID] = &records[i]
	}

	store := sourcepkg.OperationalStore{Root: root}
	statesList, _ := store.ListUpstream(ctx)
	statesMap := make(map[string]*sourcepkg.UpstreamState, len(statesList))
	for i := range statesList {
		statesMap[statesList[i].SkillID] = &statesList[i]
	}

	results := make([]SkillUpstream, 0, len(tracked))
	for _, sk := range tracked {
		localDigest := workingTreeSkillFilesDigest(root, sk.SkillRelDir)
		rec := recordsMap[sk.SourceID]
		st := statesMap[sk.SkillID]
		results = append(results, buildSkillUpstreamModel(sk, rec, st, localDigest))
	}
	return results, nil
}

func GetSkillUpstream(ctx context.Context, path, id string) (SkillUpstream, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return SkillUpstream{}, err
	}
	tracked, err := loadTrackedSkills(root)
	if err != nil {
		return SkillUpstream{}, err
	}
	var found *TrackedSkill
	for i := range tracked {
		if tracked[i].SkillID == id {
			found = &tracked[i]
			break
		}
	}
	if found == nil {
		return SkillUpstream{}, fmt.Errorf("skill %q not found or not tracked", id)
	}

	_, records, err := readSourceRecords(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return SkillUpstream{}, err
	}
	var rec *sourcepkg.Record
	for i := range records {
		if records[i].ID == found.SourceID {
			rec = &records[i]
			break
		}
	}

	store := sourcepkg.OperationalStore{Root: root}
	statesList, _ := store.ListUpstream(ctx)
	var st *sourcepkg.UpstreamState
	for i := range statesList {
		if statesList[i].SkillID == id {
			st = &statesList[i]
			break
		}
	}

	localDigest := workingTreeSkillFilesDigest(root, found.SkillRelDir)
	return buildSkillUpstreamModel(*found, rec, st, localDigest), nil
}

func (service SkillService) ListSkillUpstream(ctx context.Context, path string) ([]SkillUpstream, error) {
	return ListSkillUpstream(ctx, path)
}

func (service SkillService) GetSkillUpstream(ctx context.Context, path, id string) (SkillUpstream, error) {
	return GetSkillUpstream(ctx, path, id)
}
