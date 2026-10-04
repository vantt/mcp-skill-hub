package skillruntime

import (
	"slices"
	"strings"
	"testing"
)

func hintFiles(entries ...string) []HintFile {
	files := make([]HintFile, 0, len(entries)/2)
	for index := 0; index+1 < len(entries); index += 2 {
		files = append(files, HintFile{Path: entries[index], Content: []byte(entries[index+1])})
	}
	return files
}

func TestAnalyzeHintsInterpreters(t *testing.T) {
	t.Parallel()
	hints := AnalyzeHints(hintFiles(
		"scripts/a.py", "x", "scripts/b.mjs", "x", "scripts/c.ts", "x", "scripts/d.sh", "x",
		"scripts/e.bash", "x", "scripts/f.rb", "x", "scripts/g.ps1", "x",
		"bin/tool", "#!/usr/bin/env -S python3.12 -u\nprint()\n",
		"bin/other", "#!/bin/bash\n",
		"bin/unknown", "#!/usr/bin/perl\n",
		"SKILL.md", "plain",
	), true)
	want := []string{"bash", "node", "python3", "pwsh", "ruby", "sh"}
	slices.Sort(want)
	if !slices.Equal(hints.Interpreters, want) {
		t.Fatalf("interpreters = %v, want %v", hints.Interpreters, want)
	}
	if hints.MissingRuntimeBlock {
		t.Fatal("a skill with a runtime block is never missing one")
	}
}

func TestAnalyzeHintsDependencyManifestsAndMissingRuntimeBlock(t *testing.T) {
	t.Parallel()
	files := hintFiles("scripts/requirements-dev.txt", "x", "package.json", "{}", "notes.txt", "x", "skill.meta.yaml", "runtime: {}")
	hints := AnalyzeHints(files, false)
	if !slices.Equal(hints.DependencyManifests, []string{"package.json", "scripts/requirements-dev.txt"}) {
		t.Fatalf("manifests = %v", hints.DependencyManifests)
	}
	if !hints.MissingRuntimeBlock {
		t.Fatal("dependency manifests without a runtime block must be flagged")
	}
	if AnalyzeHints(files, true).MissingRuntimeBlock {
		t.Fatal("runtime block present")
	}
	if AnalyzeHints(hintFiles("SKILL.md", "docs only"), false).MissingRuntimeBlock {
		t.Fatal("instruction-only skills need no runtime block")
	}
}

func TestAnalyzeHintsAbsoluteInstallPathsSkipBinaryFiles(t *testing.T) {
	t.Parallel()
	hints := AnalyzeHints([]HintFile{
		{Path: "SKILL.md", Content: []byte("run ~/.claude/skills/x/run.sh")},
		{Path: "scripts/a.sh", Content: []byte(`python "$HOME/.claude/skills/x/a.py"`)},
		{Path: "docs/b.md", Content: []byte("see .agents/skills/x and .codex/skills/y")},
		{Path: "clean.md", Content: []byte("nothing here")},
		{Path: "blob.bin", Content: []byte("\x00 ~/.claude/skills/ \x00")},
	}, true)
	want := []string{"SKILL.md", "docs/b.md", "scripts/a.sh"}
	if !slices.Equal(hints.AbsoluteInstallPaths, want) {
		t.Fatalf("paths = %v, want %v", hints.AbsoluteInstallPaths, want)
	}
}

func TestAnalyzeHintsInstallCuesReportNamesOnly(t *testing.T) {
	t.Parallel()
	secret := "SECRET-PACKAGE-NAME"
	skill := "# Title\n\n## Prerequisites\n\nRun `pip install " + secret + "` then npm i left-pad and `uv sync`.\n"
	hints := AnalyzeHints(hintFiles("SKILL.md", skill, "scripts/doc.md", "pip install elsewhere"), true)
	want := []string{"heading: prerequisites", "npm i", "pip install", "uv sync"}
	if !hints.InstallProseDetected || !slices.Equal(hints.InstallCues, want) {
		t.Fatalf("cues = %v detected = %v, want %v", hints.InstallCues, hints.InstallProseDetected, want)
	}
	for _, cue := range hints.InstallCues {
		if strings.Contains(cue, secret) || strings.Contains(cue, "left-pad") {
			t.Fatalf("cue leaks surrounding text: %q", cue)
		}
	}

	none := AnalyzeHints(hintFiles("SKILL.md", "Use npm in your head. Setup is easy.", "README.md", "# Overview\n"), true)
	if none.InstallProseDetected || len(none.InstallCues) != 0 {
		t.Fatalf("no cues expected: %v", none.InstallCues)
	}
	readme := AnalyzeHints(hintFiles("README.md", "## Installation\n"), true)
	if !slices.Equal(readme.InstallCues, []string{"heading: installation"}) {
		t.Fatalf("readme cues = %v", readme.InstallCues)
	}
}

func TestAnalyzeHintsMissingLockfiles(t *testing.T) {
	t.Parallel()
	hints := AnalyzeHints(hintFiles(
		"package.json", "{}",
		"web/package.json", "{}", "web/yarn.lock", "x",
		"pyproject.toml", "x",
		"tools/pyproject.toml", "x", "tools/uv.lock", "x",
		"requirements.txt", "requests>=2\n",
		"requirements-dev.txt", "# pinned\npytest==8.0\n-r requirements.txt\n",
		"go.mod", "module x\n",
		"cmd/go.mod", "module y\n\nrequire example.com/z v1.0.0\n",
		"Cargo.toml", "x",
		"Gemfile", "x", "Gemfile.lock", "x",
	), false)
	want := []string{"cargo: Cargo.toml", "go: cmd/go.mod", "npm: package.json", "python: pyproject.toml", "python: requirements.txt"}
	if !slices.Equal(hints.MissingLockfiles, want) {
		t.Fatalf("missing lockfiles = %v, want %v", hints.MissingLockfiles, want)
	}
	for _, entry := range hints.MissingLockfiles {
		if strings.Contains(entry, ">=") {
			t.Fatalf("hint leaked file text: %q", entry)
		}
	}
	if got := AnalyzeHints(hintFiles("SKILL.md", "x"), false).MissingLockfiles; got == nil || len(got) != 0 {
		t.Fatalf("missing lockfiles must be an empty list, got %#v", got)
	}
}

func TestInterpreterOf(t *testing.T) {
	t.Parallel()
	if InterpreterOf("scripts/a.py", nil) != "python3" || InterpreterOf("bin/x", []byte("#!/bin/sh\n")) != "sh" || InterpreterOf("notes.md", []byte("hi")) != "" {
		t.Fatal("InterpreterOf misclassified a file")
	}
}
