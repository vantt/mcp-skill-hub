package web

import (
	"context"
	"fmt"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"gopkg.in/yaml.v3"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"
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

	// 1. vendor-skill (third-party, unapproved, runtime with bins/env/setup)
	vendorContent := []byte("---\nname: vendor-skill\ndescription: Vendor integration tool.\nlicense: Apache-2.0\n---\n\n# Vendor Skill\n\nRun pip install something in ~/.claude/skills/\n")
	created, err := service.PreviewCreate(context.Background(), root, skill.CreateInput{
		ID:          "vendor-skill",
		Collection:  "core",
		Name:        "Vendor Skill",
		Description: "Vendor integration tool.",
		Content:     vendorContent,
		Routing: skill.RoutingInput{
			Triggers: []string{"vendor integration"},
			NotFor:   []string{"unrelated tasks"},
			MinScope: "single_step",
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, created, created.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm create vendor = %#v, %v", result, err)
	}
	activated, err := service.PreviewActivate(context.Background(), root, "vendor-skill", false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, activated, activated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm activate vendor = %#v, %v", result, err)
	}

	vendorDir := filepath.Join(root, "skills", "core", "vendor-skill")
	// write scripts/run.py
	_ = os.MkdirAll(filepath.Join(vendorDir, "scripts"), 0o755)
	_ = os.WriteFile(filepath.Join(vendorDir, "scripts", "run.py"), []byte("#!/usr/bin/env python3\nprint('vendor')\n"), 0o644)
	// write package.json without lockfile
	_ = os.WriteFile(filepath.Join(vendorDir, "package.json"), []byte("{\"name\": \"vendor\"}\n"), 0o644)

	// update vendor-skill metadata: provenance.origin, runtime block
	vendorMetaPath := filepath.Join(vendorDir, "skill.meta.yaml")
	vendorMetaData, err := os.ReadFile(vendorMetaPath)
	if err != nil {
		t.Fatal(err)
	}
	var vendorDoc map[string]any
	if err := yaml.Unmarshal(vendorMetaData, &vendorDoc); err != nil {
		t.Fatal(err)
	}
	vendorDoc["provenance"] = map[string]any{
		"created_by": "skillhub",
		"origin": map[string]any{
			"kind":       "github",
			"repository": "https://github.com/vendor/skills",
			"commit":     strings.Repeat("a", 40),
		},
	}
	vendorDoc["runtime"] = map[string]any{
		"requires": map[string]any{
			"bins": []any{"python3"},
			"env":  []any{"VENDOR_TOKEN"},
		},
		"setup": map[string]any{
			"check": "python3 scripts/run.py",
		},
	}
	vendorEncoded, err := yaml.Marshal(vendorDoc)
	if err != nil {
		t.Fatal(err)
	}
	vendorEncoded = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z`).ReplaceAll(vendorEncoded, []byte("2026-10-04T12:00:00.000000000Z"))
	if err := os.WriteFile(vendorMetaPath, vendorEncoded, 0o644); err != nil {
		t.Fatal(err)
	}

	// Store VENDOR_TOKEN with a sentinel value
	const sentinelValue = "SENTINEL-TEST-SECRET"
	if _, err := (app.SkillEnvService{}).Set(context.Background(), root, "vendor-skill", "VENDOR_TOKEN", sentinelValue); err != nil {
		t.Fatal(err)
	}

	// 2. approved-skill (third-party, platforms-only runtime, content_reviewed_digest set, doctor run once)
	approvedContent := []byte("---\nname: approved-skill\ndescription: Approved vendor tool.\nlicense: Apache-2.0\n---\n\n# Approved Skill\n\nApproved tool.\n")
	apprCreated, err := service.PreviewCreate(context.Background(), root, skill.CreateInput{
		ID:          "approved-skill",
		Collection:  "core",
		Name:        "Approved Skill",
		Description: "Approved vendor tool.",
		Content:     approvedContent,
		Routing: skill.RoutingInput{
			Triggers: []string{"approved vendor tool"},
			NotFor:   []string{"unrelated tasks"},
			MinScope: "single_step",
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, apprCreated, apprCreated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm create approved = %#v, %v", result, err)
	}
	apprActivated, err := service.PreviewActivate(context.Background(), root, "approved-skill", false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(context.Background(), root, apprActivated, apprActivated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm activate approved = %#v, %v", result, err)
	}

	approvedDir := filepath.Join(root, "skills", "core", "approved-skill")
	apprMetaPath := filepath.Join(approvedDir, "skill.meta.yaml")
	apprMetaData, err := os.ReadFile(apprMetaPath)
	if err != nil {
		t.Fatal(err)
	}
	var apprDoc map[string]any
	if err := yaml.Unmarshal(apprMetaData, &apprDoc); err != nil {
		t.Fatal(err)
	}
	apprDoc["provenance"] = map[string]any{
		"created_by": "skillhub",
		"origin": map[string]any{
			"kind":       "github",
			"repository": "https://github.com/approved/skills",
			"commit":     strings.Repeat("b", 40),
		},
	}
	apprDoc["runtime"] = map[string]any{
		"requires": map[string]any{
			"platforms": []any{"linux", "darwin", "windows", "freebsd"},
		},
	}
	apprEncoded, err := yaml.Marshal(apprDoc)
	if err != nil {
		t.Fatal(err)
	}
	apprEncoded = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z`).ReplaceAll(apprEncoded, []byte("2026-10-04T12:00:00.000000000Z"))
	if err := os.WriteFile(apprMetaPath, apprEncoded, 0o644); err != nil {
		t.Fatal(err)
	}

	// Now calculate content digest and approve it
	trust, err := service.ContentTrustFor(context.Background(), root, "approved-skill")
	if err != nil || trust.ContentDigest == "" {
		t.Fatalf("content digest error: %v", err)
	}
	apprDoc["quality"] = map[string]any{
		"reviewed":                true,
		"content_reviewed_digest": trust.ContentDigest,
	}
	apprEncoded, _ = yaml.Marshal(apprDoc)
	apprEncoded = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z`).ReplaceAll(apprEncoded, []byte("2026-10-04T12:00:00.000000000Z"))
	_ = os.WriteFile(apprMetaPath, apprEncoded, 0o644)

	// Rebuild catalog and run doctor once
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.SkillDoctorService{}).Run(context.Background(), root, "approved-skill"); err != nil {
		t.Fatal(err)
	}

	return root
}
