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

	t.Logf("=== Profile %q (all) tools/list Measurement Report ===", ProfileAll)
	t.Logf("Total tools: %d", len(listedTools.Tools))
	t.Logf("Total payload: %d bytes (~%d tokens)", totalBytes, totalTokens)
	t.Logf("Total outputSchema bytes: %d (%.1f%% of payload)", totalOutputBytes, float64(totalOutputBytes)/float64(totalBytes)*100.0)
	t.Logf("\nPer-tool breakdown (sorted by size descending):")
	t.Logf("%-32s | %8s | %8s | %12s | %10s", "Tool Name", "Bytes", "~Tokens", "Output Bytes", "Output %")
	t.Logf("---------------------------------+----------+----------+--------------+-----------")
	for _, s := range stats {
		t.Logf("%-32s | %8d | %8d | %12d | %9.1f%%", s.name, s.toolBytes, s.toolTokens, s.outputBytes, s.outputShareRatio)
	}

	// Measure real runtime and curation profiles
	measureProfile := func(profile Profile) (int, int, int) {
		_, pServer, pErr := New(root, nil, profile)
		if pErr != nil {
			t.Fatalf("New(%s): %v", profile, pErr)
		}
		pClient := mcp.NewClient(&mcp.Implementation{Name: "skillhub-measurement-" + string(profile), Version: "1"}, nil)
		cTransport, sTransport := mcp.NewInMemoryTransports()
		sSession, pErr := pServer.Connect(t.Context(), sTransport, nil)
		if pErr != nil {
			t.Fatalf("server connect (%s): %v", profile, pErr)
		}
		defer sSession.Close()
		cSession, pErr := pClient.Connect(t.Context(), cTransport, nil)
		if pErr != nil {
			t.Fatalf("client connect (%s): %v", profile, pErr)
		}
		defer cSession.Close()
		tools, pErr := cSession.ListTools(t.Context(), nil)
		if pErr != nil {
			t.Fatalf("list tools (%s): %v", profile, pErr)
		}
		rawJSON, _ := json.Marshal(tools)
		pBytes := len(rawJSON)
		pTokens := pBytes / 4
		pOutputBytes := 0
		for _, tool := range tools.Tools {
			outJSON, _ := json.Marshal(tool.OutputSchema)
			if string(outJSON) != "null" && len(outJSON) > 0 {
				pOutputBytes += len(outJSON)
			}
		}
		t.Logf("\n=== Profile %q tools/list Measurement Report ===", profile)
		t.Logf("%s tools count: %d", profile, len(tools.Tools))
		t.Logf("%s total payload: %d bytes (~%d tokens)", profile, pBytes, pTokens)
		if pBytes > 0 {
			t.Logf("%s outputSchema bytes: %d (%.1f%% of payload)", profile, pOutputBytes, float64(pOutputBytes)/float64(pBytes)*100.0)
		}
		return len(tools.Tools), pBytes, pOutputBytes
	}

	runtimeCount, runtimeBytes, _ := measureProfile(ProfileRuntime)
	curationCount, curationBytes, _ := measureProfile(ProfileCuration)

	if totalBytes == 0 || len(listedTools.Tools) == 0 {
		t.Fatal("expected tools/list payload to be non-empty")
	}
	if runtimeCount != 3 {
		t.Fatalf("expected 3 runtime tools, found %d", runtimeCount)
	}
	if curationCount != len(listedTools.Tools)-3 {
		t.Fatalf("expected %d curation tools, found %d", len(listedTools.Tools)-3, curationCount)
	}
	if runtimeBytes >= totalBytes {
		t.Fatalf("expected runtime payload (%d) to be smaller than all tools payload (%d)", runtimeBytes, totalBytes)
	}
	if curationBytes >= totalBytes {
		t.Fatalf("expected curation payload (%d) to be smaller than all tools payload (%d)", curationBytes, totalBytes)
	}
}
