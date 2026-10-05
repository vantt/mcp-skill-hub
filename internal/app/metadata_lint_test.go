package app

import (
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

func TestRuntimeHintFindings(t *testing.T) {
	t.Parallel()

	const sentinel = "SECRET_SENTINEL_BODY_CONTENT_12345"

	t.Run("absolute_install_path", func(t *testing.T) {
		files := []skillruntime.HintFile{
			{Path: "install.sh", Content: []byte("#!/bin/sh\ncp " + sentinel + " ~/.claude/skills/target\n")},
		}
		hints := skillruntime.AnalyzeHints(files, false)
		warnings := runtimeHintFindings("test-skill", hints)
		found := false
		for _, w := range warnings {
			if w.Code == "absolute_install_path" {
				found = true
				if strings.Contains(w.Summary, sentinel) {
					t.Fatalf("warning summary leaked file content: %s", w.Summary)
				}
				if !strings.Contains(w.Summary, "install.sh") {
					t.Fatalf("expected install.sh in summary, got: %s", w.Summary)
				}
			}
		}
		if !found {
			t.Fatalf("expected absolute_install_path warning, got: %v", warnings)
		}
	})

	t.Run("missing_runtime_block", func(t *testing.T) {
		files := []skillruntime.HintFile{
			{Path: "script.py", Content: []byte("#!/usr/bin/env python3\nprint('" + sentinel + "')\n")},
		}
		hints := skillruntime.AnalyzeHints(files, false)
		warnings := runtimeHintFindings("test-skill", hints)
		found := false
		for _, w := range warnings {
			if w.Code == "missing_runtime_block" {
				found = true
				if strings.Contains(w.Summary, sentinel) {
					t.Fatalf("warning summary leaked file content: %s", w.Summary)
				}
			}
		}
		if !found {
			t.Fatalf("expected missing_runtime_block warning, got: %v", warnings)
		}
	})

	t.Run("install_prose_without_runtime", func(t *testing.T) {
		files := []skillruntime.HintFile{
			{Path: "SKILL.md", Content: []byte("# Installation\nRun pip install " + sentinel + "\n")},
		}
		hints := skillruntime.AnalyzeHints(files, false)
		warnings := runtimeHintFindings("test-skill", hints)
		found := false
		for _, w := range warnings {
			if w.Code == "install_prose_without_runtime" {
				found = true
				if strings.Contains(w.Summary, sentinel) {
					t.Fatalf("warning summary leaked file content: %s", w.Summary)
				}
			}
		}
		if !found {
			t.Fatalf("expected install_prose_without_runtime warning, got: %v", warnings)
		}
	})

	t.Run("missing_lockfile", func(t *testing.T) {
		files := []skillruntime.HintFile{
			{Path: "package.json", Content: []byte("{\"name\": \"" + sentinel + "\"}\n")},
		}
		hints := skillruntime.AnalyzeHints(files, false)
		warnings := runtimeHintFindings("test-skill", hints)
		found := false
		for _, w := range warnings {
			if w.Code == "missing_lockfile" {
				found = true
				if strings.Contains(w.Summary, sentinel) {
					t.Fatalf("warning summary leaked file content: %s", w.Summary)
				}
				if !strings.Contains(w.Summary, "package.json") {
					t.Fatalf("expected package.json in summary, got: %s", w.Summary)
				}
			}
		}
		if !found {
			t.Fatalf("expected missing_lockfile warning, got: %v", warnings)
		}
	})
}
