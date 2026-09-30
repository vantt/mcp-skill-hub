// Package evaluation provides deterministic, versioned resolver evaluation and replay.
package evaluation

import (
	"context"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/resolver"
)

const SchemaVersion = 1

type Partition string

const (
	PartitionDevelopment Partition = "development"
	PartitionCalibration Partition = "calibration"
	PartitionHeldOut     Partition = "held_out"
)

type Case struct {
	SchemaVersion int                `json:"schema_version" yaml:"schema_version"`
	ID            string             `json:"id" yaml:"id"`
	Partition     Partition          `json:"partition,omitempty" yaml:"partition,omitempty"`
	Split         Partition          `json:"split,omitempty" yaml:"split,omitempty"`
	Tags          []string           `json:"tags,omitempty" yaml:"tags,omitempty"`
	Request       resolver.Request   `json:"request" yaml:"request"`
	Expected      Expected           `json:"expected" yaml:"expected"`
	Counters      map[string]float64 `json:"counters,omitempty" yaml:"counters,omitempty"`
	Exclude       string             `json:"exclude,omitempty" yaml:"exclude,omitempty"`
	Provenance    Provenance         `json:"provenance,omitempty" yaml:"provenance,omitempty"`
}

type Provenance struct {
	Source              string   `json:"source,omitempty" yaml:"source,omitempty"`
	ReviewedBy          []string `json:"reviewed_by,omitempty" yaml:"reviewed_by,omitempty"`
	ReviewedAt          string   `json:"reviewed_at,omitempty" yaml:"reviewed_at,omitempty"`
	IntentTemplateGroup string   `json:"intent_template_group,omitempty" yaml:"intent_template_group,omitempty"`
}

type Expected struct {
	Status              resolver.Status   `json:"status,omitempty" yaml:"status,omitempty"`
	AcceptableStatuses  []resolver.Status `json:"acceptable_statuses,omitempty" yaml:"acceptable_statuses,omitempty"`
	AcceptablePrimary   []string          `json:"acceptable_primary,omitempty" yaml:"acceptable_primary,omitempty"`
	UnacceptablePrimary []string          `json:"unacceptable_primary,omitempty" yaml:"unacceptable_primary,omitempty"`
	NoSkill             bool              `json:"no_skill,omitempty" yaml:"no_skill,omitempty"`
	Question            *ExpectedQuestion `json:"question,omitempty" yaml:"question,omitempty"`
	Branches            map[string]Branch `json:"branches,omitempty" yaml:"branches,omitempty"`
	Supporting          []string          `json:"supporting,omitempty" yaml:"supporting,omitempty"`
	Rationale           string            `json:"rationale,omitempty" yaml:"rationale,omitempty"`
}

type ExpectedQuestion struct {
	ID    string `json:"id,omitempty" yaml:"id,omitempty"`
	Field string `json:"field" yaml:"field"`
}

type Branch struct {
	AcceptableStatuses []resolver.Status `json:"acceptable_statuses,omitempty" yaml:"acceptable_statuses,omitempty"`
	AcceptablePrimary  []string          `json:"acceptable_primary,omitempty" yaml:"acceptable_primary,omitempty"`
	NoSkill            bool              `json:"no_skill,omitempty" yaml:"no_skill,omitempty"`
}

type Suite struct {
	SchemaVersion int              `json:"schema_version" yaml:"schema_version"`
	ID            string           `json:"id" yaml:"id"`
	Sanitization  string           `json:"sanitization,omitempty" yaml:"sanitization,omitempty"`
	Skills        []resolver.Skill `json:"skills,omitempty" yaml:"skills,omitempty"`
	Cases         []Case           `json:"cases" yaml:"cases"`
}

type CalibrationPolicy struct {
	SchemaVersion    int    `json:"schema_version" yaml:"schema_version"`
	Corpus           string `json:"corpus" yaml:"corpus"`
	CalibrationSplit string `json:"calibration_split" yaml:"calibration_split"`
	HeldOutSplit     string `json:"held_out_split" yaml:"held_out_split"`
	CalibrationGrid  struct {
		ApplicabilityFloor []float64 `json:"applicability_floor" yaml:"applicability_floor"`
		MinimumMargin      []float64 `json:"minimum_margin" yaml:"minimum_margin"`
	} `json:"calibration_grid" yaml:"calibration_grid"`
	Selected struct {
		ApplicabilityFloor float64 `json:"applicability_floor" yaml:"applicability_floor"`
		MinimumMargin      float64 `json:"minimum_margin" yaml:"minimum_margin"`
	} `json:"selected" yaml:"selected"`
	HeldOutGates struct {
		ResolvedPrecision float64 `json:"resolved_precision_min" yaml:"resolved_precision_min"`
		NoSkillRecall     float64 `json:"no_skill_abstention_recall_min" yaml:"no_skill_abstention_recall_min"`
		AmbiguityRecall   float64 `json:"ambiguity_recall_min" yaml:"ambiguity_recall_min"`
		OverallAccuracy   float64 `json:"overall_accuracy_min" yaml:"overall_accuracy_min"`
	} `json:"held_out_gates" yaml:"held_out_gates"`
}

type BinaryIdentity struct {
	Version string `json:"version" yaml:"version"`
	Commit  string `json:"commit" yaml:"commit"`
	Digest  string `json:"digest,omitempty" yaml:"digest,omitempty"`
}

type ModelIdentity struct {
	Provider      string `json:"provider" yaml:"provider"`
	Model         string `json:"model" yaml:"model"`
	Configuration string `json:"configuration" yaml:"configuration"`
}

type Manifest struct {
	SchemaVersion        int            `json:"schema_version" yaml:"schema_version"`
	ExperimentID         string         `json:"experiment_id" yaml:"experiment_id"`
	CaseDigest           string         `json:"case_digest,omitempty" yaml:"case_digest,omitempty"`
	SuiteDigest          string         `json:"suite_digest,omitempty" yaml:"suite_digest,omitempty"`
	CatalogSnapshot      string         `json:"catalog_snapshot" yaml:"catalog_snapshot"`
	PolicyRevision       string         `json:"policy_revision" yaml:"policy_revision"`
	ProtocolSchema       string         `json:"protocol_schema" yaml:"protocol_schema"`
	NormalizationVersion string         `json:"normalization_version" yaml:"normalization_version"`
	IndexVersion         string         `json:"index_version" yaml:"index_version"`
	FactProviderFixture  string         `json:"fact_provider_fixture" yaml:"fact_provider_fixture"`
	FactProviderVersion  string         `json:"fact_provider_version" yaml:"fact_provider_version"`
	Seed                 int64          `json:"seed" yaml:"seed"`
	Binary               BinaryIdentity `json:"binary" yaml:"binary"`
	Model                *ModelIdentity `json:"model,omitempty" yaml:"model,omitempty"`
	Variant              string         `json:"variant" yaml:"variant"`
}

// Resolver is deliberately smaller than the production resolver type so replay
// can inject deterministic variants without coupling evaluation to construction.
type Resolver interface {
	Resolve(context.Context, resolver.Request) (resolver.Response, error)
}

// PinValidator must prove every behavior-affecting identity is available. It
// must never substitute a current/latest artifact for a missing pin.
type PinValidator interface {
	ValidatePins(context.Context, Manifest) error
}

type Clock interface{ Now() time.Time }

// ExecutionEvidence supplies measurements observed by the execution harness.
// Fixture counters are assertions only and are never treated as observations.
type ExecutionEvidence interface {
	Measurements(context.Context, Case, resolver.Response) (Measurements, error)
}

type Measurements struct {
	LatencyMS *float64           `json:"latency_ms,omitempty"`
	Counters  map[string]float64 `json:"counters,omitempty"`
}

type RunOptions struct {
	Partitions []Partition
	// Clock is retained for source compatibility. Timing is recorded only when
	// supplied through Evidence, so application replay does not invent latency.
	Clock            Clock
	Evidence         ExecutionEvidence
	BootstrapSamples int
}

type Interval struct {
	Low     float64 `json:"low"`
	High    float64 `json:"high"`
	Samples int     `json:"samples"`
}

type Metric struct {
	Value       *float64  `json:"value"`
	Numerator   int       `json:"numerator"`
	Denominator int       `json:"denominator"`
	CI95        *Interval `json:"ci95"`
}

type LatencyMetrics struct {
	Samples     int      `json:"samples"`
	P50MS       *float64 `json:"p50_ms"`
	P95MS       *float64 `json:"p95_ms"`
	P99MS       *float64 `json:"p99_ms"`
	MeanMS      *float64 `json:"mean_ms"`
	VarianceMS2 *float64 `json:"variance_ms2"`
}

type CounterMetric struct {
	Samples int     `json:"samples"`
	Sum     float64 `json:"sum"`
	Mean    float64 `json:"mean"`
}

type Metrics struct {
	AcceptableTop1        Metric                   `json:"acceptable_top1"`
	NoSkillPrecision      Metric                   `json:"no_skill_precision"`
	NoSkillRecall         Metric                   `json:"no_skill_recall"`
	NoSkillFalsePositive  Metric                   `json:"no_skill_false_positive"`
	ClarificationValidity Metric                   `json:"clarification_validity"`
	Latency               LatencyMetrics           `json:"latency"`
	Counters              map[string]CounterMetric `json:"counters"`
}

type Exclusion struct {
	CaseID string `json:"case_id"`
	Reason string `json:"reason"`
}

type Outcome struct {
	CaseID                     string             `json:"case_id"`
	Partition                  Partition          `json:"partition"`
	Tags                       []string           `json:"tags"`
	Status                     resolver.Status    `json:"status"`
	Primary                    string             `json:"primary,omitempty"`
	Supporting                 []string           `json:"supporting,omitempty"`
	Correct                    bool               `json:"correct"`
	ClarificationValid         bool               `json:"clarification_valid,omitempty"`
	MeasurementAssertionsValid *bool              `json:"measurement_assertions_valid,omitempty"`
	LatencyMS                  *float64           `json:"latency_ms,omitempty"`
	Counters                   map[string]float64 `json:"counters,omitempty"`
}

type Slice struct {
	Name    string  `json:"name"`
	Samples int     `json:"samples"`
	Metrics Metrics `json:"metrics"`
}

type Report struct {
	SchemaVersion int         `json:"schema_version"`
	Manifest      Manifest    `json:"manifest"`
	Samples       int         `json:"samples"`
	Exclusions    []Exclusion `json:"exclusions"`
	Metrics       Metrics     `json:"metrics"`
	Slices        []Slice     `json:"slices"`
	Outcomes      []Outcome   `json:"outcomes"`
}

type VariantComparison struct {
	SchemaVersion                int       `json:"schema_version"`
	Baseline                     string    `json:"baseline"`
	Candidate                    string    `json:"candidate"`
	PairedSamples                int       `json:"paired_samples"`
	CorrectnessDelta             *float64  `json:"correctness_delta"`
	CorrectnessDeltaCI95         *Interval `json:"correctness_delta_ci95"`
	BaselineCorrectnessVariance  *float64  `json:"baseline_correctness_variance"`
	CandidateCorrectnessVariance *float64  `json:"candidate_correctness_variance"`
	LatencyPairedSamples         int       `json:"latency_paired_samples"`
	LatencyDeltaMeanMS           *float64  `json:"latency_delta_mean_ms"`
	LatencyDeltaVarianceMS2      *float64  `json:"latency_delta_variance_ms2"`
}
