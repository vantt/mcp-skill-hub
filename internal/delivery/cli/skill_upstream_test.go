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

	metaPath := filepath.Join(root, "skills", "default", skillID, "skill.meta.yaml")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(metaBytes, &doc); err != nil {
		t.Fatal(err)
	}

	skillMDBytes, _ := os.ReadFile(filepath.Join(root, "skills", "default", skillID, "SKILL.md"))
	skillMDDigest := sourcepkg.Digest(skillMDBytes)
	cleanFilesDigest := skillruntime.ContentDigest([]skillruntime.ResourceDigest{{Path: "SKILL.md", Digest: skillMDDigest}}, skillruntime.Spec{}, false)

	filesDigest := origin.FilesDigest
	if filesDigest == "" || strings.HasPrefix(filesDigest, "sha256:00000000") {
		filesDigest = cleanFilesDigest
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
