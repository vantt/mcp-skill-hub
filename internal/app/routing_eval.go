package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/evaluation"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

type RoutingEvalCaseFailure struct {
	SkillID    string `json:"skill_id,omitempty"`
	Kind       string `json:"kind"` // "positive", "counter", "no_skill"
	Index      int    `json:"index"`
	Phrase     string `json:"phrase"`
	GotStatus  string `json:"got_status"`
	GotPrimary string `json:"got_primary,omitempty"`
}

type RoutingSkillMetrics struct {
	SkillID string   `json:"skill_id"`
	Total   int      `json:"total"`
	Correct int      `json:"correct"`
	Recall  *float64 `json:"recall"`
}

type RoutingEvalReport struct {
	TotalCases        int                      `json:"total_cases"`
	PositiveCases     int                      `json:"positive_cases"`
	CounterCases      int                      `json:"counter_cases"`
	NoSkillCases      int                      `json:"no_skill_cases"`
	Precision         *float64                 `json:"precision"`
	Recall            *float64                 `json:"recall"`
	NoSkillRecall     *float64                 `json:"no_skill_recall"`
	NoSkillPrecision  *float64                 `json:"no_skill_precision"`
	FalsePositiveRate *float64                 `json:"false_positive_rate"`
	SkillMetrics      []RoutingSkillMetrics    `json:"skill_metrics"`
	Failures          []RoutingEvalCaseFailure `json:"failures,omitempty"`
}

type RoutingEvalOptions struct {
	WorkspacePath string
	NoSkillFile   string
	PolicyFile    string
	Policy        *resolverpkg.Policy
}

type leaveOneOutCatalog struct {
	resolverpkg.Catalog
	targetSkillID string
	removeKind    string // "example" or "counter_example"
	removeIndex   int
}

func (c leaveOneOutCatalog) Skills(ctx context.Context) ([]resolverpkg.Skill, error) {
	skills, err := c.Catalog.Skills(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]resolverpkg.Skill, len(skills))
	for i, s := range skills {
		if s.ID == c.targetSkillID {
			sCopy := s
			if c.removeKind == "example" && c.removeIndex >= 0 && c.removeIndex < len(s.Examples) {
				ex := make([]string, 0, len(s.Examples)-1)
				ex = append(ex, s.Examples[:c.removeIndex]...)
				ex = append(ex, s.Examples[c.removeIndex+1:]...)
				sCopy.Examples = ex
			} else if c.removeKind == "counter_example" && c.removeIndex >= 0 && c.removeIndex < len(s.CounterExamples) {
				cx := make([]string, 0, len(s.CounterExamples)-1)
				cx = append(cx, s.CounterExamples[:c.removeIndex]...)
				cx = append(cx, s.CounterExamples[c.removeIndex+1:]...)
				sCopy.CounterExamples = cx
			}
			result[i] = sCopy
		} else {
			result[i] = s
		}
	}
	return result, nil
}

type routingEvalAccumulator struct {
	correctP                   int
	totalP                     int
	allResolvedPZ              int
	allNoSkillPZ               int
	totalN                     int
	countersResolvedToOwnSkill int
	totalZ                     int
	noSkillZ                   int
	zResolvedToAny             int
	failures                   []RoutingEvalCaseFailure
	skillMetrics               []RoutingSkillMetrics
}

func loadEvalPolicy(ctx context.Context, handle *catalog.Handle, opts RoutingEvalOptions) (resolverpkg.Policy, error) {
	if opts.Policy != nil {
		return *opts.Policy, nil
	}
	if opts.PolicyFile != "" {
		policyBytes, err := os.ReadFile(opts.PolicyFile)
		if err != nil {
			return resolverpkg.Policy{}, fmt.Errorf("read policy file: %w", err)
		}
		var parsed any
		if err := yaml.Unmarshal(policyBytes, &parsed); err != nil {
			return resolverpkg.Policy{}, fmt.Errorf("unmarshal policy YAML: %w", err)
		}
		jsonBytes, err := json.Marshal(parsed)
		if err != nil {
			return resolverpkg.Policy{}, fmt.Errorf("marshal policy JSON: %w", err)
		}
		sum := sha256.Sum256(jsonBytes)
		digest := "sha256:" + hex.EncodeToString(sum[:])
		p, err := resolverpkg.ParsePolicy(digest, jsonBytes)
		if err != nil {
			return resolverpkg.Policy{}, fmt.Errorf("parse policy: %w", err)
		}
		return p, nil
	}
	p, err := resolverpkg.LoadPolicy(ctx, handle.DB)
	if err != nil {
		return resolverpkg.Policy{}, fmt.Errorf("load catalog policy: %w", err)
	}
	return p, nil
}

// EvaluateRouting evaluates the deterministic resolver across all active skills'
// examples and counter-examples using leave-one-out cross-validation, and optionally
// across a dedicated no-skill test suite.
func EvaluateRouting(ctx context.Context, opts RoutingEvalOptions) (RoutingEvalReport, error) {
	root, err := workspace.Discover(opts.WorkspacePath)
	if err != nil {
		return RoutingEvalReport{}, err
	}

	handle, err := catalog.OpenCurrentLocked(ctx, root)
	if err != nil {
		return RoutingEvalReport{}, fmt.Errorf("open current catalog: %w", err)
	}
	defer handle.Close()

	policy, err := loadEvalPolicy(ctx, handle, opts)
	if err != nil {
		return RoutingEvalReport{}, err
	}

	sqliteCatalog, err := resolverpkg.NewSQLiteCatalog(handle.DB, handle.Pointer.CatalogSnapshot)
	if err != nil {
		return RoutingEvalReport{}, err
	}
	allSkills, err := sqliteCatalog.Skills(ctx)
	if err != nil {
		return RoutingEvalReport{}, err
	}

	var activeSkills []resolverpkg.Skill
	for _, s := range allSkills {
		if s.Status == "active" && s.ID != "system-curator" {
			activeSkills = append(activeSkills, s)
		}
	}
	sort.Slice(activeSkills, func(i, j int) bool {
		return activeSkills[i].ID < activeSkills[j].ID
	})

	resolverService := ResolverService{Policy: policy, Telemetry: nil}
	var acc routingEvalAccumulator

	if err := evaluatePositiveCases(ctx, root, handle, activeSkills, resolverService, &acc); err != nil {
		return RoutingEvalReport{}, err
	}
	if err := evaluateCounterCases(ctx, root, handle, activeSkills, resolverService, &acc); err != nil {
		return RoutingEvalReport{}, err
	}
	if opts.NoSkillFile != "" {
		if err := evaluateNoSkillCases(ctx, root, handle, opts.NoSkillFile, resolverService, &acc); err != nil {
			return RoutingEvalReport{}, err
		}
	}

	return acc.toReport(), nil
}

func evaluatePositiveCases(ctx context.Context, root string, handle *catalog.Handle, skills []resolverpkg.Skill, svc ResolverService, acc *routingEvalAccumulator) error {
	for _, skill := range skills {
		skillTotal := len(skill.Examples)
		skillCorrect := 0
		facts := buildPreconditionFacts(skill)
		caps := buildPreconditionCapabilities(skill)
		op := ""
		if len(skill.Operations) > 0 {
			op = skill.Operations[0]
		}

		for i, phrase := range skill.Examples {
			acc.totalP++
			req := resolverpkg.Request{
				SchemaVersion: "1",
				RequestID:     fmt.Sprintf("routing-eval-%s-ex-%d", skill.ID, i),
				Task: resolverpkg.Task{
					Description: phrase,
					Scope:       skill.MinScope,
				},
				Operation: op,
				Context: resolverpkg.RequestContext{
					Facts: facts,
					Execution: resolverpkg.Execution{
						Capabilities: caps,
					},
				},
			}

			decorator := func(base resolverpkg.Catalog) resolverpkg.Catalog {
				return leaveOneOutCatalog{
					Catalog:       base,
					targetSkillID: skill.ID,
					removeKind:    "example",
					removeIndex:   i,
				}
			}

			inst := svc
			inst.Cache = resolverpkg.NewCache(64)
			resp, err := inst.resolveWithin(ctx, root, handle, req, decorator)
			if err != nil {
				return fmt.Errorf("resolve positive case for %s: %w", skill.ID, err)
			}

			if resp.Status == resolverpkg.StatusResolved {
				acc.allResolvedPZ++
			}
			if resp.Status == resolverpkg.StatusNoSkill {
				acc.allNoSkillPZ++
			}

			if resp.Status == resolverpkg.StatusResolved && resp.Primary != nil && resp.Primary.ID == skill.ID {
				acc.correctP++
				skillCorrect++
			} else {
				gotPrimary := ""
				if resp.Primary != nil {
					gotPrimary = resp.Primary.ID
				}
				acc.failures = append(acc.failures, RoutingEvalCaseFailure{
					SkillID:    skill.ID,
					Kind:       "positive",
					Index:      i,
					Phrase:     phrase,
					GotStatus:  string(resp.Status),
					GotPrimary: gotPrimary,
				})
			}
		}

		if skillTotal > 0 {
			r := float64(skillCorrect) / float64(skillTotal)
			acc.skillMetrics = append(acc.skillMetrics, RoutingSkillMetrics{
				SkillID: skill.ID,
				Total:   skillTotal,
				Correct: skillCorrect,
				Recall:  &r,
			})
		}
	}
	return nil
}

func evaluateCounterCases(ctx context.Context, root string, handle *catalog.Handle, skills []resolverpkg.Skill, svc ResolverService, acc *routingEvalAccumulator) error {
	for _, skill := range skills {
		facts := buildPreconditionFacts(skill)
		caps := buildPreconditionCapabilities(skill)
		op := ""
		if len(skill.Operations) > 0 {
			op = skill.Operations[0]
		}

		for i, phrase := range skill.CounterExamples {
			acc.totalN++
			req := resolverpkg.Request{
				SchemaVersion: "1",
				RequestID:     fmt.Sprintf("routing-eval-%s-cx-%d", skill.ID, i),
				Task: resolverpkg.Task{
					Description: phrase,
					Scope:       skill.MinScope,
				},
				Operation: op,
				Context: resolverpkg.RequestContext{
					Facts: facts,
					Execution: resolverpkg.Execution{
						Capabilities: caps,
					},
				},
			}

			decorator := func(base resolverpkg.Catalog) resolverpkg.Catalog {
				return leaveOneOutCatalog{
					Catalog:       base,
					targetSkillID: skill.ID,
					removeKind:    "counter_example",
					removeIndex:   i,
				}
			}

			inst := svc
			inst.Cache = resolverpkg.NewCache(64)
			resp, err := inst.resolveWithin(ctx, root, handle, req, decorator)
			if err != nil {
				return fmt.Errorf("resolve counter case for %s: %w", skill.ID, err)
			}

			if resp.Primary != nil && resp.Primary.ID == skill.ID {
				acc.countersResolvedToOwnSkill++
				acc.failures = append(acc.failures, RoutingEvalCaseFailure{
					SkillID:    skill.ID,
					Kind:       "counter",
					Index:      i,
					Phrase:     phrase,
					GotStatus:  string(resp.Status),
					GotPrimary: resp.Primary.ID,
				})
			}
		}
	}
	return nil
}

func evaluateNoSkillCases(ctx context.Context, root string, handle *catalog.Handle, noSkillFile string, svc ResolverService, acc *routingEvalAccumulator) error {
	suite, err := evaluation.LoadSuite(noSkillFile)
	if err != nil {
		return fmt.Errorf("load no-skill suite %s: %w", noSkillFile, err)
	}
	acc.totalZ = len(suite.Cases)
	for index, c := range suite.Cases {
		inst := svc
		inst.Cache = resolverpkg.NewCache(64)
		resp, err := inst.resolveWithin(ctx, root, handle, c.Request, nil)
		if err != nil {
			return fmt.Errorf("resolve no-skill case %d: %w", index, err)
		}

		if resp.Status == resolverpkg.StatusResolved {
			acc.allResolvedPZ++
			acc.zResolvedToAny++
		}
		if resp.Status == resolverpkg.StatusNoSkill {
			acc.allNoSkillPZ++
			acc.noSkillZ++
		} else {
			gotPrimary := ""
			if resp.Primary != nil {
				gotPrimary = resp.Primary.ID
			}
			acc.failures = append(acc.failures, RoutingEvalCaseFailure{
				Kind:       "no_skill",
				Index:      index,
				Phrase:     c.Request.Task.Description,
				GotStatus:  string(resp.Status),
				GotPrimary: gotPrimary,
			})
		}
	}
	return nil
}

func (acc *routingEvalAccumulator) toReport() RoutingEvalReport {
	report := RoutingEvalReport{
		TotalCases:    acc.totalP + acc.totalN + acc.totalZ,
		PositiveCases: acc.totalP,
		CounterCases:  acc.totalN,
		NoSkillCases:  acc.totalZ,
		SkillMetrics:  acc.skillMetrics,
		Failures:      acc.failures,
	}

	if acc.allResolvedPZ > 0 {
		v := float64(acc.correctP) / float64(acc.allResolvedPZ)
		report.Precision = &v
	}
	if acc.totalP > 0 {
		v := float64(acc.correctP) / float64(acc.totalP)
		report.Recall = &v
	}
	if acc.totalZ > 0 {
		v := float64(acc.noSkillZ) / float64(acc.totalZ)
		report.NoSkillRecall = &v
	}
	if acc.allNoSkillPZ > 0 {
		v := float64(acc.noSkillZ) / float64(acc.allNoSkillPZ)
		report.NoSkillPrecision = &v
	}
	if (acc.totalN + acc.totalZ) > 0 {
		v := float64(acc.countersResolvedToOwnSkill+acc.zResolvedToAny) / float64(acc.totalN+acc.totalZ)
		report.FalsePositiveRate = &v
	}

	return report
}

func buildPreconditionFacts(skill resolverpkg.Skill) []resolverpkg.Fact {
	var facts []resolverpkg.Fact
	for _, req := range skill.Requirements.FactsAll {
		facts = append(facts, resolverpkg.Fact{
			Key:   req.Key,
			Value: req.Value,
			Basis: "user",
		})
	}
	if len(skill.Requirements.FactsAny) > 0 {
		first := skill.Requirements.FactsAny[0]
		facts = append(facts, resolverpkg.Fact{
			Key:   first.Key,
			Value: first.Value,
			Basis: "user",
		})
	}
	return facts
}

func buildPreconditionCapabilities(skill resolverpkg.Skill) []string {
	caps := append([]string{}, skill.Requirements.CapabilitiesAll...)
	if len(skill.Requirements.CapabilitiesAny) > 0 {
		caps = append(caps, skill.Requirements.CapabilitiesAny[0])
	}
	return caps
}
