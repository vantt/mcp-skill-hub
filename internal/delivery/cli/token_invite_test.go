package cli

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/delivery/hostpolicy"
	"github.com/vantt/skillhub-mcp/internal/hubtoken"
)

// withInviteEnvironment makes the command believe it runs in a terminal on
// the machine bannerInterfaces describes.
func withInviteEnvironment(t *testing.T) {
	t.Helper()
	terminal, interfaces := stdoutIsTerminal, systemInterfaces
	stdoutIsTerminal = func(io.Writer) bool { return true }
	systemInterfaces = func() ([]hostpolicy.Interface, error) { return bannerInterfaces, nil }
	t.Cleanup(func() { stdoutIsTerminal, systemInterfaces = terminal, interfaces })
}

var inviteCodePattern = regexp.MustCompile(`--invite ([A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4})\n`)

func inviteFiles(t *testing.T, workspace string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(workspace, "runtime", "config", "invites"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestTokenInviteListsOnlyAddressesPairingAccepts(t *testing.T) {
	withInviteEnvironment(t)
	ws := initTestWorkspace(t)

	code, stdout, stderr := runCLI(t, "token", "invite", "--workspace", ws)
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	match := inviteCodePattern.FindStringSubmatch(stdout)
	if match == nil {
		t.Fatalf("no connect line with a grouped code:\n%s", stdout)
	}
	for _, want := range []string{
		"Hub folder:  " + ws + "\n",
		"works once",
		"  $ skillhub connect --hub http://100.101.102.103:7420 --invite " + match[1] + "\n",
		"not the hub token",
		"Running this command again gives a new code",
		"same --workspace and --addr",
		"Tailscale or an SSH tunnel",
		"scp",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "192.168.1.20") || strings.Contains(stdout, "eth0") {
		t.Errorf("a LAN address is listed:\n%s", stdout)
	}
	if strings.Count(stdout, "Tailscale or an SSH tunnel") != 1 {
		t.Errorf("the explanation is not printed exactly once:\n%s", stdout)
	}
	if !regexp.MustCompile(`expires at \d\d:\d\d`).MatchString(stdout) {
		t.Errorf("no HH:MM expiry:\n%s", stdout)
	}
}

func TestTokenInviteForALoopbackAddressPrintsTheTunnelHint(t *testing.T) {
	withInviteEnvironment(t)
	ws := initTestWorkspace(t)

	code, stdout, stderr := runCLI(t, "token", "invite", "--workspace", ws, "--addr", "127.0.0.1:7425")
	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, want := range []string{
		"  $ ssh -N -L 7425:127.0.0.1:7425 ",
		"  $ skillhub connect --hub http://127.0.0.1:7425 --invite ",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "Tailscale or an SSH tunnel") {
		t.Errorf("explains a missing address although none was skipped:\n%s", stdout)
	}
}

func TestTokenInviteStoresOneInvitePerRunAndNeverTouchesTheHubToken(t *testing.T) {
	withInviteEnvironment(t)
	ws := initTestWorkspace(t)

	_, first, _ := runCLI(t, "token", "invite", "--workspace", ws)
	_, second, _ := runCLI(t, "token", "invite", "--workspace", ws)
	one, two := inviteCodePattern.FindStringSubmatch(first), inviteCodePattern.FindStringSubmatch(second)
	if one == nil || two == nil || one[1] == two[1] {
		t.Fatalf("two runs printed %v and %v", one, two)
	}
	if files := inviteFiles(t, ws); len(files) != 2 {
		t.Fatalf("invite files %v, want one per run", files)
	}
	if _, err := os.Stat(hubtoken.DefaultPath(ws)); !os.IsNotExist(err) {
		t.Fatalf("token invite created the hub token: %v", err)
	}

	// With a hub token present, it is never printed.
	if _, err := hubtoken.Ensure(hubtoken.DefaultPath(ws)); err != nil {
		t.Fatal(err)
	}
	token, err := hubtoken.Read(hubtoken.DefaultPath(ws))
	if err != nil {
		t.Fatal(err)
	}
	_, stdout, stderr := runCLI(t, "token", "invite", "--workspace", ws)
	if strings.Contains(stdout+stderr, token) {
		t.Fatal("token invite printed the hub token")
	}
}

func TestTokenInviteRefusesWithoutATerminal(t *testing.T) {
	ws := initTestWorkspace(t)

	code, stdout, stderr := runCLI(t, "token", "invite", "--workspace", ws)
	if code != 1 || stdout != "" || !strings.Contains(stderr, "invite is created only on a terminal") ||
		!strings.Contains(stderr, "  $ skillhub token invite --workspace ") {
		t.Fatalf("exit %d, stdout %q, stderr:\n%s", code, stdout, stderr)
	}
	if files := inviteFiles(t, ws); len(files) != 0 {
		t.Fatalf("a refused run stored %v", files)
	}
}

func TestTokenInviteFlags(t *testing.T) {
	withInviteEnvironment(t)
	ws := initTestWorkspace(t)

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--token-file", "x"}, `unknown argument "--token-file"`},
		{[]string{"--dry-run"}, `unknown argument "--dry-run"`},
		{[]string{"--addr"}, "--addr requires a value"},
		{[]string{"--addr", "127.0.0.1:0"}, "fixed port"},
	} {
		code, _, stderr := runCLI(t, append([]string{"token", "invite", "--workspace", ws}, tc.args...)...)
		if code != 2 || !strings.Contains(stderr, tc.want) {
			t.Errorf("%v: exit %d, stderr %q, want %q", tc.args, code, stderr, tc.want)
		}
	}
	if code, _, stderr := runCLI(t, "token", "show", "--addr", "127.0.0.1"); code != 2 || !strings.Contains(stderr, `unknown argument "--addr"`) {
		t.Errorf("token show --addr: exit %d, stderr %q", code, stderr)
	}
	if files := inviteFiles(t, ws); len(files) != 0 {
		t.Fatalf("rejected flags stored %v", files)
	}
}

func TestHelpTokenListsInvite(t *testing.T) {
	code, stdout, _ := runCLI(t, "help", "token")
	if code != 0 || !strings.Contains(stdout, "skillhub token invite [--workspace <path>] [--addr <host:port>[,<host:port>...]]") || !strings.Contains(stdout, "10 minutes") {
		t.Fatalf("help token: exit %d:\n%s", code, stdout)
	}
	if code, stdout, _ := runCLI(t, "help", "serve"); code != 0 || strings.Contains(stdout, "--print") {
		t.Fatalf("help serve: exit %d:\n%s", code, stdout)
	}
}
