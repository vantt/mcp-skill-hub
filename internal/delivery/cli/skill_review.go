package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

// writeSkillReview formats the comprehensive offline review report.
func writeSkillReview(stdout, stderr io.Writer, jsonOutput, verbose bool, result app.SkillReviewResult) int {
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(stdout, "Review: %s (%s)\n", result.Name, result.SkillID)
	fmt.Fprintf(stdout, "Collection: %s\n", result.Collection)
	if result.Description != "" {
		fmt.Fprintf(stdout, "Description: %s\n", result.Description)
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

	fmt.Fprintf(stdout, "State: %s (%s; routing: %s; %s)\n",
		result.LifecycleState, validStr, eligibleStr, servableStr)

	// Files and resource summary
	fmt.Fprintf(stdout, "Files: %d file(s), %d bytes (entrypoint: %s)\n",
		result.ResourceStatus.ResourceCount,
		result.ResourceStatus.TotalBytes,
		result.ResourceStatus.EntrypointPath)

	// Origin and provenance
	if result.Provenance != nil && result.Provenance.SourceLocator != "" {
		rev := result.Provenance.SourceRevision
		if len(rev) > 12 {
			rev = rev[:12]
		}
		if rev != "" {
			fmt.Fprintf(stdout, "Origin: %s at %s\n",
				result.Provenance.SourceLocator, rev)
		} else {
			fmt.Fprintf(stdout, "Origin: %s\n",
				result.Provenance.SourceLocator)
		}
	} else {
		fmt.Fprintln(stdout, "Origin: local authoring (not watched)")
	}

	// Git status
	if result.Git.Configured {
		if result.Git.Dirty {
			stagedCount := len(result.Git.Staged)
			unstagedCount := len(result.Git.Unstaged)
			untrackedCount := len(result.Git.Untracked)
			fmt.Fprintf(stdout, "Git: uncommitted changes (%d staged, %d unstaged, %d untracked)\n",
				stagedCount, unstagedCount, untrackedCount)
		} else {
			fmt.Fprintln(stdout, "Git: clean")
		}
	} else {
		fmt.Fprintln(stdout, "Git: not configured")
	}

	// Canonical issues
	if len(result.CanonicalIssues) > 0 {
		fmt.Fprintln(stdout, "\nValidation issues:")
		for _, issue := range result.CanonicalIssues {
			fmt.Fprintf(stdout, "  - %s\n", issue)
		}
	}

	// Activation readiness
	if !result.ActivationReadiness.Ready {
		fmt.Fprintln(stdout, "\nActivation readiness:")
		if result.ActivationReadiness.UntouchedScaffold {
			fmt.Fprintln(stdout, "  - Untouched scaffold instructions must be replaced before activation.")
		}
		for _, missing := range result.ActivationReadiness.MissingFields {
			fmt.Fprintf(stdout, "  - Missing required field: %s\n", missing)
		}
		for _, warn := range result.ActivationReadiness.Warnings {
			fmt.Fprintf(stdout, "  - Warning: %s\n", warn)
		}
	}

	// Verbose facts
	if verbose {
		fmt.Fprintln(stdout, "\nCatalog facts:")
		fmt.Fprintf(stdout, "  Canonical entrypoint: %s (status: %s, valid: %t)\n",
			result.CanonicalFacts.EntrypointPath, result.CanonicalFacts.Status, result.CanonicalFacts.Valid)
		fmt.Fprintf(stdout, "  Served generation:    %s (servable: %t)\n",
			result.ServedFacts.Generation, result.ServedFacts.Servable)
		if len(result.ResourceStatus.Resources) > 0 {
			fmt.Fprintln(stdout, "Resources:")
			for _, res := range result.ResourceStatus.Resources {
				digestShort := res.Digest
				if len(digestShort) > 19 && strings.HasPrefix(digestShort, "sha256:") {
					digestShort = "sha256:" + digestShort[7:19]
				}
				fmt.Fprintf(stdout, "  - %s (%s, %d bytes)\n", res.Path, digestShort, res.SizeBytes)
			}
		}
	}

	// Next action
	next := result.NextAction
	if next == "" {
		if result.LifecycleState == "draft" {
			next = fmt.Sprintf("edit with `skillhub skill edit %s --editor`, or review instructions with `skillhub skill show %s`", result.SkillID, result.SkillID)
		} else {
			next = fmt.Sprintf("ask your agent to use it, or inspect with `skillhub skill show %s`", result.SkillID)
		}
	}
	fmt.Fprintf(stdout, "\nNext: %s\n", next)
	return 0
}
