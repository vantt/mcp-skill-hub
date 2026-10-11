package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/hubtoken"
)

// tokenHandOff tells how the hub's token reaches this machine. It is printed
// by `connect --hub --dry-run`; the token itself is never printed.
func tokenHandOff(hubURL string) string {
	return "To get the token to this machine, copy the hub's token file from machine B (the hub prints its path when `skillhub serve mcp --addr ...` starts, and `skillhub token show` there shows it), for example `scp <machine-b>:<hub-folder>/runtime/config/.hub-token " + hubtoken.ClientPath(hubURL) + "`, then `chmod 600` it; or set " + app.HubTokenEnv + " in the environment the agent host starts the proxy with."
}

// runConnectHub handles `skillhub connect --hub <url>`: the host entry starts
// the stdio proxy for a hub on another machine.
func runConnectHub(ctx context.Context, dir string, flags connectFlags, stdout, stderr io.Writer) int {
	result, connection, err := (app.ConnectService{}).ConnectRemote(ctx, app.RemoteConnectRequest{
		Project: dir, Global: flags.global, Hosts: flags.hosts,
		Hub: flags.hub, TokenFile: flags.tokenFile, DryRun: flags.dryRun,
	})
	if err != nil || result.Error != nil {
		if code, handled := writeConnectSymlinkError(err, stdout, stderr, flags.json); handled {
			return code
		}
		return writeWorkspaceResult(result, err, stdout, stderr, flags.json)
	}
	if flags.json {
		return writeConnectJSON(stdout, stderr, struct {
			app.Result
			Remote *app.RemoteConnection `json:"remote"`
		}{Result: result, Remote: connection})
	}
	p := termui.New(stdout)
	renderRemoteConnect(p, result, connection)
	return printerCode(p)
}

// renderRemoteConnect prints the result of `connect --hub`, or with
// --dry-run what it would change.
func renderRemoteConnect(p *termui.Printer, result app.Result, connection *app.RemoteConnection) {
	p.Line(result.Summary)
	writeChangeLines(p, result, hostChangeLines(result, false))
	if len(connection.Replaces) > 0 {
		verb := "This replaces:"
		if result.Status == app.StatusApplied {
			verb = "This replaced:"
		}
		p.Line(verb)
		var lines []string
		for _, entry := range connection.Replaces {
			lines = append(lines, entry.Host+": "+entry.Target)
		}
		p.Bullets(lines...)
	}
	p.Blank()
	token := "SKILLHUB_HUB_TOKEN in the environment the agent host starts in"
	if connection.TokenFile != "" {
		token = connection.TokenFile + " (only this path is written, never the token)"
	}
	p.Fields(
		termui.Field{Label: "Hub", Value: connection.Hub},
		termui.Field{Label: "Token", Value: token},
		termui.Field{Label: "Hub check", Value: hubCheckSummary(connection.Check)},
		termui.Field{Label: "Skill cache", Value: connection.CacheDir},
	)
	if result.Status != app.StatusReady {
		p.Blank()
		notes := []string{"Curation stays on the hub machine: no curation server and no system-curator skill are installed here."}
		if result.Status == app.StatusActionRequired {
			notes = append(notes, tokenHandOff(connection.Hub))
		}
		p.Bullets(notes...)
	}
	for _, warning := range result.Warnings {
		p.Warning(warning.Summary)
	}
	switch result.Status {
	case app.StatusActionRequired:
		for _, action := range result.SuggestedActions {
			p.Blank()
			p.Line("To write these files, run:")
			p.Command(action.Command)
		}
	case app.StatusApplied:
		p.Blank()
		p.Next("restart your agent so it starts the proxy; if it cannot reach the hub, run `skillhub doctor`", "")
	}
}

// hubCheckSummary says in a few words what the hub answered just now.
func hubCheckSummary(probe app.HubProbe) string {
	switch probe.Outcome {
	case app.ProbeAccepted:
		if probe.Legacy || probe.Status == nil {
			return "reachable, token accepted, an older Skill Hub without version reporting"
		}
		return fmt.Sprintf("reachable, token accepted, Skill Hub %s (this machine: %s)", probe.Status.Version, app.CurrentHubStatus().Version)
	case app.ProbeUnreachable:
		return "not reachable now (" + probe.Reason + ")"
	case app.ProbeRejected:
		return "reachable, but the token is refused"
	case app.ProbeNoToken:
		return "reachable; the token was not checked (no token on this machine)"
	case app.ProbeThrottled:
		return "reachable, but refusing requests for a minute after too many wrong tokens"
	case app.ProbeRateLimited:
		return "reachable, token accepted, but busy (too many requests)"
	case app.ProbeHostRefused:
		return "reachable, but the hub refuses this host name"
	case app.ProbeHubError:
		return fmt.Sprintf("reachable, but answering with an error (HTTP %d)", probe.HTTPStatus)
	}
	return fmt.Sprintf("something answers, but it is not a Skill Hub listener (HTTP %d)", probe.HTTPStatus)
}

// hostChangeLines prints one line per host with the files a plan touches.
func hostChangeLines(result app.Result, removal bool) []string {
	files := map[string][]string{}
	var order []string
	for _, item := range result.Items {
		parts := strings.Split(item.ID, "_")
		if len(parts) < 3 || parts[0] != "host" {
			continue
		}
		if _, seen := files[parts[1]]; !seen {
			order = append(order, parts[1])
		}
		description := formatChangeKind(parts[2], item.Summary)
		if deleted := item.Impact == "Would be deleted." || item.Impact == "Deleted."; deleted && removal {
			description += " deleted"
		} else if deleted {
			description = "remove " + description
		}
		files[parts[1]] = append(files[parts[1]], description)
	}
	verbs := map[app.Status]string{app.StatusActionRequired: "will connect", app.StatusApplied: "connected", app.StatusReady: "already connected"}
	if removal {
		verbs = map[app.Status]string{app.StatusActionRequired: "will remove", app.StatusApplied: "removed"}
	}
	var lines []string
	for _, host := range order {
		lines = append(lines, fmt.Sprintf("%s: %s (%s)", hostDisplayName(host), verbs[result.Status], strings.Join(files[host], ", ")))
	}
	return lines
}

// runConnectRemove handles `skillhub connect --remove`: it takes the Skill
// Hub entry out of a scope, for a local or a remote hub.
func runConnectRemove(ctx context.Context, dir string, flags connectFlags, stdout, stderr io.Writer) int {
	result, disconnection, err := (app.ConnectService{}).Disconnect(ctx, app.DisconnectRequest{
		Project: dir, Global: flags.global, Hosts: flags.hosts, ClearCache: flags.cache, DryRun: flags.dryRun,
	})
	if err != nil || result.Error != nil {
		if code, handled := writeConnectSymlinkError(err, stdout, stderr, flags.json); handled {
			return code
		}
		return writeWorkspaceResult(result, err, stdout, stderr, flags.json)
	}
	if flags.json {
		return writeConnectJSON(stdout, stderr, struct {
			app.Result
			Removal *app.Disconnection `json:"removal"`
		}{Result: result, Removal: disconnection})
	}
	p := termui.New(stdout)
	renderConnectRemove(p, result, disconnection, flags.cache)
	return printerCode(p)
}

func renderConnectRemove(p *termui.Printer, result app.Result, removal *app.Disconnection, clearCache bool) {
	p.Line(result.Summary)
	if result.Status == app.StatusReady {
		for _, warning := range result.Warnings {
			p.Warning(warning.Summary)
		}
		return
	}
	writeChangeLines(p, result, hostChangeLines(result, true))
	remote := false
	if len(removal.Entries) > 0 {
		label := "The entries point at:"
		if result.Status == app.StatusApplied {
			label = "The removed entries pointed at:"
		}
		p.Line(label)
		var lines []string
		for _, entry := range removal.Entries {
			lines = append(lines, entry.Host+": "+entry.Target)
			remote = remote || strings.HasPrefix(entry.Target, "the remote hub")
		}
		p.Bullets(lines...)
	}
	var rows [][]string
	var empty []string
	for _, item := range removal.Cache {
		if item.Versions == 0 {
			empty = append(empty, "No skills fetched from "+item.Hub+" are cached on this machine.")
			continue
		}
		rows = append(rows, []string{item.Hub, fmt.Sprint(item.Skills), fmt.Sprint(item.Versions), formatBytes(item.Bytes)})
	}
	if len(rows) > 0 {
		p.Blank()
		label := "Cached skills that will be cleared:"
		if result.Status == app.StatusApplied {
			label = "Cleared cached skills:"
		}
		p.Line(label)
		p.Table([]string{"Hub", "Skills", "Versions", "Size"}, rows)
	}
	for _, line := range empty {
		p.Blank()
		p.Line(line)
	}
	p.Blank()
	notes := []string{"Unrelated host configuration stays as it is, and nothing changes on the hub."}
	if remote && !clearCache {
		notes = append(notes, "The skills fetched from the remote hub stay cached on this machine; add --cache to clear them as well.")
	}
	p.Bullets(notes...)
	for _, warning := range result.Warnings {
		p.Warning(warning.Summary)
	}
	switch result.Status {
	case app.StatusActionRequired:
		for _, action := range result.SuggestedActions {
			p.Blank()
			p.Line("To remove the connection, run:")
			p.Command(action.Command)
		}
	case app.StatusApplied:
		p.Blank()
		p.Next("restart your agent so it stops starting the Skill Hub server", "")
	}
}

// writeChangeLines prints one line per host; a dry run puts them under a
// "Would change:" heading.
func writeChangeLines(p *termui.Printer, result app.Result, lines []string) {
	if result.Status == app.StatusActionRequired && len(lines) > 0 {
		p.Line("Would change:")
	}
	p.Bullets(lines...)
}

func writeConnectJSON(stdout, stderr io.Writer, value any) int {
	if err := writeJSON(stdout, value); err != nil {
		termui.New(stderr).Error("Unable to write the command result.", "Output destination failed.", "Check the output destination and retry.")
		return 1
	}
	return 0
}
