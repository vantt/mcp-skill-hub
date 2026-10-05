package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"gopkg.in/yaml.v3"
)

func newWebWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (app.WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	service := app.SkillService{}
	content := []byte("---\nname: review-skill\ndescription: Review changed code safely.\nlicense: Apache-2.0\n---\n\n# Review\n\nReview carefully.\n")
	created, err := service.PreviewCreate(context.Background(), root, skill.CreateInput{
		ID:          "review-skill",
		Collection:  "core",
		Name:        "Review Skill",
		Description: "Review changed code safely.",
		Content:     content,
		Routing: skill.RoutingInput{
			Operations: []string{"review"},
			Triggers:   []string{"review changed code"},
			NotFor:     []string{"write prose"},
			MinScope:   "multi_step",
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, created, created.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm create = %#v, %v", result, err)
	}
	activated, err := service.PreviewActivate(context.Background(), root, "review-skill", false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, activated, activated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm activate = %#v, %v", result, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "core", "review-skill", "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "core", "review-skill", "references", "checks.md"), []byte("# Checks\n\nRun tests.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(root, "skills", "core", "review-skill", "skill.meta.yaml")
	if data, err := os.ReadFile(metaPath); err == nil {
		fixed := regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z`).ReplaceAll(data, []byte("2026-10-04T12:00:00.000000000Z"))
		if err := os.WriteFile(metaPath, fixed, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	// Seed telemetry rollups for review-skill
	fixedTime := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	recorder, err := (app.TelemetryService{Config: telemetry.Config{Clock: func() time.Time { return fixedTime }}}).Open(root)
	if err != nil {
		t.Fatal(err)
	}

	makeEvt := func(id, eventType string) telemetry.Event {
		return telemetry.Event{
			Version:         telemetry.EventVersion,
			ID:              id,
			Type:            eventType,
			OccurredAt:      fixedTime,
			CatalogSnapshot: "test-snapshot",
			PolicyRevision:  "test-policy",
			Client:          telemetry.Client{Name: "test", Version: "1.0"},
		}
	}

	// 10 recommendations
	for i := range 10 {
		rec := makeEvt(fmt.Sprintf("evt_rec_%d", i), telemetry.EventResolutionRecommended)
		rec.ResolutionID = fmt.Sprintf("res_%d", i)
		rec.Payload = map[string]any{
			"top_skill_id":          "review-skill",
			"recommended_skill_ids": []string{"review-skill"},
		}
		recorder.Record(rec)
	}

	// 8 recommended activations
	for i := range 8 {
		load := makeEvt(fmt.Sprintf("evt_act_%d", i), telemetry.EventSkillLoaded)
		load.ResolutionID = fmt.Sprintf("res_%d", i)
		load.Payload = map[string]any{
			"skill_id":         "review-skill",
			"basis":            telemetry.LoadBasisServerObserved,
			"resource_kind":    "entrypoint",
			"surface":          "skill_get",
			"attribution":      "recommended",
			"first_activation": true,
		}
		recorder.Record(load)
	}

	// 1 override activation
	actOver := makeEvt("evt_act_over", telemetry.EventSkillLoaded)
	actOver.ResolutionID = "res_over"
	actOver.Payload = map[string]any{
		"skill_id":         "review-skill",
		"basis":            telemetry.LoadBasisServerObserved,
		"resource_kind":    "entrypoint",
		"surface":          "skill_get",
		"attribution":      "override",
		"first_activation": true,
	}
	recorder.Record(actOver)

	// 1 unsolicited activation
	actUnsol := makeEvt("evt_act_unsol", telemetry.EventSkillLoaded)
	actUnsol.Payload = map[string]any{
		"skill_id":         "review-skill",
		"basis":            telemetry.LoadBasisServerObserved,
		"resource_kind":    "entrypoint",
		"surface":          "skill_get",
		"attribution":      "unsolicited",
		"first_activation": true,
	}
	recorder.Record(actUnsol)

	// 4 additional loads (3 entrypoint, 1 reference)
	for i := range 3 {
		load := makeEvt(fmt.Sprintf("evt_more_load_%d", i), telemetry.EventSkillLoaded)
		load.Payload = map[string]any{
			"skill_id":         "review-skill",
			"basis":            telemetry.LoadBasisServerObserved,
			"resource_kind":    "entrypoint",
			"surface":          "skill_get",
			"first_activation": false,
		}
		recorder.Record(load)
	}
	loadRef := makeEvt("evt_load_ref", telemetry.EventSkillLoaded)
	loadRef.Payload = map[string]any{
		"skill_id":         "review-skill",
		"basis":            telemetry.LoadBasisServerObserved,
		"resource_kind":    "reference",
		"surface":          "resources_read",
		"first_activation": false,
	}
	recorder.Record(loadRef)

	// Doctor runs: 3 ready, 1 setup_required
	for i := range 3 {
		doc := makeEvt(fmt.Sprintf("evt_doc_ready_%d", i), telemetry.EventSkillDoctorChecked)
		doc.Payload = map[string]any{"skill_id": "review-skill", "status": "ready"}
		recorder.Record(doc)
	}
	docFail := makeEvt("evt_doc_fail", telemetry.EventSkillDoctorChecked)
	docFail.Payload = map[string]any{"skill_id": "review-skill", "status": "setup_required"}
	recorder.Record(docFail)

	// Flush before feedback
	if err := recorder.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Feedback: setup_failed
	if _, err := recorder.RecordFeedback(context.Background(), telemetry.Feedback{
		EventID:      "fb_review_setup_fail",
		ResolutionID: "res_0",
		Outcome:      "failed",
		ReasonCode:   telemetry.FeedbackReasonSetupFailed,
		SkillID:      "review-skill",
	}); err != nil {
		t.Fatal(err)
	}

	if err := recorder.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	return root
}

func newTestServer(t *testing.T, root string) *Server {
	t.Helper()
	fakeIfaces := func() ([]InterfaceInfo, error) {
		return []InterfaceInfo{}, nil
	}
	srv, err := New(Options{
		Workspace:  root,
		Token:      "test-token",
		ListenPort: 7421,
		Interfaces: fakeIfaces,
		Assets:     fstest.MapFS{},
		Now:        func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	return srv
}

func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "127.0.0.1:7421"
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func newRuntimeWebWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "runtime-workspace")
	if _, err := (app.WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}

	service := app.SkillService{}
	ctx := context.Background()

	// 1. vendor-skill (third-party, unapproved)
	vContent := []byte("---\nname: vendor-skill\ndescription: Third-party vendor skill.\n---\n\n# Vendor Skill\n\nRun pip install some-pkg and check ~/.claude/skills/vendor-skill/scripts/run.py\n")
	vCreated, err := service.PreviewCreate(ctx, root, skill.CreateInput{
		ID:          "vendor-skill",
		Collection:  "core",
		Name:        "Vendor Skill",
		Description: "Third-party vendor skill.",
		Content:     vContent,
		Routing: skill.RoutingInput{
			Operations: []string{"operate"},
			Triggers:   []string{"operate vendor skill"},
			NotFor:     []string{"unrelated"},
			MinScope:   "multi_step",
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(ctx, root, vCreated, vCreated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm create vendor = %#v, %v", result, err)
	}
	vActivated, err := service.PreviewActivate(ctx, root, "vendor-skill", false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(ctx, root, vActivated, vActivated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm activate vendor = %#v, %v", result, err)
	}

	vendorDir := filepath.Join(root, "skills", "core", "vendor-skill")
	_ = os.MkdirAll(filepath.Join(vendorDir, "scripts"), 0o755)
	_ = os.WriteFile(filepath.Join(vendorDir, "scripts", "run.py"), []byte("#!/usr/bin/env python3\nprint('vendor')\n"), 0o644)
	_ = os.WriteFile(filepath.Join(vendorDir, "package.json"), []byte(`{"name": "vendor-skill"}`), 0o644)

	vMetaPath := filepath.Join(vendorDir, "skill.meta.yaml")
	vMetaData, err := os.ReadFile(vMetaPath)
	if err != nil {
		t.Fatal(err)
	}
	var vMetaDoc map[string]any
	if err := yaml.Unmarshal(vMetaData, &vMetaDoc); err != nil {
		t.Fatal(err)
	}
	vMetaDoc["provenance"] = map[string]any{
		"created_by": "skillhub",
		"origin": map[string]any{
			"kind":       "github",
			"repository": "https://github.com/example/vendor-skills",
			"commit":     strings.Repeat("a", 40),
		},
	}
	vMetaDoc["runtime"] = map[string]any{
		"requires": map[string]any{
			"bins": []any{"python3"},
			"env":  []any{"VENDOR_TOKEN"},
		},
		"setup": map[string]any{
			"check": "python3 scripts/run.py",
		},
	}
	encodedVMeta, _ := yaml.Marshal(vMetaDoc)
	_ = os.WriteFile(vMetaPath, encodedVMeta, 0o644)

	// Store VENDOR_TOKEN with sentinel value
	if _, err := (app.SkillEnvService{}).Set(ctx, root, "vendor-skill", "VENDOR_TOKEN", "SENTINEL-VENDOR-ENV-VALUE"); err != nil {
		t.Fatalf("env set: %v", err)
	}

	// 2. approved-skill
	aContent := []byte("---\nname: approved-skill\ndescription: Third-party approved skill.\n---\n\n# Approved Skill\n\nApproved instructions.\n")
	aCreated, err := service.PreviewCreate(ctx, root, skill.CreateInput{
		ID:          "approved-skill",
		Collection:  "core",
		Name:        "Approved Skill",
		Description: "Third-party approved skill.",
		Content:     aContent,
		Routing: skill.RoutingInput{
			Operations: []string{"operate"},
			Triggers:   []string{"operate approved skill"},
			NotFor:     []string{"unrelated"},
			MinScope:   "multi_step",
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(ctx, root, aCreated, aCreated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm create approved = %#v, %v", result, err)
	}
	aActivated, err := service.PreviewActivate(ctx, root, "approved-skill", false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(ctx, root, aActivated, aActivated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm activate approved = %#v, %v", result, err)
	}

	approvedDir := filepath.Join(root, "skills", "core", "approved-skill")
	aMetaPath := filepath.Join(approvedDir, "skill.meta.yaml")
	aMetaData, err := os.ReadFile(aMetaPath)
	if err != nil {
		t.Fatal(err)
	}
	var aMetaDoc map[string]any
	if err := yaml.Unmarshal(aMetaData, &aMetaDoc); err != nil {
		t.Fatal(err)
	}
	aMetaDoc["provenance"] = map[string]any{
		"created_by": "skillhub",
		"origin": map[string]any{
			"kind":       "github",
			"repository": "https://github.com/example/approved-skills",
			"commit":     strings.Repeat("b", 40),
		},
	}
	aMetaDoc["runtime"] = map[string]any{
		"requires": map[string]any{
			"platforms": []any{"linux", "darwin", "windows", "freebsd"},
		},
	}
	encodedAMeta, _ := yaml.Marshal(aMetaDoc)
	_ = os.WriteFile(aMetaPath, encodedAMeta, 0o644)

	// Calculate trust digest and approve
	trust, err := service.ContentTrustFor(ctx, root, "approved-skill")
	if err != nil {
		t.Fatal(err)
	}
	quality, _ := aMetaDoc["quality"].(map[string]any)
	if quality == nil {
		quality = map[string]any{}
	}
	quality["reviewed"] = true
	quality["content_reviewed_digest"] = trust.ContentDigest
	aMetaDoc["quality"] = quality
	encodedAMeta, _ = yaml.Marshal(aMetaDoc)
	_ = os.WriteFile(aMetaPath, encodedAMeta, 0o644)

	// Rebuild catalog and run doctor once on approved-skill
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(ctx, root); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.SkillDoctorService{}).Run(ctx, root, "approved-skill"); err != nil {
		t.Fatalf("doctor run approved-skill: %v", err)
	}

	return root
}
