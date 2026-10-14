package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/workspace"
)

const (
	checkoutGitignoreComment = "# skillhub checkout"
	projectSkillsDir         = ".claude/skills"
)

// SkillCheckoutInput names the skill and the directory the command ran in.
type SkillCheckoutInput struct {
	SkillID    string `json:"skill_id"`
	ProjectDir string `json:"project_dir"`
}

// SkillCheckoutResult reports what a checkout copied into the project.
type SkillCheckoutResult struct {
	Result
	SkillID string `json:"skill_id"`
	State   string `json:"state"`
	// Path is the project folder, relative to the project root.
	Path        string `json:"path"`
	ProjectRoot string `json:"project_root"`
	FileCount   int    `json:"file_count"`
	BaseVersion string `json:"base_version"`
	BaseCommit  string `json:"base_commit,omitempty"`
	// GitignoreAdded is the line the checkout added to the project's .gitignore.
	GitignoreAdded string `json:"gitignore_added,omitempty"`
}

// SkillCheckoutService copies a hub skill into a project to edit and try.
type SkillCheckoutService struct {
	Clock Clock
}

// Checkout copies every content file of a hub skill into
// <project>/.claude/skills/<id>/ and records where it came from. It writes only
// in the project: the hub keeps serving the current version and gets no commit.
func (service SkillCheckoutService) Checkout(ctx context.Context, path string, input SkillCheckoutInput) (SkillCheckoutResult, error) {
	if service.Clock == nil {
		service.Clock = SystemClock{}
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return SkillCheckoutResult{}, err
	}
	id := strings.TrimSpace(input.SkillID)
	if !skillTargetIDPattern.MatchString(id) {
		return checkoutError(NewInvalidRequestError(fmt.Sprintf("invalid skill id %q", id), "Use the lowercase id shown by `skillhub skill list`.")), nil
	}
	_, relDir, metaBytes, err := locateSkillDir(root, id)
	if err != nil {
		return SkillCheckoutResult{}, err
	}
	projectRoot := projectRootOf(ctx, input.ProjectDir)
	target := filepath.Join(projectRoot, filepath.FromSlash(projectSkillsDir), id)
	if refusal := checkoutTargetRefusal(target, id); refusal != nil {
		return checkoutError(refusal), nil
	}
	tree, err := readHubSkillTree(root, relDir)
	if err != nil {
		return SkillCheckoutResult{}, err
	}
	baseCommit := ""
	if dirty, dirtyErr := workspace.DirtyPaths(ctx, root, []string{relDir}); dirtyErr == nil && len(dirty) == 0 {
		baseCommit = workspace.HeadCommit(ctx, root)
	}
	marker := CheckoutMarker{
		SkillID:      id,
		Hub:          root,
		BaseVersion:  skillBaseVersion(tree),
		BaseCommit:   baseCommit,
		CheckedOutAt: service.Clock.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	if err := writeCheckoutFolder(target, tree, marker); err != nil {
		return SkillCheckoutResult{}, err
	}
	pattern := "/" + projectSkillsDir + "/" + id + "/"
	added, err := ensureGitignored(projectRoot, pattern)
	if err != nil {
		return SkillCheckoutResult{}, err
	}
	state := parseSkillReviewMeta(metaBytes).Status
	result := SkillCheckoutResult{
		Result:      NewResult(StatusOK, fmt.Sprintf("Copied %s (%s, %s) to %s/%s/", id, state, pluralize(len(tree), "file"), projectSkillsDir, id)),
		SkillID:     id,
		State:       state,
		Path:        projectSkillsDir + "/" + id,
		ProjectRoot: projectRoot,
		FileCount:   len(tree),
		BaseVersion: marker.BaseVersion,
		BaseCommit:  marker.BaseCommit,
	}
	if added {
		result.GitignoreAdded = pattern
	}
	return result, nil
}

func checkoutError(err *Error) SkillCheckoutResult {
	return SkillCheckoutResult{Result: ErrorResult(err)}
}

// checkoutTargetRefusal explains why a checkout cannot use target: the folder is
// already there, as a checkout or as something else.
func checkoutTargetRefusal(target, id string) *Error {
	if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if _, found, markerErr := ReadCheckoutMarker(target); found || markerErr != nil {
		return NewSkillConflictError(
			fmt.Sprintf("%s/%s is already checked out", projectSkillsDir, id),
			fmt.Sprintf("Already checked out: edit it, then run `skillhub skill commit %s`.", id),
		)
	}
	return NewSkillConflictError(
		fmt.Sprintf("%s/%s already exists and was not checked out from the hub", projectSkillsDir, id),
		fmt.Sprintf("`skillhub skill commit %s/%s` adds or updates it.", projectSkillsDir, id),
	)
}

// writeCheckoutFolder writes the tree and the marker into a temporary folder
// beside target, then moves it into place, so a failed checkout leaves nothing
// that looks like a finished one.
func writeCheckoutFolder(target string, tree map[string]treeFile, marker CheckoutMarker) (err error) {
	parent := filepath.Dir(target)
	if err = os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".checkout-")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(staging)
		}
	}()
	for rel, file := range tree {
		if !safeTreePath(rel) {
			return fmt.Errorf("skill file path %q leaves the skill folder", rel)
		}
		full := filepath.Join(staging, filepath.FromSlash(rel))
		if err = os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if file.Executable {
			mode = 0o755
		}
		if err = os.WriteFile(full, file.Data, mode); err != nil {
			return err
		}
		if err = os.Chmod(full, mode); err != nil {
			return err
		}
	}
	if err = WriteCheckoutMarker(staging, marker); err != nil {
		return err
	}
	if err = os.Chmod(staging, 0o755); err != nil {
		return err
	}
	return os.Rename(staging, target)
}

// projectRootOf is the Git top level of dir, or dir itself outside Git.
func projectRootOf(ctx context.Context, dir string) string {
	if top, ok := gitTopLevel(ctx, dir); ok {
		return top
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	return abs
}

// gitTopLevel is the top level of the Git work tree that holds dir.
func gitTopLevel(ctx context.Context, dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	output, err := offlineGitCommand(ctx, abs, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", false
	}
	top := strings.TrimSpace(string(output))
	return filepath.FromSlash(top), top != ""
}

// ensureGitignored appends pattern to the project's .gitignore under a
// "# skillhub checkout" comment unless a line already covers it. It reports
// whether it wrote anything.
func ensureGitignored(projectRoot, pattern string) (bool, error) {
	path := filepath.Join(projectRoot, ".gitignore")
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	text := string(existing)
	covers := coveringIgnoreLines(pattern)
	hasComment := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if covers[line] {
			return false, nil
		}
		if line == checkoutGitignoreComment {
			hasComment = true
		}
	}
	var addition strings.Builder
	if text != "" && !strings.HasSuffix(text, "\n") {
		addition.WriteString("\n")
	}
	if !hasComment {
		if text != "" {
			addition.WriteString("\n")
		}
		addition.WriteString(checkoutGitignoreComment + "\n")
	}
	addition.WriteString(pattern + "\n")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	_, writeErr := file.WriteString(addition.String())
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	return writeErr == nil, writeErr
}

// coveringIgnoreLines lists the .gitignore lines that already ignore the
// folder a pattern names, from the exact line up to the whole .claude folder.
func coveringIgnoreLines(pattern string) map[string]bool {
	folder := strings.Trim(pattern, "/")
	covers := map[string]bool{pattern: true, folder: true, "/" + folder: true, folder + "/": true}
	for dir := folder; dir != "." && dir != ""; dir = filepath.ToSlash(filepath.Dir(dir)) {
		if dir != folder {
			covers[dir], covers["/"+dir], covers[dir+"/"], covers["/"+dir+"/"] = true, true, true, true
		}
	}
	return covers
}

func pluralize(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
