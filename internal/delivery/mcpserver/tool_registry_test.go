package mcpserver

import (
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectLazyToolServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	_, server, err := New(newMCPWorkspace(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "lazy", Version: "1"}, nil).Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestFirstCallsToAnUnregisteredToolAllSucceedTogether(t *testing.T) {
	session := connectLazyToolServer(t)
	var group sync.WaitGroup
	failures := make(chan string, 16)
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_list", Arguments: map[string]any{}})
			if err != nil || result.IsError {
				failures <- "skill_list failed on a concurrent first call"
			}
		}()
	}
	group.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
}

func TestFirstCallToAnUnregisteredToolStillValidatesItsArguments(t *testing.T) {
	session := connectLazyToolServer(t)
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_list", Arguments: map[string]any{"limit": "many"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || len(result.Content) == 0 {
		t.Fatalf("a malformed first call must be refused, got %+v", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "validating") {
		t.Fatalf("expected a schema validation error, got %+v", result.Content[0])
	}
}
