package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"gopkg.in/yaml.v3"
)

type routingHookFunc func(context.Context, string, string, string, []byte, []byte) (skill.RoutingImpact, error)

func (hook routingHookFunc) Evaluate(ctx context.Context, root, id, snapshot string, before, after []byte) (skill.RoutingImpact, error) {
	return hook(ctx, root, id, snapshot, before, after)
}

func TestSkillPreviewExposesOptionalRoutingImpact(t *testing.T) {
	root := newSkillWorkspace(t)
	called := false
	service := SkillService{Manager: skill.Manager{RoutingHook: routingHookFunc(func(_ context.Context, _, id, snapshot string, before, after []byte) (skill.RoutingImpact, error) {
		called = id == "consumer-review" && snapshot != "" && len(before) == 0 && len(after) != 0
		return skill.RoutingImpact{Summary: "evaluation supplied by caller"}, nil
	})}}
	preview, err := service.PreviewCreate(context.Background(), root, skill.CreateInput{
		ID: "consumer-review", Collection: "software", Name: "Consumer Review", Description: "Review consumers.",
		Routing: skill.RoutingInput{Triggers: []string{"review consumers"}, NotFor: []string{"design brokers"}, MinScope: "multi_step"},
	}, false)
	if err != nil || !called || preview.RoutingImpact == nil || preview.RoutingImpact.Summary == "" {
		t.Fatalf("routing hook preview = %#v, called=%t, err=%v", preview.RoutingImpact, called, err)
	}
}

func TestSkillLifecycleCreateActivateReadDeprecateArchive(t *testing.T) {
	ctx := context.Background()
	root := newSkillWorkspace(t)
	clock := time.Date(2026, 9, 28, 18, 0, 0, 0, time.UTC)
	service := SkillService{Manager: skill.Manager{Clock: func() time.Time {
		clock = clock.Add(time.Second)
		return clock
	}}}

	created, err := service.PreviewCreate(ctx, root, skill.CreateInput{
		ID: "consumer-review", Collection: "software", Name: "Consumer Review",
		Description: "Review consumer reliability.", Content: []byte("# Consumer Review\n\nUse evidence.\n"),
		Routing: skill.RoutingInput{Operations: []string{"review"}, Triggers: []string{"review consumer reliability"}, NotFor: []string{"design broker topology"}, MinScope: "multi_step"},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusActionRequired || created.FullDiff == "" || len(created.Diff.Added) != 2 {
		t.Fatalf("create preview = %#v", created)
	}
	createdResult, err := service.ConfirmSkillMutation(ctx, root, created, created.Confirmation.Confirmation.Pins)
	if err != nil || createdResult.Status != StatusApplied || createdResult.OperationID == "" || createdResult.CatalogSnapshot == "" || createdResult.Generation == "" || !createdResult.GitDirty {
		t.Fatalf("create result = %#v, %v", createdResult, err)
	}
	if createdResult.State != "draft" || createdResult.Summary != "Draft skill consumer-review saved." {
		t.Fatalf("create result state/summary = %q / %q", createdResult.State, createdResult.Summary)
	}
	draft, err := service.ReadSkill(ctx, root, "consumer-review")
	if err != nil || draft.Manifest.Status != "draft" || !strings.Contains(draft.Content, "Use evidence") {
		t.Fatalf("draft read = %#v, %v", draft, err)
	}
	if _, err := skill.GetManifest(ctx, root, "consumer-review"); !errors.Is(err, skill.ErrNotFound) {
		t.Fatalf("draft was distributable: %v", err)
	}

	activated, err := service.PreviewActivate(ctx, root, "consumer-review", true)
	if err != nil {
		t.Fatal(err)
	}
	activatedResult, err := service.ConfirmSkillMutation(ctx, root, activated, activated.Confirmation.Confirmation.Pins)
	if err != nil || activatedResult.Status != StatusApplied || activatedResult.Summary != "Skill consumer-review is now active." || activatedResult.State != "active" {
		t.Fatalf("activate result = %#v, %v", activatedResult, err)
	}
	read, err := service.ReadSkill(ctx, root, "consumer-review")
	if err != nil || !strings.Contains(read.Content, "Use evidence") || read.Manifest.Status != "active" || len(read.Manifest.Resources) != 1 {
		t.Fatalf("read = %#v, %v", read, err)
	}

	deprecated, err := service.PreviewDeprecate(ctx, root, "consumer-review", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, deprecated, deprecated.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	archived, err := service.PreviewArchive(ctx, root, "consumer-review", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, archived, archived.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}

	metadataPath := filepath.Join(root, "skills", "software", "consumer-review", "skill.meta.yaml")
	contents, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := yaml.Unmarshal(contents, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["status"] != "archived" || metadata["provenance"].(map[string]any)["created_by"] != "skillhub" {
		t.Fatalf("archived metadata lost provenance: %#v", metadata)
	}
	history, ok := metadata["history"].([]any)
	if !ok || len(history) != 4 {
		t.Fatalf("history = %#v", metadata["history"])
	}
	receipts, err := filepath.Glob(filepath.Join(root, "history", "operations", "*", "*", "*.yaml"))
	if err != nil || len(receipts) != 4 {
		t.Fatalf("operation receipts = %#v, %v", receipts, err)
	}
}

func TestSkillPublicationFailureIsRecoveredByDoctorBeforeFinalization(t *testing.T) {
	ctx := context.Background()
	root := newSkillWorkspace(t)
	service := SkillService{}
	preview, err := service.PreviewCreate(ctx, root, skill.CreateInput{
		ID: "recover-publish", Collection: "software", Name: "Recover Publish", Description: "Recover catalog publication.",
		Routing: skill.RoutingInput{Triggers: []string{"recover publish"}, NotFor: []string{"other"}, MinScope: "single_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	generations := filepath.Join(root, "runtime", "catalog", "generations")
	backup := generations + ".backup"
	if err := os.Rename(generations, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(generations, []byte("blocked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, preview, preview.Confirmation.Confirmation.Pins); err == nil {
		t.Fatal("publication failure was not returned")
	}
	if err := os.Remove(generations); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, generations); err != nil {
		t.Fatal(err)
	}
	pending, err := mutation.Pending(root)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %#v, %v", pending, err)
	}
	if _, err := (WorkspaceService{}).DoctorFix(root, true); err != nil {
		t.Fatal(err)
	}
	status, err := catalog.Inspect(ctx, root)
	if err != nil || status.State != catalog.StateHealthy || status.Pointer == nil {
		t.Fatalf("catalog after recovery = %#v, %v", status, err)
	}
	if pending, err := mutation.Pending(root); err != nil || len(pending) != 0 {
		t.Fatalf("pending after recovery = %#v, %v", pending, err)
	}
}

func TestSkillLifecycleRetriesReturnOriginalOperationAndGeneration(t *testing.T) {
	ctx := context.Background()
	root := newSkillWorkspace(t)
	service := SkillService{}
	createInput := skill.CreateInput{
		ID: "retry-skill", IdempotencyKey: "caller-create-42", Collection: "software", Name: "Retry Skill", Description: "Retry safely.",
		Routing: skill.RoutingInput{Triggers: []string{"retry skill"}, NotFor: []string{"unrelated work"}, MinScope: "single_step"},
	}
	preview, err := service.PreviewCreate(ctx, root, createInput, false)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.ConfirmSkillMutation(ctx, root, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatal(err)
	}
	generationCount := countGenerationDatabases(t, root)
	retryPreview, err := service.PreviewCreate(ctx, root, createInput, false)
	if err != nil {
		t.Fatalf("create retry was rejected before idempotency lookup: %v", err)
	}
	retry, err := service.ConfirmSkillMutation(ctx, root, retryPreview, retryPreview.Confirmation.Confirmation.Pins)
	if err != nil || retry.OperationID != first.OperationID || retry.Generation != first.Generation {
		t.Fatalf("create retry = %#v, %v; first = %#v", retry, err, first)
	}
	if got := countGenerationDatabases(t, root); got != generationCount {
		t.Fatalf("create retry published generation: got %d want %d", got, generationCount)
	}

	description := "Updated exactly once."
	update := skill.UpdateInput{IdempotencyKey: "caller-edit-42", Description: &description}
	editPreview, err := service.PreviewSkillUpdate(ctx, root, "retry-skill", update, false)
	if err != nil {
		t.Fatal(err)
	}
	edited, err := service.ConfirmSkillMutation(ctx, root, editPreview, editPreview.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatal(err)
	}
	editRetryPreview, err := service.PreviewSkillUpdate(ctx, root, "retry-skill", update, false)
	if err != nil {
		t.Fatal(err)
	}
	editRetry, err := service.ConfirmSkillMutation(ctx, root, editRetryPreview, editRetryPreview.Confirmation.Confirmation.Pins)
	if err != nil || editRetry.OperationID != edited.OperationID || editRetry.Generation != edited.Generation {
		t.Fatalf("edit retry = %#v, %v; first = %#v", editRetry, err, edited)
	}

	activatePreview, err := service.PreviewTransitionWithKey(ctx, root, "retry-skill", "active", false, "caller-activate-42")
	if err != nil {
		t.Fatal(err)
	}
	activated, err := service.ConfirmSkillMutation(ctx, root, activatePreview, activatePreview.Confirmation.Confirmation.Pins)
	if err != nil {
		t.Fatal(err)
	}
	activateRetryPreview, err := service.PreviewTransitionWithKey(ctx, root, "retry-skill", "active", false, "caller-activate-42")
	if err != nil {
		t.Fatalf("transition retry was rejected before idempotency lookup: %v", err)
	}
	activateRetry, err := service.ConfirmSkillMutation(ctx, root, activateRetryPreview, activateRetryPreview.Confirmation.Confirmation.Pins)
	if err != nil || activateRetry.OperationID != activated.OperationID || activateRetry.Generation != activated.Generation {
		t.Fatalf("transition retry = %#v, %v; first = %#v", activateRetry, err, activated)
	}
	lateCreateRetryPreview, err := service.PreviewCreate(ctx, root, createInput, false)
	if err != nil {
		t.Fatalf("late create retry = %v", err)
	}
	lateCreateRetry, err := service.ConfirmSkillMutation(ctx, root, lateCreateRetryPreview, lateCreateRetryPreview.Confirmation.Confirmation.Pins)
	if err != nil || lateCreateRetry.OperationID != first.OperationID || lateCreateRetry.Generation != first.Generation {
		t.Fatalf("late create retry = %#v, %v; first = %#v", lateCreateRetry, err, first)
	}
}

func TestProductionRoutingImpactCoversCreateRoutingEditAndActivation(t *testing.T) {
	ctx := context.Background()
	root := newSkillWorkspace(t)
	service := SkillService{}
	created, err := service.PreviewCreate(ctx, root, skill.CreateInput{
		ID: "routing-impact", Collection: "software", Name: "Routing Impact", Description: "Measure direct routing.",
		Routing: skill.RoutingInput{Triggers: []string{"route this"}, NotFor: []string{"other"}, MinScope: "single_step"},
	}, false)
	if err != nil || created.RoutingImpact == nil || created.RoutingImpact.Summary == "" {
		t.Fatalf("create impact = %#v, %v", created.RoutingImpact, err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, created, created.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	routing := skill.RoutingInput{Triggers: []string{"route this exactly"}, NotFor: []string{"other"}, MinScope: "single_step"}
	edited, err := service.PreviewSkillUpdate(ctx, root, "routing-impact", skill.UpdateInput{Routing: &routing}, false)
	if err != nil || edited.RoutingImpact == nil || edited.RoutingImpact.Summary == "" {
		t.Fatalf("edit impact = %#v, %v", edited.RoutingImpact, err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, edited, edited.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	activated, err := service.PreviewActivate(ctx, root, "routing-impact", false)
	if err != nil || activated.RoutingImpact == nil || !strings.Contains(activated.RoutingImpact.Summary, "eligible") {
		t.Fatalf("activation impact = %#v, %v", activated.RoutingImpact, err)
	}
}

func countGenerationDatabases(t *testing.T, root string) int {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(root, "runtime", "catalog", "generations", "*.db"))
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func TestSkillStaleProposalIsRejectedWithoutReceipt(t *testing.T) {
	ctx := context.Background()
	root := newSkillWorkspace(t)
	service := SkillService{}
	createAndActivateSkill(t, service, root)

	description := "Reviewed description."
	preview, err := service.PreviewSkillUpdate(ctx, root, "consumer-review", skill.UpdateInput{Description: &description}, true)
	if err != nil {
		t.Fatal(err)
	}
	entrypoint := filepath.Join(root, "skills", "software", "consumer-review", "SKILL.md")
	if err := os.WriteFile(entrypoint, []byte("# Consumer Review\n\nExternal valid edit.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeReceipts, _ := filepath.Glob(filepath.Join(root, "history", "operations", "*", "*", "*.yaml"))
	result, err := service.ConfirmSkillMutation(ctx, root, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil || result.Status != StatusError || result.Error == nil || result.Error.Code != ErrorStaleProposal {
		t.Fatalf("stale result = %#v, %v", result, err)
	}
	afterReceipts, _ := filepath.Glob(filepath.Join(root, "history", "operations", "*", "*", "*.yaml"))
	if len(afterReceipts) != len(beforeReceipts) {
		t.Fatal("stale proposal created an operation receipt")
	}
}

func TestInvalidExternalEditPreservesPublishedGeneration(t *testing.T) {
	ctx := context.Background()
	root := newSkillWorkspace(t)
	service := SkillService{}
	createAndActivateSkill(t, service, root)
	before, err := catalog.Inspect(ctx, root)
	if err != nil || before.Pointer == nil {
		t.Fatalf("before = %#v, %v", before, err)
	}
	metadataPath := filepath.Join(root, "skills", "software", "consumer-review", "skill.meta.yaml")
	if err := os.WriteFile(metadataPath, []byte("schema_version: 1\nid: consumer-review\nstatus: active\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	validated, err := (WorkspaceService{}).ValidateWorkspace(ctx, root)
	if err != nil || validated.Status != StatusError {
		t.Fatalf("invalid edit validation = %#v, %v", validated, err)
	}
	if _, err := (CatalogService{}).BuildCatalogGeneration(ctx, root); err == nil {
		t.Fatal("invalid external edit published a generation")
	}
	published, err := catalog.InspectPublished(ctx, root)
	if err != nil || published.Pointer == nil || published.Pointer.Generation != before.Pointer.Generation {
		t.Fatalf("published generation changed: %#v, %v", published, err)
	}
}

func TestDigestPinnedReadReturnsSnapshotExpiredAfterExternalChange(t *testing.T) {
	ctx := context.Background()
	root := newSkillWorkspace(t)
	service := SkillService{}
	createAndActivateSkill(t, service, root)
	manifest, err := skill.GetManifest(ctx, root, "consumer-review")
	if err != nil {
		t.Fatal(err)
	}
	resource := manifest.Resources[0]
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(resource.Path)), []byte("# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := skill.ReadResource(ctx, root, manifest.CatalogSnapshot, resource.Path, resource.Digest); !errors.Is(err, skill.ErrSnapshotExpired) {
		t.Fatalf("stale resource read = %v", err)
	}
	if _, err := (CatalogService{}).BuildCatalogGeneration(ctx, root); err != nil {
		t.Fatal(err)
	}
	if _, err := skill.ReadResource(ctx, root, manifest.CatalogSnapshot, resource.Path, resource.Digest); !errors.Is(err, skill.ErrSnapshotExpired) {
		t.Fatalf("old manifest read after rebuild = %v", err)
	}
}

func newSkillWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	return root
}

func createAndActivateSkill(t *testing.T, service SkillService, root string) {
	t.Helper()
	ctx := context.Background()
	preview, err := service.PreviewCreate(ctx, root, skill.CreateInput{
		ID: "consumer-review", Collection: "software", Name: "Consumer Review", Description: "Review consumers.",
		Content: []byte("# Consumer Review\n"), Routing: skill.RoutingInput{Triggers: []string{"review consumers"}, NotFor: []string{"design brokers"}, MinScope: "multi_step"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, preview, preview.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	preview, err = service.PreviewActivate(ctx, root, "consumer-review", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, preview, preview.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
}
