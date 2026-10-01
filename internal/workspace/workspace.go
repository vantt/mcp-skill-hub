// Package workspace provides safe, Git-first workspace discovery and remediation.
package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	SchemaVersion = "1"

	// V1 canonical workspaces are deliberately bounded so inventory, validation,
	// and catalog builds cannot consume unbounded memory from workspace content.
	MaxCanonicalFilesV1     = 8192
	MaxCanonicalFileBytesV1 = int64(4 << 20)
	MaxCanonicalBytesV1     = int64(64 << 20)
)

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
	return InspectWithOptions(root, false)
}

// InspectWithOptions returns mechanical setup findings with optional detached mode.
func InspectWithOptions(root string, detached bool) (Plan, error) {
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
	if !detached && !isGitRepository(root) {
		findings = append(findings, Finding{"git_repository_missing", ".git", "Workspace is not a Git repository.", true})
	}
	skillhubPath := filepath.Join(root, ".skillhub")
	if err := rejectSymlink(skillhubPath); err != nil {
		return Plan{}, err
	}
	_, skillhubErr := os.Lstat(skillhubPath)
	versionPath := filepath.Join(skillhubPath, "schema-version")
	version, versionErr := readCanonicalFileBounded(versionPath, ".skillhub/schema-version")
	if versionErr != nil || strings.TrimSpace(string(version)) != SchemaVersion {
		// A missing marker only means "new workspace" when nothing canonical exists
		// yet. Otherwise it is a legacy layout that must go through migration, so
		// doctor never writes the marker over existing canonical content.
		if errors.Is(versionErr, os.ErrNotExist) && errors.Is(skillhubErr, os.ErrNotExist) && !hasCanonicalContent(root) {
			findings = append(findings, Finding{"schema_version_missing", ".skillhub/schema-version", "Workspace schema version is missing.", true})
		} else {
			summary := "Canonical schema version is incompatible with this binary."
			if errors.Is(versionErr, os.ErrNotExist) {
				summary = "Canonical schema marker is missing (legacy version 0)."
			} else if versionErr != nil {
				return Plan{}, fmt.Errorf("read canonical schema version: %w", versionErr)
			}
			findings = append(findings, Finding{"canonical_schema_incompatible", ".skillhub/schema-version", summary, false})
		}
	}
	for _, dir := range requiredDirectories {
		target := filepath.Join(root, filepath.FromSlash(dir))
		info, err := os.Lstat(target)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				findings = append(findings, Finding{"directory_collision", dir, "Required canonical directory path is occupied by a non-directory.", false})
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			components := strings.Split(filepath.ToSlash(dir), "/")
			current := root
			collision := false
			for _, comp := range components {
				current = filepath.Join(current, comp)
				compInfo, compErr := os.Lstat(current)
				if compErr == nil {
					if compInfo.Mode()&os.ModeSymlink != 0 || !compInfo.IsDir() {
						findings = append(findings, Finding{"directory_collision", dir, "Required canonical directory path is occupied by a non-directory.", false})
						collision = true
						break
					}
				} else if errors.Is(compErr, os.ErrNotExist) {
					break
				}
			}
			if !collision {
				return Plan{}, fmt.Errorf("stat canonical directory %s: %w", dir, err)
			}
		}
	}
	ignore, err := readCanonicalFileBounded(filepath.Join(root, ".gitignore"), ".gitignore")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Plan{}, fmt.Errorf("read .gitignore: %w", err)
	}
	for _, entry := range requiredIgnoreEntries {
		if err != nil || !hasLine(string(ignore), entry) {
			findings = append(findings, Finding{"gitignore_entry_missing", ".gitignore", "Required Git ignore entry is missing: " + entry, true})
		}
	}
	return Plan{Root: root, Findings: findings}, nil
}

// hasCanonicalContent reports whether any canonical entity or skill file exists.
// Directories alone are structure, not content, and links count as content so a
// hostile layout is never treated as a fresh workspace.
func hasCanonicalContent(root string) bool {
	for _, directory := range requiredDirectories {
		found := errors.New("canonical content found")
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(directory)), func(_ string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if !entry.IsDir() {
				return found
			}
			return nil
		})
		if errors.Is(err, found) {
			return true
		}
	}
	return false
}

// Apply bootstraps a workspace directly. Existing-workspace application code
// uses PrepareLayout plus RemediationFiles and commits those files through the
// mutation WAL; this direct path is retained for first-time workspace creation.
func Apply(root string) (Plan, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return Plan{}, fmt.Errorf("make workspace path absolute: %w", err)
	}
	if _, statErr := os.Lstat(absoluteRoot); statErr == nil {
		existing, inspectErr := Inspect(absoluteRoot)
		if inspectErr != nil {
			return Plan{}, inspectErr
		}
		for _, finding := range existing.Findings {
			if !finding.Fixable {
				return existing, fmt.Errorf("%s: explicit canonical migration is required", finding.Summary)
			}
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Plan{}, statErr
	}
	plan, err := PrepareLayout(absoluteRoot)
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
		if parent := filepath.Dir(path); parent == path {
			break
		}
		// On macOS, system directories like /var, /tmp, /etc are symlinks to /private/...
		if runtime.GOOS == "darwin" && (path == "/var" || path == "/tmp" || path == "/etc") {
			continue
		}
		info, err := os.Lstat(path)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace path must not contain a symlink: %s", path)
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
	if gitRoot, err := gitRoot(ancestor); err == nil && !samePath(gitRoot, root) {
		return fmt.Errorf("refusing nested workspace inside Git repository %s; choose a directory outside it", gitRoot)
	}
	if isGitRepository(root) && looksLikeSourceCheckout(root) {
		return fmt.Errorf("refusing source checkout as workspace: %s", root)
	}
	return nil
}

func samePath(a, b string) bool {
	ca := filepath.Clean(filepath.FromSlash(strings.TrimSpace(a)))
	cb := filepath.Clean(filepath.FromSlash(strings.TrimSpace(b)))
	if realA, err := filepath.EvalSymlinks(ca); err == nil {
		ca = filepath.Clean(realA)
	}
	if realB, err := filepath.EvalSymlinks(cb); err == nil {
		cb = filepath.Clean(realB)
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
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
	contents, err := readCanonicalFileBounded(path, relativePath)
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

func readCanonicalFileBounded(path, relative string) ([]byte, error) {
	directory, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	return readBoundedFromRoot(directory, filepath.Base(path), relative)
}

// ReadCanonicalFile reads one workspace-relative regular file through an os.Root
// so links cannot escape the workspace. Symlinks, non-regular files, and files
// above the V1 per-file limit are rejected before any content is read.
func ReadCanonicalFile(root, relative string) ([]byte, error) {
	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	return readBoundedFromRoot(handle, filepath.FromSlash(relative), relative)
}

func readBoundedFromRoot(root *os.Root, name, relative string) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("canonical input %s is not a regular file", relative)
	}
	if info.Size() > MaxCanonicalFileBytesV1 {
		return nil, fmt.Errorf("canonical file %s exceeds V1 per-file limit of %d bytes", relative, MaxCanonicalFileBytesV1)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || opened.Size() > MaxCanonicalFileBytesV1 {
		return nil, fmt.Errorf("canonical file %s exceeds V1 per-file limit of %d bytes", relative, MaxCanonicalFileBytesV1)
	}
	contents, err := io.ReadAll(io.LimitReader(file, MaxCanonicalFileBytesV1+1))
	if err != nil {
		return nil, err
	}
	if int64(len(contents)) > MaxCanonicalFileBytesV1 {
		return nil, fmt.Errorf("canonical file %s exceeds V1 per-file limit of %d bytes", relative, MaxCanonicalFileBytesV1)
	}
	return contents, nil
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
// It enumerates directories in small batches and enforces the V1 count and byte ceilings before callers
// allocate file contents.
func RelativeFiles(root string) ([]string, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if rootInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("unsafe workspace root symlink: %s", root)
	}
	if !rootInfo.IsDir() {
		return nil, fmt.Errorf("workspace root is not a directory: %s", root)
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open workspace root: %w", err)
	}
	defer rootHandle.Close()

	paths := make([]string, 0)
	if err := collectCanonicalFiles(rootHandle, ".", &paths); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var total int64
	for _, relative := range paths {
		info, err := rootHandle.Lstat(filepath.FromSlash(relative))
		if err != nil {
			return nil, fmt.Errorf("inspect canonical file %s: %w", relative, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("unsafe symlink: %s", relative)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("unsupported non-regular canonical path: %s", relative)
		}
		if info.Size() > MaxCanonicalFileBytesV1 {
			return nil, fmt.Errorf("canonical file %s exceeds V1 per-file limit of %d bytes", relative, MaxCanonicalFileBytesV1)
		}
		if info.Size() > MaxCanonicalBytesV1-total {
			return nil, fmt.Errorf("canonical files exceed V1 aggregate limit of %d bytes at %s", MaxCanonicalBytesV1, relative)
		}
		total += info.Size()
	}
	return paths, nil
}

func collectCanonicalFiles(root *os.Root, directory string, paths *[]string) error {
	handle, err := root.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	for {
		entries, readErr := handle.ReadDir(64)
		for _, entry := range entries {
			native := filepath.Join(directory, entry.Name())
			relative := filepath.ToSlash(strings.TrimPrefix(native, "."+string(filepath.Separator)))
			if isNonCanonicalPath(relative) {
				continue
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				return fmt.Errorf("unsafe symlink: %s", relative)
			}
			if entry.IsDir() {
				if err := collectCanonicalFiles(root, native, paths); err != nil {
					return err
				}
				continue
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("unsupported non-regular canonical path: %s", relative)
			}
			if len(*paths) >= MaxCanonicalFilesV1 {
				return fmt.Errorf("canonical file count exceeds V1 limit of %d files", MaxCanonicalFilesV1)
			}
			*paths = append(*paths, relative)
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read canonical directory %s: %w", directory, readErr)
		}
	}
}

// isNonCanonicalPath excludes derived state, repository metadata, and host
// integration surfaces. Host config, native-skill copies, and instruction
// files are operational projections; they must never affect canonical
// validation, catalog snapshots, or Git-backed mutation transactions.
func isNonCanonicalPath(relative string) bool {
	for _, directory := range []string{
		".git",
		"runtime",
		".skillhub/transactions",
		".agents",
		".claude",
		".codex",
		".gemini",
	} {
		if relative == directory || strings.HasPrefix(relative, directory+"/") {
			return true
		}
	}
	switch relative {
	case ".mcp.json", "AGENTS.md", "CLAUDE.md", "GEMINI.md":
		return true
	default:
		return false
	}
}
