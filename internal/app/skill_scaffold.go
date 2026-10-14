package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/skill"
)

// SkillScaffoldInput holds the answers of the create form, or the flags that
// stand in for it.
type SkillScaffoldInput struct {
	SkillID    string `json:"skill_id"`
	ProjectDir string `json:"project_dir"`
	// Name is the title of the skill; it defaults to the id.
	Name string `json:"name,omitempty"`
	// Description says what the skill does.
	Description string   `json:"description"`
	Triggers    []string `json:"triggers,omitempty"`
	NotFor      []string `json:"not_for,omitempty"`
	Examples    []string `json:"examples,omitempty"`
}

// SkillScaffoldResult reports the folder a create wrote in the project.
type SkillScaffoldResult struct {
	Result
	SkillID string `json:"skill_id"`
	// Path is the SKILL.md file, relative to the project root.
	Path        string `json:"path"`
	ProjectRoot string `json:"project_root"`
}

// ScaffoldProjectSkill writes .claude/skills/<id>/SKILL.md in the project and
// nothing on the hub. The folder carries no checkout marker, so its first
// commit is a new skill. An existing folder is refused, never overwritten.
func ScaffoldProjectSkill(ctx context.Context, input SkillScaffoldInput) (SkillScaffoldResult, error) {
	id := strings.TrimSpace(input.SkillID)
	if !skillTargetIDPattern.MatchString(id) {
		return scaffoldError(NewInvalidRequestError(fmt.Sprintf("invalid skill id %q", id), "Use lowercase letters, digits and hyphens, starting with a letter or digit (for example commit-style).")), nil
	}
	description := strings.TrimSpace(input.Description)
	if description == "" {
		return scaffoldError(NewInvalidRequestError("a description of what the skill does is required", "Pass --description \"<what the skill does>\".")), nil
	}
	projectRoot := projectRootOf(ctx, input.ProjectDir)
	folder := filepath.Join(projectRoot, filepath.FromSlash(projectSkillsDir), id)
	if _, err := os.Lstat(folder); err == nil {
		return scaffoldError(NewSkillConflictError(
			fmt.Sprintf("%s/%s already exists in this project", projectSkillsDir, id),
			fmt.Sprintf("Edit it, then run `skillhub skill commit %s`.", id))), nil
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return SkillScaffoldResult{}, err
	}
	file := filepath.Join(folder, "SKILL.md")
	if err := os.WriteFile(file, []byte(scaffoldSkillMD(id, input)), 0o644); err != nil {
		return SkillScaffoldResult{}, err
	}
	relative := projectSkillsDir + "/" + id + "/SKILL.md"
	return SkillScaffoldResult{
		Result:      NewResult(StatusOK, fmt.Sprintf("Created %s (nothing changed on the hub).", relative)),
		SkillID:     id,
		Path:        relative,
		ProjectRoot: projectRoot,
	}, nil
}

func scaffoldError(err *Error) SkillScaffoldResult {
	return SkillScaffoldResult{Result: ErrorResult(err)}
}

// scaffoldSkillMD is the starter SKILL.md. The description says what the skill
// does and, when there are triggers, "Use when ...". The body repeats the form
// as lists the commit reads its routing suggestions from. The scaffold marker
// keeps an untouched starter from being activated.
func scaffoldSkillMD(id string, input SkillScaffoldInput) string {
	title := strings.TrimSpace(input.Name)
	if title == "" {
		title = id
	}
	description := strings.TrimSpace(input.Description)
	if triggers := cleanPhrases(input.Triggers); len(triggers) > 0 {
		description = strings.TrimRight(description, ". ") + ". Use when " + strings.Join(triggers, " or ") + "."
	}
	var out strings.Builder
	fmt.Fprintf(&out, "---\nname: %s\ndescription: %s\n---\n\n# %s\n\n", id, yamlScalar(description), title)
	writeScaffoldList(&out, "When to use", "Add one bullet for each situation where an agent should choose this skill.", input.Triggers)
	writeScaffoldList(&out, "When not to use", "Add one bullet for each situation where an agent should not choose it.", input.NotFor)
	writeScaffoldList(&out, "Examples", "Add a few requests a person might type to ask for this skill.", input.Examples)
	fmt.Fprintf(&out, "## Instructions\n\n%s\nReplace this paragraph and the comment above it with the steps the agent should follow.\n", skill.ScaffoldMarker)
	return out.String()
}

func writeScaffoldList(out *strings.Builder, heading, hint string, items []string) {
	fmt.Fprintf(out, "## %s\n\n", heading)
	items = cleanPhrases(items)
	if len(items) == 0 {
		fmt.Fprintf(out, "_%s_\n\n", hint)
		return
	}
	for _, item := range items {
		fmt.Fprintf(out, "- %s\n", item)
	}
	out.WriteString("\n")
}

// yamlScalar quotes a frontmatter value only when plain YAML would misread it.
func yamlScalar(text string) string {
	if strings.ContainsAny(text, ":#\"'\n") || strings.HasPrefix(text, "-") || strings.HasPrefix(text, "[") || strings.HasPrefix(text, "{") {
		return fmt.Sprintf("%q", text)
	}
	return text
}
