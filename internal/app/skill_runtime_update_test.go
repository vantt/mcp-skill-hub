package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

func confirmSkillUpdate(t *testing.T, root, id string, input skill.UpdateInput) {
	t.Helper()
	service := SkillService{}
	proposal, err := service.PreviewSkillUpdate(t.Context(), root, id, input, false)
	if err != nil {
		t.Fatalf("PreviewSkillUpdate: %v", err)
	}
	if _, err := service.ConfirmSkillMutation(t.Context(), root, proposal, proposal.Confirmation.Confirmation.Pins); err != nil {
		t.Fatalf("ConfirmSkillMutation: %v", err)
	}
}

func TestSkillUpdateSetsAndRemovesRuntimeBlockAndStalesApproval(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "runtime-edit", "Runtime Edit")
	updateSkillMeta(t, root, "runtime-edit", markThirdParty)
	service := fakeSnapshotService()

	review, err := (SkillService{}).ReviewSkill(t.Context(), root, "runtime-edit")
	if err != nil {
		t.Fatal(err)
	}
	confirmSkillUpdate(t, root, "runtime-edit", skill.UpdateInput{ContentReviewedDigest: &review.ContentTrust.ContentDigest})
	if local := ensureSnapshot(t, service, root, "runtime-edit"); local.Status != LocalStatusReady || local.Preflight != nil {
		t.Fatalf("approved skill = %#v", local)
	}

	block := map[string]any{
		"requires": map[string]any{"bins": []any{map[string]any{"name": "python3", "version": ">=3.10"}}, "env": []any{"API_TOKEN"}, "platforms": []any{"linux"}},
		"setup":    map[string]any{"check": "python3 -c 'import requests'", "command": "pip install --target \"$SKILLHUB_STATE_DIR/lib\" requests"},
	}
	confirmSkillUpdate(t, root, "runtime-edit", skill.UpdateInput{Runtime: block})
	stale := ensureSnapshot(t, service, root, "runtime-edit")
	if stale.Status != LocalStatusReviewRequired || !slices.Contains(stale.ReasonCodes, skillruntime.ReasonContentReviewStale) || stale.Path != "" {
		t.Fatalf("a runtime change must make the approval stale: %#v", stale)
	}

	review, err = (SkillService{}).ReviewSkill(t.Context(), root, "runtime-edit")
	if err != nil {
		t.Fatal(err)
	}
	if review.RuntimeHints.MissingRuntimeBlock {
		t.Fatalf("hints = %#v", review.RuntimeHints)
	}
	confirmSkillUpdate(t, root, "runtime-edit", skill.UpdateInput{ContentReviewedDigest: &review.ContentTrust.ContentDigest})
	ready := ensureSnapshot(t, service, root, "runtime-edit")
	if ready.Status != LocalStatusReady || ready.Preflight == nil || ready.Preflight.Check != "python3 -c 'import requests'" || len(ready.Preflight.Requires.Bins) != 1 {
		t.Fatalf("approved runtime = %#v", ready)
	}

	confirmSkillUpdate(t, root, "runtime-edit", skill.UpdateInput{Runtime: map[string]any{}})
	removed := ensureSnapshot(t, service, root, "runtime-edit")
	if removed.Status != LocalStatusReviewRequired {
		t.Fatalf("removing the block must also stale the approval: %#v", removed)
	}
	review, err = (SkillService{}).ReviewSkill(t.Context(), root, "runtime-edit")
	if err != nil {
		t.Fatal(err)
	}
	// The content is identical to the first approved content, so the identical
	// approval request needs its own idempotency key to be applied again.
	confirmSkillUpdate(t, root, "runtime-edit", skill.UpdateInput{IdempotencyKey: "approve-after-removal", ContentReviewedDigest: &review.ContentTrust.ContentDigest})
	if final := ensureSnapshot(t, service, root, "runtime-edit"); final.Status != LocalStatusReady || final.Preflight != nil {
		t.Fatalf("removed block still has a preflight: %#v", final)
	}
}

func TestSkillUpdateRejectsInvalidRuntimeBlock(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "runtime-bad", "Runtime Bad")
	for name, block := range map[string]map[string]any{
		"unknown field": {"unexpected": true},
		"bad bin":       {"requires": map[string]any{"bins": []any{"rm -rf /"}}},
		"multiline":     {"setup": map[string]any{"command": "a\nb"}},
	} {
		_, err := (SkillService{}).PreviewSkillUpdate(t.Context(), root, "runtime-bad", skill.UpdateInput{Runtime: block}, false)
		if err == nil || !strings.Contains(err.Error(), "invalid runtime block") {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}
