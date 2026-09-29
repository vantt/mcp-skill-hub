package mutation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
)

func digestAt(root, relative string) (string, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer rootHandle.Close()
	info, err := rootHandle.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("unsafe mutation target: %s", relative)
	}
	data, err := rootHandle.ReadFile(relative)
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

// writeFileSync uses a sibling temporary file, fsyncs it, atomically replaces
// the target, and fsyncs the parent directory.
func writeFileSync(root, relative string, contents []byte) error {
	if err := ensureSafeParents(root, filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative))), 0o700); err != nil {
		return err
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	if info, err := os.Lstat(target); err == nil && (info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return fmt.Errorf("unsafe mutation target: %s", relative)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(target)
	tmp, err := os.CreateTemp(parent, ".skillhub-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err = tmp.Write(contents); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := atomicReplace(name, target); err != nil {
		return err
	}
	return syncDir(parent)
}

// replaceCanonicalChecked publishes without overwriting an unobserved target.
// Existing targets are first moved into the transaction, then the exact moved
// bytes are revalidated. A mismatch is restored when the canonical name is
// still free; otherwise the displaced file remains recoverable in the WAL.
func replaceCanonicalChecked(root string, item change, contents []byte, options Options) error {
	parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(item.Path)))
	if item.BeforeDigest == "" {
		if err := ensureSafeParents(root, parent, 0o755); err != nil {
			return err
		}
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rootHandle.Close()
	if err := verifyExistingParents(rootHandle, parent); err != nil {
		return fmt.Errorf("unsafe canonical parent for %s: %w", item.Path, err)
	}

	mode := fs.FileMode(0o644) // documented default for newly-created canonical files
	displaced := false
	if item.BeforeDigest != "" {
		if item.Displaced == "" {
			return fmt.Errorf("%w: missing displaced-target path for %s", ErrRecoveryRequired, item.Path)
		}
		if displacedDigest, digestErr := digestAt(root, item.Displaced); digestErr != nil {
			return digestErr
		} else if displacedDigest == item.BeforeDigest {
			displaced = true // recovery after a crash between displacement and publish
			info, statErr := rootHandle.Lstat(item.Displaced)
			if statErr != nil {
				return statErr
			}
			mode = info.Mode().Perm()
		} else if displacedDigest != "" {
			return fmt.Errorf("%w: displaced target digest mismatch for %s", ErrRecoveryRequired, item.Path)
		}
		if !displaced {
			if err := options.inject(FaultBeforeTargetDisplace); err != nil {
				return err
			}
			if err := verifyExistingParents(rootHandle, parent); err != nil {
				return fmt.Errorf("unsafe canonical parent for %s: %w", item.Path, err)
			}
			if err := ensureSafeParents(root, filepath.ToSlash(filepath.Dir(filepath.FromSlash(item.Displaced))), 0o700); err != nil {
				return err
			}
			if err := rootHandle.Rename(item.Path, item.Displaced); err != nil {
				return fmt.Errorf("displace canonical target %s: %w", item.Path, err)
			}
			displaced = true
			if err := options.inject(FaultAfterTargetDisplace); err != nil {
				return err
			}
			info, statErr := rootHandle.Lstat(item.Displaced)
			if statErr != nil {
				return statErr
			}
			mode = info.Mode().Perm()
		}
		exact, err := rootHandle.ReadFile(item.Displaced)
		if err != nil {
			return err
		}
		if digest(exact) != item.BeforeDigest {
			restoreErr := restoreDisplaced(rootHandle, item)
			if restoreErr != nil {
				return fmt.Errorf("%w: path %s changed during displacement; restore failed: %v", ErrConflict, item.Path, restoreErr)
			}
			return fmt.Errorf("%w: path %s changed during displacement and was restored", ErrConflict, item.Path)
		}
	} else {
		if err := options.inject(FaultBeforeTargetDisplace); err != nil {
			return err
		}
		if err := verifyExistingParents(rootHandle, parent); err != nil {
			return fmt.Errorf("unsafe canonical parent for %s: %w", item.Path, err)
		}
	}
	if item.Delete {
		return syncRootDir(rootHandle, parent)
	}

	temporary, err := createCanonicalTemp(rootHandle, parent, contents, mode)
	if err != nil {
		if displaced {
			_ = restoreDisplaced(rootHandle, item)
		}
		return err
	}
	defer rootHandle.Remove(temporary)
	if err := verifyExistingParents(rootHandle, parent); err != nil {
		if displaced {
			_ = restoreDisplaced(rootHandle, item)
		}
		return fmt.Errorf("unsafe canonical parent for %s: %w", item.Path, err)
	}
	// Link is the portable Go primitive that atomically creates a name only if
	// it is absent. Unlike rename-with-replace, it cannot silently overwrite a
	// late external edit. Transaction and target are required to share a volume.
	if err := rootHandle.Link(temporary, item.Path); err != nil {
		if displaced {
			_ = restoreDisplaced(rootHandle, item)
		}
		return fmt.Errorf("publish canonical target %s without replacement: %w", item.Path, err)
	}
	if err := rootHandle.Remove(temporary); err != nil {
		return err
	}
	return syncRootDir(rootHandle, parent)
}

func rollbackCanonicalChecked(root, txn string, item change) error {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rootHandle.Close()
	parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(item.Path)))
	if err := verifyExistingParents(rootHandle, parent); err != nil {
		return err
	}
	txnRelative, err := filepath.Rel(root, txn)
	if err != nil {
		return err
	}
	rollbackPath := filepath.ToSlash(filepath.Join(txnRelative, "rollback", filepath.FromSlash(item.Path)))
	movedCurrent := false
	if item.AfterDigest != "" {
		if err := ensureSafeParents(root, filepath.ToSlash(filepath.Dir(filepath.FromSlash(rollbackPath))), 0o700); err != nil {
			return err
		}
		if err := rootHandle.Rename(item.Path, rollbackPath); err != nil {
			return err
		}
		movedCurrent = true
		current, readErr := rootHandle.ReadFile(rollbackPath)
		if readErr != nil || digest(current) != item.AfterDigest {
			restoreErr := linkAndRemove(rootHandle, rollbackPath, item.Path)
			if readErr != nil {
				return fmt.Errorf("read rollback displacement for %s: %w (restore: %v)", item.Path, readErr, restoreErr)
			}
			return fmt.Errorf("%w: path %s changed during rollback displacement (restore: %v)", ErrRecoveryRequired, item.Path, restoreErr)
		}
	}
	if item.BeforeDigest == "" {
		return syncRootDir(rootHandle, parent)
	}
	before, err := rootHandle.ReadFile(item.Displaced)
	if err != nil || digest(before) != item.BeforeDigest {
		if movedCurrent {
			_ = linkAndRemove(rootHandle, rollbackPath, item.Path)
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("%w: displaced before-image mismatch for %s", ErrRecoveryRequired, item.Path)
	}
	if err := rootHandle.Link(item.Displaced, item.Path); err != nil {
		if movedCurrent {
			_ = linkAndRemove(rootHandle, rollbackPath, item.Path)
		}
		return err
	}
	return syncRootDir(rootHandle, parent)
}

func linkAndRemove(root *os.Root, source, target string) error {
	if err := root.Link(source, target); err != nil {
		return err
	}
	return root.Remove(source)
}

func createCanonicalTemp(root *os.Root, parent string, contents []byte, mode fs.FileMode) (string, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	name := ".skillhub-publish-" + hex.EncodeToString(random[:])
	relative := name
	if parent != "." && parent != "" {
		relative = parent + "/" + name
	}
	file, err := root.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if err := file.Chmod(mode); err != nil {
		file.Close()
		root.Remove(relative)
		return "", err
	}
	if _, err = file.Write(contents); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		root.Remove(relative)
		return "", err
	}
	return relative, nil
}

func restoreDisplaced(rootHandle *os.Root, item change) error {
	if _, err := rootHandle.Lstat(item.Path); err == nil {
		return errors.New("canonical name is occupied by a concurrent writer")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := rootHandle.Link(item.Displaced, item.Path); err != nil {
		return err
	}
	if err := rootHandle.Remove(item.Displaced); err != nil {
		return err
	}
	return syncRootDir(rootHandle, filepath.ToSlash(filepath.Dir(filepath.FromSlash(item.Path))))
}

func verifyExistingParents(root *os.Root, relative string) error {
	if relative == "." || relative == "" {
		return nil
	}
	current := ""
	for _, component := range strings.Split(filepath.ToSlash(relative), "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("invalid directory path: %s", relative)
		}
		if current == "" {
			current = component
		} else {
			current += "/" + component
		}
		info, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("path component is not a real directory: %s", current)
		}
	}
	return nil
}

func syncRootDir(root *os.Root, relative string) error {
	if relative == "" {
		relative = "."
	}
	directory, err := root.Open(relative)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil && !errors.Is(err, fs.ErrInvalid) {
		return err
	}
	return nil
}

func ensureSafeParents(root, relative string, mode fs.FileMode) error {
	if err := ensureAbsoluteDirectory(root, 0o700); err != nil {
		return err
	}
	if relative == "." || relative == "" {
		return nil
	}
	current := root
	for _, component := range strings.Split(filepath.ToSlash(relative), "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("invalid directory path: %s", relative)
		}
		current = filepath.Join(current, component)
		if err := ensureDirectory(current, mode); err != nil {
			return err
		}
	}
	return nil
}

func ensureAbsoluteDirectory(path string, mode fs.FileMode) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(absolute)
	current := string(filepath.Separator)
	trimmed := strings.TrimPrefix(strings.TrimPrefix(absolute, volume), string(filepath.Separator))
	if volume != "" {
		current = volume + string(filepath.Separator)
	}
	for _, component := range strings.Split(trimmed, string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		if err := ensureDirectory(current, mode); err != nil {
			return err
		}
	}
	return nil
}

func ensureDirectory(path string, mode fs.FileMode) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, mode); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		if err := syncDir(filepath.Dir(path)); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("unsafe directory path: %s", path)
	}
	return nil
}

func syncDir(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		// Some filesystems do not support directory fsync. Only ignore the
		// explicit unsupported-operation family, never arbitrary I/O errors.
		if errors.Is(err, fs.ErrInvalid) {
			return nil
		}
		return err
	}
	return nil
}

func removeFile(root, relative string) error {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rootHandle.Close()
	info, err := rootHandle.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("unsafe mutation target: %s", relative)
	}
	if err := rootHandle.Remove(relative); err != nil {
		return err
	}
	return syncDir(filepath.Join(root, filepath.Dir(filepath.FromSlash(relative))))
}

func readStagedFile(txn, relative string) ([]byte, error) {
	if relative == "" || strings.Contains(relative, `\`) || strings.HasPrefix(relative, "/") {
		return nil, fmt.Errorf("unsafe staged path: %s", relative)
	}
	root, err := os.OpenRoot(txn)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(relative)
	if err != nil {
		return nil, err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("unsafe staged path: %s", relative)
	}
	return root.ReadFile(relative)
}

func validateVirtual(root string, changes []Change) error {
	_, err := validateVirtualSnapshot(root, changes)
	return err
}

func validateVirtualSnapshot(root string, changes []Change) (canonical.Snapshot, error) {
	parent := filepath.Dir(root)
	virtual, err := os.MkdirTemp(parent, ".skillhub-validate-")
	if err != nil {
		return canonical.Snapshot{}, fmt.Errorf("create virtual validation workspace: %w", err)
	}
	defer os.RemoveAll(virtual)
	if err := copyTree(root, virtual); err != nil {
		return canonical.Snapshot{}, err
	}
	for _, item := range changes {
		if item.Delete {
			if err := removeFile(virtual, item.Path); err != nil {
				return canonical.Snapshot{}, err
			}
		} else if err := writeFileSync(virtual, item.Path, item.Contents); err != nil {
			return canonical.Snapshot{}, err
		}
	}
	issues, err := canonical.Validate(virtual)
	if err != nil {
		return canonical.Snapshot{}, err
	}
	if len(issues) > 0 {
		return canonical.Snapshot{}, fmt.Errorf("proposed mutation is invalid: %s: %s", issues[0].Path, issues[0].Message)
	}
	return canonical.Scan(virtual)
}

func gitDirty(root string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := []string{
		"--no-optional-locks",
		"-c", "core.hooksPath=/dev/null",
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "core.worktree=" + root,
		"-c", "core.bare=false",
		"-c", "credential.helper=",
		"-c", "protocol.file.allow=never",
		"-c", "protocol.ext.allow=never",
		"-c", "submodule.recurse=false",
		"-C", root, "status", "--porcelain", "--untracked-files=all",
	}
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = safeOfflineGitEnvironment()
	output, err := command.Output()
	if err != nil {
		if ctx.Err() != nil {
			return false, fmt.Errorf("inspect Git dirty state: %w", ctx.Err())
		}
		return false, fmt.Errorf("inspect Git dirty state: %w", err)
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

func safeOfflineGitEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+10)
	for _, item := range os.Environ() {
		name := item
		if index := strings.IndexByte(item, '='); index >= 0 {
			name = item[:index]
		}
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "GIT_") || upper == "HOME" || upper == "XDG_CONFIG_HOME" {
			continue
		}
		environment = append(environment, item)
	}
	return append(environment,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PAGER=cat",
		"HOME=/dev/null",
		"XDG_CONFIG_HOME=/dev/null",
	)
}

func copyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		relativeSlash := filepath.ToSlash(relative)
		if relativeSlash == "runtime" || strings.HasPrefix(relativeSlash, "runtime/") || relativeSlash == ".skillhub/transactions" || strings.HasPrefix(relativeSlash, ".skillhub/transactions/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("virtual validation refused symlink: %s", relativeSlash)
		}
		if entry.IsDir() {
			return ensureSafeParents(target, relativeSlash, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return writeFileSync(target, relativeSlash, data)
	})
}
