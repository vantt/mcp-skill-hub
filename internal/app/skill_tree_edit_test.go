package app

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

const treeSkillMD = "---\nname: pdf-tools\ndescription: Does pdf-tools things. Use when you need pdf-tools.\n---\n\n# pdf-tools\n\nline one\nline TWO\nline three\n"

// seededTree seeds a hub with one skill that has a few files.
func seededTree(t *testing.T) (hub string, files SkillFilesResult) {
	t.Helper()
	hub = newGitHub(t)
	seedHubSkill(t, hub, fixtureSkill{id: "pdf-tools", files: map[string]string{
		"references/forms.md":  "forms\none\n",
		"references/tables.md": "tables\n",
		"scripts/old.sh":       "#!/bin/sh\necho old\n",
	}})
	commitHubAll(t, hub, "add pdf-tools")
	files, err := (SkillService{}).ListSkillFiles(context.Background(), hub, "pdf-tools")
	if err != nil {
		t.Fatal(err)
	}
	return hub, files
}

func digestFor(t *testing.T, files SkillFilesResult, rel string) string {
	t.Helper()
	for _, file := range files.Files {
		if file.Path == rel {
			return file.Digest
		}
	}
	t.Fatalf("no %s in %+v", rel, files.Files)
	return ""
}

func TestSkillFilesListsSkillMDFirstWithoutHubMetadata(t *testing.T) {
	t.Parallel()
	_, files := seededTree(t)
	var paths []string
	for _, file := range files.Files {
		paths = append(paths, file.Path)
	}
	want := "SKILL.md references/forms.md references/tables.md scripts/old.sh"
	if strings.Join(paths, " ") != want {
		t.Fatalf("files = %v, want %s", paths, want)
	}
	if files.BaseVersion == "" || files.State != "active" || !files.Files[0].Editable {
		t.Fatalf("result = %+v", files)
	}
}

func TestSkillChangesMakeOneProposalAndOneCommit(t *testing.T) {
	t.Parallel()
	hub, files := seededTree(t)
	before := hubGit(t, hub, "rev-list", "--count", "HEAD")
	service := SkillService{}
	preview, err := service.PreviewSkillChanges(context.Background(), hub, "pdf-tools", SkillChangesInput{
		BaseVersion: files.BaseVersion,
		Changes: []SkillTreeChange{
			{Action: TreeActionModify, Path: "SKILL.md", Content: strings.Replace(treeSkillMD, "line TWO", "line 2", 1), ExpectedDigest: digestFor(t, files, "SKILL.md")},
			{Action: TreeActionAdd, Path: "assets/logo.bin", Content: base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 3}), Encoding: TreeEncodingBase64},
			{Action: TreeActionRename, Path: "references/forms-guide.md", From: "references/forms.md", ExpectedDigest: digestFor(t, files, "references/forms.md")},
			{Action: TreeActionDelete, Path: "scripts/old.sh", ExpectedDigest: digestFor(t, files, "scripts/old.sh")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]SkillChangeFile{}
	for _, file := range preview.Files {
		got[file.Path] = file
	}
	if len(got) != 4 || got["SKILL.md"].Change != "M" || got["assets/logo.bin"].Change != "A" || !got["assets/logo.bin"].Binary ||
		got["references/forms-guide.md"].Change != "R" || got["references/forms-guide.md"].From != "references/forms.md" || got["scripts/old.sh"].Change != "D" {
		t.Fatalf("files = %+v", preview.Files)
	}
	if got["SKILL.md"].Added != 1 || got["SKILL.md"].Removed != 1 || !strings.Contains(got["SKILL.md"].Diff, "+line 2") {
		t.Fatalf("SKILL.md diff = %+v", got["SKILL.md"])
	}
	if preview.DefaultMessage != "Commit pdf-tools: 4 files" || preview.BaseVersion != files.BaseVersion {
		t.Fatalf("message = %q base = %q", preview.DefaultMessage, preview.BaseVersion)
	}
	if hubGit(t, hub, "rev-list", "--count", "HEAD") != before {
		t.Fatal("a preview must not commit")
	}

	loaded, err := service.LoadSkillProposal(context.Background(), hub, preview.Confirmation.Confirmation.Pins.ProposalID)
	if err != nil || loaded.Error != nil {
		t.Fatalf("load: %v %+v", err, loaded.Error)
	}
	result, err := service.ConfirmSkillMutation(commitCtx(), hub, loaded, loaded.Confirmation.Confirmation.Pins)
	if err != nil || result.Error != nil {
		t.Fatalf("confirm: %v %+v", err, result.Error)
	}
	if result.Commit == nil || result.Commit.Hash == "" {
		t.Fatalf("commit = %+v", result.Commit)
	}
	if after := hubGit(t, hub, "rev-list", "--count", "HEAD"); after == before {
		t.Fatal("confirm made no commit")
	}
	changed := strings.Fields(hubGit(t, hub, "show", "--no-renames", "--name-only", "--format=", "HEAD"))
	want := map[string]bool{
		"skills/default/pdf-tools/SKILL.md": true, "skills/default/pdf-tools/assets/logo.bin": true,
		"skills/default/pdf-tools/references/forms.md": true, "skills/default/pdf-tools/references/forms-guide.md": true,
		"skills/default/pdf-tools/scripts/old.sh": true,
	}
	for _, path := range changed {
		if strings.HasPrefix(path, "history/operations/") {
			continue
		}
		if !want[path] {
			t.Fatalf("unexpected path %s in commit %v", path, changed)
		}
		delete(want, path)
	}
	if len(want) != 0 {
		t.Fatalf("commit lacks %v (has %v)", want, changed)
	}
}

func TestSkillChangesRefuseWhatCannotBeApplied(t *testing.T) {
	t.Parallel()
	hub, files := seededTree(t)
	service := SkillService{}
	oversized := base64.StdEncoding.EncodeToString(make([]byte, maxUploadedBytes+1))
	cases := []struct {
		name  string
		input SkillChangesInput
		code  ErrorCode
	}{
		{"stale base version", SkillChangesInput{BaseVersion: "sha256:old", Changes: []SkillTreeChange{{Action: TreeActionAdd, Path: "a.md", Content: "a"}}}, ErrorStaleProposal},
		{"oversized upload", SkillChangesInput{BaseVersion: files.BaseVersion, Changes: []SkillTreeChange{{Action: TreeActionAdd, Path: "big.bin", Content: oversized, Encoding: TreeEncodingBase64}}}, ErrorResourceLimitsExceeded},
		{"oversized text", SkillChangesInput{BaseVersion: files.BaseVersion, Changes: []SkillTreeChange{{Action: TreeActionAdd, Path: "big.md", Content: strings.Repeat("x", maxEditedTextBytes+1)}}}, ErrorResourceLimitsExceeded},
		{"edited elsewhere", SkillChangesInput{BaseVersion: files.BaseVersion, Changes: []SkillTreeChange{{Action: TreeActionModify, Path: "references/tables.md", Content: "x", ExpectedDigest: "sha256:" + strings.Repeat("0", 64)}}}, ErrorEditConflict},
		{"path leaves the folder", SkillChangesInput{BaseVersion: files.BaseVersion, Changes: []SkillTreeChange{{Action: TreeActionAdd, Path: "../escape.md", Content: "x"}}}, ErrorInvalidRequest},
		{"hub metadata", SkillChangesInput{BaseVersion: files.BaseVersion, Changes: []SkillTreeChange{{Action: TreeActionAdd, Path: ".meta/skill.yaml", Content: "x"}}}, ErrorInvalidRequest},
		{"a secret file", SkillChangesInput{BaseVersion: files.BaseVersion, Changes: []SkillTreeChange{{Action: TreeActionAdd, Path: ".env", Content: "T=1"}}}, ErrorInvalidRequest},
		{"delete SKILL.md", SkillChangesInput{BaseVersion: files.BaseVersion, Changes: []SkillTreeChange{{Action: TreeActionDelete, Path: "SKILL.md"}}}, ErrorInvalidRequest},
		{"name already used", SkillChangesInput{BaseVersion: files.BaseVersion, Changes: []SkillTreeChange{{Action: TreeActionRename, From: "scripts/old.sh", Path: "references/tables.md"}}}, ErrorInvalidRequest},
		{"binary sent as text", SkillChangesInput{BaseVersion: files.BaseVersion, Changes: []SkillTreeChange{{Action: TreeActionAdd, Path: "x.bin", Content: "a\x00b"}}}, ErrorInvalidRequest},
		{"no change", SkillChangesInput{BaseVersion: files.BaseVersion, Changes: []SkillTreeChange{{Action: TreeActionModify, Path: "references/tables.md", Content: "tables\n"}}}, ErrorInvalidRequest},
	}
	for _, tc := range cases {
		_, err := service.PreviewSkillChanges(context.Background(), hub, "pdf-tools", tc.input)
		var refusal *Error
		if !errors.As(err, &refusal) || refusal.Code != tc.code {
			t.Errorf("%s: err = %v, want code %s", tc.name, err, tc.code)
		}
	}
}

func TestSkillChangesKeepCRLFAndTheExecutableBit(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	seedHubSkill(t, hub, fixtureSkill{id: "pdf-tools", files: map[string]string{"notes.md": "a\r\nb\r\n", "scripts/run.sh": "#!/bin/sh\n"}, executable: []string{"scripts/run.sh"}})
	commitHubAll(t, hub, "add")
	service := SkillService{}
	files, err := service.ListSkillFiles(context.Background(), hub, "pdf-tools")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewSkillChanges(context.Background(), hub, "pdf-tools", SkillChangesInput{BaseVersion: files.BaseVersion, Changes: []SkillTreeChange{
		{Action: TreeActionModify, Path: "notes.md", Content: "a\nb\nc\n"},
		{Action: TreeActionModify, Path: "scripts/run.sh", Content: "#!/bin/sh\necho 1\n"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ConfirmSkillMutation(commitCtx(), hub, preview.SkillProposal, preview.Confirmation.Confirmation.Pins)
	if err != nil || result.Error != nil {
		t.Fatalf("confirm: %v %+v", err, result.Error)
	}
	after, err := service.ReadSkillFile(context.Background(), hub, "pdf-tools", "notes.md")
	if err != nil || after.Content == nil || *after.Content != "a\r\nb\r\nc\r\n" {
		t.Fatalf("notes = %+v, %v", after, err)
	}
	script, err := service.ReadSkillFile(context.Background(), hub, "pdf-tools", "scripts/run.sh")
	if err != nil || !script.Executable {
		t.Fatalf("script = %+v, %v", script, err)
	}
}
