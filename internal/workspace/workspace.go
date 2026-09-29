// Package workspace provides safe, Git-first workspace discovery and remediation.
package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const SchemaVersion = "1"

var requiredDirectories = []string{
	"skills", "sources/intake", "sources/catalog", "sources/skills",
	"distill/sources", "distill/comparisons", "distill/skills", "history/operations",
	"registry/collections", "config/schemas", "evals/routing/cases", "evals/routing/suites",
}

var requiredIgnoreEntries = []string{"/runtime/", "/.skillhub/transactions/"}

// Finding describes a workspace problem and whether the mechanical remediation can fix it.
type Finding struct {
	ID      string
	Path    string
	Summary string
	Fixable bool
}

// Plan is a deterministic remediation plan for a workspace.
type Plan struct {
	Root     string
	Findings []Finding
}

// RemediationFile is a canonical file image produced by mechanical workspace
// remediation. Existing workspaces pass these images to the mutation service;
// only first-time workspace bootstrap writes them directly.
type RemediationFile struct {
	Path     string
	Contents []byte
}

// Discover resolves an explicit workspace path or searches upward from the current directory.
func Discover(path string) (string, error) {
	if path != "" {
		return filepath.Abs(path)
	}
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(current, ".skillhub", "schema-version")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", errors.New("no workspace found; pass --workspace <path>")
}

// Inspect returns only mechanical setup findings. It never writes to disk.
func Inspect(root string) (Plan, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Plan{}, fmt.Errorf("make workspace path absolute: %w", err)
	}
	findings := make([]Finding, 0)
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		findings = append(findings, Finding{"workspace_missing", ".", "Workspace directory does not exist.", true})
		return Plan{Root: root, Findings: findings}, nil
	}
	if err != nil {
		return Plan{}, fmt.Errorf("stat workspace: %w", err)
	}
	if !info.IsDir() {
		return Plan{}, fmt.Errorf("workspace path is not a directory: %s", root)
	}
	if err := safeTarget(root); err != nil {
		return Plan{}, err
	}
	if !isGitRepository(root) {
		findings = append(findings, Finding{"git_repository_missing", ".git", "Workspace is not a Git repository.", true})
	}
	versionPath := filepath.Join(root, ".skillhub", "schema-version")
	version, versionErr := os.ReadFile(versionPath)
	if versionErr != nil || strings.TrimSpace(string(version)) != SchemaVersion {
		findings = append(findings, Finding{"schema_version_missing", ".skillhub/schema-version", "Workspace schema version is missing or incompatible.", true})
	}
	for _, dir := range requiredDirectories {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir))); err != nil || !info.IsDir() {
			findings = append(findings, Finding{"directory_missing", dir, "Required canonical directory is missing.", true})
		}
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	for _, entry := range requiredIgnoreEntries {
		if err != nil || !hasLine(string(ignore), entry) {
			findings = append(findings, Finding{"gitignore_entry_missing", ".gitignore", "Required Git ignore entry is missing: " + entry, true})
		}
	}
	return Plan{Root: root, Findings: findings}, nil
}

// Apply bootstraps a workspace directly. Existing-workspace application code
// uses PrepareLayout plus RemediationFiles and commits those files through the
// mutation WAL; this direct path is retained for first-time workspace creation.
func Apply(root string) (Plan, error) {
	plan, err := PrepareLayout(root)
	if err != nil {
		return Plan{}, err
	}
	files, err := RemediationFiles(plan.Root)
	if err != nil {
		return Plan{}, err
	}
	for _, file := range files {
		if err := writeManagedFile(plan.Root, file.Path, file.Contents, 0o644); err != nil {
			return Plan{}, fmt.Errorf("write %s: %w", file.Path, err)
		}
	}
	return Inspect(plan.Root)
}

// PrepareLayout creates only structural directories and Git metadata. Callers
// modifying an existing workspace must hold the exclusive mutation lock. File
// contents returned by RemediationFiles still require a WAL-backed mutation.
func PrepareLayout(root string) (Plan, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return Plan{}, fmt.Errorf("make workspace path absolute: %w", err)
	}
	if err := safeTarget(absoluteRoot); err != nil {
		return Plan{}, err
	}
	plan, err := Inspect(absoluteRoot)
	if err != nil {
		return Plan{}, err
	}
	if err := ensureManagedDir(plan.Root, "."); err != nil {
		return Plan{}, fmt.Errorf("create workspace directory: %w", err)
	}
	if err := safeTarget(plan.Root); err != nil {
		return Plan{}, err
	}
	if !isGitRepository(plan.Root) {
		command := exec.Command("git", "init", plan.Root)
		if output, err := command.CombinedOutput(); err != nil {
			return Plan{}, fmt.Errorf("initialize Git repository: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}
	for _, dir := range requiredDirectories {
		if err := ensureManagedDir(plan.Root, dir); err != nil {
			return Plan{}, fmt.Errorf("create canonical directory %s: %w", dir, err)
		}
	}
	if err := ensureManagedDir(plan.Root, ".skillhub"); err != nil {
		return Plan{}, fmt.Errorf("create .skillhub directory: %w", err)
	}
	return plan, nil
}

// RemediationFiles returns deterministic canonical file images without writing.
func RemediationFiles(root string) ([]RemediationFile, error) {
	contents, err := desiredGitignore(root)
	if err != nil {
		return nil, err
	}
	return []RemediationFile{
		{Path: ".gitignore", Contents: contents},
		{Path: ".skillhub/schema-version", Contents: []byte(SchemaVersion + "\n")},
	}, nil
}

func safeTarget(root string) error {
	for path := root; ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace path must not contain a symlink: %s", path)
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
	}
	// Check the closest existing ancestor before creating any path. This prevents init
	// from even creating a directory inside a source checkout or nested repository.
	ancestor := root
	for {
		if _, err := os.Stat(ancestor); err == nil {
			break
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			break
		}
		ancestor = parent
	}
	if gitRoot, err := gitRoot(ancestor); err == nil && filepath.Clean(gitRoot) != filepath.Clean(root) {
		return fmt.Errorf("refusing nested workspace inside Git repository %s; choose a directory outside it", gitRoot)
	}
	if isGitRepository(root) && looksLikeSourceCheckout(root) {
		return fmt.Errorf("refusing source checkout as workspace: %s", root)
	}
	return nil
}

func gitRoot(root string) (string, error) {
	command := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func isGitRepository(root string) bool { _, err := gitRoot(root); return err == nil }

// UnmergedGitPaths returns all paths with unresolved index stages. Conflict markers
// alone are insufficient because Git can retain an unresolved index without them.
func UnmergedGitPaths(root string) ([]string, error) {
	if !isGitRepository(root) {
		return nil, nil
	}
	command := exec.Command("git", "-C", root, "ls-files", "--unmerged", "-z")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("inspect Git index conflicts: %w", err)
	}
	paths := make(map[string]struct{})
	for _, record := range bytes.Split(output, []byte{'\x00'}) {
		if len(record) == 0 {
			continue
		}
		separator := bytes.IndexByte(record, '\t')
		if separator < 0 || separator == len(record)-1 {
			return nil, fmt.Errorf("parse Git index conflict record")
		}
		path := filepath.ToSlash(string(record[separator+1:]))
		paths[path] = struct{}{}
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}

func looksLikeSourceCheckout(root string) bool {
	for _, name := range []string{"go.mod", "package.json", "Cargo.toml", ".gitmodules"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

func hasLine(contents, entry string) bool {
	for _, line := range strings.Split(contents, "\n") {
		if strings.TrimSpace(line) == entry {
			return true
		}
	}
	return false
}

func desiredGitignore(root string) ([]byte, error) {
	const relativePath = ".gitignore"
	path := filepath.Join(root, relativePath)
	if err := rejectSymlink(path); err != nil {
		return nil, err
	}
	contents, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read .gitignore: %w", err)
	}
	for _, entry := range requiredIgnoreEntries {
		if !hasLine(string(contents), entry) {
			if len(contents) > 0 && !bytes.HasSuffix(contents, []byte("\n")) {
				contents = append(contents, '\n')
			}
			contents = append(contents, []byte(entry+"\n")...)
		}
	}
	return contents, nil
}

// ensureManagedDir creates a managed relative directory one component at a time.
// Every existing component is checked with Lstat so writes cannot traverse a symlink.
func ensureManagedDir(root, relative string) error {
	if !filepath.IsAbs(root) {
		return fmt.Errorf("workspace root must be absolute: %s", root)
	}
	current := root
	if err := ensureDirectory(current); err != nil {
		return err
	}
	if relative == "." || relative == "" {
		return nil
	}
	for _, component := range strings.Split(filepath.ToSlash(relative), "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("invalid managed directory path: %s", relative)
		}
		current = filepath.Join(current, component)
		if err := ensureDirectory(current); err != nil {
			return err
		}
	}
	return nil
}

func ensureDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed path must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("managed directory path is not a directory: %s", path)
	}
	return nil
}

func writeManagedFile(root, relative string, contents []byte, mode fs.FileMode) error {
	parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative)))
	if parent == "." {
		parent = ""
	}
	if err := ensureManagedDir(root, parent); err != nil {
		return err
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := rejectSymlink(path); err != nil {
		return err
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rootHandle.Close()
	// Root rejects links that escape the workspace even if a path changes after
	// the Lstat checks above, so managed writes never reach an external target.
	file, err := rootHandle.OpenFile(filepath.ToSlash(relative), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return fmt.Errorf("write file: %w (close: %v)", err, closeErr)
		}
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed path must not be a symlink: %s", path)
	}
	return nil
}

// RequiredDirectories exposes the canonical layout to validators without allowing mutation.
func RequiredDirectories() []string { return append([]string(nil), requiredDirectories...) }

// RelativeFiles returns sorted regular files and refuses symlinks so callers cannot escape the workspace.
func RelativeFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == ".git" || strings.HasPrefix(rel, ".git/") || rel == "runtime" || strings.HasPrefix(rel, "runtime/") || rel == ".skillhub/transactions" || strings.HasPrefix(rel, ".skillhub/transactions/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("unsafe symlink: %s", rel)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported non-regular canonical path: %s", rel)
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}
