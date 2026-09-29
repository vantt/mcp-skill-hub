// Package insight defines reviewed insight decisions, application proposals,
// provenance mappings, and explicit outcomes.
package insight

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const SchemaVersion = 1

var (
	idPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// Decision is an explicit human/authorized-agent lifecycle decision.
type Decision struct {
	State          string `yaml:"state" json:"state"`
	Rationale      string `yaml:"rationale" json:"rationale"`
	EvidenceDigest string `yaml:"evidence_digest" json:"evidence_digest"`
	DecidedAt      string `yaml:"decided_at" json:"decided_at"`
}

// PathPin makes a reviewed proposal stale when any target changes.
type PathPin struct {
	Path   string `yaml:"path" json:"path"`
	Before string `yaml:"before,omitempty" json:"before,omitempty"`
	After  string `yaml:"after,omitempty" json:"after,omitempty"`
}

// ApplicationProposal is the durable audit form of an approved runtime proposal.
type ApplicationProposal struct {
	SchemaVersion      int       `yaml:"schema_version" json:"schema_version"`
	ID                 string    `yaml:"id" json:"id"`
	InsightID          string    `yaml:"insight_id" json:"insight_id"`
	BaseCatalogVersion string    `yaml:"base_catalog_version" json:"base_catalog_version"`
	Digest             string    `yaml:"digest" json:"digest"`
	Status             string    `yaml:"status" json:"status"`
	ChangedFiles       []string  `yaml:"changed_files" json:"changed_files"`
	PathPins           []PathPin `yaml:"path_pins" json:"path_pins"`
	CreatedAt          string    `yaml:"created_at" json:"created_at"`
}

// SourceToLocalMapping records one explicit source finding to local concept relationship.
type SourceToLocalMapping struct {
	ObservationID string `yaml:"observation_id" json:"observation_id"`
	ArtifactPath  string `yaml:"artifact_path" json:"artifact_path"`
	Concept       string `yaml:"concept" json:"concept"`
}

// Incorporation is the immutable result of applying one exact proposal.
type Incorporation struct {
	SchemaVersion  int                    `yaml:"schema_version" json:"schema_version"`
	ID             string                 `yaml:"id" json:"id"`
	InsightID      string                 `yaml:"insight_id" json:"insight_id"`
	ProposalID     string                 `yaml:"proposal_id" json:"proposal_id"`
	OperationID    string                 `yaml:"operation_id" json:"operation_id"`
	State          string                 `yaml:"state" json:"state"`
	Targets        []string               `yaml:"targets" json:"targets"`
	SourceToLocal  []SourceToLocalMapping `yaml:"source_to_local" json:"source_to_local"`
	IncorporatedAt string                 `yaml:"incorporated_at" json:"incorporated_at"`
}

// Outcome is immutable, explicit post-use evidence. Applying or using a skill never creates one.
type Outcome struct {
	SchemaVersion   int      `yaml:"schema_version" json:"schema_version"`
	ID              string   `yaml:"id" json:"id"`
	IncorporationID string   `yaml:"incorporation_id" json:"incorporation_id"`
	State           string   `yaml:"state" json:"state"`
	Evidence        []string `yaml:"evidence" json:"evidence"`
	Note            string   `yaml:"note" json:"note"`
	RecordedAt      string   `yaml:"recorded_at" json:"recorded_at"`
	Supersedes      string   `yaml:"supersedes,omitempty" json:"supersedes,omitempty"`
}

func Marshal(value any) ([]byte, error) { return yaml.Marshal(value) }

func ParseApplicationProposal(data []byte) (ApplicationProposal, error) {
	var value ApplicationProposal
	if err := strict(data, &value); err != nil {
		return value, err
	}
	return value, ValidateApplicationProposal(value)
}
func ParseIncorporation(data []byte) (Incorporation, error) {
	var value Incorporation
	if err := strict(data, &value); err != nil {
		return value, err
	}
	return value, ValidateIncorporation(value)
}
func ParseOutcome(data []byte) (Outcome, error) {
	var value Outcome
	if err := strict(data, &value); err != nil {
		return value, err
	}
	return value, ValidateOutcome(value)
}

func ValidateApplicationProposal(value ApplicationProposal) error {
	if value.SchemaVersion != SchemaVersion || !validID(value.ID) || !validID(value.InsightID) || !digestPattern.MatchString(value.BaseCatalogVersion) || !digestPattern.MatchString(value.Digest) || value.Status != "approved" {
		return errors.New("invalid application proposal identity or pins")
	}
	if _, err := time.Parse(time.RFC3339Nano, value.CreatedAt); err != nil {
		return errors.New("created_at must be RFC3339")
	}
	if len(value.ChangedFiles) == 0 || len(value.PathPins) != len(value.ChangedFiles) {
		return errors.New("application proposal requires one path pin per changed file")
	}
	files := append([]string(nil), value.ChangedFiles...)
	sort.Strings(files)
	seen := map[string]bool{}
	for index, pin := range value.PathPins {
		if !safePath(pin.Path) || seen[pin.Path] || pin.Path != files[index] || !validOptionalDigest(pin.Before) || !validOptionalDigest(pin.After) || pin.Before == pin.After {
			return errors.New("application proposal path pins must be sorted, unique, changed, and digest-pinned")
		}
		seen[pin.Path] = true
	}
	return nil
}

func ValidateIncorporation(value Incorporation) error {
	if value.SchemaVersion != SchemaVersion || !validID(value.ID) || !validID(value.InsightID) || !validID(value.ProposalID) || !validID(value.OperationID) || value.State != "incorporated" {
		return errors.New("invalid incorporation identity")
	}
	if _, err := time.Parse(time.RFC3339Nano, value.IncorporatedAt); err != nil {
		return errors.New("incorporated_at must be RFC3339")
	}
	if len(value.Targets) == 0 || len(value.SourceToLocal) == 0 {
		return errors.New("incorporation requires targets and source-to-local mappings")
	}
	targets := map[string]bool{}
	for _, target := range value.Targets {
		if !safePath(target) || targets[target] {
			return errors.New("incorporation targets must be unique safe paths")
		}
		targets[target] = true
	}
	mappings := map[string]bool{}
	mappedTargets := map[string]bool{}
	for _, mapping := range value.SourceToLocal {
		concept := strings.TrimSpace(mapping.Concept)
		key := mapping.ObservationID + "\x00" + mapping.ArtifactPath + "\x00" + concept
		if !validID(mapping.ObservationID) || !targets[mapping.ArtifactPath] || concept == "" || mappings[key] {
			return errors.New("invalid or duplicate source-to-local mapping")
		}
		mappings[key] = true
		mappedTargets[mapping.ArtifactPath] = true
	}
	if len(mappedTargets) != len(targets) {
		return errors.New("every incorporation target requires a source-to-local mapping")
	}
	return nil
}

func ValidateOutcome(value Outcome) error {
	if value.SchemaVersion != SchemaVersion || !validID(value.ID) || !validID(value.IncorporationID) {
		return errors.New("invalid outcome identity")
	}
	switch value.State {
	case "confirmed", "adjusted", "ineffective":
	default:
		return errors.New("outcome state must be confirmed, adjusted, or ineffective")
	}
	if len(value.Evidence) == 0 || strings.TrimSpace(value.Note) == "" {
		return errors.New("outcome requires explicit evidence and a note")
	}
	for _, item := range value.Evidence {
		if strings.TrimSpace(item) == "" {
			return errors.New("outcome evidence cannot be empty")
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, value.RecordedAt); err != nil {
		return errors.New("recorded_at must be RFC3339")
	}
	if value.Supersedes != "" && !validID(value.Supersedes) {
		return errors.New("supersedes must be a valid outcome ID")
	}
	return nil
}

// ProposalDigest binds the semantic application payload independently from the WAL proposal digest.
func ProposalDigest(insightID, base, operationID string, pins []PathPin, mappings []SourceToLocalMapping) string {
	pins = append([]PathPin(nil), pins...)
	mappings = append([]SourceToLocalMapping(nil), mappings...)
	sort.Slice(pins, func(i, j int) bool { return pins[i].Path < pins[j].Path })
	sort.Slice(mappings, func(i, j int) bool {
		if mappings[i].ObservationID != mappings[j].ObservationID {
			return mappings[i].ObservationID < mappings[j].ObservationID
		}
		if mappings[i].ArtifactPath != mappings[j].ArtifactPath {
			return mappings[i].ArtifactPath < mappings[j].ArtifactPath
		}
		return strings.TrimSpace(mappings[i].Concept) < strings.TrimSpace(mappings[j].Concept)
	})
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\x00%s\x00%s\n", insightID, base, operationID)
	for _, pin := range pins {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\n", pin.Path, pin.Before, pin.After)
	}
	for _, mapping := range mappings {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\n", mapping.ObservationID, mapping.ArtifactPath, strings.TrimSpace(mapping.Concept))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func strict(data []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("multiple YAML documents are not allowed")
	}
	return nil
}
func validID(value string) bool             { return len(value) <= 240 && idPattern.MatchString(value) }
func validOptionalDigest(value string) bool { return value == "" || digestPattern.MatchString(value) }
func safePath(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.HasPrefix(value, "../") && !strings.HasPrefix(value, "/") && !strings.Contains(value, `\`)
}
