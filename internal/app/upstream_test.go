package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func TestDeriveUpstreamStatus(t *testing.T) {
	validDigest := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	modifiedDigest := "sha256:2222222222222222222222222222222222222222222222222222222222222222"

	baseOrigin := SkillOrigin{
		Kind:        "github",
		Repository:  "https://github.com/example/skills",
		Ref:         "main",
		Commit:      "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111",
		Path:        "skills/pdf",
		FilesDigest: validDigest,
	}

	baseRecord := &sourcepkg.Record{
		ID:      "example-skills",
		Adapter: "git",
		Locator: sourcepkg.Locator{
			Repository: "https://github.com/example/skills",
			Ref:        "main",
		},
	}

	baseState := &sourcepkg.UpstreamState{
		SkillID:        "pdf",
		SourceID:       "example-skills",
		Repository:     "https://github.com/example/skills",
		Ref:            "main",
		Path:           "skills/pdf",
		BaseCommit:     "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111",
		CheckedCommit:  "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222",
		Upstream:       "same",
		UpstreamDigest: validDigest,
	}

	cases := []struct {
		name           string
		origin         SkillOrigin
		sourceID       string
		sourceRec      *sourcepkg.Record
		state          *sourcepkg.UpstreamState
		currentLocal   string
		expectedStatus string
		expectedLocal  string
		expectedErr    string
	}{
		{
			name:           "untracked github without sourceID",
			origin:         baseOrigin,
			sourceID:       "",
			sourceRec:      baseRecord,
			state:          baseState,
			currentLocal:   validDigest,
			expectedStatus: "untracked",
			expectedLocal:  "clean",
		},
		{
			name: "untracked git without sourceID",
			origin: func() SkillOrigin {
				o := baseOrigin
				o.Kind = "git"
				return o
			}(),
			sourceID:       "",
			sourceRec:      baseRecord,
			state:          baseState,
			currentLocal:   modifiedDigest,
			expectedStatus: "untracked",
			expectedLocal:  "modified",
		},
		{
			name: "pinned 40 lowercase hex ref",
			origin: func() SkillOrigin {
				o := baseOrigin
				o.Ref = "1234567890abcdef1234567890abcdef12345678"
				return o
			}(),
			sourceID:       "example-skills",
			sourceRec:      baseRecord,
			state:          baseState,
			currentLocal:   validDigest,
			expectedStatus: "pinned",
			expectedLocal:  "clean",
		},
		{
			name:           "unavailable missing source record",
			origin:         baseOrigin,
			sourceID:       "example-skills",
			sourceRec:      nil,
			state:          baseState,
			currentLocal:   validDigest,
			expectedStatus: "unavailable",
			expectedLocal:  "clean",
			expectedErr:    "source_origin_mismatch",
		},
		{
			name:     "unavailable source record repo mismatch",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: &sourcepkg.Record{
				ID:      "example-skills",
				Adapter: "git",
				Locator: sourcepkg.Locator{
					Repository: "https://github.com/other/skills",
					Ref:        "main",
				},
			},
			state:          baseState,
			currentLocal:   validDigest,
			expectedStatus: "unavailable",
			expectedLocal:  "clean",
			expectedErr:    "source_origin_mismatch",
		},
		{
			name:     "unavailable source record ref mismatch",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: &sourcepkg.Record{
				ID:      "example-skills",
				Adapter: "git",
				Locator: sourcepkg.Locator{
					Repository: "https://github.com/example/skills",
					Ref:        "dev",
				},
			},
			state:          baseState,
			currentLocal:   validDigest,
			expectedStatus: "unavailable",
			expectedLocal:  "clean",
			expectedErr:    "source_origin_mismatch",
		},
		{
			name:           "unknown state is nil",
			origin:         baseOrigin,
			sourceID:       "example-skills",
			sourceRec:      baseRecord,
			state:          nil,
			currentLocal:   validDigest,
			expectedStatus: "unknown",
			expectedLocal:  "clean",
		},
		{
			name:      "unknown state base commit mismatch",
			origin:    baseOrigin,
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.BaseCommit = "different-base-commit"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "unknown",
			expectedLocal:  "clean",
		},
		{
			name:      "unknown state repository mismatch",
			origin:    baseOrigin,
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Repository = "https://github.com/other/skills"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "unknown",
			expectedLocal:  "clean",
		},
		{
			name:      "unknown state ref mismatch",
			origin:    baseOrigin,
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Ref = "dev"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "unknown",
			expectedLocal:  "clean",
		},
		{
			name:      "unknown state path mismatch",
			origin:    baseOrigin,
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Path = "skills/other"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "unknown",
			expectedLocal:  "clean",
		},
		{
			name:      "state unavailable",
			origin:    baseOrigin,
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "unavailable"
				s.LastError = "git remote unreachable"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "unavailable",
			expectedLocal:  "clean",
			expectedErr:    "git remote unreachable",
		},
		{
			name:      "state removed yields upstream_removed",
			origin:    baseOrigin,
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "removed"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "upstream_removed",
			expectedLocal:  "clean",
		},
		{
			name:      "same + clean yields up_to_date",
			origin:    baseOrigin,
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "same"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "up_to_date",
			expectedLocal:  "clean",
		},
		{
			name: "same + unknown local yields up_to_date",
			origin: func() SkillOrigin {
				o := baseOrigin
				o.FilesDigest = ""
				return o
			}(),
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "same"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "up_to_date",
			expectedLocal:  "unknown",
		},
		{
			name:      "same + modified local yields modified",
			origin:    baseOrigin,
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "same"
				return &s
			}(),
			currentLocal:   modifiedDigest,
			expectedStatus: "modified",
			expectedLocal:  "modified",
		},
		{
			name:      "changed + clean yields update_available",
			origin:    baseOrigin,
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "changed"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "update_available",
			expectedLocal:  "clean",
		},
		{
			name: "changed + unknown local yields update_available",
			origin: func() SkillOrigin {
				o := baseOrigin
				o.FilesDigest = ""
				return o
			}(),
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "changed"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "update_available",
			expectedLocal:  "unknown",
		},
		{
			name:      "changed + modified local yields diverged",
			origin:    baseOrigin,
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "changed"
				return &s
			}(),
			currentLocal:   modifiedDigest,
			expectedStatus: "diverged",
			expectedLocal:  "modified",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, local, errStr := deriveUpstreamStatus(tc.origin, tc.sourceID, tc.sourceRec, tc.state, tc.currentLocal)
			if status != tc.expectedStatus {
				t.Fatalf("expected status %q, got %q", tc.expectedStatus, status)
			}
			if local != tc.expectedLocal {
				t.Fatalf("expected local %q, got %q", tc.expectedLocal, local)
			}
			if tc.expectedErr != "" && errStr != tc.expectedErr {
				t.Fatalf("expected errStr %q, got %q", tc.expectedErr, errStr)
			}
		})
	}
}

func TestUpstreamCheck(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	root := newSourceWorkspace(t)
	repoDir := t.TempDir()

	runGit := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	runGit(repoDir, "init", "-b", "main")
	runGit(repoDir, "config", "user.name", "Test")
	runGit(repoDir, "config", "user.email", "test@example.com")
	runGit(repoDir, "config", "uploadpack.allowReachableSHA1InWant", "true")

	// Create skills/a and skills/b
	skillADir := filepath.Join(repoDir, "skills", "a")
	if err := os.MkdirAll(skillADir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillADir, "SKILL.md"), []byte("---\nname: a\ndescription: Skill A\n---\nBody A\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	skillBDir := filepath.Join(repoDir, "skills", "b")
	if err := os.MkdirAll(skillBDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillBDir, "SKILL.md"), []byte("---\nname: b\ndescription: Skill B\n---\nBody B\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runGit(repoDir, "add", ".")
	runGit(repoDir, "commit", "-m", "initial commit")

	adapter := sourcepkg.GitRepositoryAdapter{
		CacheRoot:         filepath.Join(root, "runtime", "sources", "git"),
		AllowFileProtocol: true,
	}
	clock := sourceClock{now: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)}
	addService := SkillAddService{
		Clock:    clock,
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	fileURL := "file://" + filepath.ToSlash(repoDir)

	// Step 1: add skills/a (phases 1-2 path)
	previewA, err := addService.PreviewSkillAdd(context.Background(), root, SkillAddInput{
		Locator:   fileURL,
		Selection: "a",
	})
	if err != nil || previewA.Error != nil {
		t.Fatalf("preview a failed: %v, %#v", err, previewA.Error)
	}
	pinsA := previewA.Confirmation.Confirmation.Pins
	resultA, err := addService.ConfirmSkillAdd(context.Background(), root, previewA, pinsA)
	if err != nil || resultA.Error != nil {
		t.Fatalf("confirm a failed: %v, %#v", err, resultA.Error)
	}

	sourceID := resultA.UpstreamSource.SourceID
	catalogRecordPath := filepath.Join(root, "sources", "catalog", sourceID+".yaml")
	catalogBytesBefore, err := os.ReadFile(catalogRecordPath)
	if err != nil {
		t.Fatalf("read catalog record failed: %v", err)
	}

	sourceService := SourceService{
		Clock:    clock,
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}

	// Initial check: head == origin.commit -> up_to_date
	initCheck, err := sourceService.CheckSources(context.Background(), root, []string{sourceID}, false)
	if err != nil {
		t.Fatalf("initial check failed: %v", err)
	}
	if len(initCheck.Results) != 1 || initCheck.Results[0].Status != "up_to_date" {
		t.Fatalf("expected initial check up_to_date, got %#v", initCheck.Results)
	}

	// Step 2: commit an upstream change to skills/a/SKILL.md
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(skillADir, "SKILL.md"), []byte("---\nname: a\ndescription: Skill A Updated\n---\nBody A Updated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(repoDir, "add", "skills/a/SKILL.md")
	runGit(repoDir, "commit", "-m", "update skill a")
	shaUpdate := runGit(repoDir, "rev-parse", "HEAD")
	commitUnixStr := runGit(repoDir, "show", "-s", "--format=%ct", shaUpdate)
	var commitUnix int64
	_, _ = fmt.Sscanf(commitUnixStr, "%d", &commitUnix)
	expectedCommitterTime := time.Unix(commitUnix, 0).UTC()

	// Call CheckSources(ctx, root, []string{sourceID}, false)
	checkRes, err := sourceService.CheckSources(context.Background(), root, []string{sourceID}, false)
	if err != nil {
		t.Fatalf("CheckSources failed: %v", err)
	}
	if len(checkRes.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(checkRes.Results))
	}
	if checkRes.Results[0].Status != "updates_available" {
		t.Fatalf("expected status updates_available, got %q", checkRes.Results[0].Status)
	}
	if len(checkRes.Results[0].Skills) != 1 || checkRes.Results[0].Skills[0].Status != "update_available" {
		t.Fatalf("expected skill status update_available, got %#v", checkRes.Results[0].Skills)
	}
	if checkRes.Results[0].Skills[0].ChangedFiles != 1 {
		t.Fatalf("expected ChangedFiles == 1, got %d", checkRes.Results[0].Skills[0].ChangedFiles)
	}
	expectedTimeStr := expectedCommitterTime.Format(time.RFC3339)
	if checkRes.Results[0].Skills[0].LatestCommittedAt != expectedTimeStr {
		t.Fatalf("expected LatestCommittedAt %q, got %q", expectedTimeStr, checkRes.Results[0].Skills[0].LatestCommittedAt)
	}

	// Assert sources/catalog/<id>.yaml bytes unchanged
	catalogBytesAfter, err := os.ReadFile(catalogRecordPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(catalogBytesBefore, catalogBytesAfter) {
		t.Fatalf("expected catalog record bytes to be unchanged, but they changed")
	}

	// Step 3: write a local edit to skills/default/a/SKILL.md and assert GetSkillUpstream returns diverged / modified
	localSkillMD := filepath.Join(root, "skills", "default", "a", "SKILL.md")
	if err := os.WriteFile(localSkillMD, []byte("---\nname: a\ndescription: Local edit\n---\nBody A Local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillUpstream, err := GetSkillUpstream(context.Background(), root, "a")
	if err != nil {
		t.Fatalf("GetSkillUpstream failed: %v", err)
	}
	if skillUpstream.Status != "diverged" {
		t.Fatalf("expected status diverged, got %q", skillUpstream.Status)
	}
	if skillUpstream.Local != "modified" {
		t.Fatalf("expected local modified, got %q", skillUpstream.Local)
	}

	// Step 4: delete skills/a upstream, check again, assert upstream_removed
	runGit(repoDir, "rm", "-r", "skills/a")
	runGit(repoDir, "commit", "-m", "delete skill a")

	checkResRemoved, err := sourceService.CheckSources(context.Background(), root, []string{sourceID}, false)
	if err != nil {
		t.Fatalf("CheckSources after delete failed: %v", err)
	}
	if len(checkResRemoved.Results) != 1 || len(checkResRemoved.Results[0].Skills) != 1 {
		t.Fatalf("expected 1 result with 1 skill, got %#v", checkResRemoved.Results)
	}
	if checkResRemoved.Results[0].Skills[0].Status != "upstream_removed" {
		t.Fatalf("expected status upstream_removed, got %q", checkResRemoved.Results[0].Skills[0].Status)
	}

	// Step 5: Separately, a commit that touches a file outside skills/a (e.g. skills/b or root) does not change skills/a's status (up_to_date)
	if err := os.WriteFile(filepath.Join(repoDir, "root_file.txt"), []byte("root file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(skillADir, 0o700)
	_ = os.WriteFile(filepath.Join(skillADir, "SKILL.md"), []byte("---\nname: a\ndescription: Skill A Restored\n---\nBody A Restored\n"), 0o644)
	runGit(repoDir, "add", ".")
	runGit(repoDir, "commit", "-m", "restore skill a and add root file")

	root2 := newSourceWorkspace(t)
	addService2 := SkillAddService{
		Clock:    clock,
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}
	prevA2, err := addService2.PreviewSkillAdd(context.Background(), root2, SkillAddInput{Locator: fileURL, Selection: "a"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = addService2.ConfirmSkillAdd(context.Background(), root2, prevA2, prevA2.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatal(err)
	}
	sourceService2 := SourceService{
		Clock:    clock,
		Adapters: map[string]sourcepkg.Adapter{"git": adapter},
	}
	chk2Init, err := sourceService2.CheckSources(context.Background(), root2, []string{sourceID}, false)
	if err != nil {
		t.Fatal(err)
	}
	if chk2Init.Results[0].Skills[0].Status != "up_to_date" {
		t.Fatalf("expected up_to_date, got %q", chk2Init.Results[0].Skills[0].Status)
	}

	// Commit touching only root_file.txt outside skills/a
	if err := os.WriteFile(filepath.Join(repoDir, "root_file.txt"), []byte("root file modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(repoDir, "add", "root_file.txt")
	runGit(repoDir, "commit", "-m", "modify outside file only")

	chk2Outside, err := sourceService2.CheckSources(context.Background(), root2, []string{sourceID}, false)
	if err != nil {
		t.Fatal(err)
	}
	if chk2Outside.Results[0].Skills[0].Status != "up_to_date" {
		t.Fatalf("expected skills/a status to remain up_to_date after outside commit, got %q", chk2Outside.Results[0].Skills[0].Status)
	}
}

func TestNextAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status   string
		skillID  string
		sha      string
		expected string
	}{
		{
			status:   "up_to_date",
			skillID:  "my-skill",
			expected: "Up to date with upstream.",
		},
		{
			status:   "update_available",
			skillID:  "my-skill",
			expected: "Run skillhub upstream review my-skill to inspect and apply changes.",
		},
		{
			status:   "modified",
			skillID:  "my-skill",
			expected: "Local edits differ from upstream base. Run skillhub upstream review my-skill to inspect drift.",
		},
		{
			status:   "diverged",
			skillID:  "my-skill",
			expected: "Both local and upstream changed. Run skillhub upstream review my-skill to 3-way merge.",
		},
		{
			status:   "upstream_removed",
			skillID:  "my-skill",
			expected: "Skill removed in upstream repository. Decide whether to retain as custom skill.",
		},
		{
			status:   "unavailable",
			skillID:  "my-skill",
			expected: "Upstream repository could not be reached. Check network or repository access.",
		},
		{
			status:   "untracked",
			skillID:  "my-skill",
			expected: "Not linked to an upstream source. Run skillhub source attach to track updates.",
		},
		{
			status:   "pinned",
			skillID:  "my-skill",
			sha:      "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111",
			expected: "Upstream tracking pinned to commit aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111. Run skillhub source attach --ref <branch> to follow a branch.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			action := deriveNextAction(tc.status, tc.skillID, tc.sha)
			if action != tc.expected {
				t.Errorf("status %q: got %q, want %q", tc.status, action, tc.expected)
			}
		})
	}
}

func TestSkillWithEmptyProvenanceSourceIDRemainsUntracked(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(root, "skills", "default", "untracked-skill")
	if err := os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Skill has origin with name, but provenance.source_id is empty
	metaContent := `schema_version: 1
id: untracked-skill
status: active
provenance:
  origin:
    kind: github
    name: some-repo
    repository: https://github.com/example/untracked.git
routing:
  triggers: [test]
  not_for: [none]
  min_scope: single_step
`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), []byte(metaContent), 0o644); err != nil {
		t.Fatal(err)
	}

	tracked, err := loadTrackedSkills(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, ts := range tracked {
		if ts.SkillID == "untracked-skill" {
			t.Fatalf("skill with empty provenance.source_id must not be tracked, got: %#v", ts)
		}
	}

	info, err := GetSkillUpstream(context.Background(), root, "untracked-skill")
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != "untracked" {
		t.Fatalf("expected status 'untracked', got %q", info.Status)
	}
}
