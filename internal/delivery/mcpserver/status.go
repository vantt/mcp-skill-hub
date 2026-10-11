package mcpserver

import (
	"encoding/json"
	"net/http"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

// HTTPStatusPath answers the build version and the remote protocol level of
// the hub, so `skillhub doctor` on a client machine can tell an older or a
// newer hub apart. It sits behind the same gate as the MCP endpoint (Host,
// token, rate limit) and reveals no path, workspace or secret.
const HTTPStatusPath = app.HubStatusPath

func serveHubStatus(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "the status endpoint answers GET only", http.StatusMethodNotAllowed)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(app.CurrentHubStatus())
}
