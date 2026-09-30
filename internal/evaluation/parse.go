package evaluation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vantt/mcp-skill-hub/internal/resolver"
	"gopkg.in/yaml.v3"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"

	MaxDocumentBytes    = 8 << 20
	MaxSuiteCases       = 1000
	MaxSuiteSkills      = 1000
	MaxTagsPerCase      = 32
	MaxStringRunes      = 4096
	MaxArrayItems       = 256
	MaxMapItems         = 128
	MaxBootstrapSamples = 100000
)

func LoadCase(path string) (Case, error) {
	data, err := readBounded(path)
	if err != nil {
		return Case{}, fmt.Errorf("read evaluation case: %w", err)
	}
	return ParseCase(data, formatForPath(path))
}
func LoadSuite(path string) (Suite, error) {
	data, err := readBounded(path)
	if err != nil {
		return Suite{}, fmt.Errorf("read evaluation suite: %w", err)
	}
	return ParseSuite(data, formatForPath(path))
}
func LoadManifest(path string) (Manifest, error) {
	data, err := readBounded(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read experiment manifest: %w", err)
	}
	return ParseManifest(data, formatForPath(path))
}
func LoadCalibrationPolicy(path string) (CalibrationPolicy, error) {
	data, err := readBounded(path)
	if err != nil {
		return CalibrationPolicy{}, fmt.Errorf("read evaluation policy: %w", err)
	}
	return ParseCalibrationPolicy(data, formatForPath(path))
}
func readBounded(path string) (data []byte, resultErr error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); resultErr == nil && closeErr != nil {
			resultErr = closeErr
		}
	}()
	data, err = io.ReadAll(io.LimitReader(file, MaxDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxDocumentBytes {
		return nil, fmt.Errorf("document exceeds %d bytes", MaxDocumentBytes)
	}
	return data, nil
}

func formatForPath(path string) Format {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return FormatYAML
	default:
		return FormatJSON
	}
}
func ParseCase(data []byte, format Format) (Case, error) {
	if err := validateDocumentSize(data); err != nil {
		return Case{}, err
	}
	var value Case
	if err := strictDecode(data, format, &value); err != nil {
		return Case{}, fmt.Errorf("parse evaluation case: %w", err)
	}
	if value.Expected.Status != "" && len(value.Expected.AcceptableStatuses) > 0 {
		return Case{}, fmt.Errorf("case %q must use status or acceptable_statuses, not both", value.ID)
	}
	normalizeCase(&value, value.SchemaVersion)
	if err := validateCase(value); err != nil {
		return Case{}, err
	}
	return value, nil
}
func ParseSuite(data []byte, format Format) (Suite, error) {
	if err := validateDocumentSize(data); err != nil {
		return Suite{}, err
	}
	var value Suite
	if err := strictDecode(data, format, &value); err != nil {
		return Suite{}, fmt.Errorf("parse evaluation suite: %w", err)
	}
	for index := range value.Cases {
		if value.Cases[index].Expected.Status != "" && len(value.Cases[index].Expected.AcceptableStatuses) > 0 {
			return Suite{}, fmt.Errorf("case %q must use status or acceptable_statuses, not both", value.Cases[index].ID)
		}
		normalizeCase(&value.Cases[index], value.SchemaVersion)
	}
	if err := validateSuite(value); err != nil {
		return Suite{}, err
	}
	return value, nil
}
func ParseCalibrationPolicy(data []byte, format Format) (CalibrationPolicy, error) {
	if err := validateDocumentSize(data); err != nil {
		return CalibrationPolicy{}, err
	}
	var value CalibrationPolicy
	if err := strictDecode(data, format, &value); err != nil {
		return CalibrationPolicy{}, fmt.Errorf("parse evaluation policy: %w", err)
	}
	if value.SchemaVersion != SchemaVersion || value.Corpus == "" || value.CalibrationSplit == "" || value.HeldOutSplit == "" || len(value.CalibrationGrid.ApplicabilityFloor) == 0 || len(value.CalibrationGrid.MinimumMargin) == 0 {
		return CalibrationPolicy{}, errors.New("evaluation policy is incomplete or unsupported")
	}
	if err := validateBounds(reflect.ValueOf(value), "policy"); err != nil {
		return CalibrationPolicy{}, err
	}
	return value, nil
}

func ParseManifest(data []byte, format Format) (Manifest, error) {
	if err := validateDocumentSize(data); err != nil {
		return Manifest{}, err
	}
	var value Manifest
	if err := strictDecode(data, format, &value); err != nil {
		return Manifest{}, fmt.Errorf("parse experiment manifest: %w", err)
	}
	if err := validateManifest(value); err != nil {
		return Manifest{}, err
	}
	return value, nil
}

func validateDocumentSize(data []byte) error {
	if len(data) > MaxDocumentBytes {
		return fmt.Errorf("document exceeds %d bytes", MaxDocumentBytes)
	}
	return nil
}

func strictDecode(data []byte, format Format, destination interface{}) error {
	if format == FormatYAML {
		converted, err := yamlToJSON(data)
		if err != nil {
			return err
		}
		data = converted
	} else if format != FormatJSON {
		return fmt.Errorf("unsupported format %q", format)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("document has trailing data")
	}
	return nil
}

func yamlToJSON(data []byte) ([]byte, error) {
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 {
		return nil, errors.New("YAML document is empty")
	}
	value, err := yamlNode(document.Content[0])
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(&yaml.Node{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("YAML document has trailing data")
	}
	return json.Marshal(value)
}

// YAML has no JSON-token stream in yaml.v3, so this boundary conversion uses
// interface{} only as the transient representation consumed immediately by JSON.
func yamlNode(node *yaml.Node) (interface{}, error) {
	switch node.Kind {
	case yaml.MappingNode:
		result := make(map[string]interface{}, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return nil, errors.New("YAML mapping keys must be strings")
			}
			if _, exists := result[key.Value]; exists {
				return nil, fmt.Errorf("duplicate YAML key %q", key.Value)
			}
			value, err := yamlNode(node.Content[i+1])
			if err != nil {
				return nil, err
			}
			result[key.Value] = value
		}
		return result, nil
	case yaml.SequenceNode:
		result := make([]interface{}, len(node.Content))
		for i, child := range node.Content {
			value, err := yamlNode(child)
			if err != nil {
				return nil, err
			}
			result[i] = value
		}
		return result, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!null":
			return nil, nil
		case "!!bool":
			return strconv.ParseBool(node.Value)
		case "!!int":
			return strconv.ParseInt(node.Value, 0, 64)
		case "!!float":
			return strconv.ParseFloat(node.Value, 64)
		case "!!str", "":
			return node.Value, nil
		default:
			return nil, fmt.Errorf("unsupported YAML scalar tag %s", node.Tag)
		}
	default:
		return nil, errors.New("YAML aliases and custom nodes are not supported")
	}
}

var allowedCounters = map[string]bool{
	"curation.turns_to_next_action": true, "curation.unnecessary_confirmations": true,
	"curation.prompts_per_batch": true, "curation.partial_failure_completions": true,
	"curation.auto_finalized": true, "curation.blocking_decision": true,
	"curation.default_status_no_network": true, "curation.default_response_bytes": true,
	"curation.raw_evidence_disclosures": true, "curation.insight_review_turns": true,
	"curation.stale_proposal_rejections": true, "curation.recovery_completed": true,
	"curation.recovery_abandoned": true, "curation.routine_git_noise": true,
	"curation.mutations_reporting_state": true, "curation.abandoned": true,
	"distillation.completed": true, "distillation.failed": true, "distillation.retried": true,
	"distillation.changed_resources_analyzed": true, "distillation.changed_resources_deferred": true,
	"distillation.changed_resources_unreadable": true, "distillation.coverage_gaps": true,
	"distillation.silent_omissions": true, "distillation.observations_created": true,
	"distillation.observations_updated": true, "distillation.observations_tombstoned": true,
	"distillation.evidence_locator_failures": true, "distillation.stale_syntheses": true,
	"distillation.cursor_atomicity_violations": true,
	"invocation.substantive_resolver_called":   true, "invocation.trivial_resolver_called": true,
	"invocation.correct_reuse": true, "invocation.operation_change_reconsulted": true,
	"invocation.duplicate_activations": true, "invocation.missed_useful_skill": true,
}

func normalizeCase(value *Case, inheritedVersion int) {
	if value.SchemaVersion == 0 {
		value.SchemaVersion = inheritedVersion
	}
	if value.Partition == "" {
		value.Partition = value.Split
	}
	value.Split = ""
	if len(value.Expected.AcceptableStatuses) == 0 && value.Expected.Status != "" {
		value.Expected.AcceptableStatuses = []resolver.Status{value.Expected.Status}
	}
	value.Expected.Status = ""
	if expectsOnlyNoSkill(value.Expected.AcceptableStatuses) {
		value.Expected.NoSkill = true
	}
	for answer, branch := range value.Expected.Branches {
		if expectsOnlyNoSkill(branch.AcceptableStatuses) {
			branch.NoSkill = true
			value.Expected.Branches[answer] = branch
		}
	}
}

func validateCase(value Case) error {
	if value.Expected.Status != "" {
		return fmt.Errorf("case %q must be normalized before validation", value.ID)
	}
	if err := validateBounds(reflect.ValueOf(value), "case"); err != nil {
		return err
	}
	if len(value.Tags) > MaxTagsPerCase {
		return fmt.Errorf("case %q has more than %d tags", value.ID, MaxTagsPerCase)
	}
	if value.SchemaVersion != SchemaVersion {
		return fmt.Errorf("case %q uses unsupported schema_version %d", value.ID, value.SchemaVersion)
	}
	if value.ID == "" {
		return errors.New("case id is required")
	}
	if !validPartition(value.Partition) {
		return fmt.Errorf("case %q has invalid partition %q", value.ID, value.Partition)
	}
	if value.Request.SchemaVersion == "" || value.Request.RequestID == "" || strings.TrimSpace(value.Request.Task.Description) == "" {
		return fmt.Errorf("case %q request is incomplete", value.ID)
	}
	if len(value.Expected.AcceptableStatuses) == 0 {
		return fmt.Errorf("case %q has no acceptable statuses", value.ID)
	}
	if err := uniqueStrings(value.Tags, "tags"); err != nil {
		return fmt.Errorf("case %q: %w", value.ID, err)
	}
	statuses, err := validateStatuses(value.Expected.AcceptableStatuses)
	if err != nil {
		return fmt.Errorf("case %q: %w", value.ID, err)
	}
	for name, values := range map[string][]string{
		"acceptable_primary":   value.Expected.AcceptablePrimary,
		"unacceptable_primary": value.Expected.UnacceptablePrimary,
		"supporting":           value.Expected.Supporting,
	} {
		if err := nonEmptyUniqueStrings(values, name); err != nil {
			return fmt.Errorf("case %q: %w", value.ID, err)
		}
	}
	for _, primary := range value.Expected.AcceptablePrimary {
		if containsString(value.Expected.UnacceptablePrimary, primary) {
			return fmt.Errorf("case %q primary %q is both acceptable and unacceptable", value.ID, primary)
		}
	}
	if value.Expected.NoSkill && !statuses[resolver.StatusNoSkill] {
		return fmt.Errorf("case %q declares no_skill without accepting no_skill status", value.ID)
	}
	if statuses[resolver.StatusResolved] && len(value.Expected.AcceptablePrimary) == 0 {
		return fmt.Errorf("case %q accepts resolved without acceptable_primary", value.ID)
	}
	if statuses[resolver.StatusNeedsContext] && value.Expected.Question == nil {
		return fmt.Errorf("case %q accepts needs_context without question", value.ID)
	}
	if value.Expected.Question != nil && value.Expected.Question.Field == "" {
		return fmt.Errorf("case %q question field is required", value.ID)
	}
	for answer, branch := range value.Expected.Branches {
		if answer == "" {
			return fmt.Errorf("case %q has empty branch answer", value.ID)
		}
		branchStatuses, err := validateStatuses(branch.AcceptableStatuses)
		if err != nil {
			return fmt.Errorf("case %q branch %q: %w", value.ID, answer, err)
		}
		if err := nonEmptyUniqueStrings(branch.AcceptablePrimary, "acceptable_primary"); err != nil {
			return fmt.Errorf("case %q branch %q: %w", value.ID, answer, err)
		}
		if branchStatuses[resolver.StatusResolved] && len(branch.AcceptablePrimary) == 0 {
			return fmt.Errorf("case %q branch %q accepts resolved without acceptable_primary", value.ID, answer)
		}
		if branch.NoSkill && !branchStatuses[resolver.StatusNoSkill] {
			return fmt.Errorf("case %q branch %q declares no_skill without accepting no_skill status", value.ID, answer)
		}
	}
	for key, count := range value.Counters {
		if !allowedCounters[key] {
			return fmt.Errorf("case %q has unknown counter %q", value.ID, key)
		}
		if count < 0 || math.IsNaN(count) || math.IsInf(count, 0) {
			return fmt.Errorf("case %q counter %q must be finite and non-negative", value.ID, key)
		}
	}
	return nil
}
func validateSuite(value Suite) error {
	if utf8.RuneCountInString(value.ID) > MaxStringRunes || utf8.RuneCountInString(value.Sanitization) > MaxStringRunes {
		return errors.New("suite string exceeds limit")
	}
	if len(value.Cases) > MaxSuiteCases {
		return fmt.Errorf("suite has more than %d cases", MaxSuiteCases)
	}
	if len(value.Skills) > MaxSuiteSkills {
		return fmt.Errorf("suite has more than %d skills", MaxSuiteSkills)
	}
	if value.SchemaVersion != SchemaVersion {
		return fmt.Errorf("suite %q uses unsupported schema_version %d", value.ID, value.SchemaVersion)
	}
	if value.ID == "" || len(value.Cases) == 0 {
		return errors.New("suite id and cases are required")
	}
	skillIDs := map[string]bool{}
	for _, skill := range value.Skills {
		if err := validateBounds(reflect.ValueOf(skill), "skill"); err != nil {
			return err
		}
		if strings.TrimSpace(skill.ID) == "" {
			return errors.New("suite skill id is required")
		}
		if skillIDs[skill.ID] {
			return fmt.Errorf("skill id %q is duplicated", skill.ID)
		}
		skillIDs[skill.ID] = true
	}
	ids := map[string]Partition{}
	for _, test := range value.Cases {
		if err := validateCase(test); err != nil {
			return err
		}
		if prior, ok := ids[test.ID]; ok {
			return fmt.Errorf("case id %q is duplicated across %s and %s", test.ID, prior, test.Partition)
		}
		ids[test.ID] = test.Partition
	}
	return nil
}
func validateStatuses(values []resolver.Status) (map[resolver.Status]bool, error) {
	if len(values) == 0 {
		return nil, errors.New("no acceptable statuses")
	}
	statuses := make(map[resolver.Status]bool, len(values))
	for _, status := range values {
		if status != resolver.StatusResolved && status != resolver.StatusNoSkill && status != resolver.StatusNeedsContext && status != resolver.StatusAlreadyCovered {
			return nil, fmt.Errorf("invalid acceptable status %q", status)
		}
		if statuses[status] {
			return nil, fmt.Errorf("repeats acceptable status %q", status)
		}
		statuses[status] = true
	}
	return statuses, nil
}

func uniqueStrings(values []string, name string) error {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if seen[value] {
			return fmt.Errorf("%s repeats %q", name, value)
		}
		seen[value] = true
	}
	return nil
}

func nonEmptyUniqueStrings(values []string, name string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s contains an empty value", name)
		}
	}
	return uniqueStrings(values, name)
}

func validateMeasurements(value Measurements) error {
	if value.LatencyMS != nil && (*value.LatencyMS < 0 || math.IsNaN(*value.LatencyMS) || math.IsInf(*value.LatencyMS, 0)) {
		return errors.New("latency_ms must be finite and non-negative")
	}
	if len(value.Counters) > MaxMapItems {
		return fmt.Errorf("counters has more than %d entries", MaxMapItems)
	}
	for key, count := range value.Counters {
		if !allowedCounters[key] {
			return fmt.Errorf("unknown counter %q", key)
		}
		if count < 0 || math.IsNaN(count) || math.IsInf(count, 0) {
			return fmt.Errorf("counter %q must be finite and non-negative", key)
		}
	}
	return nil
}

var timeType = reflect.TypeOf(time.Time{})

func validateBounds(value reflect.Value, path string) error {
	if !value.IsValid() {
		return nil
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		return validateBounds(value.Elem(), path)
	}
	if value.Type() == timeType {
		return nil
	}
	switch value.Kind() {
	case reflect.String:
		if utf8.RuneCountInString(value.String()) > MaxStringRunes {
			return fmt.Errorf("%s exceeds %d characters", path, MaxStringRunes)
		}
	case reflect.Slice, reflect.Array:
		if value.Len() > MaxArrayItems {
			return fmt.Errorf("%s has more than %d items", path, MaxArrayItems)
		}
		if value.Type().Elem().Kind() == reflect.String {
			seen := make(map[string]bool, value.Len())
			for index := 0; index < value.Len(); index++ {
				item := value.Index(index).String()
				if seen[item] {
					return fmt.Errorf("%s repeats %q", path, item)
				}
				seen[item] = true
			}
		}
		for index := 0; index < value.Len(); index++ {
			if err := validateBounds(value.Index(index), fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case reflect.Map:
		if value.Len() > MaxMapItems {
			return fmt.Errorf("%s has more than %d entries", path, MaxMapItems)
		}
		iterator := value.MapRange()
		for iterator.Next() {
			if err := validateBounds(iterator.Key(), path+" key"); err != nil {
				return err
			}
			if err := validateBounds(iterator.Value(), path+" value"); err != nil {
				return err
			}
		}
	case reflect.Struct:
		valueType := value.Type()
		for index := 0; index < value.NumField(); index++ {
			if err := validateBounds(value.Field(index), path+"."+valueType.Field(index).Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateManifest(value Manifest) error {
	if err := validateBounds(reflect.ValueOf(value), "manifest"); err != nil {
		return err
	}
	if value.SchemaVersion != SchemaVersion {
		return fmt.Errorf("manifest uses unsupported schema_version %d", value.SchemaVersion)
	}
	if value.ExperimentID == "" || value.CatalogSnapshot == "" || value.PolicyRevision == "" || value.ProtocolSchema == "" || value.NormalizationVersion == "" || value.IndexVersion == "" || value.FactProviderFixture == "" || value.FactProviderVersion == "" || value.Binary.Version == "" || value.Binary.Commit == "" || value.Variant == "" {
		return errors.New("manifest is missing a required replay identity")
	}
	if value.CaseDigest == "" && value.SuiteDigest == "" {
		return errors.New("manifest must pin a case_digest or suite_digest")
	}
	for name, digest := range map[string]string{"case_digest": value.CaseDigest, "suite_digest": value.SuiteDigest, "catalog_snapshot": value.CatalogSnapshot, "policy_revision": value.PolicyRevision} {
		if digest != "" && !validDigest(digest) {
			return fmt.Errorf("manifest %s is not a sha256 digest", name)
		}
	}
	if value.Model != nil && (value.Model.Provider == "" || value.Model.Model == "" || value.Model.Configuration == "") {
		return errors.New("model identity is incomplete")
	}
	return nil
}
func validPartition(value Partition) bool {
	return value == PartitionDevelopment || value == PartitionCalibration || value == PartitionHeldOut
}
func containsStatus(values []resolver.Status, wanted resolver.Status) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && strings.ToLower(value) == value
}
func DigestCase(value Case) (string, error) {
	if err := validateCase(value); err != nil {
		return "", err
	}
	return digestJSON(value)
}
func DigestSuite(value Suite) (string, error) {
	if err := validateSuite(value); err != nil {
		return "", err
	}
	return digestJSON(value)
}
func digestJSON(value interface{}) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
func selectedPartitions(values []Partition) map[Partition]bool {
	result := map[Partition]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}
func sortedKeys[V interface{}](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
