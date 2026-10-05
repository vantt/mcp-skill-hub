package catalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

func populate(ctx context.Context, tx *sql.Tx, input buildInput, builderVersion string, expectedCounts map[string]int64, options BuildOptions) (map[string]int64, error) {
	total := 2*len(input.Files) + 3*len(input.Entities)
	processed := 0
	checkpoint := func(message string) error {
		processed++
		if err := waitForContext(ctx); err != nil {
			return err
		}
		if processed == 1 || processed%64 == 0 || processed == total {
			options.report("build", 1, 5, fmt.Sprintf("Populating catalog records (%d/%d): %s", processed, total, message))
		}
		return nil
	}
	if err := waitForContext(ctx); err != nil {
		return nil, err
	}
	for _, file := range input.Files {
		if _, err := tx.ExecContext(ctx, `INSERT INTO canonical_files(path,digest,size_bytes,role) VALUES(?,?,?,?)`, file.Path, file.Digest, len(file.Bytes), fileRole(file.Path)); err != nil {
			return nil, fmt.Errorf("project canonical file %s: %w", file.Path, err)
		}
		if err := checkpoint("canonical files"); err != nil {
			return nil, err
		}
	}
	for _, item := range input.Entities {
		if _, err := tx.ExecContext(ctx, `INSERT INTO canonical_entities(id,path,kind,schema_version,digest,content_json) VALUES(?,?,?,?,?,?)`, item.ID, item.Path, item.Kind, item.SchemaVersion, item.Digest, item.JSON); err != nil {
			return nil, fmt.Errorf("project entity %s: %w", item.Path, err)
		}
		if err := checkpoint("canonical entities"); err != nil {
			return nil, err
		}
	}

	skillByDirectory := make(map[string]string)
	for _, item := range input.Entities {
		if item.Kind != "skill" {
			if err := checkpoint("skills and routing"); err != nil {
				return nil, err
			}
			continue
		}
		collection := stringField(item.Document, "collection_id")
		if collection == "" {
			parts := strings.Split(item.Path, "/")
			if len(parts) >= 3 {
				collection = parts[1]
			}
		}
		name := stringField(item.Document, "name")
		status := stringField(item.Document, "status")
		description := stringField(item.Document, "description")
		if _, err := tx.ExecContext(ctx, `INSERT INTO skills(id,path,collection_id,name,status,description,digest) VALUES(?,?,?,?,?,?,?)`, item.ID, item.Path, collection, name, status, description, item.Digest); err != nil {
			return nil, err
		}
		skillByDirectory[filepath.ToSlash(filepath.Dir(item.Path))] = item.ID
		triggers, examples, err := projectRouting(ctx, tx, item)
		if err != nil {
			return nil, err
		}
		aliases := strings.Join(stringValues(item.Document["aliases"]), " ")
		keywords := strings.Join(append(stringValues(item.Document["topics"]), stringValues(item.Document["technologies"])...), " ")
		if _, err := tx.ExecContext(ctx, `INSERT INTO skill_fts(skill_id,name,aliases,description,triggers,examples,keywords) VALUES(?,?,?,?,?,?,?)`, item.ID, name, aliases, description, strings.Join(triggers, " "), strings.Join(examples, " "), keywords); err != nil {
			return nil, err
		}
		if err := checkpoint("skills and routing"); err != nil {
			return nil, err
		}
	}

	for _, file := range input.Files {
		if strings.HasPrefix(file.Path, "skills/") && !strings.HasSuffix(file.Path, "/skill.meta.yaml") {
			skillID := owningSkill(file.Path, skillByDirectory)
			if _, err := tx.ExecContext(ctx, `INSERT INTO resources(path,skill_id,kind,digest,size_bytes) VALUES(?,?,?,?,?)`, file.Path, nullable(skillID), resourceKind(file.Path), file.Digest, len(file.Bytes)); err != nil {
				return nil, err
			}
			if searchableResource(file.Path) {
				if !utf8.Valid(file.Bytes) {
					return nil, fmt.Errorf("searchable resource %s is not valid UTF-8", file.Path)
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO resource_fts(path,skill_id,content) VALUES(?,?,?)`, file.Path, skillID, string(file.Bytes)); err != nil {
					return nil, err
				}
			}
		}
		if routingDocument(file.Path) && (strings.HasSuffix(file.Path, ".yaml") || strings.HasSuffix(file.Path, ".yml")) {
			var document any
			if err := yaml.Unmarshal(file.Bytes, &document); err != nil {
				return nil, fmt.Errorf("parse routing document %s: %w", file.Path, err)
			}
			encoded, err := jsonMarshalStable(document)
			if err != nil {
				return nil, err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO routing_documents(path,kind,digest,content_json) VALUES(?,?,?,?)`, file.Path, fileRole(file.Path), file.Digest, string(encoded)); err != nil {
				return nil, err
			}
		}
		if err := checkpoint("resources and routing documents"); err != nil {
			return nil, err
		}
	}

	for _, item := range input.Entities {
		if err := projectTypedEntity(ctx, tx, item); err != nil {
			return nil, fmt.Errorf("project %s: %w", item.Path, err)
		}
		if item.Kind != "skill" && item.Kind != "operation" {
			if _, err := tx.ExecContext(ctx, `INSERT INTO curation_fts(entity_id,kind,content) VALUES(?,?,?)`, item.ID, item.Kind, item.SearchText); err != nil {
				return nil, err
			}
		}
		if err := checkpoint("typed entities and search records"); err != nil {
			return nil, err
		}
	}

	// Metadata is derived from immutable input, never trusted from the database
	// being verified. Verification independently queries and compares every table.
	counts := expectedCounts
	encodedCounts, err := json.Marshal(counts)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO generation_metadata(singleton,canonical_schema_version,derived_schema_version,builder_version,catalog_snapshot,projection_input_digest,row_counts_json) VALUES(1,?,?,?,?,?,?)`, input.CanonicalSchemaVersion, DerivedSchemaVersion, builderVersion, input.CatalogSnapshot, input.ProjectionInputDigest, string(encodedCounts)); err != nil {
		return nil, err
	}
	for _, table := range countedTables {
		if _, err := tx.ExecContext(ctx, `INSERT INTO generation_row_counts(table_name,row_count) VALUES(?,?)`, table, counts[table]); err != nil {
			return nil, err
		}
	}
	return counts, nil
}

func projectTypedEntity(ctx context.Context, tx *sql.Tx, item entity) error {
	switch item.Kind {
	case "source":
		locator, _ := json.Marshal(item.Document["locator"])
		if _, err := tx.ExecContext(ctx, `INSERT INTO sources(id,path,adapter,locator_json,digest) VALUES(?,?,?,?,?)`, item.ID, item.Path, stringField(item.Document, "adapter"), string(locator), item.Digest); err != nil {
			return err
		}
		return projectRevisions(ctx, tx, item)
	case "observation", "finding":
		_, err := tx.ExecContext(ctx, `INSERT INTO findings(id,path,source_id,status,summary,content_json) VALUES(?,?,?,?,?,?)`, item.ID, item.Path, stringField(item.Document, "source_id"), stringField(item.Document, "status"), firstString(item.Document, "what", "summary", "finding"), item.JSON)
		return err
	case "comparison":
		_, err := tx.ExecContext(ctx, `INSERT INTO comparisons(id,path,subject,verdict,content_json) VALUES(?,?,?,?,?)`, item.ID, item.Path, stringField(item.Document, "subject"), stringField(item.Document, "verdict"), item.JSON)
		return err
	case "insight":
		_, err := tx.ExecContext(ctx, `INSERT INTO insights(id,path,skill_id,status,recommendation,content_json) VALUES(?,?,?,?,?,?)`, item.ID, item.Path, stringField(item.Document, "skill_id"), stringField(item.Document, "status"), stringField(item.Document, "recommendation"), item.JSON)
		return err
	case "outcome":
		evidence, _ := json.Marshal(item.Document["evidence"])
		_, err := tx.ExecContext(ctx, `INSERT INTO outcomes(id,path,incorporation_id,state,evidence_json,content_json) VALUES(?,?,?,?,?,?)`, item.ID, item.Path, stringField(item.Document, "incorporation_id"), firstString(item.Document, "state", "status"), string(evidence), item.JSON)
		return err
	case "operation":
		return projectOperation(ctx, tx, item)
	case "run", "proposal", "incorporation", "source_candidate", "skill_source_link", "routing_evaluation":
		_, err := tx.ExecContext(ctx, `INSERT INTO provenance(id,path,kind,source_id,insight_id,proposal_id,operation_id,state,content_json) VALUES(?,?,?,?,?,?,?,?,?)`, item.ID, item.Path, item.Kind, nullable(stringField(item.Document, "source_id")), nullable(stringField(item.Document, "insight_id")), nullable(stringField(item.Document, "proposal_id")), nullable(stringField(item.Document, "operation_id")), firstString(item.Document, "state", "status"), item.JSON)
		return err
	}
	return nil
}

func projectOperation(ctx context.Context, tx *sql.Tx, item entity) error {
	if item.SchemaVersion != "1" {
		return fmt.Errorf("operation schema_version must be 1")
	}
	kind, err := requiredString(item.Document, "kind")
	if err != nil {
		return err
	}
	key, err := requiredString(item.Document, "idempotency_key")
	if err != nil {
		return err
	}
	requestDigest, err := requiredString(item.Document, "request_digest")
	if err != nil || !validDigest(requestDigest) {
		return fmt.Errorf("request_digest must be a lowercase SHA-256 digest")
	}
	resultSnapshot, err := requiredString(item.Document, "result_catalog_snapshot")
	if err != nil || !validDigest(resultSnapshot) {
		return fmt.Errorf("result_catalog_snapshot must be a lowercase SHA-256 digest")
	}
	baseSnapshot := stringField(item.Document, "base_catalog_snapshot")
	if baseSnapshot != "" && !validDigest(baseSnapshot) {
		return fmt.Errorf("base_catalog_snapshot must be a lowercase SHA-256 digest")
	}
	occurredAt, err := requiredString(item.Document, "occurred_at")
	if err != nil {
		return err
	}
	if _, ok := item.Document["changes"].([]any); !ok {
		return fmt.Errorf("changes must be a sequence")
	}
	status, err := requiredString(item.Document, "status")
	if err != nil || status != "applied" {
		return fmt.Errorf("operation status must be applied")
	}
	changes, err := json.Marshal(item.Document["changes"])
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO operations(id,path,kind,occurred_at,idempotency_key,request_digest,base_catalog_snapshot,result_catalog_snapshot,status,changes_json) VALUES(?,?,?,?,?,?,?,?,?,?)`, item.ID, item.Path, kind, occurredAt, key, requestDigest, baseSnapshot, resultSnapshot, status, string(changes))
	return err
}

func validDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func projectRevisions(ctx context.Context, tx *sql.Tx, item entity) error {
	var revisions []map[string]any
	for _, key := range []string{"revision", "current_revision", "distilled_revision"} {
		if revision, ok := item.Document[key].(map[string]any); ok {
			revisions = append(revisions, revision)
		}
	}
	if list, ok := item.Document["revisions"].([]any); ok {
		for _, value := range list {
			if revision, ok := value.(map[string]any); ok {
				revisions = append(revisions, revision)
			}
		}
	}
	for index, revision := range revisions {
		if _, err := tx.ExecContext(ctx, `INSERT INTO source_revisions(source_id,ordinal,kind,value,content_digest,observed_at) VALUES(?,?,?,?,?,?)`, item.ID, index, stringField(revision, "kind"), stringField(revision, "value"), stringField(revision, "content_digest"), stringField(revision, "observed_at")); err != nil {
			return err
		}
	}
	return nil
}

func projectRouting(ctx context.Context, tx *sql.Tx, item entity) ([]string, []string, error) {
	routing, _ := item.Document["routing"].(map[string]any)
	categories := map[string]any{
		"operation": routing["operations"], "trigger": routing["triggers"], "exclusion": routing["not_for"],
		"requirement": routing["requirements"], "relationship": []any{routing["distinguish_from"], routing["supporting"], routing["equivalent_to"]},
		"example": routing["examples"], "counter_example": routing["counter_examples"],
	}
	keys := []string{"operation", "trigger", "exclusion", "requirement", "relationship", "example", "counter_example"}
	var triggers []string
	var examples []string
	for _, category := range keys {
		values := stringValues(categories[category])
		for index, value := range values {
			if _, err := tx.ExecContext(ctx, `INSERT INTO routing_metadata(skill_id,category,ordinal,value) VALUES(?,?,?,?)`, item.ID, category, index, value); err != nil {
				return nil, nil, err
			}
		}
		if category == "trigger" {
			triggers = values
		}
		if category == "example" {
			examples = values
		}
	}
	return triggers, examples, nil
}

func stringValues(value any) []string {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		var result []string
		for _, item := range typed {
			result = append(result, stringValues(item)...)
		}
		return result
	default:
		encoded, err := jsonMarshalStable(typed)
		if err != nil {
			return nil
		}
		return []string{string(encoded)}
	}
}

func expectedRowCounts(input buildInput) map[string]int64 {
	counts := make(map[string]int64, len(countedTables))
	for _, table := range countedTables {
		counts[table] = 0
	}
	counts["canonical_files"] = int64(len(input.Files))
	counts["canonical_entities"] = int64(len(input.Entities))
	for _, file := range input.Files {
		if strings.HasPrefix(file.Path, "skills/") && !strings.HasSuffix(file.Path, "/skill.meta.yaml") {
			counts["resources"]++
			if searchableResource(file.Path) {
				counts["resource_fts"]++
			}
		}
		if routingDocument(file.Path) && (strings.HasSuffix(file.Path, ".yaml") || strings.HasSuffix(file.Path, ".yml")) {
			counts["routing_documents"]++
		}
	}
	for _, item := range input.Entities {
		if item.Kind != "skill" && item.Kind != "operation" {
			counts["curation_fts"]++
		}
		switch item.Kind {
		case "skill":
			counts["skills"]++
			counts["skill_fts"]++
			routing, _ := item.Document["routing"].(map[string]any)
			for _, value := range []any{routing["operations"], routing["triggers"], routing["not_for"], routing["requirements"], []any{routing["distinguish_from"], routing["supporting"], routing["equivalent_to"]}, routing["examples"], routing["counter_examples"]} {
				counts["routing_metadata"] += int64(len(stringValues(value)))
			}
		case "source":
			counts["sources"]++
			for _, key := range []string{"revision", "current_revision", "distilled_revision"} {
				if _, ok := item.Document[key].(map[string]any); ok {
					counts["source_revisions"]++
				}
			}
			if revisions, ok := item.Document["revisions"].([]any); ok {
				for _, revision := range revisions {
					if _, ok := revision.(map[string]any); ok {
						counts["source_revisions"]++
					}
				}
			}
		case "observation", "finding":
			counts["findings"]++
		case "comparison":
			counts["comparisons"]++
		case "insight":
			counts["insights"]++
		case "outcome":
			counts["outcomes"]++
		case "operation":
			counts["operations"]++
		case "run", "proposal", "incorporation", "source_candidate", "skill_source_link", "routing_evaluation":
			counts["provenance"]++
		}
	}
	return counts
}

func queryRowCounts(query interface{ QueryRow(string, ...any) *sql.Row }) (map[string]int64, error) {
	counts := make(map[string]int64, len(countedTables))
	for _, table := range countedTables {
		var count int64
		if err := query.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil {
			return nil, err
		}
		counts[table] = count
	}
	return counts, nil
}

func fileRole(path string) string {
	switch {
	case strings.HasPrefix(path, "skills/"):
		return "skill_resource"
	case strings.HasPrefix(path, "sources/"):
		return "source"
	case strings.HasPrefix(path, "distill/"):
		return "learning"
	case strings.HasPrefix(path, "history/operations/"):
		return "operation"
	case strings.HasPrefix(path, "registry/"), strings.HasPrefix(path, "config/"), strings.HasPrefix(path, "evals/routing/"):
		return "routing"
	default:
		return "workspace"
	}
}

func routingDocument(path string) bool {
	return strings.HasPrefix(path, "registry/") || strings.HasPrefix(path, "config/") || strings.HasPrefix(path, "evals/routing/")
}

func owningSkill(path string, directories map[string]string) string {
	owner, longest := "", 0
	for directory, id := range directories {
		if len(directory) > longest && strings.HasPrefix(path, directory+"/") {
			owner, longest = id, len(directory)
		}
	}
	return owner
}

func resourceKind(path string) string {
	if filepath.Base(path) == "SKILL.md" {
		return "instructions"
	}
	parts := strings.Split(path, "/")
	for _, kind := range []string{"references", "scripts", "assets"} {
		for _, part := range parts {
			if part == kind {
				return strings.TrimSuffix(kind, "s")
			}
		}
	}
	return "resource"
}

func searchableResource(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".txt", ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

func stringField(document map[string]any, key string) string {
	switch value := document[key].(type) {
	case string:
		return value
	case time.Time:
		return value.UTC().Format(time.RFC3339Nano)
	default:
		return ""
	}
}

func firstString(document map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringField(document, key); value != "" {
			return value
		}
	}
	return ""
}

func requiredString(document map[string]any, key string) (string, error) {
	value := stringField(document, key)
	if value == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}
	return value, nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
