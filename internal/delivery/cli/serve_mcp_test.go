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
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/hubtoken"
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

func TestServeMCPWithoutAddrIsTheStdioServer(t *testing.T) {
	ws := initTestWorkspace(t)
	// Host registrations written by `skillhub connect` run this exact command.
	registered := stdioToolNames(t, "mcp", "serve", "--profile", "runtime", "--workspace", ws)
	if want := []string{"skill_feedback", "skill_get", "skill_resolve"}; !slices.Equal(registered, want) {
		t.Fatalf("mcp serve --profile runtime tools = %v, want %v", registered, want)
	}
	unified := stdioToolNames(t, "serve", "mcp", "--profile", "runtime", "--workspace", ws)
	if !slices.Equal(unified, registered) {
		t.Fatalf("serve mcp tools = %v, mcp serve tools = %v", unified, registered)
	}
	allAlias := stdioToolNames(t, "mcp", "serve", "--workspace", ws)
	allUnified := stdioToolNames(t, "serve", "mcp", "--workspace", ws)
	if !slices.Equal(allAlias, allUnified) || len(allAlias) <= len(registered) {
		t.Fatalf("default profile: mcp serve %v, serve mcp %v", allAlias, allUnified)
	}
	if entries, err := os.ReadDir(filepath.Join(ws, "runtime", "config")); err == nil {
		for _, entry := range entries {
			if entry.Name() == hubtoken.FileName {
				t.Fatal("the stdio server created a hub token")
			}
		}
	}
}

func TestServeMCPAndMCPServeShareFlagErrors(t *testing.T) {
	ws := initTestWorkspace(t)
	cases := [][]string{
		{"--workspace", ws, "--profile"},
		{"--workspace", ws, "--profile", "bogus"},
		{"--workspace", ws, "--bogus"},
		{"--workspace"},
	}
	for _, flags := range cases {
		aliasCode, aliasOut, aliasErr := runCLI(t, append([]string{"mcp", "serve"}, flags...)...)
		code, out, errText := runCLI(t, append([]string{"serve", "mcp"}, flags...)...)
		if aliasCode != 2 || code != aliasCode || out != aliasOut || errText != aliasErr {
			t.Errorf("flags %v: mcp serve (%d, %q) differs from serve mcp (%d, %q)", flags, aliasCode, aliasErr, code, errText)
		}
	}
}

func TestServeMCPNetworkFlagErrors(t *testing.T) {
	ws := initTestWorkspace(t)
	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{
			name: "non-loopback address without trust",
			args: []string{"serve", "mcp", "--addr", "192.0.2.10:7420", "--workspace", ws},
			code: 2,
			want: "ERROR: The MCP listener will not start on 192.0.2.10:7420.\n" +
				"WHY: That address can be reached from other machines and the listener speaks plain HTTP, so task\n" +
				"     text and skill content would cross the network unencrypted.\n" +
				"FIX: Listen on 127.0.0.1 and reach it through an SSH tunnel, or, on a private network such as\n" +
				"     Tailscale, add --trust-private-network.\n",
		},
		{
			name: "curation profile on the network",
			args: []string{"serve", "mcp", "--addr", "127.0.0.1:0", "--profile", "curation", "--workspace", ws},
			code: 2,
			want: "ERROR: The request cannot be accepted.\n" +
				"WHY: --addr serves the runtime tools only, so --profile curation is not allowed\n" +
				"FIX: Drop --profile, or pass --profile runtime. Curation stays on this machine over stdio.\n",
		},
		{
			name: "all profile on the network",
			args: []string{"serve", "mcp", "--addr", "127.0.0.1:0", "--profile", "all", "--workspace", ws},
			code: 2,
			want: "WHY: --addr serves the runtime tools only, so --profile all is not allowed\n",
		},
		{
			name: "the mcp serve alias keeps its old flags",
			args: []string{"mcp", "serve", "--addr", "127.0.0.1:0", "--workspace", ws},
			code: 2,
			want: "WHY: unknown MCP argument \"--addr\"\n",
		},
		{
			name: "the mcp serve alias refuses the trust flag",
			args: []string{"mcp", "serve", "--trust-private-network", "--workspace", ws},
			code: 2,
			want: "WHY: unknown MCP argument \"--trust-private-network\"\n",
		},
		{
			name: "network flag without an address",
			args: []string{"serve", "mcp", "--trust-private-network", "--workspace", ws},
			code: 2,
			want: "ERROR: The request cannot be accepted.\n" +
				"WHY: --trust-private-network, --token-file, and --allow-host only apply to the network listener\n" +
				"FIX: Add --addr <host:port>, or drop those flags to serve an agent host over stdio.\n",
		},
		{
			name: "malformed address",
			args: []string{"serve", "mcp", "--addr", "7420", "--workspace", ws},
			code: 2,
			want: "use host:port",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, tc.args...)
			if code != tc.code || !strings.Contains(stderr, tc.want) || stdout != "" {
				t.Fatalf("exit %d, stdout %q, stderr:\n%s\nwant exit %d and:\n%s", code, stdout, stderr, tc.code, tc.want)
			}
		})
	}
}

func TestListenerBannerGolden(t *testing.T) {
	loopback := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7420}
	var out bytes.Buffer
	writeListenerBanner(&out, listenerBanner{
		address:   loopback,
		workspace: "/home/user/skillhub",
		tokenFile: "/home/user/skillhub/runtime/config/.hub-token",
		hubURLs:   hubURLs(loopback),
	})
	want := `Skill Hub MCP listener is running with the runtime tools only (skill_resolve, skill_get,
skill_feedback).

Listening on:  127.0.0.1:7420
Workspace:     /home/user/skillhub
Token file:    /home/user/skillhub/runtime/config/.hub-token
Transport:     loopback only; other machines reach it through an SSH tunnel

On machine A:
Open the tunnel (keep it running):
  $ ssh -N -L 7420:127.0.0.1:7420 <user>@<this-machine>
Copy the token file to machine A (for example with scp) and pass its path there, then connect:
  $ skillhub connect --hub http://127.0.0.1:7420 --token-file /home/user/skillhub/runtime/config/.hub-token --yes

The token itself is never printed; ` + "`skillhub token show`" + ` prints it on a terminal. Stop the listener
with Ctrl-C.
`
	if out.String() != want {
		t.Fatalf("loopback banner:\n%s\nwant:\n%s", out.String(), want)
	}

	private := &net.TCPAddr{IP: net.ParseIP("100.101.102.103"), Port: 7420}
	out.Reset()
	writeListenerBanner(&out, listenerBanner{
		address:   private,
		workspace: "/home/user/skillhub",
		tokenFile: "/home/user/skillhub/runtime/config/.hub-token",
		trusted:   true,
		hubURLs:   hubURLs(private),
	})
	want = `Skill Hub MCP listener is running with the runtime tools only (skill_resolve, skill_get,
skill_feedback).
WARNING: Plain HTTP: --trust-private-network says this network is private (an SSH tunnel or a VPN
         such as Tailscale).

Listening on:  100.101.102.103:7420
Workspace:     /home/user/skillhub
Token file:    /home/user/skillhub/runtime/config/.hub-token
Transport:     plain HTTP on a network you trust to be private

On machine A:
Copy the token file to machine A (for example with scp) and pass its path there, then connect:
  $ skillhub connect --hub http://100.101.102.103:7420 --token-file /home/user/skillhub/runtime/config/.hub-token --yes

The token itself is never printed; ` + "`skillhub token show`" + ` prints it on a terminal. Stop the listener
with Ctrl-C.
`
	if out.String() != want {
		t.Fatalf("trusted banner:\n%s\nwant:\n%s", out.String(), want)
	}

	wildcard := &net.TCPAddr{IP: net.IPv4zero, Port: 7420}
	out.Reset()
	writeListenerBanner(&out, listenerBanner{
		address:   wildcard,
		workspace: "/home/user/my skillhub",
		tokenFile: "/home/user/my skillhub/runtime/config/.hub-token",
		trusted:   true,
		hubURLs:   []string{"http://192.0.2.5:7420", "http://127.0.0.1:7420"},
	})
	for _, line := range []string{
		"WARNING: This address listens on every network interface, including any that are not private.",
		"Hub URLs:\n  - http://192.0.2.5:7420\n  - http://127.0.0.1:7420\n",
		"  $ skillhub connect --hub http://192.0.2.5:7420 --token-file '/home/user/my skillhub/runtime/config/.hub-token' --yes\n",
	} {
		if !strings.Contains(out.String(), line) {
			t.Fatalf("wildcard banner lacks %q:\n%s", line, out.String())
		}
	}
}

func TestServeMCPListenerLifecycle(t *testing.T) {
	ws := initTestWorkspace(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout, stderr syncBuffer
	exitCh := make(chan int, 1)
	go func() {
		exitCh <- RunContext(ctx, []string{"serve", "mcp", "--addr", "127.0.0.1:0", "--workspace", ws}, &stdout, &stderr)
	}()

	const marker = "skillhub connect --hub "
	var hub string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && hub == "" {
		if index := strings.Index(stdout.String(), marker); index != -1 {
			rest := stdout.String()[index+len(marker):]
			if end := strings.IndexByte(rest, ' '); end != -1 {
				hub = rest[:end]
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if hub == "" {
		t.Fatalf("no connect line; stdout: %s; stderr: %s", stdout.String(), stderr.String())
	}
	tokenFile := hubtoken.DefaultPath(ws)
	if !strings.Contains(stdout.String(), "--token-file "+tokenFile+" --yes") {
		t.Fatalf("connect line lacks the token file path:\n%s", stdout.String())
	}
	token, err := hubtoken.Read(tokenFile)
	if err != nil {
		t.Fatal(err)
	}

	post := func(authorization string) int {
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
	if status := post(""); status != http.StatusUnauthorized {
		t.Errorf("without token status = %d, want 401", status)
	}
	if status := post("Bearer " + token); status != http.StatusOK {
		t.Errorf("with token status = %d, want 200", status)
	}

	cancel()
	select {
	case code := <-exitCh:
		if code != 0 {
			t.Fatalf("exit code = %d; stderr: %s", code, stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("listener did not stop")
	}
	if strings.Contains(stdout.String()+stderr.String(), token) {
		t.Fatal("the token appeared in the listener output")
	}
}

func TestTokenCommands(t *testing.T) {
	ws := initTestWorkspace(t)
	tokenFile := hubtoken.DefaultPath(ws)

	code, stdout, stderr := runCLI(t, "token", "show", "--workspace", ws)
	want := "ERROR: The hub token is printed only on a terminal.\n" +
		"WHY: Standard output is redirected or captured, and the token must not end up in a file, a log, or\n" +
		"     an agent transcript.\n" +
		"FIX: Copy the token file below instead, or run `skillhub token show` directly in a terminal.\n" +
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

	code, stdout, stderr = runCLI(t, "token", "rotate", "--workspace", ws)
	want = "This replaces the hub token with a new random one.\n" +
		"Token file:  " + tokenFile + "\n" +
		"  - Every machine that connects to this hub stops working until it gets the new token file.\n" +
		"  - A running listener uses the new token from its next request; it does not need a restart.\n" +
		"\n" +
		"Next: Apply the rotation:\n" +
		"  $ skillhub token rotate --workspace " + ws + " --yes\n"
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("token rotate preview: exit %d, stderr %q, stdout:\n%s\nwant:\n%s", code, stderr, stdout, want)
	}
	if unchanged, _ := hubtoken.Read(tokenFile); unchanged != token {
		t.Fatal("the rotate preview changed the token")
	}

	code, stdout, stderr = runCLI(t, "token", "rotate", "--workspace", ws, "--yes")
	rotated, err := hubtoken.Read(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	want = "Hub token rotated.\n" +
		"Token file:  " + tokenFile + "\n" +
		"\n" +
		"Next: Copy the new token file to each machine that connects to this hub, replacing the old one.\n"
	if code != 0 || stderr != "" || stdout != want || rotated == token {
		t.Fatalf("token rotate --yes: exit %d, stderr %q, stdout:\n%s\nwant:\n%s", code, stderr, stdout, want)
	}
	if strings.Contains(stdout+stderr, rotated) || strings.Contains(stdout+stderr, token) {
		t.Fatal("token rotate printed a token")
	}

	if code, _, stderr := runCLI(t, "token"); code != 2 || !strings.Contains(stderr, "show or rotate") {
		t.Fatalf("bare token: exit %d, stderr %q", code, stderr)
	}
	if code, _, stderr := runCLI(t, "token", "show", "--yes", "--workspace", ws); code != 2 || !strings.Contains(stderr, `unknown argument "--yes"`) {
		t.Fatalf("token show --yes: exit %d, stderr %q", code, stderr)
	}
	if code, stdout, _ := runCLI(t, "help", "token"); code != 0 || !strings.Contains(stdout, "skillhub token rotate") {
		t.Fatalf("help token: exit %d, stdout %q", code, stdout)
	}
	if code, stdout, _ := runCLI(t, "help", "serve"); code != 0 || !strings.Contains(stdout, "--trust-private-network") || !strings.Contains(stdout, "skillhub serve mcp --addr 127.0.0.1:7420") {
		t.Fatalf("help serve: exit %d, stdout %q", code, stdout)
	}
}
