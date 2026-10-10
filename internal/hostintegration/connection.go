package hostintegration

import (
	"encoding/json"
	"strings"
)

// RegisteredWorkspace reports an actual skillhub registration and the workspace
// in its arguments. Comments and similarly named servers are not registrations.
// A registered server without --workspace returns an empty workspace, true.
func RegisteredWorkspace(host Host, raw []byte) (string, bool) {
	var args []string
	if host == HostCodex {
		statements, err := scanTOMLStatements(raw)
		if err != nil {
			return "", false
		}
		table, registered := "", false
		for _, statement := range statements {
			if statement.kind == 'h' {
				table = normalizeTOMLTable(statement.name)
				if table == "mcp_servers.skillhub" {
					registered = true
				}
				continue
			}
			if table == "mcp_servers.skillhub" && normalizeTOMLTable(statement.name) == "args" {
				args, err = parseTOMLStringArray(raw, statement)
				if err != nil {
					return "", registered
				}
			}
		}
		if !registered {
			return "", false
		}
	} else {
		var config struct {
			Servers map[string]struct {
				Args []string `json:"args"`
			} `json:"mcpServers"`
		}
		if json.Unmarshal(raw, &config) != nil {
			return "", false
		}
		server, registered := config.Servers["skillhub"]
		if !registered {
			return "", false
		}
		args = server.Args
	}
	for index, arg := range args {
		if arg == "--workspace" && index+1 < len(args) {
			return args[index+1], true
		}
		if strings.HasPrefix(arg, "--workspace=") {
			return strings.TrimPrefix(arg, "--workspace="), true
		}
	}
	return "", true
}
