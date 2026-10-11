package skill

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

const resourceSkillDirPath = "skills/core/res-skill"

type resourceFixture struct {
	root string
	dir  string
}

func (f resourceFixture) path(relative string) string { return f.dir + "/" + relative }

func (f resourceFixture) abs(relative string) string {
	return filepath.Join(f.root, filepath.FromSlash(f.path(relative)))
}

func digestOf(contents []byte) string {
	sum := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// newResourceFixture creates an active skill with a mix of companion files and
// a second skill, then rebuilds the catalog so every file is a manifest row.
func newResourceFixture(t *testing.T) resourceFixture {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	manager := Manager{}
	for _, id := range []string{"res-skill", "other"} {
		created, err := manager.PreviewCreate(t.Context(), root, CreateInput{
			ID: id, Collection: "core", Name: "Resource Skill " + id, Description: "Resource editing fixture.",
			Content: []byte("# Resource Skill\n\nReal instructions.\n"),
			Routing: RoutingInput{Triggers: []string{"resource " + id}, NotFor: []string{"none"}, MinScope: "single_step"},
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Confirm(t.Context(), root, created); err != nil {
			t.Fatal(err)
		}
	}
	activate, err := manager.PreviewTransition(t.Context(), root, "res-skill", "active", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Confirm(t.Context(), root, activate); err != nil {
		t.Fatal(err)
	}
	fixture := resourceFixture{root: root, dir: resourceSkillDirPath}
	big := bytes.Repeat([]byte("0123456789abcde\n"), (3<<20)/2/16)
	files := map[string][]byte{
		"scripts/run.sh":       []byte("#!/bin/sh\necho one\necho two\n"),
		"references/guide.md":  []byte("# Guide\n\nFirst line.\n"),
		"references/win.txt":   []byte("alpha\r\nbeta\r\n"),
		"references/bom.txt":   []byte("\xef\xbb\xbfwith bom\n"),
		"references/link.md":   []byte("# Link target\n"),
		"docs/nested/note.md":  []byte("# Note\n"),
		"assets/logo.png":      []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
		"references/large.txt": big,
	}
	for relative, contents := range files {
		target := fixture.abs(relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, contents, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "skills/core/other/notes.md"), []byte("# Other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(fixture.abs("scripts/run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func TestListResourceFilesClassifiesCompanionFiles(t *testing.T) {
	t.Parallel()
	f := newResourceFixture(t)
	files, err := ListResourceFiles(t.Context(), f.root, "res-skill")
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]ResourceFile{}
	for _, file := range files {
		if file.Content != nil {
			t.Fatalf("list returned content for %s", file.Path)
		}
		byPath[file.Path] = file
	}
	for _, excluded := range []string{f.path("SKILL.md"), f.path(".meta/skill.yaml")} {
		if _, ok := byPath[excluded]; ok {
			t.Fatalf("%s must not be listed", excluded)
		}
	}
	cases := []struct {
		relative, reason, lineEnding, kind string
		editable, text                     bool
	}{
		{"scripts/run.sh", "", LineEndingLF, "script", true, true},
		{"references/guide.md", "", LineEndingLF, "reference", true, true},
		{"references/win.txt", "", LineEndingCRLF, "reference", true, true},
		{"assets/logo.png", ResourceReasonBinary, LineEndingNone, "asset", false, false},
		{"references/large.txt", ResourceReasonTooLarge, LineEndingLF, "reference", false, true},
	}
	for _, tc := range cases {
		file, ok := byPath[f.path(tc.relative)]
		if !ok {
			t.Fatalf("%s missing from list", tc.relative)
		}
		if file.Reason != tc.reason || file.Editable != tc.editable || file.IsText != tc.text || file.LineEnding != tc.lineEnding || file.Kind != tc.kind {
			t.Fatalf("%s classified as %+v", tc.relative, file)
		}
		onDisk, err := os.ReadFile(f.abs(tc.relative))
		if err != nil {
			t.Fatal(err)
		}
		if file.Digest != digestOf(onDisk) || file.SizeBytes != int64(len(onDisk)) {
			t.Fatalf("%s digest/size = %s/%d", tc.relative, file.Digest, file.SizeBytes)
		}
	}
	if _, err := ListResourceFiles(t.Context(), f.root, "missing-skill"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown skill error = %v, want ErrNotFound", err)
	}
}

func TestReadResourceFileReturnsTextAndWithholdsBinary(t *testing.T) {
	t.Parallel()
	f := newResourceFixture(t)
	guide, err := ReadResourceFile(t.Context(), f.root, "res-skill", f.path("references/guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(guide.Content) != "# Guide\n\nFirst line.\n" || guide.Truncated || !guide.Editable {
		t.Fatalf("guide = %+v", guide)
	}
	logo, err := ReadResourceFile(t.Context(), f.root, "res-skill", f.path("assets/logo.png"))
	if err != nil {
		t.Fatal(err)
	}
	if logo.Content != nil || logo.Reason != ResourceReasonBinary || logo.Digest == "" || logo.SizeBytes == 0 {
		t.Fatalf("binary file = %+v", logo)
	}
	large, err := ReadResourceFile(t.Context(), f.root, "res-skill", f.path("references/large.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !large.Truncated || len(large.Content) != MaxEditableResourceBytes || large.SizeBytes <= MaxEditableResourceBytes || large.Editable {
		t.Fatalf("large file truncated=%v len=%d size=%d editable=%v", large.Truncated, len(large.Content), large.SizeBytes, large.Editable)
	}
}

func TestTrimPartialRuneKeepsWholeRunes(t *testing.T) {
	t.Parallel()
	euro := []byte("€") // three bytes
	if got := trimPartialRune(append([]byte("ab"), euro[:2]...)); string(got) != "ab" {
		t.Fatalf("partial rune kept: %q", got)
	}
	if got := trimPartialRune(append([]byte("ab"), euro...)); string(got) != "ab€" {
		t.Fatalf("whole rune dropped: %q", got)
	}
}

func TestResourcePathsOutsideTheAllowListAreRejected(t *testing.T) {
	t.Parallel()
	f := newResourceFixture(t)
	invalid := map[string]string{
		"empty":            "",
		"traversal":        f.path("../other/notes.md"),
		"dot segment":      f.path("./scripts/run.sh"),
		"empty segment":    f.dir + "//scripts/run.sh",
		"absolute":         "/" + f.path("scripts/run.sh"),
		"backslash":        f.dir + `\scripts\run.sh`,
		"nul":              f.path("scripts/run.sh\x00"),
		"another skill":    "skills/core/other/notes.md",
		"skill entrypoint": f.path("SKILL.md"),
		"hub metadata":     f.path(".meta/skill.yaml"),
	}
	for name, path := range invalid {
		if _, err := ReadResourceFile(t.Context(), f.root, "res-skill", path); !errors.Is(err, ErrInvalidResourceEdit) {
			t.Errorf("%s: read error = %v, want ErrInvalidResourceEdit", name, err)
		}
		_, err := Manager{}.PreviewResourceUpdate(t.Context(), f.root, "res-skill", ResourceUpdateInput{
			Path: path, Content: []byte("x\n"), ExpectedDigest: digestOf(nil),
		}, false)
		if !errors.Is(err, ErrInvalidResourceEdit) {
			t.Errorf("%s: preview error = %v, want ErrInvalidResourceEdit", name, err)
		}
	}
	// A stale catalog is rebuilt on open, so an unlisted path is one that does
	// not exist in the skill folder at all.
	if _, err := ReadResourceFile(t.Context(), f.root, "res-skill", f.path("scripts/absent.sh")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlisted file error = %v, want ErrNotFound", err)
	}
}

func TestSymlinkedResourcesAreNotRegular(t *testing.T) {
	t.Parallel()
	f := newResourceFixture(t)
	manifest, err := GetManifestAnyState(t.Context(), f.root, "res-skill")
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]Resource{}
	for _, row := range manifest.Resources {
		rows[row.Path] = row
	}
	// The catalog refuses symlinks, so they are introduced after the rebuild.
	if err := os.Remove(f.abs("references/link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(f.abs("references/guide.md"), f.abs("references/link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(f.abs("docs"), f.abs("docs-real")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("docs-real", f.abs("docs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.abs("scripts/run.sh")); err != nil {
		t.Fatal(err)
	}
	handle, err := os.OpenRoot(f.root)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	for relative, reason := range map[string]string{
		"references/link.md":  ResourceReasonNotRegular,
		"docs/nested/note.md": ResourceReasonNotRegular,
		"scripts/run.sh":      ResourceReasonMissing,
	} {
		// The per-segment check guards the window between the manifest read
		// and the file read, so it is exercised on the manifest rows directly.
		file, err := classifyResource(handle, f.dir, rows[f.path(relative)])
		if err != nil {
			t.Fatal(err)
		}
		if file.Reason != reason || file.Editable || file.Content != nil || file.Digest != "" {
			t.Fatalf("%s = %+v, want reason %s", relative, file, reason)
		}
		// Through the public surface a symlink anywhere in the skills tree
		// makes the workspace refuse the request before any file is touched.
		if read, err := ReadResourceFile(t.Context(), f.root, "res-skill", f.path(relative)); err == nil {
			t.Fatalf("%s read succeeded: %+v", relative, read)
		}
		_, err = Manager{}.PreviewResourceUpdate(t.Context(), f.root, "res-skill", ResourceUpdateInput{
			Path: f.path(relative), Content: []byte("changed\n"), ExpectedDigest: digestOf([]byte("# Note\n")),
		}, false)
		if err == nil {
			t.Fatalf("%s preview succeeded", relative)
		}
	}
	target, err := os.ReadFile(f.abs("references/guide.md"))
	if err != nil || string(target) != "# Guide\n\nFirst line.\n" {
		t.Fatalf("symlink target changed: %q %v", target, err)
	}
}

func TestPreviewResourceUpdateEditsOneScript(t *testing.T) {
	t.Parallel()
	f := newResourceFixture(t)
	manager := Manager{}
	metadataBefore, err := os.ReadFile(f.abs(".meta/skill.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	current, err := ReadResourceFile(t.Context(), f.root, "res-skill", f.path("scripts/run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	updated := []byte("#!/bin/sh\necho one\necho three\n")
	proposal, err := manager.PreviewResourceUpdate(t.Context(), f.root, "res-skill", ResourceUpdateInput{
		Path: current.Path, Content: updated, ExpectedDigest: current.Digest,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Command != "skill_edit" || len(proposal.Summary.Modified) != 1 || proposal.Summary.Modified[0] != current.Path || len(proposal.Summary.Added) != 0 {
		t.Fatalf("proposal command=%s summary=%+v", proposal.Command, proposal.Summary)
	}
	if proposal.RoutingImpact != nil {
		t.Fatalf("resource edit reported routing impact: %+v", proposal.RoutingImpact)
	}
	for _, want := range []string{"--- a/" + current.Path, "+++ b/" + current.Path, "-echo two", "+echo three"} {
		if !strings.Contains(proposal.FullDiff, want) {
			t.Fatalf("full diff lacks %q:\n%s", want, proposal.FullDiff)
		}
	}
	if _, err := manager.Confirm(t.Context(), f.root, proposal); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(f.abs("scripts/run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, updated) {
		t.Fatalf("saved bytes = %q", onDisk)
	}
	replay, err := manager.PreviewResourceUpdate(t.Context(), f.root, "res-skill", ResourceUpdateInput{
		Path: current.Path, Content: updated, ExpectedDigest: current.Digest,
	}, true)
	if err != nil || replay.AlreadyApplied() == nil {
		t.Fatalf("replayed request was not recognised as applied: %+v %v", replay, err)
	}
	info, err := os.Stat(f.abs("scripts/run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no executable bit, so the saved file reports a fixed mode there.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, want 0755", info.Mode().Perm())
	}
	metadataAfter, err := os.ReadFile(f.abs(".meta/skill.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(metadataBefore, metadataAfter) {
		t.Fatal("metadata file was rewritten")
	}
	manifest, err := GetManifestAnyState(t.Context(), f.root, "res-skill")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "active" {
		t.Fatalf("status after edit = %s, want active", manifest.Status)
	}
	for _, row := range manifest.Resources {
		if row.Path == current.Path && row.Digest != digestOf(updated) {
			t.Fatalf("catalog digest not republished: %s", row.Digest)
		}
	}
}

func TestPreviewResourceUpdatePreservesLineEndingsAndBOM(t *testing.T) {
	t.Parallel()
	f := newResourceFixture(t)
	manager := Manager{}
	cases := []struct {
		relative string
		sent     string
		want     string
	}{
		{"references/win.txt", "alpha\nbeta\ngamma\n", "alpha\r\nbeta\r\ngamma\r\n"},
		{"references/bom.txt", "\ufeffwith bom, edited\n", "\xef\xbb\xbfwith bom, edited\n"},
	}
	for _, tc := range cases {
		current, err := ReadResourceFile(t.Context(), f.root, "res-skill", f.path(tc.relative))
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := manager.PreviewResourceUpdate(t.Context(), f.root, "res-skill", ResourceUpdateInput{
			Path: current.Path, Content: []byte(tc.sent), ExpectedDigest: current.Digest,
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Confirm(t.Context(), f.root, proposal); err != nil {
			t.Fatal(err)
		}
		onDisk, err := os.ReadFile(f.abs(tc.relative))
		if err != nil {
			t.Fatal(err)
		}
		if string(onDisk) != tc.want {
			t.Fatalf("%s saved as %q, want %q", tc.relative, onDisk, tc.want)
		}
	}
}

func TestPreviewResourceUpdateDetectsConcurrentChanges(t *testing.T) {
	t.Parallel()
	f := newResourceFixture(t)
	manager := Manager{}
	current, err := ReadResourceFile(t.Context(), f.root, "res-skill", f.path("references/guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	stale := digestOf([]byte("something else"))
	_, err = manager.PreviewResourceUpdate(t.Context(), f.root, "res-skill", ResourceUpdateInput{
		Path: current.Path, Content: []byte("# Mine\n"), ExpectedDigest: stale,
	}, false)
	var conflict *EditConflictError
	if !errors.As(err, &conflict) || conflict.ExpectedDigest != stale || conflict.ActualDigest != current.Digest || conflict.Path != current.Path {
		t.Fatalf("stale digest error = %v", err)
	}

	proposal, err := manager.PreviewResourceUpdate(t.Context(), f.root, "res-skill", ResourceUpdateInput{
		Path: current.Path, Content: []byte("# Mine\n"), ExpectedDigest: current.Digest,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	theirs := []byte("# Theirs\n")
	if err := os.WriteFile(f.abs("references/guide.md"), theirs, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Confirm(t.Context(), f.root, proposal); !errors.Is(err, mutation.ErrConflict) && !errors.Is(err, ErrSnapshotExpired) {
		t.Fatalf("confirm after concurrent change error = %v", err)
	}
	onDisk, err := os.ReadFile(f.abs("references/guide.md"))
	if err != nil || !bytes.Equal(onDisk, theirs) {
		t.Fatalf("concurrent bytes overwritten: %q %v", onDisk, err)
	}
}

func TestPreviewResourceUpdateRejectsUnusableContent(t *testing.T) {
	t.Parallel()
	f := newResourceFixture(t)
	current, err := ReadResourceFile(t.Context(), f.root, "res-skill", f.path("references/guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]ResourceUpdateInput{
		"binary":           {Content: []byte("text\x00more"), ExpectedDigest: current.Digest},
		"invalid utf-8":    {Content: []byte{0xff, 0xfe, '\n'}, ExpectedDigest: current.Digest},
		"too large":        {Content: bytes.Repeat([]byte("a"), MaxEditableResourceBytes+1), ExpectedDigest: current.Digest},
		"unchanged":        {Content: current.Content, ExpectedDigest: current.Digest},
		"missing digest":   {Content: []byte("# New\n")},
		"malformed digest": {Content: []byte("# New\n"), ExpectedDigest: "sha256:XYZ"},
	}
	for name, input := range cases {
		input.Path = current.Path
		if _, err := (Manager{}).PreviewResourceUpdate(t.Context(), f.root, "res-skill", input, false); !errors.Is(err, ErrInvalidResourceEdit) {
			t.Errorf("%s: error = %v, want ErrInvalidResourceEdit", name, err)
		}
	}
	large, err := ReadResourceFile(t.Context(), f.root, "res-skill", f.path("references/large.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Manager{}).PreviewResourceUpdate(t.Context(), f.root, "res-skill", ResourceUpdateInput{
		Path: large.Path, Content: []byte("small\n"), ExpectedDigest: large.Digest,
	}, false); !errors.Is(err, ErrInvalidResourceEdit) {
		t.Fatalf("too-large current file error = %v, want ErrInvalidResourceEdit", err)
	}
}

func TestLineEndingOf(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{
		"":           LineEndingNone,
		"one line":   LineEndingNone,
		"a\nb\n":     LineEndingLF,
		"a\r\nb\r\n": LineEndingCRLF,
		"a\r\nb\n":   LineEndingMixed,
	} {
		if got := lineEndingOf([]byte(input)); got != want {
			t.Errorf("lineEndingOf(%q) = %s, want %s", input, got, want)
		}
	}
}
