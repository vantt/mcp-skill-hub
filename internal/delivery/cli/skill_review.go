package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

// writeSkillReview formats the comprehensive offline review report.
func writeSkillReview(stdout, stderr io.Writer, jsonOutput, verbose bool, result app.SkillReviewResult) int {
	return writeResult(stdout, stderr, jsonOutput, result, func(p *termui.Printer) {
		var fields []termui.Field
		fields = append(fields,
			termui.Field{Label: "Review", Value: fmt.Sprintf("%s (%s)", result.Name, result.SkillID)},
			termui.Field{Label: "Collection", Value: result.Collection},
		)
		if result.Description != "" {
			fields = append(fields, termui.Field{Label: "Description", Value: result.Description})
		}

		validStr := "valid"
		if !result.Valid {
			validStr = "invalid"
		}
		eligibleStr := "eligible"
		if !result.RoutingEligible {
			eligibleStr = "not eligible"
		}
		servableStr := "servable"
		if !result.ServedFacts.Servable {
			servableStr = "not servable"
		}
		fields = append(fields, termui.Field{Label: "State", Value: fmt.Sprintf("%s (%s; routing: %s; %s)", result.LifecycleState, validStr, eligibleStr, servableStr)})

		filesStr := fmt.Sprintf("%s, %s (entrypoint: %s)",
			termui.Plural(result.ResourceStatus.ResourceCount, "file", "files"),
			termui.Bytes(result.ResourceStatus.TotalBytes),
			result.ResourceStatus.EntrypointPath)
		fields = append(fields, termui.Field{Label: "Files", Value: filesStr})

		originStr := "local authoring (not watched)"
		if result.Provenance != nil && result.Provenance.SourceLocator != "" {
			rev := result.Provenance.SourceRevision
			if len(rev) > 12 {
				rev = rev[:12]
			}
			if rev != "" {
				originStr = fmt.Sprintf("%s at %s", result.Provenance.SourceLocator, rev)
			} else {
				originStr = result.Provenance.SourceLocator
			}
		}
		fields = append(fields, termui.Field{Label: "Origin", Value: originStr})

		gitStr := "not configured"
		if result.Git.Configured {
			if result.Git.Dirty {
				gitStr = fmt.Sprintf("uncommitted changes (%d staged, %d unstaged, %d untracked)",
					len(result.Git.Staged), len(result.Git.Unstaged), len(result.Git.Untracked))
			} else {
				gitStr = "clean"
			}
		}
		fields = append(fields, termui.Field{Label: "Git", Value: gitStr})
		p.Fields(fields...)

		if len(result.CanonicalIssues) > 0 {
			p.Heading("Validation issues")
			p.Bullets(result.CanonicalIssues...)
		}

		if !result.ActivationReadiness.Ready {
			p.Heading("Activation readiness")
			var bullets []string
			if result.ActivationReadiness.UntouchedScaffold {
				bullets = append(bullets, "Untouched scaffold instructions must be replaced before activation.")
			}
			for _, missing := range result.ActivationReadiness.MissingFields {
				bullets = append(bullets, fmt.Sprintf("Missing required field: %s", missing))
			}
			for _, warn := range result.ActivationReadiness.Warnings {
				bullets = append(bullets, fmt.Sprintf("Warning: %s", warn))
			}
			p.Bullets(bullets...)
		}

		writeContentTrust(p, result.ContentTrust)
		writeRuntimeHints(p, result)

		if verbose {
			p.Heading("Catalog facts")
			p.Fields(
				termui.Field{Label: "Canonical entrypoint", Value: fmt.Sprintf("%s (status: %s, valid: %t)", result.CanonicalFacts.EntrypointPath, result.CanonicalFacts.Status, result.CanonicalFacts.Valid)},
				termui.Field{Label: "Served generation", Value: fmt.Sprintf("%s (servable: %t)", result.ServedFacts.Generation, result.ServedFacts.Servable)},
			)
			if len(result.ResourceStatus.Resources) > 0 {
				p.Heading("Resources")
				var resBullets []string
				for _, res := range result.ResourceStatus.Resources {
					digestShort := res.Digest
					if len(digestShort) > 19 && strings.HasPrefix(digestShort, "sha256:") {
						digestShort = "sha256:" + digestShort[7:19]
					}
					resBullets = append(resBullets, fmt.Sprintf("%s (%s, %s)", res.Path, digestShort, termui.Bytes(res.SizeBytes)))
				}
				p.Bullets(resBullets...)
			}
		}

		next := result.NextAction
		if next == "" {
			if result.LifecycleState == "draft" {
				next = fmt.Sprintf("edit with `skillhub skill edit %s --editor`, or review instructions with `skillhub skill show %s`", result.SkillID, result.SkillID)
			} else {
				next = fmt.Sprintf("ask your agent to use it, or inspect with `skillhub skill show %s`", result.SkillID)
			}
		}
		p.Blank()
		p.Next(next, "")
	})
}

// writeContentTrust prints the content trust block for third-party skills,
// including the exact approval command while review is required.
func writeContentTrust(p *termui.Printer, trust app.ContentTrust) {
	if !trust.ThirdParty {
		return
	}
	p.Heading("Content trust")
	state := "approved"
	if trust.RequiresReview() {
		state = "review required; agents get no content from this skill (" + strings.Join(trust.ReasonCodes, ", ") + ")"
	}
	p.Fields(
		termui.Field{Label: "Origin", Value: "third-party"},
		termui.Field{Label: "Approval", Value: state},
		termui.Field{Label: "Content digest", Value: trust.ContentDigest},
	)
	if trust.RequiresReview() {
		writeChangesSinceApproval(p, trust)
		p.Line("After reviewing the skill's files and runtime block, approve exactly this content with:")
		p.Command(trust.ApproveCommand)
	}
}

// writeRuntimeHints prints what the skill may need from the host and the
// suggested next step. It stays silent for instruction-only skills.
func writeRuntimeHints(p *termui.Printer, result app.SkillReviewResult) {
	hints := result.RuntimeHints
	if len(hints.Interpreters) == 0 && len(hints.DependencyManifests) == 0 && len(hints.MissingLockfiles) == 0 && len(hints.AbsoluteInstallPaths) == 0 && !hints.InstallProseDetected {
		return
	}
	p.Heading("Runtime")
	var fields []termui.Field
	if len(hints.Interpreters) > 0 {
		fields = append(fields, termui.Field{Label: "Interpreters", Value: strings.Join(hints.Interpreters, ", ")})
	}
	if len(hints.DependencyManifests) > 0 {
		fields = append(fields, termui.Field{Label: "Dependency files", Value: strings.Join(hints.DependencyManifests, ", ")})
	}
	if len(hints.MissingLockfiles) > 0 {
		fields = append(fields, termui.Field{Label: "Unpinned (no lockfile)", Value: strings.Join(hints.MissingLockfiles, ", ")})
	}
	if hints.InstallProseDetected {
		fields = append(fields, termui.Field{Label: "Install cues", Value: strings.Join(hints.InstallCues, ", ")})
	}
	if len(fields) > 0 {
		p.Fields(fields...)
	}
	if len(hints.AbsoluteInstallPaths) > 0 {
		p.Line("These files reference an install location that does not exist under the exported skill path:")
		p.Bullets(hints.AbsoluteInstallPaths...)
	}
	if hints.MissingRuntimeBlock || hints.InstallProseDetected {
		p.Line("Next: ask your agent to curate this skill so it proposes a runtime block (required binaries, environment variables, and a setup command that installs into $SKILLHUB_STATE_DIR) for you to confirm.")
	}
}

// writeChangesSinceApproval summarizes what changed since a human last
// approved the skill, so the reviewer can focus on the difference. Scripts,
// runtime, and dependency changes are called out first.
func writeChangesSinceApproval(p *termui.Printer, trust app.ContentTrust) {
	changes := trust.ChangesSinceApproval
	switch {
	case changes == nil && slices.Contains(trust.ReasonCodes, skillruntime.ReasonContentReviewStale):
		p.Line("Approved before, but the approved content could not be found in Git history: review the full skill.")
		return
	case changes == nil:
		p.Line("Never approved: review the full skill.")
		return
	case !changes.Found:
		note := "Approved before, but the approved content could not be found in Git history"
		if changes.HistoryTruncated {
			note += " (history walk stopped at its limit)"
		}
		p.Line(note + ": review the full skill.")
		return
	}
	p.Line("Changes since the last approval (" + changes.Commit[:12] + "):")
	var alerts []string
	if changes.ScriptsChanged {
		alerts = append(alerts, "scripts changed")
	}
	if changes.RuntimeChanged {
		alerts = append(alerts, "runtime block changed")
	}
	if changes.DependenciesChanged {
		alerts = append(alerts, "dependency files changed")
	}
	if len(alerts) > 0 {
		p.Line("Attention: " + strings.Join(alerts, ", ") + ".")
	}
	for _, group := range []struct {
		label string
		paths []string
	}{{"Added", changes.Added}, {"Modified", changes.Modified}, {"Removed", changes.Removed}} {
		if len(group.paths) > 0 {
			p.Line(group.label + ":")
			p.Bullets(group.paths...)
		}
	}
	if !changes.HasChanges() {
		p.Line("No file or runtime differences were found.")
	}
	p.Line("For the full difference run:")
	p.Command(changes.DiffCommand)
}
