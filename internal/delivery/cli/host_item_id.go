package cli

import (
	"strings"

	"github.com/vantt/skillhub-mcp/internal/app"
)

// parseHostItemID splits the ID of a host file item: host_<host>_<kind>, and
// for a skill file host_<host>_<kind>:<path relative to the root>. Host keys
// and kinds contain neither "_" nor ":"; the path may contain both.
func parseHostItemID(id string) (host, kind, relPath string, ok bool) {
	parts := strings.SplitN(id, "_", 3)
	if len(parts) != 3 || parts[0] != "host" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	kind, relPath, _ = strings.Cut(parts[2], ":")
	if kind == "" {
		return "", "", "", false
	}
	return parts[1], kind, relPath, true
}

// hostItemWritten reports whether a host item stands for a file a plan
// writes or deletes, as opposed to a system skill file it left unchanged.
func hostItemWritten(item app.Item) bool {
	return !strings.HasPrefix(item.Impact, app.BlockedImpactPrefix)
}
