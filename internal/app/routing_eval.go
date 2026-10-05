package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/evaluation"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// RoutingEvalQuery specifies parameters for running a routing evaluation.
type RoutingEvalQuery struct {
	WorkspacePath    string
	NoSkillPath      string
	PolicyPath       string
	Policy           *resolverpkg.Policy
	MinPrecision     *float64
	MinRecall        *float64
	MinNoSkillRecall *float64
	MaxFPR           *float64
}

// RoutingFailedCase records one evaluation case that failed expectations.
type RoutingFailedCase struct {
	SkillID    string `json:"skill_id"`
	Kind       string `json:"kind"` // "positive", "counter", "no_skill"
	Index      int    `json:"index"`
	Phrase     string `json:"phrase"`
	GotStatus  string `json:"got_status"`
	GotPrimary string `json:"got_primary,omitempty"`
}

// RoutingEvalReport summarizes metrics and gate checks for a routing evaluation.
type RoutingEvalReport struct {
	Precision1        *float64            `json:"precision_at_1"`
	Recall            *float64            `json:"recall"`
	NoSkillRecall     *float64            `json:"no_skill_recall"`
	NoSkillPrecision  *float64            `json:"no_skill_precision"`
	FalsePositiveRate *float64            `json:"false_positive_rate"`
	TotalPositives    int                 `json:"total_positives"`
	TotalCounters     int                 `json:"total_counters"`
	TotalNoSkill      int                 `json:"total_no_skill"`
	PerSkillRecall    map[string]float64  `json:"per_skill_recall"`
	FailedCases       []RoutingFailedCase `json:"failed_cases"`
	PassedGate        bool                `json:"passed_gate"`
	GateFailures      []string            `json:"gate_failures,omitempty"`
}

// RoutingEvalService runs leave-one-out routing evaluations.
type RoutingEvalService struct{}

type leaveOneOutCatalog struct {
	resolverpkg.Catalog
	targetSkillID string
	removeExample string
	removeCounter string
}

func (c leaveOneOutCatalog) Skills(ctx context.Context) ([]resolverpkg.Skill, error) {
	skills, err := c.Catalog.Skills(ctx)
	if err != nil {
		return nil, err
	}
	res := make([]resolverpkg.Skill, len(skills))
	for i, s := range skills {
		if s.ID == c.targetSkillID {
			sCopy := s
			if c.removeExample != "" {
				sCopy.Examples = slices.Clone(s.Examples)
				sCopy.Examples = slices.DeleteFunc(sCopy.Examples, func(ex string) bool {
					return ex == c.removeExample
				})
			}
			if c.removeCounter != "" {
				sCopy.CounterExamples = slices.Clone(s.CounterExamples)
				sCopy.CounterExamples = slices.DeleteFunc(sCopy.CounterExamples, func(cx string) bool {
					return cx == c.removeCounter
				})
			}
			res[i] = sCopy
		} else {
			res[i] = s
		}
	}
	return res, nil
}

func buildEvalRequest(s resolverpkg.Skill, reqID, phrase string) resolverpkg.Request {
	scope := s.MinScope
	if scope == "" {
		scope = "multi_step"
	}
	op := ""
	if len(s.Operations) > 0 {
		op = s.Operations[0]
	}
	var facts []resolverpkg.Fact
	for _, f := range s.Requirements.FactsAll {
		facts = append(facts, resolverpkg.Fact{Key: f.Key, Value: f.Value, Basis: "user"})
	}
	if len(s.Requirements.FactsAny) > 0 {
		f := s.Requirements.FactsAny[0]
		facts = append(facts, resolverpkg.Fact{Key: f.Key, Value: f.Value, Basis: "user"})
	}
	caps := slices.Clone(s.Requirements.CapabilitiesAll)
	if len(s.Requirements.CapabilitiesAny) > 0 {
		caps = append(caps, s.Requirements.CapabilitiesAny[0])
	}

	return resolverpkg.Request{
		SchemaVersion: "1",
		RequestID:     reqID,
		Task: resolverpkg.Task{
			Description: phrase,
			Scope:       scope,
		},
		Operation: op,
		Context: resolverpkg.RequestContext{
			Facts: facts,
			Execution: resolverpkg.Execution{
				Capabilities: caps,
			},
		},
	}
}

func loadCustomPolicy(policyPath string) (resolverpkg.Policy, error) {
	if policyPath == "" {
		return resolverpkg.Policy{}, nil
	}
	policyBytes, err := os.ReadFile(policyPath)
	if err != nil {
		return resolverpkg.Policy{}, fmt.Errorf("read policy file: %w", err)
	}
	var doc any
	if err := yaml.Unmarshal(policyBytes, &doc); err != nil {
		return resolverpkg.Policy{}, fmt.Errorf("parse policy YAML: %w", err)
	}
	jsonBytes, err := json.Marshal(doc)
	if err != nil {
		return resolverpkg.Policy{}, fmt.Errorf("encode policy JSON: %w", err)
	}
	policy, err := resolverpkg.ParsePolicy("custom", jsonBytes)
	if err != nil {
		return resolverpkg.Policy{}, fmt.Errorf("validate policy: %w", err)
	}
	return policy, nil
}

type evalCounts struct {
	correctResolvedP      int
	totalResolvedPZ       int
	totalNoSkillPZ        int
	lenP                  int
	lenN                  int
	lenZ                  int
	countersResolvedToOwn int
	zResolvedToAny        int
	noSkillInZ            int
}

func evalPositives(ctx context.Context, root string, handle *catalog.Handle, svc ResolverService, skills []resolverpkg.Skill) (counts evalCounts, perSkillRecall map[string]float64, failed []RoutingFailedCase, err error) {
	perPos := make(map[string]int)
	perCorrect := make(map[string]int)

	for _, s := range skills {
		perPos[s.ID] = len(s.Examples)
		for i, phrase := range s.Examples {
			counts.lenP++
			reqID := fmt.Sprintf("routing-eval-%s-ex-%d", s.ID, i)
			req := buildEvalRequest(s, reqID, phrase)

			decorate := func(cat resolverpkg.Catalog) resolverpkg.Catalog {
				return leaveOneOutCatalog{Catalog: cat, targetSkillID: s.ID, removeExample: phrase}
			}
			svc.Cache = resolverpkg.NewCache(1)
			resp, evalErr := svc.resolveWithin(ctx, root, handle, req, decorate)
			if evalErr != nil {
				return counts, nil, nil, fmt.Errorf("resolve positive case %s: %w", reqID, evalErr)
			}

			switch resp.Status {
			case resolverpkg.StatusResolved:
				counts.totalResolvedPZ++
			case resolverpkg.StatusNoSkill:
				counts.totalNoSkillPZ++
			}

			primID := ""
			if resp.Primary != nil {
				primID = resp.Primary.ID
			}

			if resp.Status == resolverpkg.StatusResolved && primID == s.ID {
				counts.correctResolvedP++
				perCorrect[s.ID]++
			} else {
				failed = append(failed, RoutingFailedCase{
					SkillID: s.ID, Kind: "positive", Index: i, Phrase: phrase, GotStatus: string(resp.Status), GotPrimary: primID,
				})
			}
		}
	}

	perSkillRecall = make(map[string]float64, len(skills))
	for id, total := range perPos {
		if total > 0 {
			perSkillRecall[id] = float64(perCorrect[id]) / float64(total)
		} else {
			perSkillRecall[id] = 0
		}
	}
	return counts, perSkillRecall, failed, nil
}

func evalCounters(ctx context.Context, root string, handle *catalog.Handle, svc ResolverService, skills []resolverpkg.Skill) (countersResolvedToOwn, lenN int, failed []RoutingFailedCase, err error) {
	for _, s := range skills {
		for i, phrase := range s.CounterExamples {
			lenN++
			reqID := fmt.Sprintf("routing-eval-%s-cx-%d", s.ID, i)
			req := buildEvalRequest(s, reqID, phrase)

			decorate := func(cat resolverpkg.Catalog) resolverpkg.Catalog {
				return leaveOneOutCatalog{Catalog: cat, targetSkillID: s.ID, removeCounter: phrase}
			}
			svc.Cache = resolverpkg.NewCache(1)
			resp, evalErr := svc.resolveWithin(ctx, root, handle, req, decorate)
			if evalErr != nil {
				return 0, 0, nil, fmt.Errorf("resolve counter case %s: %w", reqID, evalErr)
			}

			primID := ""
			if resp.Primary != nil {
				primID = resp.Primary.ID
			}

			if primID == s.ID {
				countersResolvedToOwn++
				failed = append(failed, RoutingFailedCase{
					SkillID: s.ID, Kind: "counter", Index: i, Phrase: phrase, GotStatus: string(resp.Status), GotPrimary: primID,
				})
			}
		}
	}
	return countersResolvedToOwn, lenN, failed, nil
}

func evalNoSkill(ctx context.Context, root string, handle *catalog.Handle, svc ResolverService, noSkillPath string) (zResolvedToAny, noSkillInZ, totalResolved, totalNoSkill, lenZ int, failed []RoutingFailedCase, err error) {
	if noSkillPath == "" {
		return 0, 0, 0, 0, 0, nil, nil
	}
	suite, err := evaluation.LoadSuite(noSkillPath)
	if err != nil {
		return 0, 0, 0, 0, 0, nil, fmt.Errorf("load no-skill suite: %w", err)
	}
	for i, c := range suite.Cases {
		lenZ++
		svc.Cache = resolverpkg.NewCache(1)
		resp, evalErr := svc.resolveWithin(ctx, root, handle, c.Request, nil)
		if evalErr != nil {
			return 0, 0, 0, 0, 0, nil, fmt.Errorf("resolve no-skill case %s: %w", c.ID, evalErr)
		}

		primID := ""
		if resp.Primary != nil {
			primID = resp.Primary.ID
		}

		switch resp.Status {
		case resolverpkg.StatusResolved:
			totalResolved++
			zResolvedToAny++
		case resolverpkg.StatusNoSkill:
			totalNoSkill++
			noSkillInZ++
		}

		if resp.Status != resolverpkg.StatusNoSkill {
			failed = append(failed, RoutingFailedCase{
				SkillID: primID, Kind: "no_skill", Index: i, Phrase: c.Request.Task.Description, GotStatus: string(resp.Status), GotPrimary: primID,
			})
		}
	}
	return zResolvedToAny, noSkillInZ, totalResolved, totalNoSkill, lenZ, failed, nil
}

func checkRoutingGate(report *RoutingEvalReport, q RoutingEvalQuery) {
	report.PassedGate = true
	var gateFailures []string

	if q.MinPrecision != nil {
		if report.Precision1 == nil || *report.Precision1 < *q.MinPrecision {
			report.PassedGate = false
			gateFailures = append(gateFailures, fmt.Sprintf("precision@1 below threshold: %v < %.2f", report.Precision1, *q.MinPrecision))
		}
	}
	if q.MinRecall != nil {
		if report.Recall == nil || *report.Recall < *q.MinRecall {
			report.PassedGate = false
			gateFailures = append(gateFailures, fmt.Sprintf("recall below threshold: %v < %.2f", report.Recall, *q.MinRecall))
		}
	}
	if q.MinNoSkillRecall != nil {
		if report.NoSkillRecall == nil || *report.NoSkillRecall < *q.MinNoSkillRecall {
			report.PassedGate = false
			gateFailures = append(gateFailures, fmt.Sprintf("no_skill_recall below threshold: %v < %.2f", report.NoSkillRecall, *q.MinNoSkillRecall))
		}
	}
	if q.MaxFPR != nil {
		if report.FalsePositiveRate == nil || *report.FalsePositiveRate > *q.MaxFPR {
			report.PassedGate = false
			gateFailures = append(gateFailures, fmt.Sprintf("false_positive_rate above threshold: %v > %.2f", report.FalsePositiveRate, *q.MaxFPR))
		}
	}
	report.GateFailures = gateFailures
}

// Run executes the leave-one-out routing evaluation and returns metrics.
func (service RoutingEvalService) Run(ctx context.Context, q RoutingEvalQuery) (report RoutingEvalReport, resultErr error) {
	root, err := workspace.Discover(q.WorkspacePath)
	if err != nil {
		return RoutingEvalReport{}, err
	}

	var customPolicy resolverpkg.Policy
	if q.Policy != nil {
		customPolicy = *q.Policy
	} else {
		customPolicy, err = loadCustomPolicy(q.PolicyPath)
		if err != nil {
			return RoutingEvalReport{}, err
		}
	}

	handle, err := catalog.OpenCurrentLocked(ctx, root)
	if err != nil {
		return RoutingEvalReport{}, err
	}
	defer func() {
		if closeErr := handle.Close(); resultErr == nil && closeErr != nil {
			resultErr = closeErr
		}
	}()

	catalogView, err := resolverpkg.NewSQLiteCatalog(handle.DB, handle.Pointer.CatalogSnapshot)
	if err != nil {
		return RoutingEvalReport{}, err
	}
	allSkills, err := catalogView.Skills(ctx)
	if err != nil {
		return RoutingEvalReport{}, err
	}

	var skills []resolverpkg.Skill
	for _, s := range allSkills {
		if s.ID != "system-curator" {
			skills = append(skills, s)
		}
	}

	resolverService := ResolverService{
		Policy:    customPolicy,
		Telemetry: nil,
	}

	counts, perSkillRecall, posFailed, err := evalPositives(ctx, root, handle, resolverService, skills)
	if err != nil {
		return RoutingEvalReport{}, err
	}

	countersToOwn, lenN, counterFailed, err := evalCounters(ctx, root, handle, resolverService, skills)
	if err != nil {
		return RoutingEvalReport{}, err
	}

	zToAny, nsInZ, nsResolved, nsNoSkill, lenZ, noSkillFailed, err := evalNoSkill(ctx, root, handle, resolverService, q.NoSkillPath)
	if err != nil {
		return RoutingEvalReport{}, err
	}

	counts.lenN = lenN
	counts.countersResolvedToOwn = countersToOwn
	counts.lenZ = lenZ
	counts.zResolvedToAny = zToAny
	counts.noSkillInZ = nsInZ
	counts.totalResolvedPZ += nsResolved
	counts.totalNoSkillPZ += nsNoSkill

	report.TotalPositives = counts.lenP
	report.TotalCounters = counts.lenN
	report.TotalNoSkill = counts.lenZ
	report.FailedCases = append(posFailed, append(counterFailed, noSkillFailed...)...)
	if report.FailedCases == nil {
		report.FailedCases = make([]RoutingFailedCase, 0)
	}

	if counts.totalResolvedPZ > 0 {
		p1 := float64(counts.correctResolvedP) / float64(counts.totalResolvedPZ)
		report.Precision1 = &p1
	}
	if counts.lenP > 0 {
		rec := float64(counts.correctResolvedP) / float64(counts.lenP)
		report.Recall = &rec
	}
	if counts.lenZ > 0 {
		nsRec := float64(counts.noSkillInZ) / float64(counts.lenZ)
		report.NoSkillRecall = &nsRec
	}
	if counts.totalNoSkillPZ > 0 {
		nsPrec := float64(counts.noSkillInZ) / float64(counts.totalNoSkillPZ)
		report.NoSkillPrecision = &nsPrec
	}
	if counts.lenN+counts.lenZ > 0 {
		fpr := float64(counts.countersResolvedToOwn+counts.zResolvedToAny) / float64(counts.lenN+counts.lenZ)
		report.FalsePositiveRate = &fpr
	}

	report.PerSkillRecall = perSkillRecall
	checkRoutingGate(&report, q)

	return report, nil
}
