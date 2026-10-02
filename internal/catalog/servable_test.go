package catalog

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildWarnsAboutActiveSkillsThatCannotBeServed(t *testing.T) {
	t.Parallel()
	root := newWorkspace(t)
	meta := func(id string) string {
		return "schema_version: 1\nid: " + id + "\nname: Skill\nstatus: active\ndescription: Fixture skill.\nrouting:\n  triggers: [fixture]\n  not_for: [unrelated]\n  min_scope: single_step\n"
	}
	writeCanonical(t, root, "skills/core/good/skill.meta.yaml", meta("good"))
	writeCanonical(t, root, "skills/core/good/SKILL.md", "---\nname: good\ndescription: Fixture skill.\n---\n\n# Good\n")
	writeCanonical(t, root, "skills/core/renamed/skill.meta.yaml", meta("renamed"))
	writeCanonical(t, root, "skills/core/renamed/SKILL.md", "---\nname: renamed\ndescription: \"\"\n---\n\n# Renamed\n")

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
	t.Parallel()
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

func TestAssessSkillStateHealthySkill(t *testing.T) {
	t.Parallel()
	root := newWorkspace(t)
	meta := "schema_version: 1\nid: review\nname: review\nstatus: active\ndescription: Code review skill.\nrouting:\n  triggers: [review code]\n  not_for: [marketing]\n  min_scope: single_step\n"
	writeCanonical(t, root, "skills/core/review/skill.meta.yaml", meta)
	writeCanonical(t, root, "skills/core/review/SKILL.md", "---\nname: review\ndescription: Code review skill.\n---\n\n# Review\n")
	build(t, root, BuildOptions{})

	assessment, err := AssessSkillState(t.Context(), root, "review")
	if err != nil {
		t.Fatalf("AssessSkillState failed: %v", err)
	}
	if !assessment.Canonical.Known || !assessment.Canonical.Valid || assessment.Canonical.Status != "active" {
		t.Fatalf("unexpected canonical facts: %#v", assessment.Canonical)
	}
	if !assessment.Served.Known || !assessment.Served.Indexed || !assessment.Served.Servable || assessment.Served.Status != "active" {
		t.Fatalf("unexpected served facts: %#v", assessment.Served)
	}
	if !assessment.ResourcesMatch || len(assessment.ChangedResources) != 0 || len(assessment.MissingResources) != 0 {
		t.Fatalf("resources should match: %#v", assessment)
	}
	if assessment.Diverged {
		t.Fatal("healthy skill should not be diverged")
	}
	if assessment.ServingMode != ServingCurrent {
		t.Fatalf("serving mode = %v, want %v", assessment.ServingMode, ServingCurrent)
	}
}

func TestAssessSkillStateInvalidCanonicalEdit(t *testing.T) {
	t.Parallel()
	root := newWorkspace(t)
	meta := "schema_version: 1\nid: review\nname: review\nstatus: active\ndescription: Code review skill.\nrouting:\n  triggers: [review code]\n  not_for: [marketing]\n  min_scope: single_step\n"
	writeCanonical(t, root, "skills/core/review/skill.meta.yaml", meta)
	writeCanonical(t, root, "skills/core/review/SKILL.md", "---\nname: review\ndescription: Code review skill.\n---\n\n# Review\n")
	build(t, root, BuildOptions{})

	// Invalidate canonical status (BUG-07 scenario)
	badMeta := "schema_version: 1\nid: review\nname: review\nstatus: shiny\ndescription: Code review skill.\n"
	writeCanonical(t, root, "skills/core/review/skill.meta.yaml", badMeta)

	assessment, err := AssessSkillState(t.Context(), root, "review")
	if err != nil {
		t.Fatalf("AssessSkillState failed: %v", err)
	}
	if !assessment.Canonical.Known {
		t.Fatal("canonical should be known")
	}
	if assessment.Canonical.Valid {
		t.Fatal("canonical should be invalid after introducing 'status: shiny'")
	}
	if len(assessment.Canonical.Issues) == 0 {
		t.Fatal("canonical issues should be reported")
	}
	if !assessment.Served.Known || !assessment.Served.Indexed {
		t.Fatal("served generation should remain known from published generation")
	}
	if assessment.ServingMode != ServingFallback {
		t.Fatalf("serving mode = %v, want %v", assessment.ServingMode, ServingFallback)
	}
	if !assessment.Diverged {
		t.Fatal("skill should be marked diverged when canonical status differs from served")
	}
}

func TestAssessSkillStateChangedLiveResource(t *testing.T) {
	t.Parallel()
	root := newWorkspace(t)
	meta := "schema_version: 1\nid: review\nname: review\nstatus: active\ndescription: Code review skill.\nrouting:\n  triggers: [review code]\n  not_for: [marketing]\n  min_scope: single_step\n"
	writeCanonical(t, root, "skills/core/review/skill.meta.yaml", meta)
	writeCanonical(t, root, "skills/core/review/SKILL.md", "---\nname: review\ndescription: Code review skill.\n---\n\n# Review\n")
	build(t, root, BuildOptions{})

	// Modify live SKILL.md bytes
	writeCanonical(t, root, "skills/core/review/SKILL.md", "---\nname: review\ndescription: Code review skill.\n---\n\n# Tampered\n")

	assessment, err := AssessSkillState(t.Context(), root, "review")
	if err != nil {
		t.Fatalf("AssessSkillState failed: %v", err)
	}
	if assessment.ResourcesMatch {
		t.Fatal("resources match should be false when live file differs from generation digest")
	}
	if len(assessment.ChangedResources) == 0 {
		t.Fatal("changed resources should list modified SKILL.md")
	}
	if !assessment.Diverged {
		t.Fatal("diverged should be true when resources differ")
	}
}
