package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

// ValidateWorkspace is the application contract behind CLI and future tool adapters.
func (WorkspaceService) ValidateWorkspace(ctx context.Context, path string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return Result{}, err
	}
	issues, err := canonical.Validate(root)
	if err != nil {
		return Result{}, err
	}
	result := formatValidationResult(issues)
	if len(issues) == 0 {
		result.Warnings = append(result.Warnings, workspaceLintWarnings(ctx, root)...)
	}
	return result, nil
}

func formatValidationResult(issues []canonical.Issue) Result {
	result := NewResult(StatusOK, "Workspace validation passed.")
	for index, issue := range issues {
		summary := issue.Path
		if issue.Line > 0 {
			summary = fmt.Sprintf("%s:%d: %s", issue.Path, issue.Line, issue.Message)
		} else {
			summary = fmt.Sprintf("%s: %s", issue.Path, issue.Message)
		}
		fix := issue.Fix
		if fix == "" {
			fix = "Correct this file and run `skillhub validate`."
		}
		result.Items = append(result.Items, Item{
			ID:      fmt.Sprintf("validation_issue_%d", index+1),
			Summary: summary,
			Impact:  fix,
		})
	}
	if len(issues) > 0 {
		result.Status = StatusError
		result.Summary = "Workspace validation failed."
		result.Error = &Error{
			Code: ErrorWorkspaceInvalid,
			Render: ErrorRender{
				Error: "The workspace has validation errors.",
				Why:   fmt.Sprintf("Canonical validation found %d issue(s).", len(issues)),
				Fix:   "Correct the findings listed above (or use `skillhub skill edit <id>`), then run `skillhub validate`.",
			},
		}
	}
	return result
}

// DiffGroup is a stable, path-safe summary of changed canonical files.
type DiffGroup struct {
	Kind  string     `json:"kind"`
	Count int        `json:"count"`
	Files []DiffFile `json:"files"`
}

// DiffFile contains only repository-relative paths and Git porcelain state.
type DiffFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// CurationDiff is shared by CLI and future workspace_diff tool adapters.
type CurationDiff struct {
	Result
	GitConfigured bool        `json:"git_configured"`
	Dirty         bool        `json:"dirty"`
	Groups        []DiffGroup `json:"groups"`
}

// GetCurationDiff returns a grouped canonical/Git summary without file contents or absolute paths.
func (WorkspaceService) GetCurationDiff(ctx context.Context, path string) (CurationDiff, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return CurationDiff{}, err
	}
	state, err := inspectGit(ctx, root)
	if err != nil {
		return CurationDiff{}, err
	}
	result := CurationDiff{
		Result:        NewResult(StatusOK, "No uncommitted changes."),
		GitConfigured: state.Configured,
		Dirty:         len(state.Files) > 0,
		Groups:        groupDiffFiles(state.Files, root),
	}
	if !state.Configured {
		result.Status = StatusActionRequired
		result.Summary = "Git is not configured for this workspace."
		result.Warnings = append(result.Warnings, Warning{Code: "git_not_configured", Summary: "Initialize the workspace Git repository before reviewing changes."})
		result.SuggestedActions = []Action{{Label: "Inspect workspace health", Command: "ValidateWorkspace"}}
		return result, nil
	}
	if result.Dirty {
		result.Status = StatusActionRequired
		result.Summary = fmt.Sprintf("Git has %d uncommitted file(s).", len(state.Files))
		result.SuggestedActions = []Action{{Label: "Review the grouped changes", Command: "GetCurationDiff"}}
	}
	for _, group := range result.Groups {
		result.Items = append(result.Items, Item{ID: group.Kind, Summary: fmt.Sprintf("%s: %d file(s)", group.Kind, group.Count), Impact: "Review before committing or restoring these changes."})
	}
	return result, nil
}

type gitState struct {
	Configured bool
	Files      []DiffFile
}

func inspectGit(ctx context.Context, root string) (gitState, error) {
	probe := offlineGitCommand(ctx, root, "rev-parse", "--is-inside-work-tree")
	output, err := probe.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return gitState{}, ctxErr
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return gitState{Configured: false, Files: []DiffFile{}}, nil
		}
		return gitState{}, fmt.Errorf("inspect Git repository: %w", err)
	}
	if strings.TrimSpace(string(output)) != "true" {
		return gitState{Configured: false, Files: []DiffFile{}}, nil
	}
	command := offlineGitCommand(ctx, root, "status", "--porcelain=v2", "-z", "--untracked-files=all")
	output, err = command.Output()
	if err != nil {
		return gitState{}, fmt.Errorf("inspect Git status: %w", err)
	}
	files, err := parsePorcelain(output)
	if err != nil {
		return gitState{}, err
	}
	return gitState{Configured: true, Files: files}, nil
}

func offlineGitCommand(ctx context.Context, root string, args ...string) *exec.Cmd {
	gitArgs := []string{
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "core.preloadIndex=false",
		"-C", root,
	}
	command := exec.CommandContext(ctx, "git", append(gitArgs, args...)...)
	command.Env = append(os.Environ(),
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		"GIT_PAGER=cat",
	)
	return command
}

func parsePorcelain(output []byte) ([]DiffFile, error) {
	records := bytes.Split(output, []byte{0})
	files := make([]DiffFile, 0, len(records))
	appendFile := func(path, status string) error {
		path = filepath.ToSlash(path)
		if !safeRelativePath(path) {
			return errors.New("Git reported an unsafe workspace path")
		}
		if canonicalDiffPath(path) {
			files = append(files, DiffFile{Path: path, Status: status})
		}
		return nil
	}
	for index := 0; index < len(records); index++ {
		record := records[index]
		if len(record) == 0 || record[0] == '#' {
			continue
		}
		switch record[0] {
		case '1':
			fields := bytes.SplitN(record, []byte{' '}, 9)
			if len(fields) != 9 || len(fields[1]) != 2 {
				return nil, errors.New("parse Git ordinary status record")
			}
			if err := appendFile(string(fields[8]), string(fields[1])); err != nil {
				return nil, err
			}
		case '2':
			fields := bytes.SplitN(record, []byte{' '}, 10)
			if len(fields) != 10 || len(fields[1]) != 2 {
				return nil, errors.New("parse Git rename status record")
			}
			index++
			if index >= len(records) || len(records[index]) == 0 {
				return nil, errors.New("parse Git rename source path")
			}
			if err := appendFile(string(fields[9]), "A"); err != nil {
				return nil, err
			}
			if bytes.Contains(fields[8], []byte("R")) {
				if err := appendFile(string(records[index]), "D"); err != nil {
					return nil, err
				}
			}
		case 'u':
			fields := bytes.SplitN(record, []byte{' '}, 11)
			if len(fields) != 11 || len(fields[1]) != 2 {
				return nil, errors.New("parse Git unmerged status record")
			}
			if err := appendFile(string(fields[10]), string(fields[1])); err != nil {
				return nil, err
			}
		case '?':
			if len(record) < 3 || record[1] != ' ' {
				return nil, errors.New("parse Git untracked status record")
			}
			if err := appendFile(string(record[2:]), "??"); err != nil {
				return nil, err
			}
		case '!':
			continue
		default:
			return nil, errors.New("parse Git status record")
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Path == files[j].Path {
			return files[i].Status < files[j].Status
		}
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func safeRelativePath(path string) bool {
	if path == "" || path == "." || path == ".." || strings.HasPrefix(path, "../") {
		return false
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return false
	}
	if filepath.VolumeName(path) != "" || filepath.IsAbs(filepath.FromSlash(path)) {
		return false
	}
	return true
}

func canonicalDiffPath(path string) bool {
	if path == ".gitignore" || path == ".skillhub/schema-version" {
		return true
	}
	for _, prefix := range []string{"skills/", "sources/", "distill/", "history/operations/", "registry/", "config/", "evals/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func groupDiffFiles(files []DiffFile, roots ...string) []DiffGroup {
	root := ""
	if len(roots) > 0 {
		root = roots[0]
	}
	order := []string{"active_skills", "draft_skills", "source_learning", "routing", "operation_history", "workspace_config"}
	grouped := make(map[string][]DiffFile, len(order))
	for _, file := range files {
		kind := "workspace_config"
		switch {
		case strings.HasPrefix(file.Path, "skills/"):
			kind = "active_skills"
			parts := strings.Split(file.Path, "/")
			if len(parts) >= 3 {
				metaPath := filepath.Join(root, parts[0], parts[1], parts[2], ".meta", "skill.yaml")
				content, err := os.ReadFile(metaPath)
				if err != nil {
					content, err = os.ReadFile(filepath.Join(root, parts[0], parts[1], parts[2], "skill.meta.yaml"))
				}
				if err == nil {
					if bytes.Contains(content, []byte("status: draft")) || bytes.Contains(content, []byte("status: \"draft\"")) {
						kind = "draft_skills"
					}
				}
			}
		case strings.HasPrefix(file.Path, "sources/"), strings.HasPrefix(file.Path, "distill/"):
			kind = "source_learning"
		case strings.HasPrefix(file.Path, "registry/"), strings.HasPrefix(file.Path, "config/"), strings.HasPrefix(file.Path, "evals/"):
			kind = "routing"
		case strings.HasPrefix(file.Path, "history/operations/"):
			kind = "operation_history"
		}
		grouped[kind] = append(grouped[kind], file)
	}
	result := make([]DiffGroup, 0, len(grouped))
	for _, kind := range order {
		if entries := grouped[kind]; len(entries) > 0 {
			result = append(result, DiffGroup{Kind: kind, Count: len(entries), Files: entries})
		}
	}
	return result
}

// GitPathSummary provides clean staged and unstaged canonical path summaries.
type GitPathSummary struct {
	Configured bool       `json:"configured"`
	Dirty      bool       `json:"dirty"`
	Staged     []DiffFile `json:"staged"`
	Unstaged   []DiffFile `json:"unstaged"`
	Untracked  []DiffFile `json:"untracked"`
	Unmerged   []DiffFile `json:"unmerged"`
}

// GetGitPathSummary returns staged, unstaged, untracked, and unmerged path summaries.
func (WorkspaceService) GetGitPathSummary(ctx context.Context, path string) (GitPathSummary, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return GitPathSummary{}, err
	}
	cmd := offlineGitCommand(ctx, root, "status", "--porcelain=v2", "-z")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return GitPathSummary{Configured: false}, nil
	}
	summary, err := parsePorcelainSummary(output)
	if err != nil {
		return GitPathSummary{}, err
	}
	summary.Configured = true
	return summary, nil
}

func parsePorcelainSummary(output []byte) (GitPathSummary, error) {
	records := bytes.Split(output, []byte{0})
	var summary GitPathSummary
	appendSafe := func(rawPath string, list *[]DiffFile, status string) error {
		p := filepath.ToSlash(rawPath)
		if !safeRelativePath(p) {
			return errors.New("Git reported an unsafe workspace path")
		}
		if canonicalDiffPath(p) {
			*list = append(*list, DiffFile{Path: p, Status: status})
		}
		return nil
	}
	for index := 0; index < len(records); index++ {
		record := records[index]
		if len(record) == 0 || record[0] == '#' {
			continue
		}
		switch record[0] {
		case '1':
			fields := bytes.SplitN(record, []byte{' '}, 9)
			if len(fields) != 9 || len(fields[1]) != 2 {
				return GitPathSummary{}, errors.New("parse Git ordinary status record")
			}
			stagedStatus := string(fields[1][0])
			unstagedStatus := string(fields[1][1])
			path := string(fields[8])
			if stagedStatus != "." {
				if err := appendSafe(path, &summary.Staged, stagedStatus); err != nil {
					return GitPathSummary{}, err
				}
			}
			if unstagedStatus != "." {
				if err := appendSafe(path, &summary.Unstaged, unstagedStatus); err != nil {
					return GitPathSummary{}, err
				}
			}
		case '2':
			fields := bytes.SplitN(record, []byte{' '}, 10)
			if len(fields) != 10 || len(fields[1]) != 2 {
				return GitPathSummary{}, errors.New("parse Git rename status record")
			}
			index++
			path := string(fields[9])
			stagedStatus := string(fields[1][0])
			unstagedStatus := string(fields[1][1])
			if stagedStatus != "." {
				if err := appendSafe(path, &summary.Staged, stagedStatus); err != nil {
					return GitPathSummary{}, err
				}
			}
			if unstagedStatus != "." {
				if err := appendSafe(path, &summary.Unstaged, unstagedStatus); err != nil {
					return GitPathSummary{}, err
				}
			}
		case 'u':
			fields := bytes.SplitN(record, []byte{' '}, 11)
			if len(fields) != 11 || len(fields[1]) != 2 {
				return GitPathSummary{}, errors.New("parse Git unmerged status record")
			}
			if err := appendSafe(string(fields[10]), &summary.Unmerged, string(fields[1])); err != nil {
				return GitPathSummary{}, err
			}
		case '?':
			if len(record) < 3 || record[1] != ' ' {
				return GitPathSummary{}, errors.New("parse Git untracked status record")
			}
			if err := appendSafe(string(record[2:]), &summary.Untracked, "??"); err != nil {
				return GitPathSummary{}, err
			}
		}
	}
	summary.Dirty = len(summary.Staged) > 0 || len(summary.Unstaged) > 0 || len(summary.Untracked) > 0 || len(summary.Unmerged) > 0
	return summary, nil
}

type OperationChange struct {
	Path                string `json:"path"`
	BeforeDigest        string `json:"before_digest"`
	AfterDigest         string `json:"after_digest"`
	Before              string `json:"before,omitempty"`
	After               string `json:"after,omitempty"`
	Diff                string `json:"diff,omitempty"`
	DiffAvailable       bool   `json:"diff_available"`
	DigestOnlyMetadata  bool   `json:"digest_only_metadata"`
	CurrentMatchesAfter bool   `json:"current_matches_after"`
}

type OperationDiffResult struct {
	Result
	OperationID     string            `json:"operation_id"`
	Changes         []OperationChange `json:"changes"`
	ReviewCommand   string            `json:"review_command"`
	RestoreGuidance []string          `json:"restore_guidance"`
	Warning         string            `json:"warning"`
}

var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

func (WorkspaceService) GetOperationDiff(ctx context.Context, path, operationID string) (OperationDiffResult, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return OperationDiffResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return OperationDiffResult{}, err
	}
	if !operationIDPattern.MatchString(operationID) {
		return OperationDiffResult{}, NewInvalidRequestError("invalid operation ID", "Pass an operation ID returned by the workspace.")
	}
	type receiptChange struct {
		Path             string `yaml:"path"`
		BeforeDigest     string `yaml:"before"`
		AfterDigest      string `yaml:"after"`
		BeforeContent    string `yaml:"before_content"`
		AfterContent     string `yaml:"after_content"`
		ContentAvailable bool   `yaml:"content_available"`
	}
	var document struct {
		ID      string          `yaml:"id"`
		Changes []receiptChange `yaml:"changes"`
	}
	found := false
	err = walkOperationsYAML(root, "history/operations", func(_ string, data []byte) error {
		var header struct {
			ID string `yaml:"id"`
		}
		if yaml.Unmarshal(data, &header) != nil || header.ID != operationID {
			return nil
		}
		if found {
			return errors.New("duplicate operation ID")
		}
		found = true
		return yaml.Unmarshal(data, &document)
	})
	if err != nil {
		return OperationDiffResult{}, err
	}
	if !found {
		return OperationDiffResult{}, errors.New("operation not found")
	}
	paths := []string{}
	changes := make([]OperationChange, 0, len(document.Changes))
	for _, stored := range document.Changes {
		paths = append(paths, stored.Path)
		currentDigest, digestErr := workspacePathDigest(root, stored.Path)
		if digestErr != nil {
			return OperationDiffResult{}, digestErr
		}
		change := OperationChange{Path: stored.Path, BeforeDigest: stored.BeforeDigest, AfterDigest: stored.AfterDigest, CurrentMatchesAfter: currentDigest == stored.AfterDigest}
		if stored.ContentAvailable {
			change.Before, change.After = stored.BeforeContent, stored.AfterContent
			change.Diff = renderBoundedOperationDiff(stored.Path, stored.BeforeContent, stored.AfterContent)
			change.DiffAvailable = true
		} else {
			change.DigestOnlyMetadata = true
		}
		changes = append(changes, change)
	}
	sort.Strings(paths)
	quoted := make([]string, len(paths))
	for index, value := range paths {
		quoted[index] = shellQuote(value)
	}
	result := OperationDiffResult{Result: NewResult(StatusOK, "Operation history loaded; retained content is shown as a bounded before/after diff and older entries are labeled digest-only metadata."), OperationID: operationID, Changes: changes, ReviewCommand: "git diff -- " + strings.Join(quoted, " "), Warning: "Skill Hub only inspects Git and never runs restore, remove, reset, checkout, or revert. Recheck current digests immediately before any manual undo."}
	for _, change := range changes {
		tracked, headDigest := inspectGitPath(root, change.Path)
		switch {
		case !change.CurrentMatchesAfter:
			result.RestoreGuidance = append(result.RestoreGuidance, fmt.Sprintf("%s: no automatic guidance; current bytes no longer match operation after-digest %s.", change.Path, change.AfterDigest))
		case tracked && change.BeforeDigest != "" && headDigest == change.BeforeDigest:
			result.RestoreGuidance = append(result.RestoreGuidance, fmt.Sprintf("%s: tracked restore candidate; current after-digest and HEAD before-digest were verified. Recheck both before manually restoring this path.", change.Path))
		case !tracked && change.BeforeDigest == "":
			result.RestoreGuidance = append(result.RestoreGuidance, fmt.Sprintf("%s: untracked creation removal candidate; current after-digest was verified. Recheck it before manually removing only this path.", change.Path))
		default:
			result.RestoreGuidance = append(result.RestoreGuidance, fmt.Sprintf("%s: use a reviewed compensating edit from retained content or Git history; no safe direct restore/removal was established.", change.Path))
		}
	}
	return result, nil
}

func walkOperationsYAML(root, relative string, visit func(string, []byte) error) error {
	base := filepath.Join(root, filepath.FromSlash(relative))
	return filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return visit(path, data)
	})
}

func workspacePathDigest(root, path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read current operation path %s: %w", path, err)
	}
	return sourcepkg.Digest(data), nil
}

func renderBoundedOperationDiff(path, before, after string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "--- before/%s\n+++ after/%s\n", path, path)
	for _, line := range strings.Split(strings.TrimSuffix(before, "\n"), "\n") {
		if before != "" {
			out.WriteString("-" + line + "\n")
		}
	}
	for _, line := range strings.Split(strings.TrimSuffix(after, "\n"), "\n") {
		if after != "" {
			out.WriteString("+" + line + "\n")
		}
	}
	return out.String()
}

func inspectGitPath(root, path string) (bool, string) {
	tracked := exec.Command("git", "-C", root, "ls-files", "--error-unmatch", "--", path)
	if tracked.Run() != nil {
		return false, ""
	}
	show := exec.Command("git", "-C", root, "show", "HEAD:"+path)
	data, err := show.Output()
	if err != nil {
		return true, ""
	}
	return true, sourcepkg.Digest(data)
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
