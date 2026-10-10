package hostintegration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMatrixSynchronized(t *testing.T) {
	docPath := filepath.Join("..", "..", "docs", "mcp-compatibility-matrix.json")
	docData, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read docs/mcp-compatibility-matrix.json: %v", err)
	}
	if !bytes.Equal(docData, matrixData) {
		t.Fatal("internal/hostintegration/matrix.json and docs/mcp-compatibility-matrix.json differ; keep both copies synchronized")
	}
}

func TestStockClientSkillsExtensionEvidence(t *testing.T) {
	var doc struct {
		StockClients []struct {
			Client          string `json:"client"`
			SkillsExtension *struct {
				Status   string `json:"status"`
				Reason   string `json:"reason"`
				Evidence string `json:"evidence"`
			} `json:"skills_extension"`
		} `json:"stock_clients"`
	}
	if err := json.Unmarshal(matrixData, &doc); err != nil {
		t.Fatal(err)
	}
	for _, client := range doc.StockClients {
		capability := client.SkillsExtension
		if capability == nil {
			t.Errorf("%s: missing skills_extension", client.Client)
			continue
		}
		switch capability.Status {
		case "verified":
			if capability.Evidence == "" {
				t.Errorf("%s: verified without evidence", client.Client)
			}
		case "unverified", "blocked_unverified":
			if capability.Reason == "" {
				t.Errorf("%s: unverified without reason", client.Client)
			}
		default:
			t.Errorf("%s: invalid status %q", client.Client, capability.Status)
		}
	}
}

func TestHostSupportsServerToggle(t *testing.T) {
	// Baseline: in matrixData, all stock clients are unverified for server toggle
	for _, host := range []Host{HostClaude, HostCodex, HostGemini} {
		if HostSupportsServerToggle(host) {
			t.Fatalf("expected host %s to be unverified for server toggle in matrix", host)
		}
	}

	// When marked verified, hostSupportsServerToggleFromData returns true
	verifiedJSON := []byte(`{
		"stock_clients": [
			{"client": "Claude Code", "server_toggle": {"status": "verified"}},
			{"client": "Codex CLI", "server_toggle": {"status": "unverified"}},
			{"client": "Gemini CLI", "server_toggle": {"status": "blocked_unverified"}}
		]
	}`)
	if !hostSupportsServerToggleFromData(HostClaude, verifiedJSON) {
		t.Fatal("expected Claude Code to support server toggle when marked verified")
	}
	if hostSupportsServerToggleFromData(HostCodex, verifiedJSON) {
		t.Fatal("expected Codex CLI to not support server toggle when unverified")
	}
	if hostSupportsServerToggleFromData(HostGemini, verifiedJSON) {
		t.Fatal("expected Gemini CLI to not support server toggle when blocked_unverified")
	}
}
