package cli

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/hubtoken"
)

func TestServePrintFlag(t *testing.T) {
	cfg, reason, _ := parseServeArgs([]string{"--print", "--addr", "127.0.0.1:7420"})
	if reason != "" || !cfg.printConnect || cfg.installService {
		t.Fatalf("--print: reason %q, config %+v", reason, cfg)
	}
	for _, args := range [][]string{
		{"web", "--print"},
		{"--print", "--install-service"},
	} {
		if _, reason, _ := parseServeArgs(args); reason == "" {
			t.Errorf("%v was accepted", args)
		}
	}
}

func TestServePrintWritesTheConnectLinesWithoutStartingAnything(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "skill-hub")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	tokenFile := hubtoken.DefaultPath(workspace)

	var out, errOut bytes.Buffer
	code := runServePrint(serveConfig{mode: serveModeMCP, printConnect: true},
		[]listenAddress{{host: "127.0.0.1", port: 7420}}, workspace, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{
		"Commands to connect another machine to this hub. Nothing was started.",
		"The token file does not exist yet",
		"ssh -N -L 7420:127.0.0.1:7420 ",
		"skillhub connect --hub http://127.0.0.1:7420 --token-file ~/.skillhub-token-127.0.0.1-7420",
		"Start the server with `skillhub serve`.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Ctrl-C") || strings.Contains(text, "is serving skills") {
		t.Errorf("preview reads like a running server:\n%s", text)
	}
	if _, err := os.Stat(tokenFile); err == nil {
		t.Fatal("--print created the token file")
	}

	if err := os.MkdirAll(filepath.Dir(tokenFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenFile, []byte("not-a-real-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := runServePrint(serveConfig{mode: serveModeMCP, printConnect: true},
		[]listenAddress{{host: "127.0.0.1", port: 7420}}, workspace, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "does not exist yet") || strings.Contains(out.String(), "not-a-real-token") {
		t.Errorf("token note or token value in output:\n%s", out.String())
	}
}

func TestServePrintNeedsAFixedPort(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runServePrint(serveConfig{mode: serveModeMCP, printConnect: true},
		[]listenAddress{{host: "127.0.0.1", port: 0}}, t.TempDir(), &out, &errOut)
	if code == 0 || !strings.Contains(out.String()+errOut.String(), "fixed port") {
		t.Fatalf("exit %d, output %q %q", code, out.String(), errOut.String())
	}
}

func TestPreviewBannerListsEveryReachableAddress(t *testing.T) {
	var out bytes.Buffer
	home := filepath.Join(t.TempDir(), "home")
	workspace := filepath.Join(home, "skill-hub")
	writeMCPBanner(&out, mcpBanner{
		addresses:  []net.Addr{&net.TCPAddr{IP: net.IPv4zero, Port: 7420}},
		workspace:  workspace,
		tokenFile:  hubtoken.DefaultPath(workspace),
		home:       home,
		user:       "ann",
		interfaces: bannerInterfaces,
		preview:    true,
	})
	text := out.String()
	for _, want := range []string{
		"Nothing was started.",
		"scp -p ann@192.168.1.20:skill-hub/runtime/config/.hub-token ~/.skillhub-token-192.168.1.20-7420",
		"skillhub connect --hub http://100.101.102.103:7420 --token-file ~/.skillhub-token-100.101.102.103-7420",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
}
