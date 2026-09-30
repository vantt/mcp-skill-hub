package catalog

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildWarnsAboutActiveSkillsThatCannotBeServed(t *testing.T) {
	root := newWorkspace(t)
	meta := func(id string) string {
		return "schema_version: 1\nid: " + id + "\nname: Skill\nstatus: active\ndescription: Fixture skill.\nrouting:\n  triggers: [fixture]\n  not_for: [unrelated]\n  min_scope: single_step\n"
	}
	writeCanonical(t, root, "skills/core/good/skill.meta.yaml", meta("good"))
	writeCanonical(t, root, "skills/core/good/SKILL.md", "---\nname: good\ndescription: Fixture skill.\n---\n\n# Good\n")
	writeCanonical(t, root, "skills/core/renamed/skill.meta.yaml", meta("renamed"))
	writeCanonical(t, root, "skills/core/renamed/SKILL.md", "---\nname: other-name\ndescription: Fixture skill.\n---\n\n# Renamed\n")

	result := build(t, root, BuildOptions{})
	var warned []string
	for _, warning := range result.Warnings {
		if strings.HasPrefix(warning, "skills/core/") {
			warned = append(warned, warning)
		}
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "renamed") || !strings.Contains(warned[0], `name "renamed"`) {
		t.Fatalf("servability warnings = %#v", warned)
	}
}

func TestValidateServableSkillRequiresMatchingFrontmatter(t *testing.T) {
	good := []byte("---\nname: alpha\ndescription: Does things.\n---\n\n# Alpha\n")
	if err := ValidateServableSkill("alpha", good, 1, int64(len(good))); err != nil {
		t.Fatalf("servable skill rejected: %v", err)
	}
	for name, entrypoint := range map[string][]byte{
		"mismatched name":   []byte("---\nname: beta\ndescription: Does things.\n---\n"),
		"blank description": []byte("---\nname: alpha\ndescription: \" \"\n---\n"),
		"no frontmatter":    []byte("# Alpha\n"),
	} {
		if err := ValidateServableSkill("alpha", entrypoint, 1, int64(len(entrypoint))); !errors.Is(err, ErrSkillNotServable) {
			t.Errorf("%s: error = %v, want ErrSkillNotServable", name, err)
		}
	}
	if err := ValidateServableSkill("alpha", good, MaxDistributedSkillResources+1, 1); !errors.Is(err, ErrSkillNotServable) {
		t.Errorf("too many resources: error = %v", err)
	}
}
