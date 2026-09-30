package canonical

import (
	"strings"
	"testing"
)

// FuzzCanonicalParsersAndPaths keeps parser and path-validation coverage bounded
// while exercising the same helpers used by workspace validation.
func FuzzCanonicalParsersAndPaths(f *testing.F) {
	seeds := []struct {
		path string
		data []byte
	}{
		{"sources/catalog/source.yaml", []byte("schema_version: 1\nid: source\nadapter: git\nlocator: https://example.invalid/repo\n")},
		{"skills/core/review/skill.meta.yaml", []byte("schema_version: 1\nid: review\nname: Review\nstatus: draft\ndescription: Review changes.\n")},
		{"../outside.yaml", []byte("id: escaped\n")},
		{"distill/value.yaml", []byte("---\nid: one\n---\nid: two\n")},
	}
	for _, seed := range seeds {
		f.Add(seed.path, seed.data)
	}

	f.Fuzz(func(t *testing.T, path string, data []byte) {
		if len(path) > 512 || len(data) > 64<<10 {
			t.Skip()
		}
		_, _, _ = parseYAMLIdentity(path, data)
		_, _ = validateSkillMetadata(path, data)
		_ = validateSourcePolicy(data)
		_ = validCanonicalPath(path)
		_ = entityPath(path)
		_ = hasConflictMarker(string(data))

		if strings.HasPrefix(path, "/") || path == ".." || strings.HasPrefix(path, "../") {
			if validCanonicalPath(path) {
				t.Fatalf("unsafe top-level path accepted: %q", path)
			}
		}
	})
}
