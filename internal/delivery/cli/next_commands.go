package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

// commandOutput carries invocation context without process-global state. JSON
// decoration copies only the envelope and actions, never proposal payload bytes.
type commandOutput struct {
	io.Writer
	args []string
}

func (out commandOutput) workspace() string {
	explicit := ""
	for i, arg := range out.args {
		if arg == "--workspace" && i+1 < len(out.args) {
			explicit = out.args[i+1]
		}
	}
	if explicit != "" {
		root, _ := filepath.Abs(explicit)
		return root
	}
	if len(out.args) > 0 && (out.args[0] == "init" || out.args[0] == "version" || out.args[0] == "help") {
		return ""
	}
	root, _ := resolveWorkspace("")
	return root
}

func cliJSONValue(value any, workspace string) any {
	original := reflect.ValueOf(value)
	if original.Kind() == reflect.Pointer {
		if original.IsNil() {
			return value
		}
		original = original.Elem()
	}
	if original.Kind() != reflect.Struct {
		return value
	}
	resultField := original.FieldByName("Result")
	if original.Type() == reflect.TypeOf(app.Result{}) {
		resultField = original
	}
	if !resultField.IsValid() || resultField.Type() != reflect.TypeOf(app.Result{}) {
		return value
	}
	result := resultField.Interface().(app.Result)
	id := ""
	if f := original.FieldByName("SkillID"); f.IsValid() && f.Kind() == reflect.String {
		id = f.String()
	}
	if home, ok := value.(app.CurationHome); ok && len(home.Actions) > 0 {
		id = home.Actions[0].ID
	}
	if f := original.FieldByName("Confirmation"); f.IsValid() && f.Type() == reflect.TypeOf(app.ConfirmationPolicy{}) {
		policy := f.Interface().(app.ConfirmationPolicy)
		if policy.Confirmation.Required && policy.Confirmation.Pins.ProposalID != "" {
			command := "skillhub skill confirm"
			yes := " --yes"
			switch value.(type) {
			case app.SourceProposal:
				command = "skillhub source confirm"
				yes = "" // This command is itself explicit confirmation; it has no --yes flag.
			case app.SourceImportProposal:
				command = "skillhub source import"
			}
			pins := policy.Confirmation.Pins
			result.CLI = cliWorkspace(fmt.Sprintf("%s --proposal %s --proposal-digest %s --base-version %s%s", command, shellArgument(pins.ProposalID), shellArgument(pins.ProposalDigest), shellArgument(pins.BaseVersion), yes), workspace)
		}
	}
	if len(result.SuggestedActions) > 0 {
		result.SuggestedActions = append([]app.Action(nil), result.SuggestedActions...)
		for i := range result.SuggestedActions {
			action := &result.SuggestedActions[i]
			if result.CLI != "" && action.RequiresConfirmation {
				action.CLI = result.CLI
			} else {
				action.CLI = cliNextCommand(action.Command, id, workspace)
			}
		}
	}
	copied := reflect.New(original.Type()).Elem()
	copied.Set(original)
	if original.Type() == reflect.TypeOf(app.Result{}) {
		copied.Set(reflect.ValueOf(result))
	} else {
		copied.FieldByName("Result").Set(reflect.ValueOf(result))
	}
	// Immediate apply results can include the reviewed preview. Decorate its copy
	// too, without mutating the application response retained by other adapters.
	if f := copied.FieldByName("Proposal"); f.IsValid() && f.CanSet() && f.Kind() == reflect.Pointer && !f.IsNil() {
		nested := cliJSONValue(f.Interface(), workspace)
		p := reflect.New(f.Type().Elem())
		p.Elem().Set(reflect.ValueOf(nested))
		f.Set(p)
	}
	return copied.Interface()
}

func cliNextCommand(command, id, workspace string) string {
	if strings.HasPrefix(command, "skillhub init ") {
		path := strings.TrimSuffix(strings.TrimPrefix(command, "skillhub init "), " --yes")
		return "skillhub init " + shellArgument(path) + " --yes --json"
	}
	if strings.HasPrefix(command, "skillhub ") {
		if workspace != "" {
			command = strings.ReplaceAll(command, "--workspace "+workspace, "--workspace "+shellArgument(workspace))
		}
		return cliWorkspace(command, workspace)
	}
	target := ""
	if id != "" {
		target = " " + shellArgument(id)
	}
	var cli string
	switch command {
	case "workspace_validate", "ValidateWorkspace":
		cli = "skillhub validate"
	case "workspace_rebuild":
		cli = "skillhub rebuild"
	case "workspace_diff", "GetCurationDiff":
		cli = "skillhub diff"
	case "source_check":
		cli = "skillhub check"
	case "skill_upstream_status":
		cli = "skillhub skill outdated"
	case "skill_review":
		cli = "skillhub skill review" + target
	case "skill_list":
		cli = "skillhub skill list"
	case "skill_active":
		cli = "skillhub skill activate" + target
	case "skill_deprecated":
		cli = "skillhub skill deprecate" + target
	case "skill_archived":
		cli = "skillhub skill archive" + target
	case "skill_edit", "PreviewSkillUpdate":
		// Original edit inputs are unavailable after a stale confirmation.
		// Inspect the affected skill rather than advertise an invalid bare edit.
		cli = "skillhub skill review" + target
	case "skill_create":
		cli = "skillhub skill create" + target
	case "doctor":
		cli = "skillhub doctor"
	case "curation_run_start", "curation_run_retry":
		cli = "skillhub source list"
	default:
		if command != "" {
			return command
		} // Declared runtime setup is already a shell command.
		cli = "skillhub status"
	}
	if target == "" && (strings.HasPrefix(cli, "skillhub skill review") || strings.HasPrefix(cli, "skillhub skill edit") || strings.HasPrefix(cli, "skillhub skill create")) {
		cli = "skillhub skill list"
	}
	return cliWorkspace(cli, workspace)
}

func cliWorkspace(command, workspace string) string {
	if workspace != "" && !strings.Contains(command, " --workspace ") {
		command += " --workspace " + shellArgument(workspace)
	}
	if !strings.Contains(command, " --json") {
		command += " --json"
	}
	return command
}

func shellArgument(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\r\n'\"`$;&|<>*?(){}[]!\\") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
