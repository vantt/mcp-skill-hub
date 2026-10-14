package app

import (
	"context"
	"testing"

	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
)

func TestDiscoveryIgnoresSkillRootsInsideExcludedDirectories(t *testing.T) {
	t.Parallel()
	md := func(name string) []byte {
		return []byte("---\nname: " + name + "\ndescription: d\n---\n\n# " + name + "\n")
	}
	files := MapResourceReader{
		"skills/real/SKILL.md":                    md("real"),
		"skills/real/vendor-note.md":              []byte("n\n"),
		"skills/real/node_modules/pkg/SKILL.md":   md("vendored"),
		"skills/real/node_modules/pkg/index.js":   []byte("x\n"),
		"node_modules/other/SKILL.md":             md("other"),
		"skills/real/__pycache__/cached/SKILL.md": md("cached"),
	}
	var resources []sourcepkg.Resource
	for path, data := range files {
		resources = append(resources, sourcepkg.Resource{Path: path, Size: int64(len(data))})
	}
	items, err := DiscoverSkillsFromResources(context.Background(), files, resources, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].TargetID != "real" {
		var ids []string
		for _, item := range items {
			ids = append(ids, item.TargetID)
		}
		t.Fatalf("discovered %v, want only [real]", ids)
	}
	left := map[string]bool{}
	for _, file := range items[0].Excluded {
		left[file.Path] = true
	}
	if !left["node_modules/pkg/SKILL.md"] || !left["node_modules/pkg/index.js"] {
		t.Fatalf("Excluded = %#v, want the vendored files listed", items[0].Excluded)
	}
}
