package migration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
)

func bumpSchemaVersionMarkerV5(root string) ([]mutation.Change, []FileDiff, error) {
	markerPath := filepath.Join(root, ".skillhub", "schema-version")
	beforeBytes, err := os.ReadFile(markerPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read schema-version marker: %w", err)
	}

	markerRel := filepath.ToSlash(filepath.Join(".skillhub", "schema-version"))
	newMarker := []byte("5\n")

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

func planV4ToV5(root string) ([]mutation.Change, []FileDiff, error) {
	markerChanges, markerDiffs, err := bumpSchemaVersionMarkerV5(root)
	if err != nil {
		return nil, nil, err
	}
	changes := append([]mutation.Change(nil), markerChanges...)
	diffs := append([]FileDiff(nil), markerDiffs...)

	opDir := filepath.Join(root, "history", "operations")
	if _, statErr := os.Stat(opDir); errors.Is(statErr, os.ErrNotExist) {
		return changes, diffs, nil
	}

	err = filepath.WalkDir(opDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read receipt %s: %w", path, readErr)
		}
		newYAML, stripped, stripErr := stripReceiptBodiesNode(data)
		if stripErr != nil {
			return fmt.Errorf("strip receipt %s: %w", path, stripErr)
		}
		if !stripped {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		sum := sha256.Sum256(data)
		changes = append(changes, mutation.Change{
			Path:         rel,
			Contents:     newYAML,
			BeforeDigest: "sha256:" + hex.EncodeToString(sum[:]),
		})
		diffs = append(diffs, FileDiff{
			Path:   rel,
			Before: string(data),
			After:  string(newYAML),
			Diff:   fmt.Sprintf("--- a/%s\n+++ b/%s\n@@ stripped bodies @@\n", rel, rel),
		})
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return changes, diffs, nil
}

func stripReceiptBodiesNode(data []byte) ([]byte, bool, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, false, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, false, nil
	}

	mapping := doc.Content[0]
	hasBodies := false

	for i := 0; i < len(mapping.Content); i += 2 {
		keyNode := mapping.Content[i]
		if keyNode.Value != "changes" {
			continue
		}
		changesVal := mapping.Content[i+1]
		if changesVal.Kind != yaml.SequenceNode {
			continue
		}

		for _, item := range changesVal.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			var newContent []*yaml.Node
			for j := 0; j < len(item.Content); j += 2 {
				k := item.Content[j].Value
				v := item.Content[j+1]
				if k == "before_content" || k == "after_content" || k == "content_available" {
					hasBodies = true
					continue
				}
				newContent = append(newContent, item.Content[j], v)
			}
			item.Content = newContent
		}
	}

	if !hasBodies {
		return nil, false, nil
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(4)
	if err := enc.Encode(&doc); err != nil {
		return nil, false, err
	}
	_ = enc.Close()

	return buf.Bytes(), true, nil
}
