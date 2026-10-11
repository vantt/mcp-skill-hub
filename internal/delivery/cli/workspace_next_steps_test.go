package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitNextStepStagesWorkspaceFoldersByName(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init=%d: %s", code, stderr.String())
	}
	text := strings.Join(strings.Fields(stdout.String()), " ")
	if strings.Contains(text, "add -A") || !strings.Contains(text, " add .gitignore .skillhub skills ") {
		t.Fatalf("the commit hint must name the folders it stages:\n%s", stdout.String())
	}
}

func TestIntegrateHelpIsTheConnectHelp(t *testing.T) {
	t.Parallel()
	var connectHelp, integrateHelp, stderr bytes.Buffer
	if code := Run([]string{"help", "connect"}, &connectHelp, &stderr); code != 0 {
		t.Fatalf("help connect=%d: %s", code, stderr.String())
	}
	if code := Run([]string{"help", "integrate"}, &integrateHelp, &stderr); code != 0 || integrateHelp.String() != connectHelp.String() {
		t.Fatalf("help integrate=%d: %s", code, stderr.String())
	}
}
