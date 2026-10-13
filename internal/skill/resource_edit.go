package skill

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/vantt/skillhub-mcp/internal/mutation"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// MaxEditableResourceBytes caps both the editable size of a companion file and
// the number of bytes returned when a larger text file is viewed.
const MaxEditableResourceBytes = 1 << 20

// Reasons a companion file is not editable. An empty reason means editable.
const (
	ResourceReasonBinary     = "binary"
	ResourceReasonTooLarge   = "too_large"
	ResourceReasonMissing    = "missing"
	ResourceReasonNotRegular = "not_regular"
)

// Line ending styles reported for a text companion file.
const (
	LineEndingLF    = "lf"
	LineEndingCRLF  = "crlf"
	LineEndingMixed = "mixed"
	LineEndingNone  = "none"
)

// ErrInvalidResourceEdit marks a request that can never succeed as sent: an
// unsafe or excluded path, a file that is not editable, or unusable content.
var ErrInvalidResourceEdit = errors.New("invalid resource edit")

var sha256DigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ResourceFile describes the working-tree state of one companion file of a
// skill. Content is set only by ReadResourceFile and only for text files.
type ResourceFile struct {
	Path       string
	Kind       string
	SizeBytes  int64
	Digest     string
	Editable   bool
	Reason     string
	LineEnding string
	Truncated  bool
	Content    []byte
	IsText     bool
}

// ResourceUpdateInput replaces the whole content of one companion file.
// ExpectedDigest pins the bytes the editor started from.
type ResourceUpdateInput struct {
	Path           string
	Content        []byte
	ExpectedDigest string
	IdempotencyKey string
}

// ListResourceFiles classifies every companion file in the skill's catalog
// manifest. SKILL.md and hub metadata are excluded; content is not returned.
func ListResourceFiles(ctx context.Context, root, id string) ([]ResourceFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := resourceSkillDir(root, id)
	if err != nil {
		return nil, err
	}
	manifest, err := GetManifestAnyState(ctx, root, id)
	if err != nil {
		return nil, err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	files := make([]ResourceFile, 0, len(manifest.Resources))
	for _, row := range manifest.Resources {
		if validateResourcePath(dir, row.Path) != nil {
			continue
		}
		file, err := classifyResource(handle, dir, row)
		if err != nil {
			return nil, err
		}
		file.Content = nil
		files = append(files, file)
	}
	return files, nil
}

// ReadResourceFile classifies one companion file and returns its content when
// it is text: the first MaxEditableResourceBytes, cut back to a whole UTF-8
// rune, with Truncated set when the file is larger.
func ReadResourceFile(ctx context.Context, root, id, path string) (ResourceFile, error) {
	if err := ctx.Err(); err != nil {
		return ResourceFile{}, err
	}
	dir, row, err := resolveResource(ctx, root, id, path)
	if err != nil {
		return ResourceFile{}, err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return ResourceFile{}, err
	}
	defer handle.Close()
	return classifyResource(handle, dir, row)
}

// PreviewResourceUpdate plans a one-file skill_edit write set for a companion
// file. The skill metadata is not rewritten, so the lifecycle state and review
// flags stay as they are.
func (manager Manager) PreviewResourceUpdate(ctx context.Context, root, id string, input ResourceUpdateInput, fullDiff bool) (Proposal, error) {
	if err := ctx.Err(); err != nil {
		return Proposal{}, err
	}
	const command = "skill_edit"
	requestDigest := normalizedDigest(struct{ ID, Path, Content, ExpectedDigest string }{id, input.Path, string(input.Content), input.ExpectedDigest})
	idempotencyKey := lifecycleIdempotencyKey(input.IdempotencyKey, command, id, requestDigest)
	if prior, found, err := mutation.LookupOperation(root, mutation.WriteSet{Command: command, IdempotencyKey: idempotencyKey, RequestDigest: requestDigest}); err != nil {
		return Proposal{}, err
	} else if found {
		return manager.appliedProposal(root, id, command, prior, requestDigest, idempotencyKey), nil
	}
	if !sha256DigestPattern.MatchString(input.ExpectedDigest) {
		return Proposal{}, fmt.Errorf("%w: expected digest must be sha256: followed by 64 lowercase hex characters", ErrInvalidResourceEdit)
	}
	dir, row, err := resolveResource(ctx, root, id, input.Path)
	if err != nil {
		return Proposal{}, err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return Proposal{}, err
	}
	current, err := classifyResource(handle, dir, row)
	handle.Close()
	if err != nil {
		return Proposal{}, err
	}
	if !current.Editable {
		return Proposal{}, fmt.Errorf("%w: resource %s is not editable (%s)", ErrInvalidResourceEdit, input.Path, current.Reason)
	}
	if current.Digest != input.ExpectedDigest {
		return Proposal{}, &EditConflictError{Path: input.Path, ExpectedDigest: input.ExpectedDigest, ActualDigest: current.Digest}
	}
	contents := input.Content
	if current.LineEnding == LineEndingCRLF {
		contents = bytes.ReplaceAll(contents, []byte("\r\n"), []byte("\n"))
		contents = bytes.ReplaceAll(contents, []byte("\n"), []byte("\r\n"))
	}
	if err := validateResourceContent(contents); err != nil {
		return Proposal{}, err
	}
	if bytes.Equal(contents, current.Content) {
		return Proposal{}, fmt.Errorf("%w: no change to save", ErrInvalidResourceEdit)
	}
	changes := []mutation.Change{{Path: input.Path, BeforeDigest: input.ExpectedDigest, Contents: contents}}
	return manager.plan(ctx, root, id, command, changes, nil, nil, fullDiff, idempotencyKey, requestDigest)
}

// resourceSkillDir returns the workspace-relative folder of a skill, found
// through the strict metadata lookup that refuses symlinked folders.
func resourceSkillDir(root, id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("%w: skill id must be a lowercase kebab-case identifier", ErrInvalidResourceEdit)
	}
	metadataPath, _, _, err := loadSkill(root, id)
	if err != nil {
		return "", err
	}
	// Resource paths use forward slashes on every platform; the folder must too, or on Windows no path
	// would ever start with it.
	return filepath.ToSlash(skillDirFromMetadataPath(metadataPath)), nil
}

// resolveResource validates a requested path against the skill folder and
// returns the matching manifest row. A path the manifest does not list for
// this skill is ErrNotFound.
func resolveResource(ctx context.Context, root, id, path string) (string, Resource, error) {
	dir, err := resourceSkillDir(root, id)
	if err != nil {
		return "", Resource{}, err
	}
	if err := validateResourcePath(dir, path); err != nil {
		return "", Resource{}, err
	}
	manifest, err := GetManifestAnyState(ctx, root, id)
	if err != nil {
		return "", Resource{}, err
	}
	for _, row := range manifest.Resources {
		if row.Path == path {
			return dir, row, nil
		}
	}
	return "", Resource{}, fmt.Errorf("%w: %s is not a resource of skill %s", ErrNotFound, path, id)
}

// validateResourcePath enforces the syntactic rules: a slash-separated path
// strictly inside the skill folder, without traversal, and not SKILL.md or hub
// metadata. Errors name the rule and never include file content.
func validateResourcePath(dir, path string) error {
	switch {
	case path == "":
		return fmt.Errorf("%w: resource path is required", ErrInvalidResourceEdit)
	case strings.ContainsRune(path, 0):
		return fmt.Errorf("%w: resource path must not contain a NUL byte", ErrInvalidResourceEdit)
	case strings.Contains(path, `\`):
		return fmt.Errorf("%w: resource path must use forward slashes", ErrInvalidResourceEdit)
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("%w: resource path must not contain empty, '.' or '..' segments", ErrInvalidResourceEdit)
		}
	}
	if !strings.HasPrefix(path, dir+"/") {
		return fmt.Errorf("%w: resource path must be inside the skill folder %s", ErrInvalidResourceEdit, dir)
	}
	if path == dir+"/SKILL.md" {
		return fmt.Errorf("%w: SKILL.md is edited through the skill editor", ErrInvalidResourceEdit)
	}
	if workspace.IsHubMeta(path) {
		return fmt.Errorf("%w: hub metadata cannot be edited as a resource", ErrInvalidResourceEdit)
	}
	return nil
}

func validateResourceContent(contents []byte) error {
	switch {
	case len(contents) > MaxEditableResourceBytes:
		return fmt.Errorf("%w: content exceeds the %d byte limit", ErrInvalidResourceEdit, MaxEditableResourceBytes)
	case bytes.IndexByte(contents, 0) >= 0:
		return fmt.Errorf("%w: content must not contain NUL bytes", ErrInvalidResourceEdit)
	case !utf8.Valid(contents):
		return fmt.Errorf("%w: content must be valid UTF-8", ErrInvalidResourceEdit)
	}
	return nil
}

// classifyResource inspects the working-tree file for a manifest row. Every
// path segment below the skill folder is checked with Lstat so a symlinked
// parent directory or file is reported as not_regular instead of followed.
// The returned Content holds at most MaxEditableResourceBytes of a text file.
func classifyResource(handle *os.Root, dir string, row Resource) (ResourceFile, error) {
	file := ResourceFile{Path: row.Path, Kind: row.Kind, LineEnding: LineEndingNone}
	current := dir
	segments := strings.Split(strings.TrimPrefix(row.Path, dir+"/"), "/")
	for index, segment := range segments {
		current += "/" + segment
		info, err := handle.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			file.Reason = ResourceReasonMissing
			return file, nil
		}
		if err != nil {
			return ResourceFile{}, err
		}
		last := index == len(segments)-1
		if info.Mode()&os.ModeSymlink != 0 || (!last && !info.IsDir()) || (last && !info.Mode().IsRegular()) {
			file.Reason = ResourceReasonNotRegular
			return file, nil
		}
	}
	lstat, err := handle.Lstat(row.Path)
	if err != nil {
		return ResourceFile{}, err
	}
	opened, err := handle.Open(row.Path)
	if err != nil {
		return ResourceFile{}, err
	}
	defer opened.Close()
	stat, err := opened.Stat()
	if err != nil {
		return ResourceFile{}, err
	}
	if !os.SameFile(lstat, stat) || !stat.Mode().IsRegular() {
		file.Reason = ResourceReasonNotRegular
		return file, nil
	}
	hasher := sha256.New()
	head, err := io.ReadAll(io.LimitReader(io.TeeReader(opened, hasher), MaxEditableResourceBytes))
	if err != nil {
		return ResourceFile{}, err
	}
	rest, err := io.Copy(hasher, opened)
	if err != nil {
		return ResourceFile{}, err
	}
	file.SizeBytes = int64(len(head)) + rest
	file.Digest = "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	file.Truncated = rest > 0
	text := head
	if file.Truncated {
		text = trimPartialRune(head)
	}
	if bytes.IndexByte(text, 0) >= 0 || !utf8.Valid(text) {
		file.Reason = ResourceReasonBinary
		return file, nil
	}
	file.IsText = true
	file.Content = text
	file.LineEnding = lineEndingOf(text)
	if file.Truncated {
		file.Reason = ResourceReasonTooLarge
		return file, nil
	}
	file.Editable = true
	return file, nil
}

// trimPartialRune drops an incomplete UTF-8 sequence cut off at the end.
func trimPartialRune(contents []byte) []byte {
	for index := len(contents) - 1; index >= 0 && index >= len(contents)-utf8.UTFMax; index-- {
		if utf8.RuneStart(contents[index]) {
			if !utf8.FullRune(contents[index:]) {
				return contents[:index]
			}
			return contents
		}
	}
	return contents
}

func lineEndingOf(contents []byte) string {
	total := bytes.Count(contents, []byte("\n"))
	if total == 0 {
		return LineEndingNone
	}
	crlf := bytes.Count(contents, []byte("\r\n"))
	switch crlf {
	case 0:
		return LineEndingLF
	case total:
		return LineEndingCRLF
	default:
		return LineEndingMixed
	}
}
