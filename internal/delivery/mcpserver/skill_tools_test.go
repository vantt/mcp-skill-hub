package mcpserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

func newEmptyMCPWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (app.WorkspaceService{}).Init(root, true); err != nil {
		t.Fatal(err)
	}
	return root
}

func connectInMemoryServer(t *testing.T, root string) *mcp.ClientSession {
	t.Helper()
	adapter, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = adapter
	client := mcp.NewClient(&mcp.Implementation{Name: "skill-tools-test", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
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

func TestSkillUpdatePreviewUnknownID(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)
	session := connectInMemoryServer(t, root)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_update_preview",
		Arguments: map[string]any{
			"skill_id":    "nonexistent-skill",
			"description": "Updated description",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected isError=true for unknown skill update")
	}
	var outcome toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, res, &outcome)
	if outcome.Error == nil {
		t.Fatal("expected error outcome")
	}
	wantMsg := "Skill nonexistent-skill does not exist; use skill_create_preview"
	if !strings.Contains(outcome.Error.Message, wantMsg) {
		t.Fatalf("error message = %q, want containing %q", outcome.Error.Message, wantMsg)
	}
}

func TestSkillToolsLifecycleInMemory(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)
	session := connectInMemoryServer(t, root)

	// 1. Create preview
	createPrevRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_create_preview",
		Arguments: map[string]any{
			"skill_id":    "reliability-review",
			"name":        "Reliability Review",
			"description": "Review code for reliability risks.",
		},
	})
	if err != nil || createPrevRes.IsError {
		t.Fatalf("create preview failed: %#v, %v", createPrevRes, err)
	}
	var createPrevOutcome toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, createPrevRes, &createPrevOutcome)
	if createPrevOutcome.Result == nil || createPrevOutcome.Result.SkillID != "reliability-review" {
		t.Fatalf("unexpected create preview outcome: %#v", createPrevOutcome)
	}
	pins := createPrevOutcome.Result.Confirmation.Confirmation.Pins

	// 2. Create confirm
	createConfRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_create_confirm",
		Arguments: map[string]any{
			"proposal_id":     pins.ProposalID,
			"proposal_digest": pins.ProposalDigest,
			"base_version":    pins.BaseVersion,
		},
	})
	if err != nil || createConfRes.IsError {
		t.Fatalf("create confirm failed: %#v, %v", createConfRes, err)
	}
	var createConfOutcome toolOutcome[app.SkillMutationResult]
	decodeStructuredContent(t, createConfRes, &createConfOutcome)
	if createConfOutcome.Result == nil || createConfOutcome.Result.State != "draft" {
		t.Fatalf("unexpected create confirm outcome: %#v", createConfOutcome)
	}

	// 3. Activation preview before routing is configured must report ALL missing requirements
	actPrevRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_transition_preview",
		Arguments: map[string]any{
			"skill_id": "reliability-review",
			"target":   "active",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !actPrevRes.IsError {
		t.Fatal("expected activation preview to fail due to missing requirements")
	}
	var actPrevOutcome toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, actPrevRes, &actPrevOutcome)
	if actPrevOutcome.Error == nil {
		t.Fatal("expected error outcome")
	}
	for _, req := range []string{"trigger", "not_for or rationale", "min_scope"} {
		if !strings.Contains(actPrevOutcome.Error.Message, req) {
			t.Errorf("expected error message to contain missing requirement %q; got %q", req, actPrevOutcome.Error.Message)
		}
	}

	// 4. Update preview with complete routing fields
	content := "# Reliability Review\n\nInspect error handling and retry loops.\n"
	updPrevRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_update_preview",
		Arguments: map[string]any{
			"skill_id": "reliability-review",
			"content":  content,
			"routing": map[string]any{
				"operations": []string{"review"},
				"triggers":   []string{"review reliability failure handling"},
				"not_for":    []string{"write marketing copy"},
				"min_scope":  "multi_step",
			},
		},
	})
	if err != nil || updPrevRes.IsError {
		t.Fatalf("update preview failed: %#v, %v", updPrevRes, err)
	}
	var updPrevOutcome toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, updPrevRes, &updPrevOutcome)
	updPins := updPrevOutcome.Result.Confirmation.Confirmation.Pins

	// 5. Update confirm
	updConfRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_update_confirm",
		Arguments: map[string]any{
			"proposal_id":     updPins.ProposalID,
			"proposal_digest": updPins.ProposalDigest,
			"base_version":    updPins.BaseVersion,
		},
	})
	if err != nil || updConfRes.IsError {
		t.Fatalf("update confirm failed: %#v, %v", updConfRes, err)
	}

	// 6. Transition preview to active now succeeds
	actPrev2Res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_transition_preview",
		Arguments: map[string]any{
			"skill_id": "reliability-review",
			"target":   "active",
		},
	})
	if err != nil || actPrev2Res.IsError {
		t.Fatalf("transition preview failed: %#v, %v", actPrev2Res, err)
	}
	var actPrev2Outcome toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, actPrev2Res, &actPrev2Outcome)
	actPins := actPrev2Outcome.Result.Confirmation.Confirmation.Pins

	// 7. Transition confirm to active
	actConfRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_transition_confirm",
		Arguments: map[string]any{
			"proposal_id":     actPins.ProposalID,
			"proposal_digest": actPins.ProposalDigest,
			"base_version":    actPins.BaseVersion,
		},
	})
	if err != nil || actConfRes.IsError {
		t.Fatalf("transition confirm failed: %#v, %v", actConfRes, err)
	}
	var actConfOutcome toolOutcome[app.SkillMutationResult]
	decodeStructuredContent(t, actConfRes, &actConfOutcome)
	if actConfOutcome.Result == nil || actConfOutcome.Result.State != "active" {
		t.Fatalf("unexpected state after activation: %#v", actConfOutcome)
	}

	// 8. skill_list shows active skill
	listRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "skill_list",
		Arguments: map[string]any{"state": "active"},
	})
	if err != nil || listRes.IsError {
		t.Fatalf("skill_list failed: %#v, %v", listRes, err)
	}
	var listOutcome toolOutcome[app.SkillListResult]
	decodeStructuredContent(t, listRes, &listOutcome)
	if listOutcome.Result == nil || len(listOutcome.Result.Skills) != 1 || listOutcome.Result.Skills[0].ID != "reliability-review" {
		t.Fatalf("unexpected skill_list result: %#v", listOutcome)
	}

	// Filter by draft should return empty
	listDraftRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "skill_list",
		Arguments: map[string]any{"state": "draft"},
	})
	if err != nil || listDraftRes.IsError {
		t.Fatalf("skill_list draft failed: %#v, %v", listDraftRes, err)
	}
	var listDraftOutcome toolOutcome[app.SkillListResult]
	decodeStructuredContent(t, listDraftRes, &listDraftOutcome)
	if listDraftOutcome.Result == nil || len(listDraftOutcome.Result.Skills) != 0 {
		t.Fatalf("expected 0 draft skills; got %#v", listDraftOutcome)
	}

	// 9. skill_get returns routing fields, path, content, status
	getRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "skill_get",
		Arguments: map[string]any{"skill_id": "reliability-review"},
	})
	if err != nil || getRes.IsError {
		t.Fatalf("skill_get failed: %#v, %v", getRes, err)
	}
	var getOutcome toolOutcome[skillGetResult]
	decodeStructuredContent(t, getRes, &getOutcome)
	if getOutcome.Result == nil {
		t.Fatal("skill_get result is nil")
	}
	got := getOutcome.Result
	if got.SkillID != "reliability-review" || got.Status != "active" {
		t.Fatalf("unexpected skill_get identity: %#v", got)
	}
	if !strings.HasSuffix(got.Path, "SKILL.md") {
		t.Fatalf("unexpected skill_get path: %s", got.Path)
	}
	if !strings.Contains(got.Content, "Inspect error handling") {
		t.Fatalf("unexpected skill_get content: %s", got.Content)
	}
	sum := sha256.Sum256([]byte(got.Content))
	wantDigest := "sha256:" + hex.EncodeToString(sum[:])
	if got.ContentDigest != wantDigest {
		t.Fatalf("content_digest = %q, want %q", got.ContentDigest, wantDigest)
	}
	if got.StateBasis != "canonical" {
		t.Fatalf("state_basis = %q, want canonical", got.StateBasis)
	}
	if got.LifecycleState != "active" {
		t.Fatalf("lifecycle_state = %q, want active", got.LifecycleState)
	}
	if len(got.Routing.Triggers) != 1 || got.Routing.Triggers[0] != "review reliability failure handling" {
		t.Fatalf("unexpected routing triggers: %#v", got.Routing)
	}
	if len(got.Routing.NotFor) != 1 || got.Routing.NotFor[0] != "write marketing copy" {
		t.Fatalf("unexpected routing not_for: %#v", got.Routing)
	}
	if got.Routing.MinScope != "multi_step" {
		t.Fatalf("unexpected routing min_scope: %s", got.Routing.MinScope)
	}

	// skill_get with unknown ID
	getUnknownRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "skill_get",
		Arguments: map[string]any{"skill_id": "unknown-id"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !getUnknownRes.IsError {
		t.Fatal("expected skill_get on unknown ID to return error")
	}
	var getUnknownOutcome toolOutcome[skillGetResult]
	decodeStructuredContent(t, getUnknownRes, &getUnknownOutcome)
	if getUnknownOutcome.Error == nil || getUnknownOutcome.Error.Code != "not_found" {
		t.Fatalf("expected not_found error; got %#v", getUnknownOutcome)
	}

	// 10. Deprecate and archive transitions
	depPrevRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_transition_preview",
		Arguments: map[string]any{
			"skill_id": "reliability-review",
			"target":   "deprecated",
		},
	})
	if err != nil || depPrevRes.IsError {
		t.Fatalf("deprecate preview failed: %#v, %v", depPrevRes, err)
	}
	var depPrevOutcome toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, depPrevRes, &depPrevOutcome)
	depPins := depPrevOutcome.Result.Confirmation.Confirmation.Pins

	depConfRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_transition_confirm",
		Arguments: map[string]any{
			"proposal_id":     depPins.ProposalID,
			"proposal_digest": depPins.ProposalDigest,
			"base_version":    depPins.BaseVersion,
		},
	})
	if err != nil || depConfRes.IsError {
		t.Fatalf("deprecate confirm failed: %#v, %v", depConfRes, err)
	}

	arcPrevRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_transition_preview",
		Arguments: map[string]any{
			"skill_id": "reliability-review",
			"target":   "archived",
		},
	})
	if err != nil || arcPrevRes.IsError {
		t.Fatalf("archive preview failed: %#v, %v", arcPrevRes, err)
	}
	var arcPrevOutcome toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, arcPrevRes, &arcPrevOutcome)
	arcPins := arcPrevOutcome.Result.Confirmation.Confirmation.Pins

	arcConfRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_transition_confirm",
		Arguments: map[string]any{
			"proposal_id":     arcPins.ProposalID,
			"proposal_digest": arcPins.ProposalDigest,
			"base_version":    arcPins.BaseVersion,
		},
	})
	if err != nil || arcConfRes.IsError {
		t.Fatalf("archive confirm failed: %#v, %v", arcConfRes, err)
	}
	var arcConfOutcome toolOutcome[app.SkillMutationResult]
	decodeStructuredContent(t, arcConfRes, &arcConfOutcome)
	if arcConfOutcome.Result == nil || arcConfOutcome.Result.State != "archived" {
		t.Fatalf("unexpected state after archive: %#v", arcConfOutcome)
	}
}

func TestSkillToolsStdioSubprocess(t *testing.T) {
	t.Parallel()
	binary := buildSkillHub(t)
	root := filepath.Join(t.TempDir(), "workspace")

	initCmd := exec.Command(binary, "init", root, "--yes")
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("init failed: %v: %s", err, out)
	}

	var stderr bytes.Buffer
	command := exec.Command(binary, "mcp", "serve", "--workspace", root)
	command.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-skill-tools-test", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	session, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect stdio: %v; stderr=%s", err, stderr.String())
	}
	defer session.Close()

	// 1. Create preview
	createPrevRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_create_preview",
		Arguments: map[string]any{
			"skill_id":    "stdio-test-skill",
			"name":        "Stdio Test Skill",
			"description": "Tested over real stdio transport.",
			"content":     "# Stdio Test Skill\n\nMeaningful procedures.\n",
			"routing": map[string]any{
				"triggers":  []string{"stdio test trigger"},
				"not_for":   []string{"unrelated"},
				"min_scope": "single_step",
			},
		},
	})
	if err != nil || createPrevRes.IsError {
		t.Fatalf("stdio create preview: %#v, %v", createPrevRes, err)
	}
	var createPrevOutcome toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, createPrevRes, &createPrevOutcome)
	pins := createPrevOutcome.Result.Confirmation.Confirmation.Pins

	// 2. Create confirm
	createConfRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_create_confirm",
		Arguments: map[string]any{
			"proposal_id":     pins.ProposalID,
			"proposal_digest": pins.ProposalDigest,
			"base_version":    pins.BaseVersion,
		},
	})
	if err != nil || createConfRes.IsError {
		t.Fatalf("stdio create confirm: %#v, %v", createConfRes, err)
	}

	// 3. Transition to active preview
	actPrevRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_transition_preview",
		Arguments: map[string]any{
			"skill_id": "stdio-test-skill",
			"target":   "active",
		},
	})
	if err != nil || actPrevRes.IsError {
		t.Fatalf("stdio transition preview: %#v, %v", actPrevRes, err)
	}
	var actPrevOutcome toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, actPrevRes, &actPrevOutcome)
	actPins := actPrevOutcome.Result.Confirmation.Confirmation.Pins

	// 4. Transition confirm
	actConfRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_transition_confirm",
		Arguments: map[string]any{
			"proposal_id":     actPins.ProposalID,
			"proposal_digest": actPins.ProposalDigest,
			"base_version":    actPins.BaseVersion,
		},
	})
	if err != nil || actConfRes.IsError {
		t.Fatalf("stdio transition confirm: %#v, %v", actConfRes, err)
	}

	// 5. List shows it
	listRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "skill_list",
		Arguments: map[string]any{"state": "active"},
	})
	if err != nil || listRes.IsError {
		t.Fatalf("stdio skill_list: %#v, %v", listRes, err)
	}
	var listOutcome toolOutcome[app.SkillListResult]
	decodeStructuredContent(t, listRes, &listOutcome)
	if listOutcome.Result == nil || len(listOutcome.Result.Skills) != 1 || listOutcome.Result.Skills[0].ID != "stdio-test-skill" {
		t.Fatalf("stdio skill_list mismatch: %#v", listOutcome)
	}

	// 6. Get shows routing
	getRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "skill_get",
		Arguments: map[string]any{"skill_id": "stdio-test-skill"},
	})
	if err != nil || getRes.IsError {
		t.Fatalf("stdio skill_get: %#v, %v", getRes, err)
	}
	var getOutcome toolOutcome[skillGetResult]
	decodeStructuredContent(t, getRes, &getOutcome)
	if getOutcome.Result == nil || getOutcome.Result.Status != "active" {
		t.Fatalf("stdio skill_get mismatch: %#v", getOutcome)
	}
	if len(getOutcome.Result.Routing.Triggers) != 1 || getOutcome.Result.Routing.Triggers[0] != "stdio test trigger" {
		t.Fatalf("stdio skill_get routing triggers: %#v", getOutcome.Result.Routing)
	}
}

func TestSkillUpdatePreviewWithExpectedContentDigest(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)
	session := connectInMemoryServer(t, root)

	// 1. Create a draft skill
	createRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_create_preview",
		Arguments: map[string]any{
			"skill_id":    "digest-test-skill",
			"name":        "Digest Test",
			"description": "Testing expected content digest.",
			"content":     "# Initial Instructions\n",
		},
	})
	if err != nil || createRes.IsError {
		t.Fatalf("create preview failed: %#v, %v", createRes, err)
	}
	var createOutcome toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, createRes, &createOutcome)
	pins := createOutcome.Result.Confirmation.Confirmation.Pins

	confRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "skill_create_confirm",
		Arguments: map[string]any{"proposal_id": pins.ProposalID, "proposal_digest": pins.ProposalDigest, "base_version": pins.BaseVersion},
	})
	if err != nil || confRes.IsError {
		t.Fatalf("create confirm failed: %#v, %v", confRes, err)
	}

	// 2. Read skill to obtain exact content digest
	getRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "skill_get",
		Arguments: map[string]any{"skill_id": "digest-test-skill"},
	})
	if err != nil || getRes.IsError {
		t.Fatalf("skill_get failed: %#v, %v", getRes, err)
	}
	var getOutcome toolOutcome[skillGetResult]
	decodeStructuredContent(t, getRes, &getOutcome)
	initialDigest := getOutcome.Result.ContentDigest
	if initialDigest == "" {
		t.Fatal("content_digest was empty")
	}

	// 3. Stale digest update preview fails with edit_conflict
	staleDigest := "sha256:" + strings.Repeat("a", 64)
	staleRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_update_preview",
		Arguments: map[string]any{
			"skill_id":                "digest-test-skill",
			"content":                 "# Updated Body\n",
			"expected_content_digest": staleDigest,
		},
	})
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	if !staleRes.IsError {
		t.Fatal("expected stale expected_content_digest to fail")
	}
	var staleOutcome toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, staleRes, &staleOutcome)
	if staleOutcome.Error == nil || staleOutcome.Error.Code != "edit_conflict" {
		t.Fatalf("expected edit_conflict error, got %#v", staleOutcome.Error)
	}

	// 4. Matching digest update preview succeeds
	matchRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_update_preview",
		Arguments: map[string]any{
			"skill_id":                "digest-test-skill",
			"content":                 "# Updated Body With Matching Digest\n",
			"expected_content_digest": initialDigest,
		},
	})
	if err != nil || matchRes.IsError {
		t.Fatalf("matching digest update preview failed: %#v, %v", matchRes, err)
	}

	// 5. Omitted digest update preview succeeds (blind replacement)
	blindRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_update_preview",
		Arguments: map[string]any{
			"skill_id": "digest-test-skill",
			"content":  "# Blind Replacement\n",
		},
	})
	if err != nil || blindRes.IsError {
		t.Fatalf("blind replacement update preview failed: %#v, %v", blindRes, err)
	}
}
