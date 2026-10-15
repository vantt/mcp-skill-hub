package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/vantt/skillhub-mcp/internal/skillruntime"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

const skillMDOf = "---\nname: %s\ndescription: Does %s things. Use when you need %s.\n---\n\n# %s\n\n%s\n"

func submittedTree(id, body string, extra map[string]string) map[string]treeFile {
	tree := map[string]treeFile{"SKILL.md": {Data: []byte(fmt.Sprintf(skillMDOf, id, id, id, id, body))}}
	for rel, data := range extra {
		tree[rel] = treeFile{Data: []byte(data)}
	}
	return tree
}

func submitTree(t *testing.T, hub, id, base, label string, tree map[string]treeFile) Submission {
	t.Helper()
	archive, err := BuildSubmissionBundle(id, tree, skillruntime.BundleSubmission{BaseVersion: base, ClientLabel: label, Excluded: []skillruntime.BundleExcluded{{Path: ".env", Reason: "looks like a secret"}}})
	if err != nil {
		t.Fatal(err)
	}
	submission, err := AddSubmission(hub, bytes.NewReader(archive), time.Now())
	if err != nil {
		t.Fatalf("AddSubmission: %v", err)
	}
	return submission
}

func TestAddSubmissionQueuesFilesAndLeavesTheHubTreeAlone(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	head := hubGit(t, hub, "rev-parse", "HEAD")
	tree := submittedTree("team-release", "bump the version", map[string]string{"scripts/bump.sh": "#!/bin/sh\n"})
	script := tree["scripts/bump.sh"]
	script.Executable = true
	tree["scripts/bump.sh"] = script

	submission := submitTree(t, hub, "team-release", "", "machine-a", tree)
	if !strings.HasPrefix(submission.ID, "SUB-") || len(submission.ID) != 30 || submission.SkillID != "team-release" || submission.ClientLabel != "machine-a" || len(submission.Files) != 2 {
		t.Fatalf("submission = %+v", submission)
	}
	if hubGit(t, hub, "status", "--porcelain") != "" || hubGit(t, hub, "rev-parse", "HEAD") != head {
		t.Fatalf("the hub's Git changed")
	}
	if _, err := os.Stat(filepath.Join(hub, "skills", "default", "team-release")); !os.IsNotExist(err) {
		t.Fatalf("a queued submission created a skill folder")
	}
	read, err := ReadSubmission(hub, submission.ID)
	if err != nil || read.Excluded[0].Path != ".env" || read.Excluded[0].Reason != "looks like a secret" || !read.Files[1].Executable {
		t.Fatalf("read = %+v, %v", read, err)
	}
	listed, err := ListSubmissions(hub)
	if err != nil || len(listed) != 1 || listed[0].ID != submission.ID {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	staged, _ := os.ReadDir(filepath.Join(hub, "runtime", "submissions", submissionStaging))
	if len(staged) != 0 {
		t.Fatalf("staging left %d entries", len(staged))
	}
}

func TestSubmissionsOrderByArrivalAndReplaceSameSkillFromSameMachine(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	first := submitTree(t, hub, "alpha", "", "machine-a", submittedTree("alpha", "one", nil))
	other := submitTree(t, hub, "alpha", "", "machine-b", submittedTree("alpha", "two", nil))
	replacing := submitTree(t, hub, "alpha", "", "machine-a", submittedTree("alpha", "three", nil))
	listed, err := ListSubmissions(hub)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].ID != other.ID || listed[1].ID != replacing.ID {
		t.Fatalf("list = %+v (first %s was replaced)", listed, first.ID)
	}
	if _, err := ReadSubmission(hub, first.ID); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("replaced submission still readable: %v", err)
	}
}

func TestSubmissionKeepsOneShortLineOfTheClientLabel(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	// A macOS CI runner's host name: longer than the 64 characters a hub keeps.
	long := "sjc22-bm211-fd82d2e3-5478-46f3-b781-843a1dd8779e-C66A5E8302EF.local"
	if got, want := submitTree(t, hub, "alpha", "", long, submittedTree("alpha", "x", nil)).ClientLabel, "sjc22-bm211-fd82d2e3-5478-46f3-b781-843a1dd8779e-C66A5E8302EF"; got != want {
		t.Fatalf("label = %q, want the whole leading part %q", got, want)
	}
	if got := submitTree(t, hub, "beta", "", " machine\tb\n ", submittedTree("beta", "x", nil)).ClientLabel; got != "machineb" {
		t.Fatalf("label = %q, want control characters dropped", got)
	}
	if got := submitTree(t, hub, "gamma", "", "  ", submittedTree("gamma", "x", nil)).ClientLabel; got != "unknown machine" {
		t.Fatalf("label = %q, want unknown machine", got)
	}
}

func TestCleanClientLabelNeverCutsInsideADotSeparatedPart(t *testing.T) {
	t.Parallel()
	ciHost := "sjc22-bm211-fd82d2e3-5478-46f3-b781-843a1dd8779e-C66A5E8302EF.local"
	for _, tc := range []struct{ name, in, want string }{
		{"short name", "machine-a", "machine-a"},
		{"short name with dots", "build.example.com", "build.example.com"},
		{"exactly the cap", strings.Repeat("a", 64), strings.Repeat("a", 64)},
		{"ci host with suffix", ciHost, "sjc22-bm211-fd82d2e3-5478-46f3-b781-843a1dd8779e-C66A5E8302EF"},
		{"ci host without suffix", strings.TrimSuffix(ciHost, ".local"), strings.TrimSuffix(ciHost, ".local")},
		{"several whole parts", strings.Repeat("a", 30) + "." + strings.Repeat("b", 30) + "." + strings.Repeat("c", 30), strings.Repeat("a", 30) + "." + strings.Repeat("b", 30)},
		{"first part too long", strings.Repeat("x", 70) + ".local", strings.Repeat("x", 63) + "…"},
		{"no dots too long", strings.Repeat("x", 70), strings.Repeat("x", 63) + "…"},
		{"empty first part", "." + strings.Repeat("x", 70), strings.Repeat("x", 63) + "…"},
		{"leading dot before whole parts", ".host." + strings.Repeat("x", 70), "host"},
		{"only dots", strings.Repeat(".", 70), "unknown machine"},
		{"multibyte runes", strings.Repeat("é", 40) + "." + strings.Repeat("ü", 40), strings.Repeat("é", 40)},
		{"multibyte first part too long", strings.Repeat("日", 70), strings.Repeat("日", 63) + "…"},
		{"control characters dropped first", " machine\tb\n ", "machineb"},
		{"empty", "  ", "unknown machine"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanClientLabel(tc.in)
			if got != tc.want {
				t.Fatalf("cleanClientLabel(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if n := utf8.RuneCountInString(got); n > maxClientLabelRunes {
				t.Fatalf("cleanClientLabel(%q) has %d runes, over the cap", tc.in, n)
			}
		})
	}
}

func TestSubmissionQueueCaps(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	for index := 0; index < MaxWaitingSubmissions; index++ {
		submitTree(t, hub, "alpha", "", fmt.Sprintf("machine-%02d", index), submittedTree("alpha", "x", nil))
	}
	archive, err := BuildSubmissionBundle("alpha", submittedTree("alpha", "x", nil), skillruntime.BundleSubmission{ClientLabel: "one-too-many"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddSubmission(hub, bytes.NewReader(archive), time.Now()); !errors.Is(err, ErrSubmissionQueueFull) {
		t.Fatalf("51st submission: %v", err)
	}
	// The same skill from a machine that is already waiting replaces its own.
	archive, _ = BuildSubmissionBundle("alpha", submittedTree("alpha", "y", nil), skillruntime.BundleSubmission{ClientLabel: "machine-00"})
	if _, err := AddSubmission(hub, bytes.NewReader(archive), time.Now()); err != nil {
		t.Fatalf("a replacement must fit under the cap: %v", err)
	}
}

func TestSubmissionQueueSizeCap(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	stage := filepath.Join(submissionsRoot(hub), "stage")
	if err := os.MkdirAll(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	huge := Submission{ID: newSubmissionID(time.Now()), SkillID: "big", ClientLabel: "machine-z", SubmittedAt: "2026-10-14T10:00:00Z", TotalBytes: MaxSubmissionQueueBytes - 10, Files: []SubmissionFile{}}
	if err := writeSubmissionRecord(stage, huge); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stage, filepath.Join(submissionsRoot(hub), huge.ID)); err != nil {
		t.Fatal(err)
	}
	archive, _ := BuildSubmissionBundle("alpha", submittedTree("alpha", "x", nil), skillruntime.BundleSubmission{ClientLabel: "machine-a"})
	if _, err := AddSubmission(hub, bytes.NewReader(archive), time.Now()); !errors.Is(err, ErrSubmissionQueueFull) {
		t.Fatalf("over the byte cap: %v", err)
	}
}

func TestAddSubmissionRefusesManagedAndExcludedFiles(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	for _, rel := range []string{".meta/skill.yaml", "skill.meta.yaml", CheckoutMarkerName, ".env"} {
		archive, err := BuildSubmissionBundle("alpha", submittedTree("alpha", "x", map[string]string{rel: "x"}), skillruntime.BundleSubmission{ClientLabel: "m"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := AddSubmission(hub, bytes.NewReader(archive), time.Now()); !errors.Is(err, ErrSubmissionInvalid) {
			t.Errorf("%s: err = %v", rel, err)
		}
	}
	if listed, _ := ListSubmissions(hub); len(listed) != 0 {
		t.Fatalf("refused submissions were queued: %+v", listed)
	}
}

func TestDiscardSubmission(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	submission := submitTree(t, hub, "alpha", "", "machine-a", submittedTree("alpha", "x", nil))
	if err := DiscardSubmission(hub, submission.ID); err != nil {
		t.Fatal(err)
	}
	if err := DiscardSubmission(hub, submission.ID); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("second discard: %v", err)
	}
	if _, err := ReadSubmission(hub, "../../etc"); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("path-like id: %v", err)
	}
}

func approveNewSubmission(t *testing.T, hub string, submission Submission) SkillCommitResult {
	t.Helper()
	service := SkillCommitService{}
	preview, err := service.PreviewSubmission(context.Background(), hub, submission.ID, SkillCommitInput{})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview: %v %+v", err, preview.Error)
	}
	result, err := service.ApplySubmission(commitCtx(), hub, preview, SkillCommitApplyOptions{Approve: true})
	if err != nil || result.Error != nil {
		t.Fatalf("apply: %v %+v", err, result.Error)
	}
	return result
}

func TestNewSubmissionIsReviewedAndLandsAsAnApprovedDraftInOneCommit(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	submission := submitTree(t, hub, "team-release", "", "machine-a", submittedTree("team-release", "bump", map[string]string{"scripts/bump.sh": "echo 1\n"}))

	service := SkillCommitService{}
	preview, err := service.PreviewSubmission(context.Background(), hub, submission.ID, SkillCommitInput{})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview: %v %+v", err, preview.Error)
	}
	if preview.Case != commitCaseNew || preview.Trust == nil || !preview.Trust.ThirdParty || preview.Trust.Origin != "submitted from machine-a" || len(preview.Files) != 2 {
		t.Fatalf("preview = %+v", preview)
	}
	if len(preview.Excluded) != 1 || preview.Excluded[0].Path != ".env" || preview.Checkout != nil {
		t.Fatalf("excluded/checkout = %+v / %+v", preview.Excluded, preview.Checkout)
	}
	if _, err := service.ApplySubmission(commitCtx(), hub, preview, SkillCommitApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	refused, _ := service.ApplySubmission(commitCtx(), hub, preview, SkillCommitApplyOptions{})
	if refused.Error == nil || refused.Error.Code != ErrorContentReviewRequired {
		t.Fatalf("applying without approval = %+v", refused.Error)
	}
	if _, err := os.Stat(filepath.Join(hub, "skills", "default", "team-release")); !os.IsNotExist(err) {
		t.Fatalf("a refused apply wrote the skill")
	}

	head := hubGit(t, hub, "rev-parse", "HEAD")
	result := approveNewSubmission(t, hub, submission)
	if !result.Approved || result.Activated || result.Commit == nil || result.Commit.Status != workspace.CommitCommitted || result.Checkout != nil {
		t.Fatalf("result = %+v", result)
	}
	if hubGit(t, hub, "rev-list", "--count", head+"..HEAD") != "1" {
		t.Fatalf("expected exactly one new hub commit")
	}
	if subject := hubGit(t, hub, "log", "-1", "--format=%s"); subject != "Add team-release (submitted from machine-a), approve content" {
		t.Fatalf("subject = %q", subject)
	}
	hubSkill, _ := loadHubSkill(hub, "team-release")
	meta := parseSkillReviewMeta(hubSkill.meta)
	if meta.Status != "draft" {
		t.Fatalf("status = %q", meta.Status)
	}
	trust, err := (SkillService{}).ContentTrustFor(context.Background(), hub, "team-release")
	if err != nil || !trust.Approved {
		t.Fatalf("trust = %+v, %v", trust, err)
	}
	if _, err := ReadSubmission(hub, submission.ID); !errors.Is(err, ErrSubmissionNotFound) {
		t.Fatalf("an applied submission is still queued: %v", err)
	}
	if _, err := os.Stat(SubmissionFilesDir(hub, submission.ID)); !os.IsNotExist(err) {
		t.Fatalf("submission files remain")
	}
}

func TestSubmissionOfACheckoutDiffsAgainstItsBaseAndKeepsHubExecutableBits(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	seedHubSkill(t, hub, fixtureSkill{id: "review-checklist", files: map[string]string{"scripts/lint.sh": "#!/bin/sh\necho a\n", "notes.md": "keep\n"}, executable: []string{"scripts/lint.sh"}})
	commitHubAll(t, hub, "add review-checklist")
	base, err := loadHubSkill(hub, "review-checklist")
	if err != nil || base == nil {
		t.Fatal(err)
	}
	tree := copyTree(base.tree)
	tree["notes.md"] = treeFile{Data: []byte("keep\nand more\n")}
	tree["scripts/lint.sh"] = treeFile{Data: []byte("#!/bin/sh\necho b\n")}
	submission := submitTree(t, hub, "review-checklist", base.version, "machine-a", tree)

	preview, err := (SkillCommitService{}).PreviewSubmission(context.Background(), hub, submission.ID, SkillCommitInput{})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview: %v %+v", err, preview.Error)
	}
	if preview.Case != commitCaseUpdate || len(preview.Files) != 2 || preview.Trust == nil || !preview.Trust.ThirdParty {
		t.Fatalf("preview = case %s files %+v trust %+v", preview.Case, preview.Files, preview.Trust)
	}
	if _, err := (SkillCommitService{}).ApplySubmission(commitCtx(), hub, preview, SkillCommitApplyOptions{Approve: true}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(hub, "skills", "default", "review-checklist", "scripts", "lint.sh"))
	// File modes are not meaningful on Windows; elsewhere the hub's bit must survive.
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0) {
		t.Fatalf("lint.sh lost its execute bit: %v %v", info, err)
	}
	data, _ := os.ReadFile(filepath.Join(hub, "skills", "default", "review-checklist", "notes.md"))
	if string(data) != "keep\nand more\n" {
		t.Fatalf("notes.md = %q", data)
	}
	if subject := hubGit(t, hub, "log", "-1", "--format=%s"); !strings.Contains(subject, "submitted from machine-a") || !strings.Contains(subject, "approve content") {
		t.Fatalf("subject = %q", subject)
	}
}

func TestSubmissionWhenTheHubChangedMergesAndSurfacesConflicts(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	seedHubSkill(t, hub, fixtureSkill{id: "alpha", files: map[string]string{"a.md": "a\n", "b.md": "b\n"}})
	commitHubAll(t, hub, "add alpha")
	base, _ := loadHubSkill(hub, "alpha")
	replaceInHub(t, hub, "alpha", "line two", "line TWO")

	tree := copyTree(base.tree)
	tree["a.md"] = treeFile{Data: []byte("a changed by the submitter\n")}
	submission := submitTree(t, hub, "alpha", base.version, "machine-a", tree)
	preview, err := (SkillCommitService{}).PreviewSubmission(context.Background(), hub, submission.ID, SkillCommitInput{})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview: %v %+v", err, preview.Error)
	}
	if preview.Case != commitCaseMerge || len(preview.Conflicts) != 0 || preview.ProjectChanged != 1 || preview.HubChanged != 1 {
		t.Fatalf("merge preview = case %s conflicts %v project %d hub %d", preview.Case, preview.Conflicts, preview.ProjectChanged, preview.HubChanged)
	}

	// Both sides change SKILL.md differently: it needs a decision.
	conflicting := copyTree(base.tree)
	conflicting["SKILL.md"] = treeFile{Data: bytes.Replace(base.tree["SKILL.md"].Data, []byte("line two"), []byte("line 2 by submitter"), 1)}
	other := submitTree(t, hub, "alpha", base.version, "machine-b", conflicting)
	preview, err = (SkillCommitService{}).PreviewSubmission(context.Background(), hub, other.ID, SkillCommitInput{})
	if err != nil || preview.Error != nil || len(preview.Conflicts) != 1 || preview.Conflicts[0] != "SKILL.md" {
		t.Fatalf("conflict preview = %v %+v conflicts %v", err, preview.Error, preview.Conflicts)
	}
	refused, _ := (SkillCommitService{}).ApplySubmission(commitCtx(), hub, preview, SkillCommitApplyOptions{Approve: true})
	if refused.Error == nil {
		t.Fatalf("a submission with open conflicts was applied")
	}
	decided, err := (SkillCommitService{}).PreviewSubmission(context.Background(), hub, other.ID, SkillCommitInput{Resolutions: []SkillCommitResolution{{Path: "SKILL.md", Choice: "hub"}}})
	if err != nil || decided.Error != nil || len(decided.Conflicts) != 0 {
		t.Fatalf("decided preview = %v %+v", err, decided.Error)
	}
}

func TestSubmissionWithoutACheckoutReplacesTheHubCopy(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	seedHubSkill(t, hub, fixtureSkill{id: "alpha"})
	commitHubAll(t, hub, "add alpha")
	submission := submitTree(t, hub, "alpha", "", "machine-a", submittedTree("alpha", "a rewrite", nil))
	preview, err := (SkillCommitService{}).PreviewSubmission(context.Background(), hub, submission.ID, SkillCommitInput{})
	if err != nil || preview.Error != nil || preview.Case != commitCaseReplace {
		t.Fatalf("preview = %v %+v case %s", err, preview.Error, preview.Case)
	}
	if len(preview.Warnings) == 0 || !strings.Contains(preview.Warnings[0].Summary, "without a checkout") {
		t.Fatalf("warnings = %+v", preview.Warnings)
	}
}

func TestReviewQueueListsSubmissions(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	seedHubSkill(t, hub, fixtureSkill{id: "alpha"})
	commitHubAll(t, hub, "add alpha")
	newOne := submitTree(t, hub, "team-release", "", "machine-a", submittedTree("team-release", "x", nil))
	existing := submitTree(t, hub, "alpha", "", "machine-b", submittedTree("alpha", "y", nil))

	queue, err := (SkillService{}).ReviewQueue(context.Background(), hub)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ReviewQueueItem{}
	for _, item := range queue.Items {
		got[item.SubmissionID] = item
	}
	if got[newOne.ID].Kind != ReviewKindSubmission || got[newOne.ID].Origin != "Submitted from machine-a" || got[newOne.ID].SkillID != "team-release" || !strings.Contains(got[newOne.ID].Reason, "new skill") {
		t.Fatalf("new = %+v", got[newOne.ID])
	}
	if got[existing.ID].Kind != ReviewKindSubmission || !strings.Contains(got[existing.ID].Reason, "change") || got[existing.ID].Since == "" {
		t.Fatalf("existing = %+v", got[existing.ID])
	}
}

func TestSubmissionReviewContentAndApprovalProposal(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	seedHubSkill(t, hub, fixtureSkill{id: "alpha", files: map[string]string{"notes.md": "keep\n"}})
	commitHubAll(t, hub, "add alpha")
	base, _ := loadHubSkill(hub, "alpha")
	tree := copyTree(base.tree)
	tree["notes.md"] = treeFile{Data: []byte("keep\nmore\n")}
	submission := submitTree(t, hub, "alpha", base.version, "machine-a", tree)

	service := SkillService{}
	review, err := service.ReviewSubmissionContent(context.Background(), hub, submission.ID)
	if err != nil {
		t.Fatal(err)
	}
	if review.SubmissionID != submission.ID || review.ClientLabel != "machine-a" || review.Baseline != reviewBaselineHub || !review.ThirdParty || review.Origin != "Submitted from machine-a" || review.Case != commitCaseUpdate {
		t.Fatalf("review = %+v", review)
	}
	changed := map[string]ReviewFile{}
	for _, file := range review.Files {
		changed[file.Path] = file
	}
	if len(review.Files) != 2 || !changed["notes.md"].Changed || changed["notes.md"].Diff == "" || changed["SKILL.md"].Changed {
		t.Fatalf("files = %+v", review.Files)
	}

	if _, err := service.PreviewSubmissionApproval(context.Background(), hub, submission.ID, "sha256:"+strings.Repeat("0", 64)); err == nil {
		t.Fatalf("a stale digest was accepted")
	}
	head := hubGit(t, hub, "rev-parse", "HEAD")
	proposal, err := service.PreviewSubmissionApproval(context.Background(), hub, submission.ID, review.ContentDigest)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.SubmissionID != submission.ID || !strings.Contains(proposal.DefaultMessage, "submitted from machine-a") || len(proposal.Files) != 1 {
		t.Fatalf("proposal = %+v", proposal)
	}
	if hubGit(t, hub, "rev-parse", "HEAD") != head {
		t.Fatalf("preview wrote to the hub")
	}
	ctx := workspace.WithCommitPolicy(context.Background(), workspace.CommitPolicy{Message: proposal.DefaultMessage})
	confirmed, err := service.ConfirmSkillMutation(ctx, hub, proposal.SkillProposal, proposal.Confirmation.Confirmation.Pins)
	if err != nil || confirmed.Error != nil {
		t.Fatalf("confirm: %v %+v", err, confirmed.Error)
	}
	if hubGit(t, hub, "rev-list", "--count", head+"..HEAD") != "1" {
		t.Fatalf("expected one commit")
	}
	trust, err := service.ContentTrustFor(context.Background(), hub, "alpha")
	if err != nil || !trust.Approved {
		t.Fatalf("content not approved in the same change: %+v %v", trust, err)
	}
	if trust.ContentDigest != review.ContentDigest {
		t.Fatalf("approved digest %s != reviewed %s", trust.ContentDigest, review.ContentDigest)
	}
}

func TestAddSubmissionRemovesStaleHalfWrittenUploads(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	old := filepath.Join(submissionsRoot(hub), submissionStaging, "incoming-old")
	fresh := filepath.Join(submissionsRoot(hub), submissionStaging, "incoming-fresh")
	for _, dir := range []string{old, fresh} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	aged := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, aged, aged); err != nil {
		t.Fatal(err)
	}
	submitTree(t, hub, "alpha", "", "machine-a", submittedTree("alpha", "x", nil))
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("a stale upload was kept")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("an upload that may still be arriving was removed: %v", err)
	}
}

func TestAddSubmissionRefusesPathsThatCanRewriteWhatTheReviewerSees(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	cases := map[string]map[string]string{
		"escape sequence":   {"a\x1b[2Jb.md": "x"},
		"bidi override":     {"notes\u202egnp.md": "x"},
		"newline":           {"a\nb.md": "x"},
		"case-folded twins": {"Notes.md": "x", "notes.md": "y"},
	}
	for name, extra := range cases {
		archive, err := BuildSubmissionBundle("alpha", submittedTree("alpha", "x", extra), skillruntime.BundleSubmission{ClientLabel: "m"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := AddSubmission(hub, bytes.NewReader(archive), time.Now()); !errors.Is(err, ErrSubmissionInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if listed, _ := ListSubmissions(hub); len(listed) != 0 {
		t.Fatalf("refused submissions were queued: %+v", listed)
	}
}

// countingReader reports how much of the upload the hub has read.
type countingReader struct {
	reader io.Reader
	read   int
}

func (counter *countingReader) Read(p []byte) (int, error) {
	n, err := counter.reader.Read(p)
	counter.read += n
	return n, err
}

func fillQueueNearCap(t *testing.T, hub string, free int64) {
	t.Helper()
	stage := filepath.Join(submissionsRoot(hub), "stage")
	if err := os.MkdirAll(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	huge := Submission{ID: newSubmissionID(time.Now()), SkillID: "big", ClientLabel: "machine-z", SubmittedAt: "2026-10-14T10:00:00Z", TotalBytes: MaxSubmissionQueueBytes - free, Files: []SubmissionFile{}}
	if err := writeSubmissionRecord(stage, huge); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(stage, filepath.Join(submissionsRoot(hub), huge.ID)); err != nil {
		t.Fatal(err)
	}
}

func TestSubmissionThatCannotFitIsRefusedBeforeItsFilesAreRead(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	fillQueueNearCap(t, hub, 1024)
	noise := make([]byte, 1<<20)
	if _, err := rand.Read(noise); err != nil {
		t.Fatal(err)
	}
	archive, err := BuildSubmissionBundle("alpha", submittedTree("alpha", "x", map[string]string{"blob.bin": string(noise)}), skillruntime.BundleSubmission{ClientLabel: "machine-a"})
	if err != nil {
		t.Fatal(err)
	}
	counter := &countingReader{reader: bytes.NewReader(archive)}
	if _, err := AddSubmission(hub, counter, time.Now()); !errors.Is(err, ErrSubmissionQueueFull) {
		t.Fatalf("over the byte cap: %v", err)
	}
	if counter.read > len(archive)/4 {
		t.Fatalf("the hub read %d of %d bytes before refusing", counter.read, len(archive))
	}
}

func TestStalledUploadsAreBoundedAndTheirBytesReserved(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	var stalled []*io.PipeWriter
	results := make(chan error, MaxConcurrentSubmissionUploads)
	for range MaxConcurrentSubmissionUploads {
		reader, writer := io.Pipe()
		stalled = append(stalled, writer)
		go func() {
			_, err := AddSubmission(hub, reader, time.Now())
			results <- err
		}()
	}
	archive, _ := BuildSubmissionBundle("alpha", submittedTree("alpha", "x", nil), skillruntime.BundleSubmission{ClientLabel: "m"})
	slots := intakeOf(hub).slots
	for deadline := time.Now().Add(10 * time.Second); len(slots) < MaxConcurrentSubmissionUploads; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("only %d of the stalled uploads were taken", len(slots))
		}
	}
	if _, err := AddSubmission(hub, bytes.NewReader(archive), time.Now()); !errors.Is(err, ErrSubmissionBusy) {
		t.Fatalf("an upload beyond the limit: %v", err)
	}
	for _, writer := range stalled {
		_ = writer.CloseWithError(errors.New("client went away"))
	}
	for range MaxConcurrentSubmissionUploads {
		<-results
	}
	if _, err := AddSubmission(hub, bytes.NewReader(archive), time.Now()); err != nil {
		t.Fatalf("a finished upload must free its slot: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(submissionsRoot(hub), submissionStaging))
	if len(entries) != 0 {
		t.Fatalf("staging holds %d leftovers", len(entries))
	}
}
