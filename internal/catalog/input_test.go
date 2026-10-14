package catalog

import (
	"strings"
	"testing"
)

func TestNormalizeSkillDocumentMatchesTheCatalogBuild(t *testing.T) {
	t.Parallel()
	meta := []byte("id: sample\nstatus: active\nrouting:\n  triggers: [review a pull request]\n  topics: [review]\n  aliases: [pr-check]\n")
	skillMD := []byte("---\nname: Sample Skill\ndescription: Reviews pull requests.\n---\n\n# Sample\n")

	got, err := NormalizeSkillDocument(meta, skillMD, "sample")
	if err != nil {
		t.Fatal(err)
	}
	entity, ok, err := parseEntity(
		inputFile{Path: "skills/software/sample/skill.meta.yaml", Bytes: meta},
		map[string][]byte{"skills/software/sample/SKILL.md": skillMD},
	)
	if err != nil || !ok {
		t.Fatalf("parseEntity ok=%v err=%v", ok, err)
	}
	if string(got) != entity.JSON {
		t.Fatalf("NormalizeSkillDocument differs from the catalog build:\n got  %s\n want %s", got, entity.JSON)
	}
	for _, want := range []string{`"name":"Sample Skill"`, `"description":"Reviews pull requests."`, `"topics":["review"]`, `"aliases":["pr-check"]`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("normalized JSON is missing %s: %s", want, got)
		}
	}
}
