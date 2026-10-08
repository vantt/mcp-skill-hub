package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxSkillResourceBytes = 16 << 20 // 16 MB limit per resource matching canonical
)

var commonLicenseFileNames = map[string]bool{
	"license":       true,
	"license.txt":   true,
	"license.md":    true,
	"license.rst":   true,
	"copying":       true,
	"copying.txt":   true,
	"copying.md":    true,
	"unlicense":     true,
	"unlicense.txt": true,
}

// ResourceReader abstracts reading raw bytes from a source, snapshot, or in-memory map.
type ResourceReader interface {
	Read(ctx context.Context, path string) ([]byte, error)
}

// MapResourceReader implements ResourceReader from an in-memory byte map.
type MapResourceReader map[string][]byte

// Read returns bytes from the map.
func (m MapResourceReader) Read(_ context.Context, path string) ([]byte, error) {
	if b, ok := m[path]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("resource not found: %s", path)
}

// AdapterResourceReader implements ResourceReader using a sourcepkg.Adapter.
type AdapterResourceReader struct {
	Adapter  sourcepkg.Adapter
	Source   sourcepkg.Source
	Revision sourcepkg.Revision
}

// Read reads from the adapter.
func (r AdapterResourceReader) Read(ctx context.Context, path string) ([]byte, error) {
	return r.Adapter.Read(ctx, r.Source, r.Revision, path)
}

// DiscoveredCompanion represents one companion resource within a discovered skill directory.
type DiscoveredCompanion struct {
	Path     string `json:"path"`      // Relative path within skill directory (e.g. "LICENSE.txt", "scripts/run.py")
	FullPath string `json:"full_path"` // Full path within the source or snapshot (e.g. "skills/pdf/LICENSE.txt")
	Bytes    int64  `json:"bytes"`     // Size in bytes
	Digest   string `json:"digest"`    // Content digest (sha256:<hex>)
}

// SkillLicenseInfo describes declared and detected license facts and warnings.
type SkillLicenseInfo struct {
	Declared      string `json:"declared,omitempty"`
	LicenseFile   string `json:"license_file,omitempty"`
	IsUnknown     bool   `json:"is_unknown"`
	IsProprietary bool   `json:"is_proprietary"`
	Warning       string `json:"warning,omitempty"`
}

// DiscoveredSkillItem represents a candidate skill found during inventory/discovery.
type DiscoveredSkillItem struct {
	Name            string                `json:"name"`
	TargetID        string                `json:"target_id"`
	Collection      string                `json:"collection"`
	Description     string                `json:"description"`
	SkillDir        string                `json:"skill_dir"`     // Directory relative to source root ("" if at root)
	SkillMDPath     string                `json:"skill_md_path"` // Path to SKILL.md in source
	SkillMDBytes    []byte                `json:"-"`
	Companions      []DiscoveredCompanion `json:"companions"`
	CompanionBytes  map[string][]byte     `json:"-"` // Map from companion.Path (relPath) to byte content
	TotalBytes      int64                 `json:"total_bytes"`
	FileCount       int                   `json:"file_count"`
	License         SkillLicenseInfo      `json:"license"`
	Warnings        []Warning             `json:"warnings,omitempty"`
	Transformations []string              `json:"transformations,omitempty"`
	Conflict        bool                  `json:"conflict"`
	SkipReason      string                `json:"skip_reason,omitempty"`
	Error           string                `json:"error,omitempty"`
}

// sanitizeSkillID normalizes a skill name into a valid canonical identifier.
func sanitizeSkillID(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash && b.Len() > 0 {
			b.WriteRune('-')
			lastDash = true
		}
	}
	res := strings.Trim(b.String(), "-")
	if len(res) > 63 {
		res = strings.Trim(res[:63], "-")
	}
	return res
}

// normalizeText standardizes line endings to LF and ensures a trailing newline.
func normalizeText(contents []byte) []byte {
	contents = bytes.ReplaceAll(contents, []byte("\r\n"), []byte("\n"))
	if len(contents) > 0 && !bytes.HasSuffix(contents, []byte("\n")) {
		contents = append(contents, '\n')
	}
	return contents
}

// splitSkillFrontmatter separates frontmatter header and body.
func splitSkillFrontmatter(contents []byte) (header, body []byte, ok bool) {
	h, b, has, err := parseAndValidateSkillFrontmatter(contents)
	if err != nil || !has {
		return nil, contents, false
	}
	return h, b, true
}

// parseAndValidateSkillFrontmatter validates and extracts frontmatter header and body.
// If frontmatter opens with `---` but is unclosed or invalid YAML, an error is returned.
func parseAndValidateSkillFrontmatter(contents []byte) (header, body []byte, hasFrontmatter bool, err error) {
	norm := normalizeText(contents)
	if !bytes.HasPrefix(norm, []byte("---\n")) {
		// Does not start with frontmatter opening delimiter
		if bytes.HasPrefix(norm, []byte("---")) {
			return nil, nil, false, errors.New("malformed SKILL.md: frontmatter opener must be followed by newline")
		}
		return nil, norm, false, nil
	}

	rest := norm[4:]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		// Check for closing delimiter at the very end of file
		if bytes.HasSuffix(rest, []byte("\n---")) || bytes.Equal(rest, []byte("---")) {
			end = len(rest) - 3
			if end > 0 && rest[end-1] == '\n' {
				end--
			}
			header = rest[:end]
			body = nil
		} else {
			return nil, nil, false, errors.New("malformed SKILL.md: unclosed frontmatter delimiter")
		}
	} else {
		header = rest[:end]
		body = rest[end+5:]
	}

	var node yaml.Node
	if err := yaml.Unmarshal(header, &node); err != nil {
		return nil, nil, false, fmt.Errorf("malformed SKILL.md: invalid frontmatter YAML: %w", err)
	}
	if len(node.Content) == 0 {
		// Empty frontmatter: treat as valid empty mapping
		return header, body, true, nil
	}
	if len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
		return nil, nil, false, errors.New("malformed SKILL.md: frontmatter must be a YAML mapping")
	}
	if bytes.HasPrefix(body, []byte("---\n")) || bytes.Equal(body, []byte("---")) || bytes.HasPrefix(body, []byte("---\r\n")) {
		return nil, nil, false, errors.New("malformed SKILL.md: duplicate frontmatter delimiter")
	}
	return header, body, true, nil
}

// parseSkillMDFrontmatter extracts name, description, and license from frontmatter.
func parseSkillMDFrontmatter(contents []byte) (name, description, license string) {
	header, _, has, err := parseAndValidateSkillFrontmatter(contents)
	if err != nil || !has {
		return "", "", ""
	}
	var value map[string]any
	if err := yaml.Unmarshal(header, &value); err != nil {
		return "", "", ""
	}
	if n, ok := value["name"].(string); ok {
		name = strings.TrimSpace(n)
	}
	if d, ok := value["description"].(string); ok {
		description = strings.TrimSpace(d)
	}
	if l, ok := value["license"].(string); ok {
		license = strings.TrimSpace(l)
	}
	return name, description, license
}

// ensureImportedSkillFrontmatter normalizes SKILL.md frontmatter with the target ID,
// preserving all existing fields, comments, and body text.
func ensureImportedSkillFrontmatter(contents []byte, id, description string) ([]byte, []string, error) {
	var transformations []string
	if bytes.Contains(contents, []byte("\r\n")) {
		transformations = append(transformations, "crlf_to_lf")
	}
	contents = normalizeText(contents)

	header, body, hasFrontmatter, err := parseAndValidateSkillFrontmatter(contents)
	if err != nil {
		return nil, nil, err
	}

	if hasFrontmatter {
		var doc yaml.Node
		if err := yaml.Unmarshal(header, &doc); err != nil {
			return nil, nil, fmt.Errorf("malformed SKILL.md: invalid frontmatter YAML: %w", err)
		}
		if len(doc.Content) == 0 {
			doc = yaml.Node{
				Kind: yaml.DocumentNode,
				Content: []*yaml.Node{
					{
						Kind: yaml.MappingNode,
						Tag:  "!!map",
					},
				},
			}
		}
		mapping := doc.Content[0]
		hasName := false
		hasDesc := false
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			key := mapping.Content[i].Value
			val := mapping.Content[i+1].Value
			if key == "name" {
				if val != id {
					mapping.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: id}
					transformations = append(transformations, "normalize_frontmatter_name")
				}
				hasName = true
			} else if key == "description" {
				if strings.TrimSpace(val) == "" && strings.TrimSpace(description) != "" {
					mapping.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: strings.TrimSpace(description)}
					transformations = append(transformations, "set_frontmatter_description")
				}
				hasDesc = true
			}
		}
		if !hasName {
			mapping.Content = append([]*yaml.Node{
				{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"},
				{Kind: yaml.ScalarNode, Tag: "!!str", Value: id},
			}, mapping.Content...)
			transformations = append(transformations, "add_frontmatter_name")
		}
		if !hasDesc && strings.TrimSpace(description) != "" {
			mapping.Content = append(mapping.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "description"},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: strings.TrimSpace(description)},
			)
			transformations = append(transformations, "set_frontmatter_description")
		}

		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		_ = enc.Encode(&doc)
		_ = enc.Close()

		res := normalizeText(append(append([]byte("---\n"), buf.Bytes()...), append([]byte("---\n\n"), body...)...))
		return res, transformations, nil
	}

	// No frontmatter: prepend frontmatter
	headerStr := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n", id, strings.TrimSpace(description))
	transformations = append(transformations, "add_frontmatter")
	return normalizeText(append([]byte(headerStr), contents...)), transformations, nil
}

// detectSkillLicense evaluates declared license and detected companion files.
func detectSkillLicense(declared string, companions []DiscoveredCompanion) SkillLicenseInfo {
	info := SkillLicenseInfo{
		Declared: strings.TrimSpace(declared),
	}

	for _, comp := range companions {
		base := strings.ToLower(filepath.Base(comp.Path))
		if commonLicenseFileNames[base] {
			info.LicenseFile = comp.Path
			break
		}
	}

	lowerDeclared := strings.ToLower(info.Declared)
	if strings.Contains(lowerDeclared, "proprietary") {
		info.IsProprietary = true
	}
	if info.Declared == "" || lowerDeclared == "unknown" {
		info.IsUnknown = true
	}

	switch {
	case info.IsProprietary:
		info.Warning = fmt.Sprintf("Declared license %q may have proprietary terms.", info.Declared)
	case info.IsUnknown && info.LicenseFile == "":
		info.Warning = "License is unknown; no license declared in SKILL.md or license file found."
	case info.IsUnknown && info.LicenseFile != "":
		info.Warning = fmt.Sprintf("License not declared in SKILL.md frontmatter, but license file %s is present.", info.LicenseFile)
	}

	return info
}

// isUnsafeCompanionPath checks if a companion relative path contains path escapes.
func isUnsafeCompanionPath(p string) bool {
	p = filepath.ToSlash(p)
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || strings.Contains(p, "..") {
		return true
	}
	if len(p) >= 2 && p[1] == ':' {
		return true
	}
	return false
}

// DiscoverSkillsFromResources discovers all skills and inventories their companion files
// from a list of resources. Folder-scoped discovery preserves every descendant regular file.
func DiscoverSkillsFromResources(ctx context.Context, reader ResourceReader, resources []sourcepkg.Resource, scopePrefix string) ([]DiscoveredSkillItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	scopePrefix = filepath.ToSlash(strings.TrimPrefix(strings.TrimSpace(scopePrefix), "/"))
	if scopePrefix == "." {
		scopePrefix = ""
	}

	// Filter resources by scope prefix if specified
	var scoped []sourcepkg.Resource
	for _, res := range resources {
		cleanPath := filepath.ToSlash(strings.TrimPrefix(res.Path, "/"))
		if scopePrefix != "" {
			if cleanPath != scopePrefix && !strings.HasPrefix(cleanPath, scopePrefix+"/") {
				continue
			}
		}
		scoped = append(scoped, res)
	}

	// 1. Identify all SKILL.md files to find skill roots
	type skillRoot struct {
		skillDir    string // relative to source root ("" if root)
		skillMDPath string
		res         sourcepkg.Resource
	}
	var roots []skillRoot
	for _, res := range scoped {
		cleanPath := filepath.ToSlash(strings.TrimPrefix(res.Path, "/"))
		if filepath.Base(cleanPath) == "SKILL.md" {
			dir := filepath.ToSlash(filepath.Dir(cleanPath))
			if dir == "." {
				dir = ""
			}
			roots = append(roots, skillRoot{
				skillDir:    dir,
				skillMDPath: cleanPath,
				res:         res,
			})
		}
	}

	if len(roots) == 0 {
		return nil, nil
	}

	// Sort roots by path length descending so sub-skills match before parent skills
	sort.Slice(roots, func(i, j int) bool {
		return len(roots[i].skillDir) > len(roots[j].skillDir)
	})

	// 2. Discover companions and read contents for each skill
	items := make([]DiscoveredSkillItem, 0, len(roots))

	// Track which resources have been claimed by deeper skills
	claimedResources := make(map[string]bool)

	for _, root := range roots {
		claimedResources[root.skillMDPath] = true

		item := DiscoveredSkillItem{
			SkillDir:       root.skillDir,
			SkillMDPath:    root.skillMDPath,
			Collection:     "default",
			CompanionBytes: make(map[string][]byte),
		}

		// Read SKILL.md
		skillMDBytes, err := reader.Read(ctx, root.skillMDPath)
		if err != nil {
			item.Error = fmt.Sprintf("failed to read %s: %v", root.skillMDPath, err)
			items = append(items, item)
			continue
		}
		item.SkillMDBytes = skillMDBytes

		// Parse frontmatter
		name, desc, license := parseSkillMDFrontmatter(skillMDBytes)
		if name == "" {
			if root.skillDir != "" {
				name = filepath.Base(root.skillDir)
			} else {
				name = "imported-skill"
			}
		}
		if desc == "" {
			desc = name
		}
		item.Name = name
		item.Description = desc
		item.TargetID = sanitizeSkillID(name)
		if item.TargetID == "" {
			if root.skillDir != "" {
				item.TargetID = sanitizeSkillID(filepath.Base(root.skillDir))
			}
			if item.TargetID == "" {
				item.TargetID = "imported-skill"
			}
		}

		// Find companions: all resources under root.skillDir not claimed by other skills
		prefix := ""
		if root.skillDir != "" {
			prefix = root.skillDir + "/"
		}

		totalBytes := int64(len(skillMDBytes))
		fileCount := 1

		for _, res := range scoped {
			cleanPath := filepath.ToSlash(strings.TrimPrefix(res.Path, "/"))
			if claimedResources[cleanPath] {
				continue
			}
			if cleanPath == root.skillMDPath {
				continue
			}

			// Check if resource is under this skill directory
			if prefix != "" {
				if !strings.HasPrefix(cleanPath, prefix) {
					continue
				}
			}

			relPath := strings.TrimPrefix(cleanPath, prefix)
			if relPath == "" || relPath == "SKILL.md" {
				continue
			}
			if workspace.IsHubMeta(relPath) {
				item.Warnings = append(item.Warnings, Warning{
					Code:    "upstream_meta_ignored",
					Summary: fmt.Sprintf("Ignored upstream metadata file %q.", cleanPath),
				})
				continue
			}

			// Check for unsafe companion paths
			if isUnsafeCompanionPath(relPath) {
				item.Error = fmt.Sprintf("unsafe companion file path: %s", cleanPath)
				break
			}

			// Check size limit if known
			if res.Size > maxSkillResourceBytes {
				item.Error = fmt.Sprintf("companion file %s exceeds %d bytes", cleanPath, maxSkillResourceBytes)
				break
			}

			// Read companion content byte-for-byte
			compBytes, readErr := reader.Read(ctx, cleanPath)
			if readErr != nil {
				item.Error = fmt.Sprintf("failed to read companion %s: %v", cleanPath, readErr)
				break
			}

			if int64(len(compBytes)) > maxSkillResourceBytes {
				item.Error = fmt.Sprintf("companion file %s exceeds %d bytes", cleanPath, maxSkillResourceBytes)
				break
			}

			sum := sha256.Sum256(compBytes)
			digest := "sha256:" + hex.EncodeToString(sum[:])

			comp := DiscoveredCompanion{
				Path:     relPath,
				FullPath: cleanPath,
				Bytes:    int64(len(compBytes)),
				Digest:   digest,
			}

			item.Companions = append(item.Companions, comp)
			item.CompanionBytes[relPath] = compBytes
			claimedResources[cleanPath] = true
			totalBytes += int64(len(compBytes))
			fileCount++
		}

		// Sort companions by relative path for determinism
		sort.Slice(item.Companions, func(i, j int) bool {
			return item.Companions[i].Path < item.Companions[j].Path
		})

		item.TotalBytes = totalBytes
		item.FileCount = fileCount
		item.License = detectSkillLicense(license, item.Companions)

		// Compute transformations
		_, transforms, normErr := ensureImportedSkillFrontmatter(skillMDBytes, item.TargetID, item.Description)
		if normErr != nil && item.Error == "" {
			item.Error = normErr.Error()
		}
		item.Transformations = transforms

		items = append(items, item)
	}

	// Sort discovered items by TargetID
	sort.Slice(items, func(i, j int) bool {
		return items[i].TargetID < items[j].TargetID
	})

	return items, nil
}
