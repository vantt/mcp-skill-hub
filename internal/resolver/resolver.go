package resolver

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type Resolver struct {
	catalog Catalog
	policy  Policy
	cache   *Cache
}

func New(catalog Catalog, policy Policy, cache *Cache) (*Resolver, error) {
	if catalog == nil {
		return nil, fmt.Errorf("catalog is required")
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if cache == nil {
		cache = NewCache(256)
	}
	return &Resolver{catalog: catalog, policy: policy, cache: cache}, nil
}

// applyPrior validates a clarification answer without stored state. Resolution
// is deterministic for a given snapshot and policy, so the prior-free request is
// replayed and the issued resolution and question must be reproduced exactly.
func (resolver *Resolver) applyPrior(ctx context.Context, request Request) (Request, error) {
	prior := request.Prior
	original := request
	original.Prior = nil
	issued, err := resolver.Resolve(ctx, original)
	if err != nil {
		return Request{}, fmt.Errorf("replay original request for prior clarification: %w", err)
	}
	if issued.Status != StatusNeedsContext || issued.Question == nil || issued.ResolutionID != prior.ResolutionID {
		return Request{}, fmt.Errorf("prior clarification was not issued by this resolver")
	}
	if issued.ContextRevision != prior.ContextRevision || issued.Question.ID != prior.QuestionID {
		return Request{}, fmt.Errorf("prior clarification is stale or does not match this request")
	}
	question := *issued.Question
	answer := ""
	for _, choice := range question.Choices {
		if equalEvidence(choice, prior.Answer) {
			answer = choice
			break
		}
	}
	if answer == "" {
		return Request{}, fmt.Errorf("prior clarification answer is not one of the issued choices")
	}
	if answer == "unknown" {
		return request, nil
	}
	switch {
	case question.Field == "context.execution.capabilities":
		capability := strings.TrimPrefix(question.ID, "capability-")
		if answer == "available" {
			request.Context.Execution.Capabilities = append(request.Context.Execution.Capabilities, capability)
		} else if answer == "unavailable" {
			request.Context.Execution.UnavailableCapabilities = append(request.Context.Execution.UnavailableCapabilities, capability)
		}
	case question.Field == "task.scope":
		request.Task.Scope = answer
	case strings.HasPrefix(question.Field, "context.facts."):
		keyValue := strings.TrimPrefix(question.Field, "context.facts.")
		parts := strings.SplitN(keyValue, "=", 2)
		if len(parts) != 2 {
			return Request{}, fmt.Errorf("issued fact clarification is malformed")
		}
		value := parts[1]
		if answer == "no" {
			value = "not:" + value
		}
		request.Context.Facts = append(request.Context.Facts, Fact{Key: parts[0], Value: value, Basis: "user", Scope: "current-task"})
	default:
		request.Context.Facts = append(request.Context.Facts, Fact{Key: question.Field, Value: answer, Basis: "user", Scope: "current-task"})
	}
	return NormalizeRequest(request)
}

func clarificationChangesDecision(top scoredCandidate, candidates []scoredCandidate) bool {
	if top.Compatibility.State != Unknown {
		return false
	}
	// A positive answer makes the leading candidate eligible; a negative answer
	// removes it. Therefore the primary/status necessarily differs, even when a
	// lower-ranked candidate becomes the fallback.
	return len(candidates) > 0
}

func (resolver *Resolver) Resolve(ctx context.Context, raw Request) (Response, error) {
	request, err := NormalizeRequest(raw)
	if err != nil {
		return Response{}, err
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if request.Prior != nil && request.Prior.Kind == "clarification" {
		request, err = resolver.applyPrior(ctx, request)
		if err != nil {
			return Response{}, err
		}
	}
	key := resolver.cacheKey(request)
	if response, ok := resolver.cache.Get(key); ok {
		response.RequestID = request.RequestID
		return response, nil
	}
	response := resolver.baseResponse(request, key)
	if covered := activeCoverage(request); covered != nil {
		response.Status = StatusAlreadyCovered
		response.CoveredBy = covered.ID
		response.CoverageBasis = "agent-host-declared"
		response.ReasonCodes = []string{"active_procedure_coverage"}
		resolver.cache.Put(key, response)
		return response, nil
	}
	if blocksPrimary(request.ActivationContext) {
		response.Status = StatusNoSkill
		response.NoSkill = &NoSkill{ReasonCode: "constraint_conflict", RetryWhen: "activation_context_changes"}
		response.ReasonCodes = []string{"active_primary_conflict"}
		resolver.cache.Put(key, response)
		return response, nil
	}
	skills, err := resolver.catalog.Skills(ctx)
	if err != nil {
		return Response{}, fmt.Errorf("load active skills: %w", err)
	}
	hits, err := resolver.catalog.Search(ctx, positiveQuery(request), resolver.policy.CandidateLimit)
	if err != nil {
		return Response{}, fmt.Errorf("search active skills: %w", err)
	}
	byID := make(map[string]Skill, len(skills))
	for _, skill := range skills {
		if err := validateProjectedSkill(skill); err != nil {
			return Response{}, fmt.Errorf("invalid projected skill %s: %w", skill.ID, err)
		}
		if skill.Status == "active" {
			byID[skill.ID] = skill
		}
	}
	candidateIDs := make(map[string]bool, len(hits))
	for _, hit := range hits {
		if _, ok := byID[hit.SkillID]; ok {
			candidateIDs[hit.SkillID] = true
		}
	}
	queryTokens := tokenize(positiveQuery(request))
	type ruleCandidate struct {
		id   string
		rank float64
	}
	var rules []ruleCandidate
	for _, skill := range byID {
		// The deterministic rule channel is a bounded global fallback. Exact
		// identity and facts outrank trigger evidence; operation is only a weak rule.
		rank := 0.0
		if explicitIdentity(skill, queryTokens) {
			rank = 4
		}
		if requirementRuleMatch(skill, request) && rank < 3 {
			rank = 3
		}
		if trigger, _ := triggerFeature(queryTokens, skill.Triggers, skill.Examples); trigger > 0 && rank < 2+trigger {
			rank = 2 + trigger
		}
		if contains(skill.Operations, request.Operation) && rank < 1 {
			rank = 1
		}
		if rank > 0 {
			rules = append(rules, ruleCandidate{id: skill.ID, rank: rank})
		}
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].rank != rules[j].rank {
			return rules[i].rank > rules[j].rank
		}
		return rules[i].id < rules[j].id
	})
	if len(rules) > resolver.policy.CandidateLimit {
		rules = rules[:resolver.policy.CandidateLimit]
	}
	for _, candidate := range rules {
		candidateIDs[candidate.id] = true
	}
	ids := make([]string, 0, len(candidateIDs))
	for id := range candidateIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	candidates := make([]scoredCandidate, 0, len(ids))
	hardReasons := map[string]int{}
	hardExcludedScore := 0.0
	for _, id := range ids {
		skill := byID[id]
		if scopeLevel(request.Task.Scope) > 0 && scopeLevel(skill.MinScope) > scopeLevel(request.Task.Scope) {
			hardReasons["below_min_scope"]++
			continue
		}
		compatibility := evaluateCompatibility(skill, request)
		if compatibility.State == Violated {
			hardReasons[compatibility.Reason]++
			continue
		}
		candidate := scoreSkill(skill, request, resolver.policy)
		if candidate.Exclusion == Violated {
			hardReasons[candidate.HardReason]++
			excludedRelevance := candidate.Score + resolver.policy.Weights.Constraint*candidate.Features.Constraint + resolver.policy.Weights.NotFor*candidate.Features.NotFor
			if excludedRelevance > hardExcludedScore {
				hardExcludedScore = excludedRelevance
			}
			continue
		}
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Skill.ID < candidates[j].Skill.ID
	})
	if len(candidates) == 0 {
		reason := "catalog_gap"
		if hardReasons["missing_required_capability"] > 0 {
			reason = "capability_unavailable"
		} else if hardReasons["constraint_conflict"] > 0 || hardReasons["not_for_match"] > 0 || hardReasons["incompatible_fact"] > 0 {
			reason = "constraint_conflict"
		} else if hardReasons["below_min_scope"] > 0 {
			reason = "below_min_scope"
		}

		return resolver.cacheNoSkill(key, response, reason), nil
	}
	top := candidates[0]
	if hardReasons["constraint_conflict"] > 0 && hardExcludedScore >= top.Score {
		return resolver.cacheNoSkill(key, response, "constraint_conflict"), nil
	}
	if top.Score < resolver.policy.ApplicabilityFloor {
		return resolver.cacheNoSkill(key, response, "below_applicability_floor"), nil
	}
	if top.Compatibility.State == Unknown {
		if question := missingEvidenceQuestion(top, request, resolver.policy); question != nil && clarificationChangesDecision(top, candidates) {
			response.Status = StatusNeedsContext
			response.Question = question
			response.ReasonCodes = []string{top.Compatibility.Reason}
			resolver.cache.Put(key, response)
			return response, nil
		}
		return resolver.cacheNoSkill(key, response, missingReason(top.Compatibility)), nil
	}
	if len(candidates) > 1 {
		second := candidates[1]
		linked := linkedByDiscriminator(top.Skill, second.Skill)
		secondApplicable := second.Score >= resolver.policy.ApplicabilityFloor || linked && second.Score >= resolver.policy.ApplicabilityFloor-.15
		closeScore := secondApplicable && top.Score-second.Score < resolver.policy.MinimumMargin
		curatedAmbiguity := secondApplicable && linked && top.Score-second.Score < .25 && !singleDiscriminatorChoicePresent(top.Skill, second.Skill, request)
		if !secondApplicable {
			// The runner-up cannot become a valid recommendation.
		} else if canonical, ok := preferredEquivalent(top, second); ok {
			top = canonical
			response.Warnings = append(response.Warnings, "equivalent_candidates_canonicalized")
			response.ReasonCodes = append(response.ReasonCodes, "canonical_equivalent_preference")
		} else if closeScore && (equivalent(top.Skill, second.Skill) || nearDuplicate(top.Skill, second.Skill) || second.Features.Trigger >= .30 && top.Score-second.Score < resolver.policy.AmbiguityWindow) || curatedAmbiguity {
			if question := ambiguityQuestion(top.Skill, second.Skill, request, resolver.policy); question != nil {
				response.Status = StatusNeedsContext
				response.Question = question
				response.ReasonCodes = []string{"missing_discriminator"}
				resolver.cache.Put(key, response)
				return response, nil
			}
			return resolver.cacheNoSkill(key, response, "unresolved_ambiguity"), nil
		}
	}
	response.Status = StatusResolved
	confidence := "medium"
	if top.Score >= resolver.policy.HighConfidence {
		confidence = "high"
	}
	response.Primary = &Recommendation{ID: top.Skill.ID, Version: top.Skill.Digest, URI: "skill://" + top.Skill.CollectionID + "/" + top.Skill.ID, Applicability: top.Skill.Description, Confidence: confidence}
	response.ReasonCodes = uniqueSorted(append(response.ReasonCodes, top.Reasons...))
	response.Supporting = resolver.supporting(top.Skill, byID, request)
	resolver.cache.Put(key, response)
	return response, nil
}

func validateProjectedSkill(skill Skill) error {
	for _, relation := range skill.Supporting {
		if relation.SkillID == "" || relation.Operation == "" || !operations[relation.Operation] || !supportRoles[relation.Role] || !supportActivations[relation.Activation] {
			return fmt.Errorf("unexpected supporting metadata")
		}
	}
	for _, relation := range skill.Equivalence {
		if relation.SkillID == "" || !equivalencePreferences[relation.Preference] || !equivalenceVersionPolicies[relation.VersionPolicy] {
			return fmt.Errorf("unexpected equivalence metadata")
		}
	}
	return nil
}

func (resolver *Resolver) supporting(primary Skill, skills map[string]Skill, request Request) []Supporting {
	var result []Supporting
	relations := append([]SupportRelation(nil), primary.Supporting...)
	sort.Slice(relations, func(i, j int) bool {
		return relations[i].SkillID+"\x00"+relations[i].Operation+"\x00"+relations[i].Role < relations[j].SkillID+"\x00"+relations[j].Operation+"\x00"+relations[j].Role
	})
	for _, relation := range relations {
		if len(result) >= resolver.policy.MaximumSupporting || (relation.Operation != "" && relation.Operation != request.Operation) {
			continue
		}
		skill, ok := skills[relation.SkillID]
		if !ok || (scopeLevel(request.Task.Scope) > 0 && scopeLevel(skill.MinScope) > scopeLevel(request.Task.Scope)) {
			continue
		}
		candidate := scoreSkill(skill, request, resolver.policy)
		if candidate.HardReason != "" || candidate.Compatibility.State != Satisfied || candidate.Score < resolver.policy.SupportingFloor {
			continue
		}
		result = append(result, Supporting{ID: skill.ID, Version: skill.Digest, Role: relation.Role, Activation: relation.Activation})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (resolver *Resolver) baseResponse(request Request, key string) Response {
	revision := 1
	if request.Prior != nil {
		revision = request.Prior.ContextRevision + 1
	}
	return Response{SchemaVersion: SchemaVersion, ResolutionID: "res_" + strings.TrimPrefix(fingerprint(key), "sha256:")[:24], RequestID: request.RequestID, ContextRevision: revision, CatalogSnapshot: resolver.catalog.Snapshot(), PolicyRevision: resolver.policy.Revision, ReasonCodes: []string{}, ValidFor: ValidFor{Operation: request.Operation, ScopeFingerprint: fingerprint(struct {
		Task     Task
		Artifact *Artifact
	}{request.Task, request.Context.ActiveArtifact})}, Supporting: []Supporting{}}
}

func (resolver *Resolver) cacheKey(request Request) string {
	semantic := request
	semantic.RequestID = ""
	return fingerprint(struct {
		Request           string
		CatalogSnapshot   string
		PolicyRevision    string
		FactSnapshot      string
		ActivationContext string
	}{fingerprint(semantic), resolver.catalog.Snapshot(), resolver.policy.Revision, fingerprint(request.Context.Facts), fingerprint(request.ActivationContext)})
}

func (resolver *Resolver) cacheNoSkill(key string, response Response, reason string) Response {
	response.Status = StatusNoSkill
	response.NoSkill = &NoSkill{ReasonCode: reason, RetryWhen: "task_scope_changes"}
	response.ReasonCodes = []string{reason}
	resolver.cache.Put(key, response)
	return response
}

func activeCoverage(request Request) *ActiveProcedure {
	if request.ActivationContext == nil {
		return nil
	}
	query := tokenize(request.Task.Description)
	for index := range request.ActivationContext.ActiveProcedures {
		procedure := &request.ActivationContext.ActiveProcedures[index]
		if procedure.Role != "primary" || procedure.StateBasis == "agent-reported" {
			continue
		}
		if overlap(query, tokenize(procedure.Summary+" "+procedure.Scope)) >= .42 {
			return procedure
		}
	}
	return nil
}

func blocksPrimary(context *ActivationContext) bool {
	if context == nil {
		return false
	}
	if context.Mode == "supplement-only" || context.Mode == "coverage-check" {
		return true
	}
	for _, procedure := range context.ActiveProcedures {
		if procedure.Role == "primary" && !procedure.Replaceable && procedure.StateBasis != "agent-reported" {
			return true
		}
	}
	return false
}

func explicitIdentity(skill Skill, query []string) bool {
	if contains(query, strings.ToLower(skill.ID)) {
		return true
	}
	for _, alias := range skill.Aliases {
		if overlap(query, tokenize(alias)) == 1 {
			return true
		}
	}
	return false
}

func requirementRuleMatch(skill Skill, request Request) bool {
	for _, required := range append(append([]Requirement{}, skill.Requirements.FactsAll...), skill.Requirements.FactsAny...) {
		for _, fact := range request.Context.Facts {
			if normalizeIdentifier(required.Key) == fact.Key && equalEvidence(required.Value, fact.Value) {
				return true
			}
		}
	}
	return false
}

func missingEvidenceQuestion(candidate scoredCandidate, request Request, policy Policy) *Question {
	if request.Prior != nil || policy.ClarificationBudget == 0 {
		return nil
	}
	if candidate.Compatibility.MissingCapability != "" {
		value := candidate.Compatibility.MissingCapability
		return &Question{ID: "capability-" + value, Field: "context.execution.capabilities", Text: "Is capability " + value + " available for this task?", Choices: []string{"available", "unavailable", "unknown"}, AnswerFrom: "existing-context-first"}
	}
	if candidate.Compatibility.MissingFact != nil {
		fact := candidate.Compatibility.MissingFact
		return &Question{ID: "fact-" + normalizeIdentifier(fact.Key), Field: "context.facts." + normalizeIdentifier(fact.Key) + "=" + normalizeText(fact.Value), Text: "Does " + fact.Key + " equal " + fact.Value + " in the active task scope?", Choices: []string{"yes", "no", "unknown"}, AnswerFrom: "existing-context-first"}
	}
	if candidate.Compatibility.MissingScope != "" {
		return &Question{ID: "scope-minimum-" + candidate.Compatibility.MissingScope, Field: "task.scope", Text: "What is the bounded scope of this task?", Choices: []string{"single_step", "multi_step", "project", "unknown"}, AnswerFrom: "existing-context-first"}
	}
	return nil
}

func linkedByDiscriminator(first, second Skill) bool {
	for _, discriminator := range append(first.DistinguishFrom, second.DistinguishFrom...) {
		if discriminator.SkillID == first.ID || discriminator.SkillID == second.ID {
			return true
		}
	}
	return false
}

func singleDiscriminatorChoicePresent(first, second Skill, request Request) bool {
	for _, discriminator := range append(first.DistinguishFrom, second.DistinguishFrom...) {
		for _, fact := range request.Context.Facts {
			if fact.Key == normalizeIdentifier(discriminator.Field) {
				return true
			}
		}
	}
	query := tokenize(request.Task.Description)
	matched := map[string]bool{}
	for _, discriminator := range append(first.DistinguishFrom, second.DistinguishFrom...) {
		for _, choice := range discriminator.Choices {
			choiceTokens := tokenize(strings.ReplaceAll(choice, "-", " "))
			if overlap(query, choiceTokens) >= .5 {
				matched[choice] = true
			}
		}
	}
	return len(matched) == 1
}

func ambiguityQuestion(first, second Skill, request Request, policy Policy) *Question {
	if request.Prior != nil || policy.ClarificationBudget == 0 {
		return nil
	}
	for _, discriminator := range append(first.DistinguishFrom, second.DistinguishFrom...) {
		if discriminator.SkillID == first.ID || discriminator.SkillID == second.ID {
			choices := append([]string(nil), discriminator.Choices...)
			if !contains(choices, "unknown") {
				choices = append(choices, "unknown")
			}
			return &Question{ID: "distinguish-" + first.ID + "-" + second.ID, Field: discriminator.Field, Text: discriminator.Question, Choices: choices, AnswerFrom: "existing-context-first"}
		}
	}
	return nil
}

func equivalent(first, second Skill) bool {
	if contains(first.EquivalentTo, second.ID) || contains(second.EquivalentTo, first.ID) {
		return true
	}
	for _, relation := range append(append([]EquivalenceRelation{}, first.Equivalence...), second.Equivalence...) {
		if relation.SkillID == first.ID || relation.SkillID == second.ID {
			return true
		}
	}
	return false
}

func preferredEquivalent(first, second scoredCandidate) (scoredCandidate, bool) {
	for _, relation := range first.Skill.Equivalence {
		if relation.SkillID != second.Skill.ID || relation.VersionPolicy == "" {
			continue
		}
		if relation.Preference == "self" && equivalenceVersionAllows(relation.VersionPolicy, first.Skill, second.Skill) {
			return first, true
		}
		if relation.Preference == "target" && equivalenceVersionAllows(relation.VersionPolicy, second.Skill, first.Skill) {
			return second, true
		}
	}
	for _, relation := range second.Skill.Equivalence {
		if relation.SkillID != first.Skill.ID || relation.VersionPolicy == "" {
			continue
		}
		if relation.Preference == "self" && equivalenceVersionAllows(relation.VersionPolicy, second.Skill, first.Skill) {
			return second, true
		}
		if relation.Preference == "target" && equivalenceVersionAllows(relation.VersionPolicy, first.Skill, second.Skill) {
			return first, true
		}
	}
	return scoredCandidate{}, false
}

func equivalenceVersionAllows(policy string, preferred, alternate Skill) bool {
	switch policy {
	case "exact":
		return preferred.Digest == alternate.Digest
	case "compatible":
		return preferred.CollectionID == alternate.CollectionID
	case "latest-reviewed":
		return preferred.Reviewed
	default:
		return false
	}
}

func nearDuplicate(first, second Skill) bool {
	return overlap(tokenize(first.Description+" "+strings.Join(first.Triggers, " ")), tokenize(second.Description+" "+strings.Join(second.Triggers, " "))) >= .75
}
func missingReason(compatibility Compatibility) string {
	if compatibility.MissingCapability != "" {
		return "capability_unavailable"
	}
	if compatibility.MissingScope != "" {
		return "missing_scope"
	}
	return "catalog_gap"
}
func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		if value != "" {
			seen[value] = true
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
