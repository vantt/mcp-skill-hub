package app

import (
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

func findWarningByCode(warnings []Warning, code string) *Warning {
	for i := range warnings {
		if warnings[i].Code == code {
			return &warnings[i]
		}
	}
	return nil
}

func TestRuntimeHintFindings(t *testing.T) {
	t.Parallel()

	const sentinel = "SENTINEL-SECRET-BODY-CONTENT-12345"

	// 1. absolute_install_path
	t.Run("absolute_install_path", func(t *testing.T) {
		files := []skillruntime.HintFile{
			{
				Path:    "scripts/run.sh",
				Content: []byte("#!/bin/sh\ncp ~/.claude/skills/foo /tmp/ " + sentinel + "\n"),
			},
		}
		hints := skillruntime.AnalyzeHints(files, true)
		warnings := runtimeHintFindings("skill-path", hints)
		w := findWarningByCode(warnings, "absolute_install_path")
		if w == nil {
			t.Fatalf("expected absolute_install_path warning, got: %+v", warnings)
		}
		if strings.Contains(w.Summary, sentinel) {
			t.Fatalf("summary leaked file body sentinel: %s", w.Summary)
		}
	})

	// 2. missing_runtime_block
	t.Run("missing_runtime_block", func(t *testing.T) {
		files := []skillruntime.HintFile{
			{
				Path:    "scripts/run.sh",
				Content: []byte("#!/bin/sh\necho " + sentinel + "\n"),
			},
		}
		hints := skillruntime.AnalyzeHints(files, false)
		warnings := runtimeHintFindings("skill-spec", hints)
		w := findWarningByCode(warnings, "missing_runtime_block")
		if w == nil {
			t.Fatalf("expected missing_runtime_block warning, got: %+v", warnings)
		}
		if strings.Contains(w.Summary, sentinel) {
			t.Fatalf("summary leaked file body sentinel: %s", w.Summary)
		}
	})

	// 3. install_prose_without_runtime
	t.Run("install_prose_without_runtime", func(t *testing.T) {
		files := []skillruntime.HintFile{
			{
				Path:    "SKILL.md",
				Content: []byte("# Installation\nRun `npm install -g foo` " + sentinel + "\n"),
			},
		}
		hints := skillruntime.AnalyzeHints(files, false)
		warnings := runtimeHintFindings("skill-prose", hints)
		w := findWarningByCode(warnings, "install_prose_without_runtime")
		if w == nil {
			t.Fatalf("expected install_prose_without_runtime warning, got: %+v", warnings)
		}
		if strings.Contains(w.Summary, sentinel) {
			t.Fatalf("summary leaked file body sentinel: %s", w.Summary)
		}
	})

	// 4. missing_lockfile
	t.Run("missing_lockfile", func(t *testing.T) {
		files := []skillruntime.HintFile{
			{
				Path:    "package.json",
				Content: []byte(`{"name": "test", "private": true, "extra": "` + sentinel + `"}`),
			},
		}
		hints := skillruntime.AnalyzeHints(files, true)
		warnings := runtimeHintFindings("skill-lockfile", hints)
		w := findWarningByCode(warnings, "missing_lockfile")
		if w == nil {
			t.Fatalf("expected missing_lockfile warning, got: %+v", warnings)
		}
		if strings.Contains(w.Summary, sentinel) {
			t.Fatalf("summary leaked file body sentinel: %s", w.Summary)
		}
	})
}
