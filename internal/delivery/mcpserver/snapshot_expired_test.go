package mcpserver

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestSnapshotExpiredRecoveryWhenSkillEdited(t *testing.T) {
	root := newMCPWorkspace(t)
	adapter, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "skillhub-snapshot-test", Version: "1"}, nil)
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

	// 1. List skills to obtain initial URI for review-skill
	listed, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session, "skills/list", &listSkillsParams{})
	if err != nil {
		t.Fatalf("skills/list failed: %v", err)
	}
	var reviewSkill app.DistributedSkill
	found := false
	for _, entry := range listed.Skills {
		if strings.Contains(entry.URI, "/review-skill/") {
			reviewSkill = app.DistributedSkill{URI: entry.URI}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("review-skill not found in initial listing: %+v", listed.Skills)
	}
	oldURI := reviewSkill.URI

	// 2. Fetch with original URI succeeds
	initialGet, err := mcp.CallCustomMethod[*getSkillParams, *getSkillResult](t.Context(), session, "skills/get", &getSkillParams{URI: oldURI})
	if err != nil {
		t.Fatalf("initial skills/get failed: %v", err)
	}
	if initialGet.Skill.URI != oldURI {
		t.Fatalf("expected URI %s, got %s", oldURI, initialGet.Skill.URI)
	}

	// 3. Edit the skill's SKILL.md after its URI was issued
	skillMDPath := filepath.Join(root, "skills", "core", "review-skill", "SKILL.md")
	content, err := os.ReadFile(skillMDPath)
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	updatedContent := string(content) + "\n<!-- edited after URI was issued -->\n"
	if err := os.WriteFile(skillMDPath, []byte(updatedContent), 0o644); err != nil {
		t.Fatalf("write updated skill: %v", err)
	}

	// Rebuild catalog so the manifest digest changes
	if _, err := (app.CatalogService{}).EnsureCatalog(t.Context(), root); err != nil {
		t.Fatalf("ensure catalog: %v", err)
	}

	// 4. Calling skills/get with the old URI returns snapshot_expired with current_uri
	_, err = mcp.CallCustomMethod[*getSkillParams, *getSkillResult](t.Context(), session, "skills/get", &getSkillParams{URI: oldURI})
	if err == nil {
		t.Fatal("expected skills/get on expired URI to fail")
	}

	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("expected jsonrpc.Error, got: %T (%v)", err, err)
	}
	if rpcErr.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("expected CodeInvalidParams, got: %d", rpcErr.Code)
	}

	var errData struct {
		Code       string `json:"code"`
		CurrentURI string `json:"current_uri"`
	}
	if err := json.Unmarshal(rpcErr.Data, &errData); err != nil {
		t.Fatalf("unmarshal error data (%s): %v", string(rpcErr.Data), err)
	}
	if errData.Code != "snapshot_expired" {
		t.Fatalf("expected code snapshot_expired, got: %s", errData.Code)
	}
	if errData.CurrentURI == "" || errData.CurrentURI == oldURI {
		t.Fatalf("expected fresh current_uri different from oldURI, got: %q (old: %q)", errData.CurrentURI, oldURI)
	}
	wantMsg := "the skill was updated; call skills/get " + errData.CurrentURI
	if rpcErr.Message != wantMsg {
		t.Fatalf("expected message %q, got %q", wantMsg, rpcErr.Message)
	}

	// 5. Calling skills/get with the provided current_uri succeeds
	recovered, err := mcp.CallCustomMethod[*getSkillParams, *getSkillResult](t.Context(), session, "skills/get", &getSkillParams{URI: errData.CurrentURI})
	if err != nil {
		t.Fatalf("recovered skills/get failed: %v", err)
	}
	if recovered.Skill.URI != errData.CurrentURI {
		t.Fatalf("expected recovered URI %s, got %s", errData.CurrentURI, recovered.Skill.URI)
	}

	// 6. ReadResource with oldURI also returns snapshot_expired with current_uri
	_, err = session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: oldURI})
	if err == nil {
		t.Fatal("expected ReadResource on expired URI to fail")
	}
	var resRpcErr *jsonrpc.Error
	if !errors.As(err, &resRpcErr) {
		t.Fatalf("expected jsonrpc.Error for ReadResource, got: %T (%v)", err, err)
	}
	var resErrData struct {
		Code       string `json:"code"`
		CurrentURI string `json:"current_uri"`
	}
	if err := json.Unmarshal(resRpcErr.Data, &resErrData); err != nil {
		t.Fatalf("unmarshal ReadResource error data: %v", err)
	}
	if resErrData.Code != "snapshot_expired" || resErrData.CurrentURI != errData.CurrentURI {
		t.Fatalf("expected snapshot_expired with %s, got code=%s current_uri=%s", errData.CurrentURI, resErrData.Code, resErrData.CurrentURI)
	}

	_ = adapter
}
