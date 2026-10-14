package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/delivery/hostpolicy"
	"github.com/vantt/skillhub-mcp/internal/hubtoken"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
	"github.com/vantt/skillhub-mcp/internal/userconfig"
)

// tokenHandOff tells how the hub's token reaches this machine. It is printed
// by `connect --hub --dry-run`; the token itself is never printed.
func tokenHandOff(hubURL string) string {
	return "To get the token to this machine, run the scp line that `skillhub serve` prints on machine B, for example `scp -p <user>@<machine-b>:<hub-folder>/runtime/config/.hub-token " + hubtoken.ClientPath(hubURL) + "`, which keeps the file private (mode 0600); or set " + app.HubTokenEnv + " in the environment the agent host starts the proxy with."
}

// runConnectHub handles `skillhub connect --hub <url>`: the host entry starts
// the stdio proxy for a hub on another machine.
func runConnectHub(ctx context.Context, dir string, flags connectFlags, stdout, stderr io.Writer) int {
	saved := ""
	if flags.inviteSet {
		var code int
		var done bool
		if saved, code, done = acceptInvite(ctx, &flags, stdout, stderr); done {
			return code
		}
	}
	result, connection, err := (app.ConnectService{}).ConnectRemote(ctx, app.RemoteConnectRequest{
		Project: dir, Global: flags.global, Hosts: flags.hosts,
		Hub: flags.hub, TokenFile: flags.tokenFile, DryRun: flags.dryRun,
	})
	if err != nil || result.Error != nil {
		if saved != "" {
			result, err = noteSavedToken(result, err, flags, saved)
		}
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
	if saved != "" {
		p.Line("Invite accepted. Hub token saved to " + saved)
		p.Blank()
	}
	renderRemoteConnect(p, result, connection)
	return printerCode(p)
}

// pairingLookup resolves the name of a hub for the address check before an
// invite is sent; tests replace it.
var pairingLookup = func(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

const pairingRefused = "with --strict-pairing an invite is only sent over Tailscale or to this machine (SSH tunnel); use the scp line instead"

// checkPairingHost refuses a hub address an invite must not be sent to: the
// host must be an address that is loopback or Tailscale, or a name that
// resolves only to such addresses. A name that does not resolve is refused
// too, so the code never goes where it was not checked.
func checkPairingHost(ctx context.Context, hubURL string) error {
	parsed, err := url.Parse(hubURL)
	if err != nil {
		return errors.New(pairingRefused)
	}
	host := parsed.Hostname()
	if address, err := netip.ParseAddr(host); err == nil {
		if !hostpolicy.PairingIP(address) {
			return errors.New(pairingRefused)
		}
		return nil
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	addresses, err := pairingLookup(lookupCtx, host)
	if err != nil || len(addresses) == 0 {
		return errors.New(pairingRefused)
	}
	for _, address := range addresses {
		if !hostpolicy.PairingIP(address) {
			return errors.New(pairingRefused)
		}
	}
	return nil
}

// acceptInvite trades the invite for the hub's token and saves it, after
// every check that does not need the hub has passed. On success it points
// flags.tokenFile at the saved file and returns its path. done reports that
// the command is over, with the exit code.
func acceptInvite(ctx context.Context, flags *connectFlags, stdout, stderr io.Writer) (saved string, code int, done bool) {
	fail := func(what, why, fix string) (string, int, bool) {
		result := app.ErrorResult(&app.Error{Code: app.ErrorInvalidRequest, Render: app.ErrorRender{Error: what, Why: why, Fix: fix}})
		return "", writeWorkspaceResult(result, nil, stdout, stderr, flags.json), true
	}
	hubURL, problem := app.NormalizeConnectHubURL(flags.hub)
	if problem != nil {
		return "", writeWorkspaceResult(app.ErrorResult(problem), nil, stdout, stderr, flags.json), true
	}
	if flags.strictPairing {
		if err := checkPairingHost(ctx, hubURL); err != nil {
			return fail("The invite was not sent.", err.Error()+".", "Use the hub's Tailscale address or an SSH tunnel to it, or copy the token file with the scp line `skillhub serve` prints.")
		}
	}
	destination := flags.tokenFile
	if destination == "" {
		configDir, err := userconfig.Dir()
		if err != nil {
			return fail("The invite was not sent.", "This machine has no folder for the hub token: "+err.Error()+".", "Pass --token-file <path> for the file to write the token to.")
		}
		destination = hubtoken.ClientStorePath(configDir, hubURL)
	}
	destination, err := filepath.Abs(destination)
	if err != nil {
		return fail("The invite was not sent.", "The token file path cannot be resolved: "+err.Error()+".", "Pass an absolute --token-file path.")
	}
	if flags.tokenFile != "" {
		if _, err := os.Lstat(destination); err == nil {
			return fail("The invite was not sent.", destination+" already exists, and saving the token there would overwrite it. The invite is still unused.", "Pass a --token-file path that does not exist yet, or leave --token-file out to use the default place.")
		}
	}
	if err := hubtoken.CheckWritable(destination); err != nil {
		return fail("The invite was not sent.", "The token cannot be saved at "+destination+": "+err.Error()+". The invite is still unused.", "Pass --token-file <path> in a folder you can write, or fix the folder, then run this command again.")
	}
	token, err := app.PairHub(ctx, hubURL, flags.invite)
	if err != nil {
		var failure *app.Error
		if errors.As(err, &failure) {
			return "", writeWorkspaceResult(app.ErrorResult(failure), nil, stdout, stderr, flags.json), true
		}
		return "", writeWorkspaceResult(app.Result{}, err, stdout, stderr, flags.json), true
	}
	if err := hubtoken.Save(destination, token); err != nil {
		return fail("The invite was used, but the token could not be saved.", err.Error()+".", "On the hub machine run `skillhub token invite` for a new invite, fix the problem with "+destination+", then run this command again.")
	}
	flags.tokenFile = destination
	return destination, 0, false
}

// noteSavedToken adds to a failed connect the fact that the invite is used
// and the token is on disk, with the command that connects without it.
func noteSavedToken(result app.Result, err error, flags connectFlags, saved string) (app.Result, error) {
	command := "skillhub connect --hub " + flags.hub + " --token-file " + shellquote.Arg(saved)
	if flags.global {
		command += " --global"
	}
	for _, host := range flags.hosts {
		command += " --host " + string(host)
	}
	note := "The invite was used and the hub token is saved at " + saved + "; run `" + command + "` once the problem is fixed."
	if result.Error != nil {
		result.Error.Render.Fix = strings.TrimSpace(note + " " + result.Error.Render.Fix)
		return result, err
	}
	return result, fmt.Errorf("%s: %w", note, err)
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
		host, kind, relPath, ok := parseHostItemID(item.ID)
		if !ok || !hostItemWritten(item) {
			continue
		}
		if _, seen := files[host]; !seen {
			order = append(order, host)
		}
		description := formatChangeKind(kind, item.Summary, relPath)
		if deleted := item.Impact == "Would be deleted." || item.Impact == "Deleted."; deleted && removal {
			description += " deleted"
		} else if deleted && result.Status == app.StatusApplied {
			description = "removed " + description
		} else if deleted {
			description = "remove " + description
		}
		if !slices.Contains(files[host], description) {
			files[host] = append(files[host], description)
		}
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
