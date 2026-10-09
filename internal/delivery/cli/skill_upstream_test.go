package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"gopkg.in/yaml.v3"
)

func seedTrackedSkill(t *testing.T, root, skillID, sourceID string, origin app.SkillOrigin) {
	t.Helper()
	service := app.SkillService{}
	prev, err := service.PreviewCreate(context.Background(), root, skill.CreateInput{
		ID:          skillID,
		Collection:  "default",
		Name:        skillID,
		Description: "Tracked skill " + skillID,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(context.Background(), root, prev, prev.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}

	metaPath := filepath.Join(root, "skills", "default", skillID, ".meta", "skill.yaml")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		metaPath = filepath.Join(root, "skills", "default", skillID, "skill.meta.yaml")
	}
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(metaBytes, &doc); err != nil {
		t.Fatal(err)
	}
	if doc == nil {
		doc = make(map[string]any)
	}

	skillMDBytes, _ := os.ReadFile(filepath.Join(root, "skills", "default", skillID, "SKILL.md"))
	skillMDDigest := sourcepkg.Digest(skillMDBytes)
	cleanFilesDigest := skillruntime.ContentDigest([]skillruntime.ResourceDigest{{Path: "SKILL.md", Digest: skillMDDigest}}, skillruntime.Spec{}, false)

	filesDigest := origin.FilesDigest
	if filesDigest == "" || strings.HasPrefix(filesDigest, "sha256:00000000") {
		filesDigest = cleanFilesDigest
	}

	doc["sources"] = []any{
		map[string]any{
			"id":           sourceID,
			"roles":        []string{"upstream"},
			"kind":         origin.Kind,
			"repository":   origin.Repository,
			"ref":          origin.Ref,
			"path":         origin.Path,
			"commit":       origin.Commit,
			"files_digest": filesDigest,
			"synced":       origin.Commit,
		},
	}
	doc["provenance"] = map[string]any{
		"source_id": sourceID,
		"origin": map[string]any{
			"kind":         origin.Kind,
			"repository":   origin.Repository,
			"ref":          origin.Ref,
			"path":         origin.Path,
			"commit":       origin.Commit,
			"files_digest": filesDigest,
		},
	}
	newMetaBytes, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, newMetaBytes, 0o644); err != nil {
		t.Fatal(err)
	}
}

func seedSourceRecord(t *testing.T, root, sourceID, repo, ref string) {
	t.Helper()
	rev := sourcepkg.Revision{
		Kind:          "git-commit",
		Value:         "1111222233334444555566667777888899990000",
		ContentDigest: sourcepkg.Digest([]byte("1111222233334444555566667777888899990000")),
		ObservedAt:    time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
	}
	rec := sourcepkg.Record{
		SchemaVersion:   1,
		ID:              sourceID,
		Adapter:         "git",
		Locator:         sourcepkg.Locator{Repository: repo, Ref: ref},
		Status:          "watching",
		Identity:        sourcepkg.Identity{Name: sourceID, Canonical: repo, DefaultBranch: ref},
		Monitoring:      sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		Limits:          sourcepkg.Limits{TimeoutSeconds: 30, MaxBytes: 1024 * 1024, MaxFiles: 100, MaxFileBytes: 1024 * 1024},
		CurrentRevision: &rev,
	}
	data, err := sourcepkg.MarshalCanonical(rec)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(root, "sources", "catalog"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "sources", "catalog", sourceID+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func recordUpstreamState(t *testing.T, root, skillID, sourceID, repo, ref, path, baseCommit, latestCommit string, filesChanged int, status string) {
	t.Helper()
	store := sourcepkg.OperationalStore{Root: root}
	var changes []sourcepkg.Change
	for i := 0; i < filesChanged; i++ {
		changes = append(changes, sourcepkg.Change{
			Path:   fmt.Sprintf("file%d.txt", i+1),
			Status: "modified",
		})
	}
	now := time.Now().UTC()
	state := sourcepkg.UpstreamState{
		SkillID:         skillID,
		SourceID:        sourceID,
		Repository:      repo,
		Ref:             ref,
		Path:            path,
		BaseCommit:      baseCommit,
		CheckedCommit:   latestCommit,
		CheckedCommitAt: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC),
		Upstream:        status,
		ChangedFiles:    changes,
		CheckedAt:       now,
	}
	if err := store.RecordUpstream(context.Background(), []sourcepkg.UpstreamState{state}); err != nil {
		t.Fatal(err)
	}
}

func TestSkillOutdated(t *testing.T) {
	t.Parallel()

	// 1. Empty workspace prints "No skills track an upstream repository." and exits 0
	t.Run("empty workspace", func(t *testing.T) {
		root := initTestWorkspace(t)
		var stdout, stderr bytes.Buffer
		code := Run([]string{"skill", "outdated", "--workspace", root}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d, stderr: %s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "No skills track an upstream repository.") {
			t.Fatalf("expected empty workspace message, got: %s", stdout.String())
		}
	})

	// 2. One changed state prints a row containing "update available"
	t.Run("one changed state", func(t *testing.T) {
		root := initTestWorkspace(t)
		sourceID := "anthropics-skills"
		repo := "https://github.com/anthropics/skills"
		ref := "main"
		skillID := "pdf"
		baseCommit := "3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a"
		latestCommit := "81d04be7c2aa1111222233334444555566667777"

		seedSourceRecord(t, root, sourceID, repo, ref)
		seedTrackedSkill(t, root, skillID, sourceID, app.SkillOrigin{
			Kind:        "github",
			Repository:  repo,
			Ref:         ref,
			Path:        "skills/pdf",
			Commit:      baseCommit,
			FilesDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		})
		recordUpstreamState(t, root, skillID, sourceID, repo, ref, "skills/pdf", baseCommit, latestCommit, 2, "changed")

		var stdout, stderr bytes.Buffer
		code := Run([]string{"skill", "outdated", "--workspace", root}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d, stderr: %s", code, stderr.String())
		}
		outStr := stdout.String()
		if !strings.Contains(outStr, "update available") {
			t.Fatalf("expected 'update available' in output, got:\n%s", outStr)
		}
		if !strings.Contains(outStr, "pdf") {
			t.Fatalf("expected 'pdf' in output, got:\n%s", outStr)
		}

		// 3. --exit-code exits 1 for update_available
		stdout.Reset()
		stderr.Reset()
		codeExit := Run([]string{"skill", "outdated", "--exit-code", "--workspace", root}, &stdout, &stderr)
		if codeExit != 1 {
			t.Fatalf("expected exit 1 with --exit-code on update_available, got %d", codeExit)
		}

		// 4. --json output decodes into UpstreamListResult with status == "update_available"
		stdout.Reset()
		stderr.Reset()
		codeJSON := Run([]string{"skill", "outdated", "--json", "--workspace", root}, &stdout, &stderr)
		if codeJSON != 0 {
			t.Fatalf("expected exit 0 for --json, got %d", codeJSON)
		}
		var listResult UpstreamListResult
		if err := json.Unmarshal(stdout.Bytes(), &listResult); err != nil {
			t.Fatalf("failed to decode JSON output: %v, raw: %s", err, stdout.String())
		}
		if len(listResult.Skills) != 1 {
			t.Fatalf("expected 1 skill in listResult, got %d", len(listResult.Skills))
		}
		if listResult.Skills[0].Status != "update_available" {
			t.Fatalf("expected status 'update_available', got %q", listResult.Skills[0].Status)
		}

		// 5. Update state to same -> --exit-code exits 0
		recordUpstreamState(t, root, skillID, sourceID, repo, ref, "skills/pdf", latestCommit, latestCommit, 0, "same")
		stdout.Reset()
		stderr.Reset()
		codeSame := Run([]string{"skill", "outdated", "--exit-code", "--workspace", root}, &stdout, &stderr)
		if codeSame != 0 {
			t.Fatalf("expected exit 0 with --exit-code on up_to_date, got %d", codeSame)
		}
	})
}

func TestSkillUpstream(t *testing.T) {
	t.Parallel()

	// 1. Valid tracked skill prints detail view
	t.Run("tracked skill detail", func(t *testing.T) {
		root := initTestWorkspace(t)
		sourceID := "anthropics-skills"
		repo := "https://github.com/anthropics/skills"
		ref := "main"
		skillID := "pdf"
		baseCommit := "3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a"
		latestCommit := "81d04be7c2aa1111222233334444555566667777"

		seedSourceRecord(t, root, sourceID, repo, ref)
		seedTrackedSkill(t, root, skillID, sourceID, app.SkillOrigin{
			Kind:        "github",
			Repository:  repo,
			Ref:         ref,
			Path:        "skills/pdf",
			Commit:      baseCommit,
			FilesDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		})
		recordUpstreamState(t, root, skillID, sourceID, repo, ref, "skills/pdf", baseCommit, latestCommit, 1, "changed")

		var stdout, stderr bytes.Buffer
		code := Run([]string{"skill", "upstream", skillID, "--workspace", root}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d, stderr: %s", code, stderr.String())
		}
		outStr := stdout.String()
		if !strings.Contains(outStr, "pdf tracks anthropics-skills") {
			t.Fatalf("missing tracking line in output:\n%s", outStr)
		}
		if !strings.Contains(outStr, "update available") {
			t.Fatalf("missing status line in output:\n%s", outStr)
		}
		if !strings.Contains(outStr, "Current 3f9c2a1b4d5e") {
			t.Fatalf("missing current line in output:\n%s", outStr)
		}
		if !strings.Contains(outStr, "Latest 81d04be7c2aa") {
			t.Fatalf("missing latest line in output:\n%s", outStr)
		}
		if !strings.Contains(outStr, "Next: skillhub skill update pdf") {
			t.Fatalf("missing next line in output:\n%s", outStr)
		}

		// JSON mode
		stdout.Reset()
		stderr.Reset()
		codeJSON := Run([]string{"skill", "upstream", skillID, "--workspace", root, "--json"}, &stdout, &stderr)
		if codeJSON != 0 {
			t.Fatalf("expected exit 0 for --json, got %d", codeJSON)
		}
		var upstream app.SkillUpstream
		if err := json.Unmarshal(stdout.Bytes(), &upstream); err != nil {
			t.Fatalf("failed to decode JSON upstream: %v, raw: %s", err, stdout.String())
		}
		if upstream.SkillID != skillID || upstream.Status != "update_available" {
			t.Fatalf("unexpected JSON upstream: %#v", upstream)
		}
	})

	// 2. Non-repository / local skill -> exits 2
	t.Run("local skill exits 2", func(t *testing.T) {
		root := initTestWorkspace(t)
		service := app.SkillService{}
		prev, err := service.PreviewCreate(context.Background(), root, skill.CreateInput{
			ID:          "local-skill",
			Collection:  "default",
			Name:        "Local Skill",
			Description: "Local untracked skill",
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.ConfirmSkillMutation(context.Background(), root, prev, prev.Confirmation.Confirmation.Pins); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		code := Run([]string{"skill", "upstream", "local-skill", "--workspace", root}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("expected exit 2 for non-repository skill, got %d, stdout: %s, stderr: %s", code, stdout.String(), stderr.String())
		}
	})
}

func TestSkillUpdate(t *testing.T) {
	t.Parallel()

	// 1. Flag parsing tests
	t.Run("flag parsing", func(t *testing.T) {
		root := initTestWorkspace(t)

		// --accept with invalid value -> exit 2 naming allowed values
		var stdout, stderr bytes.Buffer
		code := Run([]string{"skill", "update", "foo", "--accept", "SKILL.md=theirs", "--workspace", root}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("expected exit 2 on invalid --accept, got %d", code)
		}
		errStr := stderr.String()
		if !strings.Contains(errStr, "upstream") || !strings.Contains(errStr, "local") || !strings.Contains(errStr, "merged") {
			t.Fatalf("expected allowed values in error, got: %s", errStr)
		}

		// --manual with missing file -> exit 2
		stdout.Reset()
		stderr.Reset()
		code = Run([]string{"skill", "update", "foo", "--manual", "SKILL.md=/nonexistent/path/file.txt", "--workspace", root}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("expected exit 2 on missing manual file, got %d", code)
		}
		if !strings.Contains(stderr.String(), "does not exist") {
			t.Fatalf("expected missing file error, got: %s", stderr.String())
		}
	})

	// 2. Rendering tests
	t.Run("rendering unresolved with hints", func(t *testing.T) {
		preview := app.UpstreamUpdatePreview{
			SkillID:           "docx",
			SourceID:          "anthropics-skills",
			BaseCommit:        "3f9c2a1b4d5e",
			TargetCommit:      "81d04be7c2aa",
			TargetCommittedAt: "2026-10-03T12:00:00Z",
			UnchangedCount:    4,
			Unresolved:        []string{"SKILL.md"},
			Files: []app.UpstreamFile{
				{
					Path:              "SKILL.md",
					Status:            "both_changed",
					Conflicts:         1,
					MergedWithMarkers: "<<<<<<< local\nlocal\n=======\nupstream\n>>>>>>> upstream\n",
				},
				{
					Path:   "scripts/run.sh",
					Status: "upstream_only",
					Action: "upstream",
				},
				{
					Path:   "notes.txt",
					Status: "unchanged",
				},
			},
		}

		var outBuf bytes.Buffer
		p := termui.New(&outBuf)
		renderUpstreamUpdatePreview(p, "docx", "https://github.com/anthropics/skills", preview, false)
		out := outBuf.String()

		// Verify table rows
		if !strings.Contains(out, "SKILL.md") || !strings.Contains(out, "changed here and upstream") || !strings.Contains(out, "needs decision (1 conflict)") {
			t.Fatalf("missing SKILL.md row in output:\n%s", out)
		}
		if !strings.Contains(out, "scripts/run.sh") || !strings.Contains(out, "changed upstream") || !strings.Contains(out, "take upstream") {
			t.Fatalf("missing scripts/run.sh row in output:\n%s", out)
		}
		if !strings.Contains(out, "4 file(s) unchanged.") {
			t.Fatalf("missing unchanged count in output:\n%s", out)
		}

		// Verify three resolution hint lines
		if !strings.Contains(out, "--accept SKILL.md=upstream   take the upstream file (drops your edits in it)") {
			t.Fatalf("missing upstream hint in output:\n%s", out)
		}
		if !strings.Contains(out, "--accept SKILL.md=local      keep your file") {
			t.Fatalf("missing local hint in output:\n%s", out)
		}
		if !strings.Contains(out, "--manual SKILL.md=<file>     use a file you resolved yourself (no conflict markers)") {
			t.Fatalf("missing manual hint in output:\n%s", out)
		}
	})

	t.Run("rendering clean with pins and trust notice", func(t *testing.T) {
		preview := app.UpstreamUpdatePreview{
			SkillID:           "pdf",
			SourceID:          "anthropics-skills",
			BaseCommit:        "3f9c2a1b4d5e",
			TargetCommit:      "81d04be7c2aa",
			TargetCommittedAt: "2026-10-03T12:00:00Z",
			UnchangedCount:    2,
			Files: []app.UpstreamFile{
				{
					Path:       "SKILL.md",
					Status:     "upstream_only",
					Action:     "upstream",
					ResultDiff: "--- a/SKILL.md\n+++ b/SKILL.md\n@@ -1 +1 @@\n-old\n+new\n",
				},
			},
			TrustImpact: app.TrustImpact{
				ReviewRequiredAfterApply: true,
			},
			Confirmation: app.ConfirmationPolicy{
				Confirmation: app.ConfirmationRequirement{
					Pins: app.ConfirmationPins{
						ProposalID:     "PROP-12345",
						ProposalDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
						BaseVersion:    "sha256:2222222222222222222222222222222222222222222222222222222222222222",
					},
				},
			},
		}

		var outBuf bytes.Buffer
		p := termui.New(&outBuf)
		renderUpstreamUpdatePreview(p, "pdf", "https://github.com/anthropics/skills", preview, false)
		out := outBuf.String()

		if !strings.Contains(out, "After applying, agents cannot use pdf until you approve the new content:") {
			t.Fatalf("missing trust notice in output:\n%s", out)
		}
		if !strings.Contains(out, "skillhub skill review pdf") {
			t.Fatalf("missing review command in output:\n%s", out)
		}
		if !strings.Contains(out, "No collection files changed.") {
			t.Fatalf("missing no collection files changed line in output:\n%s", out)
		}
		if !strings.Contains(out, "Next: skillhub skill confirm PROP-12345") {
			t.Fatalf("missing confirm next hint in output:\n%s", out)
		}
	})

	// 3. --write-conflicts with an existing target file -> exit 2
	t.Run("write-conflicts with existing file exits 2", func(t *testing.T) {
		conflictsDir := t.TempDir()
		targetFile := filepath.Join(conflictsDir, "SKILL.md")
		_ = os.WriteFile(targetFile, []byte("existing content"), 0o644)

		files := []app.UpstreamFile{
			{
				Path:              "SKILL.md",
				Conflicts:         1,
				MergedWithMarkers: "conflict markers",
			},
		}
		err := writeConflictsFiles(conflictsDir, files)
		if err == nil {
			t.Fatalf("expected error when conflict file already exists in write-conflicts dir")
		}
	})
}

func TestHelpUpstreamAndSources(t *testing.T) {
	t.Parallel()

	// 1. skill help contains skill update <id>, skill outdated, and distinction line
	var stdout, stderr bytes.Buffer
	code := Run([]string{"skill", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("skill --help failed: %d, %s", code, stderr.String())
	}
	skillHelp := stdout.String()
	if !strings.Contains(skillHelp, "update <id>") {
		t.Fatalf("expected 'update <id>' in skill help, got:\n%s", skillHelp)
	}
	if !strings.Contains(skillHelp, "outdated") {
		t.Fatalf("expected 'outdated' in skill help, got:\n%s", skillHelp)
	}
	if !strings.Contains(skillHelp, "skillhub update") {
		t.Fatalf("expected skillhub update distinction in skill help, got:\n%s", skillHelp)
	}

	// 2. source help contains source backfill
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"source", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("source --help failed: %d, %s", code, stderr.String())
	}
	sourceHelp := stdout.String()
	if !strings.Contains(sourceHelp, "backfill") {
		t.Fatalf("expected 'backfill' in source help, got:\n%s", sourceHelp)
	}
}

func TestPrintableTextEscapesTerminalControlSequences(t *testing.T) {
	t.Parallel()

	maliciousPath := "scripts/\x1b[31mmalicious\nfile.sh"
	escaped := printableText(maliciousPath)

	if strings.Contains(escaped, "\x1b") {
		t.Fatalf("escaped text still contains raw ESC byte: %q", escaped)
	}
	if strings.Contains(escaped, "\n") {
		t.Fatalf("escaped text still contains raw newline byte: %q", escaped)
	}
	if !strings.Contains(escaped, "\\x1b") {
		t.Fatalf("expected \\x1b escape in text, got: %q", escaped)
	}
	if !strings.Contains(escaped, "\\x0a") {
		t.Fatalf("expected \\x0a escape in text, got: %q", escaped)
	}

	// Verify rendering test with malicious path
	preview := app.UpstreamUpdatePreview{
		SkillID:        "sec-skill",
		SourceID:       "src-sec",
		BaseCommit:     "111122223333",
		TargetCommit:   "444455556666",
		UnchangedCount: 0,
		Files: []app.UpstreamFile{
			{
				Path:   maliciousPath,
				Status: "upstream_only",
				Action: "upstream",
			},
		},
		Confirmation: app.ConfirmationPolicy{
			Confirmation: app.ConfirmationRequirement{
				Pins: app.ConfirmationPins{ProposalID: "PROP-SEC"},
			},
		},
	}
	var outBuf bytes.Buffer
	p := termui.New(&outBuf)
	renderUpstreamUpdatePreview(p, "sec-skill", "https://github.com/example/repo", preview, false)
	out := outBuf.String()

	if strings.Contains(out, "\x1b[31m") {
		t.Fatalf("rendered output contains raw ANSI escape sequence: %q", out)
	}
	if !strings.Contains(out, "\\x1b") {
		t.Fatalf("rendered output missing escaped \\x1b text: %q", out)
	}
}
