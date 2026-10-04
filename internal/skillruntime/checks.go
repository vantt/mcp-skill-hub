package skillruntime

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Check kinds.
const (
	KindPlatform   = "platform"
	KindBin        = "bin"
	KindEnv        = "env"
	KindSetupCheck = "setup_check"
)

// Check statuses.
const (
	StatusPass    = "pass"
	StatusFail    = "fail"
	StatusSkipped = "skipped"
)

// Check details that callers may match on.
const (
	DetailNotFound          = "not_found"
	DetailMissing           = "missing"
	DetailVersionUnparsable = "version_unparsable"
)

// Check is one prerequisite result. Detail never contains environment values.
type Check struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// LookupEnv matches os.LookupEnv.
type LookupEnv func(key string) (string, bool)

// LookPath matches exec.LookPath.
type LookPath func(file string) (string, error)

// PlatformChecks evaluates the one requirement the hub can verify for the
// agent: the operating system. Binaries and environment variables live in the
// agent's own shell, which the hub process cannot see, so they are verified by
// the skill's `check` command or by the terminal doctor, never here.
func PlatformChecks(spec Spec, goos string) []Check {
	if platform, ok := platformCheck(spec, goos); ok {
		return []Check{platform}
	}
	return nil
}

func platformCheck(spec Spec, goos string) (Check, bool) {
	if len(spec.Requires.Platforms) == 0 {
		return Check{}, false
	}
	check := Check{Kind: KindPlatform, Name: goos, Status: StatusPass}
	if !slices.Contains(spec.Requires.Platforms, goos) {
		check.Status = StatusFail
		check.Detail = "requires " + strings.Join(spec.Requires.Platforms, ", ")
	}
	return check, true
}

func envChecks(spec Spec, env LookupEnv) []Check {
	checks := make([]Check, 0, len(spec.Requires.Env))
	for _, name := range spec.Requires.Env {
		// The value is discarded immediately; only presence is recorded.
		_, present := env(name)
		check := Check{Kind: KindEnv, Name: name, Status: StatusPass}
		if !present {
			check.Status = StatusFail
			check.Detail = DetailMissing
		}
		checks = append(checks, check)
	}
	return checks
}

type version [3]int

func (v version) compare(other version) int {
	for i := range v {
		if v[i] != other[i] {
			if v[i] < other[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

type constraint struct {
	op   string
	want version
	text string
}

var (
	constraintPattern = regexp.MustCompile(`^(>=|>|<=|<|=)?\s*(\d+(?:\.\d+){0,2})$`)
	versionPattern    = regexp.MustCompile(`\d+(?:\.\d+){0,2}`)
)

// parseConstraint accepts ">=18", "> 3.9", "=1.2.3", or a bare version. A bare
// version means a minimum (">="), matching how tool requirements are usually
// written ("python 3.11" means 3.11 or newer).
func parseConstraint(text string) (constraint, error) {
	match := constraintPattern.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		return constraint{}, fmt.Errorf("version constraint %q must look like \">=18\" or \"3.11\"", text)
	}
	op := match[1]
	if op == "" {
		op = ">="
	}
	want, err := parseVersion(match[2])
	if err != nil {
		return constraint{}, err
	}
	return constraint{op: op, want: want, text: op + match[2]}, nil
}

// parseVersion reads up to three dot-separated numbers; missing parts are 0.
func parseVersion(text string) (version, error) {
	var out version
	for i, part := range strings.SplitN(text, ".", 3) {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return version{}, fmt.Errorf("invalid version %q", text)
		}
		out[i] = n
	}
	return out, nil
}

// extractVersion returns the first version-looking token in probe output.
func extractVersion(output string) (string, version, bool) {
	token := versionPattern.FindString(output)
	if token == "" {
		return "", version{}, false
	}
	parsed, err := parseVersion(token)
	if err != nil {
		return "", version{}, false
	}
	return token, parsed, true
}

func (c constraint) satisfiedBy(have version) bool {
	cmp := have.compare(c.want)
	switch c.op {
	case ">=":
		return cmp >= 0
	case ">":
		return cmp > 0
	case "<=":
		return cmp <= 0
	case "<":
		return cmp < 0
	default:
		return cmp == 0
	}
}
