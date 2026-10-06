package source

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Candidate is the canonical low-friction intake record.
type Candidate struct {
	SchemaVersion  int    `yaml:"schema_version" json:"schema_version"`
	ID             string `yaml:"id" json:"id"`
	Locator        string `yaml:"locator" json:"locator"`
	CapturedAt     string `yaml:"captured_at" json:"captured_at"`
	Reason         string `yaml:"reason" json:"reason"`
	Status         string `yaml:"status" json:"status"`
	DecisionReason string `yaml:"decision_reason,omitempty" json:"decision_reason,omitempty"`
}

// Monitoring is durable policy, not the volatile scheduler/check state.
type Monitoring struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Cadence string `yaml:"cadence" json:"cadence"`
}

type Trust struct {
	Source   string `yaml:"source" json:"source"`
	Reviewed bool   `yaml:"reviewed" json:"reviewed"`
}

// Record is the canonical source catalog representation.
type Record struct {
	SchemaVersion     int        `yaml:"schema_version" json:"schema_version"`
	ID                string     `yaml:"id" json:"id"`
	Adapter           string     `yaml:"adapter" json:"adapter"`
	Locator           Locator    `yaml:"locator" json:"locator"`
	Purpose           string     `yaml:"purpose,omitempty" json:"purpose,omitempty"`
	Status            string     `yaml:"status" json:"status"`
	Identity          Identity   `yaml:"identity" json:"identity"`
	License           string     `yaml:"license,omitempty" json:"license,omitempty"`
	Trust             Trust      `yaml:"trust" json:"trust"`
	Monitoring        Monitoring `yaml:"monitoring" json:"monitoring"`
	Limits            Limits     `yaml:"limits" json:"limits"`
	CurrentRevision   *Revision  `yaml:"current_revision,omitempty" json:"current_revision,omitempty"`
	DistilledRevision *Revision  `yaml:"distilled_revision,omitempty" json:"distilled_revision,omitempty"`
}

// Link records source-to-skill provenance without copying content.
type Link struct {
	SchemaVersion int    `yaml:"schema_version" json:"schema_version"`
	ID            string `yaml:"id" json:"id"`
	SkillID       string `yaml:"skill_id" json:"skill_id"`
	SourceID      string `yaml:"source_id" json:"source_id"`
	Role          string `yaml:"role" json:"role"`
}

func MarshalCanonical(value any) ([]byte, error) {
	data, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func ParseCandidate(data []byte) (Candidate, error) {
	var value Candidate
	if err := strictYAML(data, &value); err != nil {
		return value, err
	}
	if value.SchemaVersion != 1 || !validID(value.ID) || value.Locator == "" || value.Reason == "" {
		return value, errors.New("invalid source candidate")
	}
	if _, err := time.Parse(time.RFC3339Nano, value.CapturedAt); err != nil {
		return value, errors.New("invalid candidate capture time")
	}
	switch value.Status {
	case "pending", "accepted", "rejected", "deferred":
	default:
		return value, errors.New("invalid candidate status")
	}
	if value.Status == "rejected" && strings.TrimSpace(value.DecisionReason) == "" {
		return value, errors.New("rejected candidate requires a reason")
	}
	return value, nil
}

func ParseRecord(data []byte) (Record, error) {
	var value Record
	if err := strictYAML(data, &value); err != nil {
		return value, err
	}
	if value.SchemaVersion != 1 || !validID(value.ID) || value.Adapter == "" || value.Status == "" {
		return value, errors.New("invalid source record")
	}
	if err := ValidateLocator(value.Adapter, value.Locator); err != nil {
		return value, err
	}
	if value.Purpose != "" && value.Purpose != "upstream" {
		return value, errors.New("source purpose must be upstream or empty")
	}
	if value.Monitoring.Cadence != "daily" && value.Monitoring.Cadence != "weekly" && value.Monitoring.Cadence != "manual" {
		return value, errors.New("monitoring cadence must be daily, weekly, or manual")
	}
	if value.Limits.TimeoutSeconds <= 0 || value.Limits.MaxBytes <= 0 || value.Limits.MaxFiles <= 0 || value.Limits.MaxFileBytes <= 0 || value.Limits.MaxFileBytes > value.Limits.MaxBytes {
		return value, errors.New("source limits must be positive and max_file_bytes must not exceed max_bytes")
	}
	if value.CurrentRevision != nil {
		if err := validateRevision(*value.CurrentRevision); err != nil {
			return value, err
		}
	}
	if value.DistilledRevision != nil {
		if err := validateRevision(*value.DistilledRevision); err != nil {
			return value, err
		}
	}
	return value, nil
}

func ValidateLocator(adapter string, locator Locator) error {
	switch adapter {
	case "git":
		if _, err := ValidateRemoteURLWithOptions(locator.Repository, URLValidationOptions{AllowFile: true}); err != nil {
			return err
		}
		if locator.Path != "" && !safeResourcePath(locator.Path) {
			return ErrInvalidLocator
		}
	case "filesystem":
		if locator.SnapshotDigest != "" {
			if !validDigest(locator.SnapshotDigest) {
				return ErrInvalidLocator
			}
			if locator.Path != "" && !safeResourcePath(locator.Path) {
				return ErrInvalidLocator
			}
		} else {
			if !safeSourceLocatorPath(locator.Path) {
				return ErrInvalidLocator
			}
		}
	case "immutable-http", "living-http":
		if _, err := ValidateRemoteURL(locator.URL, false); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: unsupported adapter %q", ErrInvalidLocator, adapter)
	}
	return nil
}

func safeSourceLocatorPath(value string) bool {
	if value == "" || strings.ContainsRune(value, '\x00') {
		return false
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func validateRevision(value Revision) error {
	if value.Value == "" || value.ObservedAt.IsZero() || !validDigest(value.ContentDigest) {
		return errors.New("invalid source revision")
	}
	switch value.Kind {
	case "git-commit":
		if !validGitObject(value.Value) {
			return errors.New("invalid Git commit revision")
		}
	case "filesystem-snapshot", "content-digest":
		if value.Value != value.ContentDigest {
			return errors.New("digest revision value is incoherent")
		}
	case "declared-version":
		if len(value.Value) > 256 || strings.ContainsAny(value.Value, "\r\n\x00") {
			return errors.New("invalid declared revision")
		}
	default:
		return errors.New("unsupported source revision kind")
	}
	return nil
}

func strictYAML(data []byte, target any) error {
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

func validID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
