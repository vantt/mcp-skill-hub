package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	sourcepkg "github.com/vantt/skillhub-mcp/internal/source"
)

// fullDiffSection returns the removed and added lines a full diff shows for
// one path.
func fullDiffSection(fullDiff, path string) (removed, added []string) {
	header := "--- a/" + path + "\n+++ b/" + path + "\n"
	start := strings.Index(fullDiff, header)
	if start < 0 {
		return nil, nil
	}
	body := fullDiff[start+len(header):]
	if next := strings.Index(body, "\n--- a/"); next >= 0 {
		body = body[:next+1]
	}
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "-"):
			removed = append(removed, line[1:])
		case strings.HasPrefix(line, "+"):
			added = append(added, line[1:])
		}
	}
	return removed, added
}

func trackTestService() SourceService {
	return SourceService{Clock: sourceClock{now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}}
}

func TestPreviewTrackUpstreamProposesMinimalChangeAndConfirmTracksUpstream(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	ctx := context.Background()
	metaPath := writeBackfillSkill(t, root, "herdr-cook-plan", herdrCookPlanMeta)
	commitWorkspace(t, root)
	service := trackTestService()

	proposal, err := service.PreviewTrackUpstream(ctx, root, "herdr-cook-plan")
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Error != nil || proposal.Status != StatusActionRequired {
		t.Fatalf("proposal result = %#v", proposal.Result)
	}
	if proposal.Candidate == nil || proposal.Candidate.SourceID != "herdr-cook-plan-1" || !proposal.Candidate.CreateSource {
		t.Fatalf("candidate = %#v", proposal.Candidate)
	}
	pins := proposal.Confirmation.Confirmation.Pins
	if pins.ProposalID == "" || pins.ProposalDigest == "" || pins.BaseVersion == "" {
		t.Fatalf("pins = %#v", pins)
	}
	catalogPath := "sources/catalog/herdr-cook-plan-1.yaml"
	metaRel := "skills/default/herdr-cook-plan/.meta/skill.yaml"
	if !slices.Contains(proposal.Diff.Added, catalogPath) || !slices.Contains(proposal.Diff.Modified, metaRel) {
		t.Fatalf("diff = %#v", proposal.Diff)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(catalogPath))); !os.IsNotExist(err) {
		t.Fatalf("preview wrote the source record: %v", err)
	}
	if current, _ := os.ReadFile(metaPath); string(current) != herdrCookPlanMeta {
		t.Fatalf("preview changed the skill metadata:\n%s", current)
	}
	// The metadata diff shows only the changed upstream line as -/+; every
	// other line of the file is either omitted or shown as context.
	removed, added := fullDiffSection(proposal.FullDiff, metaRel)
	assertLines(t, "removed", removed, []string{`      id: ""`})
	assertLines(t, "added", added, []string{`      id: herdr-cook-plan-1`})
	if !strings.Contains(proposal.FullDiff, "+++ b/"+metaRel+"\n@@ -") {
		t.Fatalf("metadata diff has no hunk header:\n%s", proposal.FullDiff)
	}
	if _, recordLines := fullDiffSection(proposal.FullDiff, catalogPath); len(recordLines) == 0 {
		t.Fatalf("full diff misses the new source record:\n%s", proposal.FullDiff)
	}

	stored, err := service.LoadSourceProposal(ctx, root, pins.ProposalID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ConfirmSourceProposal(ctx, root, stored, pins)
	if err != nil {
		t.Fatal(err)
	}
	if result.Error != nil || result.Summary != "Now tracking upstream herdr-cook-plan-1 for herdr-cook-plan." {
		t.Fatalf("confirm result = %#v", result)
	}
	upstream, err := GetSkillUpstream(ctx, root, "herdr-cook-plan")
	if err != nil {
		t.Fatal(err)
	}
	if upstream.Status == "untracked" || upstream.SourceID != "herdr-cook-plan-1" {
		t.Fatalf("upstream after confirm = %#v", upstream)
	}

	// The mutation journal replays an already applied proposal instead of
	// writing it twice.
	replay, err := service.ConfirmSourceProposal(ctx, root, stored, pins)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Error != nil || replay.OperationID != result.OperationID {
		t.Fatalf("replayed confirm = %#v, want the original operation %s", replay, result.OperationID)
	}

	tracked, err := service.PreviewTrackUpstream(ctx, root, "herdr-cook-plan")
	if err != nil {
		t.Fatal(err)
	}
	if tracked.Status != StatusOK || tracked.Candidate != nil || tracked.Confirmation.Confirmation.Pins.ProposalID != "" {
		t.Fatalf("tracked skill proposal = %#v", tracked)
	}
}

func TestPreviewTrackUpstreamLinksMatchingSourceRecord(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	ctx := context.Background()
	record := sourcepkg.Record{
		SchemaVersion: 1,
		ID:            "herdr-upstream",
		Adapter:       "git",
		Locator:       sourcepkg.Locator{Repository: "https://github.com/thieung/herdr-cook-plan", Ref: "main"},
		Status:        "watching",
		Identity:      sourcepkg.Identity{Name: "herdr-upstream", Canonical: "https://github.com/thieung/herdr-cook-plan", DefaultBranch: "main"},
		Monitoring:    sourcepkg.Monitoring{Enabled: true, Cadence: "weekly"},
		Limits:        sourcepkg.Limits{TimeoutSeconds: 30, MaxBytes: 1024 * 1024, MaxFiles: 100, MaxFileBytes: 1024 * 1024},
	}
	recordBytes, err := sourcepkg.MarshalCanonical(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sources", "catalog", "herdr-upstream.yaml"), recordBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	writeBackfillSkill(t, root, "herdr-cook-plan", herdrCookPlanMeta)
	commitWorkspace(t, root)

	proposal, err := trackTestService().PreviewTrackUpstream(ctx, root, "herdr-cook-plan")
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Candidate == nil || proposal.Candidate.SourceID != "herdr-upstream" || proposal.Candidate.CreateSource {
		t.Fatalf("candidate = %#v", proposal.Candidate)
	}
	if len(proposal.Diff.Added) != 0 || proposal.Source.ID != "herdr-upstream" || proposal.Source.Adapter != "git" {
		t.Fatalf("proposal = diff %#v source %#v", proposal.Diff, proposal.Source)
	}
	pins := proposal.Confirmation.Confirmation.Pins
	if pins.ProposalID == "" {
		t.Fatal("proposal has no pins")
	}

	metaPath := filepath.Join(root, "skills", "default", "herdr-cook-plan", ".meta", "skill.yaml")
	edited := strings.Replace(herdrCookPlanMeta, "reviewed: true", "reviewed: false", 1)
	if err := os.WriteFile(metaPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	stored, err := trackTestService().LoadSourceProposal(ctx, root, pins.ProposalID)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := trackTestService().ConfirmSourceProposal(ctx, root, stored, pins)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Error == nil || stale.Error.Code != ErrorStaleProposal {
		t.Fatalf("confirm after the metadata changed = %#v", stale)
	}
	if current, _ := os.ReadFile(metaPath); string(current) != edited {
		t.Fatalf("stale confirm wrote the skill metadata:\n%s", current)
	}
}

func TestPreviewTrackUpstreamWithoutUntrackedUpstream(t *testing.T) {
	t.Parallel()
	root := newSourceWorkspace(t)
	ctx := context.Background()
	writeBackfillSkill(t, root, "local-skill", `description: Backfill fixture
id: local-skill
name: local-skill
schema_version: 1
status: draft
`)
	commitWorkspace(t, root)
	service := trackTestService()

	local, err := service.PreviewTrackUpstream(ctx, root, "local-skill")
	if err != nil {
		t.Fatal(err)
	}
	if local.Status != StatusOK || local.Summary != "Nothing to track: the skill has no untracked upstream." ||
		local.Candidate != nil || local.Confirmation.Confirmation.Pins.ProposalID != "" {
		t.Fatalf("local skill proposal = %#v", local)
	}

	unknown, err := service.PreviewTrackUpstream(ctx, root, "missing-skill")
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Error == nil || !unknown.SkillNotFound() {
		t.Fatalf("unknown skill proposal = %#v", unknown)
	}
}
