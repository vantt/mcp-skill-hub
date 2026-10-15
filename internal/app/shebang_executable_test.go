package app

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// hubFileMode returns the mode Git records for a file of a hub skill, for
// example "100755". It works on every platform, because it reads the index.
func hubFileMode(t *testing.T, hub, id, rel string) string {
	t.Helper()
	line := hubGit(t, hub, "ls-files", "-s", "--", "skills/default/"+id+"/"+rel)
	mode, _, _ := strings.Cut(line, " ")
	if mode == "" {
		t.Fatalf("%s/%s is not tracked by the hub's Git", id, rel)
	}
	return mode
}

func TestFolderCommitMakesShebangScriptsExecutableInGit(t *testing.T) {
	t.Parallel()
	// core.filemode=false is how Git runs on Windows: the index never takes
	// the executable bit from the file system.
	for _, fileMode := range []string{"true", "false"} {
		t.Run("core.filemode="+fileMode, func(t *testing.T) {
			t.Parallel()
			hub := newGitHub(t)
			hubGit(t, hub, "config", "core.filemode", fileMode)
			project := newProjectDir(t)
			folder := filepath.Join(project, "shebang-tools")
			writeProjectFile(t, folder, "SKILL.md", "---\nname: shebang-tools\ndescription: Runs shebang tools. Use when you need shebang tools.\n---\n\n# shebang-tools\n\nbody\n", 0o644)
			writeProjectFile(t, folder, "scripts/run.sh", "#!/bin/sh\necho run\n", 0o644)
			writeProjectFile(t, folder, "references/notes.md", "# notes\n", 0o644)

			preview := previewCommit(t, hub, project, "shebang-tools", SkillCommitInput{})
			if result, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{}); err != nil || result.Error != nil {
				t.Fatalf("apply = %+v %v", result.Error, err)
			}
			if mode := hubFileMode(t, hub, "shebang-tools", "scripts/run.sh"); mode != "100755" {
				t.Fatalf("shebang script mode = %s, want 100755", mode)
			}
			if mode := hubFileMode(t, hub, "shebang-tools", "references/notes.md"); mode != "100644" {
				t.Fatalf("plain file mode = %s, want 100644", mode)
			}
			if status := hubGit(t, hub, "status", "--porcelain", "--", "skills"); status != "" {
				t.Fatalf("skills left uncommitted:\n%s", status)
			}
		})
	}
}

func TestCheckoutCommitKeepsTheHubExecutableBitOfAFileWithoutShebang(t *testing.T) {
	t.Parallel()
	hub, project, folder := checkedOut(t, fixtureSkill{id: "keep-exec", files: map[string]string{"scripts/tool": "echo tool\n"}, executable: []string{"scripts/tool"}})
	hubGit(t, hub, "config", "core.filemode", "false")
	if mode := hubFileMode(t, hub, "keep-exec", "scripts/tool"); mode != "100755" {
		t.Fatalf("seeded mode = %s, want 100755", mode)
	}
	// A Windows checkout has no executable bit to send back.
	writeProjectFile(t, folder, "scripts/tool", "echo tool v2\n", 0o644)
	preview := previewCommit(t, hub, project, "keep-exec", SkillCommitInput{})
	if result, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{}); err != nil || result.Error != nil {
		t.Fatalf("apply = %+v %v", result.Error, err)
	}
	if mode := hubFileMode(t, hub, "keep-exec", "scripts/tool"); mode != "100755" {
		t.Fatalf("mode = %s, want the hub's 100755 kept", mode)
	}
}

func TestWebUploadOfAShebangFileMakesItExecutable(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	hubGit(t, hub, "config", "core.filemode", "false")
	seedHubSkill(t, hub, fixtureSkill{id: "upload-exec"})
	commitHubAll(t, hub, "add")
	service := SkillService{}
	files, err := service.ListSkillFiles(context.Background(), hub, "upload-exec")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewSkillChanges(context.Background(), hub, "upload-exec", SkillChangesInput{BaseVersion: files.BaseVersion, Changes: []SkillTreeChange{
		{Action: TreeActionAdd, Path: "scripts/new.py", Content: "#!/usr/bin/env python3\nprint(1)\n"},
		{Action: TreeActionAdd, Path: "references/new.md", Content: "# new\n"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ConfirmSkillMutation(commitCtx(), hub, preview.SkillProposal, preview.Confirmation.Confirmation.Pins)
	if err != nil || result.Error != nil {
		t.Fatalf("confirm: %v %+v", err, result.Error)
	}
	if mode := hubFileMode(t, hub, "upload-exec", "scripts/new.py"); mode != "100755" {
		t.Fatalf("uploaded shebang file mode = %s, want 100755", mode)
	}
	if mode := hubFileMode(t, hub, "upload-exec", "references/new.md"); mode != "100644" {
		t.Fatalf("uploaded plain file mode = %s, want 100644", mode)
	}
	script, err := service.ReadSkillFile(context.Background(), hub, "upload-exec", "scripts/new.py")
	if err != nil {
		t.Fatal(err)
	}
	// The file system has no executable bit on Windows; Git's mode above does.
	if runtime.GOOS != "windows" && !script.Executable {
		t.Fatalf("the hub must list the uploaded script as executable: %+v", script.SkillFileEntry)
	}
}
