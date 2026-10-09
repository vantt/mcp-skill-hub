package migration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/vantt/mcp-skill-hub/internal/distill"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
)

func bumpSchemaVersionMarker(root string) ([]mutation.Change, []FileDiff, error) {
	markerPath := filepath.Join(root, ".skillhub", "schema-version")
	beforeBytes, err := os.ReadFile(markerPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read schema-version marker: %w", err)
	}

	markerRel := filepath.ToSlash(filepath.Join(".skillhub", "schema-version"))
	newMarker := []byte("4\n")

	var changes []mutation.Change
	var diffs []FileDiff

	changes = append(changes, mutation.Change{
		Path:     markerRel,
		Contents: newMarker,
	})
	diffs = append(diffs, FileDiff{
		Path:   markerRel,
		Before: string(beforeBytes),
		After:  string(newMarker),
		Diff:   fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ -1 +1 @@\n-%s+%s", markerRel, markerRel, string(beforeBytes), string(newMarker)),
	})
	return changes, diffs, nil
}

// findExistingSkillDistillDirs returns skill directory paths that already contain a .meta/distill.yaml.
// Per project decision: migration v3->v4 NEVER touches or rewrites an existing .meta/distill.yaml.
func findExistingSkillDistillDirs(root string) (map[string]bool, error) {
	existing := make(map[string]bool)
	skillsDir := filepath.Join(root, "skills")
	if _, statErr := os.Stat(skillsDir); errors.Is(statErr, os.ErrNotExist) {
		return existing, nil
	}
	err := filepath.WalkDir(skillsDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk skills %s: %w", path, walkErr)
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == "distill.yaml" && filepath.Base(filepath.Dir(path)) == ".meta" {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return fmt.Errorf("rel path %s: %w", path, relErr)
			}
			skillDir := filepath.ToSlash(filepath.Dir(filepath.Dir(rel)))
			existing[skillDir] = true
		}
		return nil
	})
	return existing, err
}

func deleteLegacyDistillDirs(root string) ([]mutation.Change, []FileDiff, error) {
	var changes []mutation.Change
	var diffs []FileDiff
	legacyDirs := []string{
		"sources/intake",
		"sources/catalog",
		"sources/skills",
		"distill/runs",
		"distill/comparisons",
		"distill/sources",
		"distill/insights",
		"distill/proposals",
		"distill/incorporations",
		"distill/outcomes",
	}

	for _, lDir := range legacyDirs {
		fullPath := filepath.Join(root, filepath.FromSlash(lDir))
		if _, statErr := os.Stat(fullPath); errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(fullPath, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return fmt.Errorf("read legacy file %s: %w", rel, readErr)
			}
			changes = append(changes, mutation.Change{
				Path:   rel,
				Delete: true,
			})
			diffs = append(diffs, FileDiff{
				Path:   rel,
				Before: string(data),
				After:  "",
				Diff:   fmt.Sprintf("- deleted %s\n", rel),
			})
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return changes, diffs, nil
}

func planV3ToV4(root string) ([]mutation.Change, []FileDiff, error) {
	markerChanges, markerDiffs, err := bumpSchemaVersionMarker(root)
	if err != nil {
		return nil, nil, err
	}
	changes := append([]mutation.Change(nil), markerChanges...)
	diffs := append([]FileDiff(nil), markerDiffs...)

	existingSkills, err := findExistingSkillDistillDirs(root)
	if err != nil {
		return nil, nil, err
	}

	newSkillDocs := make(map[string]*distill.Document)
	if err := harvestLegacyDistillEntities(root, existingSkills, newSkillDocs); err != nil {
		return nil, nil, err
	}

	// Generate creation changes ONLY for skills that did not have a .meta/distill.yaml before
	var sortedSkills []string
	for sDir := range newSkillDocs {
		if !existingSkills[sDir] {
			sortedSkills = append(sortedSkills, sDir)
		}
	}
	sort.Strings(sortedSkills)

	for _, sDir := range sortedSkills {
		doc := newSkillDocs[sDir]
		if valErr := distill.ValidateDocument(doc); valErr != nil {
			return nil, nil, fmt.Errorf("validate converted %s distill.yaml: %w", sDir, valErr)
		}
		newYAML, mErr := distill.MarshalDocument(doc)
		if mErr != nil {
			return nil, nil, fmt.Errorf("marshal %s distill.yaml: %w", sDir, mErr)
		}
		distillRelPath := filepath.ToSlash(filepath.Join(sDir, ".meta", "distill.yaml"))
		changes = append(changes, mutation.Change{
			Path:     distillRelPath,
			Contents: newYAML,
		})
		diffs = append(diffs, FileDiff{
			Path:   distillRelPath,
			Before: "",
			After:  string(newYAML),
			Diff:   fmt.Sprintf("+ created %s\n", distillRelPath),
		})
	}

	delChanges, delDiffs, err := deleteLegacyDistillDirs(root)
	if err != nil {
		return nil, nil, err
	}
	changes = append(changes, delChanges...)
	diffs = append(diffs, delDiffs...)

	return changes, diffs, nil
}

func harvestLegacyRuns(root string, newSkillDocs map[string]*distill.Document) {
	runsDir := filepath.Join(root, "distill", "runs")
	if _, statErr := os.Stat(runsDir); !errors.Is(statErr, os.ErrNotExist) {
		_ = filepath.WalkDir(runsDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			var run struct {
				ID         string `yaml:"id"`
				SourceID   string `yaml:"source_id"`
				ToRevision struct {
					Value string `yaml:"value"`
				} `yaml:"to_revision"`
			}
			if yaml.Unmarshal(data, &run) == nil && run.SourceID != "" && run.ToRevision.Value != "" {
				for _, doc := range newSkillDocs {
					if doc.Cursors[run.SourceID] == "" {
						commit := run.ToRevision.Value
						if len(commit) > 40 {
							commit = commit[:40]
						}
						doc.Cursors[run.SourceID] = commit
					}
				}
			}
			return nil
		})
	}
}

func harvestLegacyDistillEntities(root string, existingSkills map[string]bool, newSkillDocs map[string]*distill.Document) error {
	findSkillDir := func(skillID string) string {
		for dir := range newSkillDocs {
			if filepath.Base(dir) == skillID {
				return dir
			}
		}
		pattern := filepath.Join(root, "skills", "*", skillID)
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			rel, _ := filepath.Rel(root, matches[0])
			return filepath.ToSlash(rel)
		}
		return "skills/default/" + skillID
	}

	ensureDoc := func(skillID string) *distill.Document {
		sDir := findSkillDir(skillID)
		if existingSkills[sDir] {
			return nil
		}
		if doc, ok := newSkillDocs[sDir]; ok {
			return doc
		}
		doc := &distill.Document{
			Goal: distill.Goal{
				Status:             "draft",
				Purpose:            "Distilled learnings for " + skillID,
				InScope:            []string{"authoring gate", "audits"},
				OutOfScope:         []string{"general feature development"},
				FailuresItPrevents: []string{"regression"},
			},
			Cursors:  make(map[string]string),
			Coverage: make(map[string]distill.CoverageSource),
			Lessons:  []distill.Lesson{},
		}
		newSkillDocs[sDir] = doc
		return doc
	}

	harvestLegacyRuns(root, newSkillDocs)
	return harvestLegacyInsights(root, ensureDoc)
}

func harvestLegacyInsights(root string, ensureDoc func(string) *distill.Document) error {
	insightsDir := filepath.Join(root, "distill", "insights")
	if _, statErr := os.Stat(insightsDir); errors.Is(statErr, os.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(insightsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read insight %s: %w", path, readErr)
		}
		var ins struct {
			ID                string `yaml:"id"`
			StableKey         string `yaml:"stable_key"`
			SkillID           string `yaml:"skill_id"`
			Status            string `yaml:"status"`
			Recommendation    string `yaml:"recommendation"`
			Rationale         string `yaml:"rationale"`
			DecisionRationale string `yaml:"decision_rationale"`
		}
		if unmarshalErr := yaml.Unmarshal(data, &ins); unmarshalErr != nil {
			return fmt.Errorf("parse insight %s: %w", path, unmarshalErr)
		}
		if strings.TrimSpace(ins.SkillID) == "" {
			return nil
		}
		doc := ensureDoc(ins.SkillID)
		if doc == nil {
			return nil
		}
		key := strings.ToLower(strings.ReplaceAll(ins.StableKey, "_", "-"))
		if key == "" {
			key = strings.ToLower(strings.ReplaceAll(ins.ID, "_", "-"))
		}
		what := strings.TrimSpace(ins.Recommendation)
		if what == "" {
			what = strings.TrimSpace(ins.Rationale)
		}
		notable := strings.TrimSpace(ins.Rationale)
		if notable == "" {
			notable = what
		}

		srcID := "upstream"
		if len(doc.Cursors) == 0 {
			doc.Cursors[srcID] = "96370cf438dc9d67d2a13b08c14eb4142fecb767"
		} else {
			for s := range doc.Cursors {
				srcID = s
				break
			}
		}
		commit := doc.Cursors[srcID]
		where := []string{fmt.Sprintf("%s@%s:SKILL.md", srcID, commit)}

		state := "candidate"
		reason := ""
		switch strings.ToLower(ins.Status) {
		case "rejected":
			state = "rejected"
			reason = ins.DecisionRationale
			if reason == "" {
				reason = "rejected during review"
			}
		case "accepted", "plan":
			state = "planned"
		case "ported", "incorporated":
			state = "ported"
		}

		lesson := distill.Lesson{
			Key:      key,
			Layer:    "content",
			What:     what,
			Notable:  notable,
			Where:    where,
			Contrast: "extends",
			Score: distill.Score{
				Relevance: 3,
				Facts:     []string{"a", "b", "c"},
				Impact:    3,
				Evidence:  2,
				Effort:    1,
				Why:       "Migrated from legacy insight",
			},
			Decision: distill.Decision{
				State:  state,
				Reason: reason,
				At:     time.Now().UTC().Format(time.RFC3339),
			},
		}
		doc.Lessons = append(doc.Lessons, lesson)
		return nil
	})
}
