package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/skill"
	"github.com/vantt/skillhub-mcp/internal/systemskills"
)

func newSkillCoreWorkspace(t *testing.T) string {
	t.Helper()
	root := newResolverWorkspace(t)
	skills := []struct{ id, status, extra, body string }{
		{"core-active", "active", "  examples: [review the retry loop]\n  counter_examples: [write a poem]\nquality:\n  reviewed: true\n  routing_review_rationale: Checks retry handling only.\n", "# Active\n\nUse the references.\n"},
		{"core-plain", "active", "quality:\n  reviewed: true\n", "# Plain\n"},
		{"core-draft", "draft", "", "# Draft\n"},
		{"core-archived", "archived", "", "# Archived\n"},
	}
	for _, item := range skills {
		meta := fmt.Sprintf("schema_version: 1\nid: %s\nname: %s\nstatus: %s\ndescription: Core fixture %s.\nrouting:\n  triggers: [review %s]\n  operations: [review]\n  not_for: [write prose]\n  min_scope: multi_step\n", item.id, item.id, item.status, item.id, item.id)
		writeResolveBenchmarkFile(t, root, "skills/core/"+item.id+"/skill.meta.yaml", meta+item.extra)
		writeResolveBenchmarkFile(t, root, "skills/core/"+item.id+"/SKILL.md", fmt.Sprintf("---\nname: %s\ndescription: Core fixture %s.\n---\n\n%s", item.id, item.id, item.body))
	}
	writeResolveBenchmarkFile(t, root, "skills/core/core-active/references/checks.md", "# Checks\n")
	writeResolveBenchmarkFile(t, root, "skills/core/core-active/scripts/run.sh", "#!/bin/sh\necho ok\n")
	if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	return root
}

// The runtime core carries every kept field of the curation read model with
// the same value, in every lifecycle state.
func TestReadSkillCoreMatchesTheSeparateReads(t *testing.T) {
	t.Parallel()
	root := newSkillCoreWorkspace(t)
	service := SkillService{}
	for _, id := range []string{"core-active", "core-plain", "core-draft", "core-archived"} {
		t.Run(id, func(t *testing.T) {
			read, err := service.ReadSkill(t.Context(), root, id)
			if err != nil {
				t.Fatal(err)
			}
			routing, err := service.ReadSkillRouting(t.Context(), root, id)
			if err != nil {
				t.Fatal(err)
			}
			rationale, err := service.ReadSkillRationale(t.Context(), root, id)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256([]byte(read.Content))
			entrypoint := ""
			for _, resource := range read.Manifest.Resources {
				if strings.HasSuffix(resource.Path, "/SKILL.md") {
					entrypoint = resource.Path
					break
				}
			}
			want := SkillCore{
				SkillID: read.Manifest.SkillID, Name: read.Manifest.Name, Description: read.Manifest.Description,
				Status: read.Manifest.Status, Path: entrypoint, CatalogSnapshot: read.Manifest.CatalogSnapshot,
				Content: read.Content, ContentDigest: "sha256:" + hex.EncodeToString(sum[:]),
				LifecycleState: read.Manifest.Status, Routing: routing, Rationale: rationale, Resources: read.Manifest.Resources,
			}
			got, err := service.ReadSkillCore(t.Context(), root, id)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("core differs from the separate reads\n got: %#v\nwant: %#v", got, want)
			}
			detail, err := service.GetSkillDetail(t.Context(), root, id)
			if err != nil {
				t.Fatal(err)
			}
			if detail.Content != got.Content || detail.ContentDigest != got.ContentDigest || detail.Path != got.Path ||
				!reflect.DeepEqual(detail.Routing, got.Routing) || detail.Rationale != got.Rationale || !reflect.DeepEqual(detail.Resources, got.Resources) {
				t.Fatalf("curation detail differs from the core\ndetail: %#v\n  core: %#v", detail, got)
			}
		})
	}
}

func TestReadSkillCoreReportsMissingAndInvalidSkills(t *testing.T) {
	t.Parallel()
	root := newSkillCoreWorkspace(t)
	service := SkillService{}
	// The bundled curator is never a workspace skill, so it reads as missing.
	for _, id := range []string{"core-missing", systemskills.CuratorSkillID} {
		if _, err := service.ReadSkillCore(t.Context(), root, id); !errors.Is(err, skill.ErrNotFound) {
			t.Fatalf("%s: core error = %v, want not found", id, err)
		}
		if _, err := service.ReadSkill(t.Context(), root, id); !errors.Is(err, skill.ErrNotFound) {
			t.Fatalf("%s: separate read error = %v, want not found", id, err)
		}
	}
	_, coreErr := service.ReadSkillCore(t.Context(), root, "Not/Valid")
	_, readErr := service.ReadSkill(t.Context(), root, "Not/Valid")
	if coreErr == nil || readErr == nil || coreErr.Error() != readErr.Error() {
		t.Fatalf("invalid id errors differ: core %v, separate %v", coreErr, readErr)
	}
}
