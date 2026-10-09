package distill

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	EvidencePattern  = regexp.MustCompile(`^(?:[A-Za-z0-9_./:-]+@[0-9a-f]{40}:[^#\s]+(?:#L\d+(?:-L\d+)?)?|usage:[A-Za-z0-9_-]+)$`)
	LessonKeyPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

// Document models one skill's hub-side distillation knowledge in .meta/distill.yaml.
type Document struct {
	Goal     string         `yaml:"goal" json:"goal"`
	Cursors  []Cursor       `yaml:"cursors,omitempty" json:"cursors,omitempty"`
	Coverage []CoverageItem `yaml:"coverage,omitempty" json:"coverage,omitempty"`
	Lessons  []Lesson       `yaml:"lessons,omitempty" json:"lessons,omitempty"`
}

// Cursor tracks distillation progress against one source.
type Cursor struct {
	SourceID string `yaml:"source_id" json:"source_id"`
	Commit   string `yaml:"commit,omitempty" json:"commit,omitempty"`
	SyncedAt string `yaml:"synced_at,omitempty" json:"synced_at,omitempty"`
}

// CoverageItem records review status of one resource in a source.
type CoverageItem struct {
	Resource string `yaml:"resource" json:"resource"`
	Status   string `yaml:"status" json:"status"` // analyzed | deferred | skipped
	Reason   string `yaml:"reason,omitempty" json:"reason,omitempty"`
	Blocking bool   `yaml:"blocking,omitempty" json:"blocking,omitempty"`
}

// Lesson represents one distilled knowledge item anchored to a skill.
type Lesson struct {
	Key      string   `yaml:"key" json:"key"`
	What     string   `yaml:"what" json:"what"`
	Notable  string   `yaml:"notable" json:"notable"`
	Contrast string   `yaml:"contrast,omitempty" json:"contrast,omitempty"`
	Scores   *Scores  `yaml:"scores,omitempty" json:"scores,omitempty"`
	Where    []string `yaml:"where" json:"where"`
	Decision Decision `yaml:"decision,omitempty" json:"decision,omitempty"`
}

// Scores carries optional experimental scorecard metrics from Phase 0 discovery.
type Scores struct {
	Relevance       *float64 `yaml:"relevance,omitempty" json:"relevance,omitempty"`
	EvidenceQuality *float64 `yaml:"evidence_quality,omitempty" json:"evidence_quality,omitempty"`
	Fit             *float64 `yaml:"fit,omitempty" json:"fit,omitempty"`
}

// Decision records the human or agent review disposition for one lesson.
type Decision struct {
	Status    string   `yaml:"status" json:"status"` // candidate | planned | ported | rejected
	Reason    string   `yaml:"reason,omitempty" json:"reason,omitempty"`
	At        string   `yaml:"at,omitempty" json:"at,omitempty"`
	SeenWhere []string `yaml:"seen_where,omitempty" json:"seen_where,omitempty"`
}

// ValidateDocument validates the structure and fields of a Document.
func ValidateDocument(doc *Document) error {
	if doc == nil {
		return errors.New("distill document is nil")
	}
	if strings.TrimSpace(doc.Goal) == "" {
		return errors.New("distill goal is required")
	}
	for i, c := range doc.Cursors {
		if strings.TrimSpace(c.SourceID) == "" {
			return fmt.Errorf("cursors[%d]: source_id is required", i)
		}
	}
	for i, cov := range doc.Coverage {
		if strings.TrimSpace(cov.Resource) == "" {
			return fmt.Errorf("coverage[%d]: resource is required", i)
		}
		if !slices.Contains([]string{"analyzed", "deferred", "skipped"}, cov.Status) {
			return fmt.Errorf("coverage[%d]: invalid status %q (must be analyzed, deferred, or skipped)", i, cov.Status)
		}
	}
	seenKeys := make(map[string]bool, len(doc.Lessons))
	for i, l := range doc.Lessons {
		if err := ValidateLesson(&l); err != nil {
			return fmt.Errorf("lessons[%d] (%s): %w", i, l.Key, err)
		}
		if seenKeys[l.Key] {
			return fmt.Errorf("duplicate lesson key %q", l.Key)
		}
		seenKeys[l.Key] = true
	}
	return nil
}

// ValidateLesson checks key format, required non-empty fields, evidence formatting, and decision status.
func ValidateLesson(lesson *Lesson) error {
	if !LessonKeyPattern.MatchString(lesson.Key) {
		return fmt.Errorf("invalid lesson key %q: must be a kebab-case identifier", lesson.Key)
	}
	if strings.TrimSpace(lesson.What) == "" {
		return errors.New("what is required")
	}
	if strings.TrimSpace(lesson.Notable) == "" {
		return errors.New("notable explanation is required")
	}
	if len(lesson.Where) == 0 {
		return errors.New("where evidence is required (at least one source reference)")
	}
	seenEvidence := make(map[string]bool, len(lesson.Where))
	for _, w := range lesson.Where {
		if !EvidencePattern.MatchString(w) {
			return fmt.Errorf("invalid evidence reference %q: must match repo@<40-hex-sha>:path[#Lx-Ly] or usage:<case_id>", w)
		}
		if seenEvidence[w] {
			return fmt.Errorf("duplicate evidence reference %q in where", w)
		}
		seenEvidence[w] = true
	}
	if lesson.Decision.Status != "" {
		if !slices.Contains([]string{"candidate", "planned", "ported", "rejected"}, lesson.Decision.Status) {
			return fmt.Errorf("invalid decision status %q: must be candidate, planned, ported, or rejected", lesson.Decision.Status)
		}
	}
	return nil
}

// ApplyLesson updates or appends a lesson, implementing D7 reopening logic:
// If the lesson already has a decision (planned, ported, rejected) and the updated Where entries
// include any new evidence not seen when the decision was recorded (in Decision.SeenWhere),
// the decision status is automatically reset to "candidate".
func (doc *Document) ApplyLesson(incoming Lesson, now time.Time) error {
	if err := ValidateLesson(&incoming); err != nil {
		return err
	}
	foundIdx := -1
	for i, l := range doc.Lessons {
		if l.Key == incoming.Key {
			foundIdx = i
			break
		}
	}

	if foundIdx < 0 {
		// New lesson
		if incoming.Decision.Status == "" {
			incoming.Decision.Status = "candidate"
		}
		if incoming.Decision.At == "" {
			incoming.Decision.At = now.UTC().Format(time.RFC3339)
		}
		doc.Lessons = append(doc.Lessons, incoming)
		return nil
	}
	existing := doc.Lessons[foundIdx]
	// Union where entries
	mergedWhere := append([]string{}, existing.Where...)
	for _, w := range incoming.Where {
		if !slices.Contains(mergedWhere, w) {
			mergedWhere = append(mergedWhere, w)
		}
	}
	slices.Sort(mergedWhere)

	// Check if any evidence in mergedWhere was not in existing.Decision.SeenWhere
	hasNewEvidence := false
	if existing.Decision.Status != "" && existing.Decision.Status != "candidate" {
		seenMap := make(map[string]bool, len(existing.Decision.SeenWhere))
		for _, w := range existing.Decision.SeenWhere {
			seenMap[w] = true
		}
		for _, w := range mergedWhere {
			if !seenMap[w] {
				hasNewEvidence = true
				break
			}
		}
	}

	existing.What = incoming.What
	existing.Notable = incoming.Notable
	if incoming.Contrast != "" {
		existing.Contrast = incoming.Contrast
	}
	if incoming.Scores != nil {
		existing.Scores = incoming.Scores
	}
	existing.Where = mergedWhere

	if hasNewEvidence {
		// D7: A new where entry reopens a decided lesson
		existing.Decision.Status = "candidate"
		existing.Decision.Reason = fmt.Sprintf("reopened: new evidence observed (%d sources)", len(mergedWhere))
		existing.Decision.At = now.UTC().Format(time.RFC3339)
		// Keep existing SeenWhere so human knows what was previously evaluated
	} else if incoming.Decision.Status != "" && incoming.Decision.Status != existing.Decision.Status {
		// Explicit new decision
		existing.Decision.Status = incoming.Decision.Status
		existing.Decision.Reason = incoming.Decision.Reason
		existing.Decision.At = now.UTC().Format(time.RFC3339)
		existing.Decision.SeenWhere = append([]string{}, mergedWhere...)
	}

	doc.Lessons[foundIdx] = existing
	return nil
}

// AdvanceCursor updates or inserts a cursor for sourceID in the document.
func (doc *Document) AdvanceCursor(sourceID, commit string, now time.Time) {
	for i, c := range doc.Cursors {
		if c.SourceID == sourceID {
			doc.Cursors[i].Commit = commit
			doc.Cursors[i].SyncedAt = now.UTC().Format(time.RFC3339)
			return
		}
	}
	doc.Cursors = append(doc.Cursors, Cursor{
		SourceID: sourceID,
		Commit:   commit,
		SyncedAt: now.UTC().Format(time.RFC3339),
	})
}

// MarshalDocument serializes a Document to YAML with 2-space indentation.
func MarshalDocument(doc *Document) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

// UnmarshalDocument deserializes YAML into a Document and validates it.
func UnmarshalDocument(data []byte) (*Document, error) {
	var doc Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if err := ValidateDocument(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// LoadDocument reads and validates a distill.yaml file from disk.
func LoadDocument(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return UnmarshalDocument(data)
}

// SaveDocument saves a Document to disk atomically, creating directories if needed.
func SaveDocument(path string, doc *Document) error {
	if err := ValidateDocument(doc); err != nil {
		return err
	}
	data, err := MarshalDocument(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
