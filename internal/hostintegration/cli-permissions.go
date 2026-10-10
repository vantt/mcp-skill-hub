package hostintegration

import (
	"fmt"
	"slices"
	"strings"
)

var claudeCLIRules = []struct {
	key    string
	values []string
}{
	{"allow", []string{"Bash(skillhub:*)"}},
	{"ask", []string{"Bash(skillhub * --yes*)", "Bash(skillhub * confirm *)"}},
	{"deny", []string{"Bash(skillhub * --approve-content*)"}},
}

func claudePermissionsPreview(raw, desired []byte) string {
	var out strings.Builder
	out.WriteString("managed permissions:")
	for _, key := range []string{"additionalDirectories", "allow", "ask", "deny"} {
		before, _ := jsonStringArrayAt(raw, []string{"permissions", key})
		after, _ := jsonStringArrayAt(desired, []string{"permissions", key})
		for _, value := range after {
			if !slices.Contains(before, value) {
				fmt.Fprintf(&out, "\npermissions.%s: add %s", key, value)
			}
		}
	}
	return boundPreview(out.String())
}
