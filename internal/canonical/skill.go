package canonical

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	var schemaVersion int
	if markerBytes, readErr := os.ReadFile(filepath.Join(root, ".skillhub", "schema-version")); readErr == nil {
		schemaVersion, _ = strconv.Atoi(strings.TrimSpace(string(markerBytes)))
	}
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
		if schemaVersion < 3 && strings.Contains(path, "/.meta/") {
			continue
		}
		parts := strings.Split(path, "/")
		if len(parts) >= 4 {
			resourceDirectories[strings.Join(parts[:3], "/")] = struct{}{}
		}
		isMetaFile := (len(parts) == 4 && parts[3] == "skill.meta.yaml") || (schemaVersion >= 3 && len(parts) == 5 && parts[3] == ".meta" && parts[4] == "skill.yaml")
		if isMetaFile {
			contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			if err != nil {
				issues = append(issues, Issue{Path: path, Message: err.Error()})
				continue
			}
			item, validationIssues := validateSkillMetadata(path, contents)
			issues = append(issues, validationIssues...)
			metadata = append(metadata, item)
			skillDir := strings.Join(parts[:3], "/")
			entrypoint := skillDir + "/SKILL.md"
			if _, ok := fileSet[entrypoint]; !ok {
				issues = append(issues, Issue{Path: entrypoint, Line: 1, Message: "skill entrypoint is missing", Fix: "Create SKILL.md in the skill directory."})
			}
			continue
		}
		if len(parts) == 4 && parts[3] == "SKILL.md" {
			if entryIssues := validateSkillEntrypoint(path, filepath.Join(root, filepath.FromSlash(path)), parts[2]); len(entryIssues) > 0 {
				issues = append(issues, entryIssues...)
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
		skillDir := strings.Join(strings.Split(item.Path, "/")[:3], "/")
		metadataDirectories[skillDir] = struct{}{}
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
	dir := filepath.ToSlash(filepath.Dir(path))
	if filepath.Base(dir) == ".meta" {
		dir = filepath.ToSlash(filepath.Dir(dir))
	}
	item := skillMetadata{Path: path, Directory: dir}
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	if err := decoder.Decode(&document); err != nil {
		return item, []Issue{{Path: path, Message: "invalid skill metadata: " + err.Error()}}
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return item, []Issue{{Path: path, Message: "skill metadata must be one YAML mapping"}}
	}
	root := document.Content[0]
	allowed := stringSet("schema_version", "id", "name", "status", "description", "collection_id", "collection", "aliases", "domain", "topics", "technologies", "content", "routing", "runtime", "quality", "provenance", "history", "created_at", "updated_at", "sources")
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
	isDotMeta := strings.HasSuffix(filepath.ToSlash(path), "/.meta/skill.yaml") || filepath.ToSlash(path) == ".meta/skill.yaml"
	for _, field := range []string{"name", "description"} {
		if strings.TrimSpace(scalar(values[field])) == "" {
			if !isDotMeta {
				add(field+" must be a non-empty string", values[field])
			}
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
	if provenance := values["provenance"]; provenance != nil {
		if provenance.Kind != yaml.MappingNode {
			add("provenance must be a mapping")
		} else {
			provValues, err := mappingValues(provenance, stringSet("created_by", "source_id", "revision", "path", "origin"))
			if err != nil {
				add("invalid provenance: " + err.Error())
			} else {
				if origin := provValues["origin"]; origin != nil {
					if origin.Kind != yaml.MappingNode {
						add("provenance.origin must be a mapping")
					} else {
						originValues, err := mappingValues(origin, stringSet("kind", "repository", "ref", "commit", "path", "name", "folder_digest", "files_digest", "content_digest", "transformations", "added_at"))
						if err != nil {
							add("invalid provenance.origin: " + err.Error())
						} else {
							kind := scalar(originValues["kind"])
							if !oneOf(kind, "github", "git", "local") {
								add("provenance.origin.kind must be github, git, or local", originValues["kind"])
							}
							if kind == "local" {
								if repo := scalar(originValues["repository"]); repo != "" {
									add("provenance.origin.repository is not allowed for local origin", originValues["repository"])
								}
								if p := scalar(originValues["path"]); p != "" && isUnsafeLocalPath(p) {
									add("provenance.origin.path must be a relative, safe skill path", originValues["path"])
								}
								if name := scalar(originValues["name"]); name != "" && isUnsafeLocalName(name) {
									add("provenance.origin.name must be a basename without path separators or absolute label", originValues["name"])
								}
							} else {
								if p := scalar(originValues["path"]); p != "" && isUnsafeLocalPath(p) {
									add("provenance.origin.path must be a relative, safe path", originValues["path"])
								}
							}
							if addedAt := scalar(originValues["added_at"]); addedAt != "" {
								if _, err := time.Parse(time.RFC3339Nano, addedAt); err != nil {
									add("provenance.origin.added_at must be an RFC3339 timestamp", originValues["added_at"])
								}
							}
							for _, digestField := range []string{"folder_digest", "files_digest", "content_digest"} {
								if d := scalar(originValues[digestField]); d != "" {
									if !isValidDigest(d) {
										add("provenance.origin."+digestField+" must be a lowercase SHA-256 digest (e.g. sha256:<hex>)", originValues[digestField])
									}
								}
							}
							if transforms := originValues["transformations"]; transforms != nil {
								if _, err := stringSequence(transforms); err != nil {
									add("provenance.origin.transformations " + err.Error())
								}
							}
						}
					}
				}
			}
		}
	}
	if sourcesNode := values["sources"]; sourcesNode != nil {
		if sourcesNode.Kind != yaml.SequenceNode {
			add("sources must be a sequence", sourcesNode)
		} else {
			for _, itemNode := range sourcesNode.Content {
				if itemNode.Kind != yaml.MappingNode {
					add("source entry must be a mapping", itemNode)
					continue
				}
				sVals, sErr := mappingValues(itemNode, stringSet("id", "roles", "kind", "repository", "repo", "ref", "commit", "path", "files_digest", "folder_digest", "transformations", "synced", "learn_paths", "added_at"))
				if sErr != nil {
					add("invalid source entry: "+sErr.Error(), itemNode)
					continue
				}
				if sID := scalar(sVals["id"]); sID == "" {
					add("source id must be a non-empty string", itemNode)
				}
				roles, rErr := stringSequence(sVals["roles"])
				if rErr != nil || len(roles) == 0 {
					add("source roles must be a non-empty list", itemNode)
				} else {
					for _, r := range roles {
						if r != "upstream" && r != "learning" {
							add(fmt.Sprintf("invalid source role %q (must be upstream or learning)", r), sVals["roles"])
						}
					}
				}
				if lp := sVals["learn_paths"]; lp != nil {
					if _, err := stringSequence(lp); err != nil {
						add("source.learn_paths "+err.Error(), lp)
					}
				}
				for _, digestField := range []string{"files_digest", "folder_digest"} {
					if d := scalar(sVals[digestField]); d != "" && !isValidDigest(d) {
						add("source "+digestField+" must be a lowercase SHA-256 digest (e.g. sha256:<hex>)", sVals[digestField])
					}
				}
			}
		}
	}
	if runtimeNode := values["runtime"]; runtimeNode != nil {
		if err := validateRuntime(runtimeNode); err != nil {
			add("runtime "+err.Error(), runtimeNode)
		}
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
	routingValues, err := mappingValues(routing, stringSet("operations", "triggers", "not_for", "min_scope", "requirements", "boosts", "distinguish_from", "supporting", "equivalent_to", "examples", "counter_examples", "aliases", "domain", "topics", "technologies"))
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
	for _, field := range []string{"aliases", "domain", "topics", "technologies"} {
		if routingValues[field] != nil {
			if _, err := stringSequence(routingValues[field]); err != nil {
				add("routing." + field + " " + err.Error())
			}
		}
	}
	for _, field := range []string{"examples", "counter_examples"} {
		if err := validateRoutingExamples(routingValues[field]); err != nil {
			add("routing."+field+" "+err.Error(), routingValues[field])
		}
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

func validateSkillEntrypoint(path, absolute, skillID string) []Issue {
	var issues []Issue
	info, err := os.Lstat(absolute)
	if err != nil {
		return []Issue{{Path: path, Message: err.Error()}}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return []Issue{{Path: path, Message: "skill entrypoint must be a regular file"}}
	}
	if info.Size() == 0 {
		return []Issue{{Path: path, Message: "skill resource must not be empty"}}
	}
	if info.Size() > maxSkillResourceBytes {
		return []Issue{{Path: path, Message: fmt.Sprintf("skill resource exceeds %d bytes", maxSkillResourceBytes)}}
	}
	contents, err := os.ReadFile(absolute)
	if err != nil {
		return []Issue{{Path: path, Message: err.Error()}}
	}
	if !utf8.Valid(contents) {
		return []Issue{{Path: path, Message: "text skill resource must be valid UTF-8"}}
	}
	if fmName, line, ok := parseFrontmatterName(contents); ok && fmName != "" && fmName != skillID {
		issues = append(issues, Issue{
			Path:    path,
			Line:    line,
			Message: fmt.Sprintf("SKILL.md frontmatter name %q does not match skill ID %q", fmName, skillID),
			Fix:     fmt.Sprintf("Update frontmatter name to %q, remove the name field, or use `skillhub skill edit %s`.", skillID, skillID),
		})
	}
	return issues
}

func validateSkillResource(path, absolute string) error {
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		return fmt.Errorf("skill resource must be under skills/<collection>/<skill>")
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("skill resource must be a regular file")
	}
	if info.Size() > maxSkillResourceBytes {
		return fmt.Errorf("skill resource exceeds %d bytes", maxSkillResourceBytes)
	}
	return nil
}

func isUnsafeLocalPath(p string) bool {
	p = filepath.ToSlash(p)
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || strings.Contains(p, "..") {
		return true
	}
	if len(p) >= 2 && p[1] == ':' {
		return true
	}
	return false
}

func isUnsafeLocalName(name string) bool {
	if strings.ContainsAny(name, "/\\:") || strings.HasPrefix(name, "~") || strings.Contains(name, "..") {
		return true
	}
	return false
}

func isValidDigest(d string) bool {
	if strings.HasPrefix(d, "sha256:") {
		d = strings.TrimPrefix(d, "sha256:")
	}
	if len(d) != 64 {
		return false
	}
	for _, c := range d {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
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
	values, err := mappingValues(node, stringSet("reviewed", "curated", "reviewed_at", "routing_review_rationale", "content_reviewed_digest"))
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
	if value := values["content_reviewed_digest"]; value != nil {
		if digest := scalar(value); !strings.HasPrefix(digest, "sha256:") || !isValidDigest(digest) {
			return fmt.Errorf("content_reviewed_digest must be a lowercase SHA-256 digest (sha256:<64 hex>)")
		}
	}
	return nil
}

const (
	maxRoutingExamples       = 10
	maxRoutingExampleRunes   = 300
	maxRuntimeCommandBytes   = 1024
	runtimeBinNameExpression = `^[A-Za-z0-9._+-]{1,64}$`
)

var (
	runtimeBinNamePattern    = regexp.MustCompile(runtimeBinNameExpression)
	runtimeBinVersionPattern = regexp.MustCompile(`^(>=|>|<=|<|=)?\s*\d+(\.\d+){0,2}$`)
	runtimeEnvNamePattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
)

// validateRoutingExamples checks routing.examples and routing.counter_examples:
// unique non-empty strings, bounded in count and length so they stay useful
// as routing evidence rather than becoming free-form documentation.
func validateRoutingExamples(node *yaml.Node) error {
	examples, err := stringSequence(node)
	if err != nil {
		return err
	}
	if len(examples) > maxRoutingExamples {
		return fmt.Errorf("must contain at most %d entries", maxRoutingExamples)
	}
	for _, example := range examples {
		if utf8.RuneCountInString(example) > maxRoutingExampleRunes {
			return fmt.Errorf("entries must be at most %d characters", maxRoutingExampleRunes)
		}
	}
	return nil
}

// validateRuntime checks the optional runtime block. It declares what a skill
// needs from the host machine; it never stores secret values, only names.
func validateRuntime(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("must be a mapping")
	}
	values, err := mappingValues(node, stringSet("requires", "setup"))
	if err != nil {
		return err
	}
	if requires := values["requires"]; requires != nil {
		if err := validateRuntimeRequires(requires); err != nil {
			return fmt.Errorf("requires %w", err)
		}
	}
	if setup := values["setup"]; setup != nil {
		if setup.Kind != yaml.MappingNode {
			return fmt.Errorf("setup must be a mapping")
		}
		setupValues, err := mappingValues(setup, stringSet("command", "check"))
		if err != nil {
			return fmt.Errorf("setup %w", err)
		}
		for _, field := range []string{"command", "check"} {
			value := setupValues[field]
			if value == nil {
				continue
			}
			command := scalar(value)
			if strings.TrimSpace(command) == "" || strings.ContainsAny(command, "\r\n") || len(command) > maxRuntimeCommandBytes {
				return fmt.Errorf("setup.%s must be a non-empty single-line string of at most %d bytes", field, maxRuntimeCommandBytes)
			}
		}
	}
	return nil
}

func validateRuntimeRequires(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("must be a mapping")
	}
	values, err := mappingValues(node, stringSet("bins", "env", "platforms"))
	if err != nil {
		return err
	}
	if bins := values["bins"]; bins != nil {
		if bins.Kind != yaml.SequenceNode {
			return fmt.Errorf("bins must be a sequence")
		}
		seen := make(map[string]bool, len(bins.Content))
		for _, entry := range bins.Content {
			name, err := runtimeBinName(entry)
			if err != nil {
				return err
			}
			if seen[name] {
				return fmt.Errorf("bins must not contain duplicate names")
			}
			seen[name] = true
		}
	}
	if env := values["env"]; env != nil {
		names, err := stringSequence(env)
		if err != nil {
			return fmt.Errorf("env %w", err)
		}
		for _, name := range names {
			if !runtimeEnvNamePattern.MatchString(name) {
				return fmt.Errorf("env entries must be variable names matching %s (values are never stored)", runtimeEnvNamePattern.String())
			}
		}
	}
	if platforms := values["platforms"]; platforms != nil {
		names, err := stringSequence(platforms)
		if err != nil {
			return fmt.Errorf("platforms %w", err)
		}
		for _, name := range names {
			if !oneOf(name, "linux", "darwin", "windows", "freebsd") {
				return fmt.Errorf("platforms entries must be linux, darwin, windows, or freebsd")
			}
		}
	}
	return nil
}

// runtimeBinName validates one bins entry, either a bare executable name or a
// {name, version} mapping, and returns the executable name.
func runtimeBinName(entry *yaml.Node) (string, error) {
	switch entry.Kind {
	case yaml.ScalarNode:
		name := scalar(entry)
		if !runtimeBinNamePattern.MatchString(name) {
			return "", fmt.Errorf("bins entries must match %s", runtimeBinNameExpression)
		}
		return name, nil
	case yaml.MappingNode:
		fields, err := mappingValues(entry, stringSet("name", "version"))
		if err != nil {
			return "", fmt.Errorf("bins entry %w", err)
		}
		name := scalar(fields["name"])
		if !runtimeBinNamePattern.MatchString(name) {
			return "", fmt.Errorf("bins entry name must match %s", runtimeBinNameExpression)
		}
		if version := fields["version"]; version != nil && !runtimeBinVersionPattern.MatchString(scalar(version)) {
			return "", fmt.Errorf("bins entry version must be a quoted constraint string such as \">=18\" or \"3.11\"")
		}
		return name, nil
	default:
		return "", fmt.Errorf("bins entries must be a name string or a {name, version} mapping")
	}
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
