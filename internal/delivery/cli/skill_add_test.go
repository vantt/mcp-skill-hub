package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
)

// These tests add from a file:// repository through source adapters that accept
// one, and replace package-level terminal stand-ins, so none of them runs in
// parallel.

const pdfSkillMD = "---\nname: pdf\ndescription: Fill and extract PDF forms. Use when the user asks to fill a pdf form. Do not use when the user wants to edit a word document.\n%s---\n\n# PDF\n\nFill the form fields with the values the user gives.\n"

// addFixtureFile is one file of a fixture repository.
type addFixtureFile struct {
	path, body string
	executable bool
}

// addFixtureRepo commits the files to a new repository and returns its file:// URL.
func addFixtureRepo(t *testing.T, files ...addFixtureFile) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		mode := os.FileMode(0o644)
		if file.executable {
			mode = 0o755
		}
		writeCLIAddFixture(t, dir, file.path, file.body, mode)
	}
	tree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Add("."); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Commit("initial", &git.CommitOptions{Author: &object.Signature{Name: "Tester", Email: "tester@example.com", When: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	return localFileURL(dir)
}

// commitFixtureFile adds a commit to a fixture repository.
func commitFixtureFile(t *testing.T, url, rel, body string) {
	t.Helper()
	dir := localPathOfFileURL(url)
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeCLIAddFixture(t, dir, rel, body, 0o644)
	tree, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Add("."); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Commit("change", &git.CommitOptions{Author: &object.Signature{Name: "Tester", Email: "tester@example.com", When: time.Now()}}); err != nil {
		t.Fatal(err)
	}
}

// useFileSourceAdapters lets skill add read file:// repositories for one test.
func useFileSourceAdapters(t *testing.T) {
	t.Helper()
	saved := skillAddAdapters
	t.Cleanup(func() { skillAddAdapters = saved })
	skillAddAdapters = func(root string) map[string]sourcepkg.Adapter {
		return map[string]sourcepkg.Adapter{"git": sourcepkg.GitRepositoryAdapter{CacheRoot: filepath.Join(root, "runtime", "sources", "git"), AllowFileProtocol: true}}
	}
}

// pdfFixtureRepo is a repository with one skill whose folder holds a script, a
// license file and a secret that must stay out.
func pdfFixtureRepo(t *testing.T, frontmatterExtra string) string {
	return addFixtureRepo(t,
		addFixtureFile{path: "skills/pdf/SKILL.md", body: strings.Replace(pdfSkillMD, "%s", frontmatterExtra, 1)},
		addFixtureFile{path: "skills/pdf/forms.md", body: "How to fill a form.\n"},
		addFixtureFile{path: "skills/pdf/scripts/fill.py", body: "print('fill')\n", executable: true},
		addFixtureFile{path: "skills/pdf/LICENSE.txt", body: "All rights reserved.\n"},
		addFixtureFile{path: "skills/pdf/.env", body: "TOKEN=abc\n"},
	)
}

func TestSkillAddRefusesAFolderAndPointsToCommit(t *testing.T) {
	hub := newNextStepHub(t)
	dir := filepath.Join(t.TempDir(), "team-release")
	writeCLIAddFixture(t, dir, "SKILL.md", "---\nname: team-release\ndescription: Release the team.\n---\n\n# Release\n", 0o644)
	before := hubHead(t, hub.root)

	code, stdout, stderr := hub.run("skill", "add", dir)
	if code != 2 || stdout != "" {
		t.Fatalf("exit = %d, stdout = %q", code, stdout)
	}
	if !strings.Contains(flatText(stderr), "skill add takes a GitHub URL.") || !strings.Contains(flatText(stderr), "skillhub skill commit "+dir) {
		t.Fatalf("refusal must name the replacement:\n%s", stderr)
	}
	code, stdout, _ = hub.run("skill", "add", dir, "--json")
	var refused app.Result
	if code != 2 || json.Unmarshal([]byte(stdout), &refused) != nil || refused.Error == nil || refused.Error.Code != app.ErrorInvalidRequest || !strings.Contains(refused.Error.Render.Fix, "skillhub skill commit") {
		t.Fatalf("json refusal: exit = %d\n%s", code, stdout)
	}
	if code, _, stderr = hub.run("skill", "add", "file:///tmp/some/repo.git"); code != 2 || !strings.Contains(stderr, "skillhub skill commit") {
		t.Fatalf("a file:// URL is a folder: exit = %d\n%s", code, stderr)
	}
	if hubHead(t, hub.root) != before || !hubIsClean(t, hub.root) {
		t.Fatalf("a refused add must not change the hub")
	}
}

func TestSkillAddPreviewShowsEverythingAPersonNeedsToDecide(t *testing.T) {
	useFileSourceAdapters(t)
	hub := newNextStepHub(t)
	repo := pdfFixtureRepo(t, "license: Proprietary terms apply\n")
	before := hubHead(t, hub.root)

	code, stdout, stderr := hub.run("skill", "add", repo, "--skill", "pdf")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	out := flatText(stdout)
	for _, want := range []string{
		"Add pdf from ", "tracked for upstream updates.",
		"Files (4,", "SKILL.md", "forms.md", "scripts/fill.py", "LICENSE.txt",
		"Left out: skills/pdf/.env (looks like a secret)",
		"License: Proprietary terms apply (LICENSE.txt)", "may have proprietary terms",
		"Routing, suggested from the description (edit with --edit-routing):", "trigger", "min scope",
		"Routing lint:",
		"Third-party: agents get pdf only after you approve its content.",
		"Nothing written yet.", "Show every file: skillhub skill add ", " --diff", "Apply: skillhub skill add ", " --yes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("preview lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(out, "TOKEN=abc") || strings.Contains(out, "Fill the form fields") {
		t.Errorf("the preview lists files; --diff shows content:\n%s", stdout)
	}
	if hubHead(t, hub.root) != before || !hubIsClean(t, hub.root) {
		t.Fatalf("a preview must not change the hub")
	}

	code, stdout, stderr = hub.run("skill", "add", repo, "--skill", "pdf", "--diff")
	if code != 0 || !strings.Contains(stdout, "Fill the form fields with the values the user gives.") || !strings.Contains(stdout, "How to fill a form.") || strings.Contains(stdout, "[added") || strings.Contains(stdout, "Show every file:") {
		t.Fatalf("--diff must print every file's content as an added file: exit = %d\n%s\n%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "TOKEN=abc") {
		t.Fatalf("a left-out file must stay out of the diff:\n%s", stdout)
	}
}

func TestSkillAddPreviewSaysWhenNoLicenseIsDeclared(t *testing.T) {
	useFileSourceAdapters(t)
	hub := newNextStepHub(t)
	repo := addFixtureRepo(t, addFixtureFile{path: "SKILL.md", body: strings.Replace(pdfSkillMD, "%s", "", 1)})
	code, stdout, stderr := hub.run("skill", "add", repo)
	if code != 0 || !strings.Contains(flatText(stdout), "License: not declared") || !strings.Contains(flatText(stdout), "License is unknown") {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
}

func TestSkillAddAtATerminalChainsAddApproveAndActivateIntoOneCommit(t *testing.T) {
	useFileSourceAdapters(t)
	hub := newNextStepHub(t)
	repo := pdfFixtureRepo(t, "license: MIT\n")
	before := hubHead(t, hub.root)

	atTerminal(t, "y\ny\ny\n")
	code, stdout, stderr := hub.run("skill", "add", repo, "--skill", "pdf")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	for _, want := range []string{"Add pdf as a draft? [y/N] ", "Approve this content for agents? [y/N] ", "Activate pdf now? [y/N] ", "Added pdf, approved its content and activated it.", "Committed ", ", approve content, activate"} {
		if !strings.Contains(flatText(stdout), want) {
			t.Fatalf("output lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "Apply this change?") || strings.Contains(stdout, "Nothing written yet.") {
		t.Fatalf("a preview with questions must not print the two-step hints:\n%s", stdout)
	}
	if gitOutput(t, hub.root, "rev-list", "--count", before+"..HEAD") != "1" || !hubIsClean(t, hub.root) {
		t.Fatalf("add, approve and activate must be one commit and leave the hub clean:\n%s", stdout)
	}
	meta := readFileText(t, filepath.Join(hub.root, "skills", "default", "pdf", ".meta", "skill.yaml"))
	if !strings.Contains(meta, "status: active") || !strings.Contains(meta, "content_reviewed_digest: sha256:") || !strings.Contains(meta, "kind: github") || !strings.Contains(meta, "triggers:") {
		t.Fatalf("the skill must be active, approved, tracked and routed:\n%s", meta)
	}
	// File modes are not meaningful on Windows; elsewhere the bit must survive.
	if info, err := os.Stat(filepath.Join(hub.root, "skills", "default", "pdf", "scripts", "fill.py")); err != nil || (runtime.GOOS != "windows" && info.Mode()&0o111 == 0) {
		t.Fatalf("the executable bit must survive the add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hub.root, "skills", "default", "pdf", ".env")); err == nil {
		t.Fatalf("a secret must stay out of the hub")
	}
	code, stdout, stderr = hub.run("skill", "review", "pdf", "--json")
	var review struct {
		ContentTrust struct {
			ThirdParty bool `json:"third_party"`
			Approved   bool `json:"approved"`
		} `json:"content_trust"`
	}
	if code != 0 || json.Unmarshal([]byte(stdout), &review) != nil || !review.ContentTrust.ThirdParty || !review.ContentTrust.Approved {
		t.Fatalf("the approval must match the content on the hub: exit = %d\n%s\n%s", code, stdout, stderr)
	}
}

func TestSkillAddEachNoStopsTheChainAndCommitsWhatWasAccepted(t *testing.T) {
	useFileSourceAdapters(t)
	for _, tc := range []struct {
		name, answers string
		asked         []string
		subject       string
		status        string
		approved      bool
	}{
		{name: "no approval", answers: "y\nn\n", asked: []string{"Add pdf as a draft?", "Approve this content for agents?"}, subject: `"Add pdf from `, status: "status: draft"},
		{name: "no activation", answers: "y\ny\nn\n", asked: []string{"Approve this content for agents?", "Activate pdf now?"}, subject: ", approve content\"", status: "status: draft", approved: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hub := newNextStepHub(t)
			repo := pdfFixtureRepo(t, "license: MIT\n")
			before := hubHead(t, hub.root)
			atTerminal(t, tc.answers)
			code, stdout, stderr := hub.run("skill", "add", repo, "--skill", "pdf")
			if code != 0 {
				t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
			}
			for _, want := range tc.asked {
				if !strings.Contains(flatText(stdout), want) {
					t.Fatalf("output lacks %q:\n%s", want, stdout)
				}
			}
			if !strings.Contains(flatText(stdout), tc.subject) {
				t.Fatalf("commit subject lacks %q:\n%s", tc.subject, stdout)
			}
			if tc.approved != strings.Contains(flatText(stdout), "Activate pdf now?") {
				t.Fatalf("activation is asked only after approval:\n%s", stdout)
			}
			if gitOutput(t, hub.root, "rev-list", "--count", before+"..HEAD") != "1" || !hubIsClean(t, hub.root) {
				t.Fatalf("what was accepted must be one commit:\n%s", stdout)
			}
			meta := readFileText(t, filepath.Join(hub.root, "skills", "default", "pdf", ".meta", "skill.yaml"))
			if !strings.Contains(meta, tc.status) || strings.Contains(meta, "content_reviewed_digest") != tc.approved {
				t.Fatalf("wrong state (approved=%v):\n%s", tc.approved, meta)
			}
		})
	}
}

func TestSkillAddDecliningTheFirstQuestionWritesNothing(t *testing.T) {
	useFileSourceAdapters(t)
	hub := newNextStepHub(t)
	repo := pdfFixtureRepo(t, "license: MIT\n")
	before := hubHead(t, hub.root)
	atTerminal(t, "\n")
	code, stdout, _ := hub.run("skill", "add", repo, "--skill", "pdf")
	if code != 0 || !strings.Contains(stdout, "Nothing written.") || hubHead(t, hub.root) != before || !hubIsClean(t, hub.root) {
		t.Fatalf("exit = %d\n%s", code, stdout)
	}
}

func TestSkillAddWithoutATerminalAddsTheDraftAndNeverApproves(t *testing.T) {
	useFileSourceAdapters(t)
	hub := newNextStepHub(t)
	repo := pdfFixtureRepo(t, "license: MIT\n")
	before := hubHead(t, hub.root)

	code, stdout, stderr := hub.run("skill", "add", repo, "--skill", "pdf", "--yes", "--trigger", "fill a pdf form", "--not-for", "edit a word document", "--min-scope", "single_step")
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	out := flatText(stdout)
	for _, want := range []string{"Draft pdf added. Agent use: off.", "Committed ", "Next: skillhub skill review pdf", "A person approves its content"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(out, "[y/N]") || strings.Contains(out, "approve content") {
		t.Errorf("no prompt and no approval without a terminal:\n%s", stdout)
	}
	if gitOutput(t, hub.root, "rev-list", "--count", before+"..HEAD") != "1" || !hubIsClean(t, hub.root) {
		t.Fatalf("the draft must be one commit")
	}
	meta := readFileText(t, filepath.Join(hub.root, "skills", "default", "pdf", ".meta", "skill.yaml"))
	for _, want := range []string{"status: draft", "fill a pdf form", "edit a word document", "min_scope: single_step"} {
		if !strings.Contains(meta, want) {
			t.Errorf("meta lacks %q:\n%s", want, meta)
		}
	}
	if strings.Contains(meta, "content_reviewed_digest") {
		t.Errorf("an unattended add must not approve content:\n%s", meta)
	}

	hub2 := newNextStepHub(t)
	code, _, stderr = hub2.run("skill", "add", repo, "--skill", "pdf", "--yes", "--approve-content", "sha256:"+strings.Repeat("a", 64))
	if code != 2 || !strings.Contains(stderr, "only a person approves") || !hubIsClean(t, hub2.root) {
		t.Fatalf("--approve-content must be refused: exit = %d\n%s", code, stderr)
	}
}

func TestSkillAddEditRoutingOpensTheSuggestionsInTheEditor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	useFileSourceAdapters(t)
	hub := newNextStepHub(t)
	repo := pdfFixtureRepo(t, "license: MIT\n")
	editor := filepath.Join(t.TempDir(), "editor.sh")
	script := "#!/bin/sh\ncat > \"$1\" <<'EOF'\ntriggers:\n  - fill a pdf form\nnot_for:\n  - editing a word document\nmin_scope: multi_step\nEOF\n"
	if err := os.WriteFile(editor, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", editor)
	atTerminal(t, "y\nn\n")
	code, stdout, stderr := hub.run("skill", "add", repo, "--skill", "pdf", "--edit-routing")
	if code != 0 || !strings.Contains(stdout, `"fill a pdf form"`) {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	meta := readFileText(t, filepath.Join(hub.root, "skills", "default", "pdf", ".meta", "skill.yaml"))
	if !strings.Contains(meta, "fill a pdf form") || !strings.Contains(meta, "multi_step") {
		t.Fatalf("edited routing was not saved:\n%s", meta)
	}
	if code, _, stderr := hub.run("skill", "add", repo, "--skill", "pdf", "--edit-routing", "--yes"); code != 2 || !strings.Contains(stderr, "--edit-routing") {
		t.Fatalf("--edit-routing needs a terminal: exit = %d stderr=%s", code, stderr)
	}
}

func TestSkillAddSeveralSkillsAreDraftsOnly(t *testing.T) {
	useFileSourceAdapters(t)
	hub := newNextStepHub(t)
	repo := addFixtureRepo(t,
		addFixtureFile{path: "tool-a/SKILL.md", body: "---\nname: tool-a\ndescription: Tool A\n---\n\n# Tool A\n"},
		addFixtureFile{path: "tool-b/SKILL.md", body: "---\nname: tool-b\ndescription: Tool B\n---\n\n# Tool B\n"},
	)
	code, _, stderr := hub.run("skill", "add", repo)
	if code != 2 || !strings.Contains(stderr, "explicit selection is required") {
		t.Fatalf("exit = %d: %s", code, stderr)
	}
	before := hubHead(t, hub.root)
	atTerminal(t, "y\n")
	code, stdout, stderr := hub.run("skill", "add", repo, "--all")
	if code != 0 || !strings.Contains(stdout, "Add tool-a, tool-b as drafts? [y/N] ") || strings.Contains(stdout, "Approve this content") {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	for _, id := range []string{"tool-a", "tool-b"} {
		if !strings.Contains(readFileText(t, filepath.Join(hub.root, "skills", "default", id, ".meta", "skill.yaml")), "status: draft") {
			t.Fatalf("%s must be a draft", id)
		}
	}
	if gitOutput(t, hub.root, "rev-list", "--count", before+"..HEAD") != "1" {
		t.Fatalf("both drafts must be one commit")
	}
}

func TestSkillAddAgainExplainsNothingChanged(t *testing.T) {
	useFileSourceAdapters(t)
	hub := newNextStepHub(t)
	repo := pdfFixtureRepo(t, "license: MIT\n")
	if code, _, stderr := hub.run("skill", "add", repo, "--skill", "pdf", "--yes"); code != 0 {
		t.Fatalf("first add exit %d: %s", code, stderr)
	}
	for _, args := range [][]string{{"skill", "add", repo, "--skill", "pdf"}, {"skill", "add", repo, "--skill", "pdf", "--yes"}} {
		code, stdout, stderr := hub.run(args...)
		if code != 0 {
			t.Fatalf("%v exit %d: %s", args, code, stderr)
		}
		out := flatText(stdout)
		if !strings.Contains(out, "already added") || !strings.Contains(out, "skillhub skill review pdf") || strings.Contains(out, "Draft pdf added") || strings.Contains(out, "skill confirm") {
			t.Errorf("%v: want an already-added message with a review step:\n%s", args, stdout)
		}
	}
}

func TestSkillAddErrorPreviewKeepsItsOwnCauseUnderYes(t *testing.T) {
	hub := newNextStepHub(t)
	for _, args := range [][]string{{"skill", "add", "https://gitlab.com/owner/repo"}, {"skill", "add", "https://gitlab.com/owner/repo", "--yes"}} {
		code, _, stderr := hub.run(args...)
		if code != 2 {
			t.Errorf("%v exit = %d, want 2", args, code)
		}
		text := flatText(stderr)
		if !strings.Contains(text, `unsupported host "gitlab.com"; only public github.com is supported`) || !strings.Contains(text, "supported hosts: github.com") || strings.Contains(strings.ToLower(stderr), "stale") {
			t.Errorf("%v stderr should carry the real cause:\n%s", args, stderr)
		}
	}
}

func TestSkillAddShortConfirmDispatched(t *testing.T) {
	useFileSourceAdapters(t)
	hub := newNextStepHub(t)
	repo := addFixtureRepo(t, addFixtureFile{path: "SKILL.md", body: "---\nname: short-confirm\ndescription: Short confirm test.\n---\n\n# Short Confirm\n"})

	// Preview with JSON to get the proposal ID
	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "add", repo, "--workspace", hub.root, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("preview exit %d: %s", code, stderr.String())
	}
	var preview app.SkillAddProposal
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatalf("unmarshal preview: %v", err)
	}
	proposalID := preview.Confirmation.Confirmation.Pins.ProposalID
	if proposalID == "" {
		t.Fatal("missing proposal ID in preview JSON")
	}

	// Confirm with the short form: skillhub skill confirm <proposal_id>
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"skill", "confirm", proposalID, "--workspace", hub.root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("short confirm exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Draft short-confirm added.") {
		t.Errorf("confirm output missing draft added summary: %s", stdout.String())
	}
}

func TestSkillAddPreviewAndResultRenderUpstream(t *testing.T) {
	var buf bytes.Buffer
	p := termui.NewWithWidth(&buf, termui.MaxWidth)

	// 1. Preview with new upstream source
	preview := app.SkillAddProposal{
		SkillID: "pdf",
		Origin: app.SkillOrigin{
			Kind:       "github",
			Repository: "https://github.com/anthropics/skills",
			Path:       "skills/pdf",
			Commit:     "3f9c2a1b4d5e",
		},
		Resources:  []app.DiscoveredCompanion{{Path: "f1"}, {Path: "f2"}},
		TotalBytes: 49152,
		Confirmation: app.ConfirmationPolicy{
			Confirmation: app.ConfirmationRequirement{
				Pins: app.ConfirmationPins{ProposalID: "PROP-12345"},
			},
		},
		UpstreamSource: &app.UpstreamSourceRef{
			SourceID: "anthropics-skills",
			Created:  true,
		},
	}
	writeSkillAddPreview(p, preview, previewHints{rerun: "skillhub skill add x"})
	out := buf.String()
	wantPreviewLine := "Upstream: tracked by new source anthropics-skills (checked weekly by `skillhub source check`; nothing runs in the background)."
	if !strings.Contains(flatText(out), wantPreviewLine) {
		t.Fatalf("preview output missing upstream line %q, got:\n%s", wantPreviewLine, out)
	}
	if strings.Contains(out, "Watching: off") {
		t.Fatalf("preview output should not contain 'Watching: off', got:\n%s", out)
	}

	// 2. Preview with existing upstream source
	buf.Reset()
	preview.UpstreamSource.Created = false
	writeSkillAddPreview(p, preview, previewHints{rerun: "skillhub skill add x"})
	outExisting := buf.String()
	wantExistingLine := "Upstream: tracked by source anthropics-skills."
	if !strings.Contains(outExisting, wantExistingLine) {
		t.Fatalf("preview output missing existing upstream line %q, got:\n%s", wantExistingLine, outExisting)
	}

	// 3. Result with upstream source
	buf.Reset()
	result := app.SkillAddResult{
		SkillID: "pdf",
		UpstreamSource: &app.UpstreamSourceRef{
			SourceID: "anthropics-skills",
		},
	}
	writeSkillAddResult(p, false, result)
	outResult := buf.String()
	wantResultLine := "Draft pdf added. Agent use: off. Upstream: anthropics-skills. Changes are not committed."
	if !strings.Contains(outResult, wantResultLine) {
		t.Fatalf("result output missing upstream line %q, got:\n%s", wantResultLine, outResult)
	}
	if strings.Contains(outResult, "Watching: off") {
		t.Fatalf("result output should not contain 'Watching: off', got:\n%s", outResult)
	}

	// 4. Result without upstream source (local add)
	buf.Reset()
	resultLocal := app.SkillAddResult{
		SkillID: "pdf",
	}
	writeSkillAddResult(p, false, resultLocal)
	outLocal := buf.String()
	wantLocalLine := "Draft pdf added. Agent use: off. Changes are not committed."
	if !strings.Contains(outLocal, wantLocalLine) {
		t.Fatalf("local result output missing expected line %q, got:\n%s", wantLocalLine, outLocal)
	}
	if strings.Contains(outLocal, "Watching: off") {
		t.Fatalf("local result output should not contain 'Watching: off', got:\n%s", outLocal)
	}
}

func writeCLIAddFixture(t *testing.T, dir, rel, body string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// An approved draft that lacks routing cannot be activated, so the next step
// names the missing fields with the edit command instead of `skill activate`.
func TestSkillAddNamesTheMissingRoutingInsteadOfAnActivateThatWouldFail(t *testing.T) {
	useFileSourceAdapters(t)
	hub := newNextStepHub(t)
	repo := addFixtureRepo(t, addFixtureFile{path: "SKILL.md", body: "---\nname: pdf\ndescription: Fill and extract PDF forms.\n---\n\n# PDF\n\nFill the form fields with the values the user gives.\n"})
	atTerminal(t, "y\ny\n")
	code, stdout, stderr := hub.run("skill", "add", repo)
	if code != 0 {
		t.Fatalf("exit = %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	out := flatText(stdout)
	if strings.Contains(out, "skillhub skill activate pdf") {
		t.Fatalf("the next step offers an activation that would fail:\n%s", stdout)
	}
	for _, want := range []string{"skillhub skill edit pdf --trigger ", " --not-for ", " --example "} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, stdout)
		}
	}
}
