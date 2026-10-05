package app

import (
	"fmt"
	"strings"

	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

func runtimeHintFindings(skillID string, hints skillruntime.Hints, hasRuntimeBlock ...bool) []Warning {
	var warnings []Warning
	hasRuntime := len(hasRuntimeBlock) > 0 && hasRuntimeBlock[0]
	if len(hints.AbsoluteInstallPaths) > 0 {
		warnings = append(warnings, Warning{
			Code:    "absolute_install_path",
			Summary: fmt.Sprintf("%s: References files with absolute install paths (%s). Fix: Reference files relative to the skill folder ($SKILLHUB_SKILL_DIR).", skillID, strings.Join(hints.AbsoluteInstallPaths, ", ")),
		})
	}

	if hints.MissingRuntimeBlock {
		warnings = append(warnings, Warning{
			Code:    "missing_runtime_block",
			Summary: fmt.Sprintf("%s: Missing runtime block. Fix: Declare a `runtime` block (bins, env, setup) with `skillhub skill edit %s --runtime-file <yaml>`.", skillID, skillID),
		})
	}

	if hints.InstallProseDetected && !hasRuntime {
		warnings = append(warnings, Warning{
			Code:    "install_prose_without_runtime",
			Summary: fmt.Sprintf("%s: Installation prose was detected without a declared runtime block. Fix: Turn the install steps into `runtime.setup` that installs into $SKILLHUB_STATE_DIR.", skillID),
		})
	}

	if len(hints.MissingLockfiles) > 0 {
		warnings = append(warnings, Warning{
			Code:    "missing_lockfile",
			Summary: fmt.Sprintf("%s: Dependency manifests missing lockfiles (%s). Fix: Commit a lockfile or pin versions.", skillID, strings.Join(hints.MissingLockfiles, ", ")),
		})
	}

	return warnings
}

func routingLintWarning(f resolverpkg.LintFinding) Warning {
	prefix := f.SkillID
	if f.OtherSkillID != "" {
		prefix = fmt.Sprintf("%s, %s", f.SkillID, f.OtherSkillID)
	}
	return Warning{
		Code:    f.Code,
		Summary: fmt.Sprintf("%s: %s Fix: %s.", prefix, f.Message, f.Fix),
	}
}
