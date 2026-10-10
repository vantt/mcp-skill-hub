package hostintegration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

const maxManagedHostFileBytes = int64(4 << 20)

func readManagedFile(path, workspace string) ([]byte, os.FileMode, bool, error) {
	if err := rejectSymlinkComponents(workspace, path); err != nil {
		return nil, 0, false, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0o644, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("inspect %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, 0, false, fmt.Errorf("%w: %s", ErrSymlink, path)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, fmt.Errorf("managed path is not a regular file: %s", path)
	}
	if info.Size() > maxManagedHostFileBytes {
		return nil, 0, false, managedFileLimitError(path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, false, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	content, err := readManagedBytes(file, path)
	if err != nil {
		return nil, 0, false, err
	}
	return content, info.Mode(), true, nil
}

func rejectSymlinkComponents(workspace, target string) error {
	workspace = filepath.Clean(workspace)
	target = filepath.Clean(target)
	if !filepath.IsAbs(workspace) || !filepath.IsAbs(target) {
		return fmt.Errorf("managed paths must be absolute")
	}
	relative, err := filepath.Rel(workspace, target)
	if err != nil || relative == ".." || (len(relative) > 3 && relative[:3] == ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("managed path escapes workspace: %s", target)
	}
	// Check the workspace and every existing descendant without following links.
	parts := splitAbsolute(workspace)
	current := string(filepath.Separator)
	if volume := filepath.VolumeName(workspace); volume != "" {
		current = volume + string(filepath.Separator)
	}
	for _, part := range parts {
		current = filepath.Join(current, part)
		if err := rejectExistingSymlink(current); err != nil {
			return err
		}
	}
	if relative == "." {
		return nil
	}
	for _, part := range splitRelative(relative) {
		current = filepath.Join(current, part)
		if err := rejectExistingSymlink(current); err != nil {
			return err
		}
	}
	return nil
}

func splitAbsolute(path string) []string {
	volume := filepath.VolumeName(path)
	trimmed := path[len(volume):]
	return splitRelative(trimmed)
}

func splitRelative(path string) []string {
	var result []string
	for path != "" && path != "." && path != string(filepath.Separator) {
		directory, base := filepath.Split(filepath.Clean(path))
		if base != "" && base != "." {
			result = append([]string{base}, result...)
		}
		path = filepath.Clean(directory)
		if path == "." || path == string(filepath.Separator) {
			break
		}
	}
	return result
}

func rejectExistingSymlink(path string) error {
	if runtime.GOOS == "darwin" && (path == "/var" || path == "/tmp" || path == "/etc") {
		return nil
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect path component %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s", ErrSymlink, path)
	}
	return nil
}

func apply(ctx context.Context, plan PlanResult, options ApplyOptions) (ApplyResult, error) {
	if !options.Confirmed {
		return ApplyResult{}, ErrConfirmationRequired
	}
	if !filepath.IsAbs(plan.Workspace) || !filepath.IsAbs(plan.Binary) {
		return ApplyResult{}, fmt.Errorf("plan workspace and binary paths must be absolute")
	}
	writeRoot := plan.Root
	if writeRoot == "" {
		writeRoot = plan.Workspace
	}
	if !filepath.IsAbs(writeRoot) {
		return ApplyResult{}, fmt.Errorf("plan root path must be absolute")
	}
	writeRoot = filepath.Clean(writeRoot)
	if err := rejectSymlinkComponents(writeRoot, writeRoot); err != nil {
		return ApplyResult{}, err
	}
	root, err := os.OpenRoot(writeRoot)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("open workspace root: %w", err)
	}
	defer func() { _ = root.Close() }()

	lastOrder := -1
	seen := make(map[string]struct{}, len(plan.Changes))
	type pendingWrite struct {
		change Change
		mode   os.FileMode
	}
	pending := make([]pendingWrite, 0, len(plan.Changes))
	for _, change := range plan.Changes {
		if err := ctx.Err(); err != nil {
			return ApplyResult{}, err
		}
		order := changeOrder(change.Kind)
		if order < 0 || order < lastOrder {
			return ApplyResult{}, fmt.Errorf("plan changes are not dependency ordered")
		}
		lastOrder = order
		if _, duplicate := seen[change.Path]; duplicate {
			return ApplyResult{}, fmt.Errorf("plan contains duplicate path %s", change.Path)
		}
		seen[change.Path] = struct{}{}
		if err := validateChangePath(writeRoot, plan.Scope, change); err != nil {
			return ApplyResult{}, err
		}
		raw, mode, exists, err := readManagedFileAtRoot(root, writeRoot, change.Path)
		if err != nil {
			return ApplyResult{}, err
		}
		if (bytes.Equal(raw, change.Desired) && exists) || (nativeRemoval(change) && !exists) {
			continue
		}
		if digest(raw, exists) != change.PreimageDigest {
			return ApplyResult{}, fmt.Errorf("%w: %s", ErrStalePlan, change.Path)
		}
		expected, err := expectedDesired(change, raw, plan)
		if err != nil {
			return ApplyResult{}, err
		}
		if !bytes.Equal(expected, change.Desired) {
			return ApplyResult{}, fmt.Errorf("plan desired content is invalid for %s", change.Path)
		}
		if !exists {
			mode = os.FileMode(change.Mode)
			if mode.Perm() == 0 {
				mode = 0o644
			}
		}
		pending = append(pending, pendingWrite{change: change, mode: mode.Perm()})
	}

	result := ApplyResult{}
	for _, write := range pending {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		raw, _, exists, err := readManagedFileAtRoot(root, writeRoot, write.change.Path)
		if err != nil {
			return result, err
		}
		if (bytes.Equal(raw, write.change.Desired) && exists) || (nativeRemoval(write.change) && !exists) {
			continue
		}
		if digest(raw, exists) != write.change.PreimageDigest {
			return result, fmt.Errorf("%w: %s", ErrStalePlan, write.change.Path)
		}
		if nativeRemoval(write.change) {
			relative, err := managedRelativePath(writeRoot, write.change.Path)
			if err != nil {
				return result, err
			}
			if err := rejectRootSymlinkComponents(root, relative); err != nil {
				return result, err
			}
			if err := root.Remove(relative); err != nil {
				return result, err
			}
		} else if err := atomicWrite(root, writeRoot, write.change.Path, write.change.Desired, write.mode); err != nil {
			return result, err
		}
		result.Changed = append(result.Changed, write.change)
	}
	return result, nil
}

func expectedDesired(change Change, raw []byte, plan PlanResult) ([]byte, error) {
	switch change.Kind {
	case ChangeNativeSkill:
		if HostSupportsSkillsExtension(change.Host) {
			if !nativeCuratorOwned(raw) {
				return nil, &ConflictError{Path: change.Path, Reason: nativeCuratorConflict}
			}
			return nil, nil
		}
		return []byte(systemCuratorInstructions()), nil
	case ChangeBootstrap:
		desired, conflict := updateBootstrap(raw)
		if conflict != "" {
			return nil, &ConflictError{Path: change.Path, Reason: conflict}
		}
		return desired, nil
	case ChangeMCP:
		supportsToggle := HostSupportsServerToggle(change.Host)
		switch change.Host {
		case HostClaude:
			return desiredClaudeCLICurationConfig(raw, plan.Binary, plan.Workspace)
		case HostGemini:
			return desiredGeminiConfig(raw, plan.Binary, plan.Workspace, supportsToggle)
		case HostCodex:
			return desiredCodexConfig(raw, plan.Binary, plan.Workspace, supportsToggle)
		}
	case ChangeHostPermissions:
		if change.Host == HostClaude {
			file, err := preparePermissions(adapters[0], plan.Scope, change.Path, plan.Root, plan.Workspace)
			return file.desired, err
		}
	}
	return nil, fmt.Errorf("unsupported change %q for host %q", change.Kind, change.Host)
}

func changeOrder(kind ChangeKind) int {
	switch kind {
	case ChangeMCP:
		return 1
	case ChangeHostPermissions:
		return 2
	case ChangeNativeSkill:
		return 3
	case ChangeBootstrap:
		return 4
	default:
		return -1
	}
}

func validateChangePath(root string, scope Scope, change Change) error {
	var adapter *Adapter
	for index := range adapters {
		if adapters[index].Host == change.Host {
			adapter = &adapters[index]
			break
		}
	}
	if adapter == nil {
		return fmt.Errorf("unsupported host %q", change.Host)
	}
	var relative string
	configRel, skillRel, instructionRel := adapter.relativePaths(scope)
	switch change.Kind {
	case ChangeMCP:
		relative = configRel
	case ChangeHostPermissions:
		relative = adapter.permissionsPath(scope)
		if relative == "" {
			return fmt.Errorf("host %q has no separate permissions file", change.Host)
		}
	case ChangeNativeSkill:
		if !adapter.NativeSkill {
			return fmt.Errorf("host %q has no native skill path", change.Host)
		}
		relative = skillRel
	case ChangeBootstrap:
		relative = instructionRel
	default:
		return fmt.Errorf("unsupported change kind %q", change.Kind)
	}
	expected := filepath.Join(root, filepath.FromSlash(relative))
	if filepath.Clean(change.Path) != expected {
		return fmt.Errorf("change path does not match host contract: %s", change.Path)
	}
	return rejectSymlinkComponents(root, change.Path)
}

// atomicWriteBeforeRename is a deterministic test seam for exercising a
// parent replacement at the final commit boundary.
var atomicWriteBeforeRename = func(string) error { return nil }

func readManagedFileAtRoot(root *os.Root, workspace, path string) ([]byte, os.FileMode, bool, error) {
	relative, err := managedRelativePath(workspace, path)
	if err != nil {
		return nil, 0, false, err
	}
	if err := rejectRootSymlinkComponents(root, relative); err != nil {
		return nil, 0, false, err
	}
	info, err := root.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0o644, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("inspect %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, 0, false, fmt.Errorf("%w: %s", ErrSymlink, path)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, fmt.Errorf("managed path is not a regular file: %s", path)
	}
	if info.Size() > maxManagedHostFileBytes {
		return nil, 0, false, managedFileLimitError(path)
	}
	file, err := root.Open(relative)
	if err != nil {
		return nil, 0, false, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	content, err := readManagedBytes(file, path)
	if err != nil {
		return nil, 0, false, err
	}
	return content, info.Mode(), true, nil
}

func readManagedBytes(file *os.File, path string) ([]byte, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened managed file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("managed path is not a regular file: %s", path)
	}
	if info.Size() > maxManagedHostFileBytes {
		return nil, managedFileLimitError(path)
	}
	content, err := io.ReadAll(io.LimitReader(file, maxManagedHostFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if int64(len(content)) > maxManagedHostFileBytes {
		return nil, managedFileLimitError(path)
	}
	return content, nil
}

func managedFileLimitError(path string) error {
	return fmt.Errorf("managed host file %s exceeds read limit of %d bytes", path, maxManagedHostFileBytes)
}

func atomicWrite(root *os.Root, workspace, path string, content []byte, mode os.FileMode) error {
	relative, err := managedRelativePath(workspace, path)
	if err != nil {
		return err
	}
	directory := filepath.Dir(relative)
	if err := ensureSafeDirectoryAtRoot(root, directory); err != nil {
		return err
	}
	if err := rejectRootSymlinkComponents(root, relative); err != nil {
		return err
	}

	temporaryName, temporary, err := createRootTemp(root, directory, mode.Perm())
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	cleanup := func() {
		_ = temporary.Close()
		_ = root.Remove(temporaryName)
	}
	// OpenFile applies the process umask; restore the preimage permission bits
	// on the already-open temporary file before it becomes visible.
	if err := temporary.Chmod(mode.Perm()); err != nil {
		cleanup()
		return fmt.Errorf("set temporary mode for %s: %w", path, err)
	}
	if _, err := temporary.Write(content); err != nil {
		cleanup()
		return fmt.Errorf("write temporary file for %s: %w", path, err)
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temporary file for %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		_ = root.Remove(temporaryName)
		return fmt.Errorf("close temporary file for %s: %w", path, err)
	}
	if err := atomicWriteBeforeRename(relative); err != nil {
		_ = root.Remove(temporaryName)
		return fmt.Errorf("before atomic replace of %s: %w", path, err)
	}
	if err := root.Rename(temporaryName, relative); err != nil {
		_ = root.Remove(temporaryName)
		return fmt.Errorf("replace %s atomically: %w", path, err)
	}
	if err := syncRootDirectory(root, directory); err != nil {
		return fmt.Errorf("sync directory for %s: %w", path, err)
	}
	return nil
}

func createRootTemp(root *os.Root, directory string, mode os.FileMode) (string, *os.File, error) {
	for attempts := 0; attempts < 100; attempts++ {
		var random [12]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, err
		}
		name := filepath.Join(directory, ".skillhub-"+hex.EncodeToString(random[:]))
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return name, file, err
	}
	return "", nil, fmt.Errorf("temporary filename collision limit reached")
}

func managedRelativePath(workspace, path string) (string, error) {
	relative, err := filepath.Rel(workspace, path)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || stringsHasDotDotPrefix(relative) {
		return "", fmt.Errorf("managed path escapes workspace: %s", path)
	}
	return relative, nil
}

func stringsHasDotDotPrefix(path string) bool {
	return len(path) > 3 && path[:3] == ".."+string(filepath.Separator)
}

func ensureSafeDirectoryAtRoot(root *os.Root, directory string) error {
	if directory == "." {
		return nil
	}
	current := ""
	for _, part := range splitRelative(directory) {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := root.Mkdir(current, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("create managed directory %s: %w", current, err)
			}
			info, err = root.Lstat(current)
		}
		if err != nil {
			return fmt.Errorf("inspect managed directory %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrSymlink, current)
		}
		if !info.IsDir() {
			return fmt.Errorf("managed directory component is not a directory: %s", current)
		}
	}
	return nil
}

func rejectRootSymlinkComponents(root *os.Root, relative string) error {
	current := ""
	for _, part := range splitRelative(relative) {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect path component %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrSymlink, current)
		}
	}
	return nil
}

func syncRootDirectory(root *os.Root, directory string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	handle, err := root.Open(directory)
	if err != nil {
		return err
	}
	defer func() { _ = handle.Close() }()
	if err := handle.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) {
		return err
	}
	return nil
}
