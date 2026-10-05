package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
)

type UpstreamListResult struct {
	app.Result
	Skills []app.SkillUpstream `json:"skills"`
}

func runSkillOutdated(ctx context.Context, flags skillFlags, stdout, stderr io.Writer) int {
	if flags.check {
		checkTrackedSources(ctx, flags.workspace, stderr, flags.jsonOutput)
	}

	skills, err := app.ListSkillUpstream(ctx, flags.workspace)
	if err != nil {
		return writeSkillErrorFor("", stdout, stderr, flags.jsonOutput, err)
	}

	now := time.Now().UTC()

	var rows [][]string
	var attentionCount int
	var hasOutdatedOrDiverged bool
	var firstAttentionID string
	var allUnknown = true
	var latestChecked time.Time

	for _, sk := range skills {
		if sk.CheckedAt != "" {
			if t, err := time.Parse(time.RFC3339, sk.CheckedAt); err == nil {
				if t.After(latestChecked) {
					latestChecked = t
				}
			}
		}

		if sk.Status != "unknown" {
			allUnknown = false
		}

		isActionRequired := sk.Status == "update_available" || sk.Status == "diverged" || sk.Status == "upstream_removed"
		if isActionRequired {
			hasOutdatedOrDiverged = true
			if firstAttentionID == "" {
				firstAttentionID = sk.SkillID
			}
		}

		include := false
		if flags.all {
			include = true
		} else {
			switch sk.Status {
			case "update_available", "diverged", "upstream_removed", "unknown", "unavailable", "untracked":
				include = true
			}
		}

		if include {
			attentionCount++
			sourceStr := "-"
			if sk.SourceID != "" {
				if sk.Ref != "" {
					sourceStr = fmt.Sprintf("%s@%s", sk.SourceID, sk.Ref)
				} else {
					sourceStr = sk.SourceID
				}
			}

			rows = append(rows, []string{
				printableText(sk.SkillID),
				printableText(sourceStr),
				shortSHA(sk.BaseCommit),
				shortSHA(sk.LatestCommit),
				dateStr(sk.LatestCommittedAt),
				formatChangedFiles(sk.ChangedFiles),
				formatLocalStatus(sk.Local),
				formatStatusWords(sk.Status),
			})
		}
	}

	exitCode := 0
	if flags.exitCode && hasOutdatedOrDiverged {
		exitCode = 1
	}

	if flags.jsonOutput {
		result := UpstreamListResult{
			Result: app.NewResult(app.StatusOK, fmt.Sprintf("%d repository skill(s).", len(skills))),
			Skills: skills,
		}
		if err := writeJSON(stdout, result); err != nil {
			p := termui.New(stderr)
			p.Line(err.Error())
			return 1
		}
		return exitCode
	}

	p := termui.New(stdout)
	renderSkillOutdated(p, outdatedRenderContext{
		skills:           skills,
		rows:             rows,
		attentionCount:   attentionCount,
		allUnknown:       allUnknown,
		allFlag:          flags.all,
		latestChecked:    latestChecked,
		now:              now,
		firstAttentionID: firstAttentionID,
	})
	return exitCode
}

func checkTrackedSources(ctx context.Context, workspace string, stderr io.Writer, jsonOutput bool) {
	service := app.SourceService{}
	initialSkills, err := app.ListSkillUpstream(ctx, workspace)
	if err != nil {
		return
	}
	sourceSet := make(map[string]bool)
	var sourceIDs []string
	for _, sk := range initialSkills {
		if sk.SourceID != "" && !sourceSet[sk.SourceID] {
			sourceSet[sk.SourceID] = true
			sourceIDs = append(sourceIDs, sk.SourceID)
		}
	}
	sort.Strings(sourceIDs)
	if len(sourceIDs) == 0 {
		return
	}
	checkRes, chkErr := service.CheckSources(ctx, workspace, sourceIDs, false)
	if chkErr != nil && !jsonOutput {
		p := termui.New(stderr)
		p.Warning(fmt.Sprintf("Failed to check sources: %s", chkErr.Error()))
		return
	}
	if chkErr == nil && !jsonOutput {
		p := termui.New(stderr)
		for _, item := range checkRes.Results {
			if item.Status == "unavailable" || item.Error != "" {
				p.Warning(fmt.Sprintf("Source %s is unavailable: %s", item.SourceID, item.Error))
			}
		}
	}
}

type outdatedRenderContext struct {
	skills           []app.SkillUpstream
	rows             [][]string
	attentionCount   int
	allUnknown       bool
	allFlag          bool
	latestChecked    time.Time
	now              time.Time
	firstAttentionID string
}

func renderSkillOutdated(p *termui.Printer, rc outdatedRenderContext) {
	if len(rc.skills) == 0 {
		p.Line("No skills track an upstream repository.")
		p.Blank()
		p.Line("Next: skillhub skill add <github-url>")
		return
	}

	if len(rc.rows) == 0 {
		if !rc.latestChecked.IsZero() {
			p.Line(fmt.Sprintf("All %d repository skills are up to date. Checked %s.", len(rc.skills), relativeTime(rc.latestChecked, rc.now)))
		} else {
			p.Line(fmt.Sprintf("All %d repository skills are up to date.", len(rc.skills)))
		}
		return
	}

	if rc.allUnknown && !rc.allFlag {
		p.Line(fmt.Sprintf("%d repository skills have not been checked yet.", len(rc.rows)))
		p.Blank()
		headers := []string{"SKILL", "SOURCE", "CURRENT", "LATEST", "UPDATED", "CHANGED", "LOCAL", "STATUS"}
		p.Table(headers, rc.rows)
		p.Blank()
		p.Line("Next: skillhub skill outdated --check")
		return
	}

	timeStr := ""
	if !rc.latestChecked.IsZero() {
		timeStr = fmt.Sprintf(" Checked %s.", relativeTime(rc.latestChecked, rc.now))
	}
	p.Line(fmt.Sprintf("%d of %d repository skills need attention.%s", rc.attentionCount, len(rc.skills), timeStr))
	p.Blank()
	headers := []string{"SKILL", "SOURCE", "CURRENT", "LATEST", "UPDATED", "CHANGED", "LOCAL", "STATUS"}
	p.Table(headers, rc.rows)
	p.Blank()

	if rc.firstAttentionID != "" {
		p.Line(fmt.Sprintf("Next: skillhub skill update %s", rc.firstAttentionID))
	} else {
		p.Line("Next: skillhub skill outdated --check")
	}
}

func runSkillUpstream(ctx context.Context, flags skillFlags, stdout, stderr io.Writer) int {
	skillID := strings.TrimSpace(flags.id)
	if skillID == "" {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "upstream requires a skill ID", "Run `skillhub skill upstream <id>`.")
	}

	service := app.SourceService{}

	if flags.check {
		info, err := app.GetSkillUpstream(ctx, flags.workspace, skillID)
		if err == nil && info.SourceID != "" {
			_, _ = service.CheckSources(ctx, flags.workspace, []string{info.SourceID}, false)
		}
	}

	upstream, err := app.GetSkillUpstream(ctx, flags.workspace, skillID)
	if err != nil {
		return writeSkillErrorFor(skillID, stdout, stderr, flags.jsonOutput, err)
	}

	if flags.jsonOutput {
		return writeResult(stdout, stderr, true, upstream, nil)
	}

	p := termui.New(stdout)
	now := time.Now().UTC()

	refStr := upstream.Ref
	if refStr == "" {
		refStr = "main"
	}
	pathStr := upstream.Path
	if pathStr == "" {
		pathStr = "."
	}
	p.Line(fmt.Sprintf("%s tracks %s (%s@%s, %s).",
		printableText(upstream.SkillID),
		printableText(upstream.SourceID),
		printableText(upstream.Repository),
		printableText(refStr),
		printableText(pathStr),
	))

	localDesc := "local copy is clean"
	if upstream.Local == "modified" {
		localDesc = "local copy is modified"
	}
	p.Line(fmt.Sprintf("Status   %s (%s)", formatStatusWords(upstream.Status), localDesc))
	p.Line(fmt.Sprintf("Current  %s", short12(upstream.BaseCommit)))

	var checkedTimeStr string
	if upstream.CheckedAt != "" {
		if t, err := time.Parse(time.RFC3339, upstream.CheckedAt); err == nil {
			checkedTimeStr = fmt.Sprintf(" (checked %s)", relativeTime(t, now))
		}
	}
	committedDate := dateStr(upstream.LatestCommittedAt)
	if committedDate != "-" {
		p.Line(fmt.Sprintf("Latest   %s, committed %s%s", short12(upstream.LatestCommit), committedDate, checkedTimeStr))
	} else {
		p.Line(fmt.Sprintf("Latest   %s%s", short12(upstream.LatestCommit), checkedTimeStr))
	}

	if len(upstream.Files) > 0 {
		p.Line(fmt.Sprintf("Changed upstream (%d):", len(upstream.Files)))
		for _, f := range upstream.Files {
			p.Line(fmt.Sprintf("  %-8s  %s", printableText(f.Status), printableText(f.Path)))
		}
	}

	p.Blank()
	p.Line(fmt.Sprintf("Next: skillhub skill update %s", printableText(upstream.SkillID)))
	return 0
}

func runSkillUpdate(ctx context.Context, flags skillFlags, stdout, stderr io.Writer) int {
	skillID := strings.TrimSpace(flags.id)
	if skillID == "" {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
			"update requires a skill ID (e.g. `skillhub skill update <id>`)",
			"To update the skillhub binary itself, run `skillhub update`.")
	}

	service := app.SourceService{}

	if !flags.noCheck {
		info, err := app.GetSkillUpstream(ctx, flags.workspace, skillID)
		if err == nil && info.SourceID != "" {
			_, _ = service.CheckSources(ctx, flags.workspace, []string{info.SourceID}, false)
		}
	}

	acceptResolutions, err := parseAcceptResolutions(flags.accepts)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Allowed values are: upstream, local, merged.")
	}

	manualResolutions, err := parseManualFiles(flags.manuals)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Verify manual file path exists.")
	}

	var resolutions []app.UpstreamResolution
	for path, action := range acceptResolutions {
		resolutions = append(resolutions, app.UpstreamResolution{
			Path:   path,
			Action: action,
		})
	}
	for path, content := range manualResolutions {
		resolutions = append(resolutions, app.UpstreamResolution{
			Path:    path,
			Action:  "manual",
			Content: string(content),
		})
	}

	upstreamService := app.UpstreamService{}
	input := app.UpstreamUpdateInput{
		SkillID:        skillID,
		TargetCommit:   flags.target,
		Resolutions:    resolutions,
		IdempotencyKey: flags.idempotencyKey,
	}

	preview, err := upstreamService.PreviewUpdate(ctx, flags.workspace, input)
	if err != nil {
		return writeSkillErrorFor(skillID, stdout, stderr, flags.jsonOutput, err)
	}

	if flags.writeConflicts != "" {
		if writeErr := writeConflictsFiles(flags.writeConflicts, preview.Files); writeErr != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, writeErr.Error(), "Choose an empty or different directory for --write-conflicts.")
		}
	}

	if flags.yes {
		if len(preview.Unresolved) > 0 {
			msg := fmt.Sprintf("%d file(s) need a decision before this update can be applied.", len(preview.Unresolved))
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, msg, "Pass --accept or --manual to resolve conflicts.")
		}
		result, confirmErr := upstreamService.ConfirmUpdate(ctx, flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
		if confirmErr != nil {
			return writeSkillErrorFor(skillID, stdout, stderr, flags.jsonOutput, confirmErr)
		}
		return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) {
			p.Line(result.Summary)
			if result.TrustImpact.ReviewRequiredAfterApply {
				p.Line(fmt.Sprintf("Next: skillhub skill review %s", skillID))
			}
		})
	}

	if flags.jsonOutput {
		return writeResult(stdout, stderr, true, preview, nil)
	}

	p := termui.New(stdout)
	repoStr := ""
	if info, err := app.GetSkillUpstream(ctx, flags.workspace, skillID); err == nil {
		repoStr = info.Repository
	}
	renderUpstreamUpdatePreview(p, skillID, repoStr, preview, flags.verbose)
	return 0
}

func renderUpstreamUpdatePreview(p *termui.Printer, skillID string, repoURL string, preview app.UpstreamUpdatePreview, verbose bool) {
	commitDate := dateStr(preview.TargetCommittedAt)
	b7 := shortSHA(preview.BaseCommit)
	u7 := shortSHA(preview.TargetCommit)

	dateClause := ""
	if commitDate != "-" {
		dateClause = fmt.Sprintf(", upstream commit of %s", commitDate)
	}
	repoPart := ""
	if repoURL != "" {
		repoPart = fmt.Sprintf("%s, source %s", printableText(repoURL), printableText(preview.SourceID))
	} else {
		repoPart = fmt.Sprintf("source %s", printableText(preview.SourceID))
	}
	p.Line(fmt.Sprintf("Update %s from %s to %s (%s)%s.",
		printableText(skillID), b7, u7, repoPart, dateClause))
	p.Blank()

	headers := []string{"FILE", "CHANGE", "ACTION"}
	var rows [][]string
	for _, f := range preview.Files {
		if f.Status == "unchanged" {
			continue
		}
		rows = append(rows, []string{
			printableText(f.Path),
			formatChangeWord(f.Status),
			formatActionWord(f),
		})
	}
	p.Table(headers, rows)

	if preview.UnchangedCount > 0 {
		p.Line(fmt.Sprintf("%d file(s) unchanged.", preview.UnchangedCount))
	}
	p.Blank()

	if len(preview.Unresolved) > 0 {
		p.Line(fmt.Sprintf("%d file(s) need a decision before this update can be applied. For each, pass one of:", len(preview.Unresolved)))
		for _, unres := range preview.Unresolved {
			p.Raw(fmt.Sprintf("  --accept %s=upstream   take the upstream file (drops your edits in it)\n", unres))
			p.Raw(fmt.Sprintf("  --accept %s=local      keep your file\n", unres))
			p.Raw(fmt.Sprintf("  --manual %s=<file>     use a file you resolved yourself (no conflict markers)\n", unres))
		}
		p.Line(fmt.Sprintf("To edit the conflict: skillhub skill update %s --write-conflicts ./%s-conflicts", skillID, skillID))
		return
	}

	// Clean preview diffs
	for _, f := range preview.Files {
		if f.ResultDiff != "" {
			p.Raw(f.ResultDiff + "\n")
		}
		if verbose {
			if f.UpstreamDiff != "" {
				p.Line("Upstream Diff:")
				p.Raw(f.UpstreamDiff + "\n")
			}
			if f.LocalDiff != "" {
				p.Line("Local Diff:")
				p.Raw(f.LocalDiff + "\n")
			}
		}
	}

	if preview.TrustImpact.ReviewRequiredAfterApply {
		p.Line(fmt.Sprintf("After applying, agents cannot use %s until you approve the new content:", skillID))
		p.Raw(fmt.Sprintf("  skillhub skill review %s\n", skillID))
	}
	p.Line("No collection files changed.")
	pins := preview.Confirmation.Confirmation.Pins
	p.Line(fmt.Sprintf("Next: skillhub skill confirm %s", pins.ProposalID))
}

func writeConflictsFiles(dir string, files []app.UpstreamFile) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dirRoot, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer dirRoot.Close()

	for _, f := range files {
		if f.Conflicts > 0 || f.MergedWithMarkers != "" {
			cleanPath := filepath.Clean(f.Path)
			if _, statErr := dirRoot.Stat(cleanPath); statErr == nil {
				return fmt.Errorf("conflict file %q already exists in target directory", f.Path)
			}
			parent := filepath.Dir(cleanPath)
			if parent != "." && parent != "/" {
				if err := dirRoot.MkdirAll(parent, 0o755); err != nil {
					return err
				}
			}
			file, createErr := dirRoot.OpenFile(cleanPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if createErr != nil {
				return createErr
			}
			_, writeErr := file.WriteString(f.MergedWithMarkers)
			_ = file.Close()
			if writeErr != nil {
				return writeErr
			}
		}
	}
	return nil
}

func parseAcceptResolutions(accepts []string) (map[string]string, error) {
	result := make(map[string]string, len(accepts))
	for _, item := range accepts {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --accept flag %q: must be <path>=<choice>", item)
		}
		path := strings.TrimSpace(parts[0])
		choice := strings.TrimSpace(parts[1])
		switch choice {
		case "upstream", "local", "merged":
			result[path] = choice
		default:
			return nil, fmt.Errorf("--accept resolution %q is invalid: must be upstream, local, or merged", choice)
		}
	}
	return result, nil
}

func parseManualFiles(manuals []string) (map[string][]byte, error) {
	result := make(map[string][]byte, len(manuals))
	for _, item := range manuals {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --manual flag %q: must be <path>=<file>", item)
		}
		path := strings.TrimSpace(parts[0])
		filePath := strings.TrimSpace(parts[1])
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("manual file %q cannot be read: %w", filePath, err)
		}
		result[path] = data
	}
	return result, nil
}

func formatChangeWord(status string) string {
	switch status {
	case "upstream_only":
		return "changed upstream"
	case "local_only":
		return "changed here"
	case "both_changed":
		return "changed here and upstream"
	case "added_upstream":
		return "added upstream"
	case "added_local":
		return "added here"
	case "removed_upstream":
		return "removed upstream"
	case "removed_local":
		return "removed here"
	case "blocked":
		return "blocked (contains lines Skill Hub reads as conflict markers)"
	default:
		return printableText(status)
	}
}

func formatActionWord(f app.UpstreamFile) string {
	if f.Conflicts > 0 {
		return fmt.Sprintf("needs decision (%d conflict)", f.Conflicts)
	}
	if f.Status == "blocked" {
		return "needs decision (conflict markers)"
	}
	switch f.Action {
	case "upstream":
		return "take upstream"
	case "local":
		return "keep local"
	case "merged":
		return "merged"
	case "manual":
		return "use manual"
	default:
		if f.DefaultAction == "upstream" {
			return "take upstream"
		}
		if f.DefaultAction != "" {
			return f.DefaultAction
		}
		return "needs decision"
	}
}

func printableText(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if unicode.IsPrint(r) && r != '\t' && r != '\n' && r != '\r' {
			sb.WriteRune(r)
		} else {
			fmt.Fprintf(&sb, "\\x%02x", r)
		}
	}
	return sb.String()
}

func formatStatusWords(status string) string {
	switch status {
	case "update_available":
		return "update available"
	case "diverged":
		return "diverged"
	case "upstream_removed":
		return "removed upstream"
	case "modified":
		return "modified locally"
	case "up_to_date":
		return "up to date"
	case "pinned":
		return "pinned"
	case "unavailable":
		return "unreachable"
	case "unknown":
		return "not checked"
	case "untracked":
		return "not tracked"
	default:
		return printableText(status)
	}
}

func formatLocalStatus(local string) string {
	switch local {
	case "clean":
		return "clean"
	case "modified":
		return "modified"
	default:
		if local == "" {
			return "-"
		}
		return printableText(local)
	}
}

func formatChangedFiles(count int) string {
	if count > 1 {
		return fmt.Sprintf("%d files", count)
	}
	if count == 1 {
		return "1 file"
	}
	if count == 0 {
		return "-"
	}
	return "?"
}

func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	if sha == "" {
		return "-"
	}
	return sha
}

func short12(sha string) string {
	if len(sha) >= 12 {
		return sha[:12]
	}
	if sha == "" {
		return "-"
	}
	return sha
}

func dateStr(iso string) string {
	if len(iso) >= 10 {
		return iso[:10]
	}
	return "-"
}

func relativeTime(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	diff := now.Sub(t)
	if diff < 0 {
		diff = 0
	}
	secs := int(diff.Seconds())
	if secs < 60 {
		return "just now"
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%dm ago", mins)
	}
	hours := mins / 60
	if hours < 24 {
		return fmt.Sprintf("%dh ago", hours)
	}
	days := hours / 24
	if days == 1 {
		return "yesterday"
	}
	return fmt.Sprintf("%dd ago", days)
}
