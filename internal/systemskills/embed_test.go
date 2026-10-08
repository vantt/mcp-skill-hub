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
		"source_intake_add",
		"source_intake_list",
		"source_triage",
		"source_check",
		"skill_upstream_status",
		"source_link_preview",
		"source_unwatch_preview",
		"source_import_preview",
		"source_import_confirm",
		"curation_run_start",
		"curation_run_submit",
		"curation_run_get",
		"curation_run_retry",
		"curation_run_cancel",
		"observation_list",
		"comparison_get",
		"inbox_list",
		"insight_get",
		"insight_decide",
		"insight_apply_preview",
		"insight_apply_confirm",
		"skill_create_preview",
		"skill_create_confirm",
		"skill_transition_preview",
		"skill_transition_confirm",
		"skill_list",
		"skill_get",
		"skill_update_preview",
		"skill_update_confirm",
		"routing_evaluate",
		"outcome_record",
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

func TestCuratorGuidanceDurableBehaviorContract(t *testing.T) {
	t.Parallel()

	normalizedSkill := strings.Join(strings.Fields(CuratorSkill), " ")
	contracts := map[string]string{
		"explicit activation":       "Use this skill only when the user explicitly asks",
		"best-effort boundary":      "instruction-only coordination contract",
		"application services":      "Domain mutation remains in application services",
		"status-first offline flow": "call `hub_status` first. It is local and offline",
		"recovery priority":         "Interrupted or failed operations",
		"one primary question":      "Ask at most one primary, high-value question per turn",
		"terminology translation":   "| Observation | Finding |",
		"progressive disclosure":    "**L0:** status and one recommended next action",
		"approval matrix":           "Preview first, then require explicit approval pinned to proposal ID, digest, and base version",
		"batch orchestration":       "Call `source_check` once for the requested batch",
		"no active batch apply":     "**Active skills were not changed.**",
		"independent recovery":      "recommend `skillhub doctor` or `skillhub doctor --fix`",
		"honest session telemetry":  "Never guess, infer, or backfill a measurement that was not observed",
		"completion-only telemetry": "only after the curation session has actually ended",
	}
	for name, text := range contracts {
		if !strings.Contains(normalizedSkill, text) {
			t.Errorf("missing %s contract %q", name, text)
		}
	}
}

func TestCuratorGuidanceRequiresPreviewBeforeConfirm(t *testing.T) {
	t.Parallel()

	pairs := [][2]string{
		{"`insight_apply_preview`", "`insight_apply_confirm`"},
		{"`skill_create_preview`", "`skill_create_confirm`"},
		{"`skill_transition_preview`", "`skill_transition_confirm`"},
		{"`skill_update_preview`", "`skill_update_confirm`"},
	}
	for _, pair := range pairs {
		preview := strings.Index(CuratorSkill, pair[0])
		confirm := strings.Index(CuratorSkill, pair[1])
		if preview < 0 || confirm < 0 {
			t.Fatalf("missing semantic mutation pair %v", pair)
		}
		if preview >= confirm {
			t.Errorf("preview %s must precede confirm %s", pair[0], pair[1])
		}
	}

	normalizedSkill := strings.Join(strings.Fields(CuratorSkill), " ")
	for _, prohibited := range []string{
		"edit canonical Hub files directly",
		"emulate the missing tool by editing files or databases",
	} {
		if !strings.Contains(normalizedSkill, prohibited) {
			t.Errorf("missing no-bypass instruction %q", prohibited)
		}
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
