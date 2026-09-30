package catalog

const catalogSchema = `
PRAGMA foreign_keys = ON;
CREATE TABLE generation_metadata (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  canonical_schema_version INTEGER NOT NULL,
  derived_schema_version INTEGER NOT NULL,
  builder_version TEXT NOT NULL,
  catalog_snapshot TEXT NOT NULL,
  projection_input_digest TEXT NOT NULL,
  row_counts_json TEXT NOT NULL
) STRICT;
CREATE TABLE generation_row_counts (
  table_name TEXT PRIMARY KEY,
  row_count INTEGER NOT NULL CHECK (row_count >= 0)
) STRICT;
CREATE TABLE canonical_files (
  path TEXT PRIMARY KEY,
  digest TEXT NOT NULL,
  size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0),
  role TEXT NOT NULL
) STRICT;
CREATE TABLE canonical_entities (
  id TEXT PRIMARY KEY,
  path TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  schema_version TEXT NOT NULL,
  digest TEXT NOT NULL,
  content_json TEXT NOT NULL
) STRICT;
CREATE TABLE skills (
  id TEXT PRIMARY KEY REFERENCES canonical_entities(id),
  path TEXT NOT NULL UNIQUE,
  collection_id TEXT NOT NULL,
  name TEXT NOT NULL,
  status TEXT NOT NULL,
  description TEXT NOT NULL,
  digest TEXT NOT NULL
) STRICT;
CREATE TABLE resources (
  path TEXT PRIMARY KEY,
  skill_id TEXT REFERENCES skills(id),
  kind TEXT NOT NULL,
  digest TEXT NOT NULL,
  size_bytes INTEGER NOT NULL CHECK (size_bytes >= 0)
) STRICT;
CREATE TABLE routing_metadata (
  skill_id TEXT NOT NULL REFERENCES skills(id),
  category TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  value TEXT NOT NULL,
  PRIMARY KEY (skill_id, category, ordinal)
) STRICT;
CREATE TABLE routing_documents (
  path TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  digest TEXT NOT NULL,
  content_json TEXT NOT NULL
) STRICT;
CREATE TABLE sources (
  id TEXT PRIMARY KEY REFERENCES canonical_entities(id),
  path TEXT NOT NULL UNIQUE,
  adapter TEXT NOT NULL,
  locator_json TEXT NOT NULL,
  digest TEXT NOT NULL
) STRICT;
CREATE TABLE source_revisions (
  source_id TEXT NOT NULL REFERENCES sources(id),
  ordinal INTEGER NOT NULL,
  kind TEXT NOT NULL,
  value TEXT NOT NULL,
  content_digest TEXT NOT NULL,
  observed_at TEXT NOT NULL,
  PRIMARY KEY (source_id, ordinal)
) STRICT;
CREATE TABLE findings (
  id TEXT PRIMARY KEY REFERENCES canonical_entities(id),
  path TEXT NOT NULL UNIQUE,
  source_id TEXT NOT NULL REFERENCES sources(id) DEFERRABLE INITIALLY DEFERRED,
  status TEXT NOT NULL,
  summary TEXT NOT NULL,
  content_json TEXT NOT NULL
) STRICT;
CREATE TABLE comparisons (
  id TEXT PRIMARY KEY REFERENCES canonical_entities(id),
  path TEXT NOT NULL UNIQUE,
  subject TEXT NOT NULL,
  verdict TEXT NOT NULL,
  content_json TEXT NOT NULL
) STRICT;
CREATE TABLE insights (
  id TEXT PRIMARY KEY REFERENCES canonical_entities(id),
  path TEXT NOT NULL UNIQUE,
  skill_id TEXT NOT NULL REFERENCES skills(id) DEFERRABLE INITIALLY DEFERRED,
  status TEXT NOT NULL,
  recommendation TEXT NOT NULL,
  content_json TEXT NOT NULL
) STRICT;
CREATE TABLE provenance (
  id TEXT PRIMARY KEY REFERENCES canonical_entities(id),
  path TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  source_id TEXT REFERENCES sources(id) DEFERRABLE INITIALLY DEFERRED,
  insight_id TEXT REFERENCES insights(id) DEFERRABLE INITIALLY DEFERRED,
  proposal_id TEXT REFERENCES canonical_entities(id) DEFERRABLE INITIALLY DEFERRED,
  operation_id TEXT REFERENCES operations(id) DEFERRABLE INITIALLY DEFERRED,
  state TEXT NOT NULL,
  content_json TEXT NOT NULL
) STRICT;
CREATE TABLE outcomes (
  id TEXT PRIMARY KEY REFERENCES canonical_entities(id),
  path TEXT NOT NULL UNIQUE,
  incorporation_id TEXT NOT NULL REFERENCES canonical_entities(id) DEFERRABLE INITIALLY DEFERRED,
  state TEXT NOT NULL,
  evidence_json TEXT NOT NULL,
  content_json TEXT NOT NULL
) STRICT;
CREATE TABLE operations (
  id TEXT PRIMARY KEY REFERENCES canonical_entities(id),
  path TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  occurred_at TEXT NOT NULL,
  idempotency_key TEXT NOT NULL UNIQUE,
  request_digest TEXT NOT NULL,
  base_catalog_snapshot TEXT NOT NULL,
  result_catalog_snapshot TEXT NOT NULL,
  status TEXT NOT NULL,
  changes_json TEXT NOT NULL
) STRICT;
CREATE VIRTUAL TABLE skill_fts USING fts5(skill_id UNINDEXED, name, aliases, description, triggers);
CREATE VIRTUAL TABLE resource_fts USING fts5(path UNINDEXED, skill_id UNINDEXED, content);
CREATE VIRTUAL TABLE curation_fts USING fts5(entity_id UNINDEXED, kind UNINDEXED, content);
`

var countedTables = []string{
	"canonical_files", "canonical_entities", "skills", "resources", "routing_metadata", "routing_documents",
	"sources", "source_revisions", "findings", "comparisons", "insights", "provenance", "outcomes", "operations",
	"skill_fts", "resource_fts", "curation_fts",
}
