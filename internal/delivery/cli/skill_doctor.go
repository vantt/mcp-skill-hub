package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

// runSkillDoctor checks a skill's declared runtime requirements on this
// machine. Exit codes: 0 ready, 1 setup_required or unsupported_platform,
// 2 invalid request or unknown skill.
func runSkillDoctor(ctx context.Context, flags skillFlags, stdout, stderr io.Writer) int {
	service := app.SkillDoctorService{}
	finishTelemetry := startCommandTelemetry(flags.workspace, func(sink app.TelemetrySink) { service.Telemetry = sink })
	defer finishTelemetry()
	result, err := service.Run(ctx, flags.workspace, flags.id)
	if err != nil {
		var structured *app.Error
		switch {
		case errors.As(err, &structured):
			writeStructuredSkillError(stdout, stderr, flags.jsonOutput, structured)
			return 2
		case errors.Is(err, app.ErrDoctorSkillUnavailable):
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Run `skillhub skill list --state active`; doctor checks active skills whose files match the catalog. Run `skillhub skill review "+flags.id+"` for details.")
		}
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	code := doctorExitCode(result.State)
	if flags.jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			termui.New(stderr).Line(err.Error())
			return 1
		}
		return code
	}
	p := termui.New(stdout)
	writeSkillDoctor(p, result)
	if p.Err() != nil {
		return 1
	}
	return code
}

func doctorExitCode(state string) int {
	if state == skillruntime.StateReady {
		return 0
	}
	return 1
}

func writeSkillDoctor(p *termui.Printer, result app.SkillDoctorResult) {
	p.Heading("Skill doctor: " + result.SkillID)
	if result.Note != "" {
		p.Line("Note: " + result.Note)
	}
	for _, check := range result.Checks {
		line := fmt.Sprintf("%-4s  %s %s", doctorStatusLabel(check.Status), check.Kind, check.Name)
		if check.Detail != "" {
			line += "  " + check.Detail
		}
		// Raw keeps the column spacing; manifest-derived text is still
		// stripped of terminal controls.
		p.Raw(stripTerminalControls(line) + "\n")
	}
	p.Blank()
	p.Line("State: " + result.State)
	if result.Basis != "" {
		p.Line("Basis: " + result.Basis + " (this shell; an agent may run in a different environment, so its own check decides)")
	}
	if len(result.ReasonCodes) > 0 {
		p.Line("Reasons: " + strings.Join(result.ReasonCodes, ", "))
	}
	if result.CheckOutput != "" {
		p.Blank()
		p.Line("Check output (last 2 KiB, not stored):")
		output := stripTerminalControls(result.CheckOutput)
		p.Raw(output)
		if !strings.HasSuffix(output, "\n") {
			p.Raw("\n")
		}
	}
	if result.State == skillruntime.StateSetupRequired {
		if result.SetupCommand != "" {
			p.Blank()
			p.Line("Suggested setup (not run; review it, then run it in " + result.WorkingDirectory + "):")
			p.Command(result.SetupCommand)
		}
		for _, action := range result.SuggestedActions {
			if action.Command != result.SetupCommand {
				p.Next(action.Label, action.Command)
			}
		}
	}
}

// stripTerminalControls removes control characters other than newline and tab
// so a skill's check output cannot drive the user's terminal.
func stripTerminalControls(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, text)
}

func doctorStatusLabel(status string) string {
	switch status {
	case skillruntime.StatusPass:
		return "PASS"
	case skillruntime.StatusSkipped:
		return "SKIP"
	}
	return "FAIL"
}
