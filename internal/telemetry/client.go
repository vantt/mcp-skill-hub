package telemetry

import "strings"

// ClientRemotePrefix starts the client name of a caller that reached the hub
// through its network listener, followed by the normalized host name the
// caller reported in the MCP initialize handshake, for example
// "remote:claude-code". Local callers keep the bare host name, so local
// baselines stay comparable with earlier data, and remote numbers can still be
// compared per host. The label never carries an address or a credential.
const ClientRemotePrefix = "remote:"

// RemoteClient labels client as a remote caller. A missing name becomes
// "other", as for a local caller without clientInfo. Labelling twice keeps a
// single prefix.
func RemoteClient(client Client) Client {
	name := client.Name
	if name == "" {
		name = "other"
	}
	if !strings.HasPrefix(name, ClientRemotePrefix) {
		name = ClientRemotePrefix + name
	}
	client.Name = name
	return client
}
