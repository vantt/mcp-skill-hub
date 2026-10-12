package hostintegration

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed matrix.json
var matrixData []byte

type stockClientMatrixEntry struct {
	Client       string `json:"client"`
	ServerToggle *struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"server_toggle"`
	SkillsExtension *struct {
		Status   string `json:"status"`
		Reason   string `json:"reason"`
		Evidence string `json:"evidence"`
	} `json:"skills_extension"`
}

type matrixDocument struct {
	StockClients []stockClientMatrixEntry `json:"stock_clients"`
}

// HostSupportsServerToggle checks whether docs/mcp-compatibility-matrix.json
// marks the host as verified to dynamically enable and disable each server.
func HostSupportsServerToggle(host Host) bool {
	return hostSupportsServerToggleFromData(host, matrixData)
}

func hostSupportsServerToggleFromData(host Host, data []byte) bool {
	var doc matrixDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	expectedName := ""
	switch host {
	case HostClaude:
		expectedName = "Claude Code"
	case HostCodex:
		expectedName = "Codex CLI"
	case HostGemini:
		expectedName = "Gemini CLI"
	default:
		return false
	}
	for _, client := range doc.StockClients {
		if strings.EqualFold(client.Client, expectedName) {
			if client.ServerToggle != nil && client.ServerToggle.Status == "verified" {
				return true
			}
			return false
		}
	}
	return false
}

// parsedMatrix decodes the embedded matrix once; every skills/list and
// skills/get asks it, and the matrix never changes while the process runs.
var parsedMatrix = newParsedMatrix()

func newParsedMatrix() func() (matrixDocument, error) {
	return sync.OnceValues(func() (matrixDocument, error) {
		var doc matrixDocument
		err := json.Unmarshal(matrixData, &doc)
		return doc, err
	})
}

// HostSupportsSkillsExtension selects MCP-only curator delivery only for verified clients.
func HostSupportsSkillsExtension(host Host) bool {
	doc, err := parsedMatrix()
	if err != nil {
		return false
	}
	names := map[Host]string{HostClaude: "Claude Code", HostCodex: "Codex CLI", HostGemini: "Gemini CLI"}
	name, ok := names[host]
	if !ok {
		return false
	}
	for _, client := range doc.StockClients {
		if strings.EqualFold(client.Client, name) {
			return client.SkillsExtension != nil && client.SkillsExtension.Status == "verified" && client.SkillsExtension.Evidence != ""
		}
	}
	return false
}
