package resolver

import "fmt"

type Policy struct {
	Revision            string  `json:"revision"`
	CandidateLimit      int     `json:"candidate_limit"`
	ApplicabilityFloor  float64 `json:"applicability_floor"`
	HighConfidence      float64 `json:"high_confidence"`
	MinimumMargin       float64 `json:"minimum_margin"`
	AmbiguityWindow     float64 `json:"ambiguity_window"`
	SupportingFloor     float64 `json:"supporting_floor"`
	MaximumSupporting   int     `json:"maximum_supporting"`
	ClarificationBudget int     `json:"clarification_budget"`
	Weights             Weights `json:"weights"`
	VectorEnabled       bool    `json:"vector_enabled"`
	LLMEnabled          bool    `json:"llm_enabled"`
}

type Weights struct {
	Lexical    float64 `json:"lexical"`
	Trigger    float64 `json:"trigger"`
	Artifact   float64 `json:"artifact"`
	Fact       float64 `json:"fact"`
	Operation  float64 `json:"operation"`
	Quality    float64 `json:"quality"`
	NotFor     float64 `json:"not_for_penalty"`
	Constraint float64 `json:"constraint_penalty"`
	Scope      float64 `json:"scope_penalty"`
}

func DefaultPolicy() Policy {
	policy := Policy{
		CandidateLimit: 40, ApplicabilityFloor: 0.16, HighConfidence: 0.68,
		MinimumMargin: 0.04, AmbiguityWindow: 0.04, SupportingFloor: 0.08,
		MaximumSupporting: 2, ClarificationBudget: 1,
		Weights: Weights{Lexical: .28, Trigger: .44, Artifact: .08, Fact: .10, Operation: .15, Quality: .03, NotFor: .36, Constraint: .48, Scope: .25},
	}
	policy.Revision = fingerprint(struct {
		CandidateLimit                                                                      int
		ApplicabilityFloor, HighConfidence, MinimumMargin, AmbiguityWindow, SupportingFloor float64
		MaximumSupporting, ClarificationBudget                                              int
		Weights                                                                             Weights
	}{policy.CandidateLimit, policy.ApplicabilityFloor, policy.HighConfidence, policy.MinimumMargin, policy.AmbiguityWindow, policy.SupportingFloor, policy.MaximumSupporting, policy.ClarificationBudget, policy.Weights})
	return policy
}

func (policy Policy) Validate() error {
	if policy.Revision == "" {
		return fmt.Errorf("policy revision is required")
	}
	if policy.CandidateLimit < 1 || policy.CandidateLimit > 200 {
		return fmt.Errorf("candidate limit must be between 1 and 200")
	}
	for name, value := range map[string]float64{"applicability floor": policy.ApplicabilityFloor, "high confidence": policy.HighConfidence, "minimum margin": policy.MinimumMargin, "ambiguity window": policy.AmbiguityWindow, "supporting floor": policy.SupportingFloor} {
		if value < 0 || value > 1 {
			return fmt.Errorf("%s must be between zero and one", name)
		}
	}
	if policy.HighConfidence < policy.ApplicabilityFloor {
		return fmt.Errorf("high confidence must not be below the applicability floor")
	}
	positive := policy.Weights.Lexical + policy.Weights.Trigger + policy.Weights.Artifact + policy.Weights.Fact + policy.Weights.Operation + policy.Weights.Quality
	for name, value := range map[string]float64{"lexical weight": policy.Weights.Lexical, "trigger weight": policy.Weights.Trigger, "artifact weight": policy.Weights.Artifact, "fact weight": policy.Weights.Fact, "operation weight": policy.Weights.Operation, "quality weight": policy.Weights.Quality, "not-for penalty": policy.Weights.NotFor, "constraint penalty": policy.Weights.Constraint, "scope penalty": policy.Weights.Scope} {
		if value < 0 || value > 2 {
			return fmt.Errorf("%s must be between zero and two", name)
		}
	}
	if positive == 0 {
		return fmt.Errorf("policy must include positive evidence weights")
	}
	if policy.MaximumSupporting < 0 || policy.MaximumSupporting > 2 {
		return fmt.Errorf("maximum supporting must be between zero and two")
	}
	if policy.ClarificationBudget < 0 || policy.ClarificationBudget > 1 {
		return fmt.Errorf("clarification budget must be zero or one")
	}
	if policy.VectorEnabled || policy.LLMEnabled {
		return fmt.Errorf("vector and LLM extensions are disabled in this resolver version")
	}
	return nil
}
