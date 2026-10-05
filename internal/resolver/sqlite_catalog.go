package resolver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

type SQLiteCatalog struct {
	database *sql.DB
	snapshot string
}

const reservedSystemSkillID = "system-curator"

var supportRoles = setOf("validation", "research", "implementation", "review", "documentation", "operations")
var supportActivations = setOf("on-demand")
var equivalencePreferences = setOf("self", "target")
var equivalenceVersionPolicies = setOf("exact", "compatible", "latest-reviewed")

// LoadPolicy reads the optional Git-derived recommendation policy. Absence uses
// the calibrated built-in baseline; malformed policy is an infrastructure error.
func LoadPolicy(ctx context.Context, database *sql.DB) (Policy, error) {
	var digest, content string
	err := database.QueryRowContext(ctx, `SELECT digest,content_json FROM routing_documents WHERE path='config/recommendation.yaml'`).Scan(&digest, &content)
	if err == sql.ErrNoRows {
		return DefaultPolicy(), nil
	}
	if err != nil {
		return Policy{}, err
	}
	return ParsePolicy(digest, []byte(content))
}

// ParsePolicy parses and validates recommendation policy JSON content.
func ParsePolicy(digest string, contentJSON []byte) (Policy, error) {
	policy := DefaultPolicy()
	type thresholds struct {
		ApplicabilityFloor float64 `json:"applicability_floor"`
		HighConfidence     float64 `json:"high_confidence"`
		MinimumMargin      float64 `json:"minimum_margin"`
		AmbiguityWindow    float64 `json:"ambiguity_window"`
		SupportingFloor    float64 `json:"supporting_floor"`
	}
	type extensions struct {
		Vector string `json:"vector"`
		LLM    string `json:"llm"`
	}
	var document struct {
		SchemaVersion        int         `json:"schema_version"`
		PolicyVersion        string      `json:"policy_version"`
		NormalizationVersion string      `json:"normalization_version"`
		Weights              *Weights    `json:"weights"`
		Thresholds           *thresholds `json:"thresholds"`
		Extensions           *extensions `json:"extensions"`
	}
	decoder := json.NewDecoder(bytes.NewReader(contentJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return Policy{}, fmt.Errorf("decode recommendation policy: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Policy{}, fmt.Errorf("decode recommendation policy: trailing data")
	}
	if document.SchemaVersion != 1 || document.PolicyVersion == "" || document.NormalizationVersion != "fts-rules-v1" || document.Weights == nil || document.Thresholds == nil || document.Extensions == nil || document.Extensions.Vector != "disabled" || document.Extensions.LLM != "disabled" {
		return Policy{}, fmt.Errorf("recommendation policy version, normalization, fields, or extensions are invalid")
	}
	policy.Revision = digest
	policy.Weights = *document.Weights
	policy.ApplicabilityFloor = document.Thresholds.ApplicabilityFloor
	policy.HighConfidence = document.Thresholds.HighConfidence
	policy.MinimumMargin = document.Thresholds.MinimumMargin
	policy.AmbiguityWindow = document.Thresholds.AmbiguityWindow
	policy.SupportingFloor = document.Thresholds.SupportingFloor
	if err := policy.Validate(); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

func NewSQLiteCatalog(database *sql.DB, snapshot string) (*SQLiteCatalog, error) {
	if database == nil || snapshot == "" {
		return nil, fmt.Errorf("database and catalog snapshot are required")
	}
	return &SQLiteCatalog{database: database, snapshot: snapshot}, nil
}
func (catalog *SQLiteCatalog) Snapshot() string { return catalog.snapshot }

func (catalog *SQLiteCatalog) Search(ctx context.Context, query string, limit int) ([]SearchHit, error) {
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return []SearchHit{}, nil
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		parts = append(parts, `"`+strings.ReplaceAll(token, `"`, `""`)+`"`)
	}
	rows, err := catalog.database.QueryContext(ctx, `SELECT skill_fts.skill_id,bm25(skill_fts,0.0,8.0,5.0,7.0,1.0,3.0,2.0) AS rank FROM skill_fts JOIN skills ON skills.id=skill_fts.skill_id WHERE skills.status='active' AND skills.id<>? AND skill_fts MATCH ? ORDER BY rank ASC, skill_fts.skill_id ASC LIMIT ?`, reservedSystemSkillID, strings.Join(parts, " OR "), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var hit SearchHit
		if err := rows.Scan(&hit.SkillID, &hit.Rank); err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

func (catalog *SQLiteCatalog) Skills(ctx context.Context) ([]Skill, error) {
	rows, err := catalog.database.QueryContext(ctx, `SELECT skills.id,skills.collection_id,skills.name,skills.status,skills.description,skills.digest,canonical_entities.content_json FROM skills JOIN canonical_entities ON canonical_entities.id=skills.id WHERE skills.status='active' AND skills.id<>? ORDER BY skills.id`, reservedSystemSkillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var skills []Skill
	for rows.Next() {
		var skill Skill
		var content string
		if err := rows.Scan(&skill.ID, &skill.CollectionID, &skill.Name, &skill.Status, &skill.Description, &skill.Digest, &content); err != nil {
			return nil, err
		}
		if err := decodeRouting(content, &skill); err != nil {
			return nil, fmt.Errorf("decode routing metadata for %s: %w", skill.ID, err)
		}
		if skill.CollectionID == "" {
			skill.CollectionID = "default"
		}
		skills = append(skills, skill)
	}
	return skills, rows.Err()
}

type skillDocument struct {
	Aliases      []string `json:"aliases"`
	Topics       []string `json:"topics"`
	Technologies []string `json:"technologies"`
	Quality      struct {
		Reviewed bool `json:"reviewed"`
	} `json:"quality"`
	Routing struct {
		Operations      []string `json:"operations"`
		Triggers        []string `json:"triggers"`
		NotFor          []string `json:"not_for"`
		Examples        []string `json:"examples"`
		CounterExamples []string `json:"counter_examples"`
		MinScope        string   `json:"min_scope"`
		Requirements    struct {
			Facts struct {
				All []Requirement `json:"all"`
				Any []Requirement `json:"any"`
			} `json:"facts"`
			Capabilities struct {
				All []string `json:"all"`
				Any []string `json:"any"`
			} `json:"capabilities"`
		} `json:"requirements"`
		DistinguishFrom []struct {
			Skill         string `json:"skill"`
			Discriminator struct {
				Field    string   `json:"field"`
				Question string   `json:"question"`
				Choices  []string `json:"choices"`
			} `json:"discriminator"`
		} `json:"distinguish_from"`
		Supporting []struct {
			Skill string `json:"skill"`
			When  struct {
				Operation string `json:"operation"`
			} `json:"when"`
			Role       string `json:"role"`
			Activation string `json:"activation"`
		} `json:"supporting"`
		EquivalentTo []struct {
			Skill         string `json:"skill"`
			Preference    string `json:"preference"`
			VersionPolicy string `json:"version_policy"`
		} `json:"equivalent_to"`
	} `json:"routing"`
}

func normalizeSkillRouting(skill *Skill) {
	sort.Strings(skill.Aliases)
	sort.Strings(skill.Operations)
	sort.Strings(skill.Triggers)
	sort.Strings(skill.NotFor)
	sort.Strings(skill.Examples)
	sort.Strings(skill.CounterExamples)
	sort.Strings(skill.Topics)
	sort.Strings(skill.Technologies)
	sort.Strings(skill.Requirements.CapabilitiesAll)
	sort.Strings(skill.Requirements.CapabilitiesAny)
	sort.Slice(skill.Requirements.FactsAll, func(i, j int) bool {
		return skill.Requirements.FactsAll[i].Key+"\x00"+skill.Requirements.FactsAll[i].Value < skill.Requirements.FactsAll[j].Key+"\x00"+skill.Requirements.FactsAll[j].Value
	})
	sort.Slice(skill.Requirements.FactsAny, func(i, j int) bool {
		return skill.Requirements.FactsAny[i].Key+"\x00"+skill.Requirements.FactsAny[i].Value < skill.Requirements.FactsAny[j].Key+"\x00"+skill.Requirements.FactsAny[j].Value
	})
	sort.Slice(skill.DistinguishFrom, func(i, j int) bool {
		return skill.DistinguishFrom[i].SkillID+"\x00"+skill.DistinguishFrom[i].Field < skill.DistinguishFrom[j].SkillID+"\x00"+skill.DistinguishFrom[j].Field
	})
	sort.Slice(skill.Supporting, func(i, j int) bool {
		return skill.Supporting[i].SkillID+"\x00"+skill.Supporting[i].Operation < skill.Supporting[j].SkillID+"\x00"+skill.Supporting[j].Operation
	})
	sort.Strings(skill.EquivalentTo)
	sort.Slice(skill.Equivalence, func(i, j int) bool { return skill.Equivalence[i].SkillID < skill.Equivalence[j].SkillID })
}

func decodeRouting(content string, skill *Skill) error {
	var document skillDocument
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		return err
	}
	skill.Aliases = document.Aliases
	skill.Operations = document.Routing.Operations
	skill.Triggers = document.Routing.Triggers
	skill.NotFor = document.Routing.NotFor
	skill.Examples = document.Routing.Examples
	skill.CounterExamples = document.Routing.CounterExamples
	skill.Topics = document.Topics
	skill.Technologies = document.Technologies
	skill.MinScope = document.Routing.MinScope
	skill.Reviewed = document.Quality.Reviewed
	skill.Requirements = Requirements{FactsAll: document.Routing.Requirements.Facts.All, FactsAny: document.Routing.Requirements.Facts.Any, CapabilitiesAll: document.Routing.Requirements.Capabilities.All, CapabilitiesAny: document.Routing.Requirements.Capabilities.Any}
	for _, relation := range document.Routing.DistinguishFrom {
		skill.DistinguishFrom = append(skill.DistinguishFrom, Discriminator{SkillID: relation.Skill, Field: relation.Discriminator.Field, Question: relation.Discriminator.Question, Choices: relation.Discriminator.Choices})
	}
	for _, relation := range document.Routing.Supporting {
		if relation.Skill == "" || !operations[relation.When.Operation] || relation.When.Operation == "" || !supportRoles[relation.Role] || !supportActivations[relation.Activation] {
			return fmt.Errorf("supporting relationship has invalid skill, when.operation, role, or activation")
		}
		skill.Supporting = append(skill.Supporting, SupportRelation{SkillID: relation.Skill, Operation: relation.When.Operation, Role: relation.Role, Activation: relation.Activation})
	}
	for _, relation := range document.Routing.EquivalentTo {
		if relation.Skill == "" || !equivalencePreferences[relation.Preference] || !equivalenceVersionPolicies[relation.VersionPolicy] {
			return fmt.Errorf("equivalent relationship requires curated preference and version_policy")
		}
		skill.EquivalentTo = append(skill.EquivalentTo, relation.Skill)
		skill.Equivalence = append(skill.Equivalence, EquivalenceRelation{SkillID: relation.Skill, Preference: relation.Preference, VersionPolicy: relation.VersionPolicy})
	}
	normalizeSkillRouting(skill)
	return nil
}
