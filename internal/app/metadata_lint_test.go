package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/skillhub-mcp/internal/skillruntime"
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

func TestValidateWorkspaceWarnsAboutSecretLikeFilesInSkills(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	dir := filepath.Join(t.TempDir(), "team-release")
	writeAddFixture(t, dir, "SKILL.md", "---\nname: team-release\ndescription: Release the team.\n---\n\n# Release\n", 0o644)
	service := SkillAddService{Clock: sourceClock{now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)}}
	preview, err := service.PreviewSkillAdd(context.Background(), root, SkillAddInput{Locator: dir})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview failed: %v, %#v", err, preview.Error)
	}
	if result, err := service.ConfirmSkillAdd(context.Background(), root, preview, preview.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm failed: %v, %#v", err, result.Error)
	}
	validate := func() Result {
		result, err := (WorkspaceService{}).ValidateWorkspace(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	for _, w := range validate().Warnings {
		if w.Code == "secret_like_file" {
			t.Fatalf("clean hub warned: %#v", w)
		}
	}

	if err := os.WriteFile(filepath.Join(root, "skills", "default", "team-release", ".env"), []byte("TOKEN=abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := validate()
	if result.Status != StatusOK || len(result.Items) != 0 {
		t.Fatalf("a secret-like file must warn, not fail validation: %#v", result)
	}
	found := false
	for _, w := range result.Warnings {
		if w.Code == "secret_like_file" {
			found = true
			if !strings.Contains(w.Summary, "skills/default/team-release/.env") || strings.Contains(w.Summary, "TOKEN=abc") {
				t.Fatalf("warning summary = %q", w.Summary)
			}
		}
	}
	if !found {
		t.Fatalf("no secret_like_file warning in %#v", result.Warnings)
	}
}
