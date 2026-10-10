package hostintegration_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

func TestDoctorReportsNativeCuratorVersionSkew(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "workspace")
	service := app.WorkspaceService{}
	if _, err := service.Init(root, true); err != nil {
		t.Fatal(err)
	}
	for _, adapter := range hostintegration.SupportedAdapters() {
		path := filepath.Join(root, filepath.FromSlash(adapter.SkillRelativePath))
		stale := []byte("---\nname: system-curator\n---\nPrevious curator instructions.\n")
		if err := os.WriteFile(path, stale, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	result, err := service.Doctor(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != app.StatusActionRequired {
		t.Fatalf("doctor status: %s", result.Status)
	}
	for _, adapter := range hostintegration.SupportedAdapters() {
		path := filepath.Join(root, filepath.FromSlash(adapter.SkillRelativePath))
		found := false
		for _, item := range result.Items {
			if item.ID == "host_"+string(adapter.Host)+"_native-skill" && strings.Contains(item.Summary, path) && strings.Contains(item.Summary, "sha256:") {
				found = true
			}
		}
		if !found {
			t.Fatalf("doctor omitted version skew for %s: %#v", adapter.Host, result.Items)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(content, []byte(systemskills.CuratorBundle().Instructions)) {
			t.Fatal("doctor silently overwrote stale curator")
		}
	}
	command := "skillhub doctor --fix --workspace " + root + " --yes"
	found := false
	for _, action := range result.SuggestedActions {
		if action.Command == command && action.RequiresConfirmation {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing confirmed fix command %q: %#v", command, result.SuggestedActions)
	}
	if _, err := service.DoctorFix(root, true); err != nil {
		t.Fatal(err)
	}
	for _, adapter := range hostintegration.SupportedAdapters() {
		path := filepath.Join(root, filepath.FromSlash(adapter.SkillRelativePath))
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(content, []byte(systemskills.CuratorBundle().Instructions)) {
			t.Fatalf("fix did not restore embedded curator for %s", adapter.Host)
		}
	}
}
