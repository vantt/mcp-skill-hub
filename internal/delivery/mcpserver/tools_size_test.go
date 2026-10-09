package mcpserver

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolsListSize(t *testing.T) {
	root := newMCPWorkspace(t)
	_, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "skillhub-measurement", Version: "1"}, nil)
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

	listedTools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}

	rawTotalJSON, err := json.Marshal(listedTools)
	if err != nil {
		t.Fatal(err)
	}
	totalBytes := len(rawTotalJSON)
	totalTokens := totalBytes / 4

	type toolStat struct {
		name             string
		toolBytes        int
		toolTokens       int
		outputBytes      int
		outputShareRatio float64
	}

	var stats []toolStat
	totalOutputBytes := 0

	for _, tool := range listedTools.Tools {
		toolJSON, _ := json.Marshal(tool)
		tBytes := len(toolJSON)
		tTokens := tBytes / 4

		outJSON, _ := json.Marshal(tool.OutputSchema)
		oBytes := 0
		if string(outJSON) != "null" && len(outJSON) > 0 {
			oBytes = len(outJSON)
		}
		totalOutputBytes += oBytes

		ratio := 0.0
		if tBytes > 0 {
			ratio = float64(oBytes) / float64(tBytes) * 100.0
		}

		stats = append(stats, toolStat{
			name:             tool.Name,
			toolBytes:        tBytes,
			toolTokens:       tTokens,
			outputBytes:      oBytes,
			outputShareRatio: ratio,
		})
	}

	// Sort descending by toolBytes
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].toolBytes > stats[j].toolBytes
	})

	t.Logf("=== MCP tools/list Measurement Report ===")
	t.Logf("Total tools: %d", len(listedTools.Tools))
	t.Logf("Total payload: %d bytes (~%d tokens)", totalBytes, totalTokens)
	t.Logf("Total outputSchema bytes: %d (%.1f%% of payload)", totalOutputBytes, float64(totalOutputBytes)/float64(totalBytes)*100.0)
	t.Logf("\nPer-tool breakdown (sorted by size descending):")
	t.Logf("%-32s | %8s | %8s | %12s | %10s", "Tool Name", "Bytes", "~Tokens", "Output Bytes", "Output %")
	t.Logf("---------------------------------+----------+----------+--------------+-----------")
	for _, s := range stats {
		t.Logf("%-32s | %8d | %8d | %12d | %9.1f%%", s.name, s.toolBytes, s.toolTokens, s.outputBytes, s.outputShareRatio)
	}

	// Runtime set measurement
	runtimeSet := map[string]bool{
		"skill_resolve":  true,
		"skill_get":      true,
		"skill_feedback": true,
	}

	var runtimeTools []*mcp.Tool
	var runtimeToolsNoOutput []*mcp.Tool
	for _, tool := range listedTools.Tools {
		if runtimeSet[tool.Name] {
			runtimeTools = append(runtimeTools, tool)
			stripped := *tool
			stripped.OutputSchema = nil
			runtimeToolsNoOutput = append(runtimeToolsNoOutput, &stripped)
		}
	}

	runtimeRawJSON, _ := json.Marshal(map[string]any{"tools": runtimeTools})
	runtimeBytes := len(runtimeRawJSON)
	runtimeTokens := runtimeBytes / 4

	runtimeNoOutputJSON, _ := json.Marshal(map[string]any{"tools": runtimeToolsNoOutput})
	runtimeNoOutputBytes := len(runtimeNoOutputJSON)
	runtimeNoOutputTokens := runtimeNoOutputBytes / 4

	t.Logf("\n=== Hypothetical Runtime Profile {skill_resolve, skill_get, skill_feedback} ===")
	t.Logf("Runtime tools count: %d", len(runtimeTools))
	t.Logf("Runtime total payload: %d bytes (~%d tokens)", runtimeBytes, runtimeTokens)
	t.Logf("Runtime payload without outputSchema: %d bytes (~%d tokens)", runtimeNoOutputBytes, runtimeNoOutputTokens)
	if runtimeBytes > 0 {
		runtimeOutputShare := float64(runtimeBytes-runtimeNoOutputBytes) / float64(runtimeBytes) * 100.0
		t.Logf("outputSchema share in runtime profile: %.1f%%", runtimeOutputShare)
	}

	if totalBytes == 0 || len(listedTools.Tools) == 0 {
		t.Fatal("expected tools/list payload to be non-empty")
	}
	if len(runtimeTools) != len(runtimeSet) {
		t.Fatalf("expected %d runtime tools, found %d", len(runtimeSet), len(runtimeTools))
	}
	if len(runtimeTools) >= len(listedTools.Tools) {
		t.Fatalf("expected runtime tools (%d) to be a strict subset of all tools (%d)", len(runtimeTools), len(listedTools.Tools))
	}
}
