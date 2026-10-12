package mcpserver

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The size of a tools/list result is measured once per tool set and follows
// the tool set when it changes.
func TestToolsListBytesFollowsTheToolSet(t *testing.T) {
	t.Parallel()
	adapter := &Server{}
	short := &mcp.Tool{Name: "short", Description: "a"}
	long := &mcp.Tool{Name: "long", Description: "a much longer description than the other tool has"}

	measure := func(tools ...*mcp.Tool) int {
		t.Helper()
		result := &mcp.ListToolsResult{Tools: tools}
		size, ok := adapter.toolsListBytes(result)
		if !ok {
			t.Fatal("tools/list result was not measured")
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if want := len(encoded); size != want {
			t.Fatalf("size = %d, want %d", size, want)
		}
		return size
	}

	first := measure(short)
	if again := measure(short); again != first {
		t.Fatalf("same tool set measured %d then %d", first, again)
	}
	if changed := measure(short, long); changed <= first {
		t.Fatalf("larger tool set measured %d, not above %d", changed, first)
	}
}
