package hostintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeCuratorMatrixCutover(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	original := bytes.Clone(matrixData)
	t.Cleanup(func() { matrixData = original })
	for _, adapter := range SupportedAdapters() {
		t.Run(string(adapter.Host), func(t *testing.T) {
			matrixData = bytes.Clone(original)
			root := t.TempDir()
			request := Request{Workspace: root, Binary: filepath.Join(root, "skillhub"), Hosts: []Host{adapter.Host}}
			install := func() {
				t.Helper()
				plan, err := Plan(context.Background(), request)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = Apply(context.Background(), plan, ApplyOptions{Confirmed: true}); err != nil {
					t.Fatal(err)
				}
			}
			install()
			path := filepath.Join(root, filepath.FromSlash(adapter.SkillRelativePath))
			bundled := readTestFile(t, path)
			var doc map[string]any
			if err := json.Unmarshal(original, &doc); err != nil {
				t.Fatal(err)
			}
			names := map[Host]string{HostClaude: "Claude Code", HostCodex: "Codex CLI", HostGemini: "Gemini CLI"}
			for _, value := range doc["stock_clients"].([]any) {
				entry := value.(map[string]any)
				if entry["client"] == names[adapter.Host] {
					entry["skills_extension"] = map[string]any{"status": "verified", "evidence": "test fixture"}
				}
			}
			matrixData, _ = json.Marshal(doc)
			inspection, err := Inspect(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range inspection.Hosts[0].Files {
				if file.Kind == ChangeNativeSkill && file.Current {
					t.Fatal("obsolete native copy reported current")
				}
			}
			edited := append(bytes.Clone(bundled), []byte("\nuser customization\n")...)
			writeTestFile(t, path, edited, 0o644)
			if _, err := Plan(context.Background(), request); !errors.Is(err, ErrConflict) {
				t.Fatalf("edited copy: %v", err)
			} else if strings.Contains(err.Error(), "doctor --fix") || !strings.Contains(err.Error(), "delete it") || !strings.Contains(err.Error(), "skillhub connect") {
				// doctor --fix plans through the same cutover and hits this conflict too.
				t.Fatalf("edited-copy conflict must point at manual deletion, not doctor --fix: %v", err)
			}
			if !bytes.Equal(readTestFile(t, path), edited) {
				t.Fatal("user copy modified")
			}
			writeTestFile(t, path, bundled, 0o644)
			install()
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("native copy remains: %v", err)
			}
			install()
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("native copy reinstalled: %v", err)
			}
		})
	}
}
