package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
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
	result := NewResult(StatusOK, "Workspace validation passed.")
	for index, issue := range issues {
		result.Items = append(result.Items, Item{
			ID:      fmt.Sprintf("validation_issue_%d", index+1),
			Summary: issue.Path + ": " + issue.Message,
			Impact:  "Correct this canonical workspace issue.",
		})
	}
	if len(issues) > 0 {
		result.Status = StatusError
		result.Summary = "Workspace validation failed."
		result.Error = NewWorkspaceInvalidError(fmt.Sprintf("Canonical validation found %d issue(s).", len(issues)))
	}
	return result, nil
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
		Result:        NewResult(StatusOK, "No uncommitted canonical changes."),
		GitConfigured: state.Configured,
		Dirty:         len(state.Files) > 0,
		Groups:        groupDiffFiles(state.Files),
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
		result.Summary = fmt.Sprintf("Git has %d uncommitted canonical file(s).", len(state.Files))
		result.SuggestedActions = []Action{{Label: "Review the grouped changes", Command: "GetCurationDiff"}}
	}
	for _, group := range result.Groups {
		result.Items = append(result.Items, Item{ID: group.Kind, Summary: fmt.Sprintf("%s: %d file(s)", group.Kind, group.Count), Impact: "Review before committing or restoring these canonical changes."})
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
	return path != "" && path != "." && !filepath.IsAbs(filepath.FromSlash(path)) && path != ".." && !strings.HasPrefix(path, "../")
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

func groupDiffFiles(files []DiffFile) []DiffGroup {
	order := []string{"active_skills", "source_learning", "routing", "operation_history", "workspace_config"}
	grouped := make(map[string][]DiffFile, len(order))
	for _, file := range files {
		kind := "workspace_config"
		switch {
		case strings.HasPrefix(file.Path, "skills/"):
			kind = "active_skills"
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
