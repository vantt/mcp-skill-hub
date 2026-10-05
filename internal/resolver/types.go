// Package resolver provides deterministic, evidence-first skill recommendation.
// It recommends procedures only; activation remains the caller's responsibility.
package resolver

import (
	"context"
	"errors"
	"time"
)

const SchemaVersion = "1"

const (
	StatusResolved       Status = "resolved"
	StatusNeedsContext   Status = "needs_context"
	StatusNoSkill        Status = "no_skill"
	StatusAlreadyCovered Status = "already_covered"
)

type Status string

type Request struct {
	SchemaVersion     string             `json:"schema_version"`
	RequestID         string             `json:"request_id"`
	Task              Task               `json:"task"`
	Operation         string             `json:"operation,omitempty"`
	Context           RequestContext     `json:"context,omitempty"`
	ActivationContext *ActivationContext `json:"activation_context,omitempty"`
	Prior             *Prior             `json:"prior,omitempty"`
}

type Task struct {
	Description string     `json:"description"`
	Constraints []string   `json:"constraints,omitempty"`
	Exclusions  Exclusions `json:"exclusions,omitempty"`
	Scope       string     `json:"scope,omitempty"`
}

// Exclusions are authoritative caller constraints. SkillIDs are exact canonical
// IDs; Terms are normalized phrases matched only against skill identity fields.
type Exclusions struct {
	SkillIDs []string `json:"skill_ids,omitempty"`
	Terms    []string `json:"terms,omitempty"`
}

type RequestContext struct {
	ActiveArtifact *Artifact `json:"active_artifact,omitempty"`
	Facts          []Fact    `json:"facts,omitempty"`
	Execution      Execution `json:"execution,omitempty"`
}

type Artifact struct {
	Kind     string `json:"kind,omitempty"`
	PathHint string `json:"path_hint,omitempty"`
	Language string `json:"language,omitempty"`
}

type Fact struct {
	Key        string    `json:"key"`
	Value      string    `json:"value"`
	Basis      string    `json:"basis"`
	Scope      string    `json:"scope,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
}

type Execution struct {
	Signal                  *Signal  `json:"signal,omitempty"`
	Capabilities            []string `json:"capabilities,omitempty"`
	UnavailableCapabilities []string `json:"unavailable_capabilities,omitempty"`
}

type Signal struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}

type ActivationContext struct {
	Mode             string            `json:"mode"`
	ActiveProcedures []ActiveProcedure `json:"active_procedures,omitempty"`
}

type ActiveProcedure struct {
	ID          string             `json:"id"`
	Role        string             `json:"role"`
	Scope       string             `json:"scope"`
	Summary     string             `json:"summary,omitempty"`
	StateBasis  string             `json:"state_basis"`
	Replaceable bool               `json:"replaceable"`
	LockedBy    string             `json:"locked_by,omitempty"`
	Identity    *ProcedureIdentity `json:"identity,omitempty"`
}

type ProcedureIdentity struct {
	Digest string `json:"digest,omitempty"`
	Origin string `json:"origin,omitempty"`
}

type Prior struct {
	ResolutionID    string `json:"resolution_id"`
	ContextRevision int    `json:"context_revision"`
	Kind            string `json:"kind"`
	QuestionID      string `json:"question_id,omitempty"`
	Answer          string `json:"answer,omitempty"`
	Basis           string `json:"basis,omitempty"`
	ReasonCode      string `json:"reason_code,omitempty"`
}

type Response struct {
	SchemaVersion   string          `json:"schema_version"`
	ResolutionID    string          `json:"resolution_id"`
	RequestID       string          `json:"request_id"`
	ContextRevision int             `json:"context_revision"`
	Status          Status          `json:"status"`
	CatalogSnapshot string          `json:"catalog_snapshot"`
	PolicyRevision  string          `json:"policy_revision"`
	ReasonCodes     []string        `json:"reason_codes"`
	ValidFor        ValidFor        `json:"valid_for"`
	Primary         *Recommendation `json:"primary,omitempty"`
	Supporting      []Supporting    `json:"supporting"`
	Question        *Question       `json:"question,omitempty"`
	NoSkill         *NoSkill        `json:"no_skill,omitempty"`
	CoveredBy       string          `json:"covered_by,omitempty"`
	CoverageBasis   string          `json:"coverage_basis,omitempty"`
	Warnings        []string        `json:"warnings,omitempty"`
}

type ValidFor struct {
	Operation        string `json:"operation,omitempty"`
	ScopeFingerprint string `json:"scope_fingerprint"`
}

type Recommendation struct {
	ID            string       `json:"id"`
	Version       string       `json:"version"`
	URI           string       `json:"uri"`
	Applicability string       `json:"applicability"`
	Confidence    string       `json:"confidence"`
	Setup         *SetupStatus `json:"setup,omitempty"`
}

type Supporting struct {
	ID         string       `json:"id"`
	Version    string       `json:"version"`
	URI        string       `json:"uri"`
	Role       string       `json:"role"`
	Activation string       `json:"activation"`
	Setup      *SetupStatus `json:"setup,omitempty"`
}

// SetupStatus is a post-ranking hint about whether a recommended skill can be
// used here. It is attached to skills with a runtime block and to third-party
// skills awaiting content review, and never influences ranking or status. The
// hub verifies only trust and platform; a doctor-derived state is a hint whose
// Basis names the terminal that produced it.
type SetupStatus struct {
	State       string   `json:"state"`
	ReasonCodes []string `json:"reason_codes,omitempty"`
	Basis       string   `json:"basis,omitempty"`
	CheckedAt   string   `json:"checked_at,omitempty"`
}

type Question struct {
	ID         string   `json:"id"`
	Field      string   `json:"field"`
	Text       string   `json:"text"`
	Choices    []string `json:"choices"`
	AnswerFrom string   `json:"answer_from"`
}

type NoSkill struct {
	ReasonCode string `json:"reason_code"`
	RetryWhen  string `json:"retry_when"`
}

type Requirement struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Requirements struct {
	FactsAll        []Requirement `json:"facts_all,omitempty"`
	FactsAny        []Requirement `json:"facts_any,omitempty"`
	CapabilitiesAll []string      `json:"capabilities_all,omitempty"`
	CapabilitiesAny []string      `json:"capabilities_any,omitempty"`
}

type Discriminator struct {
	SkillID  string   `json:"skill_id"`
	Field    string   `json:"field"`
	Question string   `json:"question"`
	Choices  []string `json:"choices"`
}

type SupportRelation struct {
	SkillID    string `json:"skill_id"`
	Operation  string `json:"operation,omitempty"`
	Role       string `json:"role"`
	Activation string `json:"activation"`
}

type EquivalenceRelation struct {
	SkillID       string `json:"skill_id"`
	Preference    string `json:"preference"`
	VersionPolicy string `json:"version_policy"`
}

type Skill struct {
	ID              string                `json:"id"`
	CollectionID    string                `json:"collection_id"`
	Name            string                `json:"name"`
	Description     string                `json:"description"`
	Status          string                `json:"status"`
	Digest          string                `json:"digest"`
	Aliases         []string              `json:"aliases,omitempty"`
	Operations      []string              `json:"operations,omitempty"`
	Triggers        []string              `json:"triggers"`
	NotFor          []string              `json:"not_for"`
	Examples        []string              `json:"examples,omitempty"`
	CounterExamples []string              `json:"counter_examples,omitempty"`
	Topics          []string              `json:"topics,omitempty"`
	Technologies    []string              `json:"technologies,omitempty"`
	MinScope        string                `json:"min_scope"`
	Requirements    Requirements          `json:"requirements,omitempty"`
	DistinguishFrom []Discriminator       `json:"distinguish_from,omitempty"`
	Supporting      []SupportRelation     `json:"supporting,omitempty"`
	EquivalentTo    []string              `json:"equivalent_to,omitempty"`
	Equivalence     []EquivalenceRelation `json:"equivalence,omitempty"`
	Reviewed        bool                  `json:"reviewed"`
}

type SearchHit struct {
	SkillID string
	Rank    float64
}

type Catalog interface {
	Snapshot() string
	Skills(context.Context) ([]Skill, error)
	Search(context.Context, string, int) ([]SearchHit, error)
}

var ErrExtensionDisabled = errors.New("resolver extension is disabled")

type SemanticRetriever interface {
	Search(context.Context, string, string, int) ([]SearchHit, error)
}

type LLMReranker interface {
	Rerank(context.Context, Request, []Skill) (string, error)
}

type DisabledSemanticRetriever struct{}

func (DisabledSemanticRetriever) Search(context.Context, string, string, int) ([]SearchHit, error) {
	return nil, ErrExtensionDisabled
}

type DisabledLLMReranker struct{}

func (DisabledLLMReranker) Rerank(context.Context, Request, []Skill) (string, error) {
	return "", ErrExtensionDisabled
}
