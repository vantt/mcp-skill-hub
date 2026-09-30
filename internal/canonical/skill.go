package canonical

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	maxSkillResourceBytes = 16 << 20
	reservedSystemSkillID = "system-curator"
)

var skillIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type skillMetadata struct {
	Path          string
	Directory     string
	ID            string
	Status        string
	Relationships []string
}

// validateSkills performs the domain-specific validation that generic YAML
// identity checks cannot express. Drafts may be incomplete, while active
// skills must be independently routable and distributable.
func validateSkills(root string, files []string) []Issue {
	fileSet := make(map[string]struct{}, len(files))
	for _, path := range files {
		fileSet[path] = struct{}{}
	}
	var issues []Issue
	var metadata []skillMetadata
	resourceDirectories := make(map[string]struct{})
	for _, path := range files {
		if !strings.HasPrefix(path, "skills/") {
			continue
		}
		parts := strings.Split(path, "/")
		if len(parts) >= 4 {
			resourceDirectories[strings.Join(parts[:3], "/")] = struct{}{}
		}
		if len(parts) == 4 && parts[3] == "skill.meta.yaml" {
			contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			if err != nil {
				issues = append(issues, Issue{Path: path, Message: err.Error()})
				continue
			}
			item, validationIssues := validateSkillMetadata(path, contents)
			issues = append(issues, validationIssues...)
			metadata = append(metadata, item)
			entrypoint := item.Directory + "/SKILL.md"
			if _, ok := fileSet[entrypoint]; !ok {
				issues = append(issues, Issue{Path: entrypoint, Line: 1, Message: "skill entrypoint is missing", Fix: "Create SKILL.md in the skill directory."})
			}
			continue
		}
		if err := validateSkillResource(path, filepath.Join(root, filepath.FromSlash(path))); err != nil {
			issues = append(issues, Issue{Path: path, Message: err.Error()})
		}
	}

	ids := make(map[string]string, len(metadata))
	metadataDirectories := make(map[string]struct{}, len(metadata))
	for _, item := range metadata {
		metadataDirectories[item.Directory] = struct{}{}
		if item.ID != "" {
			ids[item.ID] = item.Path
		}
	}
	for directory := range resourceDirectories {
		if _, ok := metadataDirectories[directory]; !ok {
			issues = append(issues, Issue{Path: directory + "/skill.meta.yaml", Message: "skill metadata is missing"})
		}
	}
	for _, item := range metadata {
		for _, target := range item.Relationships {
			switch {
			case target == item.ID:
				issues = append(issues, Issue{Path: item.Path, Message: "routing relationship must not target the same skill"})
			case ids[target] == "":
				issues = append(issues, Issue{Path: item.Path, Message: fmt.Sprintf("routing relationship target %q does not exist", target)})
			}
		}
	}
	return issues
}

func validateSkillMetadata(path string, contents []byte) (skillMetadata, []Issue) {
	item := skillMetadata{Path: path, Directory: filepath.ToSlash(filepath.Dir(path))}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	if err := decoder.Decode(&document); err != nil {
		return item, []Issue{{Path: path, Message: "invalid skill metadata: " + err.Error()}}
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return item, []Issue{{Path: path, Message: "skill metadata must be one YAML mapping"}}
	}
	root := document.Content[0]
	allowed := stringSet("schema_version", "id", "name", "status", "description", "collection_id", "collection", "aliases", "domain", "topics", "technologies", "content", "routing", "quality", "provenance", "history", "created_at", "updated_at")
	values, err := mappingValues(root, allowed)
	if err != nil {
		return item, []Issue{{Path: path, Message: "invalid skill metadata: " + err.Error()}}
	}
	var issues []Issue
	add := func(message string, node ...*yaml.Node) {
		line := 1
		if len(node) > 0 && node[0] != nil && node[0].Line > 0 {
			line = node[0].Line
		}
		fix := skillIssueFix(message, item.ID)
		issues = append(issues, Issue{Path: path, Line: line, Message: message, Fix: fix})
	}
	if scalarValue(values["schema_version"]) != "1" {
		add("skill schema_version must be 1", values["schema_version"])
	}
	item.ID = scalar(values["id"])
	if !skillIDPattern.MatchString(item.ID) {
		add("skill id must be a lowercase kebab-case identifier")
	}
	if item.ID == reservedSystemSkillID {
		add(fmt.Sprintf("skill id %q is reserved for the bundled system skill; choose a different workspace skill id", reservedSystemSkillID))
	}
	if item.ID != filepath.Base(item.Directory) {
		add(fmt.Sprintf("skill id %q does not match directory %q", item.ID, filepath.Base(item.Directory)))
	}
	for _, field := range []string{"name", "description"} {
		if strings.TrimSpace(scalar(values[field])) == "" {
			add(field+" must be a non-empty string", values[field])
		}
	}
	item.Status = scalar(values["status"])
	if !oneOf(item.Status, "draft", "active", "deprecated", "archived") {
		add("skill status must be draft, active, deprecated, or archived", values["status"])
	}
	for _, field := range []string{"created_at", "updated_at"} {
		if value := scalar(values[field]); value != "" {
			if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
				add(field + " must be an RFC3339 timestamp")
			}
		}
	}
	for _, field := range []string{"aliases", "domain", "topics", "technologies"} {
		if _, err := stringSequence(values[field]); err != nil {
			add(field + " " + err.Error())
		}
	}
	if collection := values["collection_id"]; collection != nil && !skillIDPattern.MatchString(scalar(collection)) {
		add("collection_id must be a lowercase kebab-case identifier")
	}
	if content := values["content"]; content != nil && content.Kind != yaml.MappingNode {
		add("content must be a mapping")
	}
	if provenance := values["provenance"]; provenance != nil && provenance.Kind != yaml.MappingNode {
		add("provenance must be a mapping")
	}
	if quality := values["quality"]; quality != nil {
		if err := validateQuality(quality); err != nil {
			add("quality " + err.Error())
		}
	}
	if history := values["history"]; history != nil {
		if err := validateHistory(history); err != nil {
			add("history " + err.Error())
		}
	}

	routing := values["routing"]
	if routing == nil {
		if item.Status == "active" {
			add("active skill routing metadata is required")
		}
		return item, issues
	}
	if routing.Kind != yaml.MappingNode {
		add("routing must be a mapping")
		return item, issues
	}
	routingValues, err := mappingValues(routing, stringSet("operations", "triggers", "not_for", "min_scope", "requirements", "boosts", "distinguish_from", "supporting", "equivalent_to"))
	if err != nil {
		add("invalid routing metadata: " + err.Error())
		return item, issues
	}
	triggers, triggerErr := stringSequence(routingValues["triggers"])
	if triggerErr != nil {
		add("routing.triggers " + triggerErr.Error())
	}
	notFor, notForErr := stringSequence(routingValues["not_for"])
	if notForErr != nil {
		add("routing.not_for " + notForErr.Error())
	}
	if _, err := stringSequence(routingValues["operations"]); err != nil {
		add("routing.operations " + err.Error())
	}
	if scope := scalar(routingValues["min_scope"]); scope != "" && !oneOf(scope, "single_step", "multi_step", "project") {
		add("routing.min_scope must be single_step, multi_step, or project", routingValues["min_scope"])
	}
	if requirements := routingValues["requirements"]; requirements != nil {
		if err := validateRequirements(requirements); err != nil {
			add("routing.requirements " + err.Error())
		}
	}
	for _, field := range []string{"distinguish_from", "supporting", "equivalent_to"} {
		targets, err := relationshipTargets(routingValues[field], field)
		if err != nil {
			add("routing." + field + " " + err.Error())
		} else {
			seen := make(map[string]bool, len(targets))
			for _, target := range targets {
				if seen[target] {
					add("routing." + field + " must not contain duplicate skill targets")
					break
				}
				seen[target] = true
			}
			item.Relationships = append(item.Relationships, targets...)
		}
	}
	if item.Status == "active" {
		if len(triggers) == 0 {
			add("active skill requires at least one routing trigger", routing)
		}
		if routingValues["not_for"] == nil {
			add("active skill requires routing.not_for (an explicit empty list requires quality.routing_review_rationale)", routing)
		} else if len(notFor) == 0 && qualityRationale(values["quality"]) == "" {
			add("active skill with empty routing.not_for requires quality.routing_review_rationale", routing)
		}
		if scalar(routingValues["min_scope"]) == "" {
			add("active skill requires routing.min_scope", routing)
		}
	}
	return item, issues
}

func validateSkillResource(path, absolute string) error {
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		return fmt.Errorf("skill resource must be under skills/<collection>/<skill>")
	}
	base := filepath.Base(path)
	if base != "SKILL.md" && parts[3] != "references" && parts[3] != "scripts" && parts[3] != "assets" {
		return fmt.Errorf("skill resources must be SKILL.md or live under references, scripts, or assets")
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("skill resource must not be empty")
	}
	if info.Size() > maxSkillResourceBytes {
		return fmt.Errorf("skill resource exceeds %d bytes", maxSkillResourceBytes)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if base == "SKILL.md" || oneOf(ext, ".md", ".txt", ".yaml", ".yml", ".json") {
		contents, err := os.ReadFile(absolute)
		if err != nil {
			return err
		}
		if !utf8.Valid(contents) {
			return fmt.Errorf("text skill resource must be valid UTF-8")
		}
	}
	return nil
}

func mappingValues(node *yaml.Node, allowed map[string]struct{}) (map[string]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.MappingNode || len(node.Content)%2 != 0 {
		return nil, fmt.Errorf("must be a mapping")
	}
	result := make(map[string]*yaml.Node, len(node.Content)/2)
	for index := 0; index < len(node.Content); index += 2 {
		key, value := node.Content[index], node.Content[index+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "" {
			return nil, fmt.Errorf("keys must be non-empty strings")
		}
		if _, ok := allowed[key.Value]; !ok {
			return nil, fmt.Errorf("unknown field %q", key.Value)
		}
		if _, duplicate := result[key.Value]; duplicate {
			return nil, fmt.Errorf("duplicate field %q", key.Value)
		}
		result[key.Value] = value
	}
	return result, nil
}

func scalar(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return ""
	}
	return node.Value
}

func scalarValue(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

func stringSequence(node *yaml.Node) ([]string, error) {
	if node == nil {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("must be a sequence of non-empty strings")
	}
	result := make([]string, 0, len(node.Content))
	seen := make(map[string]struct{}, len(node.Content))
	for _, value := range node.Content {
		text := strings.TrimSpace(scalar(value))
		if text == "" {
			return nil, fmt.Errorf("must be a sequence of non-empty strings")
		}
		if _, duplicate := seen[text]; duplicate {
			return nil, fmt.Errorf("must not contain duplicate values")
		}
		seen[text] = struct{}{}
		result = append(result, text)
	}
	return result, nil
}

func relationshipTargets(node *yaml.Node, kind string) ([]string, error) {
	if node == nil {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("must be a sequence")
	}
	allowed := stringSet("skill")
	switch kind {
	case "distinguish_from":
		allowed = stringSet("skill", "discriminator")
	case "supporting":
		allowed = stringSet("skill", "when", "role", "activation")
	case "equivalent_to":
		allowed = stringSet("skill", "preference", "version_policy")
	}
	var result []string
	for _, entry := range node.Content {
		if entry.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("entries must be mappings")
		}
		values, err := mappingValues(entry, allowed)
		if err != nil {
			return nil, err
		}
		target := scalar(values["skill"])
		if !skillIDPattern.MatchString(target) {
			return nil, fmt.Errorf("entry skill must be a lowercase kebab-case identifier")
		}
		switch kind {
		case "distinguish_from":
			discriminator := values["discriminator"]
			if discriminator == nil || discriminator.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("distinguish_from entries require a discriminator mapping")
			}
			fields, err := mappingValues(discriminator, stringSet("field", "question", "choices"))
			if err != nil || scalar(fields["field"]) == "" || scalar(fields["question"]) == "" {
				return nil, fmt.Errorf("discriminator requires non-empty field and question strings")
			}
			choices, err := stringSequence(fields["choices"])
			if err != nil || len(choices) < 2 {
				return nil, fmt.Errorf("discriminator choices must contain at least two strings")
			}
		case "supporting":
			when := values["when"]
			if when == nil || when.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("supporting entries require when.operation")
			}
			fields, err := mappingValues(when, stringSet("operation"))
			operation := scalar(fields["operation"])
			if err != nil || !oneOf(operation, "explore", "design", "implement", "review", "debug", "test", "refactor", "migrate", "document", "operate", "research", "other") {
				return nil, fmt.Errorf("supporting when.operation is invalid")
			}
			if !oneOf(scalar(values["role"]), "validation", "research", "implementation", "review", "documentation", "operations") {
				return nil, fmt.Errorf("supporting role is invalid")
			}
			if scalar(values["activation"]) != "on-demand" {
				return nil, fmt.Errorf("supporting activation must be on-demand")
			}
		case "equivalent_to":
			if !oneOf(scalar(values["preference"]), "self", "target") {
				return nil, fmt.Errorf("equivalent_to preference is invalid")
			}
			if !oneOf(scalar(values["version_policy"]), "exact", "compatible", "latest-reviewed") {
				return nil, fmt.Errorf("equivalent_to version_policy is invalid")
			}
		}
		result = append(result, target)
	}
	return result, nil
}

func validateRequirements(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("must be a mapping")
	}
	values, err := mappingValues(node, stringSet("facts", "capabilities"))
	if err != nil {
		return err
	}
	if capabilities := values["capabilities"]; capabilities != nil {
		if capabilities.Kind != yaml.MappingNode {
			return fmt.Errorf("capabilities must be a mapping")
		}
		entries, err := mappingValues(capabilities, stringSet("all", "any"))
		if err != nil {
			return err
		}
		for key, entry := range entries {
			if _, err := stringSequence(entry); err != nil {
				return fmt.Errorf("capabilities.%s %w", key, err)
			}
		}
	}
	if facts := values["facts"]; facts != nil {
		if facts.Kind != yaml.MappingNode {
			return fmt.Errorf("facts must be a mapping")
		}
		entries, err := mappingValues(facts, stringSet("all", "any"))
		if err != nil {
			return err
		}
		for key, list := range entries {
			if list.Kind != yaml.SequenceNode {
				return fmt.Errorf("facts.%s must be a sequence", key)
			}
			for _, fact := range list.Content {
				fields, err := mappingValues(fact, stringSet("key", "value"))
				if err != nil || scalar(fields["key"]) == "" || scalar(fields["value"]) == "" {
					return fmt.Errorf("facts.%s entries require non-empty key and value strings", key)
				}
			}
		}
	}
	return nil
}

func validateQuality(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("must be a mapping")
	}
	values, err := mappingValues(node, stringSet("reviewed", "curated", "reviewed_at", "routing_review_rationale"))
	if err != nil {
		return err
	}
	for _, field := range []string{"reviewed", "curated"} {
		if value := values[field]; value != nil && (value.Kind != yaml.ScalarNode || value.Tag != "!!bool") {
			return fmt.Errorf("%s must be a boolean", field)
		}
	}
	if value := values["reviewed_at"]; value != nil {
		if _, err := time.Parse(time.RFC3339Nano, scalar(value)); err != nil {
			return fmt.Errorf("reviewed_at must be an RFC3339 timestamp")
		}
	}
	if value := values["routing_review_rationale"]; value != nil && strings.TrimSpace(scalar(value)) == "" {
		return fmt.Errorf("routing_review_rationale must be a non-empty string")
	}
	return nil
}

func validateHistory(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("must be a sequence")
	}
	for _, entry := range node.Content {
		values, err := mappingValues(entry, stringSet("from", "state", "occurred_at"))
		if err != nil {
			return err
		}
		if !oneOf(scalar(values["state"]), "draft", "active", "deprecated", "archived") {
			return fmt.Errorf("entries require a valid state")
		}
		if value := values["from"]; value != nil && !oneOf(scalar(value), "draft", "active", "deprecated") {
			return fmt.Errorf("entry from must be a valid prior state")
		}
		if _, err := time.Parse(time.RFC3339Nano, scalar(values["occurred_at"])); err != nil {
			return fmt.Errorf("entry occurred_at must be an RFC3339 timestamp")
		}
	}
	return nil
}

func qualityRationale(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.MappingNode {
		return ""
	}
	for index := 0; index < len(node.Content); index += 2 {
		if node.Content[index].Value == "routing_review_rationale" {
			return strings.TrimSpace(scalar(node.Content[index+1]))
		}
	}
	return ""
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func oneOf(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func parseFrontmatterName(content []byte) (string, int, bool) {
	if !bytes.HasPrefix(content, []byte("---\n")) && !bytes.HasPrefix(content, []byte("---\r\n")) {
		return "", 0, false
	}
	lines := strings.Split(string(content), "\n")
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			break
		}
		if strings.HasPrefix(line, "name:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			val = strings.Trim(val, `"'`)
			return val, i + 1, true
		}
	}
	return "", 0, false
}

func skillIssueFix(message, id string) string {
	if id == "" {
		id = "<id>"
	}
	switch {
	case strings.Contains(message, "trigger"):
		return fmt.Sprintf("Run `skillhub skill edit %s --trigger \"<when to use>\" --yes`", id)
	case strings.Contains(message, "not_for"):
		return fmt.Sprintf("Run `skillhub skill edit %s --not-for \"<when not to use>\" --yes`", id)
	case strings.Contains(message, "min_scope"):
		return fmt.Sprintf("Run `skillhub skill edit %s --min-scope <single_step|multi_step|project> --yes`", id)
	case strings.Contains(message, "description"):
		return fmt.Sprintf("Run `skillhub skill edit %s --description \"<description>\" --yes`", id)
	case strings.Contains(message, "status"):
		return "Set status to draft, active, deprecated, or archived"
	default:
		return "Correct the field in this file and run `skillhub validate`."
	}
}
