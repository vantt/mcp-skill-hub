package resolver

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSQLiteCatalogUsesFTSAndStructuredRoutingMetadata(t *testing.T) {
	database, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	statements := []string{
		`CREATE TABLE skills(id TEXT PRIMARY KEY,collection_id TEXT,name TEXT,status TEXT,description TEXT,digest TEXT)`,
		`CREATE TABLE canonical_entities(id TEXT PRIMARY KEY,content_json TEXT)`,
		`CREATE VIRTUAL TABLE skill_fts USING fts5(skill_id UNINDEXED,name,aliases,description,triggers)`,
		`CREATE TABLE routing_documents(path TEXT PRIMARY KEY,digest TEXT,content_json TEXT)`,
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	document := map[string]any{"aliases": []string{"consumer safety"}, "quality": map[string]any{"reviewed": true}, "routing": map[string]any{"operations": []string{"review"}, "triggers": []string{"inspect retry acknowledgement"}, "not_for": []string{"broker topology"}, "min_scope": "multi_step", "requirements": map[string]any{"capabilities": map[string]any{"all": []string{"read-files"}}}}}
	encoded, _ := json.Marshal(document)
	if _, err := database.Exec(`INSERT INTO skills VALUES(?,?,?,?,?,?)`, "consumer-review", "reliability", "Consumer Review", "active", "Review duplicate processing", "sha256:test"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO canonical_entities VALUES(?,?)`, "consumer-review", string(encoded)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO skill_fts VALUES(?,?,?,?,?)`, "consumer-review", "Consumer Review", "consumer safety", "Review duplicate processing", "inspect retry acknowledgement"); err != nil {
		t.Fatal(err)
	}
	reservedDocument := map[string]any{"quality": map[string]any{"reviewed": true}, "routing": map[string]any{"operations": []string{"review"}, "triggers": []string{"reserved curator collision sentinel"}, "not_for": []string{"ordinary work"}, "min_scope": "multi_step"}}
	reservedEncoded, _ := json.Marshal(reservedDocument)
	if _, err := database.Exec(`INSERT INTO skills VALUES(?,?,?,?,?,?)`, reservedSystemSkillID, "core", "Workspace Curator Shadow", "active", "Reserved curator collision sentinel", "sha256:reserved"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO canonical_entities VALUES(?,?)`, reservedSystemSkillID, string(reservedEncoded)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO skill_fts VALUES(?,?,?,?,?)`, reservedSystemSkillID, "Workspace Curator Shadow", "", "Reserved curator collision sentinel", "reserved curator collision sentinel"); err != nil {
		t.Fatal(err)
	}
	catalog, err := NewSQLiteCatalog(database, "sha256:"+repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	hits, err := catalog.Search(context.Background(), "acknowledgement ordering", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].SkillID != "consumer-review" {
		t.Fatalf("hits=%#v", hits)
	}
	skills, err := catalog.Skills(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Requirements.CapabilitiesAll[0] != "read-files" || !skills[0].Reviewed {
		t.Fatalf("skills=%#v", skills)
	}
	reservedHits, err := catalog.Search(context.Background(), "reserved curator collision sentinel", 10)
	if err != nil || len(reservedHits) != 0 {
		t.Fatalf("reserved system skill entered search: hits=%#v err=%v", reservedHits, err)
	}
	engine, err := New(catalog, DefaultPolicy(), NewCache(4))
	if err != nil {
		t.Fatal(err)
	}
	response, err := engine.Resolve(context.Background(), Request{SchemaVersion: "1", RequestID: "reserved-collision", Task: Task{Description: "reserved curator collision sentinel", Scope: "multi_step"}, Operation: "review", Context: RequestContext{Execution: Execution{UnavailableCapabilities: []string{"read-files"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != StatusNoSkill || response.Primary != nil {
		t.Fatalf("resolver selected reserved system skill: %#v", response)
	}
	inactiveDocument := map[string]any{"routing": map[string]any{"triggers": []string{"acknowledgement ordering"}, "not_for": []string{}, "min_scope": "multi_step"}}
	inactiveEncoded, _ := json.Marshal(inactiveDocument)
	if _, err := database.Exec(`INSERT INTO skills VALUES(?,?,?,?,?,?)`, "inactive-exact", "reliability", "Inactive Exact", "deprecated", "acknowledgement ordering", "sha256:inactive"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO canonical_entities VALUES(?,?)`, "inactive-exact", string(inactiveEncoded)); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO skill_fts VALUES(?,?,?,?,?)`, "inactive-exact", "Inactive Exact", "", "acknowledgement ordering", "acknowledgement ordering"); err != nil {
		t.Fatal(err)
	}
	hits, err = catalog.Search(context.Background(), "acknowledgement ordering", 10)
	if err != nil || len(hits) != 1 || hits[0].SkillID != "consumer-review" {
		t.Fatalf("inactive skill entered search: hits=%#v err=%v", hits, err)
	}
	skills, err = catalog.Skills(context.Background())
	if err != nil || len(skills) != 1 {
		t.Fatalf("inactive skill entered projection: skills=%#v err=%v", skills, err)
	}
	invalidSupport := `{"routing":{"supporting":[{"skill":"tests","when":{"operation":"review"},"role":"arbitrary prose","activation":"always"}]}}`
	if err := decodeRouting(invalidSupport, &Skill{}); err == nil {
		t.Fatal("unexpected supporting metadata accepted")
	}
	policyDocument := `{"schema_version":1,"policy_version":"calibrated-v1","normalization_version":"fts-rules-v1","weights":{"lexical":0.28,"trigger":0.38,"artifact":0.08,"fact":0.1,"operation":0.08,"quality":0.03,"not_for_penalty":0.42,"constraint_penalty":0.48,"scope_penalty":0.25},"thresholds":{"applicability_floor":0.28,"high_confidence":0.68,"minimum_margin":0.08,"ambiguity_window":0.07,"supporting_floor":0.31},"extensions":{"vector":"disabled","llm":"disabled"}}`
	if _, err := database.Exec(`INSERT INTO routing_documents VALUES(?,?,?)`, "config/recommendation.yaml", "sha256:"+repeat("b", 64), policyDocument); err != nil {
		t.Fatal(err)
	}
	policy, err := LoadPolicy(context.Background(), database)
	if err != nil {
		t.Fatal(err)
	}
	if policy.ApplicabilityFloor != .28 || policy.Revision != "sha256:"+repeat("b", 64) {
		t.Fatalf("policy=%#v", policy)
	}
}
