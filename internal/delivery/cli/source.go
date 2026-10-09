package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

type sourceFlags struct {
	workspace, reason, status, decision, sourceID, adapter, ref, sourcePath, license, trust, cadence, skillID, newSkillID string
	proposalID, proposalDigest, baseVersion, idempotencyKey                                                               string
	jsonOutput, monitoring, yes, all                                                                                      bool
	skills                                                                                                                []string
}

func runSource(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "source requires a subcommand", "Run `skillhub source watch|check|capture|list|show|triage|confirm|import|attach|detach|unwatch|backfill`.")
	}
	sub := args[0]
	if sub == "check" {
		return runCheck(ctx, args[1:], stdout, stderr)
	}
	switch sub {
	case "watch", "capture", "list", "show", "triage", "confirm", "import", "attach", "detach", "unwatch", "backfill":
	default:
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), fmt.Sprintf("unsupported source subcommand %q", sub), "Run `skillhub source watch|check|capture|list|show|triage|confirm|import|attach|detach|unwatch|backfill`.")
	}
	flags, positionals, err := parseSourceFlags(args[1:])
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Review source command arguments and retry.")
	}
	switch sub {
	case "watch":
		if len(flags.skills) > 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--skill"`, "watch does not take --skill.")
		}
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "watch requires one locator", "Run `skillhub source watch <locator> [--id <id>] [--ref <ref>] [--path <path>] [--cadence daily|weekly|manual] [--yes]`.")
		}
	case "capture":
		if flags.yes {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--yes"`, "capture applies immediately without --yes.")
		}
		if len(flags.skills) > 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--skill"`, "capture does not take --skill.")
		}
		if len(positionals) != 1 || flags.reason == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "capture requires one locator and --reason", "Run `skillhub source capture <locator> --reason <text> --workspace <path>`.")
		}
	case "list":
		if flags.yes {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--yes"`, "list does not take --yes.")
		}
		if len(flags.skills) > 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--skill"`, "list does not take --skill.")
		}
		if len(positionals) != 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "list accepts no positional arguments", "Remove extra arguments.")
		}
	case "show":
		if flags.yes {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--yes"`, "show does not take --yes.")
		}
		if len(flags.skills) > 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--skill"`, "show does not take --skill.")
		}
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "show requires one candidate or source ID", "Provide exactly one ID.")
		}
	case "triage":
		if flags.yes {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--yes"`, "triage does not take --yes.")
		}
		if len(flags.skills) > 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--skill"`, "triage does not take --skill.")
		}
		if len(positionals) != 1 || flags.decision == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "triage requires one candidate ID and --decision", "Use accept, defer, reject, or import.")
		}
	case "confirm":
		if flags.yes {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--yes"`, "confirm does not take --yes.")
		}
		if len(flags.skills) > 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--skill"`, "confirm does not take --skill.")
		}
		if len(positionals) != 0 || flags.proposalID == "" || flags.proposalDigest == "" || flags.baseVersion == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "confirm requires --proposal, --proposal-digest, and --base-version", "Pass the exact pins printed by triage.")
		}
	case "import":
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "import requires one source ID", "Run `skillhub source import <source-id> [--path <subdir>] [--skill <name>]... [--yes]`.")
		}
	case "attach":
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "attach requires one source locator or source ID", "Run `skillhub source attach <source-id|locator> --skill-id <id> [--ref <r>] [--path <p>] [--cadence <c>] [--yes]`.")
		}
		if flags.skillID == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "attach requires --skill-id", "Provide --skill-id <id>.")
		}
	case "detach":
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "detach requires one source ID", "Run `skillhub source detach <source-id> --skill-id <id> [--yes]`.")
		}
		if flags.skillID == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "detach requires --skill-id", "Provide --skill-id <id>.")
		}
	case "unwatch":
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "unwatch requires one source ID", "Run `skillhub source unwatch <source-id> [--yes]`.")
		}
	case "backfill":
		if len(positionals) != 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "backfill accepts no positional arguments", "Run `skillhub source backfill [--skill <id> [--path <repo-path>]] [--yes]`.")
		}
	}
	resolved, resErr := resolveWorkspace(flags.workspace)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, flags.jsonOutput, resErr)
	}
	flags.workspace = resolved
	service := app.SourceService{}
	switch sub {
	case "watch":
		locator := ""
		if len(positionals) > 0 {
			locator = positionals[0]
		}
		return runSourceWatch(ctx, service, flags, locator, stdout, stderr)
	case "capture":
		if len(positionals) != 1 || flags.reason == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "capture requires one locator and --reason", "Run `skillhub source capture <locator> --reason <text> --workspace <path>`.")
		}
		finishTelemetry := startCommandTelemetry(flags.workspace, func(sink app.TelemetrySink) { service.Telemetry = sink })
		defer finishTelemetry()
		result, err := service.CaptureSourceCandidate(ctx, flags.workspace, app.SourceCandidateInput{Locator: positionals[0], Reason: flags.reason, IdempotencyKey: flags.idempotencyKey})
		return writeSourceValue(stdout, stderr, flags.jsonOutput, result, err)
	case "list":
		if len(positionals) != 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "list accepts no positional arguments", "Remove extra arguments.")
		}
		result, err := service.ListSourceGroups(ctx, flags.workspace)
		return writeSourceList(stdout, stderr, flags.jsonOutput, result, err)
	case "show":
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "show requires one candidate or source ID", "Provide exactly one ID.")
		}
		result, err := service.ListSources(ctx, flags.workspace, "")
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		for _, candidate := range result.Candidates {
			if candidate.ID == positionals[0] {
				return writeSingleCandidate(stdout, stderr, flags.jsonOutput, candidate)
			}
		}
		for _, item := range result.Sources {
			if item.Record.ID == positionals[0] {
				return writeSingleSource(stdout, stderr, flags.jsonOutput, item)
			}
		}
		return writeSourceError(stdout, stderr, flags.jsonOutput, errors.New("source record not found"))
	case "triage":
		if len(positionals) != 1 || flags.decision == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "triage requires one candidate ID and --decision", "Use accept, defer, or reject.")
		}
		finishTelemetry := startCommandTelemetry(flags.workspace, func(sink app.TelemetrySink) { service.Telemetry = sink })
		defer finishTelemetry()
		proposal, result, err := service.TriageSourceCandidate(ctx, flags.workspace, app.SourceTriageInput{CandidateID: positionals[0], Decision: flags.decision, DecisionReason: flags.reason, SourceID: flags.sourceID, Adapter: flags.adapter, Ref: flags.ref, SourcePath: flags.sourcePath, License: flags.license, Trust: flags.trust, Cadence: flags.cadence, SkillID: flags.skillID, NewSkillID: flags.newSkillID, MonitoringEnabled: flags.monitoring, IdempotencyKey: flags.idempotencyKey})
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		if flags.decision == "accept" {
			return writeSourceProposal(stdout, stderr, flags.jsonOutput, proposal)
		}
		return writeSourceMutation(stdout, stderr, flags.jsonOutput, result)
	case "confirm":
		if len(positionals) != 0 || flags.proposalID == "" || flags.proposalDigest == "" || flags.baseVersion == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "confirm requires --proposal, --proposal-digest, and --base-version", "Pass the exact pins printed by triage.")
		}
		finishTelemetry := startCommandTelemetry(flags.workspace, func(sink app.TelemetrySink) { service.Telemetry = sink })
		defer finishTelemetry()
		preview, err := service.LoadSourceProposal(ctx, flags.workspace, flags.proposalID)
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		result, err := service.ConfirmSourceProposal(ctx, flags.workspace, preview, app.ConfirmationPins{ProposalID: flags.proposalID, ProposalDigest: flags.proposalDigest, BaseVersion: flags.baseVersion})
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		return writeSourceMutation(stdout, stderr, flags.jsonOutput, result)
	case "import":
		sourceID := positionals[0]
		importService := app.SourceImportService{}
		if flags.proposalID != "" {
			if flags.proposalDigest == "" || flags.baseVersion == "" {
				return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "confirming import requires --proposal, --proposal-digest, and --base-version", "Pass all three pins or use --yes.")
			}
			preview, err := importService.LoadSourceImportProposal(ctx, flags.workspace, flags.proposalID)
			if err != nil {
				return writeSourceError(stdout, stderr, flags.jsonOutput, err)
			}
			result, err := importService.ConfirmSourceImport(ctx, flags.workspace, preview, app.ConfirmationPins{
				ProposalID:     flags.proposalID,
				ProposalDigest: flags.proposalDigest,
				BaseVersion:    flags.baseVersion,
			})
			return writeSourceImportResult(stdout, stderr, flags.jsonOutput, result, err)
		}
		preview, err := importService.PreviewSourceImport(ctx, flags.workspace, app.SourceImportPreviewInput{
			SourceID:       sourceID,
			Path:           flags.sourcePath,
			Skills:         flags.skills,
			IdempotencyKey: flags.idempotencyKey,
		})
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		if flags.yes {
			if len(preview.Importable) == 0 {
				if flags.jsonOutput {
					return writeSourceJSON(stdout, stderr, preview)
				}
				p := termui.New(stdout)
				p.Line(preview.Summary)
				var bullets []string
				for _, sk := range preview.Skipped {
					bullets = append(bullets, fmt.Sprintf("skipped %s: %s", sk.TargetID, sk.SkipReason))
				}
				p.Bullets(bullets...)
				return 0
			}
			result, err := importService.ConfirmSourceImport(ctx, flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
			return writeSourceImportResult(stdout, stderr, flags.jsonOutput, result, err)
		}
		return writeSourceImportProposal(stdout, stderr, flags.jsonOutput, preview, sourceID)
	case "attach":
		target := positionals[0]
		input := app.SourceAttachInput{
			SkillID:           flags.skillID,
			SourceID:          target,
			Locator:           target,
			Ref:               flags.ref,
			Path:              flags.sourcePath,
			Cadence:           flags.cadence,
			MonitoringEnabled: &flags.monitoring,
			Trust:             flags.trust,
			License:           flags.license,
			IdempotencyKey:    flags.idempotencyKey,
		}
		if !strings.Contains(target, "://") && !strings.HasPrefix(target, "git@") && !strings.Contains(target, "/") {
			input.Locator = ""
		} else {
			input.SourceID = ""
		}
		preview, err := service.PreviewAttach(ctx, flags.workspace, input)
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		if flags.yes {
			result, confirmErr := service.ConfirmSourceProposal(ctx, flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
			if confirmErr != nil {
				return writeSourceError(stdout, stderr, flags.jsonOutput, confirmErr)
			}
			return writeSourceMutation(stdout, stderr, flags.jsonOutput, result)
		}
		return writeSourceProposal(stdout, stderr, flags.jsonOutput, preview)
	case "detach":
		preview, err := service.PreviewDetach(ctx, flags.workspace, flags.skillID, positionals[0])
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		if flags.yes {
			result, confirmErr := service.ConfirmSourceProposal(ctx, flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
			if confirmErr != nil {
				return writeSourceError(stdout, stderr, flags.jsonOutput, confirmErr)
			}
			return writeSourceMutation(stdout, stderr, flags.jsonOutput, result)
		}
		return writeSourceProposal(stdout, stderr, flags.jsonOutput, preview)
	case "unwatch":
		preview, err := service.PreviewUnwatch(ctx, flags.workspace, positionals[0])
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		if flags.yes {
			result, confirmErr := service.ConfirmSourceProposal(ctx, flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
			if confirmErr != nil {
				return writeSourceError(stdout, stderr, flags.jsonOutput, confirmErr)
			}
			return writeSourceMutation(stdout, stderr, flags.jsonOutput, result)
		}
		return writeSourceProposal(stdout, stderr, flags.jsonOutput, preview)
	case "backfill":
		input := app.BackfillInput{
			SkillID:  flags.skillID,
			RepoPath: flags.sourcePath,
		}
		preview, err := service.PreviewBackfill(ctx, flags.workspace, input)
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		if len(preview.Candidates) == 0 {
			if flags.jsonOutput {
				return writeResult(stdout, stderr, true, preview, nil)
			}
			p := termui.New(stdout)
			p.Line("Nothing to backfill.")
			return 0
		}
		if flags.yes {
			result, applyErr := service.ApplyBackfill(ctx, flags.workspace, preview)
			if applyErr != nil {
				return writeSourceError(stdout, stderr, flags.jsonOutput, applyErr)
			}
			return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) {
				p.Line(result.Summary)
			})
		}
		if flags.jsonOutput {
			return writeResult(stdout, stderr, true, preview, nil)
		}
		p := termui.New(stdout)
		p.Line(preview.Summary)
		p.Blank()
		headers := []string{"SKILL", "ACTION", "SOURCE", "PATH", "REASON"}
		var rows [][]string
		for _, c := range preview.Candidates {
			action := "link existing source"
			reason := "vendored skill missing source reference"
			if c.CreateSource {
				action = "create and link source"
			}
			if c.Kind == app.BackfillKindSourceWithoutOrigin {
				action = "populate origin"
				reason = "imported skill missing origin block"
			}
			pathStr := c.RepoPath
			if pathStr == "" {
				pathStr = "-"
			}
			rows = append(rows, []string{
				printableText(c.SkillID),
				action,
				printableText(c.SourceID),
				printableText(pathStr),
				reason,
			})
		}
		p.Table(headers, rows)
		p.Blank()
		p.Line("Next: skillhub source backfill --yes")
		return 0
	default:
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, fmt.Sprintf("unsupported source subcommand %q", sub), "Run `skillhub source capture|list|show|triage|confirm`.")
	}
}

func parseSourceFlags(args []string) (sourceFlags, []string, error) {
	flags := sourceFlags{monitoring: true}
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		value := args[i]
		next := func() (string, error) {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", fmt.Errorf("%s requires a value", value)
			}
			i++
			return args[i], nil
		}
		switch value {
		case "--workspace", "--reason", "--status", "--decision", "--source-id", "--id", "--adapter", "--ref", "--path", "--license", "--trust", "--cadence", "--skill-id", "--new-skill", "--proposal", "--proposal-digest", "--base-version", "--idempotency-key":
			item, err := next()
			if err != nil {
				return flags, nil, err
			}
			switch value {
			case "--workspace":
				flags.workspace = item
			case "--reason":
				flags.reason = item
			case "--status":
				flags.status = item
			case "--decision":
				flags.decision = item
			case "--source-id", "--id":
				flags.sourceID = item
			case "--adapter":
				flags.adapter = item
			case "--ref":
				flags.ref = item
			case "--path":
				flags.sourcePath = item
			case "--license":
				flags.license = item
			case "--trust":
				flags.trust = item
			case "--cadence":
				flags.cadence = item
			case "--skill-id":
				flags.skillID = item
			case "--new-skill":
				flags.newSkillID = item
			case "--proposal":
				flags.proposalID = item
			case "--proposal-digest":
				flags.proposalDigest = item
			case "--base-version":
				flags.baseVersion = item
			case "--idempotency-key":
				flags.idempotencyKey = item
			}
		case "--skill":
			item, err := next()
			if err != nil {
				return flags, nil, err
			}
			flags.skills = append(flags.skills, item)
			if flags.skillID == "" {
				flags.skillID = item
			}
		case "--yes":
			flags.yes = true
		case "--all":
			flags.all = true
		case "--json":
			flags.jsonOutput = true
		case "--no-monitor":
			flags.monitoring = false
		default:
			if strings.HasPrefix(value, "-") {
				return flags, nil, fmt.Errorf("unknown argument %q", value)
			}
			positionals = append(positionals, value)
		}
	}
	return flags, positionals, nil
}

func runCheck(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	path := ""
	jsonOutput := false
	allDue := false
	ids := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--workspace":
			if i+1 >= len(args) {
				return writeInvalidRequest(stdout, stderr, jsonOutput, "--workspace requires a path", "Provide a workspace path.")
			}
			i++
			path = args[i]
		case "--json":
			jsonOutput = true
		case "--all-due":
			allDue = true
		case "--all":
		default:
			if strings.HasPrefix(args[i], "-") {
				return writeInvalidRequest(stdout, stderr, jsonOutput, "unknown check argument "+args[i], "Review check arguments.")
			}
			ids = append(ids, args[i])
		}
	}
	resolved, resErr := resolveWorkspace(path)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, jsonOutput, resErr)
	}
	path = resolved
	service := app.SourceService{}
	finishTelemetry := startCommandTelemetry(path, func(sink app.TelemetrySink) { service.Telemetry = sink })
	defer finishTelemetry()
	result, err := service.CheckSources(ctx, path, ids, allDue)
	return writeSourceValue(stdout, stderr, jsonOutput, result, err)
}

func writeSourceValue(stdout, stderr io.Writer, jsonOutput bool, value any, err error) int {
	if err != nil {
		return writeSourceError(stdout, stderr, jsonOutput, err)
	}
	switch result := value.(type) {
	case app.SourceCandidateResult:
		return writeResult(stdout, stderr, jsonOutput, result, func(p *termui.Printer) {
			p.Line(result.Summary)
			p.Fields(termui.Field{Label: "Candidate", Value: result.Candidate.ID})
		})
	case app.SourceCheckResult:
		if result.Error != nil {
			if jsonOutput {
				if err := writeJSON(stdout, result); err != nil {
					p := termui.New(stderr)
					p.Line(err.Error())
					return 1
				}
				return 2
			}
			p := termui.New(stderr)
			p.Error(result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
			return 2
		}
		if jsonOutput {
			if err := writeJSON(stdout, value); err != nil {
				p := termui.New(stderr)
				p.Line(err.Error())
				return 1
			}
			if result.Status == app.StatusError {
				return 2
			}
			return 0
		}
		p := termui.New(stdout)
		p.Line(result.Summary)
		for _, item := range result.Results {
			p.Bullets(fmt.Sprintf("%s: %s", item.SourceID, item.Status))
			for _, sk := range item.Skills {
				p.Raw(fmt.Sprintf("    %s: %s\n", printableText(sk.SkillID), formatStatusWords(sk.Status)))
			}
		}
		for _, warning := range result.Warnings {
			p.Warning(warning.Summary)
		}
		if result.Status == app.StatusError {
			return 2
		}
		return 0
	}
	if jsonOutput {
		return writeSourceJSON(stdout, stderr, value)
	}
	return 0
}

func writeSourceList(stdout, stderr io.Writer, jsonOutput bool, result app.SourceListResult, err error) int {
	if err != nil {
		return writeSourceError(stdout, stderr, jsonOutput, err)
	}
	return writeResult(stdout, stderr, jsonOutput, result, func(p *termui.Printer) {
		if len(result.Groups) > 0 {
			now := time.Now().UTC()
			numSources := len(result.Sources)
			numRepos := len(result.Groups)
			numCandidates := len(result.Candidates)

			p.Line(fmt.Sprintf("%s from %s; %s in intake.",
				termui.Plural(numSources, "source", "sources"),
				termui.Plural(numRepos, "repository", "repositories"),
				termui.Plural(numCandidates, "candidate", "candidates"),
			))

			hasOrphan := false
			orphanID := ""
			for _, group := range result.Groups {
				p.Blank()
				p.Line(group.Repository)
				headers := []string{"  SOURCE", "REF", "ROLE", "SKILLS", "CHECKED", "WATCH"}
				var rows [][]string
				for _, s := range group.Sources {
					skillsStr := "no skills"
					if len(s.ReferencingSkills) > 0 {
						var skillParts []string
						for _, sk := range s.ReferencingSkills {
							skillParts = append(skillParts, printableText(sk))
						}
						joined := strings.Join(skillParts, ", ")
						if utf8.RuneCountInString(joined) > 40 {
							joined = string([]rune(joined)[:39]) + "…"
						}
						skillsStr = joined
					} else {
						hasOrphan = true
						if orphanID == "" {
							orphanID = s.ID
						}
					}

					checkedStr := "never"
					if s.LastCheckedAt != nil {
						checkedStr = relativeTime(*s.LastCheckedAt, now)
					}

					refStr := "main"
					watchStr := "weekly"
					for _, srcItem := range result.Sources {
						if srcItem.Record.ID == s.ID {
							if srcItem.Record.Locator.Ref != "" {
								refStr = srcItem.Record.Locator.Ref
							}
							if srcItem.Record.Monitoring.Cadence != "" {
								watchStr = srcItem.Record.Monitoring.Cadence
							}
							break
						}
					}

					rows = append(rows, []string{
						"  " + printableText(s.ID),
						printableText(refStr),
						printableText(s.Role),
						skillsStr,
						checkedStr,
						printableText(watchStr),
					})
				}
				p.Table(headers, rows)
			}

			if len(result.Candidates) > 0 {
				p.Blank()
				for _, cand := range result.Candidates {
					p.Line(fmt.Sprintf("Intake: %s %s (%s)", cand.ID, cand.Locator, cand.Status))
				}
			}

			p.Blank()
			if hasOrphan && orphanID != "" {
				p.Line(fmt.Sprintf("Next: skillhub source attach %s --skill-id <skill> or skillhub source unwatch %s", orphanID, orphanID))
			} else {
				p.Line("Next: skillhub skill outdated")
			}
			return
		}

		p.Line(result.Summary)
		var bullets []string
		for _, item := range result.Candidates {
			bullets = append(bullets, fmt.Sprintf("%s [%s] %s — %s", item.ID, item.Status, item.Locator, item.Reason))
		}
		for _, item := range result.Sources {
			bullets = append(bullets, fmt.Sprintf("%s [%s] %s (%s)", item.Record.ID, item.Record.Status, item.Record.Identity.Name, item.Record.Adapter))
		}
		p.Bullets(bullets...)
	})
}

func writeSourceProposal(stdout, stderr io.Writer, jsonOutput bool, result app.SourceProposal) int {
	if result.Error != nil {
		if jsonOutput {
			return writeSourceJSON(stdout, stderr, result)
		}
		p := termui.New(stderr)
		p.Error(result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
		return 2
	}
	if jsonOutput {
		return writeSourceJSON(stdout, stderr, result)
	}
	p := termui.New(stdout)
	for _, warning := range result.Warnings {
		p.Warning(warning.Summary)
	}
	pins := result.Confirmation.Confirmation.Pins
	p.Line(result.Summary)
	if result.Source.ID != "" && result.Source.CurrentRevision != nil {
		p.Line(fmt.Sprintf("Detected: %s; adapter %s; revision %s; license %s; trust %s; cadence %s.",
			result.Source.Identity.Name, result.Source.Adapter, result.Source.CurrentRevision.Value,
			firstText(result.Source.License, "unknown"), result.Source.Trust.Source, result.Source.Monitoring.Cadence))
		p.Fields(
			termui.Field{Label: "Proposal", Value: pins.ProposalID},
			termui.Field{Label: "Digest", Value: pins.ProposalDigest},
			termui.Field{Label: "Base version", Value: pins.BaseVersion},
		)
	} else if pins.ProposalID != "" {
		p.Fields(
			termui.Field{Label: "Proposal", Value: pins.ProposalID},
			termui.Field{Label: "Digest", Value: pins.ProposalDigest},
			termui.Field{Label: "Base version", Value: pins.BaseVersion},
		)
	}
	p.Line("No files changed.")
	if result.Confirmation.ApplicationCommand == "PreviewSkillAdd" || result.Confirmation.ApplicationCommand == "SkillAdd" {
		p.Line(fmt.Sprintf("Next: skillhub skill confirm %s", pins.ProposalID))
	} else {
		p.Line(fmt.Sprintf("Confirm with:\n  skillhub source confirm --proposal %s --proposal-digest %s --base-version %s", pins.ProposalID, pins.ProposalDigest, pins.BaseVersion))
	}
	return 0
}

func writeSingleCandidate(stdout, stderr io.Writer, jsonOutput bool, candidate sourcepkg.Candidate) int {
	if jsonOutput {
		return writeSourceJSON(stdout, stderr, candidate)
	}
	p := termui.New(stdout)
	p.Bullets(fmt.Sprintf("%s [%s] %s — %s", candidate.ID, candidate.Status, candidate.Locator, candidate.Reason))
	return 0
}

func writeSingleSource(stdout, stderr io.Writer, jsonOutput bool, item app.SourceListItem) int {
	if jsonOutput {
		return writeSourceJSON(stdout, stderr, item.Record)
	}
	p := termui.New(stdout)
	p.Line(fmt.Sprintf("- %s [%s] %s (%s)", item.Record.ID, item.Record.Status, item.Record.Identity.Name, item.Record.Adapter))
	p.Line(fmt.Sprintf("  Role: %s", item.Role))
	if len(item.Skills) > 0 {
		p.Line(fmt.Sprintf("  Skills: %s", strings.Join(item.Skills, ", ")))
	} else {
		p.Line("  Skills: (none)")
	}
	if item.Record.CurrentRevision != nil && !item.Record.CurrentRevision.ObservedAt.IsZero() {
		p.Line(fmt.Sprintf("  Last check: %s", relativeTime(item.Record.CurrentRevision.ObservedAt, time.Now().UTC())))
	} else {
		p.Line("  Last check: never")
	}
	return 0
}

func writeSourceImportProposal(stdout, stderr io.Writer, jsonOutput bool, proposal app.SourceImportProposal, sourceID string) int {
	if jsonOutput {
		return writeSourceJSON(stdout, stderr, proposal)
	}
	p := termui.New(stdout)
	p.Line(proposal.Summary)
	var bullets []string
	for _, item := range proposal.Importable {
		bullets = append(bullets, fmt.Sprintf("%s -> %s [draft] (importable)", item.Name, item.TargetID))
	}
	for _, item := range proposal.Skipped {
		bullets = append(bullets, fmt.Sprintf("%s -> %s [skipped: %s]", item.Name, item.TargetID, item.SkipReason))
	}
	p.Bullets(bullets...)
	for _, warn := range proposal.Warnings {
		p.Warning(warn.Summary)
	}
	p.Blank()
	pins := proposal.Confirmation.Confirmation.Pins
	p.Fields(
		termui.Field{Label: "Proposal", Value: pins.ProposalID},
		termui.Field{Label: "Digest", Value: pins.ProposalDigest},
		termui.Field{Label: "Base version", Value: pins.BaseVersion},
	)
	p.Line(fmt.Sprintf("Confirm with:\n  skillhub source import %s --proposal %s --proposal-digest %s --base-version %s\nor re-run with --yes to import directly.", sourceID, pins.ProposalID, pins.ProposalDigest, pins.BaseVersion))
	return 0
}

func writeSourceImportResult(stdout, stderr io.Writer, jsonOutput bool, result app.SourceImportResult, err error) int {
	if err != nil {
		return writeSourceError(stdout, stderr, jsonOutput, err)
	}
	return writeResult(stdout, stderr, jsonOutput, result, func(p *termui.Printer) {
		p.Line(result.Summary)
		if len(result.SkippedIDs) > 0 {
			p.Line(fmt.Sprintf("Skipped %d existing skill(s): %s", len(result.SkippedIDs), strings.Join(result.SkippedIDs, ", ")))
		}
	})
}

func writeSourceMutation(stdout, stderr io.Writer, jsonOutput bool, result app.SourceMutationResult) int {
	return writeResult(stdout, stderr, jsonOutput, result, func(p *termui.Printer) {
		if result.SourceID != "" {
			p.Line(fmt.Sprintf("Watching %s. Distill with distill-lab to write .meta/distill.yaml; porting lessons to skill content uses skill_update.\nWatching does not auto-import skills.", result.SourceID))
			return
		}
		p.Line(result.Summary)
		p.Fields(
			termui.Field{Label: "Operation", Value: result.OperationID},
			termui.Field{Label: "Catalog snapshot", Value: result.CatalogSnapshot},
			termui.Field{Label: "Git dirty", Value: fmt.Sprintf("%t", result.GitDirty)},
		)
	})
}

func writeSourceJSON(stdout, stderr io.Writer, value any) int {
	if err := writeJSON(stdout, value); err != nil {
		p := termui.New(stderr)
		p.Line(err.Error())
		return 1
	}
	return 0
}

func writeSourceError(stdout, stderr io.Writer, jsonOutput bool, err error) int {
	var limitErr *sourcepkg.LimitExceededError
	if errors.As(err, &limitErr) {
		why := fmt.Sprintf("Source resource limit exceeded: %s (%d > %d).", limitErr.Limit, limitErr.Actual, limitErr.Max)
		fix := "Scope the source using `skillhub source triage <candidate-id> --decision accept --path <subdir>`."
		return writeInvalidRequest(stdout, stderr, jsonOutput, why, fix)
	}
	why := err.Error()
	if strings.Contains(why, "statat") && strings.Contains(why, "no such file or directory") {
		why = "The requested file or record was not found."
	}
	return writeInvalidRequest(stdout, stderr, jsonOutput, why, "Correct the source locator, policy, or workspace state and retry.")
}
func firstText(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
