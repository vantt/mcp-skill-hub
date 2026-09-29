package systemskills

import (
	"os"
	"strings"
	"testing"
)

func TestCuratorSkillIsEmbedded(t *testing.T) {
	t.Parallel()

	if strings.TrimSpace(CuratorSkill) == "" {
		t.Fatal("CuratorSkill is empty")
	}
	if !strings.Contains(CuratorSkill, "Domain mutation remains in application services") {
		t.Fatal("CuratorSkill must preserve the application-service mutation boundary")
	}

	canonicalSkill, err := os.ReadFile("../../system-skills/curator/SKILL.md")
	if err != nil {
		t.Fatalf("read canonical Curator Skill: %v", err)
	}
	if CuratorSkill != string(canonicalSkill) {
		t.Fatal("embedded Curator Skill differs from the canonical system-skills asset")
	}
}
