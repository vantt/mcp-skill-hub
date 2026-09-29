package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestInsightApplyRejectsYesInHumanAndJSONModes(t *testing.T) {
	for _, jsonMode := range []bool{false, true} {
		name := "human"
		args := []string{"apply", "INS-one", "--workspace", t.TempDir(), "--proposal-file", "unused.json", "--yes"}
		if jsonMode {
			name = "json"
			args = append(args, "--json")
		}
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := runInsight(context.Background(), args, &stdout, &stderr); code == 0 {
				t.Fatalf("apply --yes unexpectedly succeeded: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			combined := stdout.String() + stderr.String()
			if !strings.Contains(combined, "preview-only") || !strings.Contains(combined, "separate insight confirm") {
				t.Fatalf("apply --yes response did not explain the confirmation boundary: %q", combined)
			}
		})
	}
}
