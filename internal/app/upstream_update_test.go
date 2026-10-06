package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func runGitInDir(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v: %s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func setupUpstreamUpdateHarness(t *testing.T) (string, string, string, string, UpstreamService, SkillAddService, sourcepkg.GitRepositoryAdapter) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	root := newSourceWorkspace(t)
	repoDir := t.TempDir()

	runGitInDir(t, repoDir, "init", "-b", "main")
	runGitInDir(t, repoDir, "config", "user.name", "Test")
	runGitInDir(t, repoDir, "config", "user.email", "test@example.com")
	runGitInDir(t, repoDir, "config", "uploadpack.allowReachableSHA1InWant", "true")

	skillDir := filepath.Join(repoDir, "skills", "my-skill")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatal(err)
	}
	initialContent := "---\nname: my-skill\ndescription: Original description\n---\nline 1\nline 2\nline 3\nline 4\nline 5\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(initialContent), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitInDir(t, repoDir, "add", ".")
	runGitInDir(t, repoDir, "commit", "-m", "initial base commit")
	baseCommit := runGitInDir(t, repoDir, "rev-parse", "HEAD")
	_ = baseCommit

	adapter := sourcepkg.GitRepositoryAdapter{
		CacheRoot:         filepath.Join(root, "runtime", "sources", "git"),
		AllowFileProtocol: true,
	}
	now := time.Now().UTC()
	addService := SkillAddService{
		Clock:    sourceClock{now: now},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}
	upstreamService := UpstreamService{
		Clock:    sourceClock{now: now},
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	fileURL := "file://" + filepath.ToSlash(repoDir)
	preview, err := addService.PreviewSkillAdd(context.Background(), root, SkillAddInput{
		Locator: fileURL,
		All:     true,
	})
	if err != nil || preview.Error != nil {
		t.Fatalf("preview add failed: %v, %#v", err, preview.Error)
	}
	pins := preview.Confirmation.Confirmation.Pins
	result, err := addService.ConfirmSkillAdd(context.Background(), root, preview, pins)
	if err != nil || result.Error != nil {
		t.Fatalf("confirm add failed: %v, %#v", err, result.Error)
	}
	sourceID := result.UpstreamSource.SourceID

	// Approve initial content trust so CurrentlyApproved starts true
	trust, err := (SkillService{}).ContentTrustFor(context.Background(), root, "my-skill")
	if err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(root, "skills", "default", "my-skill", "skill.meta.yaml")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	approvedMeta := strings.Replace(string(metaData), "reviewed: false", fmt.Sprintf("reviewed: true\n    content_reviewed_digest: %s", trust.ContentDigest), 1)
	if err := os.WriteFile(metaPath, []byte(approvedMeta), 0o644); err != nil {
		t.Fatal(err)
	}

	return root, repoDir, "my-skill", sourceID, upstreamService, addService, adapter
}

func TestUpstreamUpdate(t *testing.T) {
	// Subtest 1: clean update
	t.Run("clean", func(t *testing.T) {
		root, repoDir, skillID, sourceID, upstreamService, _, adapter := setupUpstreamUpdateHarness(t)
		ctx := context.Background()

		// Upstream edits SKILL.md
		skillDir := filepath.Join(repoDir, "skills", skillID)
		updatedContent := "---\nname: my-skill\ndescription: Original description\n---\nline 1\nline 2-upstream\nline 3\nline 4\nline 5\n"
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(updatedContent), 0o644)
		runGitInDir(t, repoDir, "add", ".")
		runGitInDir(t, repoDir, "commit", "-m", "upstream edit")
		headU := runGitInDir(t, repoDir, "rev-parse", "HEAD")

		sourceService := SourceService{Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
		_, _ = sourceService.CheckSources(ctx, root, []string{sourceID}, false)

		preview, err := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{SkillID: skillID})
		if err != nil || preview.Error != nil {
			t.Fatalf("preview failed: %v, %#v", err, preview.Error)
		}
		if len(preview.Unresolved) != 0 {
			t.Fatalf("expected 0 unresolved, got %v", preview.Unresolved)
		}
		if preview.Confirmation.Confirmation.Pins.ProposalID == "" {
			t.Fatal("expected non-empty proposal ID")
		}

		confirmResult, err := upstreamService.ConfirmUpdate(ctx, root, preview, preview.Confirmation.Confirmation.Pins)
		if err != nil || confirmResult.Error != nil {
			t.Fatalf("confirm failed: %v, %#v", err, confirmResult.Error)
		}
		if !strings.Contains(confirmResult.Summary, "skillhub skill review my-skill") {
			t.Fatalf("expected summary to contain review command, got: %s", confirmResult.Summary)
		}

		// local SKILL.md equals transformed upstream
		localMD, _ := os.ReadFile(filepath.Join(root, "skills", "default", skillID, "SKILL.md"))
		if !strings.Contains(string(localMD), "line 2-upstream") {
			t.Fatalf("expected updated content in local SKILL.md, got:\n%s", string(localMD))
		}

		// meta origin.commit == U
		metaData, _ := os.ReadFile(filepath.Join(root, "skills", "default", skillID, "skill.meta.yaml"))
		if !strings.Contains(string(metaData), "commit: "+headU) {
			t.Fatalf("expected origin.commit %s, got:\n%s", headU, string(metaData))
		}

		// ContentTrust reports content_review_stale / unapproved
		trustAfter, _ := (SkillService{}).ContentTrustFor(ctx, root, skillID)
		if trustAfter.Approved {
			t.Fatal("skill should return to unapproved (review_required) after update")
		}

		// GetSkillUpstream reports up_to_date
		skUpstream, err := GetSkillUpstream(ctx, root, skillID)
		if err != nil || skUpstream.Status != "up_to_date" {
			t.Fatalf("expected up_to_date, got status=%s, err=%v", skUpstream.Status, err)
		}
	})

	// Subtest 2: merged
	t.Run("merged", func(t *testing.T) {
		root, repoDir, skillID, sourceID, upstreamService, _, adapter := setupUpstreamUpdateHarness(t)
		ctx := context.Background()

		// Local edits line 1
		localMD := filepath.Join(root, "skills", "default", skillID, "SKILL.md")
		_ = os.WriteFile(localMD, []byte("---\nname: my-skill\ndescription: Original description\n---\nline 1-local\nline 2\nline 3\nline 4\nline 5\n"), 0o644)

		// Upstream edits line 5
		skillDir := filepath.Join(repoDir, "skills", skillID)
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\ndescription: Original description\n---\nline 1\nline 2\nline 3\nline 4\nline 5-upstream\n"), 0o644)
		runGitInDir(t, repoDir, "add", ".")
		runGitInDir(t, repoDir, "commit", "-m", "upstream edit line 5")

		sourceService := SourceService{Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
		_, _ = sourceService.CheckSources(ctx, root, []string{sourceID}, false)

		preview, err := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{SkillID: skillID})
		if err != nil || preview.Error != nil {
			t.Fatalf("preview failed: %v, %#v", err, preview.Error)
		}
		if len(preview.Unresolved) != 0 {
			t.Fatalf("expected 0 unresolved for clean merge, got %v", preview.Unresolved)
		}

		res, err := upstreamService.ConfirmUpdate(ctx, root, preview, preview.Confirmation.Confirmation.Pins)
		if err != nil || res.Error != nil {
			t.Fatalf("confirm failed: %v, %#v", err, res.Error)
		}

		mergedData, _ := os.ReadFile(localMD)
		mergedStr := string(mergedData)
		if !strings.Contains(mergedStr, "line 1-local") || !strings.Contains(mergedStr, "line 5-upstream") {
			t.Fatalf("expected merged content with both edits, got:\n%s", mergedStr)
		}
	})

	// Subtest 3: conflict
	t.Run("conflict", func(t *testing.T) {
		root, repoDir, skillID, sourceID, upstreamService, _, adapter := setupUpstreamUpdateHarness(t)
		ctx := context.Background()

		// Both edit line 3
		localMD := filepath.Join(root, "skills", "default", skillID, "SKILL.md")
		_ = os.WriteFile(localMD, []byte("---\nname: my-skill\ndescription: Original description\n---\nline 1\nline 2\nline 3-local\nline 4\nline 5\n"), 0o644)

		skillDir := filepath.Join(repoDir, "skills", skillID)
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\ndescription: Original description\n---\nline 1\nline 2\nline 3-upstream\nline 4\nline 5\n"), 0o644)
		runGitInDir(t, repoDir, "add", ".")
		runGitInDir(t, repoDir, "commit", "-m", "upstream edit line 3")

		sourceService := SourceService{Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
		_, _ = sourceService.CheckSources(ctx, root, []string{sourceID}, false)

		preview, err := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{SkillID: skillID})
		if err != nil || preview.Error != nil {
			t.Fatalf("preview failed: %v, %#v", err, preview.Error)
		}
		if len(preview.Unresolved) != 1 || preview.Unresolved[0] != "SKILL.md" {
			t.Fatalf("expected unresolved SKILL.md, got %v", preview.Unresolved)
		}
		if preview.Confirmation.Confirmation.Pins.ProposalID != "" {
			t.Fatal("expected empty pins when unresolved")
		}

		// Resolution manual with marker text -> invalid_request
		_, err = upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{
			SkillID: skillID,
			Resolutions: []UpstreamResolution{
				{Path: "SKILL.md", Action: "manual", Content: "line 1\n<<<<<<< local\nline 3-local\n=======\nline 3-upstream\n>>>>>>> upstream\n"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Resolution manual with clean text -> pins filled
		cleanContent := "---\nname: my-skill\ndescription: Original description\n---\nline 1\nline 2\nline 3-resolved\nline 4\nline 5\n"
		cleanPreview, err := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{
			SkillID: skillID,
			Resolutions: []UpstreamResolution{
				{Path: "SKILL.md", Action: "manual", Content: cleanContent},
			},
		})
		if err != nil || cleanPreview.Error != nil {
			t.Fatalf("clean preview failed: %v, %#v", err, cleanPreview.Error)
		}
		if cleanPreview.Confirmation.Confirmation.Pins.ProposalID == "" {
			t.Fatal("expected non-empty proposal ID with resolution")
		}

		res, err := upstreamService.ConfirmUpdate(ctx, root, cleanPreview, cleanPreview.Confirmation.Confirmation.Pins)
		if err != nil || res.Error != nil {
			t.Fatalf("confirm failed: %v, %#v", err, res.Error)
		}
		// status afterwards: local is modified because cleanContent differs from transformed upstream
		skUpstream, _ := GetSkillUpstream(ctx, root, skillID)
		if skUpstream.Status != "modified" {
			t.Fatalf("expected status modified after manual resolution, got %s", skUpstream.Status)
		}
	})

	// Subtest 4: removed-upstream-with-local-edit
	t.Run("removed-upstream-with-local-edit", func(t *testing.T) {
		root, repoDir, skillID, sourceID, upstreamService, _, adapter := setupUpstreamUpdateHarness(t)
		ctx := context.Background()

		// Add companion file helper.txt upstream first and sync
		skillDir := filepath.Join(repoDir, "skills", skillID)
		_ = os.WriteFile(filepath.Join(skillDir, "helper.txt"), []byte("helper base\n"), 0o644)
		runGitInDir(t, repoDir, "add", ".")
		runGitInDir(t, repoDir, "commit", "-m", "add helper")

		sourceService := SourceService{Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
		_, _ = sourceService.CheckSources(ctx, root, []string{sourceID}, false)
		p, _ := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{SkillID: skillID})
		_, _ = upstreamService.ConfirmUpdate(ctx, root, p, p.Confirmation.Confirmation.Pins)

		// Local edits helper.txt
		localHelper := filepath.Join(root, "skills", "default", skillID, "helper.txt")
		_ = os.WriteFile(localHelper, []byte("helper local edit\n"), 0o644)

		// Upstream removes helper.txt
		runGitInDir(t, repoDir, "rm", "skills/"+skillID+"/helper.txt")
		runGitInDir(t, repoDir, "commit", "-m", "remove helper upstream")

		_, _ = sourceService.CheckSources(ctx, root, []string{sourceID}, false)

		preview, err := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{SkillID: skillID})
		if err != nil || preview.Error != nil {
			t.Fatalf("preview failed: %v, %#v", err, preview.Error)
		}
		if len(preview.Unresolved) != 1 || preview.Unresolved[0] != "helper.txt" {
			t.Fatalf("expected unresolved helper.txt, got %v", preview.Unresolved)
		}

		// Resolve with action: local keeps the file
		resolvedPreview, err := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{
			SkillID: skillID,
			Resolutions: []UpstreamResolution{
				{Path: "helper.txt", Action: "local"},
			},
		})
		if err != nil || resolvedPreview.Error != nil {
			t.Fatalf("resolved preview failed: %v, %#v", err, resolvedPreview.Error)
		}
		_, err = upstreamService.ConfirmUpdate(ctx, root, resolvedPreview, resolvedPreview.Confirmation.Confirmation.Pins)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(localHelper); err != nil {
			t.Fatalf("expected helper.txt to be kept locally: %v", err)
		}
	})

	// Subtest 5: stale
	t.Run("stale", func(t *testing.T) {
		root, repoDir, skillID, sourceID, upstreamService, _, adapter := setupUpstreamUpdateHarness(t)
		ctx := context.Background()

		skillDir := filepath.Join(repoDir, "skills", skillID)
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\ndescription: Original description\n---\nupstream change\n"), 0o644)
		runGitInDir(t, repoDir, "add", ".")
		runGitInDir(t, repoDir, "commit", "-m", "upstream change")

		sourceService := SourceService{Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
		_, _ = sourceService.CheckSources(ctx, root, []string{sourceID}, false)

		preview, err := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{SkillID: skillID})
		if err != nil || preview.Error != nil {
			t.Fatal(err)
		}

		// Local edit AFTER preview
		localMD := filepath.Join(root, "skills", "default", skillID, "SKILL.md")
		_ = os.WriteFile(localMD, []byte("intervening local edit\n"), 0o644)

		res, err := upstreamService.ConfirmUpdate(ctx, root, preview, preview.Confirmation.Confirmation.Pins)
		if err != nil {
			t.Fatal(err)
		}
		if res.Error == nil || res.Error.Code != "stale_proposal" {
			t.Fatalf("expected stale_proposal error, got %#v", res.Error)
		}
	})

	// Subtest 6: base-mismatch
	t.Run("base-mismatch", func(t *testing.T) {
		root, repoDir, skillID, sourceID, upstreamService, _, adapter := setupUpstreamUpdateHarness(t)
		ctx := context.Background()

		skillDir := filepath.Join(repoDir, "skills", skillID)
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\ndescription: Original description\n---\nupstream change\n"), 0o644)
		runGitInDir(t, repoDir, "add", ".")
		runGitInDir(t, repoDir, "commit", "-m", "upstream change")

		sourceService := SourceService{Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
		_, _ = sourceService.CheckSources(ctx, root, []string{sourceID}, false)

		// Overwrite origin.files_digest with a different valid digest
		metaPath := filepath.Join(root, "skills", "default", skillID, "skill.meta.yaml")
		metaData, _ := os.ReadFile(metaPath)
		fakeDigest := "sha256:9999999999999999999999999999999999999999999999999999999999999999"
		lines := strings.Split(string(metaData), "\n")
		for i, l := range lines {
			if strings.Contains(l, "files_digest:") {
				indent := l[:strings.Index(l, "files_digest:")]
				lines[i] = indent + "files_digest: " + fakeDigest
			}
		}
		_ = os.WriteFile(metaPath, []byte(strings.Join(lines, "\n")), 0o644)
		preview, err := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{SkillID: skillID})
		if err != nil || preview.Error != nil {
			t.Fatalf("preview failed: %v, %#v", err, preview.Error)
		}
		if preview.BaseAvailable {
			t.Fatal("expected BaseAvailable == false on digest mismatch")
		}
		if len(preview.Unresolved) == 0 {
			t.Fatal("expected unresolved files when base is unavailable")
		}
	})

	// Subtest 7: confirm-via-dispatch
	t.Run("confirm-via-dispatch", func(t *testing.T) {
		root, repoDir, skillID, sourceID, upstreamService, _, adapter := setupUpstreamUpdateHarness(t)
		ctx := context.Background()

		skillDir := filepath.Join(repoDir, "skills", skillID)
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\ndescription: Original description\n---\nupstream dispatch test\n"), 0o644)
		runGitInDir(t, repoDir, "add", ".")
		runGitInDir(t, repoDir, "commit", "-m", "dispatch edit")
		headU := runGitInDir(t, repoDir, "rev-parse", "HEAD")

		sourceService := SourceService{Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
		_, _ = sourceService.CheckSources(ctx, root, []string{sourceID}, false)

		preview, err := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{SkillID: skillID})
		if err != nil || preview.Error != nil {
			t.Fatal(err)
		}

		skillService := SkillService{}
		pins := preview.Confirmation.Confirmation.Pins
		dispatched, err := skillService.DispatchConfirmProposal(ctx, root, preview.Confirmation.Confirmation.Pins.ProposalID, &pins)
		if err != nil {
			t.Fatalf("dispatch failed: %v", err)
		}
		result, ok := dispatched.(UpstreamUpdateResult)
		if !ok || result.Error != nil {
			t.Fatalf("expected UpstreamUpdateResult, got %#v", dispatched)
		}
		if !result.TrustImpact.ReviewRequiredAfterApply {
			t.Fatal("expected ReviewRequiredAfterApply to be true")
		}

		st, _ := GetSkillUpstream(ctx, root, skillID)
		if st.BaseCommit != headU {
			t.Fatalf("expected BaseCommit == %s, got %s", headU, st.BaseCommit)
		}
	})

	// Subtest 8: kind-guard
	t.Run("kind-guard", func(t *testing.T) {
		root, repoDir, skillID, sourceID, upstreamService, _, adapter := setupUpstreamUpdateHarness(t)
		ctx := context.Background()

		skillDir := filepath.Join(repoDir, "skills", skillID)
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\ndescription: Original description\n---\nkind guard\n"), 0o644)
		runGitInDir(t, repoDir, "add", ".")
		runGitInDir(t, repoDir, "commit", "-m", "kind guard edit")

		sourceService := SourceService{Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
		_, _ = sourceService.CheckSources(ctx, root, []string{sourceID}, false)

		preview, err := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{SkillID: skillID})
		if err != nil || preview.Error != nil {
			t.Fatal(err)
		}

		// SkillService.LoadSkillProposal returns invalid_request for upstream_update proposal
		loaded, err := (SkillService{}).LoadSkillProposal(ctx, root, preview.Confirmation.Confirmation.Pins.ProposalID)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Error == nil || loaded.Error.Code != "invalid_request" {
			t.Fatalf("expected invalid_request from LoadSkillProposal, got %#v", loaded.Error)
		}
	})

	// Subtest 9: base-fetched-by-sha
	t.Run("base-fetched-by-sha", func(t *testing.T) {
		root, repoDir, skillID, sourceID, upstreamService, _, adapter := setupUpstreamUpdateHarness(t)
		ctx := context.Background()

		skillDir := filepath.Join(repoDir, "skills", skillID)
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\ndescription: Original description\n---\nsha fetch test\n"), 0o644)
		runGitInDir(t, repoDir, "add", ".")
		runGitInDir(t, repoDir, "commit", "-m", "sha fetch edit")

		sourceService := SourceService{Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
		_, _ = sourceService.CheckSources(ctx, root, []string{sourceID}, false)

		// Delete local mirror directory before preview to test shallow SHA fetch
		_ = os.RemoveAll(filepath.Join(root, "runtime", "sources", "git"))

		preview, err := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{SkillID: skillID})
		if err != nil || preview.Error != nil {
			t.Fatalf("preview failed after mirror wipe: %v, %#v", err, preview.Error)
		}
		if !preview.BaseAvailable {
			t.Fatal("expected BaseAvailable == true even when mirror was wiped")
		}
	})

	// Subtest 10: no-description-and-nested
	t.Run("no-description-and-nested", func(t *testing.T) {
		root := newSourceWorkspace(t)
		repoDir := t.TempDir()
		runGitInDir(t, repoDir, "init", "-b", "main")
		runGitInDir(t, repoDir, "config", "user.name", "Test")
		runGitInDir(t, repoDir, "config", "user.email", "test@example.com")
		runGitInDir(t, repoDir, "config", "uploadpack.allowReachableSHA1InWant", "true")

		parentDir := filepath.Join(repoDir, "skills", "parent")
		nestedDir := filepath.Join(parentDir, "sub")
		_ = os.MkdirAll(nestedDir, 0o700)
		// parent has NO description
		_ = os.WriteFile(filepath.Join(parentDir, "SKILL.md"), []byte("---\nname: parent\n---\nParent body\n"), 0o644)
		// nested has its own SKILL.md
		_ = os.WriteFile(filepath.Join(nestedDir, "SKILL.md"), []byte("---\nname: sub\ndescription: Sub\n---\nSub body\n"), 0o644)
		runGitInDir(t, repoDir, "add", ".")
		runGitInDir(t, repoDir, "commit", "-m", "parent and nested")

		adapter := sourcepkg.GitRepositoryAdapter{
			CacheRoot:         filepath.Join(root, "runtime", "sources", "git"),
			AllowFileProtocol: true,
		}
		addService := SkillAddService{
			Clock:    sourceClock{now: time.Now().UTC()},
			Adapters: map[string]sourcepkg.Adapter{"git": adapter},
		}
		fileURL := "file://" + filepath.ToSlash(repoDir)
		prevAdd, err := addService.PreviewSkillAdd(context.Background(), root, SkillAddInput{
			Locator:   fileURL,
			Selection: "parent",
		})
		if err != nil || prevAdd.Error != nil {
			t.Fatalf("preview add parent: %v, %#v", err, prevAdd.Error)
		}
		_, err = addService.ConfirmSkillAdd(context.Background(), root, prevAdd, prevAdd.Confirmation.Confirmation.Pins)
		if err != nil {
			t.Fatal(err)
		}

		// Upstream edits parent's SKILL.md
		_ = os.WriteFile(filepath.Join(parentDir, "SKILL.md"), []byte("---\nname: parent\n---\nParent body updated\n"), 0o644)
		runGitInDir(t, repoDir, "add", ".")
		runGitInDir(t, repoDir, "commit", "-m", "parent edit")

		sourceService := SourceService{Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
		_, _ = sourceService.CheckSources(context.Background(), root, []string{prevAdd.UpstreamSource.SourceID}, false)

		upstreamService := UpstreamService{
			Clock:    sourceClock{now: time.Now().UTC()},
			Adapters: map[string]sourcepkg.Adapter{"git": adapter},
		}
		prevUpd, err := upstreamService.PreviewUpdate(context.Background(), root, UpstreamUpdateInput{SkillID: "parent"})
		if err != nil || prevUpd.Error != nil {
			t.Fatalf("preview update parent: %v, %#v", err, prevUpd.Error)
		}
		if !prevUpd.BaseAvailable {
			t.Fatal("expected BaseAvailable == true")
		}
		for _, f := range prevUpd.Files {
			if strings.HasPrefix(f.Path, "sub/") {
				t.Fatalf("nested file %s should not appear in parent's update files", f.Path)
			}
		}

		confRes, err := upstreamService.ConfirmUpdate(context.Background(), root, prevUpd, prevUpd.Confirmation.Confirmation.Pins)
		if err != nil || confRes.Error != nil {
			t.Fatalf("confirm update parent: %v, %#v", err, confRes.Error)
		}
	})

	// Subtest 11: rejected-paths
	t.Run("rejected-paths", func(t *testing.T) {
		root, repoDir, skillID, sourceID, upstreamService, _, adapter := setupUpstreamUpdateHarness(t)
		ctx := context.Background()

		skillDir := filepath.Join(repoDir, "skills", skillID)
		// 1. upstream file containing a line ======= is blocked
		_ = os.WriteFile(filepath.Join(skillDir, "heading.md"), []byte("Heading\n=======\n"), 0o644)
		// 2. SKILL.META.YAML is ignored with warning
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.META.YAML"), []byte("ignored\n"), 0o644)
		runGitInDir(t, repoDir, "add", ".")
		runGitInDir(t, repoDir, "commit", "-m", "heading and meta")

		sourceService := SourceService{Adapters: map[string]sourcepkg.Adapter{"git": adapter}}
		_, _ = sourceService.CheckSources(ctx, root, []string{sourceID}, false)

		preview, err := upstreamService.PreviewUpdate(ctx, root, UpstreamUpdateInput{SkillID: skillID})
		if err != nil || preview.Error != nil {
			t.Fatalf("preview failed: %v, %#v", err, preview.Error)
		}
		hasBlocked := false
		for _, f := range preview.Files {
			if f.Path == "heading.md" && f.Status == "blocked" {
				hasBlocked = true
			}
			if strings.EqualFold(f.Path, "skill.meta.yaml") {
				t.Fatalf("skill.meta.yaml case variant should be ignored, found: %s", f.Path)
			}
		}
		if !hasBlocked {
			t.Fatal("expected heading.md to have status blocked")
		}
		hasMetaIgnoredWarning := false
		for _, w := range preview.Warnings {
			if w.Code == "upstream_meta_ignored" {
				hasMetaIgnoredWarning = true
			}
		}
		if !hasMetaIgnoredWarning {
			t.Fatal("expected upstream_meta_ignored warning")
		}
	})

	// Subtest 12: planned-before-digest
	t.Run("planned-before-digest", func(t *testing.T) {
		root, _, skillID, _, _, _, adapter := setupUpstreamUpdateHarness(t)
		ctx := context.Background()

		now := time.Now().UTC()
		wc := updateWriteContext{
			Root:           root,
			SkillID:        skillID,
			SkillRelDir:    "skills/default/" + skillID,
			TargetCommit:   "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222",
			IdempotencyKey: "test-idemp",
			MetaBytes:      []byte("schema_version: 1\nid: my-skill\nname: my-skill\nstatus: draft\ndescription: d\n"),
			Tracked: TrackedSkill{
				SkillID:     skillID,
				SkillRelDir: "skills/default/" + skillID,
				SourceID:    "src-1",
				Origin: SkillOrigin{
					Kind:       "github",
					Repository: "https://github.com/example/repo",
					Ref:        "main",
					Commit:     "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111",
					Path:       "skills/" + skillID,
				},
			},
			SourceRec: &sourcepkg.Record{
				ID:      "src-1",
				Adapter: "git",
				Locator: sourcepkg.Locator{Repository: "https://github.com/example/repo", Ref: "main"},
			},
			Adapter: adapter,
			LocalFiles: map[string][]byte{
				"SKILL.md": []byte("original\n"),
			},
			ResolvedBytes: map[string][]byte{
				"new.txt": []byte("new file\n"),
			},
			IsDeleteFile: map[string]bool{},
			Now:          now,
		}

		// Inject a local file before planning that was absent in LocalFiles (expectedBeforeDigests has "")
		newFilePath := filepath.Join(root, "skills", "default", skillID, "new.txt")
		_ = os.WriteFile(newFilePath, []byte("intervening file\n"), 0o644)

		_, _, _, err := buildAndPlanUpstreamWriteSet(ctx, wc)
		if err == nil {
			t.Fatal("expected stale_proposal error when intervening file created on disk")
		}
		var appErr *Error
		if errors.As(err, &appErr) && appErr.Code != "stale_proposal" {
			t.Fatalf("expected code stale_proposal, got %s", appErr.Code)
		}
	})
}
