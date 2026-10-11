package hostintegration

import (
	"encoding/json"
	"strings"
)

// RegisteredWorkspace reports an actual skillhub registration and the workspace
// in its arguments. Comments and similarly named servers are not registrations.
// A registered server without --workspace returns an empty workspace, true.
func RegisteredWorkspace(host Host, raw []byte) (string, bool) {
	registration := ReadRegistration(host, raw)
	return registration.Workspace, registration.Present
}

// Registration describes the skillhub entry of one host config file: absent,
// a local hub (`mcp serve --workspace <path>`), or a remote hub reached
// through the stdio proxy (`mcp proxy --hub <url>`).
type Registration struct {
	Present   bool
	Workspace string
	HubURL    string
	TokenFile string
}

// Remote reports whether the entry starts the proxy for a hub on another machine.
func (registration Registration) Remote() bool { return registration.HubURL != "" }

// ReadRegistration parses the skillhub entry of a host config file. Comments
// and similarly named servers are not registrations.
func ReadRegistration(host Host, raw []byte) Registration {
	args, registered := registeredArgs(host, raw)
	if !registered {
		return Registration{}
	}
	registration := Registration{Present: true}
	if len(args) >= 2 && args[0] == "mcp" && args[1] == "proxy" {
		registration.HubURL = argumentValue(args[2:], "--hub")
		registration.TokenFile = argumentValue(args[2:], "--token-file")
		return registration
	}
	registration.Workspace = argumentValue(args, "--workspace")
	return registration
}

// argumentValue returns the value of `--name value` or `--name=value`.
func argumentValue(args []string, name string) string {
	for index, arg := range args {
		if arg == name && index+1 < len(args) {
			return args[index+1]
		}
		if strings.HasPrefix(arg, name+"=") {
			return strings.TrimPrefix(arg, name+"=")
		}
	}
	return ""
}

// registeredArgs returns the arguments of the skillhub server entry and
// whether the entry exists at all.
func registeredArgs(host Host, raw []byte) ([]string, bool) {
	if host == HostCodex {
		statements, err := scanTOMLStatements(raw)
		if err != nil {
			return nil, false
		}
		var args []string
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
					return nil, registered
				}
			}
		}
		return args, registered
	}
	var config struct {
		Servers map[string]struct {
			Args []string `json:"args"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal(raw, &config) != nil {
		return nil, false
	}
	server, registered := config.Servers["skillhub"]
	if !registered {
		return nil, false
	}
	return server.Args, true
}
