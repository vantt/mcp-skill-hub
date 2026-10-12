package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeNode(path, version string, err error) systemSkillProvider {
	return systemSkillProvider{
		lookPath: func(string) (string, error) {
			if path == "" {
				return "", errors.New("not found")
			}
			return path, nil
		},
		nodeVersion: func(context.Context, string) (string, error) { return version, err },
	}
}

// distillLabHub is a folder that holds the distill-lab entry file, as an
// initialized hub does.
func distillLabHub(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	entry := filepath.Join(root, ".agents", "skills", "distill-lab", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDistillLabNodeNotice(t *testing.T) {
	t.Parallel()
	root := distillLabHub(t)
	report := RoleReport{Cwd: root, Hub: &HubRef{Root: root}}
	cases := []struct {
		name     string
		provider systemSkillProvider
		want     string
	}{
		{"missing", fakeNode("", "", nil), "not found"},
		{"old", fakeNode("/usr/bin/node", "v16.20.2\n", nil), "found v16.20.2"},
		{"unreadable", fakeNode("/usr/bin/node", "node", nil), "could not be read"},
		{"failing", fakeNode("/usr/bin/node", "", errors.New("exit status 1")), "failed"},
		{"18", fakeNode("/usr/bin/node", "v18.0.0\n", nil), ""},
		{"24", fakeNode("/usr/bin/node", "v24.21.0\n", nil), ""},
	}
	for _, test := range cases {
		findings := test.provider.Check(t.Context(), report, DepthFull)
		if test.want == "" {
			if len(findings) != 0 {
				t.Errorf("%s: findings = %+v", test.name, findings)
			}
			continue
		}
		if len(findings) != 1 {
			t.Fatalf("%s: findings = %+v", test.name, findings)
		}
		finding := findings[0]
		if finding.Code != "distill_lab_node_missing" || finding.Severity != SeverityNotice || finding.Fix != nil ||
			!strings.Contains(finding.Message, "Node 18 or newer") || !strings.Contains(finding.Message, test.want) || finding.Manual != "" || !strings.Contains(finding.Message, "https://nodejs.org") {
			t.Errorf("%s: finding = %+v", test.name, finding)
		}
	}
}

func TestDistillLabNodeNoticeRunsOnlyInDoctorForAHubWithDistillLab(t *testing.T) {
	t.Parallel()
	ran := false
	provider := fakeNode("", "", nil)
	provider.lookPath = func(string) (string, error) { ran = true; return "", errors.New("not found") }
	root := distillLabHub(t)
	if findings := provider.Check(t.Context(), RoleReport{Cwd: root, Hub: &HubRef{Root: root}}, DepthLight); len(findings) != 0 || ran {
		t.Fatalf("status looked for node: %+v", findings)
	}
	if provider.Applies(RoleReport{Cwd: t.TempDir()}) {
		t.Fatal("applies without a hub")
	}
	without := t.TempDir()
	if findings := provider.Check(t.Context(), RoleReport{Cwd: without, Hub: &HubRef{Root: without}}, DepthFull); len(findings) != 0 || ran {
		t.Fatalf("checked node for a hub without distill-lab: %+v", findings)
	}
}

func TestSystemSkillProviderIsRegisteredOnceAndReportsNoIntegrity(t *testing.T) {
	t.Parallel()
	count := 0
	for _, provider := range registeredProviders() {
		if provider.Name() == "system-skills" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("system-skills provider registered %d times", count)
	}
	root := healthHub(t)
	for _, section := range Run(t.Context(), RoleReport{Cwd: root, Hub: &HubRef{Root: root}}, DepthFull) {
		for _, finding := range section.Findings {
			if finding.Code == "system_skill_blocked" || strings.HasPrefix(finding.Code, "host_") {
				t.Fatalf("a healthy hub reports %+v", finding)
			}
		}
	}
}
