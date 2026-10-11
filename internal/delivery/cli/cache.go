package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/delivery/mcpserver"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

type cacheClearConfig struct {
	hub    bool
	hubURL string
	yes    bool
}

func parseCacheClearFlags(args []string) (cacheClearConfig, string) {
	var cfg cacheClearConfig
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--hub":
			cfg.hub = true
			if index+1 < len(args) && !strings.HasPrefix(args[index+1], "-") {
				cfg.hubURL = args[index+1]
				index++
			}
		case "--yes":
			cfg.yes = true
		default:
			return cfg, fmt.Sprintf("unknown argument %q", args[index])
		}
	}
	if !cfg.hub {
		return cfg, "cache clear needs --hub to say which cache to clear"
	}
	return cfg, ""
}

// runCache handles `skillhub cache clear --hub [<url>] [--yes]`: it removes
// the copies of skills this machine fetched from remote hubs. It previews by
// default and applies with --yes. It never contacts a hub.
func runCache(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "clear" {
		return writeInvalidRequest(stdout, stderr, false, "cache requires the clear subcommand", "Run `skillhub cache clear --hub`.")
	}
	cfg, reason := parseCacheClearFlags(args[1:])
	if reason != "" {
		return writeInvalidRequest(stdout, stderr, false, reason, "Run `skillhub cache clear --hub` to preview, then add --yes.")
	}
	hubID := ""
	if cfg.hubURL != "" {
		normalized, err := mcpserver.NormalizeHubURL(cfg.hubURL)
		if err == nil {
			hubID, err = skillruntime.HubID(normalized)
		}
		if err != nil {
			termui.New(stderr).Error("The hub URL cannot be used.", err.Error(), "Pass the URL the agent host connects to, for example --hub http://127.0.0.1:7420, or drop it to clear every hub's cache.")
			return 2
		}
		cfg.hubURL = normalized
	}
	stateDir, err := skillruntime.ClientStateDir()
	if err != nil {
		termui.New(stderr).Error("This machine has no directory for cached skills.", err.Error(), "Set XDG_STATE_HOME to the directory the proxy uses.")
		return 1
	}
	cache := skillruntime.RemoteCache{Root: skillruntime.RemoteCacheDir(stateDir)}
	usage, err := cache.Usage(hubID)
	if err != nil {
		termui.New(stderr).Error("The skill cache could not be read.", err.Error(), "Check the permissions of "+cache.Root+".")
		return 1
	}
	p := termui.New(stdout)
	if len(usage) == 0 {
		if cfg.yes {
			// Nothing is cached, but a crash may have left staging files.
			_, _ = cache.Clear(hubID)
		}
		p.Line(cacheScope(cfg.hubURL, "No skills fetched from a remote hub are cached on this machine.", "No skills fetched from "+cfg.hubURL+" are cached on this machine."))
		p.Fields(termui.Field{Label: "Cache directory", Value: cache.Root})
		return printerCode(p)
	}
	if !cfg.yes {
		p.Line(cacheScope(cfg.hubURL, "This removes the copies of skills fetched from remote hubs.", "This removes the copies of skills fetched from "+cfg.hubURL+"."))
		p.Fields(termui.Field{Label: "Cache directory", Value: cache.Root})
		p.Blank()
		writeCacheUsage(p, usage)
		p.Blank()
		p.Bullets(
			"An agent fetches each skill again from its hub the next time it uses it.",
			"The skills' working directories (installed dependencies) and stored secrets are kept.",
			"Nothing on the hub changes.",
		)
		p.Blank()
		p.Next("Apply the cleanup:", cacheClearCommand(cfg))
		return printerCode(p)
	}
	cleared, err := cache.Clear(hubID)
	if err != nil {
		termui.New(stderr).Error("The skill cache could not be cleared completely.", err.Error(), "Close agent sessions that use these skills, then run the command again.")
		return 1
	}
	versions, bytes := 0, int64(0)
	for _, item := range cleared {
		versions += item.Versions
		bytes += item.Bytes
	}
	p.Line(fmt.Sprintf("Removed %d cached skill %s (%s).", versions, plural(versions, "version", "versions"), formatBytes(bytes)))
	p.Fields(termui.Field{Label: "Cache directory", Value: cache.Root})
	return printerCode(p)
}

func writeCacheUsage(p *termui.Printer, usage []skillruntime.HubUsage) {
	rows := make([][]string, 0, len(usage))
	for _, item := range usage {
		hub := item.HubURL
		if hub == "" {
			hub = item.HubID
		}
		rows = append(rows, []string{hub, fmt.Sprint(item.Skills), fmt.Sprint(item.Versions), formatBytes(item.Bytes)})
	}
	p.Table([]string{"Hub", "Skills", "Versions", "Size"}, rows)
}

func cacheScope(hubURL, all, one string) string {
	if hubURL == "" {
		return all
	}
	return one
}

func cacheClearCommand(cfg cacheClearConfig) string {
	command := "skillhub cache clear --hub"
	if cfg.hubURL != "" {
		command += " " + shellQuote(cfg.hubURL)
	}
	return command + " --yes"
}

func printerCode(p *termui.Printer) int {
	if p.Err() != nil {
		return 1
	}
	return 0
}

func plural(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
}

// formatBytes renders a size with one decimal in KB or MB (powers of 1024).
func formatBytes(size int64) string {
	switch {
	case size >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
	case size >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(size)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", size)
}
