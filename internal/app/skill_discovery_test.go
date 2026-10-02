package app

import (
	"context"
	"strings"
	"testing"

	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func TestSanitizeSkillID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    string
		expected string
	}{
		{"pdf-extractor", "pdf-extractor"},
		{"PDF Extractor", "pdf-extractor"},
		{"PDF   ---   Extractor!!!", "pdf-extractor"},
		{"--leading-and-trailing--", "leading-and-trailing"},
		{"special_chars@#$123", "special-chars-123"},
		{strings.Repeat("a", 100), strings.Repeat("a", 63)},
	}

	for _, c := range cases {
		got := sanitizeSkillID(c.input)
		if got != c.expected {
			t.Errorf("sanitizeSkillID(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestSkillFrontmatterParsingAndValidation(t *testing.T) {
	t.Parallel()
	// Valid frontmatter with all fields
	valid := []byte(`---
name: my-skill
description: A helpful skill
license: MIT
author: Team
---

# My Skill Body
`)
	name, desc, lic := parseSkillMDFrontmatter(valid)
	if name != "my-skill" || desc != "A helpful skill" || lic != "MIT" {
		t.Fatalf("unexpected parsed frontmatter: name=%q desc=%q lic=%q", name, desc, lic)
	}

	// Unclosed frontmatter error
	unclosed := []byte(`---
name: broken
# no closing delimiter
`)
	_, _, _, err := parseAndValidateSkillFrontmatter(unclosed)
	if err == nil || !strings.Contains(err.Error(), "unclosed") {
		t.Fatalf("expected unclosed delimiter error, got %v", err)
	}

	// Invalid YAML error
	invalidYAML := []byte(`---
name: [unclosed list
---
`)
	_, _, _, err = parseAndValidateSkillFrontmatter(invalidYAML)
	if err == nil || !strings.Contains(err.Error(), "invalid frontmatter YAML") {
		t.Fatalf("expected invalid YAML error, got %v", err)
	}

	// Non-mapping frontmatter error
	scalarFrontmatter := []byte(`---
"just a string"
---
`)
	_, _, _, err = parseAndValidateSkillFrontmatter(scalarFrontmatter)
	if err == nil || !strings.Contains(err.Error(), "must be a YAML mapping") {
		t.Fatalf("expected mapping error, got %v", err)
	}
}

func TestEnsureImportedSkillFrontmatterPreservesContentAndNormalizes(t *testing.T) {
	t.Parallel()
	// Existing frontmatter with different name and custom fields
	input := []byte("---\r\nname: Original Name\r\ndescription: Existing desc\r\nlicense: Apache-2.0\r\ncustom_field: 42\r\n---\r\n\r\n# Body Heading\r\n\r\nBody text\r\n")

	normalized, transforms, err := ensureImportedSkillFrontmatter(input, "target-id", "fallback desc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	normStr := string(normalized)
	if !strings.Contains(normStr, "name: target-id") {
		t.Errorf("expected name normalized to target-id, got:\n%s", normStr)
	}
	if !strings.Contains(normStr, "description: Existing desc") {
		t.Errorf("expected existing description preserved, got:\n%s", normStr)
	}
	if !strings.Contains(normStr, "license: Apache-2.0") {
		t.Errorf("expected custom license preserved, got:\n%s", normStr)
	}
	if !strings.Contains(normStr, "custom_field: 42") {
		t.Errorf("expected custom field preserved, got:\n%s", normStr)
	}
	if !strings.Contains(normStr, "# Body Heading\n\nBody text\n") {
		t.Errorf("expected body text preserved, got:\n%s", normStr)
	}

	// Verify transformations
	hasNormalizeName := false
	hasCRLF := false
	for _, tr := range transforms {
		if tr == "normalize_frontmatter_name" {
			hasNormalizeName = true
		}
		if tr == "crlf_to_lf" {
			hasCRLF = true
		}
	}
	if !hasNormalizeName {
		t.Errorf("expected normalize_frontmatter_name transformation, got %v", transforms)
	}
	if !hasCRLF {
		t.Errorf("expected crlf_to_lf transformation, got %v", transforms)
	}

	// No frontmatter: prepends frontmatter
	noFM := []byte("# Raw Skill Without Frontmatter\n")
	prepended, pTransforms, err := ensureImportedSkillFrontmatter(noFM, "raw-id", "Raw Description")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	prepStr := string(prepended)
	if !strings.HasPrefix(prepStr, "---\nname: raw-id\ndescription: Raw Description\n---\n\n# Raw Skill") {
		t.Errorf("expected prepended frontmatter, got:\n%s", prepStr)
	}
	if len(pTransforms) == 0 || pTransforms[0] != "add_frontmatter" {
		t.Errorf("expected add_frontmatter transform, got %v", pTransforms)
	}

	// Malformed frontmatter rejected
	malformed := []byte("---\nname: broken\n---\n---\n") // duplicate / broken
	_, _, err = ensureImportedSkillFrontmatter(malformed, "id", "desc")
	if err == nil {
		t.Fatal("expected error on malformed frontmatter, got nil")
	}
}

func TestDetectSkillLicense(t *testing.T) {
	t.Parallel()
	// 1. Declared standard license, no file
	info1 := detectSkillLicense("MIT", nil)
	if info1.Declared != "MIT" || info1.IsUnknown || info1.IsProprietary || info1.Warning != "" {
		t.Errorf("info1 mismatch: %#v", info1)
	}

	// 2. Declared proprietary license with license file (BUG-16)
	companions := []DiscoveredCompanion{
		{Path: "LICENSE.txt", FullPath: "skills/pdf/LICENSE.txt"},
	}
	info2 := detectSkillLicense("Proprietary. LICENSE.txt has complete terms", companions)
	if !info2.IsProprietary {
		t.Errorf("expected IsProprietary=true, got %#v", info2)
	}
	if info2.LicenseFile != "LICENSE.txt" {
		t.Errorf("expected LicenseFile=LICENSE.txt, got %q", info2.LicenseFile)
	}
	if !strings.Contains(info2.Warning, "proprietary") {
		t.Errorf("expected proprietary warning, got %q", info2.Warning)
	}

	// 3. Unknown declared license without license file
	info3 := detectSkillLicense("", nil)
	if !info3.IsUnknown || !strings.Contains(info3.Warning, "unknown") {
		t.Errorf("expected unknown warning, got %#v", info3)
	}

	// 4. Unknown declared license with license file
	info4 := detectSkillLicense("", companions)
	if !info4.IsUnknown || info4.LicenseFile != "LICENSE.txt" || !strings.Contains(info4.Warning, "LICENSE.txt is present") {
		t.Errorf("expected file notice, got %#v", info4)
	}
}

func TestDiscoverSkillsFromResources_FolderScopedBUG04(t *testing.T) {
	t.Parallel()
	// BUG-04 scenario: Folder-scoped source where SKILL.md is at root (skillDir == "")
	// Contains: SKILL.md, LICENSE.txt, forms.md, reference.md, scripts/extract.py, empty.txt, binary.bin
	files := map[string][]byte{
		"SKILL.md":           []byte("---\nname: pdf\ndescription: PDF Tool\nlicense: Proprietary. LICENSE.txt has complete terms\n---\n\n# PDF Tool\n"),
		"LICENSE.txt":        []byte("Commercial License Terms\n"),
		"forms.md":           []byte("# Forms Documentation\n"),
		"reference.md":       []byte("# Reference API\n"),
		"scripts/extract.py": []byte("print('extracting')\n"),
		"empty.txt":          []byte(""),
		"assets/icon.bin":    {0x00, 0xFF, 0xFE, 0x80, 0x12}, // binary
	}

	resources := []sourcepkg.Resource{
		{Path: "SKILL.md", Size: int64(len(files["SKILL.md"]))},
		{Path: "LICENSE.txt", Size: int64(len(files["LICENSE.txt"]))},
		{Path: "forms.md", Size: int64(len(files["forms.md"]))},
		{Path: "reference.md", Size: int64(len(files["reference.md"]))},
		{Path: "scripts/extract.py", Size: int64(len(files["scripts/extract.py"]))},
		{Path: "empty.txt", Size: 0},
		{Path: "assets/icon.bin", Size: int64(len(files["assets/icon.bin"]))},
	}

	reader := MapResourceReader(files)
	discovered, err := DiscoverSkillsFromResources(context.Background(), reader, resources, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(discovered) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(discovered))
	}

	item := discovered[0]
	if item.TargetID != "pdf" {
		t.Errorf("expected target ID 'pdf', got %q", item.TargetID)
	}
	if item.SkillDir != "" {
		t.Errorf("expected empty skillDir for root-scoped skill, got %q", item.SkillDir)
	}

	// Verify all 6 companions are preserved!
	if len(item.Companions) != 6 {
		t.Fatalf("expected 6 companions preserved, got %d: %#v", len(item.Companions), item.Companions)
	}

	expectedCompanions := map[string]int64{
		"LICENSE.txt":        int64(len(files["LICENSE.txt"])),
		"forms.md":           int64(len(files["forms.md"])),
		"reference.md":       int64(len(files["reference.md"])),
		"scripts/extract.py": int64(len(files["scripts/extract.py"])),
		"empty.txt":          0,
		"assets/icon.bin":    5,
	}

	for _, comp := range item.Companions {
		expectedBytes, ok := expectedCompanions[comp.Path]
		if !ok {
			t.Errorf("unexpected companion path: %s", comp.Path)
		}
		if comp.Bytes != expectedBytes {
			t.Errorf("companion %s: expected %d bytes, got %d", comp.Path, expectedBytes, comp.Bytes)
		}
		data, inMap := item.CompanionBytes[comp.Path]
		if !inMap {
			t.Errorf("companion %s missing from CompanionBytes map", comp.Path)
		}
		if int64(len(data)) != expectedBytes {
			t.Errorf("companion %s content length %d, expected %d", comp.Path, len(data), expectedBytes)
		}
	}

	// Verify license detection
	if !item.License.IsProprietary {
		t.Error("expected IsProprietary=true")
	}
	if item.License.LicenseFile != "LICENSE.txt" {
		t.Errorf("expected LicenseFile=LICENSE.txt, got %q", item.License.LicenseFile)
	}
}

func TestDiscoverSkillsFromResources_MultipleAndNested(t *testing.T) {
	t.Parallel()
	files := map[string][]byte{
		"skills/parent/SKILL.md":         []byte("---\nname: Parent\ndescription: Parent Skill\n---\n"),
		"skills/parent/top-file.txt":     []byte("parent file"),
		"skills/parent/child/SKILL.md":   []byte("---\nname: Child\ndescription: Child Skill\n---\n"),
		"skills/parent/child/nested.txt": []byte("child file"),
	}

	resources := []sourcepkg.Resource{
		{Path: "skills/parent/SKILL.md"},
		{Path: "skills/parent/top-file.txt", Size: 11},
		{Path: "skills/parent/child/SKILL.md"},
		{Path: "skills/parent/child/nested.txt", Size: 10},
	}

	reader := MapResourceReader(files)
	discovered, err := DiscoverSkillsFromResources(context.Background(), reader, resources, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(discovered) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(discovered))
	}

	// Skills are sorted by TargetID: "child", "parent"
	child := discovered[0]
	parent := discovered[1]

	if child.TargetID != "child" || parent.TargetID != "parent" {
		t.Fatalf("unexpected order: %s, %s", child.TargetID, parent.TargetID)
	}

	// Child has nested.txt
	if len(child.Companions) != 1 || child.Companions[0].Path != "nested.txt" {
		t.Errorf("child companions: %#v", child.Companions)
	}

	// Parent has top-file.txt, NOT nested.txt or child/SKILL.md
	if len(parent.Companions) != 1 || parent.Companions[0].Path != "top-file.txt" {
		t.Errorf("parent companions: %#v", parent.Companions)
	}
}
