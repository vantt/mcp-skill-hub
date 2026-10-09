package migration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
)

func migrateSkillYAMLToV3(root string) ([]mutation.Change, []FileDiff, error) {
	var changes []mutation.Change
	var diffs []FileDiff

	skillsDir := filepath.Join(root, "skills")
	err := filepath.WalkDir(skillsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || d.Name() != "skill.meta.yaml" {
			return nil
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		oldContents, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		newRelPath := filepath.ToSlash(filepath.Join(filepath.Dir(relPath), ".meta", "skill.yaml"))
		targetAbsPath := filepath.Join(root, filepath.FromSlash(newRelPath))

		var existingContents []byte
		if data, err := os.ReadFile(targetAbsPath); err == nil {
			existingContents = data
		}

		newContents, err := transformSkillMetaToV3(oldContents, existingContents, relPath, newRelPath)
		if err != nil {
			return err
		}

		oldSum := sha256.Sum256(oldContents)
		changes = append(changes,
			mutation.Change{
				Path:         relPath,
				Delete:       true,
				BeforeDigest: "sha256:" + hex.EncodeToString(oldSum[:]),
			},
		)
		diffs = append(diffs,
			FileDiff{
				Path:   relPath,
				Before: string(oldContents),
				After:  "",
				Diff:   fmt.Sprintf("- deleted %s\n", relPath),
			},
		)

		if existingContents != nil {
			sum := sha256.Sum256(existingContents)
			beforeDigest := "sha256:" + hex.EncodeToString(sum[:])
			changes = append(changes,
				mutation.Change{Path: newRelPath, BeforeDigest: beforeDigest, Contents: newContents},
			)
			diffs = append(diffs,
				FileDiff{
					Path:   newRelPath,
					Before: string(existingContents),
					After:  string(newContents),
					Diff:   fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ merged @@\n", newRelPath, newRelPath),
				},
			)
		} else {
			changes = append(changes,
				mutation.Change{Path: newRelPath, Contents: newContents},
			)
			diffs = append(diffs,
				FileDiff{
					Path:   newRelPath,
					Before: "",
					After:  string(newContents),
					Diff:   fmt.Sprintf("+ created %s\n", newRelPath),
				},
			)
		}
		return nil
	})

	return changes, diffs, err
}

func transformSkillMetaToV3(oldContents, existingContents []byte, oldRelPath, newRelPath string) ([]byte, error) {
	var oldMap map[string]any
	if err := yaml.Unmarshal(oldContents, &oldMap); err != nil {
		return nil, fmt.Errorf("parse %s: %w", oldRelPath, err)
	}

	var existingMap map[string]any
	if len(existingContents) > 0 {
		if err := yaml.Unmarshal(existingContents, &existingMap); err != nil {
			return nil, fmt.Errorf("parse %s: %w", newRelPath, err)
		}
	}

	oldID, _ := oldMap["id"].(string)
	if existingMap != nil {
		if exID, ok := existingMap["id"].(string); ok && exID != "" && oldID != "" && exID != oldID {
			return nil, fmt.Errorf("skill ID mismatch between %s (%q) and %s (%q)", oldRelPath, oldID, newRelPath, exID)
		}
	}

	merged := make(map[string]any)
	merged["schema_version"] = 1

	if oldID != "" {
		merged["id"] = oldID
	} else if existingMap != nil {
		merged["id"] = existingMap["id"]
	}

	status := "draft"
	if s, ok := oldMap["status"].(string); ok && s != "" {
		status = s
	} else if existingMap != nil {
		if s, ok := existingMap["status"].(string); ok && s != "" {
			status = s
		}
	}
	merged["status"] = status

	if routing := mergeRouting(oldMap, existingMap); routing != nil {
		merged["routing"] = routing
	}

	if rt, ok := oldMap["runtime"]; ok && rt != nil {
		merged["runtime"] = rt
	} else if existingMap != nil {
		if rt, ok := existingMap["runtime"]; ok && rt != nil {
			merged["runtime"] = rt
		}
	}

	if quality := mergeQuality(oldMap, existingMap); quality != nil {
		merged["quality"] = quality
	}

	if sources := mergeSources(oldMap, existingMap, oldID); len(sources) > 0 {
		merged["sources"] = sources
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(merged); err != nil {
		return nil, err
	}
	_ = enc.Close()

	return buf.Bytes(), nil
}

func mergeRouting(oldMap, existingMap map[string]any) map[string]any {
	routing := make(map[string]any)
	if existingMap != nil {
		if r, ok := existingMap["routing"].(map[string]any); ok {
			for k, v := range r {
				routing[k] = v
			}
		}
	}
	if r, ok := oldMap["routing"].(map[string]any); ok {
		for k, v := range r {
			routing[k] = v
		}
	}
	for _, k := range []string{"aliases", "domain", "topics", "technologies"} {
		if val, exists := oldMap[k]; exists {
			routing[k] = val
		}
	}
	if len(routing) == 0 {
		return nil
	}
	return routing
}

func mergeQuality(oldMap, existingMap map[string]any) map[string]any {
	quality := make(map[string]any)
	if existingMap != nil {
		if q, ok := existingMap["quality"].(map[string]any); ok {
			for k, v := range q {
				quality[k] = v
			}
		}
	}
	if q, ok := oldMap["quality"].(map[string]any); ok {
		for k, v := range q {
			quality[k] = v
		}
	}
	delete(quality, "curated")
	if len(quality) == 0 {
		return nil
	}
	return quality
}

func mergeSources(oldMap, existingMap map[string]any, defaultID string) []any {
	var sources []any
	if existingMap != nil {
		if srcs, ok := existingMap["sources"].([]any); ok {
			sources = append(sources, srcs...)
		}
	}
	prov, _ := oldMap["provenance"].(map[string]any)
	if prov == nil {
		return sources
	}
	sourceID, _ := prov["source_id"].(string)
	origin, _ := prov["origin"].(map[string]any)
	if origin == nil && sourceID == "" {
		return sources
	}
	upstreamSrc := make(map[string]any)
	srcID := sourceID
	if srcID == "" {
		if n, ok := origin["name"].(string); ok && n != "" {
			srcID = n
		} else {
			srcID = defaultID
		}
	}
	upstreamSrc["id"] = srcID
	upstreamSrc["roles"] = []string{"upstream"}
	if origin != nil {
		for k, v := range origin {
			if k != "added_at" && k != "content_digest" {
				upstreamSrc[k] = v
			}
		}
		if c, ok := origin["commit"].(string); ok && c != "" {
			upstreamSrc["synced"] = c
		}
	}
	foundUpstream := false
	for i, s := range sources {
		if sMap, ok := s.(map[string]any); ok {
			roles, _ := sMap["roles"].([]any)
			for _, r := range roles {
				if r == "upstream" {
					foundUpstream = true
					for k, v := range upstreamSrc {
						if k == "synced" && sMap["synced"] != nil {
							continue
						}
						sMap[k] = v
					}
					sources[i] = sMap
					break
				}
			}
			if foundUpstream {
				break
			}
		}
	}
	if !foundUpstream {
		sources = append([]any{upstreamSrc}, sources...)
	}
	return sources
}
