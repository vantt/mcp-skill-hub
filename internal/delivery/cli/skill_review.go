package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
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
