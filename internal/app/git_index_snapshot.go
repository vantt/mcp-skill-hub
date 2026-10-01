package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// IndexEntry represents one stage-0 entry parsed from Git ls-files --stage -z.
type IndexEntry struct {
	Mode  string
	SHA   string
	Stage string
	Path  string
}

// ReadGitIndexIdentity returns the SHA-256 digest of the .git/index file.
func ReadGitIndexIdentity(ctx context.Context, root string) (string, error) {
	cmd := offlineGitCommand(ctx, root, "rev-parse", "--git-path", "index")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve git index path: %w", err)
	}
	indexPath := strings.TrimSpace(string(out))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(root, indexPath)
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "empty", nil
		}
		return "", fmt.Errorf("read git index: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// CaptureGitIndexSnapshot materializes the Git stage-0 index into a detached private directory.
// It pins index identity before and after, reads raw blobs via git cat-file without filters or checkout,
// rejects unmerged/symlink/special modes, and returns the temp dir, cleanup func, unmerged paths, and syntax issues.
func CaptureGitIndexSnapshot(ctx context.Context, root string) (string, func(), []string, []canonical.Issue, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, nil, nil, err
	}
	initialIdentity, err := ReadGitIndexIdentity(ctx, root)
	if err != nil {
		return "", nil, nil, nil, err
	}

	// 1. Enumerate NUL-delimited entries with git ls-files --stage -z
	cmd := offlineGitCommand(ctx, root, "ls-files", "--stage", "-z")
	output, err := cmd.Output()
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("enumerate git index entries: %w", err)
	}

	var entries []IndexEntry
	var unmergedPaths []string
	var issues []canonical.Issue

	records := bytes.Split(output, []byte{0})
	for _, rec := range records {
		if len(rec) == 0 {
			continue
		}
		tabIdx := bytes.IndexByte(rec, '\t')
		if tabIdx < 0 {
			continue
		}
		meta := string(rec[:tabIdx])
		relPath := filepath.ToSlash(string(rec[tabIdx+1:]))

		if !safeRelativePath(relPath) {
			return "", nil, nil, nil, fmt.Errorf("unsafe git index path: %s", relPath)
		}

		parts := strings.Fields(meta)
		if len(parts) < 3 {
			continue
		}
		mode, sha, stage := parts[0], parts[1], parts[2]

		if stage != "0" {
			unmergedPaths = append(unmergedPaths, relPath)
			continue
		}

		switch mode {
		case "120000":
			issues = append(issues, canonical.Issue{
				Path:    relPath,
				Message: "unsafe symlink in Git index",
				Fix:     "Replace symlink with a regular file or remove it from the Git index.",
			})
			continue
		case "160000":
			issues = append(issues, canonical.Issue{
				Path:    relPath,
				Message: "unsupported gitlink/submodule in Git index",
				Fix:     "Remove submodule from canonical workspace layout.",
			})
			continue
		case "100644", "100755":
			// valid regular file
		default:
			issues = append(issues, canonical.Issue{
				Path:    relPath,
				Message: fmt.Sprintf("unsupported non-regular mode %q in Git index", mode),
				Fix:     "Ensure canonical file has regular permissions (100644 or 100755).",
			})
			continue
		}

		entries = append(entries, IndexEntry{
			Mode:  mode,
			SHA:   sha,
			Stage: stage,
			Path:  relPath,
		})
	}

	// 2. Read blobs via git cat-file --batch
	blobContents := make(map[string][]byte, len(entries))
	if len(entries) > 0 {
		catCmd := offlineGitCommand(ctx, root, "cat-file", "--batch")
		stdin, err := catCmd.StdinPipe()
		if err != nil {
			return "", nil, nil, nil, fmt.Errorf("pipe to git cat-file: %w", err)
		}
		stdout, err := catCmd.StdoutPipe()
		if err != nil {
			_ = stdin.Close()
			return "", nil, nil, nil, fmt.Errorf("stdout from git cat-file: %w", err)
		}
		if err := catCmd.Start(); err != nil {
			_ = stdin.Close()
			return "", nil, nil, nil, fmt.Errorf("start git cat-file: %w", err)
		}

		go func() {
			for _, entry := range entries {
				_, _ = fmt.Fprintf(stdin, "%s\n", entry.SHA)
			}
			_ = stdin.Close()
		}()

		bufReader := bufio.NewReader(stdout)
		for _, entry := range entries {
			headerLine, err := bufReader.ReadString('\n')
			if err != nil {
				_ = catCmd.Wait()
				return "", nil, nil, nil, fmt.Errorf("read git cat-file header for %s: %w", entry.Path, err)
			}
			headerParts := strings.Fields(strings.TrimSpace(headerLine))
			if len(headerParts) < 3 || headerParts[1] != "blob" {
				_ = catCmd.Wait()
				return "", nil, nil, nil, fmt.Errorf("unexpected git cat-file header: %s", headerLine)
			}
			size, err := strconv.ParseInt(headerParts[2], 10, 64)
			if err != nil {
				_ = catCmd.Wait()
				return "", nil, nil, nil, fmt.Errorf("parse blob size: %w", err)
			}
			content := make([]byte, size)
			if _, err := io.ReadFull(bufReader, content); err != nil {
				_ = catCmd.Wait()
				return "", nil, nil, nil, fmt.Errorf("read blob content for %s: %w", entry.Path, err)
			}
			if _, err := bufReader.ReadByte(); err != nil {
				_ = catCmd.Wait()
				return "", nil, nil, nil, fmt.Errorf("read trailing newline: %w", err)
			}
			blobContents[entry.Path] = content
		}
		if err := catCmd.Wait(); err != nil {
			return "", nil, nil, nil, fmt.Errorf("git cat-file finished with error: %w", err)
		}
	}

	// 3. Materialize with private permissions (0o700 dirs, 0o600 files) in temp dir
	tempDir, err := os.MkdirTemp("", ".skillhub-validate-staged-*")
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("create temp snapshot directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	for _, entry := range entries {
		content := blobContents[entry.Path]
		targetPath := filepath.Join(tempDir, filepath.FromSlash(entry.Path))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
			cleanup()
			return "", nil, nil, nil, fmt.Errorf("create target dir for %s: %w", entry.Path, err)
		}
		if err := os.WriteFile(targetPath, content, 0o600); err != nil {
			cleanup()
			return "", nil, nil, nil, fmt.Errorf("write staged file %s: %w", entry.Path, err)
		}
	}

	// 4. Recheck index identity
	finalIdentity, err := ReadGitIndexIdentity(ctx, root)
	if err != nil {
		cleanup()
		return "", nil, nil, nil, err
	}
	if initialIdentity != finalIdentity {
		cleanup()
		return "", nil, nil, nil, &Error{
			Code: ErrorIndexStale,
			Render: ErrorRender{
				Error: "Git index changed concurrently during staged snapshot capture.",
				Why:   "Index file identity changed between initial capture and materialization.",
				Fix:   "Retry the operation after current staging operations complete.",
			},
		}
	}

	return tempDir, cleanup, unmergedPaths, issues, nil
}

// ValidateWorkspaceStaged validates the staged Git index without checkout conversion or filters.
func (WorkspaceService) ValidateWorkspaceStaged(ctx context.Context, path string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return Result{}, err
	}
	tempDir, cleanup, unmergedPaths, preIssues, err := CaptureGitIndexSnapshot(ctx, root)
	if err != nil {
		var appErr *Error
		if errors.As(err, &appErr) {
			return ErrorResult(appErr), nil
		}
		return Result{}, err
	}
	defer cleanup()

	canonicalIssues, err := canonical.ValidateDetached(tempDir, unmergedPaths)
	if err != nil {
		return Result{}, err
	}
	allIssues := append(preIssues, canonicalIssues...)
	return formatValidationResult(allIssues), nil
}

// ValidateStaged is an alias for ValidateWorkspaceStaged.
func (w WorkspaceService) ValidateStaged(ctx context.Context, path string) (Result, error) {
	return w.ValidateWorkspaceStaged(ctx, path)
}
