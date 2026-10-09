// Package migration plans explicit, ordered canonical schema migrations.
package migration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

const CurrentVersion = 3

var (
	ErrNoMigrationPath = errors.New("no canonical migration path is registered")
	ErrAlreadyCurrent  = errors.New("canonical schema is already at the requested version")
)

// FileDiff is a deterministic, content-safe preview of one canonical change.
type FileDiff struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
	Diff   string `json:"diff"`
}

// Proposal pins the exact mutation preview that may subsequently be confirmed.
type Proposal struct {
	SourceVersion int               `json:"source_schema_version"`
	TargetVersion int               `json:"target_schema_version"`
	Changes       []FileDiff        `json:"changes"`
	Mutation      mutation.Proposal `json:"-"`
	ID            string            `json:"proposal_id"`
	Digest        string            `json:"proposal_digest"`
	BaseSnapshot  string            `json:"base_catalog_snapshot"`
}

type step struct {
	from int
	to   int
	plan func(string) ([]mutation.Change, []FileDiff, error)
}

// Registry contains the only supported ordered canonical migrations.
type Registry struct{ steps map[int]step }

// Empty reports whether the registry has no migration steps.
func (registry Registry) Empty() bool { return len(registry.steps) == 0 }

// DefaultRegistry includes every migration understood by this binary.
func DefaultRegistry() Registry {
	return Registry{steps: map[int]step{
		0: {from: 0, to: 1, plan: planLegacyV0ToV1},
		1: {from: 1, to: 2, plan: planV1ToV2},
		2: {from: 2, to: 3, plan: planV2ToV3},
	}}
}

// DetectVersion reads the canonical marker. A missing marker is the legacy v0
// representation; malformed markers are never guessed or rewritten.
func DetectVersion(root string) (int, error) {
	contents, err := workspace.ReadCanonicalFile(root, ".skillhub/schema-version")
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read canonical schema marker: %w", err)
	}
	value := strings.TrimSpace(string(contents))
	version, err := strconv.Atoi(value)
	if err != nil || version < 0 || value != strconv.Itoa(version) {
		return 0, errors.New("canonical schema marker is not a canonical non-negative integer")
	}
	return version, nil
}

// Preview validates the source layout and creates a read-only, pinned proposal.
func (registry Registry) Preview(root string, target int) (Proposal, error) {
	source, err := DetectVersion(root)
	if err != nil {
		return Proposal{}, err
	}
	if target == source {
		return Proposal{}, ErrAlreadyCurrent
	}
	if target <= source || target > CurrentVersion {
		return Proposal{}, fmt.Errorf("%w: %d to %d", ErrNoMigrationPath, source, target)
	}

	changeMap := make(map[string]mutation.Change)
	diffMap := make(map[string]FileDiff)
	orderedPaths := make([]string, 0)
	version := source
	for version < target {
		next, ok := registry.steps[version]
		if !ok || next.from != version || next.to <= version || next.to > target {
			return Proposal{}, fmt.Errorf("%w: %d to %d", ErrNoMigrationPath, source, target)
		}
		stepChanges, stepDiffs, err := next.plan(root)
		if err != nil {
			return Proposal{}, err
		}
		for _, sc := range stepChanges {
			if _, exists := changeMap[sc.Path]; !exists {
				orderedPaths = append(orderedPaths, sc.Path)
			}
			changeMap[sc.Path] = sc
		}
		for _, sd := range stepDiffs {
			if prev, exists := diffMap[sd.Path]; exists {
				beforeVal := prev.Before
				afterVal := sd.After
				diffStr := fmt.Sprintf("--- a/%s\n+++ b/%s\n", sd.Path, sd.Path)
				if beforeVal == "" {
					diffStr += "@@ -0,0 +1 @@\n+" + strings.TrimSuffix(afterVal, "\n") + "\n"
				} else {
					diffStr += fmt.Sprintf("@@ -1 +1 @@\n-%s\n+%s\n", strings.TrimSuffix(beforeVal, "\n"), strings.TrimSuffix(afterVal, "\n"))
				}
				diffMap[sd.Path] = FileDiff{
					Path:   sd.Path,
					Before: beforeVal,
					After:  afterVal,
					Diff:   diffStr,
				}
			} else {
				diffMap[sd.Path] = sd
			}
		}
		version = next.to
	}
	changes := make([]mutation.Change, 0, len(changeMap))
	diffs := make([]FileDiff, 0, len(diffMap))
	for _, p := range orderedPaths {
		changes = append(changes, changeMap[p])
		diffs = append(diffs, diffMap[p])
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].Path < diffs[j].Path })
	sourceVersion, targetVersion := source, target
	planned, err := mutation.PlanMutation(root, mutation.WriteSet{
		Command: "canonical_migration", IdempotencyKey: fmt.Sprintf("canonical-migration:%d:%d", source, target),
		SourceSchemaVersion: &sourceVersion, TargetSchemaVersion: &targetVersion, Changes: changes,
	})
	if err != nil {
		return Proposal{}, err
	}
	return Proposal{
		SourceVersion: source, TargetVersion: target, Changes: diffs, Mutation: planned,
		ID: planned.ID, Digest: planned.Digest, BaseSnapshot: planned.BaseCatalogSnapshot,
	}, nil
}

func planLegacyV0ToV1(root string) ([]mutation.Change, []FileDiff, error) {
	plan, err := workspace.Inspect(root)
	if err != nil {
		return nil, nil, err
	}
	for _, finding := range plan.Findings {
		if finding.ID != "canonical_schema_incompatible" {
			return nil, nil, fmt.Errorf("legacy v0 layout is not otherwise v1-compatible: %s: %s", finding.Path, finding.Summary)
		}
	}
	if len(plan.Findings) != 1 {
		return nil, nil, errors.New("legacy v0 migration requires exactly one missing schema marker finding")
	}
	issues, err := canonical.Validate(root)
	if err != nil {
		return nil, nil, err
	}
	for _, issue := range issues {
		if issue.Path != ".skillhub/schema-version" {
			return nil, nil, fmt.Errorf("legacy v0 layout is not otherwise v1-compatible: %s: %s", issue.Path, issue.Message)
		}
	}
	if len(issues) != 1 {
		return nil, nil, errors.New("legacy v0 migration requires an otherwise valid v1 canonical layout")
	}
	markerPath := filepath.Join(root, ".skillhub", "schema-version")
	beforeBytes, err := os.ReadFile(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		beforeBytes = nil
	} else if err != nil {
		return nil, nil, fmt.Errorf("read legacy schema marker: %w", err)
	}
	before := string(beforeBytes)
	after := "1\n"
	diff := "--- a/.skillhub/schema-version\n+++ b/.skillhub/schema-version\n"
	if before == "" {
		diff += "@@ -0,0 +1 @@\n+1\n"
	} else {
		diff += "@@ -1 +1 @@\n-" + strings.TrimSuffix(before, "\n") + "\n+1\n"
	}
	return []mutation.Change{{Path: ".skillhub/schema-version", Contents: []byte(after)}}, []FileDiff{{
		Path: ".skillhub/schema-version", Before: before, After: after, Diff: diff,
	}}, nil
}

func planV1ToV2(root string) ([]mutation.Change, []FileDiff, error) {
	plan, err := workspace.Inspect(root)
	if err != nil {
		return nil, nil, err
	}
	for _, finding := range plan.Findings {
		if finding.ID != "canonical_schema_incompatible" {
			return nil, nil, fmt.Errorf("v1 layout is not otherwise v2-compatible: %s: %s", finding.Path, finding.Summary)
		}
	}
	if len(plan.Findings) != 1 {
		return nil, nil, errors.New("v1 migration requires exactly one schema marker finding")
	}
	issues, err := canonical.Validate(root)
	if err != nil {
		return nil, nil, err
	}
	for _, issue := range issues {
		if issue.Path != ".skillhub/schema-version" {
			return nil, nil, fmt.Errorf("v1 layout is not otherwise v2-compatible: %s: %s", issue.Path, issue.Message)
		}
	}
	if len(issues) != 1 {
		return nil, nil, errors.New("v1 migration requires an otherwise valid v2 canonical layout")
	}
	markerPath := filepath.Join(root, ".skillhub", "schema-version")
	beforeBytes, err := os.ReadFile(markerPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read schema marker: %w", err)
	}
	before := string(beforeBytes)
	if before == "" {
		before = "1\n"
	} else if strings.TrimSpace(before) != "1" {
		return nil, nil, fmt.Errorf("expected schema version 1, got %q", strings.TrimSpace(before))
	}
	after := "2\n"
	diff := "--- a/.skillhub/schema-version\n+++ b/.skillhub/schema-version\n@@ -1 +1 @@\n-1\n+2\n"
	return []mutation.Change{{Path: ".skillhub/schema-version", Contents: []byte(after)}}, []FileDiff{{
		Path: ".skillhub/schema-version", Before: before, After: after, Diff: diff,
	}}, nil
}

func planV2ToV3(root string) ([]mutation.Change, []FileDiff, error) {
	// First, bump the schema version.
	markerPath := filepath.Join(root, ".skillhub", "schema-version")
	beforeBytes, err := os.ReadFile(markerPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read schema marker: %w", err)
	}
	before := "2\n"
	if len(beforeBytes) > 0 && strings.TrimSpace(string(beforeBytes)) == "2" {
		before = string(beforeBytes)
	}
	after := "3\n"
	diff := "--- a/.skillhub/schema-version\n+++ b/.skillhub/schema-version\n@@ -1 +1 @@\n-2\n+3\n"
	changes := []mutation.Change{{Path: ".skillhub/schema-version", Contents: []byte(after)}}
	diffs := []FileDiff{{
		Path: ".skillhub/schema-version", Before: before, After: after, Diff: diff,
	}}

	// Walk over skills directly instead of parsing via yaml.v3
	// We'll let canonical.Validate and workspace validation handle errors later if needed.
	// Actually, wait, doing it with yaml.v3 is correct for migration.
	skillChanges, skillDiffs, err := migrateSkillYAMLToV3(root)
	if err != nil {
		return nil, nil, err
	}
	changes = append(changes, skillChanges...)
	diffs = append(diffs, skillDiffs...)
	return changes, diffs, nil
}
