package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

const (
	// MaxDistributedSkillResources bounds the files one skill may expose over distribution.
	MaxDistributedSkillResources = 512
	// MaxDistributedSkillBytes bounds the total bytes one skill may expose over distribution.
	MaxDistributedSkillBytes = 16 << 20
)

// ErrSkillNotServable marks an active skill whose files cannot be distributed.
var ErrSkillNotServable = errors.New("skill cannot be served")

// ParseSkillFrontmatter decodes SKILL.md frontmatter into the JSON data model
// carried by skill distribution.
func ParseSkillFrontmatter(contents []byte) (map[string]any, error) {
	contents = bytes.ReplaceAll(contents, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(contents, []byte("---\n")) {
		return nil, errors.New("SKILL.md must begin with YAML frontmatter")
	}
	end := bytes.Index(contents[4:], []byte("\n---\n"))
	if end < 0 {
		return nil, errors.New("SKILL.md frontmatter is not terminated")
	}
	var value map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(contents[4 : 4+end]))
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	if value == nil {
		return nil, errors.New("SKILL.md frontmatter must be an object")
	}
	// Round-trip through JSON to reject YAML-only map keys and normalize values
	// to the exact JSON data model carried by SEP-2640.
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("frontmatter is not JSON-compatible: %w", err)
	}
	var normalized map[string]any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

// ValidateServableSkill applies the invariants skill distribution enforces so an
// active skill that could never be served is rejected when the catalog is built.
func ValidateServableSkill(id string, entrypoint []byte, resourceCount int, totalBytes int64) error {
	if resourceCount < 1 || resourceCount > MaxDistributedSkillResources {
		return fmt.Errorf("%w: skill %s has %d distributed resources; supported range is 1..%d", ErrSkillNotServable, id, resourceCount, MaxDistributedSkillResources)
	}
	if totalBytes > MaxDistributedSkillBytes {
		return fmt.Errorf("%w: skill %s exceeds the %d-byte distribution limit", ErrSkillNotServable, id, MaxDistributedSkillBytes)
	}
	frontmatter, err := ParseSkillFrontmatter(entrypoint)
	if err != nil {
		return fmt.Errorf("%w: skill %s is not distributable: %v", ErrSkillNotServable, id, err)
	}
	name, _ := frontmatter["name"].(string)
	description, _ := frontmatter["description"].(string)
	if name != id || strings.TrimSpace(description) == "" {
		return fmt.Errorf("%w: skill %s frontmatter must contain name %q and a non-empty description", ErrSkillNotServable, id, id)
	}
	return nil
}

// servableSkillWarnings reports every active skill that distribution cannot
// serve. Such skills stay published (a hand edit must not block unrelated
// work) but are listed so the author can repair them; distribution and
// resolution skip them instead of failing for every other skill.
func servableSkillWarnings(input buildInput) []string {
	var warnings []string
	for _, item := range input.Entities {
		if item.Kind != "skill" || stringField(item.Document, "status") != "active" {
			continue
		}
		directory := item.Path[:strings.LastIndex(item.Path, "/")]
		var entrypoint []byte
		count, total := 0, int64(0)
		for _, file := range input.Files {
			if !strings.HasPrefix(file.Path, directory+"/") || workspace.IsHubMeta(file.Path) {
				continue
			}
			count++
			total += int64(len(file.Bytes))
			if file.Path == directory+"/SKILL.md" {
				entrypoint = file.Bytes
			}
		}
		if entrypoint == nil {
			warnings = append(warnings, fmt.Sprintf("%s: skill %s will not be served: %s/SKILL.md is missing", directory, item.ID, directory))
			continue
		}
		if err := ValidateServableSkill(item.ID, entrypoint, count, total); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v; fix SKILL.md frontmatter and rebuild, or the skill is omitted from skills/list", directory, err))
		}
	}
	return warnings
}

// StateBasis identifies whether facts come from current canonical files or a published generation.
type StateBasis string

const (
	BasisCanonical StateBasis = "canonical"
	BasisServed    StateBasis = "served"
)

// CanonicalSkillFacts describes a skill's current state in the canonical workspace.
type CanonicalSkillFacts struct {
	Known          bool     `json:"known"`
	Collection     string   `json:"collection,omitempty"`
	Path           string   `json:"path,omitempty"`
	Status         string   `json:"status,omitempty"`
	Valid          bool     `json:"valid"`
	Issues         []string `json:"issues,omitempty"`
	EntrypointPath string   `json:"entrypoint_path,omitempty"`
}

// ServedSkillFacts describes a skill's state in the currently served/published generation.
type ServedSkillFacts struct {
	Known           bool   `json:"known"`
	Indexed         bool   `json:"indexed"`
	Generation      string `json:"generation,omitempty"`
	CatalogSnapshot string `json:"catalog_snapshot,omitempty"`
	Name            string `json:"name,omitempty"`
	Description     string `json:"description,omitempty"`
	Status          string `json:"status,omitempty"`
	Servable        bool   `json:"servable"`
	ServableReason  string `json:"servable_reason,omitempty"`
	ResourceCount   int    `json:"resource_count"`
	TotalBytes      int64  `json:"total_bytes"`
}

// SkillStateAssessment provides a basis-aware projection comparing canonical and served state.
type SkillStateAssessment struct {
	SkillID          string              `json:"skill_id"`
	ServingMode      ServingMode         `json:"serving_mode"`
	Warning          string              `json:"warning,omitempty"`
	Canonical        CanonicalSkillFacts `json:"canonical"`
	Served           ServedSkillFacts    `json:"served"`
	ResourcesMatch   bool                `json:"resources_match"`
	ChangedResources []string            `json:"changed_resources,omitempty"`
	MissingResources []string            `json:"missing_resources,omitempty"`
	Diverged         bool                `json:"diverged"`
}

// AssessSkillState assesses a skill's canonical and served facts under a shared lock.
func AssessSkillState(ctx context.Context, root, id string) (SkillStateAssessment, error) {
	lock, err := mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
	if err != nil {
		return SkillStateAssessment{}, err
	}
	defer lock.Unlock()
	return AssessSkillStateWhileLocked(ctx, root, id)
}

// AssessSkillStateWhileLocked assesses a skill while the caller already holds a shared lock.
func AssessSkillStateWhileLocked(ctx context.Context, root, id string) (SkillStateAssessment, error) {
	assessment := SkillStateAssessment{
		SkillID:        id,
		ServingMode:    ServingUnavailable,
		ResourcesMatch: true,
	}

	// 1. Inspect canonical files
	skillsDir := filepath.Join(root, "skills")
	collections, err := os.ReadDir(skillsDir)
	if err == nil {
		for _, coll := range collections {
			if !coll.IsDir() {
				continue
			}
			metaPath := filepath.Join(skillsDir, coll.Name(), id, ".meta", "skill.yaml")
			metaBytes, readErr := os.ReadFile(metaPath)
			if readErr != nil {
				metaPath = filepath.Join(skillsDir, coll.Name(), id, "skill.meta.yaml")
				metaBytes, readErr = os.ReadFile(metaPath)
			}
			if readErr == nil {
				assessment.Canonical.Known = true
				assessment.Canonical.Collection = coll.Name()
				assessment.Canonical.Path = "skills/" + coll.Name() + "/" + id
				var meta struct {
					Status string `yaml:"status"`
				}
				_ = yaml.Unmarshal(metaBytes, &meta)
				assessment.Canonical.Status = meta.Status
				entrypointPath := filepath.Join(skillsDir, coll.Name(), id, "SKILL.md")
				if _, statErr := os.Stat(entrypointPath); statErr == nil {
					assessment.Canonical.EntrypointPath = "skills/" + coll.Name() + "/" + id + "/SKILL.md"
				}
				break
			}
		}
	}

	if assessment.Canonical.Known {
		allIssues, valErr := canonical.Validate(root)
		if valErr == nil {
			skillPrefix := assessment.Canonical.Path + "/"
			var skillIssues []string
			for _, issue := range allIssues {
				if strings.HasPrefix(issue.Path, skillPrefix) {
					skillIssues = append(skillIssues, issue.Path+": "+issue.Message)
				}
			}
			assessment.Canonical.Issues = skillIssues
			assessment.Canonical.Valid = len(skillIssues) == 0
		}
	}

	// 2. Inspect published generation
	published, pubErr := inspectPublishedWhileLocked(ctx, root)
	if pubErr == nil && published.Pointer != nil && published.State != StateCorrupt && published.State != StateIncompatible {
		dbPath := generationPath(root, *published.Pointer)
		db, dbErr := sql.Open("sqlite", sqliteDSN(dbPath, true))
		if dbErr == nil {
			defer db.Close()
			db.SetMaxOpenConns(1)
			var name, desc, status string
			queryErr := db.QueryRowContext(ctx, `SELECT name, description, status FROM skills WHERE id=?`, id).Scan(&name, &desc, &status)
			if queryErr == nil {
				assessment.Served.Known = true
				assessment.Served.Indexed = true
				assessment.Served.Generation = published.Pointer.Generation
				assessment.Served.CatalogSnapshot = published.Pointer.CatalogSnapshot
				assessment.Served.Name = name
				assessment.Served.Description = desc
				assessment.Served.Status = status

				// Query resources
				type resRow struct {
					path      string
					digest    string
					sizeBytes int64
				}
				var rows []resRow
				r, rErr := db.QueryContext(ctx, `SELECT path, digest, size_bytes FROM resources WHERE skill_id=? ORDER BY path`, id)
				if rErr == nil {
					for r.Next() {
						var row resRow
						if scanErr := r.Scan(&row.path, &row.digest, &row.sizeBytes); scanErr == nil {
							rows = append(rows, row)
						}
					}
					_ = r.Close()
				}
				assessment.Served.ResourceCount = len(rows)
				var totalBytes int64
				var entrypointBytes []byte
				for _, res := range rows {
					totalBytes += res.sizeBytes
					// Check live file on disk
					livePath := filepath.Join(root, filepath.FromSlash(res.path))
					contents, readErr := os.ReadFile(livePath)
					if readErr != nil {
						assessment.MissingResources = append(assessment.MissingResources, res.path)
						assessment.ResourcesMatch = false
						continue
					}
					sum := sha256.Sum256(contents)
					actualDigest := "sha256:" + hex.EncodeToString(sum[:])
					if actualDigest != res.digest {
						assessment.ChangedResources = append(assessment.ChangedResources, res.path)
						assessment.ResourcesMatch = false
					}
					if strings.HasSuffix(res.path, "/SKILL.md") {
						entrypointBytes = contents
					}
				}
				assessment.Served.TotalBytes = totalBytes

				// Assess servability
				if entrypointBytes == nil {
					assessment.Served.Servable = false
					assessment.Served.ServableReason = "SKILL.md is missing or unreadable"
				} else if valErr := ValidateServableSkill(id, entrypointBytes, len(rows), totalBytes); valErr != nil {
					assessment.Served.Servable = false
					assessment.Served.ServableReason = valErr.Error()
				} else {
					assessment.Served.Servable = true
				}
			} else if errors.Is(queryErr, sql.ErrNoRows) {
				assessment.Served.Known = true
				assessment.Served.Indexed = false
				assessment.Served.Generation = published.Pointer.Generation
				assessment.Served.CatalogSnapshot = published.Pointer.CatalogSnapshot
			}
		}
	}

	// Overall status & serving mode
	status, inspectErr := InspectWhileLocked(ctx, root)
	if inspectErr == nil {
		assessment.ServingMode = status.ServingMode
		assessment.Warning = status.Warning
	}

	// Compute divergence
	if !assessment.ResourcesMatch {
		assessment.Diverged = true
	} else if assessment.Canonical.Known && assessment.Served.Known && assessment.Canonical.Status != assessment.Served.Status {
		assessment.Diverged = true
	}

	return assessment, nil
}
