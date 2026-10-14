package cli

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/skillhub-mcp/internal/delivery/hostpolicy"
	"github.com/vantt/skillhub-mcp/internal/hubtoken"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
)

// stdioToolNames starts the given skillhub command as a stdio MCP server in a
// subprocess, lists its tools, closes stdin, and checks a clean exit.
func stdioToolNames(t *testing.T, args ...string) []string {
	t.Helper()
	command := exec.Command(os.Args[0], args...)
	command.Env = append(os.Environ(), cliHelperEnv+"=1")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-regression", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("skillhub %s: connect: %v; stderr=%s", strings.Join(args, " "), err, stderr.String())
	}
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("skillhub %s: tools/list: %v", strings.Join(args, " "), err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("skillhub %s: close: %v; stderr=%s", strings.Join(args, " "), err, stderr.String())
	}
	if command.ProcessState == nil || command.ProcessState.ExitCode() != 0 {
		t.Fatalf("skillhub %s: exit state %v; stderr=%s", strings.Join(args, " "), command.ProcessState, stderr.String())
	}
	var names []string
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

func TestMCPServeIsTheStdioServer(t *testing.T) {
	ws := initTestWorkspace(t)
	// Host registrations written by `skillhub connect` run this exact command.
	registered := stdioToolNames(t, "mcp", "serve", "--profile", "runtime", "--workspace", ws)
	if want := []string{"skill_feedback", "skill_get", "skill_resolve"}; !slices.Equal(registered, want) {
		t.Fatalf("mcp serve --profile runtime tools = %v, want %v", registered, want)
	}
	all := stdioToolNames(t, "mcp", "serve", "--workspace", ws)
	if len(all) <= len(registered) {
		t.Fatalf("default profile tools = %v, want more than the runtime tools", all)
	}
	if entries, err := os.ReadDir(filepath.Join(ws, "runtime", "config")); err == nil {
		for _, entry := range entries {
			if entry.Name() == hubtoken.FileName {
				t.Fatal("the stdio server created a hub token")
			}
		}
	}
}

func TestMCPServeFlagErrors(t *testing.T) {
	ws := initTestWorkspace(t)
	cases := []struct {
		flags []string
		want  string
	}{
		{[]string{"--workspace", ws, "--profile"}, "WHY: --profile requires a value\n"},
		{[]string{"--workspace", ws, "--profile", "bogus"}, "WHY: invalid profile \"bogus\"\n"},
		{[]string{"--workspace", ws, "--bogus"}, "WHY: unknown MCP argument \"--bogus\"\n"},
		{[]string{"--workspace"}, "WHY: --workspace requires a path\n"},
		{[]string{"--addr", "127.0.0.1:0", "--workspace", ws}, "WHY: unknown MCP argument \"--addr\"\nFIX: `skillhub mcp serve` is the stdio server agents start; run `skillhub serve` for agents on other\n     machines.\n"},
		{[]string{"--trust-private-network", "--workspace", ws}, "WHY: unknown MCP argument \"--trust-private-network\"\n"},
	}
	for _, tc := range cases {
		code, stdout, stderr := runCLI(t, append([]string{"mcp", "serve"}, tc.flags...)...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, tc.want) {
			t.Errorf("mcp serve %v: exit %d, stdout %q, stderr:\n%s\nwant:\n%s", tc.flags, code, stdout, stderr, tc.want)
		}
	}
}

// TestServeFlagErrors covers the refusals of `skillhub serve` that happen
// before anything is bound: each exits 2 and prints nothing on stdout.
func TestServeFlagErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no profile on the network", []string{"serve", "--profile", "runtime"}, "WHY: serve has no --profile"},
		{"no profile on serve mcp", []string{"serve", "mcp", "--profile", "all"}, "WHY: serve has no --profile"},
		{"unknown mode", []string{"serve", "stdio"}, "WHY: serve has no mode \"stdio\""},
		{"web flag on mcp", []string{"serve", "--no-open"}, "WHY: --no-open only applies to `skillhub serve web`"},
		{"dev flag on mcp", []string{"serve", "mcp", "--dev"}, "WHY: --dev only applies to `skillhub serve web`"},
		{"token file on web", []string{"serve", "web", "--token-file", "x"}, "WHY: --token-file only applies to `skillhub serve` (mcp)"},
		{"trust flag is gone", []string{"serve", "--trust-private-network"}, "WHY: unknown argument \"--trust-private-network\""},
		{"loopback-only flag is gone", []string{"serve", "web", "--loopback-only"}, "WHY: unknown argument \"--loopback-only\""},
		{"bogus flag", []string{"serve", "web", "--bogus"}, "WHY: unknown argument \"--bogus\""},
		{"addr without value", []string{"serve", "--addr"}, "WHY: --addr requires a value"},
		{"addr twice", []string{"serve", "--addr", "127.0.0.1:1", "--addr", "127.0.0.1:2"}, "WHY: --addr is given twice"},
		{"host name", []string{"serve", "--addr", "hub.example:7420"}, "give an IP address"},
		{"bad port", []string{"serve", "web", "--addr", "127.0.0.1:99999"}, "the port must be a number from 0 to 65535"},
		{"four addresses", []string{"serve", "--addr", "127.0.0.1:1,127.0.0.1:2,127.0.0.1:3,127.0.0.1:4"}, "WHY: --addr takes at most 3 addresses, got 4"},
		{"duplicate", []string{"serve", "--addr", "127.0.0.1,127.0.0.1:7420"}, "WHY: --addr lists 127.0.0.1:7420 twice"},
		{"wildcard overlap", []string{"serve", "--addr", "0.0.0.0:7420,[::]:7420"}, "WHY: --addr 0.0.0.0:7420 and [::]:7420 overlap"},
		{"dev off loopback", []string{"serve", "web", "--dev", "--addr", "0.0.0.0:7421"}, "WHY: --dev only listens on loopback"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, tc.args...)
			if code != 2 || stdout != "" || !strings.Contains(stderr, tc.want) {
				t.Fatalf("exit %d, stdout %q, stderr:\n%s\nwant exit 2 and %q", code, stdout, stderr, tc.want)
			}
		})
	}
}

// bannerInterfaces is a machine with a LAN card, Tailscale, a down Wi-Fi,
// and the bridges Docker and libvirt create.
var bannerInterfaces = []hostpolicy.Interface{
	{Name: "lo", Up: true, Loopback: true, IPs: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}},
	{Name: "eth0", Up: true, IPs: []net.IP{net.ParseIP("192.168.1.20"), net.ParseIP("fe80::2")}},
	{Name: "tailscale0", Up: true, IPs: []net.IP{net.ParseIP("100.101.102.103")}},
	{Name: "wlan0", Up: false, IPs: []net.IP{net.ParseIP("10.0.0.5")}},
	{Name: "docker0", Up: true, IPs: []net.IP{net.ParseIP("172.17.0.1")}},
	{Name: "br-5f0c", Up: true, IPs: []net.IP{net.ParseIP("172.18.0.1")}},
	{Name: "virbr0", Up: true, IPs: []net.IP{net.ParseIP("192.168.122.1")}},
}

// checkBannerCommands runs every printed skillhub command through the help
// check, as docs are checked.
func checkBannerCommands(t *testing.T, banner string) {
	t.Helper()
	for number, line := range strings.Split(banner, "\n") {
		for _, command := range commandSegments(strings.TrimSpace(line)) {
			checkGuideCommand(t, number+1, command)
		}
	}
}

func TestMCPBannerGolden(t *testing.T) {
	// A listener asked for port 0 reports the port the system chose; the
	// banner must print that port, never 7420.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()
	portText := strconv.Itoa(port)
	home := filepath.Join(t.TempDir(), "home")
	workspace := filepath.Join(home, "skill-hub")
	tokenFile := hubtoken.DefaultPath(workspace)

	var out bytes.Buffer
	writeMCPBanner(&out, mcpBanner{
		addresses:  []net.Addr{&net.TCPAddr{IP: net.IPv4zero, Port: port}},
		workspace:  workspace,
		tokenFile:  tokenFile,
		home:       home,
		user:       "ann",
		interfaces: bannerInterfaces,
	})
	want := "Skill Hub is serving skills to agents on other machines.\n" +
		"It serves the runtime tools only (skill_resolve, skill_get, skill_feedback); curation stays on this\n" +
		"machine.\n" +
		"\n" +
		"Hub folder:  " + workspace + "\n" +
		"Token file:  " + tokenFile + "\n" +
		"WARNING: Listening on all network interfaces over plain HTTP; use --addr 127.0.0.1:7420 to keep it on this machine.\n" +
		"\n" +
		"eth0 (192.168.1.20), on the other machine:\n" +
		"  $ scp -p ann@192.168.1.20:skill-hub/runtime/config/.hub-token ~/.skillhub-token-192.168.1.20-" + portText + "\n" +
		"  $ skillhub connect --hub http://192.168.1.20:" + portText + " --token-file ~/.skillhub-token-192.168.1.20-" + portText + "\n" +
		"\n" +
		"Tailscale (100.101.102.103), on the other machine:\n" +
		"  $ scp -p ann@100.101.102.103:skill-hub/runtime/config/.hub-token ~/.skillhub-token-100.101.102.103-" + portText + "\n" +
		"  $ skillhub connect --hub http://100.101.102.103:" + portText + " --token-file ~/.skillhub-token-100.101.102.103-" + portText + "\n" +
		"\n" +
		"To connect a machine without copying the token file, run `skillhub token invite`.\n" +
		"The token itself is never printed. Stop the server with Ctrl-C.\n"
	if out.String() != want {
		t.Fatalf("wildcard banner:\n%s\nwant:\n%s", out.String(), want)
	}
	for _, hidden := range []string{"docker0", "172.17.", "br-", "172.18.", "virbr0", "wlan0", "fe80", ":7420 --token-file", "-7420\n"} {
		if strings.Contains(out.String(), hidden) {
			t.Errorf("the banner prints %q", hidden)
		}
	}
	checkBannerCommands(t, out.String())

	out.Reset()
	writeMCPBanner(&out, mcpBanner{
		addresses:  []net.Addr{&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}},
		workspace:  workspace,
		tokenFile:  tokenFile,
		home:       home,
		user:       "ann",
		interfaces: bannerInterfaces,
	})
	want = "Skill Hub is serving skills to agents on other machines.\n" +
		"It serves the runtime tools only (skill_resolve, skill_get, skill_feedback); curation stays on this\n" +
		"machine.\n" +
		"\n" +
		"Hub folder:  " + workspace + "\n" +
		"Token file:  " + tokenFile + "\n" +
		"\n" +
		"This machine only (127.0.0.1:" + portText + "); on the other machine, open an SSH tunnel and keep it running:\n" +
		"  $ ssh -N -L " + portText + ":127.0.0.1:" + portText + " ann@<this-machine>\n" +
		"Then, in another terminal there:\n" +
		"  $ scp -p ann@<this-machine>:skill-hub/runtime/config/.hub-token ~/.skillhub-token-127.0.0.1-" + portText + "\n" +
		"  $ skillhub connect --hub http://127.0.0.1:" + portText + " --token-file ~/.skillhub-token-127.0.0.1-" + portText + "\n" +
		"\n" +
		"To connect a machine without copying the token file, run `skillhub token invite`.\n" +
		"The token itself is never printed. Stop the server with Ctrl-C.\n"
	if out.String() != want {
		t.Fatalf("loopback banner:\n%s\nwant:\n%s", out.String(), want)
	}
	checkBannerCommands(t, out.String())

	// A hub outside the home folder, or with a space in its path, gets no
	// scp line: legacy scp and SFTP quote remote paths differently.
	for _, folder := range []string{filepath.Join(t.TempDir(), "hub"), filepath.Join(home, "my hub")} {
		out.Reset()
		writeMCPBanner(&out, mcpBanner{
			addresses:  []net.Addr{&net.TCPAddr{IP: net.ParseIP("100.101.102.103"), Port: 7420}},
			workspace:  folder,
			tokenFile:  hubtoken.DefaultPath(folder),
			home:       home,
			interfaces: bannerInterfaces,
		})
		text := out.String()
		if strings.Contains(text, "scp -p") || !strings.Contains(text, "Copy the token file to ~/.skillhub-token-100.101.102.103-7420 there by another means") {
			t.Errorf("hub at %s: banner without the copy advice:\n%s", folder, text)
		}
		if !strings.Contains(text, "WARNING: Listening on a network address over plain HTTP; use --addr 127.0.0.1:7420 to keep it on this machine.\n") {
			t.Errorf("hub at %s: banner without the warning line:\n%s", folder, text)
		}
		checkBannerCommands(t, text)
	}

	// A wildcard bind on a machine with no usable network address still
	// tells how to reach it through a tunnel.
	out.Reset()
	writeMCPBanner(&out, mcpBanner{
		addresses: []net.Addr{&net.TCPAddr{IP: net.IPv6unspecified, Port: 7420}},
		workspace: workspace, tokenFile: tokenFile, home: home,
		interfaces: []hostpolicy.Interface{bannerInterfaces[0]},
	})
	if !strings.Contains(out.String(), "  $ ssh -N -L 7420:127.0.0.1:7420 <user>@<this-machine>\n") {
		t.Errorf("wildcard bind without addresses:\n%s", out.String())
	}
}

func TestScpTokenPath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	cases := []struct {
		tokenFile string
		want      string
		ok        bool
	}{
		{filepath.Join(home, "skill-hub", "runtime", "config", ".hub-token"), "skill-hub/runtime/config/.hub-token", true},
		{filepath.Join(home, "my hub", ".hub-token"), "", false},
		{filepath.Join(home, "it's", ".hub-token"), "", false},
		{filepath.Join(home, "-hub", ".hub-token"), "", false},
		{filepath.Join(filepath.Dir(home), "elsewhere", ".hub-token"), "", false},
	}
	for _, tc := range cases {
		got, ok := scpTokenPath(tc.tokenFile, home)
		if got != tc.want || ok != tc.ok {
			t.Errorf("scpTokenPath(%q) = %q, %v; want %q, %v", tc.tokenFile, got, ok, tc.want, tc.ok)
		}
	}
	if _, ok := scpTokenPath(filepath.Join(home, "x"), ""); ok {
		t.Error("an unknown home folder gave an scp path")
	}
}

// serveOutput starts `skillhub <args>` in process and returns its output
// buffers once stdout holds want times the connect marker, and a function
// that stops it and returns the exit code.
func serveOutput(t *testing.T, args []string, connectLines int) (*syncBuffer, *syncBuffer, func() int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var stdout, stderr syncBuffer
	exitCh := make(chan int, 1)
	go func() { exitCh <- RunContext(ctx, args, &stdout, &stderr) }()
	code, stopped := 0, false
	stop := func() int {
		if !stopped {
			stopped = true
			cancel()
			select {
			case code = <-exitCh:
			case <-time.After(15 * time.Second):
				t.Error("serve did not stop")
				code = -1
			}
		}
		return code
	}
	t.Cleanup(func() { stop() })
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Count(stdout.String(), "skillhub connect --hub ") >= connectLines && strings.Contains(stdout.String(), "Ctrl-C") {
			return &stdout, &stderr, stop
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no banner; stdout: %s; stderr: %s", stdout.String(), stderr.String())
	return nil, nil, stop
}

// bannerHubURLs returns the hub URLs of the connect lines.
func bannerHubURLs(stdout string) []string {
	var hubs []string
	for _, line := range strings.Split(stdout, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "$ skillhub connect --hub "); found {
			hubs = append(hubs, strings.Fields(rest)[0])
		}
	}
	return hubs
}

func postInitializeTo(t *testing.T, hub, authorization string) int {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, hub+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"cli-test","version":"1"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	return response.StatusCode
}

func TestServeMCPListenerLifecycle(t *testing.T) {
	ws := initTestWorkspace(t)
	stdout, stderr, stop := serveOutput(t, []string{"serve", "mcp", "--addr", "127.0.0.1:0", "--workspace", ws}, 1)
	hubs := bannerHubURLs(stdout.String())
	if len(hubs) != 1 || strings.HasSuffix(hubs[0], ":0") {
		t.Fatalf("hub URLs = %v; stdout:\n%s", hubs, stdout.String())
	}
	hub := hubs[0]
	if !strings.Contains(stdout.String(), "--token-file "+hubtoken.ClientPath(hub)+"\n") {
		t.Fatalf("connect line lacks the client token path:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Token file:  "+hubtoken.DefaultPath(ws)) {
		t.Fatalf("banner lacks the hub token file:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "WARNING") {
		t.Fatalf("a loopback listener printed a warning:\n%s", stdout.String())
	}
	checkBannerCommands(t, stdout.String())
	token, err := hubtoken.Read(hubtoken.DefaultPath(ws))
	if err != nil {
		t.Fatal(err)
	}
	if status := postInitializeTo(t, hub, ""); status != http.StatusUnauthorized {
		t.Errorf("without token status = %d, want 401", status)
	}
	if status := postInitializeTo(t, hub, "Bearer "+token); status != http.StatusOK {
		t.Errorf("with token status = %d, want 200", status)
	}
	if code := stop(); code != 0 {
		t.Fatalf("exit code = %d; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), token) {
		t.Fatal("the token appeared in the listener output")
	}
}

// TestServeListensOnSeveralAddresses runs the bare `skillhub serve` with two
// loopback listeners: two ports, one token that works on both.
func TestServeListensOnSeveralAddresses(t *testing.T) {
	ws := initTestWorkspace(t)
	stdout, stderr, stop := serveOutput(t, []string{"serve", "--addr", "127.0.0.1:0,127.0.0.1:0", "--workspace", ws}, 2)
	hubs := bannerHubURLs(stdout.String())
	if len(hubs) != 2 || hubs[0] == hubs[1] {
		t.Fatalf("hub URLs = %v, want two distinct; stdout:\n%s", hubs, stdout.String())
	}
	token, err := hubtoken.Read(hubtoken.DefaultPath(ws))
	if err != nil {
		t.Fatal(err)
	}
	for _, hub := range hubs {
		if status := postInitializeTo(t, hub, "Bearer "+token); status != http.StatusOK {
			t.Errorf("%s with token status = %d, want 200", hub, status)
		}
	}
	if code := stop(); code != 0 {
		t.Fatalf("exit code = %d; stderr: %s", code, stderr.String())
	}
}

// TestServeBindsAllOrNothing occupies one port: serve names that address,
// exits 1 and leaves no listener behind.
func TestServeBindsAllOrNothing(t *testing.T) {
	ws := initTestWorkspace(t)
	taken, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	busy := taken.Addr().String()
	for _, mode := range []string{"mcp", "web"} {
		args := []string{"serve", mode, "--addr", "127.0.0.1:0," + busy, "--workspace", ws}
		if mode == "web" {
			args = append(args, "--no-open")
		}
		code, stdout, stderr := runCLI(t, args...)
		if code != 1 || stdout != "" || !strings.Contains(stderr, "ERROR: Could not listen on "+busy+": ") || !strings.Contains(stderr, "Nothing was started") {
			t.Fatalf("serve %s with a busy port: exit %d, stdout %q, stderr:\n%s", mode, code, stdout, stderr)
		}
	}
}

func TestTokenCommands(t *testing.T) {
	ws := initTestWorkspace(t)
	tokenFile := hubtoken.DefaultPath(ws)

	code, stdout, stderr := runCLI(t, "token", "show", "--workspace", ws)
	want := "ERROR: The hub token is printed only on a terminal.\n" +
		"WHY: Standard output is redirected or captured, and the token must not end up in a file, a log, or\n" +
		"     an agent transcript.\n" +
		"FIX: Copy the token file below with the scp line `skillhub serve` prints, or run\n" +
		"     `skillhub token show` directly in a terminal.\n" +
		"Token file:  " + tokenFile + "\n"
	if code != 1 || stdout != "" || stderr != want {
		t.Fatalf("token show off a terminal: exit %d, stdout %q, stderr:\n%s\nwant:\n%s", code, stdout, stderr, want)
	}
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
		t.Fatalf("a refused token show touched the token file: %v", err)
	}

	original := stdoutIsTerminal
	stdoutIsTerminal = func(io.Writer) bool { return true }
	code, stdout, stderr = runCLI(t, "token", "show", "--workspace", ws)
	stdoutIsTerminal = original
	token, err := hubtoken.Read(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Token:       "+token+"\n") || !strings.Contains(stdout, "a new one was created") {
		t.Fatalf("token show on a terminal: exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}

	code, stdout, stderr = runCLI(t, "token", "rotate", "--workspace", ws, "--dry-run")
	want = "Dry run: the hub token was not replaced.\n" +
		"Token file:  " + tokenFile + "\n" +
		"Would change:\n" +
		"  - The token is replaced with a new random one.\n" +
		"  - Every machine that connects to this hub stops working until it gets the new token file.\n" +
		"  - A running listener uses the new token from its next request; it does not need a restart.\n" +
		"\n" +
		"To replace the token, run in a terminal:\n" +
		"  $ skillhub token rotate --workspace " + shellquote.Arg(ws) + "\n"
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("token rotate --dry-run: exit %d, stderr %q, stdout:\n%s\nwant:\n%s", code, stderr, stdout, want)
	}
	if unchanged, _ := hubtoken.Read(tokenFile); unchanged != token {
		t.Fatal("token rotate --dry-run changed the token")
	}

	// Off a terminal, rotate refuses and names the command to run in one.
	code, stdout, stderr = runCLI(t, "token", "rotate", "--workspace", ws)
	if code != 1 || stdout != "" || !strings.Contains(stderr, "rotated only on a terminal") || !strings.Contains(stderr, "  $ skillhub token rotate --workspace "+shellquote.Arg(ws)+"\n") {
		t.Fatalf("token rotate off a terminal: exit %d, stdout %q, stderr:\n%s", code, stdout, stderr)
	}
	if unchanged, _ := hubtoken.Read(tokenFile); unchanged != token {
		t.Fatal("a refused token rotate changed the token")
	}

	stdoutIsTerminal = func(io.Writer) bool { return true }
	code, stdout, stderr = runCLI(t, "token", "rotate", "--workspace", ws)
	stdoutIsTerminal = original
	rotated, err := hubtoken.Read(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	want = "Hub token rotated.\n" +
		"Token file:  " + tokenFile + "\n" +
		"\n" +
		"Next: Copy the new token file to each machine that connects to this hub, replacing the old one.\n"
	if code != 0 || stderr != "" || stdout != want || rotated == token {
		t.Fatalf("token rotate: exit %d, stderr %q, stdout:\n%s\nwant:\n%s", code, stderr, stdout, want)
	}
	if strings.Contains(stdout+stderr, rotated) || strings.Contains(stdout+stderr, token) {
		t.Fatal("token rotate printed a token")
	}

	if code, _, stderr := runCLI(t, "token"); code != 2 || !strings.Contains(stderr, "show, rotate or invite") {
		t.Fatalf("bare token: exit %d, stderr %q", code, stderr)
	}
	if code, _, stderr := runCLI(t, "token", "show", "--yes", "--workspace", ws); code != 2 || !strings.Contains(stderr, `unknown argument "--yes"`) {
		t.Fatalf("token show --yes: exit %d, stderr %q", code, stderr)
	}
	if code, stdout, _ := runCLI(t, "help", "token"); code != 0 || !strings.Contains(stdout, "skillhub token rotate") {
		t.Fatalf("help token: exit %d, stdout %q", code, stdout)
	}
	if code, stdout, _ := runCLI(t, "help", "serve"); code != 0 || strings.Contains(stdout, "--trust-private-network") || !strings.Contains(stdout, "skillhub serve --addr 127.0.0.1:7420") {
		t.Fatalf("help serve: exit %d, stdout %q", code, stdout)
	}
}
