package mcpserver

import (
	"slices"
	"strings"
)

// instructionOptions selects the paragraphs that depend on what the host and
// the hub offer beyond the registered tools.
type instructionOptions struct {
	// skillsExtension is set when the server advertises the MCP skills
	// extension.
	skillsExtension bool
	// curatorSkill is set when the native system-curator skill is installed
	// where the agent can load it.
	curatorSkill bool
}

// instructionParagraph is one piece of the server instructions. It is rendered
// only when every tool it names is registered and its flag, if any, holds.
type instructionParagraph struct {
	text     string
	requires []string
	flag     func(instructionOptions) bool
}

var runtimeToolNames = []string{"skill_resolve", "skill_feedback", "skill_get"}

var runtimeParagraphs = []instructionParagraph{
	{requires: []string{"skill_resolve"}, text: "Skill Hub picks the best skill in the user's hub for a task.\n"},
	{requires: []string{"skill_resolve"}, text: "- At the start of each new task, or when its operation, scope or constraints change, call `skill_resolve` before choosing a skill; skip trivial edits. Send `task.description` in English. Follow at most one primary procedure."},
	{requires: []string{"skill_resolve", "skill_feedback"}, text: "- If you do not use the recommended skill, call `skill_resolve` again with `prior.kind: rejected`. If you use another skill, send `skill_feedback` with its real `skill_id`."},
	{requires: []string{"skill_get"}, text: "- When a response has `local.path`, resolve the skill's relative files (such as `scripts/`) against it, and export `local.env` (`SKILLHUB_SKILL_DIR`, `SKILLHUB_STATE_DIR`, `SKILLHUB_CONFIG_DIR`) when you run its `check`, `setup` or scripts. If `local.path` is gone, call `skill_get` again."},
	{requires: []string{"skill_get"}, text: "- On `snapshot_expired` with `current_uri`, call `skill_get` again.", flag: func(options instructionOptions) bool { return !options.skillsExtension }},
	{requires: []string{"skill_get"}, text: "- On `snapshot_expired` with `current_uri`, call `skill_get` again. A host with the MCP skills extension reloads with `skills/get <current_uri>`.", flag: func(options instructionOptions) bool { return options.skillsExtension }},
	{requires: []string{"skill_resolve"}, text: "- If `$SKILLHUB_CONFIG_DIR/env` exists, load it before `check`, `setup` or scripts and never print its values. If a variable is missing, tell the user to run `skillhub skill env set <id> <NAME>`; never ask for it in chat."},
	{requires: []string{"skill_resolve"}, text: "- With `local.preflight`, run `check` in `working_directory` first and ask the user before `setup`. Treat install steps written in prose as `setup`: ask first, install into `SKILLHUB_STATE_DIR`, never globally. Do not run `setup` while `$SKILLHUB_STATE_DIR/.setup.lock` is recent."},
	{requires: []string{"skill_resolve"}, text: "- If `local.status` is `review_required`, do not use the skill; tell the user to run `skillhub skill review <id>`."},
	{requires: []string{"skill_feedback"}, text: "- If a script fails on a missing dependency, stop, tell the user, and send `skill_feedback` with `outcome: failed` and `reason_code: setup_failed`."},
	{requires: []string{"skill_resolve"}, text: "- Only when the user explicitly asks to manage, curate, check, distill, repair or inspect Skill Hub itself, load the native `system-curator` skill.", flag: func(options instructionOptions) bool { return options.curatorSkill }},
	{requires: []string{"skill_resolve"}, text: "- These are recommendations; your environment controls permissions and execution."},
}

// serverInstructions builds the text a server returns in its initialize
// result from the tools it registers. It names only registered tools, so a
// profile never tells an agent to call a tool it lacks. It returns "" when no
// paragraph applies. Text is ASCII so that clients counting UTF-16 units and
// clients counting bytes agree on its length.
func serverInstructions(names []string, options instructionOptions) string {
	var lines []string
	for _, paragraph := range runtimeParagraphs {
		if paragraph.applies(names, options) {
			lines = append(lines, paragraph.text)
		}
	}
	if curation := curationInstructions(names); curation != "" {
		lines = append(lines, curation)
	}
	return strings.Join(lines, "\n")
}

func (paragraph instructionParagraph) applies(names []string, options instructionOptions) bool {
	for _, required := range paragraph.requires {
		if !slices.Contains(names, required) {
			return false
		}
	}
	return paragraph.flag == nil || paragraph.flag(options)
}

// curationInstructions describes the curation tools, which change the hub.
// It renders only when a tool other than the runtime tools is registered.
func curationInstructions(names []string) string {
	curation := false
	preview := false
	for _, name := range names {
		if !slices.Contains(runtimeToolNames, name) {
			curation = true
		}
		preview = preview || strings.HasSuffix(name, "_preview")
	}
	if !curation {
		return ""
	}
	text := "Skill Hub curation tools change the user's hub. Use them only when the user explicitly asks to manage, curate, check, distill, repair or inspect Skill Hub, and follow the native `system-curator` skill."
	if slices.Contains(names, "hub_status") {
		text += " Start with `hub_status`."
	}
	if preview {
		text += " A `_preview` tool shows a change; apply it with the matching `_confirm` tool only after the user agrees."
	}
	return text
}
