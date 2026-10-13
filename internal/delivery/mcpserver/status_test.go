package mcpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/app"
)

func TestStatusEndpointNeedsTheTokenAndRevealsOnlyTheVersion(t *testing.T) {
	gate, _, token := newTestGate(t)
	get := func(token, method string) (int, string) {
		recorder := gateRequest(gate, token, "127.0.0.1:50000", func(request *http.Request) {
			request.Method = method
			request.URL.Path = HTTPStatusPath
			request.Body = http.NoBody
		})
		return recorder.Code, recorder.Body.String()
	}
	if code, body := get("", http.MethodGet); code != http.StatusUnauthorized || strings.Contains(body, "remote_api") {
		t.Fatalf("status without a token = %d %q", code, body)
	}
	if code, _ := get(strings.Repeat("0", 64), http.MethodGet); code != http.StatusUnauthorized {
		t.Fatalf("status with a wrong token = %d", code)
	}
	if code, _ := get(token, http.MethodPost); code != http.StatusMethodNotAllowed {
		t.Fatalf("POST to the status endpoint = %d", code)
	}
	code, body := get(token, http.MethodGet)
	if code != http.StatusOK {
		t.Fatalf("status with the token = %d %q", code, body)
	}
	var status map[string]any
	if err := json.Unmarshal([]byte(body), &status); err != nil {
		t.Fatal(err)
	}
	want := app.CurrentHubStatus()
	if len(status) != 3 || status["service"] != "skillhub" || status["version"] != want.Version || status["remote_api"] != float64(app.RemoteAPILevel) {
		t.Fatalf("status body = %s", body)
	}
	if strings.Contains(body, "/") || strings.Contains(body, token) {
		t.Fatalf("status body names a path or the token: %s", body)
	}
}
