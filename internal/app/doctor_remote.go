package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/hubtoken"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/version"
)

// HubTokenEnv names the environment variable that may carry the hub token on
// the client machine when an entry has no --token-file.
const HubTokenEnv = "SKILLHUB_HUB_TOKEN"

// HubStatusPath is the network listener's status endpoint. It needs the hub
// token and answers HubStatus: the build version and the remote protocol
// level, never a path or a secret.
const HubStatusPath = "/mcp/status"

// RemoteAPILevel is the remote protocol level of this build: the bundle
// endpoint, the status endpoint and the proxy's use of them. It changes only
// when a client of one level cannot work with a hub of another.
const RemoteAPILevel = 1

// HubStatus is the body of the status endpoint.
type HubStatus struct {
	Service   string `json:"service"`
	Version   string `json:"version"`
	RemoteAPI int    `json:"remote_api"`
}

// CurrentHubStatus describes this build.
func CurrentHubStatus() HubStatus {
	return HubStatus{Service: "skillhub", Version: version.Current().Version, RemoteAPI: RemoteAPILevel}
}

// Outcomes of a hub probe.
const (
	ProbeUnreachable  = "unreachable"
	ProbeHostRefused  = "host_refused"
	ProbeNotAHub      = "not_a_hub"
	ProbeHubError     = "hub_error"
	ProbeAccepted     = "accepted"
	ProbeRejected     = "token_rejected"
	ProbeNoToken      = "no_token"
	ProbeThrottled    = "throttled"
	ProbeRateLimited  = "rate_limited"
	hubProbeTimeout   = 3 * time.Second
	hubProbeBodyLimit = 4096
)

// HubProbe is what one status request found out about a hub. It never holds
// the token.
type HubProbe struct {
	Outcome string `json:"outcome"`
	// Reason is a short plain description of a failure, for example
	// "connection refused".
	Reason     string     `json:"reason,omitempty"`
	HTTPStatus int        `json:"http_status,omitempty"`
	Status     *HubStatus `json:"status,omitempty"`
	// Legacy reports a hub that accepted the token but has no status
	// endpoint: it predates version reporting.
	Legacy    bool  `json:"legacy,omitempty"`
	ElapsedMS int64 `json:"elapsed_ms"`
}

// Reachable reports whether a Skill Hub listener answered.
func (probe HubProbe) Reachable() bool {
	switch probe.Outcome {
	case ProbeUnreachable, ProbeNotAHub, ProbeHostRefused:
		return false
	}
	return true
}

// ProbeHub asks the hub's status endpoint, with the token when one is given.
// Redirects are not followed and proxy settings are ignored, so the token
// only ever goes to the hub's own address.
func ProbeHub(ctx context.Context, client *http.Client, hubURL, token string) HubProbe {
	if client == nil {
		client = hubProbeClient()
	}
	started := time.Now()
	probe := probeOnce(ctx, client, hubURL+HubStatusPath, token)
	if probe.Outcome == ProbeNotAHub && probe.HTTPStatus == http.StatusNotFound {
		probe = probeLegacyHub(ctx, client, hubURL, token, probe)
	}
	probe.ElapsedMS = time.Since(started).Milliseconds()
	return probe
}

// probeLegacyHub checks a hub that predates the status endpoint: it answers
// 404 there before checking the token. Its MCP endpoint must first prove it is
// a Skill Hub listener (401 with the skillhub realm without a token); only
// then does an answer other than 401 with the token mean the token passed.
func probeLegacyHub(ctx context.Context, client *http.Client, hubURL, token string, notFound HubProbe) HubProbe {
	anonymous := probeOnce(ctx, client, hubURL+"/mcp", "")
	switch anonymous.Outcome {
	case ProbeNoToken:
	case ProbeUnreachable, ProbeHubError, ProbeThrottled:
		return anonymous
	default:
		return notFound
	}
	if token == "" {
		return anonymous
	}
	authenticated := probeOnce(ctx, client, hubURL+"/mcp", token)
	if authenticated.Outcome == ProbeNotAHub {
		if authenticated.HTTPStatus == http.StatusNotFound {
			return notFound
		}
		return HubProbe{Outcome: ProbeAccepted, Legacy: true}
	}
	return authenticated
}

func hubProbeClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{
		Transport:     transport,
		Timeout:       hubProbeTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func probeOnce(ctx context.Context, client *http.Client, target, token string) HubProbe {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return HubProbe{Outcome: ProbeUnreachable, Reason: "the URL cannot be used"}
	}
	request.Header.Set("Accept", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return HubProbe{Outcome: ProbeUnreachable, Reason: networkReason(err)}
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, hubProbeBodyLimit))
	probe := HubProbe{HTTPStatus: response.StatusCode}
	skillHub := strings.Contains(response.Header.Get("WWW-Authenticate"), `realm="skillhub"`)
	switch {
	case response.StatusCode == http.StatusOK:
		var status HubStatus
		if json.Unmarshal(body, &status) != nil || status.Service != "skillhub" {
			probe.Outcome = ProbeNotAHub
			return probe
		}
		probe.Outcome, probe.Status = ProbeAccepted, &status
	case response.StatusCode == http.StatusUnauthorized && skillHub:
		probe.Outcome = ProbeRejected
		if token == "" {
			probe.Outcome = ProbeNoToken
		}
	case response.StatusCode == http.StatusForbidden && strings.Contains(string(body), "--allow-host"):
		probe.Outcome = ProbeHostRefused
	case response.StatusCode == http.StatusTooManyRequests:
		probe.Outcome = ProbeRateLimited
		if strings.Contains(string(body), "failed authentication") {
			probe.Outcome = ProbeThrottled
		}
	case response.StatusCode >= 500:
		// Only the hub's own known answers are named; any other body is never
		// shown, because a misbehaving proxy in between could echo request
		// headers, including the token.
		probe.Outcome, probe.Reason = ProbeHubError, http.StatusText(response.StatusCode)
		for _, known := range []string{"the hub is shutting down", "the hub has too many open sessions"} {
			if strings.Contains(string(body), known) {
				probe.Reason = known
			}
		}
	default:
		probe.Outcome = ProbeNotAHub
	}
	return probe
}

// networkReason turns a transport error into a few plain words.
func networkReason(err error) string {
	var dnsError *net.DNSError
	switch {
	case errors.Is(err, syscall.ECONNREFUSED), strings.Contains(err.Error(), "refused"):
		// Windows reports a refused connection with its own error number.
		return "connection refused"
	case errors.As(err, &dnsError):
		return "the host name " + dnsError.Name + " cannot be resolved"
	case os.IsTimeout(err), errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("no answer within %d seconds", int(hubProbeTimeout.Seconds()))
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, io.EOF):
		return "the connection was closed"
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return "the network is unreachable"
	}
	return "the connection failed"
}

// HubToken is the token an entry gives the proxy, read the way the proxy
// reads it. Value never leaves the process.
type HubToken struct {
	Value  string
	Source string
	// Problem is set when no usable token was found.
	Problem *RemoteCheck
}

// ReadHubToken reads the token from tokenFile, or from SKILLHUB_HUB_TOKEN
// when tokenFile is empty. A failure is described as a failed check.
func ReadHubToken(hubURL, tokenFile string) HubToken {
	if tokenFile == "" {
		value := strings.TrimSpace(os.Getenv(HubTokenEnv))
		if value == "" {
			return HubToken{Source: "environment", Problem: &RemoteCheck{
				ID: "token", Label: "Hub token", Status: CheckFailed,
				Detail: "The entry has no --token-file and " + HubTokenEnv + " is not set in this shell, so the proxy may have no token.",
				Fix:    "Run `skillhub connect --hub " + hubURL + " --token-file <path> --yes` with a copy of the hub's token file, or set " + HubTokenEnv + " in the environment the agent host starts in.",
			}}
		}
		return HubToken{Value: value, Source: "environment"}
	}
	value, err := hubtoken.Read(tokenFile)
	if err == nil {
		return HubToken{Value: value, Source: "file"}
	}
	check := &RemoteCheck{ID: "token", Label: "Hub token", Status: CheckFailed}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		check.Detail = "The token file " + tokenFile + " does not exist on this machine."
		check.Fix = tokenCopyFix(tokenFile)
	case errors.Is(err, hubtoken.ErrInsecureMode):
		check.Detail = "The token file " + tokenFile + " can be read by other users, so the proxy refuses it."
		check.Fix = "Run `chmod 600 " + tokenFile + "`."
	case errors.Is(err, hubtoken.ErrInvalidToken):
		check.Detail = "The token file " + tokenFile + " does not hold a hub token."
		check.Fix = tokenCopyFix(tokenFile)
	default:
		check.Detail = "The token file " + tokenFile + " cannot be read: " + plainError(err) + "."
		check.Fix = "Check that you can read " + tokenFile + ", or copy the hub's token file again."
	}
	return HubToken{Source: "file", Problem: check}
}

// tokenCopyFix tells how to bring the hub's token file to this machine.
func tokenCopyFix(tokenFile string) string {
	return "Copy the token file from machine B, where the hub prints its path when it starts and `skillhub token show` shows it, for example `scp <machine-b>:<workspace>/runtime/config/.hub-token " + tokenFile + "`, then run `chmod 600 " + tokenFile + "`."
}

func plainError(err error) string {
	var pathError *fs.PathError
	if errors.As(err, &pathError) {
		return pathError.Err.Error()
	}
	return err.Error()
}

// Check states.
const (
	CheckOK      = "ok"
	CheckFailed  = "failed"
	CheckSkipped = "skipped"
	CheckInfo    = "info"
)

// RemoteCheck is one doctor check of a remote hub connection. A failed check
// has a plain-words cause in Detail and one fix in Fix.
type RemoteCheck struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// RemoteHubReport holds the checks of one remote hub that entries in the
// current project or the user scope point at.
type RemoteHubReport struct {
	HubURL    string        `json:"hub"`
	TokenFile string        `json:"token_file,omitempty"`
	UsedBy    []string      `json:"used_by"`
	Checks    []RemoteCheck `json:"checks"`
}

// Healthy reports whether no check failed.
func (report RemoteHubReport) Healthy() bool {
	for _, check := range report.Checks {
		if check.Status == CheckFailed {
			return false
		}
	}
	return true
}

// RemoteEntry is one host entry that starts the proxy for a remote hub.
type RemoteEntry struct {
	Host      hostintegration.Host
	Global    bool
	Root      string
	HubURL    string
	TokenFile string
}

// ScopeLabel names the scope of an entry for people.
func (entry RemoteEntry) ScopeLabel() string {
	if entry.Global {
		return "your user account"
	}
	return "project " + entry.Root
}

// HostDisplayName is the product name of a host.
func HostDisplayName(host hostintegration.Host) string {
	switch host {
	case hostintegration.HostClaude:
		return "Claude Code"
	case hostintegration.HostCodex:
		return "Codex"
	case hostintegration.HostGemini:
		return "Gemini CLI"
	}
	return string(host)
}

// FindRemoteEntries lists the entries for remote hubs in the project
// directory and in the user scope. Unreadable files are skipped; a local
// entry is not a remote entry.
func FindRemoteEntries(project, home string) []RemoteEntry {
	var entries []RemoteEntry
	scopes := []struct {
		root   string
		global bool
	}{{project, false}, {home, true}}
	// In the home directory the project and user config of Codex and Gemini
	// CLI are the same file; it is reported once, as the user scope.
	seen := map[string]int{}
	for _, scope := range scopes {
		if scope.root == "" {
			continue
		}
		for _, adapter := range hostintegration.SupportedAdapters() {
			relative := adapter.ConfigRelativePath
			if scope.global {
				relative = adapter.UserConfigRelativePath
			}
			path := filepath.Clean(filepath.Join(scope.root, filepath.FromSlash(relative)))
			raw, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if position, duplicate := seen[path]; duplicate {
				entries[position].Global = true
				continue
			}
			registration := hostintegration.ReadRegistration(adapter.Host, raw)
			if !registration.Remote() {
				continue
			}
			seen[path] = len(entries)
			entries = append(entries, RemoteEntry{Host: adapter.Host, Global: scope.global, Root: scope.root, HubURL: registration.HubURL, TokenFile: registration.TokenFile})
		}
	}
	return entries
}

// RemoteDoctorService checks the connection of this machine to remote hubs.
type RemoteDoctorService struct {
	// Client sends the probes; nil uses a client with a 3 second timeout
	// that follows no redirect and ignores proxy settings.
	Client *http.Client
}

// Check runs the remote checks for every hub the entries point at, one
// report per hub and token file, in the order the entries name them.
func (service RemoteDoctorService) Check(ctx context.Context, entries []RemoteEntry) []RemoteHubReport {
	var reports []RemoteHubReport
	index := map[string]int{}
	// Hosts per scope, so one project lists its hosts once.
	type scopeHosts struct {
		scope string
		hosts []string
	}
	scopes := map[int][]scopeHosts{}
	for _, entry := range entries {
		key := entry.HubURL + "\x00" + entry.TokenFile
		position, seen := index[key]
		if !seen {
			position = len(reports)
			index[key] = position
			reports = append(reports, RemoteHubReport{HubURL: entry.HubURL, TokenFile: entry.TokenFile})
		}
		scope := entry.ScopeLabel()
		found := false
		for item := range scopes[position] {
			if scopes[position][item].scope == scope {
				scopes[position][item].hosts = append(scopes[position][item].hosts, HostDisplayName(entry.Host))
				found = true
			}
		}
		if !found {
			scopes[position] = append(scopes[position], scopeHosts{scope: scope, hosts: []string{HostDisplayName(entry.Host)}})
		}
	}
	for position := range reports {
		for _, item := range scopes[position] {
			reports[position].UsedBy = append(reports[position].UsedBy, strings.Join(item.hosts, ", ")+" ("+item.scope+")")
		}
	}
	for position := range reports {
		reports[position].Checks = service.checks(ctx, reports[position].HubURL, reports[position].TokenFile, tokenMaySend(reports[position].HubURL, entries))
	}
	return reports
}

// tokenMaySend reports whether doctor may send the hub token to hubURL: when
// the user scope names that hub, or when it is on this machine (loopback,
// for example an SSH tunnel). A hub named only by a project's files could be
// anyone's, for example in a cloned repository, so its token check needs the
// user's own `connect --hub` preview.
func tokenMaySend(hubURL string, entries []RemoteEntry) bool {
	if parsed, err := url.Parse(hubURL); err == nil && isLoopbackName(parsed.Hostname()) {
		return true
	}
	for _, entry := range entries {
		if entry.Global && entry.HubURL == hubURL {
			return true
		}
	}
	return false
}

func (service RemoteDoctorService) checks(ctx context.Context, hubURL, tokenFile string, maySend bool) []RemoteCheck {
	if _, problem := NormalizeConnectHubURL(hubURL); problem != nil {
		return []RemoteCheck{{
			ID: "hub_reachable", Label: "Hub reachable", Status: CheckFailed,
			Detail: "The entry's hub URL cannot be used: " + problem.Render.Why,
			Fix:    "Connect again with the URL the hub printed when it started: `skillhub connect --hub <url> --token-file <path> --yes`.",
		}}
	}
	token := ReadHubToken(hubURL, tokenFile)
	tokenCheck := RemoteCheck{ID: "token", Label: "Hub token", Status: CheckOK, Detail: "read from " + tokenFile}
	if token.Source == "environment" {
		tokenCheck.Detail = "taken from " + HubTokenEnv + " in this shell; the agent host must start the proxy with it set as well"
	}
	if token.Problem != nil {
		tokenCheck = *token.Problem
	}
	sent := token.Value
	if !maySend {
		sent = ""
	}
	probe := ProbeHub(ctx, service.Client, hubURL, sent)
	checks := []RemoteCheck{tokenCheck, reachableCheck(hubURL, probe)}
	accepted, version := acceptedCheck(probe, token), versionCheck(hubURL, probe)
	if !maySend && token.Problem == nil && probe.Outcome == ProbeNoToken {
		command := "skillhub connect --hub " + hubURL
		if tokenFile != "" {
			command += " --token-file " + shellArgument(tokenFile)
		}
		accepted.Status = CheckSkipped
		accepted.Detail = "not checked: only this project's files name this hub, so doctor does not send it the token; check it with `" + command + "`, a preview that writes nothing"
		version.Detail = "not checked: the token was not sent"
	}
	checks = append(checks, accepted, version)
	return append(checks, cacheChecks(hubURL)...)
}

func reachableCheck(hubURL string, probe HubProbe) RemoteCheck {
	check := RemoteCheck{ID: "hub_reachable", Label: "Hub reachable"}
	hostPort := hubHostPort(hubURL)
	switch probe.Outcome {
	case ProbeUnreachable:
		check.Status = CheckFailed
		check.Detail = "Nothing answers at " + hubURL + " (" + probe.Reason + "): the hub is stopped, or the SSH tunnel or the Tailscale connection is down."
		check.Fix = unreachableFix(hubURL, hostPort)
	case ProbeHostRefused:
		check.Status = CheckFailed
		host := hostPort
		if parsed, err := url.Parse(hubURL); err == nil {
			host = parsed.Hostname()
		}
		check.Detail = "The hub at " + hubURL + " refuses requests for the host name " + host + "."
		check.Fix = "On machine B restart the listener with `--allow-host " + host + "` added to `skillhub serve mcp --addr ...`."
	case ProbeNotAHub:
		check.Status = CheckFailed
		check.Detail = fmt.Sprintf("Something answers at %s, but it is not a Skill Hub listener (HTTP %d).", hubURL, probe.HTTPStatus)
		check.Fix = "Check the address: machine B prints the hub URL when `skillhub serve mcp --addr ...` starts; reconnect with `skillhub connect --hub <that URL> --yes`."
	case ProbeHubError:
		check.Status = CheckFailed
		check.Detail = fmt.Sprintf("The hub at %s answers with an error (HTTP %d: %s).", hubURL, probe.HTTPStatus, probe.Reason)
		check.Fix = "Look at the listener's output on machine B and restart `skillhub serve mcp --addr " + hostPort + "` there."
	default:
		check.Status = CheckOK
		check.Detail = fmt.Sprintf("answered in %d ms", probe.ElapsedMS)
	}
	return check
}

func unreachableFix(hubURL, hostPort string) string {
	_, port, _ := net.SplitHostPort(hostPort)
	if parsed, err := url.Parse(hubURL); err == nil && isLoopbackName(parsed.Hostname()) {
		return "Check that `skillhub serve mcp --addr 127.0.0.1:" + port + "` is running on machine B and that the SSH tunnel is open on this machine, for example `ssh -N -L " + port + ":127.0.0.1:" + port + " <machine-b>`; if you use Tailscale instead, check `tailscale status` on both machines."
	}
	return "Check that `skillhub serve mcp --addr " + hostPort + " --trust-private-network` is running on machine B and that both machines are connected to Tailscale, see `tailscale status`; if you use an SSH tunnel instead, check that it is open."
}

func acceptedCheck(probe HubProbe, token HubToken) RemoteCheck {
	check := RemoteCheck{ID: "token_accepted", Label: "Token accepted"}
	switch {
	case !probe.Reachable() || probe.Outcome == ProbeHubError:
		check.Status, check.Detail = CheckSkipped, "not checked: the hub did not answer"
	case token.Problem != nil:
		check.Status, check.Detail = CheckSkipped, "not checked: no usable token"
	case probe.Outcome == ProbeRejected || probe.Outcome == ProbeNoToken:
		check.Status = CheckFailed
		check.Detail = "The hub refused the token: it is not the hub's current token (the token was rotated, or the file comes from another hub)."
		check.Fix = "Copy the current token file from machine B again, where `skillhub token show` shows its path, then restart the agent."
	case probe.Outcome == ProbeThrottled:
		check.Status = CheckFailed
		check.Detail = "The hub is refusing requests for a minute after too many attempts with a wrong token."
		check.Fix = "Wait a minute, then copy the current token file from machine B again, where `skillhub token show` shows its path."
	default:
		check.Status, check.Detail = CheckOK, "the hub accepts the token"
	}
	return check
}

func versionCheck(hubURL string, probe HubProbe) RemoteCheck {
	check := RemoteCheck{ID: "hub_version", Label: "Hub version"}
	client := CurrentHubStatus()
	switch {
	case probe.Outcome == ProbeRateLimited:
		check.Status, check.Detail = CheckSkipped, "not checked: the hub is busy (too many requests); run doctor again in a moment"
	case probe.Outcome != ProbeAccepted:
		check.Status, check.Detail = CheckSkipped, "not checked: the hub must accept the token first"
	case probe.Legacy || probe.Status.RemoteAPI < client.RemoteAPI:
		hubVersion := "a version without version reporting"
		if probe.Status != nil {
			hubVersion = probe.Status.Version
		}
		check.Status = CheckFailed
		check.Detail = "The hub on machine B runs an older Skill Hub (" + hubVersion + ") than this machine (" + client.Version + "); some remote features will not work."
		check.Fix = "Update skillhub on machine B, then restart `skillhub serve mcp --addr " + hubHostPort(hubURL) + "` there."
	case probe.Status.RemoteAPI > client.RemoteAPI:
		check.Status = CheckFailed
		check.Detail = "This machine runs an older Skill Hub (" + client.Version + ") than the hub on machine B (" + probe.Status.Version + "); some remote features will not work."
		check.Fix = "Update skillhub on this machine with `skillhub update`, then restart the agent."
	default:
		check.Status = CheckOK
		check.Detail = probe.Status.Version + " (this machine: " + client.Version + ")"
	}
	return check
}

// cacheChecks report whether fetched skills can be stored, and when a skill
// was last fetched or used from the cache.
func cacheChecks(hubURL string) []RemoteCheck {
	writable := RemoteCheck{ID: "cache_writable", Label: "Skill cache"}
	last := RemoteCheck{ID: "last_fetch", Label: "Last fetch", Status: CheckInfo}
	dir, err := hostintegration.RemoteAccessDir(hubURL)
	if err != nil {
		writable.Status = CheckFailed
		writable.Detail = "This machine has no directory for cached skills: " + err.Error() + "."
		writable.Fix = "Set XDG_STATE_HOME to a writable directory in the environment the agent host starts in."
		last.Detail = "unknown"
		return []RemoteCheck{writable, last}
	}
	if problem := writableDirectory(dir); problem != "" {
		writable.Status = CheckFailed
		writable.Detail = "The skill cache " + dir + " is not writable (" + problem + "), so skills fetched from the hub cannot be stored."
		writable.Fix = "Make it writable for your user (for example `chmod u+w " + nearestExisting(dir) + "`), or set XDG_STATE_HOME to a writable directory in the environment the agent host starts in."
	} else {
		writable.Status, writable.Detail = CheckOK, "writable: "+dir
	}
	last.Detail = lastFetch(filepath.Dir(dir), filepath.Base(dir))
	return []RemoteCheck{writable, last}
}

// writableDirectory checks that dir, or the nearest directory above it that
// exists, accepts a new file. The probe file is removed at once.
func writableDirectory(dir string) string {
	existing := nearestExisting(dir)
	info, err := os.Stat(existing)
	if err != nil {
		return plainError(err)
	}
	if !info.IsDir() {
		return existing + " is not a directory"
	}
	file, err := os.CreateTemp(existing, ".skillhub-doctor-*")
	if err != nil {
		return plainError(err)
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
	return ""
}

func nearestExisting(dir string) string {
	for current := filepath.Clean(dir); ; current = filepath.Dir(current) {
		if _, err := os.Lstat(current); err == nil || filepath.Dir(current) == current {
			return current
		}
	}
}

// lastFetch describes the most recent fetch or use of a cached skill version.
func lastFetch(cacheRoot, hubID string) string {
	cache := skillruntime.RemoteCache{Root: cacheRoot}
	entries, err := os.ReadDir(cache.HubDir(hubID))
	if err != nil {
		return "never: no skill was fetched from this hub yet"
	}
	var latest time.Time
	skills := 0
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if cached, ok := cache.Latest(hubID, entry.Name()); ok {
			skills++
			if cached.LastUsed.After(latest) {
				latest = cached.LastUsed
			}
		}
	}
	if skills == 0 {
		return "never: no skill was fetched from this hub yet"
	}
	return fmt.Sprintf("%s (%d %s cached)", latest.Local().Format("2006-01-02 15:04"), skills, pluralWord(skills, "skill", "skills"))
}

func pluralWord(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
}

func hubHostPort(hubURL string) string {
	parsed, err := url.Parse(hubURL)
	if err != nil {
		return hubURL
	}
	if parsed.Port() != "" {
		return parsed.Host
	}
	port := "80"
	if parsed.Scheme == "https" {
		port = "443"
	}
	return net.JoinHostPort(parsed.Hostname(), port)
}

func isLoopbackName(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// RemoteDoctorResult turns reports into a Result for the JSON output and the
// summary line.
func RemoteDoctorResult(reports []RemoteHubReport) Result {
	result := NewResult(StatusReady, "The connection to the remote hub works.")
	if len(reports) > 1 {
		result.Summary = "The connections to the remote hubs work."
	}
	for _, report := range reports {
		for _, check := range report.Checks {
			if check.Status != CheckFailed {
				continue
			}
			result.Status = StatusActionRequired
			result.Items = append(result.Items, Item{ID: "remote_" + check.ID, Summary: report.HubURL + ": " + check.Detail, Impact: check.Fix})
		}
	}
	if result.Status == StatusActionRequired {
		result.Summary = "The connection to the remote hub needs attention."
		if len(reports) > 1 {
			result.Summary = "The connections to the remote hubs need attention."
		}
	}
	return result
}
