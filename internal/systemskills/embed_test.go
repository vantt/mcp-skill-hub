package systemskills

import (
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCuratorSkillIsEmbeddedAndVersioned(t *testing.T) {
	t.Parallel()

	if strings.TrimSpace(CuratorSkill) == "" {
		t.Fatal("CuratorSkill is empty")
	}

	canonicalSkill, err := os.ReadFile("../../system-skills/curator/SKILL.md")
	if err != nil {
		t.Fatalf("read canonical Curator Skill: %v", err)
	}
	if CuratorSkill != string(canonicalSkill) {
		t.Fatal("embedded Curator Skill differs from the canonical system-skills asset")
	}

	for _, want := range []string{
		"name: " + CuratorSkillID,
		"version: " + CuratorSkillVersion,
		"contract-version: \"" + CuratorContractVersion + "\"",
	} {
		if !strings.Contains(CuratorSkill, want) {
			t.Errorf("CuratorSkill does not declare %q", want)
		}
	}

	bundle := CuratorBundle()
	if bundle.Instructions != CuratorSkill {
		t.Fatal("CuratorBundle instructions differ from embedded content")
	}
	sum := sha256.Sum256([]byte(CuratorSkill))
	wantDigest := fmt.Sprintf("sha256:%x", sum)
	if bundle.Digest != wantDigest {
		t.Fatalf("CuratorBundle digest = %q, want %q", bundle.Digest, wantDigest)
	}
}

func TestCuratorCompatibilityMetadata(t *testing.T) {
	t.Parallel()

	metadata := CuratorMetadata()
	if metadata.SkillID != CuratorSkillID || metadata.SkillVersion != CuratorSkillVersion {
		t.Fatalf("identity metadata = %q@%q", metadata.SkillID, metadata.SkillVersion)
	}
	if metadata.ContractVersion != CuratorContractVersion {
		t.Fatalf("contract version = %q, want %q", metadata.ContractVersion, CuratorContractVersion)
	}
	if metadata.ActivationPolicy != "explicit-only" {
		t.Fatalf("activation policy = %q, want explicit-only", metadata.ActivationPolicy)
	}
	if metadata.CoordinationBoundary != "instruction-only-best-effort" {
		t.Fatalf("coordination boundary = %q", metadata.CoordinationBoundary)
	}
	if !metadata.InstructionOnly || !metadata.BestEffortCoordination || !metadata.RequiresApplicationService {
		t.Fatalf("unsafe curator boundaries: %+v", metadata)
	}

	wantTools := []string{
		"hub_status",
		"source_list",
		"source_check",
		"skill_upstream_status",
		"source_watch_preview",
		"source_watch_confirm",
		"source_import_preview",
		"source_import_confirm",
		"skill_add_preview",
		"skill_add_confirm",
		"skill_create_preview",
		"skill_create_confirm",
		"skill_transition_preview",
		"skill_transition_confirm",
		"skill_list",
		"skill_review",
		"skill_update_preview",
		"skill_update_confirm",
		"routing_evaluate",
		"curation_session_record",
		"workspace_validate",
		"workspace_rebuild",
		"workspace_diff",
	}
	if strings.Join(metadata.CompatibleTools, "\n") != strings.Join(wantTools, "\n") {
		t.Fatalf("compatible tools do not match the contract:\n got %v\nwant %v", metadata.CompatibleTools, wantTools)
	}

	seen := make(map[string]struct{}, len(metadata.CompatibleTools))
	validName := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for _, tool := range metadata.CompatibleTools {
		if !validName.MatchString(tool) {
			t.Errorf("invalid tool name %q", tool)
		}
		if _, exists := seen[tool]; exists {
			t.Errorf("duplicate tool name %q", tool)
		}
		seen[tool] = struct{}{}
		if !strings.Contains(CuratorSkill, "\n"+tool) {
			t.Errorf("compatible tool %q is absent from embedded guidance", tool)
		}
	}

	metadata.CompatibleTools[0] = "mutated_by_caller"
	if CuratorMetadata().CompatibleTools[0] != "hub_status" {
		t.Fatal("CuratorMetadata returned mutable package-level tool metadata")
	}
}

// The curator's SKILL.md is served verbatim, so its frontmatter is where the
// compatibility contract lives; this keeps it equal to CuratorMetadata.
func TestCuratorFrontmatterDeclaresCompatibilityMetadata(t *testing.T) {
	t.Parallel()

	end := strings.Index(CuratorSkill[4:], "\n---\n")
	if !strings.HasPrefix(CuratorSkill, "---\n") || end < 0 {
		t.Fatal("CuratorSkill has no frontmatter")
	}
	var frontmatter struct {
		Name                       string   `yaml:"name"`
		Version                    string   `yaml:"version"`
		ContractVersion            string   `yaml:"contract-version"`
		ActivationPolicy           string   `yaml:"activation-policy"`
		CoordinationBoundary       string   `yaml:"coordination-boundary"`
		InstructionOnly            bool     `yaml:"instruction-only"`
		BestEffortCoordination     bool     `yaml:"best-effort-coordination"`
		RequiresApplicationService bool     `yaml:"requires-application-service"`
		CompatibleTools            []string `yaml:"compatible-tools"`
	}
	if err := yaml.Unmarshal([]byte(CuratorSkill[4:4+end]), &frontmatter); err != nil {
		t.Fatalf("parse frontmatter: %v", err)
	}
	metadata := CuratorMetadata()
	got := CompatibilityMetadata{
		SkillID:                    frontmatter.Name,
		SkillVersion:               frontmatter.Version,
		ContractVersion:            frontmatter.ContractVersion,
		ActivationPolicy:           frontmatter.ActivationPolicy,
		CoordinationBoundary:       frontmatter.CoordinationBoundary,
		CompatibleTools:            frontmatter.CompatibleTools,
		InstructionOnly:            frontmatter.InstructionOnly,
		BestEffortCoordination:     frontmatter.BestEffortCoordination,
		RequiresApplicationService: frontmatter.RequiresApplicationService,
	}
	if !reflect.DeepEqual(got, metadata) {
		t.Fatalf("SKILL.md frontmatter = %#v\nCuratorMetadata  = %#v", got, metadata)
	}
}
