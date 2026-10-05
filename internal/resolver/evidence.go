package resolver

import (
	"sort"
	"strings"
	"unicode"
)

type Truth string

const (
	Satisfied Truth = "satisfied"
	Violated  Truth = "violated"
	Unknown   Truth = "unknown"
)

type Compatibility struct {
	State             Truth
	MissingFact       *Requirement
	MissingCapability string
	MissingScope      string
	Reason            string
}

func evaluateRequirements(requirements Requirements, request Request) Compatibility {
	factsAll := evaluateFactGroup(requirements.FactsAll, request.Context.Facts, true)
	factsAny := evaluateFactGroup(requirements.FactsAny, request.Context.Facts, false)
	capsAll := evaluateCapabilityGroup(requirements.CapabilitiesAll, request.Context.Execution, true)
	capsAny := evaluateCapabilityGroup(requirements.CapabilitiesAny, request.Context.Execution, false)
	checks := []Compatibility{factsAll, factsAny, capsAll, capsAny}
	result := Compatibility{State: Satisfied}
	for _, check := range checks {
		if check.State == Violated {
			return check
		}
		if check.State == Unknown && result.State == Satisfied {
			result = check
		}
	}
	return result
}

func evaluateCompatibility(skill Skill, request Request) Compatibility {
	if skill.MinScope != "" {
		if request.Task.Scope == "" {
			return Compatibility{State: Unknown, MissingScope: skill.MinScope, Reason: "missing_scope"}
		}
		if scopeLevel(request.Task.Scope) < scopeLevel(skill.MinScope) {
			return Compatibility{State: Violated, MissingScope: skill.MinScope, Reason: "below_min_scope"}
		}
	}
	return evaluateRequirements(skill.Requirements, request)
}

func evaluateFactGroup(required []Requirement, facts []Fact, all bool) Compatibility {
	if len(required) == 0 {
		return Compatibility{State: Satisfied}
	}
	states := make([]Compatibility, 0, len(required))
	for index := range required {
		requirement := required[index]
		state := Compatibility{State: Unknown, MissingFact: &requirement, Reason: "missing_required_fact"}
		contradiction := false
		for _, fact := range facts {
			if fact.Key != normalizeIdentifier(requirement.Key) || !hardFactEvidence(fact) {
				continue
			}
			if equalEvidence(fact.Value, requirement.Value) {
				state = Compatibility{State: Satisfied}
				break
			}
			contradiction = true
		}
		if state.State != Satisfied && contradiction {
			state = Compatibility{State: Violated, Reason: "incompatible_fact"}
		}
		states = append(states, state)
	}
	return aggregate(states, all)
}

func hardFactEvidence(fact Fact) bool {
	if fact.Basis == "agent-inference" {
		return false
	}
	return fact.Scope == "" || fact.Scope == "current-task" || fact.Scope == "active-component"
}

func evaluateCapabilityGroup(required []string, execution Execution, all bool) Compatibility {
	if len(required) == 0 {
		return Compatibility{State: Satisfied}
	}
	available, unavailable := sliceSet(execution.Capabilities), sliceSet(execution.UnavailableCapabilities)
	states := make([]Compatibility, 0, len(required))
	for _, capability := range required {
		capability = normalizeIdentifier(capability)
		switch {
		case available[capability]:
			states = append(states, Compatibility{State: Satisfied})
		case unavailable[capability]:
			states = append(states, Compatibility{State: Violated, MissingCapability: capability, Reason: "missing_required_capability"})
		default:
			states = append(states, Compatibility{State: Unknown, MissingCapability: capability, Reason: "missing_required_capability"})
		}
	}
	return aggregate(states, all)
}

func aggregate(states []Compatibility, all bool) Compatibility {
	if all {
		for _, state := range states {
			if state.State == Violated {
				return state
			}
		}
		for _, state := range states {
			if state.State == Unknown {
				return state
			}
		}
		return Compatibility{State: Satisfied}
	}
	for _, state := range states {
		if state.State == Satisfied {
			return state
		}
	}
	for _, state := range states {
		if state.State == Unknown {
			return state
		}
	}
	return states[0]
}

type scoredCandidate struct {
	Skill         Skill
	Score         float64
	Reasons       []string
	Compatibility Compatibility
	Exclusion     Truth
	HardReason    string
	Features      features
}

type features struct {
	Lexical, Trigger, Artifact, Fact, Operation, Quality, NotFor, Constraint, Scope float64
}

var stopWords = setOf("a", "an", "and", "are", "as", "at", "be", "by", "for", "from", "in", "into", "is", "it", "of", "on", "or", "the", "this", "to", "with", "without", "please", "current", "task")

func tokenize(value string) []string {
	value = strings.ToLower(normalizeText(value))
	words := strings.FieldsFunc(value, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '+' || r == '#')
	})
	seen := map[string]bool{}
	result := make([]string, 0, len(words))
	for _, word := range words {
		if len([]rune(word)) < 2 || stopWords[word] || seen[word] {
			continue
		}
		seen[word] = true
		result = append(result, word)
	}
	sort.Strings(result)
	return result
}

func overlap(left, right []string) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	set := sliceSet(left)
	common := 0
	for _, value := range right {
		if set[value] {
			common++
		}
	}
	denominator := len(left)
	if len(right) > denominator {
		denominator = len(right)
	}
	return float64(common) / float64(denominator)
}

func bestOverlap(query []string, values []string) float64 {
	best := 0.0
	for _, value := range values {
		if current := overlap(query, tokenize(value)); current > best {
			best = current
		}
	}
	return best
}

func scopeLevel(value string) int {
	switch value {
	case "single_step":
		return 1
	case "multi_step":
		return 2
	case "project":
		return 3
	default:
		return 0
	}
}

func exclusionClauses(value string) []string {
	clauses := strings.FieldsFunc(strings.ToLower(normalizeText(value)), func(r rune) bool {
		return r == ';' || r == '.' || r == '\n'
	})
	var result []string
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		for _, prefix := range []string{"do not use ", "don't use ", "must not use ", "avoid using ", "do not ", "don't ", "must not ", "avoid ", "exclude "} {
			if strings.HasPrefix(clause, prefix) {
				term := strings.Trim(strings.TrimSpace(strings.TrimPrefix(clause, prefix)), "\"'")
				if term != "" {
					result = append(result, term)
				}
				break
			}
		}
	}
	return result
}

func excludedSkill(skill Skill, request Request) bool {
	if contains(request.Task.Exclusions.SkillIDs, normalizeIdentifier(skill.ID)) {
		return true
	}
	identities := append([]string{skill.ID, skill.Name}, skill.Aliases...)
	for _, term := range request.Task.Exclusions.Terms {
		term = normalizeText(term)
		for _, identity := range identities {
			if equalEvidence(term, identity) || strings.Contains(strings.ToLower(term), strings.ToLower(identity)) {
				return true
			}
		}
	}
	for _, constraint := range request.Task.Constraints {
		for _, term := range exclusionClauses(constraint) {
			for _, target := range append(identities, skill.Triggers...) {
				if equalEvidence(term, target) || strings.Contains(strings.ToLower(term), strings.ToLower(target)) || overlap(tokenize(term), tokenize(target)) >= .8 {
					return true
				}
			}
		}
	}
	return false
}

const ExampleDiscount = 0.9

func triggerFeature(query []string, triggers, examples []string) (float64, bool) {
	tScore := bestOverlap(query, triggers)
	eScore := ExampleDiscount * bestOverlap(query, examples)
	if eScore > tScore {
		return eScore, true
	}
	return tScore, false
}

var techFactKeys = map[string]bool{
	"language": true, "framework": true, "library": true, "dependency": true,
	"technology": true, "runtime": true, "platform": true,
}

func scoreSkill(skill Skill, request Request, policy Policy) scoredCandidate {
	query := tokenize(positiveQuery(request))
	metaParts := append([]string{skill.Name, skill.Description}, skill.Aliases...)
	metaParts = append(metaParts, skill.Topics...)
	metaParts = append(metaParts, skill.Technologies...)
	metadata := tokenize(strings.Join(metaParts, " "))
	tScore, exampleWon := triggerFeature(query, skill.Triggers, skill.Examples)
	f := features{Lexical: overlap(query, metadata), Trigger: tScore}
	if artifact := request.Context.ActiveArtifact; artifact != nil {
		f.Artifact = bestOverlap(tokenize(artifact.Kind+" "+artifact.Language), append(skill.Triggers, skill.Description))
	}
	var techEvidence []string
	if artifact := request.Context.ActiveArtifact; artifact != nil && artifact.Language != "" {
		techEvidence = append(techEvidence, artifact.Language)
	}
	for _, fact := range request.Context.Facts {
		if techFactKeys[normalizeIdentifier(fact.Key)] && fact.Value != "" {
			techEvidence = append(techEvidence, fact.Value)
		}
	}
	techMatched := false
	for _, tech := range skill.Technologies {
		for _, ev := range techEvidence {
			if equalEvidence(tech, ev) {
				techMatched = true
				break
			}
		}
		if techMatched {
			break
		}
	}
	if techMatched && f.Artifact < 1.0 {
		f.Artifact = 1.0
	}
	factTotal := len(skill.Requirements.FactsAll) + len(skill.Requirements.FactsAny)
	if factTotal > 0 {
		satisfied := 0
		for _, required := range append(append([]Requirement{}, skill.Requirements.FactsAll...), skill.Requirements.FactsAny...) {
			for _, fact := range request.Context.Facts {
				if fact.Key == normalizeIdentifier(required.Key) && equalEvidence(fact.Value, required.Value) {
					satisfied++
					break
				}
			}
		}
		f.Fact = float64(satisfied) / float64(factTotal)
	}
	for _, discriminator := range skill.DistinguishFrom {
		for _, fact := range request.Context.Facts {
			if fact.Key == normalizeIdentifier(discriminator.Field) && discriminatorSupportsSkill(skill, discriminator, fact.Value) {
				f.Fact = 1
			}
		}
	}
	if contains(skill.Operations, request.Operation) {
		f.Operation = 1
	}
	if skill.Reviewed {
		f.Quality = 1
	}
	notForScore := bestOverlap(query, skill.NotFor)
	if counterScore := bestOverlap(query, skill.CounterExamples); counterScore > notForScore {
		notForScore = counterScore
	}
	f.NotFor = notForScore
	if excludedSkill(skill, request) {
		f.Constraint = 1
	}
	if scopeLevel(request.Task.Scope) > 0 && scopeLevel(skill.MinScope) > scopeLevel(request.Task.Scope) {
		f.Scope = float64(scopeLevel(skill.MinScope)-scopeLevel(request.Task.Scope)) / 2
	}
	compatibility := evaluateCompatibility(skill, request)
	exclusion := Satisfied
	hardReason := ""
	if f.NotFor >= .72 {
		exclusion, hardReason = Violated, "not_for_match"
	} else if f.NotFor >= .35 {
		exclusion = Unknown
	}
	if excludedSkill(skill, request) {
		exclusion, hardReason = Violated, "constraint_conflict"
	}
	if compatibility.State == Violated {
		hardReason = compatibility.Reason
	}
	score := policy.Weights.Lexical*f.Lexical + policy.Weights.Trigger*f.Trigger + policy.Weights.Artifact*f.Artifact + policy.Weights.Fact*f.Fact + policy.Weights.Operation*f.Operation + policy.Weights.Quality*f.Quality - policy.Weights.NotFor*f.NotFor - policy.Weights.Constraint*f.Constraint - policy.Weights.Scope*f.Scope
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	reasons := []string{}
	if f.Lexical > 0 {
		reasons = append(reasons, "task_match")
	}
	if exampleWon && f.Trigger >= .3 {
		reasons = append(reasons, "example_match")
	} else if f.Trigger >= .3 {
		reasons = append(reasons, "trigger_match")
	}
	if f.Artifact > 0 {
		reasons = append(reasons, "artifact_match")
	}
	if techMatched {
		reasons = append(reasons, "technology_match")
	}
	if f.Fact > 0 {
		reasons = append(reasons, "fact_match")
	}
	if f.NotFor > 0 {
		reasons = append(reasons, "not_for_penalty")
	}
	if f.Constraint > 0 {
		reasons = append(reasons, "constraint_penalty")
	}
	return scoredCandidate{Skill: skill, Score: score, Reasons: reasons, Compatibility: compatibility, Exclusion: exclusion, HardReason: hardReason, Features: f}
}

func discriminatorSupportsSkill(skill Skill, discriminator Discriminator, answer string) bool {
	answerTokens := tokenize(strings.ReplaceAll(answer, "-", " "))
	selfIdentity := []string{skill.ID, skill.Name, skill.Description}
	if bestOverlap(answerTokens, selfIdentity) > 0 {
		return true
	}
	for _, choice := range discriminator.Choices {
		if equalEvidence(choice, answer) {
			continue
		}
		choiceTokens := tokenize(strings.ReplaceAll(choice, "-", " "))
		if bestOverlap(choiceTokens, selfIdentity) > 0 {
			return false
		}
		if bestOverlap(choiceTokens, []string{discriminator.SkillID}) > 0 {
			return true
		}
	}
	return false
}

func positiveQuery(request Request) string {
	parts := []string{request.Task.Description, request.Operation}
	if artifact := request.Context.ActiveArtifact; artifact != nil {
		parts = append(parts, artifact.Kind, artifact.Language, pathBase(artifact.PathHint))
	}
	if signal := request.Context.Execution.Signal; signal != nil {
		parts = append(parts, signal.Kind, signal.Summary)
	}
	return strings.Join(parts, " ")
}

func equalEvidence(first, second string) bool {
	return strings.EqualFold(normalizeText(first), normalizeText(second))
}

func pathBase(value string) string {
	if index := strings.LastIndex(value, "/"); index >= 0 {
		return value[index+1:]
	}
	return value
}
func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
