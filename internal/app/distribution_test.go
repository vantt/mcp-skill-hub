package app

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

func TestDistributionServesBundledSystemCurator(t *testing.T) {
	root := newSkillWorkspace(t)
	service := DistributionService{}

	entries, snapshot, err := service.ListSkills(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot == "" || len(entries) != 1 {
		t.Fatalf("list = %#v, snapshot %q", entries, snapshot)
	}
	curator := entries[0]
	if curator.SkillID != systemskills.CuratorSkillID || !strings.HasPrefix(curator.URI, "skill://skillhub/") || len(curator.Resources) != 1 {
		t.Fatalf("curator entry = %#v", curator)
	}
	for key, want := range map[string]any{
		"name":                         systemskills.CuratorSkillID,
		"version":                      systemskills.CuratorSkillVersion,
		"contract-version":             systemskills.CuratorContractVersion,
		"activation-policy":            systemskills.CuratorActivationPolicy,
		"coordination-boundary":        systemskills.CuratorCoordinationBoundary,
		"instruction-only":             true,
		"best-effort-coordination":     true,
		"requires-application-service": true,
	} {
		if got := curator.Frontmatter[key]; got != want {
			t.Errorf("frontmatter[%q] = %#v, want %#v", key, got, want)
		}
	}
	tools, ok := curator.Frontmatter["compatible-tools"].([]string)
	if !ok || !reflect.DeepEqual(tools, systemskills.CuratorMetadata().CompatibleTools) {
		t.Fatalf("compatible tools = %#v", curator.Frontmatter["compatible-tools"])
	}

	lookedUp, lookupSnapshot, err := service.LookupSkills(t.Context(), root, []string{systemskills.CuratorSkillID, systemskills.CuratorSkillID})
	if err != nil || lookupSnapshot != snapshot || len(lookedUp) != 1 || lookedUp[systemskills.CuratorSkillID].URI != curator.URI {
		t.Fatalf("lookup = %#v, snapshot %q, err %v", lookedUp, lookupSnapshot, err)
	}
	got, err := service.GetSkill(t.Context(), root, curator.URI)
	if err != nil || !reflect.DeepEqual(got, curator) {
		t.Fatalf("get = %#v, err %v", got, err)
	}
	content, err := service.ReadResource(t.Context(), root, curator.URI)
	if err != nil {
		t.Fatal(err)
	}
	if content.MIMEType != "text/markdown" || !bytes.Equal(content.Bytes, []byte(systemskills.CuratorSkill)) {
		t.Fatalf("read = MIME %q, %d bytes", content.MIMEType, len(content.Bytes))
	}

	tamperedURI := strings.Replace(curator.URI, "/system-curator/", "/system-curator-tampered/", 1)
	if _, err := service.GetSkill(t.Context(), root, tamperedURI); !errors.Is(err, skill.ErrNotFound) {
		t.Fatalf("tampered identity error = %v", err)
	}
	badDigestURI := strings.Replace(curator.URI, strings.TrimPrefix(curator.Version, "sha256:"), strings.Repeat("0", 64), 1)
	if _, err := service.ReadResource(t.Context(), root, badDigestURI); !errors.Is(err, skill.ErrSnapshotExpired) {
		t.Fatalf("expired digest error = %v", err)
	}
}

func TestDistributionRejectsWorkspaceSystemCuratorCollisionAndKeepsBundleAvailable(t *testing.T) {
	root := newSkillWorkspace(t)
	service := DistributionService{}
	before, _, err := service.ListSkills(t.Context(), root)
	if err != nil || len(before) != 1 {
		t.Fatalf("initial list = %#v, %v", before, err)
	}
	original := before[0]

	createActiveDistributionSkill(t, root, "workspace-skill", "Workspace Skill")
	_, err = (SkillService{}).PreviewCreate(t.Context(), root, skill.CreateInput{
		ID: systemskills.CuratorSkillID, Collection: "core", Name: "Workspace Curator Shadow", Description: "Attempts to shadow the bundled curator.",
		Routing: skill.RoutingInput{Operations: []string{"review"}, Triggers: []string{"shadow bundled curator"}, NotFor: []string{"ordinary work"}, MinScope: "multi_step"},
	}, false)
	if err == nil || !strings.Contains(err.Error(), `skill id "system-curator" is reserved for the bundled system skill; choose a different workspace skill id`) {
		t.Fatalf("workspace curator collision error = %v", err)
	}

	first, _, err := service.ListSkills(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := service.ListSkills(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("non-deterministic lists:\nfirst  %#v\nsecond %#v", first, second)
	}
	if len(first) != 2 || first[0].SkillID != systemskills.CuratorSkillID || first[1].SkillID != "workspace-skill" {
		t.Fatalf("unexpected distribution after rejected collision: %#v", first)
	}
	if !reflect.DeepEqual(first[0], original) {
		t.Fatalf("workspace mutation changed bundled curator:\nbefore %#v\nafter  %#v", original, first[0])
	}
	content, err := service.ReadResource(t.Context(), root, original.URI)
	if err != nil || !bytes.Equal(content.Bytes, []byte(systemskills.CuratorSkill)) {
		t.Fatalf("read original curator after workspace mutation = %#v, %v", content, err)
	}
}

func createActiveDistributionSkill(t *testing.T, root, id, name string) {
	t.Helper()
	service := SkillService{}
	content := []byte("---\nname: " + id + "\ndescription: Mutable workspace fixture.\n---\n\n# " + name + "\n")
	preview, err := service.PreviewCreate(t.Context(), root, skill.CreateInput{
		ID: id, Collection: "core", Name: name, Description: "Mutable workspace fixture.", Content: content,
		Routing: skill.RoutingInput{Operations: []string{"review"}, Triggers: []string{"mutable workspace fixture"}, NotFor: []string{"system maintenance"}, MinScope: "multi_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ConfirmSkillMutation(t.Context(), root, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil || result.Error != nil {
		t.Fatalf("create %s = %#v, %v", id, result, err)
	}
	activation, err := service.PreviewActivate(t.Context(), root, id, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err = service.ConfirmSkillMutation(t.Context(), root, activation, activation.Confirmation.Confirmation.Pins)
	if err != nil || result.Error != nil {
		t.Fatalf("activate %s = %#v, %v", id, result, err)
	}
}
