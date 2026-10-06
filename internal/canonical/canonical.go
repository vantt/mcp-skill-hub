// Package canonical validates and deterministically scans workspace canonical files.
package canonical

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	distillpkg "github.com/vantt/mcp-skill-hub/internal/distill"
	insightpkg "github.com/vantt/mcp-skill-hub/internal/insight"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

// FileDigest identifies one canonical file using a repository-relative slash path.
type FileDigest struct {
	Path   string
	Digest string
}

// Snapshot contains deterministic digests for all canonical inputs and routing inputs.
type Snapshot struct {
	Files                 []FileDigest
	CatalogSnapshot       string
	ProjectionInputDigest string
}

// Issue is a precise canonical validation failure.
type Issue struct {
	Path    string
	Line    int
	Message string
	Fix     string
}

// ValidationOptions configures canonical validation for working trees or detached snapshots.
type ValidationOptions struct {
	Detached      bool
	UnmergedPaths []string
}

// Validate checks workspace structure, canonical files, identity, references, and conflict markers.
func Validate(root string) ([]Issue, error) {
	return ValidateWithOptions(root, ValidationOptions{})
}

// ValidateDetached checks a detached snapshot with caller-supplied index facts.
func ValidateDetached(root string, unmergedPaths []string) ([]Issue, error) {
	return ValidateWithOptions(root, ValidationOptions{
		Detached:      true,
		UnmergedPaths: unmergedPaths,
	})
}

// ValidateWithOptions checks workspace structure, canonical files, identity, references, and conflict markers.
func ValidateWithOptions(root string, opts ValidationOptions) ([]Issue, error) {
	plan, err := workspace.InspectWithOptions(root, opts.Detached)
	if err != nil {
		return nil, err
	}
	issues := make([]Issue, 0, len(plan.Findings))
	for _, finding := range plan.Findings {
		issues = append(issues, Issue{Path: finding.Path, Message: finding.Summary})
	}
	files, err := workspace.RelativeFiles(root)
	if err != nil {
		if os.IsNotExist(err) {
			return issues, nil
		}
		return append(issues, Issue{Path: ".", Message: err.Error()}), nil
	}
	if opts.Detached {
		for _, path := range opts.UnmergedPaths {
			issues = append(issues, Issue{Path: path, Message: "unresolved Git index conflict"})
		}
	} else {
		conflicts, err := workspace.UnmergedGitPaths(root)
		if err != nil {
			return nil, err
		}
		for _, path := range conflicts {
			issues = append(issues, Issue{Path: path, Message: "unresolved Git index conflict"})
		}
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open canonical workspace: %w", err)
	}
	defer rootHandle.Close()
	ids := make(map[string]string)
	refs := make([]reference, 0)
	var totalBytes int64
	for _, rel := range files {
		if !validCanonicalPath(rel) {
			issues = append(issues, Issue{Path: rel, Message: "path is outside the canonical workspace layout"})
			continue
		}
		contents, err := readCanonicalFile(rootHandle, rel)
		if err != nil {
			return nil, err
		}
		if int64(len(contents)) > workspace.MaxCanonicalBytesV1-totalBytes {
			return nil, fmt.Errorf("canonical files exceed V1 aggregate limit of %d bytes at %s", workspace.MaxCanonicalBytesV1, rel)
		}
		totalBytes += int64(len(contents))
		if HasConflictMarker(string(contents)) {
			issues = append(issues, Issue{Path: rel, Message: "unresolved Git conflict marker"})
		}
		if !entityPath(rel) {
			continue
		}
		parsedID, parsedRefs, shapeErr := parseYAMLIdentity(rel, contents)
		if shapeErr == nil && strings.HasPrefix(rel, "sources/catalog/") {
			shapeErr = validateSourcePolicy(contents)
		}
		if shapeErr == nil && strings.HasPrefix(rel, "distill/") {
			switch {
			case strings.Contains(rel, "/proposals/"):
				_, shapeErr = insightpkg.ParseApplicationProposal(contents)
			case strings.Contains(rel, "/incorporations/"):
				_, shapeErr = insightpkg.ParseIncorporation(contents)
			case strings.Contains(rel, "/outcomes/"):
				_, shapeErr = insightpkg.ParseOutcome(contents)
			default:
				shapeErr = distillpkg.ValidateCanonical(rel, contents)
			}
		}
		if shapeErr != nil {
			issues = append(issues, Issue{Path: rel, Message: "invalid YAML: " + shapeErr.Error()})
			continue
		}
		if entityPath(rel) && parsedID == "" {
			issues = append(issues, Issue{Path: rel, Message: "canonical entity is missing an ID"})
		}
		if parsedID != "" {
			if previous, exists := ids[parsedID]; exists {
				issues = append(issues, Issue{Path: rel, Message: fmt.Sprintf("duplicate ID %q (also in %s)", parsedID, previous)})
			} else {
				ids[parsedID] = rel
			}
		}
		refs = append(refs, parsedRefs...)
	}
	virtualMutation := strings.HasPrefix(filepath.Base(root), ".skillhub-validate-")
	for _, ref := range refs {
		if _, exists := ids[ref.ID]; !exists {
			// The mutation receipt is deliberately written last and is not part of
			// the pre-commit virtual tree. Post-apply validation still requires it.
			if virtualMutation && ref.Field == "operation_id" {
				continue
			}
			issues = append(issues, Issue{Path: ref.Path, Message: fmt.Sprintf("broken reference %s: %q does not exist", ref.Field, ref.ID)})
		}
	}
	issues = append(issues, validateSkills(root, files)...)
	if err := distillpkg.ValidateWorkspace(root); err != nil {
		issues = append(issues, Issue{Path: "distill", Message: err.Error()})
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Path == issues[j].Path {
			return issues[i].Message < issues[j].Message
		}
		return issues[i].Path < issues[j].Path
	})
	return issues, nil
}

type reference struct{ Path, Field, ID string }

// parseYAMLIdentity decodes one YAML document and validates the shared canonical shape.
func parseYAMLIdentity(path string, contents []byte) (string, []reference, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return "", nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return "", nil, fmt.Errorf("multiple YAML documents are not allowed")
		}
		return "", nil, err
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return "", nil, fmt.Errorf("expected one YAML document")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return "", nil, fmt.Errorf("document root must be a mapping")
	}
	refs := make([]reference, 0)
	id, err := validateYAMLMapping(path, root, true, &refs)
	if err != nil {
		return "", nil, err
	}
	return id, refs, nil
}

func validateYAMLMapping(path string, node *yaml.Node, root bool, refs *[]reference) (string, error) {
	if len(node.Content)%2 != 0 {
		return "", fmt.Errorf("mapping has an incomplete key/value pair")
	}
	keys := make(map[string]struct{}, len(node.Content)/2)
	var id string
	for index := 0; index < len(node.Content); index += 2 {
		key, value := node.Content[index], node.Content[index+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "" {
			return "", fmt.Errorf("mapping keys must be non-empty strings")
		}
		if _, duplicate := keys[key.Value]; duplicate {
			return "", fmt.Errorf("duplicate key %q", key.Value)
		}
		keys[key.Value] = struct{}{}
		if err := validateYAMLNode(path, key.Value, value, refs); err != nil {
			return "", err
		}
		if root && key.Value == "id" {
			if !stringScalar(value) || value.Value == "" {
				return "", fmt.Errorf("id must be a non-empty string")
			}
			id = value.Value
		}
	}
	return id, nil
}

func validateYAMLNode(path, field string, node *yaml.Node, refs *[]reference) error {
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("aliases are not allowed")
	}
	if strings.HasSuffix(field, "_ids") {
		if node.Kind != yaml.SequenceNode {
			return fmt.Errorf("%s must be a sequence of non-empty strings", field)
		}
		for _, item := range node.Content {
			if !stringScalar(item) || item.Value == "" {
				return fmt.Errorf("%s must be a sequence of non-empty strings", field)
			}
			*refs = append(*refs, reference{Path: path, Field: field, ID: item.Value})
		}
		return nil
	}
	if field != "id" && strings.HasSuffix(field, "_id") {
		if !stringScalar(node) || node.Value == "" {
			return fmt.Errorf("%s must be a non-empty string", field)
		}
		*refs = append(*refs, reference{Path: path, Field: field, ID: node.Value})
		return nil
	}
	switch node.Kind {
	case yaml.MappingNode:
		_, err := validateYAMLMapping(path, node, false, refs)
		return err
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if err := validateYAMLNode(path, field, item, refs); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		return nil
	default:
		return fmt.Errorf("unsupported YAML node for %s", field)
	}
	return nil
}

func stringScalar(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Tag == "!!str"
}

func validateSourcePolicy(contents []byte) error {
	var document map[string]any
	if err := yaml.Unmarshal(contents, &document); err != nil {
		return err
	}
	for _, field := range []string{"last_checked_at", "latency", "latency_ms", "retry_count", "next_check_at", "availability", "credentials", "token", "password"} {
		if _, exists := document[field]; exists {
			return fmt.Errorf("source field %s is operational or sensitive and cannot be canonical", field)
		}
	}
	adapter, _ := document["adapter"].(string)
	locator, mapped := document["locator"].(map[string]any)
	if !mapped {
		return nil
	} // Legacy scalar locators remain readable; managed writes always use mappings.
	for key, raw := range locator {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "credential") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "auth") {
			return fmt.Errorf("source locator contains forbidden credential field %s", key)
		}
		value, _ := raw.(string)
		if (key == "url" || key == "repository") && value != "" {
			if _, err := sourcepkg.ValidateRemoteURLWithOptions(value, sourcepkg.URLValidationOptions{AllowFile: adapter == "git"}); err != nil {
				return err
			}
		}
	}
	if adapter == "filesystem" {
		value, _ := locator["path"].(string)
		if value == "" || filepath.IsAbs(filepath.FromSlash(value)) || value == ".." || strings.HasPrefix(value, "../") {
			return fmt.Errorf("filesystem source path must be relative and contained")
		}
	}
	return nil
}

func validCanonicalPath(path string) bool {
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

func entityPath(path string) bool {
	if strings.HasPrefix(path, "skills/") {
		return strings.HasSuffix(path, "/skill.meta.yaml")
	}
	return (strings.HasPrefix(path, "sources/") || strings.HasPrefix(path, "distill/") || strings.HasPrefix(path, "history/operations/") || strings.HasPrefix(path, "registry/collections/") || strings.HasPrefix(path, "evals/routing/")) && (strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml"))
}

func HasConflictMarker(contents string) bool {
	for _, line := range strings.Split(contents, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "<<<<<<<") || strings.HasPrefix(line, "=======") || strings.HasPrefix(line, ">>>>>>>") {
			return true
		}
	}
	return false
}

func hasConflictMarker(contents string) bool {
	return HasConflictMarker(contents)
}

// Scan computes deterministic SHA-256 digests without reading runtime or Git metadata.
func Scan(root string) (Snapshot, error) { return scan(root, nil) }

// scan makes a bounded second inventory pass before publishing a snapshot. The hook is
// test-only plumbing for deterministic concurrent-modification regression coverage.
func scan(root string, beforeRecheck func()) (Snapshot, error) {
	first, err := inventory(root)
	if err != nil {
		return Snapshot{}, err
	}
	if beforeRecheck != nil {
		beforeRecheck()
	}
	second, err := inventory(root)
	if err != nil {
		return Snapshot{}, err
	}
	if !sameInventory(first, second) {
		return Snapshot{}, fmt.Errorf("canonical workspace changed during scan; retry")
	}
	catalog := make([]FileDigest, 0, len(first))
	for _, item := range first {
		if catalogAffecting(item.Path) {
			catalog = append(catalog, item)
		}
	}
	return Snapshot{Files: first, CatalogSnapshot: aggregate(catalog), ProjectionInputDigest: aggregate(first)}, nil
}

func inventory(root string) ([]FileDigest, error) {
	files, err := workspace.RelativeFiles(root)
	if err != nil {
		return nil, err
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open canonical workspace: %w", err)
	}
	defer rootHandle.Close()
	items := make([]FileDigest, 0, len(files))
	var totalBytes int64
	for _, rel := range files {
		contents, err := readCanonicalFile(rootHandle, rel)
		if err != nil {
			return nil, err
		}
		if int64(len(contents)) > workspace.MaxCanonicalBytesV1-totalBytes {
			return nil, fmt.Errorf("canonical files exceed V1 aggregate limit of %d bytes at %s", workspace.MaxCanonicalBytesV1, rel)
		}
		totalBytes += int64(len(contents))
		digest := sha256.Sum256(contents)
		items = append(items, FileDigest{Path: rel, Digest: "sha256:" + hex.EncodeToString(digest[:])})
	}
	return items, nil
}

func readCanonicalFile(root *os.Root, relative string) ([]byte, error) {
	info, err := root.Lstat(relative)
	if err != nil {
		return nil, fmt.Errorf("inspect canonical input %s: %w", relative, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("canonical input %s is not a regular file", relative)
	}
	if info.Size() > workspace.MaxCanonicalFileBytesV1 {
		return nil, fmt.Errorf("canonical file %s exceeds V1 per-file limit of %d bytes", relative, workspace.MaxCanonicalFileBytesV1)
	}
	file, err := root.Open(relative)
	if err != nil {
		return nil, fmt.Errorf("open canonical input %s: %w", relative, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened canonical input %s: %w", relative, err)
	}
	if !opened.Mode().IsRegular() || opened.Size() > workspace.MaxCanonicalFileBytesV1 {
		return nil, fmt.Errorf("canonical file %s exceeds V1 per-file limit of %d bytes", relative, workspace.MaxCanonicalFileBytesV1)
	}
	contents, err := io.ReadAll(io.LimitReader(file, workspace.MaxCanonicalFileBytesV1+1))
	if err != nil {
		return nil, fmt.Errorf("read canonical input %s: %w", relative, err)
	}
	if int64(len(contents)) > workspace.MaxCanonicalFileBytesV1 {
		return nil, fmt.Errorf("canonical file %s exceeds V1 per-file limit of %d bytes", relative, workspace.MaxCanonicalFileBytesV1)
	}
	return contents, nil
}

func sameInventory(first, second []FileDigest) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}

func catalogAffecting(path string) bool {
	return path == ".skillhub/schema-version" || strings.HasPrefix(path, "skills/") || strings.HasPrefix(path, "registry/") || strings.HasPrefix(path, "config/") || strings.HasPrefix(path, "evals/routing/")
}

func aggregate(files []FileDigest) string {
	hash := sha256.New()
	for _, file := range files {
		fmt.Fprintf(hash, "%s\x00%s\n", file.Path, file.Digest)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}
