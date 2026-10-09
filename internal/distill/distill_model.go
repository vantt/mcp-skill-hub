package distill

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	KeyPattern         = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	CommitPattern      = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	CommitWherePattern = regexp.MustCompile(`^([a-z0-9-]+)@([0-9a-f]{7,40})$`)
	PathWherePattern   = regexp.MustCompile(`^([a-z0-9-]+)@([0-9a-f]{7,40}):([^#\s]+)(?:#L(\d+)(?:-L(\d+))?)?$`)
	AlsoFitsPattern    = regexp.MustCompile(`^(hub|new-skill:[a-z0-9-]+|[a-z0-9]+(?:-[a-z0-9]+)*)$`)
	StatusPattern      = regexp.MustCompile(`^(removed|superseded-by:[a-z0-9-]+)$`)

	// EvidencePattern accepts either commit-level or path-level citations (7-40 hex SHA).
	EvidencePattern  = regexp.MustCompile(`^[a-z0-9-]+@[0-9a-f]{7,40}(?::[^#\s]+(?:#L\d+(?:-L\d+)?)?)?$`)
	LessonKeyPattern = KeyPattern
)

var (
	ValidLayers         = []string{"enforcement", "content", "history", "validation", "craft", "wording", "ecosystem"}
	ValidContrasts      = []string{"new", "extends", "contradicts", "already-covered"}
	ValidDecisionStates = []string{"candidate", "planned", "ported", "rejected"}
	ValidGoalStatuses   = []string{"draft", "confirmed"}
	ValidImpactFacts    = []string{"a", "b", "c", "d", "e"}
)

// Document models one skill's hub-side distillation knowledge in .meta/distill.yaml.
// The format adheres strictly to .claude/skills/distill-lab/scripts/distill.py.
type Document struct {
	Goal     Goal                      `yaml:"goal" json:"goal"`
	Cursors  map[string]string         `yaml:"cursors" json:"cursors"`
	Coverage map[string]CoverageSource `yaml:"coverage,omitempty" json:"coverage,omitempty"`
	Lessons  []Lesson                  `yaml:"lessons" json:"lessons"`
}

// Goal defines the objective and bounds of distillation for a skill.
type Goal struct {
	Status             string   `yaml:"status" json:"status"`
	Purpose            string   `yaml:"purpose" json:"purpose"`
	InScope            []string `yaml:"in_scope" json:"in_scope"`
	OutOfScope         []string `yaml:"out_of_scope" json:"out_of_scope"`
	FailuresItPrevents []string `yaml:"failures_it_prevents" json:"failures_it_prevents"`
}

// CoverageSource records read and unread resources for one source.
type CoverageSource struct {
	Read    []string              `yaml:"read,omitempty" json:"read,omitempty"`
	NotRead []CoverageNotReadItem `yaml:"not_read,omitempty" json:"not_read,omitempty"`
}

// CoverageNotReadItem describes an unread resource with reason.
type CoverageNotReadItem struct {
	Path   string `yaml:"path" json:"path"`
	Reason string `yaml:"reason" json:"reason"`
}

// Lesson represents one distilled knowledge item anchored to a skill.
type Lesson struct {
	Key        string   `yaml:"key" json:"key"`
	Layer      string   `yaml:"layer" json:"layer"`
	What       string   `yaml:"what" json:"what"`
	Notable    string   `yaml:"notable" json:"notable"`
	Where      []string `yaml:"where" json:"where"`
	Contrast   string   `yaml:"contrast" json:"contrast"`
	Score      Score    `yaml:"score" json:"score"`
	FinalScore *float64 `yaml:"final_score,omitempty" json:"final_score,omitempty"`
	AlsoFits   []string `yaml:"also_fits,omitempty" json:"also_fits,omitempty"`
	Status     string   `yaml:"status,omitempty" json:"status,omitempty"`
	FoundBy    string   `yaml:"found_by,omitempty" json:"found_by,omitempty"`
	Decision   Decision `yaml:"decision" json:"decision"`
}

// Score contains the scorecard evaluation metrics for a lesson.
type Score struct {
	Relevance int      `yaml:"relevance" json:"relevance"`
	Facts     []string `yaml:"facts" json:"facts"`
	Impact    int      `yaml:"impact" json:"impact"`
	Evidence  int      `yaml:"evidence" json:"evidence"`
	Effort    int      `yaml:"effort" json:"effort"`
	Why       string   `yaml:"why" json:"why"`
}

// Decision records the human or agent review disposition for one lesson.
type Decision struct {
	State  string `yaml:"state" json:"state"` // candidate | planned | ported | rejected
	Reason string `yaml:"reason,omitempty" json:"reason,omitempty"`
	At     string `yaml:"at,omitempty" json:"at,omitempty"`
}

// ImpactOf returns the impact score (0-5) based on the presence of fact 'a'.
func ImpactOf(facts []string) int {
	if slices.Contains(facts, "a") {
		return len(facts)
	}
	return 0
}

// FinalScore computes the final score: relevance * impact * evidence / effort rounded to 2 decimals.
func FinalScore(s Score) float64 {
	impact := ImpactOf(s.Facts)
	if s.Effort <= 0 {
		return 0
	}
	val := float64(s.Relevance*impact*s.Evidence) / float64(s.Effort)
	return math.Round(val*100) / 100
}

// ValidateDocument validates the structure and fields of a Document against distill.py rules.
func ValidateDocument(doc *Document) error {
	if doc == nil {
		return errors.New("distill document is nil")
	}

	// Goal validation
	if !slices.Contains(ValidGoalStatuses, doc.Goal.Status) {
		return fmt.Errorf("goal.status must be draft or confirmed, got %q", doc.Goal.Status)
	}
	if strings.TrimSpace(doc.Goal.Purpose) == "" {
		return errors.New("goal.purpose must be non-empty text")
	}
	if len(doc.Goal.InScope) == 0 {
		return errors.New("goal.in_scope must be a non-empty list of text")
	}
	for i, s := range doc.Goal.InScope {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("goal.in_scope[%d] must be non-empty text", i)
		}
	}
	if len(doc.Goal.OutOfScope) == 0 {
		return errors.New("goal.out_of_scope must be a non-empty list of text")
	}
	for i, s := range doc.Goal.OutOfScope {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("goal.out_of_scope[%d] must be non-empty text", i)
		}
	}
	if len(doc.Goal.FailuresItPrevents) == 0 {
		return errors.New("goal.failures_it_prevents must be a non-empty list of text")
	}
	for i, s := range doc.Goal.FailuresItPrevents {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("goal.failures_it_prevents[%d] must be non-empty text", i)
		}
	}

	// Cursors validation
	if len(doc.Cursors) == 0 {
		return errors.New("cursors: must map source id to commit")
	}
	for src, commit := range doc.Cursors {
		if strings.TrimSpace(src) == "" {
			return errors.New("cursors: source id must be non-empty")
		}
		if !CommitPattern.MatchString(commit) {
			return fmt.Errorf("cursors.%s: not a commit sha (%q)", src, commit)
		}
	}

	// Lessons validation
	seenKeys := make(map[string]bool, len(doc.Lessons))
	for i, l := range doc.Lessons {
		if err := ValidateLesson(&l, doc.Cursors); err != nil {
			return fmt.Errorf("lessons[%d] (%s): %w", i, l.Key, err)
		}
		if seenKeys[l.Key] {
			return fmt.Errorf("duplicate lesson key %q", l.Key)
		}
		seenKeys[l.Key] = true
	}
	return nil
}

// ValidateLesson checks all fields of a lesson against distill.py rules.
func ValidateLesson(lesson *Lesson, cursors map[string]string) error {
	if !KeyPattern.MatchString(lesson.Key) {
		return fmt.Errorf("invalid lesson key %q: must be kebab-case", lesson.Key)
	}
	if !slices.Contains(ValidLayers, lesson.Layer) {
		return fmt.Errorf("layer must be one of %s, got %q", strings.Join(ValidLayers, ", "), lesson.Layer)
	}
	if strings.TrimSpace(lesson.What) == "" {
		return errors.New("what must be non-empty text")
	}
	if strings.TrimSpace(lesson.Notable) == "" {
		return errors.New("notable must be non-empty text")
	}
	if len(lesson.Where) == 0 {
		return errors.New("where must be a non-empty list")
	}
	seenWhere := make(map[string]bool, len(lesson.Where))
	for _, w := range lesson.Where {
		matchCommit := CommitWherePattern.FindStringSubmatch(w)
		matchPath := PathWherePattern.FindStringSubmatch(w)
		if matchCommit == nil && matchPath == nil {
			return fmt.Errorf("bad where %q", w)
		}
		src := ""
		if matchCommit != nil {
			src = matchCommit[1]
		} else if matchPath != nil {
			src = matchPath[1]
		}
		if cursors != nil {
			if _, ok := cursors[src]; !ok {
				return fmt.Errorf("where cites unknown source %q", src)
			}
		}
		if seenWhere[w] {
			return fmt.Errorf("duplicate where entry %q", w)
		}
		seenWhere[w] = true
	}
	if !slices.Contains(ValidContrasts, lesson.Contrast) {
		return fmt.Errorf("contrast must be one of %s, got %q", strings.Join(ValidContrasts, ", "), lesson.Contrast)
	}

	// Score validation
	if lesson.Score.Relevance < 0 || lesson.Score.Relevance > 3 {
		return fmt.Errorf("score.relevance must be an integer 0-3, got %d", lesson.Score.Relevance)
	}
	if lesson.Score.Evidence < 1 || lesson.Score.Evidence > 3 {
		return fmt.Errorf("score.evidence must be an integer 1-3, got %d", lesson.Score.Evidence)
	}
	if lesson.Score.Effort < 1 || lesson.Score.Effort > 3 {
		return fmt.Errorf("score.effort must be an integer 1-3, got %d", lesson.Score.Effort)
	}
	seenFacts := make(map[string]bool, len(lesson.Score.Facts))
	for _, f := range lesson.Score.Facts {
		if !slices.Contains(ValidImpactFacts, f) {
			return fmt.Errorf("score.facts item %q must be one of a-e", f)
		}
		if seenFacts[f] {
			return fmt.Errorf("score.facts has duplicate %q", f)
		}
		seenFacts[f] = true
	}
	if len(lesson.Score.Facts) > 0 && !seenFacts["a"] {
		return errors.New("score.facts without fact a count for nothing; add a or clear the list")
	}
	if strings.TrimSpace(lesson.Score.Why) == "" {
		return errors.New("score.why must explain the weights")
	}

	// Optional fields validation
	for _, fit := range lesson.AlsoFits {
		if !AlsoFitsPattern.MatchString(fit) {
			return fmt.Errorf("also_fits entry %q must be hub, new-skill:<name> or a skill id", fit)
		}
	}
	if lesson.Status != "" && !StatusPattern.MatchString(lesson.Status) {
		return fmt.Errorf("status must be removed or superseded-by:<key>, got %q", lesson.Status)
	}
	if lesson.FoundBy != "" && lesson.FoundBy != "human" {
		return fmt.Errorf("found_by may only be human, got %q", lesson.FoundBy)
	}

	// Decision validation
	if !slices.Contains(ValidDecisionStates, lesson.Decision.State) {
		return fmt.Errorf("decision.state must be one of %s, got %q", strings.Join(ValidDecisionStates, ", "), lesson.Decision.State)
	}
	if lesson.Decision.State == "rejected" && strings.TrimSpace(lesson.Decision.Reason) == "" {
		return errors.New("a rejected decision needs a reason")
	}
	return nil
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
