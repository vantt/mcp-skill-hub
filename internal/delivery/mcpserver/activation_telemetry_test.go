package mcpserver

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

func newClientSession(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	capabilities := &mcp.ClientCapabilities{}
	capabilities.AddExtension("io.modelcontextprotocol/skills", map[string]any{})
	client := mcp.NewClient(&mcp.Implementation{Name: "telemetry-test", Version: "1"}, &mcp.ClientOptions{Capabilities: capabilities})
	if err := mcp.AddSendingCustomMethod[*listSkillsParams, *listSkillsResult](client, "skills/list"); err != nil {
		t.Fatal(err)
	}
	if err := mcp.AddSendingCustomMethod[*getSkillParams, *getSkillResult](client, "skills/get"); err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func connectTelemetrySession(t *testing.T, root string) (*Server, *mcp.Server, *telemetry.Recorder, *mcp.ClientSession) {
	t.Helper()
	adapter, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := (app.TelemetryService{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	adapter.telemetry = recorder
	adapter.resolver.Telemetry = recorder
	adapter.feedback.Recorder = recorder

	session := newClientSession(t, server)
	return adapter, server, recorder, session
}
func callSkillResolve(t *testing.T, session *mcp.ClientSession, args map[string]any) resolveResult {
	t.Helper()
	response, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_resolve", Arguments: args})
	if err != nil || response.IsError {
		t.Fatalf("skill_resolve = %#v, %v", response, err)
	}
	var outcome toolOutcome[resolveResult]
	decodeStructuredContent(t, response, &outcome)
	if outcome.Result == nil {
		t.Fatalf("skill_resolve has no result: %#v", response)
	}
	return *outcome.Result
}
func TestActivationTelemetryEndToEnd(t *testing.T) {
	t.Parallel()

	root := newMCPWorkspace(t)
	_, server, recorder, session := connectTelemetrySession(t, root)
	privateDescription := "review changed code for private-task-description"
	resolveArgs := map[string]any{
		"schema_version": "1",
		"request_id":     "REQ-activation-test",
		"task":           map[string]any{"description": privateDescription, "scope": "multi_step"},
		"operation":      "review",
	}

	resolveRes := callSkillResolve(t, session, resolveArgs)
	if resolveRes.Resolution.Primary == nil || resolveRes.Resolution.Primary.ID != "review-skill" {
		t.Fatalf("expected resolution primary review-skill, got %#v", resolveRes.Resolution.Primary)
	}

	// 1. Two skill_get calls for the same recommended primary within one session/resolution
	get1 := callSkillGet(t, session, "review-skill")
	if get1.SkillID != "review-skill" {
		t.Fatalf("expected review-skill, got %s", get1.SkillID)
	}
	get2 := callSkillGet(t, session, "review-skill")
	if get2.SkillID != "review-skill" {
		t.Fatalf("expected review-skill, got %s", get2.SkillID)
	}

	// 2. Read resource of active skill (references/checks.md -> reference kind)
	listed, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session, "skills/list", &listSkillsParams{})
	if err != nil {
		t.Fatal(err)
	}
	entry := findSkillEntryByName(t, listed.Skills, "review-skill")
	var checksURI string
	for _, res := range entry.Resources {
		if filepath.Base(res.URI) == "checks.md" {
			checksURI = res.URI
			break
		}
	}
	if checksURI == "" {
		t.Fatalf("checks.md resource not found in entry: %#v", entry.Resources)
	}
	resRead, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: checksURI})
	if err != nil || len(resRead.Contents) != 1 {
		t.Fatalf("read resource checks.md = %#v, %v", resRead, err)
	}

	// 3. Draft skill produces no event
	skillService := app.SkillService{}
	draftCreated, err := skillService.PreviewCreate(t.Context(), root, skill.CreateInput{
		ID:          "draft-skill",
		Collection:  "core",
		Name:        "Draft Skill",
		Description: "A draft skill that is not active.",
		Content:     []byte("---\nname: draft-skill\ndescription: Draft skill.\n---\n\n# Draft\n"),
		Routing:     skill.RoutingInput{Operations: []string{"draft"}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := skillService.ConfirmSkillMutation(t.Context(), root, draftCreated, draftCreated.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("confirm create draft = %#v, %v", result, err)
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	draftGet := callSkillGet(t, session, "draft-skill")
	if draftGet.LifecycleState != "draft" {
		t.Fatalf("expected draft state, got %s", draftGet.LifecycleState)
	}

	// 4. Blocked loads: unapproved third-party skill
	setSkillMeta(t, root, func(doc map[string]any) {
		doc["sources"] = []any{
			map[string]any{
				"id":         "upstream-skill",
				"roles":      []string{"upstream"},
				"kind":       "github",
				"repository": "https://github.com/example/skills",
			},
		}
		doc["quality"] = map[string]any{
			"reviewed": false,
		}
	})

	session2 := newClientSession(t, server)
	blockedGet := callSkillGet(t, session2, "review-skill")
	if blockedGet.Local == nil || blockedGet.Local.Status != app.LocalStatusReviewRequired {
		t.Fatalf("expected review_required, got %#v", blockedGet.Local)
	}
	if blockedGet.Content != "" {
		t.Fatalf("expected empty content for review_required, got %q", blockedGet.Content)
	}

	listedAfter, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session2, "skills/list", &listSkillsParams{})
	if err != nil {
		t.Fatal(err)
	}
	entryAfter := findSkillEntryByName(t, listedAfter.Skills, "review-skill")
	var newChecksURI string
	for _, res := range entryAfter.Resources {
		if filepath.Base(res.URI) == "checks.md" {
			newChecksURI = res.URI
			break
		}
	}
	if newChecksURI == "" {
		t.Fatalf("checks.md not found in updated entry: %#v", entryAfter.Resources)
	}
	_, blockedReadErr := session2.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: newChecksURI})
	if blockedReadErr == nil {
		t.Fatal("expected error reading resource of unapproved skill")
	}

	// Flush and close the recorder to ensure everything is written to SQLite
	flushCtx, cancelFlush := context.WithTimeout(t.Context(), 5*time.Second)
	if err := recorder.Flush(flushCtx); err != nil {
		t.Fatalf("flush telemetry: %v", err)
	}
	cancelFlush()

	closeCtx, cancelClose := context.WithTimeout(t.Context(), 5*time.Second)
	if err := recorder.Close(closeCtx); err != nil {
		t.Fatalf("close telemetry: %v", err)
	}
	cancelClose()

	// Query rollups
	reader, err := (app.TelemetryService{}).Open(root)
	if err != nil {
		t.Fatalf("open telemetry for rollups: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = reader.Close(ctx)
	}()

	rollups, err := reader.Rollups(t.Context(), "", "")
	if err != nil {
		t.Fatalf("query rollups: %v", err)
	}

	counts := make(map[string]map[string]int64)
	for _, r := range rollups {
		if counts[r.SkillID] == nil {
			counts[r.SkillID] = make(map[string]int64)
		}
		counts[r.SkillID][r.Metric] = r.Count
	}

	// Assertions on rollups:
	// review-skill:
	// - load:entrypoint == 2
	// - activation:recommended == 1
	// - load:reference == 1
	// - blocked:review_required == 2
	reviewCounts := counts["review-skill"]
	if reviewCounts == nil {
		t.Fatalf("no rollups found for review-skill: %#v", counts)
	}
	if reviewCounts["load:entrypoint"] != 2 {
		t.Errorf("load:entrypoint = %d, want 2", reviewCounts["load:entrypoint"])
	}
	if reviewCounts["activation:recommended"] != 1 {
		t.Errorf("activation:recommended = %d, want 1", reviewCounts["activation:recommended"])
	}
	if reviewCounts["load:reference"] != 1 {
		t.Errorf("load:reference = %d, want 1", reviewCounts["load:reference"])
	}
	if reviewCounts["blocked:review_required"] != 2 {
		t.Errorf("blocked:review_required = %d, want 2", reviewCounts["blocked:review_required"])
	}

	// draft-skill must have NO rollups
	if counts["draft-skill"] != nil {
		t.Errorf("draft-skill should have no rollups, got: %#v", counts["draft-skill"])
	}

	// Close reader before reading file bytes to ensure clean DB state
	closeReaderCtx, cancelReader := context.WithTimeout(context.Background(), 5*time.Second)
	_ = reader.Close(closeReaderCtx)
	cancelReader()

	// 5. Raw telemetry.db privacy check
	dbBytes, err := os.ReadFile(filepath.Join(root, "runtime", "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(dbBytes, []byte("skill://")) {
		t.Errorf("telemetry.db leaked skill:// URI")
	}
	if bytes.Contains(dbBytes, []byte(root)) {
		t.Errorf("telemetry.db leaked workspace path %q", root)
	}
	if bytes.Contains(dbBytes, []byte(privateDescription)) {
		t.Errorf("telemetry.db leaked task description %q", privateDescription)
	}
}
