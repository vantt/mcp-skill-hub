package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/skill"
)

const resourceSkillDir = "skills/software/consumer-review"

func newSkillResourceWorkspace(t *testing.T) (string, SkillService) {
	t.Helper()
	root := newSkillWorkspace(t)
	service := SkillService{}
	createAndActivateSkill(t, service, root)
	for relative, contents := range map[string]string{
		"references/checks.md": "# Checks\n\nRun tests.\n",
		"assets/logo.bin":      "\x89PNG\r\n\x1a\n\x00\x00",
	} {
		target := filepath.Join(root, filepath.FromSlash(resourceSkillDir+"/"+relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	return root, service
}

func requireAppErrorCode(t *testing.T, err error, code ErrorCode) ErrorRender {
	t.Helper()
	var appErr *Error
	if !errors.As(err, &appErr) || appErr.Code != code {
		t.Fatalf("error = %#v, want app error %s", err, code)
	}
	return appErr.Render
}

func resourceUpdate(path, content, expectedDigest string) skill.ResourceUpdateInput {
	return skill.ResourceUpdateInput{Path: path, Content: []byte(content), ExpectedDigest: expectedDigest}
}

func TestListSkillResourcesClassifiesCompanionFiles(t *testing.T) {
	t.Parallel()
	root, service := newSkillResourceWorkspace(t)
	list, err := service.ListSkillResources(t.Context(), root, "consumer-review")
	if err != nil {
		t.Fatal(err)
	}
	if list.SkillID != "consumer-review" {
		t.Fatalf("skill id = %q", list.SkillID)
	}
	rows := map[string]SkillResourceItem{}
	for _, item := range list.Items {
		if strings.HasSuffix(item.Path, "/SKILL.md") || strings.Contains(item.Path, "/.meta/") {
			t.Fatalf("list includes %s", item.Path)
		}
		rows[item.Path] = item
	}
	checks := rows[resourceSkillDir+"/references/checks.md"]
	if !checks.Editable || checks.Reason != "" || checks.SizeBytes == 0 || !strings.HasPrefix(checks.Digest, "sha256:") {
		t.Fatalf("checks.md = %+v", checks)
	}
	logo := rows[resourceSkillDir+"/assets/logo.bin"]
	if logo.Editable || logo.Reason != skill.ResourceReasonBinary {
		t.Fatalf("logo.bin = %+v", logo)
	}
	if _, err := service.ListSkillResources(t.Context(), root, "missing-skill"); !errors.Is(err, skill.ErrNotFound) {
		t.Fatalf("unknown skill error = %v, want ErrNotFound", err)
	}
}

func TestReadSkillResourceReturnsTextOnly(t *testing.T) {
	t.Parallel()
	root, service := newSkillResourceWorkspace(t)
	text, err := service.ReadSkillResource(t.Context(), root, "consumer-review", resourceSkillDir+"/references/checks.md")
	if err != nil {
		t.Fatal(err)
	}
	if text.Content == nil || *text.Content != "# Checks\n\nRun tests.\n" || text.LineEnding != skill.LineEndingLF || !text.Editable || text.Truncated {
		t.Fatalf("text resource = %+v", text)
	}
	binary, err := service.ReadSkillResource(t.Context(), root, "consumer-review", resourceSkillDir+"/assets/logo.bin")
	if err != nil {
		t.Fatal(err)
	}
	if binary.Content != nil || binary.Reason != skill.ResourceReasonBinary || binary.Digest == "" {
		t.Fatalf("binary resource = %+v", binary)
	}
	for _, path := range []string{resourceSkillDir + "/SKILL.md", resourceSkillDir + "/../consumer-review/SKILL.md", "/etc/passwd"} {
		_, err := service.ReadSkillResource(t.Context(), root, "consumer-review", path)
		render := requireAppErrorCode(t, err, ErrorInvalidRequest)
		if render.Why == "" || strings.HasPrefix(render.Why, skill.ErrInvalidResourceEdit.Error()) {
			t.Fatalf("%s WHY = %q", path, render.Why)
		}
	}
	if _, err := service.ReadSkillResource(t.Context(), root, "consumer-review", resourceSkillDir+"/references/absent.md"); !errors.Is(err, skill.ErrNotFound) {
		t.Fatalf("unlisted path error = %v, want ErrNotFound", err)
	}
}

func TestPreviewSkillResourceUpdateConfirmsThroughSkillProposal(t *testing.T) {
	t.Parallel()
	root, service := newSkillResourceWorkspace(t)
	path := resourceSkillDir + "/references/checks.md"
	current, err := service.ReadSkillResource(t.Context(), root, "consumer-review", path)
	if err != nil {
		t.Fatal(err)
	}
	updated := "# Checks\n\nRun all tests.\n"
	preview, err := service.PreviewSkillResourceUpdate(t.Context(), root, "consumer-review", resourceUpdate(path, updated, current.Digest))
	if err != nil {
		t.Fatal(err)
	}
	if preview.Command != "skill_edit" || preview.Summary != "Resource edit is ready for review." || !strings.Contains(preview.FullDiff, "+Run all tests.") {
		t.Fatalf("preview = %+v", preview)
	}
	if onDisk, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))); string(onDisk) != *current.Content {
		t.Fatalf("preview wrote the file: %q", onDisk)
	}
	pins := preview.Confirmation.Confirmation.Pins
	loaded, err := service.LoadSkillProposal(t.Context(), root, pins.ProposalID)
	if err != nil || loaded.Error != nil {
		t.Fatalf("load proposal = %+v, %v", loaded, err)
	}
	result, err := service.ConfirmSkillMutation(t.Context(), root, loaded, pins)
	if err != nil || result.Error != nil {
		t.Fatalf("confirm = %+v, %v", result, err)
	}
	if result.State != "active" {
		t.Fatalf("state after resource edit = %q, want active", result.State)
	}
	if onDisk, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))); string(onDisk) != updated {
		t.Fatalf("file after confirm = %q", onDisk)
	}
}

func TestPreviewSkillResourceUpdateRefusals(t *testing.T) {
	t.Parallel()
	root, service := newSkillResourceWorkspace(t)
	path := resourceSkillDir + "/references/checks.md"
	current, err := service.ReadSkillResource(t.Context(), root, "consumer-review", path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.PreviewSkillResourceUpdate(t.Context(), root, "consumer-review", resourceUpdate(path, *current.Content, current.Digest))
	requireAppErrorCode(t, err, ErrorInvalidRequest)

	_, err = service.PreviewSkillResourceUpdate(t.Context(), root, "consumer-review", resourceUpdate(path, "bad\x00byte", current.Digest))
	requireAppErrorCode(t, err, ErrorInvalidRequest)

	_, err = service.PreviewSkillResourceUpdate(t.Context(), root, "consumer-review", resourceUpdate(resourceSkillDir+"/.meta/skill.yaml", "id: x\n", current.Digest))
	requireAppErrorCode(t, err, ErrorInvalidRequest)

	_, err = service.PreviewSkillResourceUpdate(t.Context(), root, "missing-skill", resourceUpdate(path, "x\n", current.Digest))
	if !errors.Is(err, skill.ErrNotFound) {
		t.Fatalf("unknown skill error = %v, want ErrNotFound", err)
	}

	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte("# Changed elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = service.PreviewSkillResourceUpdate(t.Context(), root, "consumer-review", resourceUpdate(path, "# Mine\n", current.Digest))
	conflict := requireAppErrorCode(t, err, ErrorEditConflict)
	if !strings.Contains(conflict.Why, path) || strings.Contains(conflict.Why, "SKILL.md") || conflict.Fix != "Reload the file, then apply your edit again." {
		t.Fatalf("conflict render = %+v", conflict)
	}
}

func TestSkillResourceSymlinkReportsCatalogProblem(t *testing.T) {
	t.Parallel()
	root, service := newSkillResourceWorkspace(t)
	link := filepath.Join(root, filepath.FromSlash(resourceSkillDir+"/references/link.md"))
	if err := os.Symlink(filepath.Join(root, filepath.FromSlash(resourceSkillDir+"/references/checks.md")), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := service.ListSkillResources(t.Context(), root, "consumer-review")
	render := requireAppErrorCode(t, err, ErrorSnapshotExpired)
	if !strings.Contains(render.Why, "symlink") {
		t.Fatalf("symlink render = %+v", render)
	}
	_, err = service.ReadSkillResource(t.Context(), root, "consumer-review", resourceSkillDir+"/references/link.md")
	requireAppErrorCode(t, err, ErrorSnapshotExpired)
}
