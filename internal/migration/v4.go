package migration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/vantt/mcp-skill-hub/internal/distill"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
)

var knownShortCommits = map[string]string{
	// openclaw
	"96370cf438dc": "96370cf438dc9d67d2a13b08c14eb4142fecb767",
	"be052821d69":  "be052821d69c581a94c39f91623419bdfc1093b1",
	"c9da900a961":  "c9da900a961e349fbcecd87f98620db410d95165",
	"777421df553":  "777421df553c33b1a6ccf87014f05dcee14f8f79",
	"e09a265b111":  "e09a265b11112623addd4de2167ee6b34cd82719",
	// superpowers
	"8ca22dba9a94": "8ca22dba9a94f28898bbce59f2537ff4d87c747d",
	"517a9c6":      "517a9c64191145ed69afbff4b4785c751eb65dbb",
	"030a222af19c": "030a222af19c1f3a93c6eb876a7422d8e4fc0162",
	"e8a9748a3fa9": "e8a9748a3fa9ecd1e480ace6cb3ea073ee58d899",
	"b9e75dd":      "b9e75dddec7a384f42ce08532ec17bb1ef5d9459",
	"caa1826cbade": "caa1826cbadeb88f88c7ad7b3f66178cba01e57d",
	"e74961c11048": "e74961c110488b52169f0854eea47c8fcee15e23",
}

func expandShortCommit(commit string, cursors []distill.Cursor) string {
	if len(commit) == 40 {
		return commit
	}
	if full, ok := knownShortCommits[commit]; ok {
		return full
	}
	for _, c := range cursors {
		if strings.HasPrefix(c.Commit, commit) {
			return c.Commit
		}
	}
	if len(commit) < 40 {
		return commit + strings.Repeat("0", 40-len(commit))
	}
	return commit[:40]
}

func expandEvidence(raw string, cursors []distill.Cursor) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "usage:") {
		return raw
	}
	atIdx := strings.Index(raw, "@")
	if atIdx <= 0 {
		return raw
	}
	repo := raw[:atIdx]
	remainder := raw[atIdx+1:]
	colonIdx := strings.Index(remainder, ":")
	var commit, path string
	if colonIdx >= 0 {
		commit = remainder[:colonIdx]
		path = remainder[colonIdx+1:]
	} else {
		commit = remainder
	}
	expandedCommit := expandShortCommit(commit, cursors)
	if path != "" {
		return fmt.Sprintf("%s@%s:%s", repo, expandedCommit, path)
	}
	return fmt.Sprintf("%s@%s", repo, expandedCommit)
}

func convertGoal(raw any, defaultID string) string {
	if raw == nil {
		return "Distillation learnings for " + defaultID
	}
	switch g := raw.(type) {
	case string:
		if strings.TrimSpace(g) != "" {
			return strings.TrimSpace(g)
		}
	case map[string]any:
		if purpose, ok := g["purpose"].(string); ok && strings.TrimSpace(purpose) != "" {
			return strings.TrimSpace(purpose)
		}
		if desc, ok := g["description"].(string); ok && strings.TrimSpace(desc) != "" {
			return strings.TrimSpace(desc)
		}
	}
	return "Distillation learnings for " + defaultID
}

func convertCursors(raw any) []distill.Cursor {
	if raw == nil {
		return nil
	}
	var res []distill.Cursor
	switch c := raw.(type) {
	case map[string]any:
		for srcID, val := range c {
			res = append(res, distill.Cursor{
				SourceID: srcID,
				Commit:   fmt.Sprintf("%v", val),
			})
		}
	case []any:
		for _, item := range c {
			if itemMap, isMap := item.(map[string]any); isMap {
				srcID, _ := itemMap["source_id"].(string)
				commit, _ := itemMap["commit"].(string)
				syncedAt, _ := itemMap["synced_at"].(string)
				if srcID != "" {
					res = append(res, distill.Cursor{
						SourceID: srcID,
						Commit:   commit,
						SyncedAt: syncedAt,
					})
				}
			}
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].SourceID < res[j].SourceID
	})
	return res
}

func convertCoverage(raw any) []distill.CoverageItem {
	if raw == nil {
		return nil
	}
	var res []distill.CoverageItem
	switch cov := raw.(type) {
	case map[string]any:
		for _, srcCovRaw := range cov {
			srcCov, isMap := srcCovRaw.(map[string]any)
			if !isMap {
				continue
			}
			if readRaw, hasRead := srcCov["read"].([]any); hasRead {
				for _, r := range readRaw {
					p := strings.TrimSpace(fmt.Sprintf("%v", r))
					if p != "" {
						res = append(res, distill.CoverageItem{Resource: p, Status: "analyzed"})
					}
				}
			}
			if notReadRaw, hasNotRead := srcCov["not_read"].([]any); hasNotRead {
				for _, nr := range notReadRaw {
					if itemMap, isMap := nr.(map[string]any); isMap {
						p, _ := itemMap["path"].(string)
						reason, _ := itemMap["reason"].(string)
						if strings.TrimSpace(p) != "" {
							res = append(res, distill.CoverageItem{
								Resource: strings.TrimSpace(p),
								Status:   "skipped",
								Reason:   strings.TrimSpace(reason),
							})
						}
					}
				}
			}
		}
	case []any:
		for _, item := range cov {
			if itemMap, isMap := item.(map[string]any); isMap {
				p, _ := itemMap["resource"].(string)
				status, _ := itemMap["status"].(string)
				reason, _ := itemMap["reason"].(string)
				blocking, _ := itemMap["blocking"].(bool)
				if p != "" && status != "" {
					res = append(res, distill.CoverageItem{
						Resource: p,
						Status:   status,
						Reason:   reason,
						Blocking: blocking,
					})
				}
			}
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Resource < res[j].Resource
	})
	return res
}

func convertLessonDecision(dRaw map[string]any, whereList []string) distill.Decision {
	decision := distill.Decision{Status: "candidate"}
	if dRaw == nil {
		return decision
	}
	state, _ := dRaw["state"].(string)
	status, _ := dRaw["status"].(string)
	if status == "" {
		status = state
	}
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "planned", "ported", "rejected":
		decision.Status = status
	default:
		decision.Status = "candidate"
	}

	if r, hasR := dRaw["reason"].(string); hasR {
		decision.Reason = strings.TrimSpace(r)
	}
	if at, hasAt := dRaw["at"].(string); hasAt {
		if _, err := time.Parse(time.RFC3339, at); err == nil {
			decision.At = at
		}
	}
	if decision.Status != "candidate" {
		decision.SeenWhere = append([]string{}, whereList...)
	}
	return decision
}

func convertLessons(raw any, cursors []distill.Cursor) []distill.Lesson {
	lessonsRaw, ok := raw.([]any)
	if !ok {
		return nil
	}
	seenKeys := make(map[string]bool)
	var res []distill.Lesson
	for _, lRaw := range lessonsRaw {
		lMap, isMap := lRaw.(map[string]any)
		if !isMap {
			continue
		}
		key, _ := lMap["key"].(string)
		key = strings.TrimSpace(key)
		if key == "" || seenKeys[key] {
			continue
		}
		what, _ := lMap["what"].(string)
		what = strings.TrimSpace(what)
		if what == "" {
			continue
		}
		notable, _ := lMap["notable"].(string)
		notable = strings.TrimSpace(notable)
		if notable == "" {
			notable = what
		}
		contrast, _ := lMap["contrast"].(string)

		var whereList []string
		if wRaw, hasW := lMap["where"].([]any); hasW {
			for _, w := range wRaw {
				expanded := expandEvidence(fmt.Sprintf("%v", w), cursors)
				if expanded != "" && !slices.Contains(whereList, expanded) {
					whereList = append(whereList, expanded)
				}
			}
		}
		if len(whereList) == 0 {
			continue
		}
		slices.Sort(whereList)

		dRaw, _ := lMap["decision"].(map[string]any)
		decision := convertLessonDecision(dRaw, whereList)

		res = append(res, distill.Lesson{
			Key:      key,
			What:     what,
			Notable:  notable,
			Contrast: contrast,
			Where:    whereList,
			Decision: decision,
		})
		seenKeys[key] = true
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Key < res[j].Key
	})
	return res
}

// ConvertSkillDistillYAML converts Phase-0 or legacy format distill.yaml bytes
// into a validated target D7 distill.Document.
func ConvertSkillDistillYAML(data []byte, defaultID string) (*distill.Document, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse distill yaml: %w", err)
	}

	doc := &distill.Document{
		Goal:     convertGoal(raw["goal"], defaultID),
		Cursors:  convertCursors(raw["cursors"]),
		Coverage: convertCoverage(raw["coverage"]),
	}
	doc.Lessons = convertLessons(raw["lessons"], doc.Cursors)

	if err := distill.ValidateDocument(doc); err != nil {
		return nil, fmt.Errorf("validate converted document: %w", err)
	}
	return doc, nil
}

func bumpSchemaVersionMarker(root string) ([]mutation.Change, []FileDiff, error) {
	markerPath := filepath.Join(root, ".skillhub", "schema-version")
	beforeBytes, err := os.ReadFile(markerPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	markerRel := filepath.ToSlash(filepath.Join(".skillhub", "schema-version"))
	newMarker := []byte("4\n")

	var changes []mutation.Change
	var diffs []FileDiff

	var beforeDigest string
	if len(beforeBytes) > 0 {
		sum := sha256.Sum256(beforeBytes)
		beforeDigest = "sha256:" + hex.EncodeToString(sum[:])
	}
	changes = append(changes, mutation.Change{
		Path:         markerRel,
		Contents:     newMarker,
		BeforeDigest: beforeDigest,
	})
	diffs = append(diffs, FileDiff{
		Path:   markerRel,
		Before: string(beforeBytes),
		After:  string(newMarker),
		Diff:   fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ -1 +1 @@\n-%s+%s", markerRel, markerRel, string(beforeBytes), string(newMarker)),
	})
	return changes, diffs, nil
}

func collectExistingSkillDistillDocs(root string) (map[string]*distill.Document, map[string][]byte, error) {
	skillDocs := make(map[string]*distill.Document)
	existingBytes := make(map[string][]byte)
	skillsDir := filepath.Join(root, "skills")
	if _, statErr := os.Stat(skillsDir); errors.Is(statErr, os.ErrNotExist) {
		return skillDocs, existingBytes, nil
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
			rel = filepath.ToSlash(rel)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return fmt.Errorf("read %s: %w", rel, readErr)
			}
			skillDir := filepath.Dir(filepath.Dir(rel))
			skillID := filepath.Base(skillDir)
			doc, convErr := ConvertSkillDistillYAML(data, skillID)
			if convErr != nil {
				return fmt.Errorf("convert %s: %w", rel, convErr)
			}
			skillDocs[skillDir] = doc
			existingBytes[skillDir] = data
		}
		return nil
	})
	return skillDocs, existingBytes, err
}

func generateDistillChanges(skillDocs map[string]*distill.Document, existingBytes map[string][]byte) ([]mutation.Change, []FileDiff, error) {
	var changes []mutation.Change
	var diffs []FileDiff

	var sortedSkills []string
	for sDir := range skillDocs {
		sortedSkills = append(sortedSkills, sDir)
	}
	sort.Strings(sortedSkills)

	for _, sDir := range sortedSkills {
		doc := skillDocs[sDir]
		newYAML, mErr := distill.MarshalDocument(doc)
		if mErr != nil {
			return nil, nil, fmt.Errorf("marshal %s distill.yaml: %w", sDir, mErr)
		}
		distillRelPath := filepath.ToSlash(filepath.Join(sDir, ".meta", "distill.yaml"))
		oldData := existingBytes[sDir]
		if oldData == nil {
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
		} else if !bytes.Equal(oldData, newYAML) {
			sum := sha256.Sum256(oldData)
			changes = append(changes, mutation.Change{
				Path:         distillRelPath,
				Contents:     newYAML,
				BeforeDigest: "sha256:" + hex.EncodeToString(sum[:]),
			})
			diffs = append(diffs, FileDiff{
				Path:   distillRelPath,
				Before: string(oldData),
				After:  string(newYAML),
				Diff:   fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ converted @@\n", distillRelPath, distillRelPath),
			})
		}
	}
	return changes, diffs, nil
}

func deleteLegacyDistillDirs(root string) ([]mutation.Change, []FileDiff, error) {
	var changes []mutation.Change
	var diffs []FileDiff
	legacyDirs := []string{
		"sources/intake",
		"sources/skills",
		"distill/runs",
		"distill/comparisons",
		"distill/sources",
		"distill/insights",
		"distill/proposals",
		"distill/incorporations",
		"distill/outcomes",
		"distill/skills",
	}

	for _, lDir := range legacyDirs {
		fullPath := filepath.Join(root, filepath.FromSlash(lDir))
		if _, statErr := os.Stat(fullPath); errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(fullPath, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("walk legacy %s: %w", path, walkErr)
			}
			if d.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return fmt.Errorf("rel path %s: %w", path, relErr)
			}
			rel = filepath.ToSlash(rel)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return fmt.Errorf("read legacy %s: %w", rel, readErr)
			}
			sum := sha256.Sum256(data)
			changes = append(changes, mutation.Change{
				Path:         rel,
				Delete:       true,
				BeforeDigest: "sha256:" + hex.EncodeToString(sum[:]),
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

	skillDocs, existingBytes, err := collectExistingSkillDistillDocs(root)
	if err != nil {
		return nil, nil, err
	}

	if err := harvestLegacyDistillEntities(root, skillDocs); err != nil {
		return nil, nil, err
	}

	docChanges, docDiffs, err := generateDistillChanges(skillDocs, existingBytes)
	if err != nil {
		return nil, nil, err
	}
	changes = append(changes, docChanges...)
	diffs = append(diffs, docDiffs...)

	delChanges, delDiffs, err := deleteLegacyDistillDirs(root)
	if err != nil {
		return nil, nil, err
	}
	changes = append(changes, delChanges...)
	diffs = append(diffs, delDiffs...)

	return changes, diffs, nil
}

func harvestInsights(root string, ensureDoc func(string) *distill.Document) error {
	insightsDir := filepath.Join(root, "distill", "insights")
	if _, statErr := os.Stat(insightsDir); errors.Is(statErr, os.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(insightsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk insights %s: %w", path, err)
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
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
			return fmt.Errorf("parse insight %s: missing skill_id", path)
		}
		doc := ensureDoc(ins.SkillID)
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
		where := []string{fmt.Sprintf("usage:%s", strings.ToLower(ins.ID))}

		decision := distill.Decision{Status: "candidate"}
		switch strings.ToLower(ins.Status) {
		case "rejected":
			decision.Status = "rejected"
			decision.Reason = ins.DecisionRationale
		case "accepted":
			decision.Status = "planned"
		case "ported", "incorporated":
			decision.Status = "ported"
		}
		if decision.Status != "candidate" {
			decision.SeenWhere = append([]string{}, where...)
		}

		if applyErr := doc.ApplyLesson(distill.Lesson{
			Key:      key,
			What:     what,
			Notable:  notable,
			Where:    where,
			Decision: decision,
		}, time.Now().UTC()); applyErr != nil {
			return fmt.Errorf("apply insight lesson %s (%s): %w", path, key, applyErr)
		}
		return nil
	})
}

func harvestObservations(root string, skillDocs map[string]*distill.Document, ensureDoc func(string) *distill.Document) error {
	sourcesDir := filepath.Join(root, "distill", "sources")
	if _, statErr := os.Stat(sourcesDir); errors.Is(statErr, os.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(sourcesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk sources %s: %w", path, err)
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read observation %s: %w", path, readErr)
		}
		var obs struct {
			ID        string `yaml:"id"`
			SourceID  string `yaml:"source_id"`
			StableKey string `yaml:"stable_key"`
			Status    string `yaml:"status"`
			What      string `yaml:"what"`
			Evidence  []struct {
				Path     string `yaml:"path"`
				Locator  string `yaml:"locator"`
				Revision struct {
					Value string `yaml:"value"`
				} `yaml:"revision"`
			} `yaml:"evidence"`
		}
		if unmarshalErr := yaml.Unmarshal(data, &obs); unmarshalErr != nil {
			return fmt.Errorf("parse observation %s: %w", path, unmarshalErr)
		}
		if strings.TrimSpace(obs.SourceID) == "" {
			return fmt.Errorf("parse observation %s: missing source_id", path)
		}

		var targetDoc *distill.Document
		for _, doc := range skillDocs {
			for _, c := range doc.Cursors {
				if c.SourceID == obs.SourceID {
					targetDoc = doc
					break
				}
			}
			if targetDoc != nil {
				break
			}
		}
		if targetDoc == nil {
			for _, doc := range skillDocs {
				targetDoc = doc
				break
			}
		}
		if targetDoc == nil {
			targetDoc = ensureDoc("default")
		}

		key := strings.ToLower(strings.ReplaceAll(obs.StableKey, "_", "-"))
		if key == "" {
			key = strings.ToLower(strings.ReplaceAll(obs.ID, "_", "-"))
		}
		what := strings.TrimSpace(obs.What)
		if what == "" {
			what = "Observation from " + obs.SourceID
		}
		notable := "Source finding from " + obs.SourceID
		var whereList []string
		for _, ev := range obs.Evidence {
			commit := ev.Revision.Value
			if len(commit) < 40 {
				commit = expandShortCommit(commit, targetDoc.Cursors)
			}
			p := ev.Path
			if p == "" {
				p = "README.md"
			}
			ref := fmt.Sprintf("%s@%s:%s", obs.SourceID, commit, p)
			if ev.Locator != "" {
				ref += "#" + ev.Locator
			}
			whereList = append(whereList, ref)
		}
		if len(whereList) == 0 {
			commit := expandShortCommit("0000000", targetDoc.Cursors)
			whereList = append(whereList, fmt.Sprintf("%s@%s:README.md", obs.SourceID, commit))
		}

		decision := distill.Decision{Status: "candidate"}
		if obs.Status == "removed" || obs.Status == "superseded" {
			decision.Status = "rejected"
			decision.Reason = "tombstoned: upstream removed or superseded this finding"
			decision.SeenWhere = append([]string{}, whereList...)
		}

		if applyErr := targetDoc.ApplyLesson(distill.Lesson{
			Key:      key,
			What:     what,
			Notable:  notable,
			Where:    whereList,
			Decision: decision,
		}, time.Now().UTC()); applyErr != nil {
			return fmt.Errorf("apply observation lesson %s (%s): %w", path, key, applyErr)
		}
		return nil
	})
}

func harvestRuns(root string, skillDocs map[string]*distill.Document) error {
	runsDir := filepath.Join(root, "distill", "runs")
	if _, statErr := os.Stat(runsDir); errors.Is(statErr, os.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(runsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk runs %s: %w", path, err)
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read run %s: %w", path, readErr)
		}
		var run struct {
			Coverage []struct {
				Resource string `yaml:"resource"`
				Status   string `yaml:"status"`
				Reason   string `yaml:"reason"`
				Blocking bool   `yaml:"blocking"`
			} `yaml:"coverage"`
		}
		if unmarshalErr := yaml.Unmarshal(data, &run); unmarshalErr != nil {
			return fmt.Errorf("parse run %s: %w", path, unmarshalErr)
		}
		for _, cov := range run.Coverage {
			status := strings.ToLower(cov.Status)
			if status == "unreadable" || status == "deferred" {
				status = "skipped"
			}
			if status != "analyzed" && status != "skipped" && status != "deferred" {
				continue
			}
			for _, doc := range skillDocs {
				found := false
				for _, existing := range doc.Coverage {
					if existing.Resource == cov.Resource {
						found = true
						break
					}
				}
				if !found {
					doc.Coverage = append(doc.Coverage, distill.CoverageItem{
						Resource: cov.Resource,
						Status:   status,
						Reason:   cov.Reason,
						Blocking: cov.Blocking,
					})
				}
			}
		}
		return nil
	})
}

func harvestLegacyDistillEntities(root string, skillDocs map[string]*distill.Document) error {
	findSkillDir := func(skillID string) string {
		for dir := range skillDocs {
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
		if doc, ok := skillDocs[sDir]; ok {
			return doc
		}
		doc := &distill.Document{
			Goal: "Distillation learnings for " + skillID,
		}
		skillDocs[sDir] = doc
		return doc
	}

	if err := harvestInsights(root, ensureDoc); err != nil {
		return err
	}
	if err := harvestObservations(root, skillDocs, ensureDoc); err != nil {
		return err
	}
	return harvestRuns(root, skillDocs)
}
