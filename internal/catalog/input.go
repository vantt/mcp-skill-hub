package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

type inputFile struct {
	Path   string
	Digest string
	Bytes  []byte
}

type buildInput struct {
	Files                  []inputFile
	CatalogSnapshot        string
	ProjectionInputDigest  string
	CanonicalSchemaVersion int
	Entities               []entity
	Warnings               []string
}

// entity retains the schema-specific YAML object only inside the build boundary;
// projection functions immediately validate and copy supported fields into typed tables.
type entity struct {
	ID            string
	Path          string
	Kind          string
	SchemaVersion string
	Digest        string
	Document      map[string]any
	JSON          string
	SearchText    string
}

func captureInput(ctx context.Context, root string) (buildInput, error) {
	if err := waitForContext(ctx); err != nil {
		return buildInput{}, err
	}
	lock, err := mutation.AcquireSharedLock(ctx, root, mutation.DefaultLockTimeout)
	if err != nil {
		return buildInput{}, err
	}
	defer lock.Unlock()
	return captureInputWhileLocked(ctx, root, false)
}

func captureInputWhileLocked(ctx context.Context, root string, allowRecovery bool) (buildInput, error) {
	if err := waitForContext(ctx); err != nil {
		return buildInput{}, err
	}
	if !allowRecovery {
		if recoveries, err := mutation.InspectRecoveryWhileLocked(root); err != nil {
			return buildInput{}, err
		} else if len(recoveries) != 0 {
			return buildInput{}, mutation.ErrRecoveryRequired
		}
	}
	issues, err := canonical.Validate(root)
	if err != nil {
		return buildInput{}, err
	}
	if len(issues) != 0 {
		return buildInput{}, fmt.Errorf("canonical validation failed: %s: %s", issues[0].Path, issues[0].Message)
	}
	input, err := readInput(root)
	if err != nil {
		return buildInput{}, err
	}
	// A second inventory while the shared lock remains held catches unmanaged
	// editors that changed bytes while the immutable image was being captured.
	recheck, err := canonical.Scan(root)
	if err != nil {
		return buildInput{}, err
	}
	if recheck.CatalogSnapshot != input.CatalogSnapshot || recheck.ProjectionInputDigest != input.ProjectionInputDigest || !sameFiles(input.Files, recheck.Files) {
		return buildInput{}, fmt.Errorf("%w: canonical workspace changed while build input was captured", ErrCanonicalChanged)
	}
	return input, nil
}

func readInput(root string) (buildInput, error) {
	paths, err := workspace.RelativeFiles(root)
	if err != nil {
		return buildInput{}, err
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return buildInput{}, err
	}
	defer rootHandle.Close()
	input := buildInput{Files: make([]inputFile, 0, len(paths))}
	var totalBytes int64
	for _, path := range paths {
		contents, err := readCanonicalInput(rootHandle, path)
		if err != nil {
			return buildInput{}, err
		}
		if int64(len(contents)) > workspace.MaxCanonicalBytesV1-totalBytes {
			return buildInput{}, fmt.Errorf("canonical files exceed V1 aggregate limit of %d bytes at %s", workspace.MaxCanonicalBytesV1, path)
		}
		totalBytes += int64(len(contents))
		sum := sha256.Sum256(contents)
		input.Files = append(input.Files, inputFile{Path: path, Digest: "sha256:" + hex.EncodeToString(sum[:]), Bytes: contents})
	}
	input.ProjectionInputDigest = aggregate(input.Files, func(inputFile) bool { return true })
	input.CatalogSnapshot = aggregate(input.Files, func(file inputFile) bool { return catalogAffecting(file.Path) })
	for _, file := range input.Files {
		if file.Path == ".skillhub/schema-version" {
			version, err := strconv.Atoi(strings.TrimSpace(string(file.Bytes)))
			if err != nil || version <= 0 {
				return buildInput{}, fmt.Errorf("invalid canonical schema version")
			}
			input.CanonicalSchemaVersion = version
		}
		if strings.HasSuffix(file.Path, ".yaml") || strings.HasSuffix(file.Path, ".yml") {
			item, ok, err := parseEntity(file)
			if err != nil {
				return buildInput{}, err
			}
			if strings.HasSuffix(file.Path, "/skill.meta.yaml") && !ok {
				return buildInput{}, fmt.Errorf("skill metadata %s is missing an ID", file.Path)
			}
			if ok {
				input.Entities = append(input.Entities, item)
			}
		}
	}
	if input.CanonicalSchemaVersion == 0 {
		return buildInput{}, fmt.Errorf("canonical schema version is missing")
	}
	sort.Slice(input.Entities, func(i, j int) bool { return input.Entities[i].Path < input.Entities[j].Path })
	if err := validateEntities(input.Entities); err != nil {
		return buildInput{}, err
	}
	input.Warnings = servableSkillWarnings(input)
	return input, nil
}

func readCanonicalInput(root *os.Root, path string) ([]byte, error) {
	info, err := root.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect canonical input %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("canonical input %s is not a regular file", path)
	}
	if info.Size() > workspace.MaxCanonicalFileBytesV1 {
		return nil, fmt.Errorf("canonical file %s exceeds V1 per-file limit of %d bytes", path, workspace.MaxCanonicalFileBytesV1)
	}
	file, err := root.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open canonical input %s: %w", path, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened canonical input %s: %w", path, err)
	}
	if !opened.Mode().IsRegular() || opened.Size() > workspace.MaxCanonicalFileBytesV1 {
		return nil, fmt.Errorf("canonical file %s exceeds V1 per-file limit of %d bytes", path, workspace.MaxCanonicalFileBytesV1)
	}
	contents, err := io.ReadAll(io.LimitReader(file, workspace.MaxCanonicalFileBytesV1+1))
	if err != nil {
		return nil, fmt.Errorf("read canonical input %s: %w", path, err)
	}
	if int64(len(contents)) > workspace.MaxCanonicalFileBytesV1 {
		return nil, fmt.Errorf("canonical file %s exceeds V1 per-file limit of %d bytes", path, workspace.MaxCanonicalFileBytesV1)
	}
	return contents, nil
}

func parseEntity(file inputFile) (entity, bool, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(file.Bytes))
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		return entity{}, false, fmt.Errorf("parse %s: %w", file.Path, err)
	}
	id, _ := document["id"].(string)
	if id == "" {
		return entity{}, false, nil
	}
	encoded, err := jsonMarshalStable(document)
	if err != nil {
		return entity{}, false, fmt.Errorf("normalize %s: %w", file.Path, err)
	}
	return entity{
		ID: id, Path: file.Path, Kind: classifyEntity(file.Path), SchemaVersion: scalarString(document["schema_version"]),
		Digest: file.Digest, Document: document, JSON: string(encoded), SearchText: flattenText(document),
	}, true, nil
}

func jsonMarshalStable(value any) ([]byte, error) {
	// encoding/json sorts string map keys, giving logical rows stable serialization.
	return json.Marshal(value)
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int:
		return strconv.Itoa(typed)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return ""
	}
}

func flattenText(value any) string {
	var values []string
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(typed[key])
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case string:
			values = append(values, typed)
		}
	}
	walk(value)
	return strings.Join(values, " ")
}

func classifyEntity(path string) string {
	switch {
	case strings.HasSuffix(path, "/skill.meta.yaml"):
		return "skill"
	case strings.HasPrefix(path, "sources/catalog/"):
		return "source"
	case strings.HasPrefix(path, "sources/intake/"):
		return "source_candidate"
	case strings.HasPrefix(path, "sources/skills/"):
		return "skill_source_link"
	case strings.Contains(path, "/observations/"):
		return "observation"
	case strings.Contains(path, "/findings/"):
		return "finding"
	case strings.Contains(path, "/runs/"):
		return "run"
	case strings.HasPrefix(path, "distill/comparisons/"):
		return "comparison"
	case strings.Contains(path, "/insights/"):
		return "insight"
	case strings.Contains(path, "/proposals/"):
		return "proposal"
	case strings.Contains(path, "/incorporations/"):
		return "incorporation"
	case strings.Contains(path, "/outcomes/"):
		return "outcome"
	case strings.HasPrefix(path, "history/operations/"):
		return "operation"
	case strings.HasPrefix(path, "registry/collections/"):
		return "collection"
	case strings.HasPrefix(path, "evals/routing/"):
		return "routing_evaluation"
	default:
		return "entity"
	}
}

func catalogAffecting(path string) bool {
	return path == ".skillhub/schema-version" || strings.HasPrefix(path, "skills/") || strings.HasPrefix(path, "registry/") || strings.HasPrefix(path, "config/") || strings.HasPrefix(path, "evals/routing/")
}

func aggregate(files []inputFile, include func(inputFile) bool) string {
	hash := sha256.New()
	for _, file := range files {
		if include(file) {
			fmt.Fprintf(hash, "%s\x00%s\n", file.Path, file.Digest)
		}
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func sameInputFiles(first, second []inputFile) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index].Path != second[index].Path || first[index].Digest != second[index].Digest {
			return false
		}
	}
	return true
}

func validateEntities(entities []entity) error {
	ids := make(map[string]entity, len(entities))
	// Pass one validates identity, schema and the shape consumed by projections.
	for _, item := range entities {
		if previous, ok := ids[item.ID]; ok {
			return fmt.Errorf("duplicate entity ID %q in %s and %s", item.ID, previous.Path, item.Path)
		}
		ids[item.ID] = item
		if item.SchemaVersion != "1" {
			return fmt.Errorf("%s: schema_version must be 1", item.Path)
		}
		expected := strings.TrimSuffix(filepath.Base(item.Path), filepath.Ext(item.Path))
		if item.Kind == "skill" {
			expected = filepath.Base(filepath.Dir(item.Path))
		}
		if item.ID != expected {
			return fmt.Errorf("%s: id %q does not match path identity %q", item.Path, item.ID, expected)
		}
		if err := validateEntityShape(item); err != nil {
			return fmt.Errorf("%s: %w", item.Path, err)
		}
	}
	// Pass two resolves every explicit entity reference after all IDs are known.
	for _, item := range entities {
		for key, value := range item.Document {
			if key != "id" && strings.HasSuffix(key, "_id") {
				id, ok := value.(string)
				if !ok || id == "" {
					return fmt.Errorf("%s: %s must be a non-empty string", item.Path, key)
				}
				target, ok := ids[id]
				if !ok {
					return fmt.Errorf("%s: dangling reference %s=%q", item.Path, key, id)
				}
				expectedKinds := map[string][]string{
					"skill_id": {"skill"}, "source_id": {"source"}, "insight_id": {"insight"},
					"proposal_id": {"proposal"}, "operation_id": {"operation"}, "incorporation_id": {"incorporation"},
				}
				if kinds := expectedKinds[key]; len(kinds) != 0 && !containsString(kinds, target.Kind) {
					return fmt.Errorf("%s: reference %s=%q targets %s, expected %s", item.Path, key, id, target.Kind, strings.Join(kinds, " or "))
				}
			}
		}
	}
	return nil
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func validateEntityShape(item entity) error {
	require := func(fields ...string) error {
		for _, field := range fields {
			if _, err := requiredString(item.Document, field); err != nil {
				return err
			}
		}
		return nil
	}
	switch item.Kind {
	case "skill":
		return require("name", "status", "description")
	case "source_candidate":
		if err := require("locator", "captured_at", "reason", "status"); err != nil {
			return err
		}
		if !containsString([]string{"pending", "accepted", "rejected", "deferred"}, stringField(item.Document, "status")) {
			return fmt.Errorf("status must be pending, accepted, rejected, or deferred")
		}
		if _, err := time.Parse(time.RFC3339Nano, stringField(item.Document, "captured_at")); err != nil {
			return fmt.Errorf("captured_at must be an RFC3339 timestamp")
		}
	case "source":
		if err := require("adapter"); err != nil {
			return err
		}
		if _, ok := item.Document["locator"]; !ok {
			return fmt.Errorf("locator is required")
		}
		for _, key := range []string{"revision", "current_revision", "distilled_revision"} {
			if value, ok := item.Document[key]; ok {
				revision, ok := value.(map[string]any)
				if !ok {
					return fmt.Errorf("%s must be a mapping", key)
				}
				if err := validateRevision(revision); err != nil {
					return fmt.Errorf("%s: %w", key, err)
				}
			}
		}
	case "observation", "finding":
		if err := require("source_id", "status"); err != nil {
			return err
		}
		if firstString(item.Document, "what", "summary", "finding") == "" {
			return fmt.Errorf("one of what, summary, or finding must be a non-empty string")
		}
	case "comparison":
		return require("subject", "verdict")
	case "insight":
		return require("skill_id", "status", "recommendation")
	case "outcome":
		if err := require("incorporation_id"); err != nil {
			return err
		}
		if firstString(item.Document, "state", "status") == "" {
			return fmt.Errorf("state or status must be a non-empty string")
		}
	case "skill_source_link":
		if err := require("skill_id", "source_id"); err != nil {
			return err
		}
		if role := stringField(item.Document, "role"); role != "" && !containsString([]string{"learning-source", "origin", "inspiration"}, role) {
			return fmt.Errorf("role must be learning-source, origin, or inspiration")
		}
		return nil
	case "proposal":
		if err := require("insight_id", "base_catalog_version", "digest", "status"); err != nil {
			return err
		}
		if !validDigest(stringField(item.Document, "base_catalog_version")) || !validDigest(stringField(item.Document, "digest")) {
			return fmt.Errorf("base_catalog_version and digest must be lowercase SHA-256 digests")
		}
		if _, ok := item.Document["changed_files"].([]any); !ok {
			return fmt.Errorf("changed_files must be a sequence")
		}
	case "incorporation":
		return require("insight_id", "proposal_id", "operation_id", "state")
	case "operation":
		if _, err := time.Parse(time.RFC3339Nano, stringField(item.Document, "occurred_at")); err != nil {
			return fmt.Errorf("occurred_at must be an RFC3339 timestamp")
		}
	}
	return nil
}

func validateRevision(revision map[string]any) error {
	for _, field := range []string{"kind", "value", "content_digest", "observed_at"} {
		if _, err := requiredString(revision, field); err != nil {
			return err
		}
	}
	if !validDigest(stringField(revision, "content_digest")) {
		return fmt.Errorf("content_digest must be a lowercase SHA-256 digest")
	}
	if _, err := time.Parse(time.RFC3339Nano, stringField(revision, "observed_at")); err != nil {
		return fmt.Errorf("observed_at must be an RFC3339 timestamp")
	}
	return nil
}

func sameFiles(files []inputFile, digests []canonical.FileDigest) bool {
	if len(files) != len(digests) {
		return false
	}
	for index := range files {
		if files[index].Path != digests[index].Path || files[index].Digest != digests[index].Digest {
			return false
		}
	}
	return true
}
