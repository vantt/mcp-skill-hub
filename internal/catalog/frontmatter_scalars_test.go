package catalog

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func frontmatterOf(t *testing.T, header string) map[string]any {
	t.Helper()
	got, err := ParseSkillFrontmatter([]byte("---\n" + header + "\n---\n\n# Body\n"))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestFrontmatterScalarsKeepWrittenForm(t *testing.T) {
	t.Parallel()
	got := frontmatterOf(t, `name: a
description: d
released: 2026-10-12
stamp: 2026-10-12T08:30:00Z
quoted: "2026-10-12"
big: 12345678901234567890
huge: -123456789012345678901234567890
hex: 0xFFFFFFFFFFFFFFFF
small: 42
ratio: 1.5
flag: true
nested:
  when: 2026-01-02
  items: [2026-03-04, text, 7, 98765432109876543210]
list:
  - when: 2026-05-06
anchor: &d 2026-07-08
again: *d`)
	want := map[string]any{
		"name": "a", "description": "d", "released": "2026-10-12", "stamp": "2026-10-12T08:30:00Z", "quoted": "2026-10-12",
		"big": json.Number("12345678901234567890"), "huge": json.Number("-123456789012345678901234567890"),
		"hex": json.Number("18446744073709551615"), "small": float64(42), "ratio": 1.5, "flag": true,
		"nested": map[string]any{"when": "2026-01-02", "items": []any{"2026-03-04", "text", float64(7), json.Number("98765432109876543210")}},
		"list":   []any{map[string]any{"when": "2026-05-06"}},
		"anchor": "2026-07-08", "again": "2026-07-08",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("frontmatter = %#v\nwant %#v", got, want)
	}
	encoded, err := json.Marshal(got)
	if err != nil || !bytes.Contains(encoded, []byte(`"big":12345678901234567890`)) || !bytes.Contains(encoded, []byte(`"released":"2026-10-12"`)) {
		t.Fatalf("json = %s, %v", encoded, err)
	}
}

func TestFrontmatterMergedKeysKeepWrittenForm(t *testing.T) {
	t.Parallel()
	got := frontmatterOf(t, "name: a\ndescription: d\nbase: &b\n  when: 2026-02-03\nderived:\n  <<: *b\n  extra: 1")
	derived, _ := got["derived"].(map[string]any)
	if derived["when"] != "2026-02-03" || derived["extra"] != float64(1) {
		t.Fatalf("derived = %#v", derived)
	}
}

// legacyFrontmatter is the decoder as it was before scalars kept their written
// form, kept here to prove ordinary frontmatter is unchanged.
func legacyFrontmatter(t *testing.T, contents []byte) map[string]any {
	t.Helper()
	contents = bytes.ReplaceAll(contents, []byte("\r\n"), []byte("\n"))
	end := bytes.Index(contents[4:], []byte("\n---\n"))
	var value map[string]any
	if err := yaml.NewDecoder(bytes.NewReader(contents[4 : 4+end])).Decode(&value); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		t.Fatal(err)
	}
	return normalized
}

func TestOrdinaryFrontmatterMatchesPreviousDecoder(t *testing.T) {
	t.Parallel()
	var files []string
	for _, root := range []string{"../../system-skills", "../systemskills", "../../docs/references/skills", "../../.agents/skills"} {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && entry.Name() == "SKILL.md" {
				files = append(files, path)
			}
			return nil
		})
	}
	if len(files) == 0 {
		t.Fatal("no fixture skills found")
	}
	files = append(files, "")
	for _, path := range files {
		var contents []byte
		if path == "" {
			contents = []byte("---\nname: x\ndescription: \"quoted: text\"\nn: 3\nf: 0.25\nneg: -9007199254740992\nb: no\nnull_value: ~\nlist: [a, 1, true]\nmap: {k: v}\n---\n")
		} else {
			var err error
			if contents, err = os.ReadFile(path); err != nil {
				t.Fatal(err)
			}
		}
		got, err := ParseSkillFrontmatter(contents)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if want := legacyFrontmatter(t, contents); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: frontmatter changed\n got %#v\nwant %#v", path, got, want)
		}
	}
}
