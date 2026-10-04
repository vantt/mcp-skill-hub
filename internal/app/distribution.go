package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

const (
	MaxDistributedSkillResources = catalog.MaxDistributedSkillResources
	MaxDistributedSkillBytes     = catalog.MaxDistributedSkillBytes
)

// DistributedResource is the public, digest-pinned view of one skill file.
type DistributedResource struct {
	URI    string `json:"uri"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// DistributedSkill is a complete static SEP-2640 skill entry.
type DistributedSkill struct {
	URI           string                `json:"uri"`
	Frontmatter   map[string]any        `json:"frontmatter"`
	Resources     []DistributedResource `json:"resources"`
	Version       string                `json:"-"`
	Snapshot      string                `json:"-"`
	SkillID       string                `json:"-"`
	resourcePaths map[string]string
	// digests lists every distributed file by relative path and digest, taken
	// from catalog rows, so content trust never needs the file bytes.
	digests []skillruntime.ResourceDigest
}

// DistributedContent is a verified resource payload.
type DistributedContent struct {
	URI      string
	MIMEType string
	Bytes    []byte
	SkillID  string
	Path     string // relative to the skill folder
}

// IsText reports whether the payload may be sent as JSON text without changing
// its bytes: the type must promise text and the bytes must be valid UTF-8.
func (content DistributedContent) IsText() bool {
	return isTextMIMEType(content.MIMEType) && utf8.Valid(content.Bytes)
}

// DistributionService maps the immutable catalog/resource application model to
// standards-compatible skill entries. It never recommends or activates skills.
type DistributionService struct{}

// SkippedSkill identifies an active skill that could not be served, with the reason.
type SkippedSkill struct {
	SkillID string
	Reason  string
}

// ListSkills returns every servable active skill. Skills that cannot be served
// are omitted; use ListSkillsReport to learn which were skipped and why.
func (service DistributionService) ListSkills(ctx context.Context, path string) ([]DistributedSkill, string, error) {
	entries, snapshot, _, err := service.ListSkillsReport(ctx, path)
	return entries, snapshot, err
}

// ListSkillsReport isolates per-skill failures: one unservable skill is reported
// in skipped instead of failing the listing for every other skill.
func (DistributionService) ListSkillsReport(ctx context.Context, path string) (entries []DistributedSkill, snapshot string, skipped []SkippedSkill, resultErr error) {
	root, handle, err := openDistributionGeneration(ctx, path)
	if err != nil {
		return nil, "", nil, err
	}
	defer closeDistributionHandle(handle, &resultErr)
	rows, err := handle.DB.QueryContext(ctx, `SELECT id FROM skills WHERE status='active' ORDER BY id`)
	if err != nil {
		return nil, "", nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, "", nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, "", nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, "", nil, err
	}
	curator, err := buildBundledCuratorSkill()
	if err != nil {
		return nil, "", nil, err
	}
	entries = append(entries, curator)
	for _, id := range ids {
		if id == systemskills.CuratorSkillID {
			continue
		}
		entry, err := buildDistributedSkill(ctx, root, handle, id)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, "", nil, ctxErr
			}
			skipped = append(skipped, SkippedSkill{SkillID: id, Reason: err.Error()})
			continue
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].SkillID < entries[j].SkillID })
	return entries, handle.Pointer.CatalogSnapshot, skipped, nil
}

// LookupSkills resolves only the requested IDs while one immutable catalog
// generation is pinned. Invalid unrelated skills are never parsed or loaded.
func (DistributionService) LookupSkills(ctx context.Context, path string, ids []string) (entries map[string]DistributedSkill, snapshot string, resultErr error) {
	root, handle, err := openDistributionGeneration(ctx, path)
	if err != nil {
		return nil, "", err
	}
	defer closeDistributionHandle(handle, &resultErr)
	entries = make(map[string]DistributedSkill, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		if id == systemskills.CuratorSkillID {
			entry, err := buildBundledCuratorSkill()
			if err != nil {
				return nil, "", err
			}
			entries[id] = entry
			continue
		}
		entry, err := buildDistributedSkill(ctx, root, handle, id)
		if err != nil {
			return nil, "", err
		}
		entries[id] = entry
	}
	return entries, handle.Pointer.CatalogSnapshot, nil
}

func (DistributionService) GetSkill(ctx context.Context, path, uri string) (entry DistributedSkill, resultErr error) {
	_, manifestDigest, id, relative, err := parseDistributedURI(uri)
	if err != nil || relative != "SKILL.md" {
		return entry, skill.ErrSnapshotExpired
	}
	root, handle, err := openDistributionGeneration(ctx, path)
	if err != nil {
		return entry, err
	}
	defer closeDistributionHandle(handle, &resultErr)
	if id == systemskills.CuratorSkillID {
		entry, err = buildBundledCuratorSkill()
		if err != nil {
			return DistributedSkill{}, err
		}
	} else {
		entry, err = buildDistributedSkill(ctx, root, handle, id)
		if err != nil {
			return DistributedSkill{}, err
		}
	}
	if entry.Version != "sha256:"+manifestDigest || entry.URI != uri {
		return DistributedSkill{}, skill.ErrSnapshotExpired
	}
	return entry, nil
}

func (DistributionService) ReadResource(ctx context.Context, path, uri string) (content DistributedContent, resultErr error) {
	_, manifestDigest, id, relative, err := parseDistributedURI(uri)
	if err != nil {
		return content, skill.ErrSnapshotExpired
	}
	root, handle, err := openDistributionGeneration(ctx, path)
	if err != nil {
		return content, err
	}
	defer closeDistributionHandle(handle, &resultErr)
	var entry DistributedSkill
	if id == systemskills.CuratorSkillID {
		entry, err = buildBundledCuratorSkill()
		if err != nil {
			return content, err
		}
	} else {
		entry, err = buildDistributedSkill(ctx, root, handle, id)
		if err != nil {
			return content, err
		}
	}
	if entry.Version != "sha256:"+manifestDigest {
		return content, skill.ErrSnapshotExpired
	}
	var selected *DistributedResource
	for index := range entry.Resources {
		if entry.Resources[index].URI == uri {
			selected = &entry.Resources[index]
			break
		}
	}
	if selected == nil {
		return content, skill.ErrSnapshotExpired
	}
	var contents []byte
	if id == systemskills.CuratorSkillID {
		bundle := systemskills.CuratorBundle()
		if selected.Digest != bundle.Digest {
			return content, skill.ErrResourceDigestMismatch
		}
		contents = []byte(bundle.Instructions)
	} else {
		internal := entry.resourcePaths[uri]
		if internal == "" {
			return content, skill.ErrSnapshotExpired
		}
		contents, err = readPinnedResource(root, internal, selected.Digest)
		if err != nil {
			return content, err
		}
	}
	if int64(len(contents)) != selected.Size {
		return content, skill.ErrResourceDigestMismatch
	}
	mimeType := resourceMIMEType(relative)
	if isTextMIMEType(mimeType) && !utf8.Valid(contents) {
		// Bytes that are not UTF-8 cannot travel as text without being altered.
		mimeType = "application/octet-stream"
	}
	return DistributedContent{URI: uri, MIMEType: mimeType, Bytes: contents, SkillID: id, Path: relative}, nil
}

func openDistributionGeneration(ctx context.Context, path string) (string, *catalog.Handle, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return "", nil, err
	}
	handle, err := catalog.OpenWithFallbackLocked(ctx, root)
	if err != nil {
		return "", nil, err
	}
	return root, handle, nil
}

func closeDistributionHandle(handle *catalog.Handle, resultErr *error) {
	if err := handle.Close(); *resultErr == nil && err != nil {
		*resultErr = fmt.Errorf("close catalog generation: %w", err)
	}
}

func buildDistributedSkill(ctx context.Context, root string, handle *catalog.Handle, id string) (DistributedSkill, error) {
	if id == "" || strings.ContainsAny(id, `/\\`) {
		return DistributedSkill{}, skill.ErrNotFound
	}
	manifest := skill.Manifest{SkillID: id, CatalogSnapshot: handle.Pointer.CatalogSnapshot, Resources: []skill.Resource{}}
	if err := handle.DB.QueryRowContext(ctx, `SELECT name,description,status FROM skills WHERE id=?`, id).Scan(&manifest.Name, &manifest.Description, &manifest.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DistributedSkill{}, skill.ErrNotFound
		}
		return DistributedSkill{}, err
	}
	if manifest.Status != "active" {
		return DistributedSkill{}, skill.ErrNotFound
	}
	rows, err := handle.DB.QueryContext(ctx, `SELECT path,kind,digest,size_bytes FROM resources WHERE skill_id=? ORDER BY path`, id)
	if err != nil {
		return DistributedSkill{}, err
	}
	for rows.Next() {
		var resource skill.Resource
		if err := rows.Scan(&resource.Path, &resource.Kind, &resource.Digest, &resource.SizeBytes); err != nil {
			_ = rows.Close()
			return DistributedSkill{}, err
		}
		manifest.Resources = append(manifest.Resources, resource)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return DistributedSkill{}, err
	}
	if err := rows.Close(); err != nil {
		return DistributedSkill{}, err
	}
	digestInput := make([]manifestDigestEntry, 0, len(manifest.Resources))
	var total int64
	var entrypoint *skill.Resource
	for index := range manifest.Resources {
		resource := &manifest.Resources[index]
		relative, ok := distributedRelativePath(resource.Path, id)
		if !ok {
			continue
		}
		if relative == "SKILL.md" {
			entrypoint = resource
		}
		total += resource.SizeBytes
		digestInput = append(digestInput, manifestDigestEntry{Path: relative, Digest: resource.Digest, Size: resource.SizeBytes})
	}
	if entrypoint == nil {
		if len(digestInput) == 0 || len(digestInput) > MaxDistributedSkillResources {
			return DistributedSkill{}, fmt.Errorf("%w: skill %s has %d distributed resources; supported range is 1..%d", catalog.ErrSkillNotServable, id, len(digestInput), MaxDistributedSkillResources)
		}
		return DistributedSkill{}, fmt.Errorf("%w: %w: active skill has no top-level SKILL.md", catalog.ErrSkillNotServable, skill.ErrSnapshotExpired)
	}
	contents, err := readPinnedResource(root, entrypoint.Path, entrypoint.Digest)
	if err != nil {
		return DistributedSkill{}, fmt.Errorf("%w: %w", catalog.ErrSkillNotServable, err)
	}
	if err := catalog.ValidateServableSkill(id, contents, len(digestInput), total); err != nil {
		return DistributedSkill{}, err
	}
	frontmatter, err := catalog.ParseSkillFrontmatter(contents)
	if err != nil {
		return DistributedSkill{}, fmt.Errorf("%w: skill %s is not distributable: %v", catalog.ErrSkillNotServable, id, err)
	}

	// Verify all other distributed resources of this skill match their pinned digests
	for _, res := range digestInput {
		if res.Path == "SKILL.md" {
			continue
		}
		for _, internal := range manifest.Resources {
			if rel, ok := relativeSkillPath(internal.Path, id); ok && rel == res.Path {
				if _, err := readPinnedResource(root, internal.Path, internal.Digest); err != nil {
					return DistributedSkill{}, fmt.Errorf("%w: %w", catalog.ErrSkillNotServable, err)
				}
				break
			}
		}
	}

	sort.Slice(digestInput, func(i, j int) bool { return digestInput[i].Path < digestInput[j].Path })
	hexDigest, err := hashDistributedManifest(digestInput)
	if err != nil {
		return DistributedSkill{}, err
	}
	base := "skill://skillhub/" + hexDigest + "/" + id + "/"
	resources := make([]DistributedResource, 0, len(digestInput))
	resourcePaths := make(map[string]string, len(digestInput))
	digests := make([]skillruntime.ResourceDigest, 0, len(digestInput))
	for _, resource := range digestInput {
		digests = append(digests, skillruntime.ResourceDigest{Path: resource.Path, Digest: resource.Digest})
		uri := base + escapeURIPath(resource.Path)
		resources = append(resources, DistributedResource{URI: uri, Digest: resource.Digest, Size: resource.Size})
		for _, internal := range manifest.Resources {
			if relative, ok := relativeSkillPath(internal.Path, id); ok && relative == resource.Path {
				resourcePaths[uri] = internal.Path
				break
			}
		}
	}
	return DistributedSkill{URI: base + "SKILL.md", Frontmatter: frontmatter, Resources: resources, Version: "sha256:" + hexDigest, Snapshot: manifest.CatalogSnapshot, SkillID: id, resourcePaths: resourcePaths, digests: digests}, nil
}

// manifestDigestEntry is one file in the distributed manifest digest input.
type manifestDigestEntry struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// hashDistributedManifest returns the lowercase hex manifest digest over
// entries already sorted by path.
func hashDistributedManifest(entries []manifestDigestEntry) (string, error) {
	encoded, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// distributedRelativePath maps a catalog resource path to its path inside the
// distributed skill folder. skill.meta.yaml is hub metadata and never
// distributed.
func distributedRelativePath(internal, id string) (string, bool) {
	relative, ok := relativeSkillPath(internal, id)
	if !ok || relative == "skill.meta.yaml" {
		return "", false
	}
	return relative, true
}

// distributedSkillFile is one digest-verified file of a distributed skill.
type distributedSkillFile struct {
	Path   string // relative to the skill folder, slash separated
	Kind   string
	Digest string
	Size   int64
	Bytes  []byte
}

// distributedSkillSource is a distributed skill together with its manifest
// document (content_json) and the verified bytes of every distributed file,
// all read from one pinned catalog generation.
type distributedSkillSource struct {
	Skill       DistributedSkill
	ContentJSON []byte
	Files       []distributedSkillFile
}

// loadDistributedSkillSource builds an active, servable skill and reads every
// distributed file, verifying each against its pinned digest and size. The
// catalog handle is released before returning, so callers may write files
// without holding the shared catalog lock. When admit returns false, the file
// bytes are not read and Files stays empty; admit sees the skill entry and its
// manifest document, which is enough to decide trust.
func loadDistributedSkillSource(ctx context.Context, path, id string, admit func(DistributedSkill, []byte) (bool, error)) (root string, source distributedSkillSource, resultErr error) {
	root, handle, err := openDistributionGeneration(ctx, path)
	if err != nil {
		return "", source, err
	}
	defer closeDistributionHandle(handle, &resultErr)
	entry, err := buildDistributedSkill(ctx, root, handle, id)
	if err != nil {
		return "", source, err
	}
	var contentJSON string
	if err := handle.DB.QueryRowContext(ctx, `SELECT content_json FROM canonical_entities WHERE id=?`, id).Scan(&contentJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", source, skill.ErrNotFound
		}
		return "", source, err
	}
	if admit != nil {
		proceed, err := admit(entry, []byte(contentJSON))
		if err != nil {
			return "", source, err
		}
		if !proceed {
			return root, distributedSkillSource{Skill: entry, ContentJSON: []byte(contentJSON)}, nil
		}
	}
	kinds := make(map[string]string, len(entry.Resources))
	rows, err := handle.DB.QueryContext(ctx, `SELECT path,kind FROM resources WHERE skill_id=?`, id)
	if err != nil {
		return "", source, err
	}
	for rows.Next() {
		var internal, kind string
		if err := rows.Scan(&internal, &kind); err != nil {
			_ = rows.Close()
			return "", source, err
		}
		kinds[internal] = kind
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", source, err
	}
	if err := rows.Close(); err != nil {
		return "", source, err
	}
	files := make([]distributedSkillFile, 0, len(entry.Resources))
	for _, resource := range entry.Resources {
		internal := entry.resourcePaths[resource.URI]
		relative, ok := distributedRelativePath(internal, id)
		if internal == "" || !ok {
			return "", source, fmt.Errorf("%w: resource %s has no verified workspace path", skill.ErrSnapshotExpired, resource.URI)
		}
		contents, err := readPinnedResource(root, internal, resource.Digest)
		if err != nil {
			return "", source, err
		}
		if int64(len(contents)) != resource.Size {
			return "", source, fmt.Errorf("%w: resource %s size changed", ErrResourceContentUnavailable, relative)
		}
		files = append(files, distributedSkillFile{Path: relative, Kind: kinds[internal], Digest: resource.Digest, Size: resource.Size, Bytes: contents})
	}
	return root, distributedSkillSource{Skill: entry, ContentJSON: []byte(contentJSON), Files: files}, nil
}

func buildBundledCuratorSkill() (DistributedSkill, error) {
	bundle := systemskills.CuratorBundle()
	metadata := bundle.Metadata
	if len(bundle.Instructions) == 0 || len(bundle.Instructions) > MaxDistributedSkillBytes {
		return DistributedSkill{}, fmt.Errorf("bundled skill %s has unsupported size %d", metadata.SkillID, len(bundle.Instructions))
	}
	frontmatter, err := catalog.ParseSkillFrontmatter([]byte(bundle.Instructions))
	if err != nil {
		return DistributedSkill{}, fmt.Errorf("bundled skill %s is not distributable: %w", metadata.SkillID, err)
	}
	frontmatter["name"] = metadata.SkillID
	frontmatter["version"] = metadata.SkillVersion
	frontmatter["contract-version"] = metadata.ContractVersion
	frontmatter["activation-policy"] = metadata.ActivationPolicy
	frontmatter["coordination-boundary"] = metadata.CoordinationBoundary
	frontmatter["compatible-tools"] = append([]string(nil), metadata.CompatibleTools...)
	frontmatter["instruction-only"] = metadata.InstructionOnly
	frontmatter["best-effort-coordination"] = metadata.BestEffortCoordination
	frontmatter["requires-application-service"] = metadata.RequiresApplicationService

	resource := struct {
		Path   string `json:"path"`
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	}{Path: "SKILL.md", Digest: bundle.Digest, Size: int64(len(bundle.Instructions))}
	encoded, err := json.Marshal([]any{resource})
	if err != nil {
		return DistributedSkill{}, fmt.Errorf("encode bundled skill %s manifest: %w", metadata.SkillID, err)
	}
	sum := sha256.Sum256(encoded)
	manifestDigest := hex.EncodeToString(sum[:])
	uri := "skill://skillhub/" + manifestDigest + "/" + metadata.SkillID + "/SKILL.md"
	return DistributedSkill{
		URI:         uri,
		Frontmatter: frontmatter,
		Resources: []DistributedResource{{
			URI: uri, Digest: bundle.Digest, Size: resource.Size,
		}},
		Version:       "sha256:" + manifestDigest,
		Snapshot:      bundle.Digest,
		SkillID:       metadata.SkillID,
		resourcePaths: map[string]string{},
	}, nil
}

// ErrResourceContentUnavailable indicates that a required skill resource has changed or is missing on disk.
var ErrResourceContentUnavailable = errors.New("resource_content_unavailable")

func readPinnedResource(root, relative, expectedDigest string) ([]byte, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("%w: open root: %v", ErrResourceContentUnavailable, err)
	}
	defer rootHandle.Close()
	info, err := rootHandle.Lstat(relative)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: resource %s is missing or not regular: %w", ErrResourceContentUnavailable, relative, skill.ErrSnapshotExpired)
	}
	contents, err := rootHandle.ReadFile(relative)
	if err != nil {
		return nil, fmt.Errorf("%w: read resource %s: %v", ErrResourceContentUnavailable, relative, err)
	}
	sum := sha256.Sum256(contents)
	actualDigest := "sha256:" + hex.EncodeToString(sum[:])
	if actualDigest != expectedDigest {
		return nil, fmt.Errorf("%w: resource %s digest mismatch (expected %s, got %s): %w", ErrResourceContentUnavailable, relative, expectedDigest, actualDigest, skill.ErrResourceDigestMismatch)
	}
	return contents, nil
}

func parseDistributedURI(raw string) (*url.URL, string, string, string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "skill" || parsed.Host != "skillhub" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, "", "", "", errors.New("invalid skill resource URI")
	}
	parts := strings.Split(strings.TrimPrefix(parsed.EscapedPath(), "/"), "/")
	if len(parts) < 3 || len(parts[0]) != 64 || !isLowerHex(parts[0]) {
		return nil, "", "", "", errors.New("invalid skill resource URI")
	}
	id, err := url.PathUnescape(parts[1])
	if err != nil || id == "" || strings.Contains(id, "/") {
		return nil, "", "", "", errors.New("invalid skill resource URI")
	}
	relParts := make([]string, 0, len(parts)-2)
	for _, part := range parts[2:] {
		decoded, decodeErr := url.PathUnescape(part)
		if decodeErr != nil || decoded == "" || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/\\") {
			return nil, "", "", "", errors.New("invalid skill resource URI")
		}
		relParts = append(relParts, decoded)
	}
	return parsed, parts[0], id, strings.Join(relParts, "/"), nil
}

func relativeSkillPath(internal, id string) (string, bool) {
	parts := strings.Split(internal, "/")
	if len(parts) < 4 || parts[0] != "skills" || parts[2] != id {
		return "", false
	}
	return strings.Join(parts[3:], "/"), true
}

func escapeURIPath(path string) string {
	parts := strings.Split(path, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func isLowerHex(value string) bool {
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

// resourceMIMETypes is fixed so a resource has the same type on every host;
// the operating system's MIME database must never influence delivered content.
var resourceMIMETypes = map[string]string{
	".md": "text/markdown", ".markdown": "text/markdown", ".txt": "text/plain; charset=utf-8",
	".csv": "text/csv; charset=utf-8", ".tsv": "text/tab-separated-values; charset=utf-8",
	".html": "text/html; charset=utf-8", ".htm": "text/html; charset=utf-8", ".css": "text/css; charset=utf-8",
	".js": "text/javascript; charset=utf-8", ".py": "text/x-python; charset=utf-8", ".sh": "text/x-shellscript; charset=utf-8",
	".json": "application/json", ".yaml": "application/yaml", ".yml": "application/yaml",
	".toml": "application/toml", ".xml": "application/xml", ".pdf": "application/pdf",
	".svg": "image/svg+xml", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp",
}

func resourceMIMEType(path string) string {
	if filepath.Base(path) == "SKILL.md" {
		return "text/markdown"
	}
	if value, ok := resourceMIMETypes[strings.ToLower(filepath.Ext(path))]; ok {
		return value
	}
	return "application/octet-stream"
}

// isTextMIMEType reports whether the type promises UTF-8 text content.
func isTextMIMEType(mimeType string) bool {
	return strings.HasPrefix(mimeType, "text/") || mimeType == "application/json" || strings.Contains(mimeType, "yaml")
}
