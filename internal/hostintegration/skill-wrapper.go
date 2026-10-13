package hostintegration

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vantt/skillhub-mcp/internal/systemskills"
)

// wrapperTemplateVersion is written into the first body line of every
// wrapper. Change it whenever the rendered bytes change, and record the
// digest of the shipped render in known-wrappers.go after a release.
const wrapperTemplateVersion = "wrapper v1"

// renderWrapper returns the SKILL.md that Claude Code reads for a system
// skill whose real files live in the shared Agent Skills folder. Claude Code
// substitutes ${CLAUDE_SKILL_DIR} with the wrapper's folder, so the same
// relative path works in a project and in the home folder.
func renderWrapper(id, description string) ([]byte, error) {
	frontmatter, err := yaml.Marshal(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: "name"}, {Kind: yaml.ScalarNode, Value: id},
		{Kind: yaml.ScalarNode, Value: "description"}, {Kind: yaml.ScalarNode, Value: description},
	}})
	if err != nil {
		return nil, fmt.Errorf("encode the %s wrapper frontmatter: %w", id, err)
	}
	target := "${CLAUDE_SKILL_DIR}/../../../" + SharedSkillsDirRelativePath + "/" + id + "/"
	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(frontmatter)
	out.WriteString("---\n")
	fmt.Fprintf(&out, "<!-- Written by Skill Hub (%s). Edit the skill in %s/%s/, not here. -->\n\n", wrapperTemplateVersion, SharedSkillsDirRelativePath, id)
	out.WriteString("This skill lives in the shared Agent Skills folder. Read\n")
	fmt.Fprintf(&out, "`%sSKILL.md` now and follow it as this skill.\n", target)
	out.WriteString("Wherever it mentions this skill's folder or its scripts, use\n")
	fmt.Fprintf(&out, "`%s`.\n", target)
	return out.Bytes(), nil
}

// skillWrapper renders the current wrapper of a bundled system skill from the
// description in its canonical SKILL.md frontmatter.
func skillWrapper(skill systemskills.SystemSkill) ([]byte, error) {
	description, err := frontmatterDescription(skill.Files[0].Content)
	if err != nil {
		return nil, fmt.Errorf("read the description of %s: %w", skill.ID, err)
	}
	return renderWrapper(skill.ID, description)
}

func frontmatterDescription(content []byte) (string, error) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", fmt.Errorf("SKILL.md has no frontmatter")
	}
	block, _, found := strings.Cut(text[len("---\n"):], "\n---\n")
	if !found {
		return "", fmt.Errorf("SKILL.md frontmatter is not closed")
	}
	var fields struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(block), &fields); err != nil {
		return "", err
	}
	if strings.TrimSpace(fields.Description) == "" {
		return "", fmt.Errorf("SKILL.md frontmatter has no description")
	}
	return fields.Description, nil
}

// wrapperOwned reports whether raw is a wrapper that Skill Hub wrote for the
// skill: the current render or a render an earlier release shipped. The
// description inside the file is never trusted, so an edited wrapper is the
// user's. CRLF line endings, as a Windows checkout with core.autocrlf writes
// them, are read as LF.
func wrapperOwned(skill systemskills.SystemSkill, raw []byte) bool {
	raw = systemskills.NormalizeLineEndings(raw)
	if current, err := skillWrapper(skill); err == nil && bytes.Equal(raw, current) {
		return true
	}
	digest := systemskills.FileDigest(raw)
	for _, known := range knownWrappers[skill.ID] {
		if known == digest {
			return true
		}
	}
	return false
}
