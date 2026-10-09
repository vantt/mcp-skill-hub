package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

const chainResolutionTTL = 2 * time.Hour

// RateMetric holds a calculated rate along with its exact numerator and denominator.
type RateMetric struct {
	Rate        *float64 `json:"rate"`
	Numerator   int64    `json:"numerator"`
	Denominator int64    `json:"denominator"`
	Status      string   `json:"status,omitempty"` // "ok" or "unknown"
}

// ChainMetrics contains O5 metrics aggregated across chains.
type ChainMetrics struct {
	TotalChains            int64             `json:"total_chains"`
	ChainsResolved         int64             `json:"chains_resolved"`
	ChainsNoSkill          int64             `json:"chains_no_skill"`
	ChainsNeedsContext     int64             `json:"chains_needs_context"`
	ChainsAlreadyCovered   int64             `json:"chains_already_covered"`
	ChainsFailed           int64             `json:"chains_failed"`
	AcceptanceRate         RateMetric        `json:"acceptance_rate"`
	OverrideRate           RateMetric        `json:"override_rate"`
	FalseNoSkillRate       RateMetric        `json:"false_no_skill_rate"`
	TrueNoSkill            RateMetric        `json:"true_no_skill"`
	ReformulationRate      RateMetric        `json:"reformulation_rate"`
	IgnoreRate             RateMetric        `json:"ignore_rate"`
	NeedsContextAnswerRate RateMetric        `json:"needs_context_answer_rate"`
	BypassRate             RateMetric        `json:"bypass_rate"`
	NegativeAfterLoad      RateMetric        `json:"negative_after_load"`
	FirstValidDay          map[string]string `json:"first_valid_day,omitempty"`
}

// FunnelCut groups chain metrics by a chosen dimension (client, operation, or snapshot).
type FunnelCut struct {
	Key     string       `json:"key"`
	Metrics ChainMetrics `json:"metrics"`
}

// DisagreementChain represents one observed disagreement chain (override, after_no_skill, reformulation).
type DisagreementChain struct {
	ChainID          string   `json:"chain_id"`
	SessionHash      string   `json:"session_hash"`
	ResolutionID     string   `json:"resolution_id"`
	EventID          string   `json:"event_id"`
	OccurredAt       string   `json:"occurred_at"`
	Kind             string   `json:"kind"` // "override", "after_no_skill", "reformulation"
	Client           string   `json:"client"`
	Operation        string   `json:"operation,omitempty"`
	RecommendedSkill string   `json:"recommended_skill,omitempty"`
	LoadedSkill      string   `json:"loaded_skill,omitempty"`
	ReasonCodes      []string `json:"reason_codes,omitempty"`
	TopKRank         int      `json:"topk_rank,omitempty"`
	TopKMatched      string   `json:"topk_matched,omitempty"`
}

type chainResolution struct {
	SessionHash       string
	ResolutionID      string
	EventID           string
	OccurredAt        time.Time
	Status            string
	PrimaryID         string
	SupportingIDs     []string
	Operation         string
	CatalogSnapshot   string
	PolicyRevision    string
	Client            string
	PriorResolutionID string
	PriorKind         string
	PriorVerified     bool
	ReasonCodes       []string
	TopKSkillIDs      []string
	TopKMatched       []string
}

type chainLoad struct {
	SessionHash  string
	ResolutionID string
	OccurredAt   time.Time
	SkillID      string
	Attribution  string
	TopKSkillIDs []string
	TopKMatched  []string
}

type rawEventChain struct {
	Key          string
	SessionHash  string
	Resolutions  []chainResolution
	Loads        []chainLoad
	Reformulated bool
}

func makeRate(num, den int64) RateMetric {
	if den <= 0 {
		return RateMetric{Rate: nil, Numerator: num, Denominator: den, Status: "unknown"}
	}
	if num > den {
		r := float64(num) / float64(den)
		return RateMetric{Rate: &r, Numerator: num, Denominator: den, Status: "overflow"}
	}
	r := float64(num) / float64(den)
	if r < 0.0 {
		r = 0.0
	}
	return RateMetric{Rate: &r, Numerator: num, Denominator: den, Status: "ok"}
}

func (s UsageService) buildChains(rawEvents []telemetry.Event) ([]*rawEventChain, map[string]int64, map[string]string) {
	resolutions, loads, counts, firstValid := parseChainEvents(rawEvents)
	allChains, resToChain := linkResolutionChains(resolutions)
	associateChainLoads(allChains, resToChain, loads)
	return allChains, counts, firstValid
}

func parseChainEvents(rawEvents []telemetry.Event) ([]chainResolution, []chainLoad, map[string]int64, map[string]string) {
	var resolutions []chainResolution
	var loads []chainLoad
	counts := map[string]int64{}
	firstValid := map[string]string{}

	recordFirstValid := func(metric string, t time.Time) {
		day := t.Format("2006-01-02")
		if cur, ok := firstValid[metric]; !ok || day < cur {
			firstValid[metric] = day
		}
	}

	for _, event := range rawEvents {
		switch event.Type {
		case telemetry.EventResolutionCompleted, telemetry.EventResolutionFailed:
			res := parseChainResolution(event)
			resolutions = append(resolutions, res)
			recordFirstValid("resolution", event.OccurredAt)
		case telemetry.EventSkillLoaded:
			if load, ok := parseChainLoad(event); ok {
				loads = append(loads, load)
				recordFirstValid("load", event.OccurredAt)
			}
		case telemetry.EventClarificationRequested:
			counts["clarification_requested"]++
			recordFirstValid("needs_context_answer_rate", event.OccurredAt)
		case telemetry.EventClarificationAnswered:
			counts["clarification_answered"]++
			recordFirstValid("needs_context_answer_rate", event.OccurredAt)
		case telemetry.EventTranscriptToolObserved:
			counts["transcript_skill_uses"]++
			if resolvedBefore, ok := event.Payload["resolved_before"].(bool); ok && !resolvedBefore {
				counts["native_no_resolve"]++
			}
			recordFirstValid("bypass_rate", event.OccurredAt)
		case telemetry.EventSkillUtilityReported, telemetry.EventTaskOutcomeReported:
			if afterLoad, ok := event.Payload["after_load"].(bool); ok && afterLoad {
				utility, _ := event.Payload["utility"].(string)
				status, _ := event.Payload["status"].(string)
				if utility == "harmful" || status == "failed" || status == "rejected" {
					counts["negative_after_load"]++
					recordFirstValid("negative_after_load", event.OccurredAt)
				}
			}
		}
	}

	sort.Slice(resolutions, func(i, j int) bool {
		return resolutions[i].OccurredAt.Before(resolutions[j].OccurredAt)
	})
	return resolutions, loads, counts, firstValid
}

func parseChainResolution(event telemetry.Event) chainResolution {
	res := chainResolution{
		SessionHash:     event.SessionIDHash,
		ResolutionID:    event.ResolutionID,
		EventID:         event.ID,
		OccurredAt:      event.OccurredAt,
		CatalogSnapshot: event.CatalogSnapshot,
		PolicyRevision:  event.PolicyRevision,
		Client:          event.Client.Name,
	}
	if res.Client == "" {
		res.Client = "skillhub"
	}
	if s, ok := event.Payload["status"].(string); ok {
		res.Status = s
	} else {
		res.Status = "resolved"
	}
	if op, ok := event.Payload["operation"].(string); ok {
		res.Operation = op
	}
	if top, ok := event.Payload["top_skill_id"].(string); ok {
		res.PrimaryID = top
	}
	if prim, ok := event.Payload["skill_id"].(string); ok && res.PrimaryID == "" {
		res.PrimaryID = prim
	}
	if recs, ok := event.Payload["recommended_skill_ids"].([]string); ok {
		res.SupportingIDs = recs
	}
	if reasons, ok := event.Payload["reason_codes"].([]string); ok {
		res.ReasonCodes = reasons
	}
	if priorID, ok := event.Payload["prior_resolution_id"].(string); ok {
		res.PriorResolutionID = priorID
	}
	if priorKind, ok := event.Payload["prior_kind"].(string); ok {
		res.PriorKind = priorKind
	}
	if priorVer, ok := event.Payload["prior_verified"].(bool); ok {
		res.PriorVerified = priorVer
	}
	if topkIDs, ok := event.Payload["topk_skill_ids"].([]any); ok {
		for _, val := range topkIDs {
			if str, ok := val.(string); ok {
				res.TopKSkillIDs = append(res.TopKSkillIDs, str)
			}
		}
	} else if topkIDsStr, ok := event.Payload["topk_skill_ids"].([]string); ok {
		res.TopKSkillIDs = topkIDsStr
	}
	if topkMatched, ok := event.Payload["topk_matched"].([]any); ok {
		for _, val := range topkMatched {
			if str, ok := val.(string); ok {
				res.TopKMatched = append(res.TopKMatched, str)
			}
		}
	} else if topkMatchedStr, ok := event.Payload["topk_matched"].([]string); ok {
		res.TopKMatched = topkMatchedStr
	}
	return res
}

func parseChainLoad(event telemetry.Event) (chainLoad, bool) {
	basis, _ := event.Payload["basis"].(string)
	status, _ := event.Payload["status"].(string)
	if basis != telemetry.LoadBasisServerObserved || status == "review_required" {
		return chainLoad{}, false
	}
	skillID, _ := event.Payload["skill_id"].(string)
	attr, _ := event.Payload["attribution"].(string)
	load := chainLoad{
		SessionHash:  event.SessionIDHash,
		ResolutionID: event.ResolutionID,
		OccurredAt:   event.OccurredAt,
		SkillID:      skillID,
		Attribution:  attr,
	}
	if topkIDs, ok := event.Payload["topk_skill_ids"].([]any); ok {
		for _, val := range topkIDs {
			if str, ok := val.(string); ok {
				load.TopKSkillIDs = append(load.TopKSkillIDs, str)
			}
		}
	} else if topkIDsStr, ok := event.Payload["topk_skill_ids"].([]string); ok {
		load.TopKSkillIDs = topkIDsStr
	}
	if topkMatched, ok := event.Payload["topk_matched"].([]any); ok {
		for _, val := range topkMatched {
			if str, ok := val.(string); ok {
				load.TopKMatched = append(load.TopKMatched, str)
			}
		}
	} else if topkMatchedStr, ok := event.Payload["topk_matched"].([]string); ok {
		load.TopKMatched = topkMatchedStr
	}
	return load, true
}

func linkResolutionChains(resolutions []chainResolution) ([]*rawEventChain, map[string]*rawEventChain) {
	var allChains []*rawEventChain
	resToChain := make(map[string]*rawEventChain)

	for _, res := range resolutions {
		var targetChain *rawEventChain
		if res.SessionHash != "" && res.PriorResolutionID != "" && res.PriorVerified {
			if existing, ok := resToChain[res.PriorResolutionID]; ok && existing.SessionHash == res.SessionHash {
				targetChain = existing
			}
		}
		if targetChain != nil {
			targetChain.Resolutions = append(targetChain.Resolutions, res)
			if res.PriorKind == "rejected" && res.PriorVerified {
				targetChain.Reformulated = true
			}
			if res.ResolutionID != "" {
				resToChain[res.ResolutionID] = targetChain
			}
		} else {
			key := fmt.Sprintf("%s:%s:%s", res.SessionHash, res.ResolutionID, res.EventID)
			chain := &rawEventChain{
				Key:          key,
				SessionHash:  res.SessionHash,
				Resolutions:  []chainResolution{res},
				Reformulated: res.PriorKind == "rejected" && res.PriorVerified,
			}
			if res.ResolutionID != "" {
				resToChain[res.ResolutionID] = chain
			}
			allChains = append(allChains, chain)
		}
	}
	return allChains, resToChain
}

func associateChainLoads(allChains []*rawEventChain, resToChain map[string]*rawEventChain, loads []chainLoad) {
	for _, load := range loads {
		if load.ResolutionID != "" {
			if chain, ok := resToChain[load.ResolutionID]; ok {
				chain.Loads = append(chain.Loads, load)
				continue
			}
		}
		if load.SessionHash != "" {
			var bestChain *rawEventChain
			for i := len(allChains) - 1; i >= 0; i-- {
				ch := allChains[i]
				if ch.SessionHash == load.SessionHash && len(ch.Resolutions) > 0 {
					newest := ch.Resolutions[len(ch.Resolutions)-1]
					if load.OccurredAt.Sub(newest.OccurredAt) <= chainResolutionTTL && !load.OccurredAt.Before(newest.OccurredAt.Add(-time.Minute)) {
						bestChain = ch
						break
					}
				}
			}
			if bestChain != nil {
				bestChain.Loads = append(bestChain.Loads, load)
			}
		}
	}
}

func calculateChainMetrics(chains []*rawEventChain, counts map[string]int64, firstValid map[string]string, now time.Time, skillFilter string) ChainMetrics {
	var totalChains int64
	var chainsResolved, chainsNoSkill, chainsNeedsContext, chainsAlreadyCovered, chainsFailed int64
	var acceptedCount, overrideCount, falseNoSkillCount, trueNoSkillCount, ignoredCount, reformulationCount int64
	var noSkillTTLOldCount, resolvedTTLOldCount int64
	var totalAttributedLoads int64

	for _, ch := range chains {
		if len(ch.Resolutions) == 0 {
			continue
		}
		newest := ch.Resolutions[len(ch.Resolutions)-1]
		if skillFilter != "" && newest.PrimaryID != skillFilter {
			continue
		}

		totalChains++
		if ch.Reformulated {
			reformulationCount++
		}

		hasPrimaryLoad := false
		hasDifferentLoad := false
		for _, load := range ch.Loads {
			if newest.PrimaryID != "" && load.SkillID == newest.PrimaryID {
				hasPrimaryLoad = true
			} else if load.SkillID != "" {
				hasDifferentLoad = true
			}
		}
		if len(ch.Loads) > 0 {
			totalAttributedLoads++
		}

		isTTLOld := now.Sub(newest.OccurredAt) >= chainResolutionTTL

		switch newest.Status {
		case "already_covered":
			chainsAlreadyCovered++

		case "resolved":
			chainsResolved++
			if hasPrimaryLoad {
				acceptedCount++
			} else if hasDifferentLoad {
				overrideCount++
			} else {
				if isTTLOld {
					resolvedTTLOldCount++
					ignoredCount++
				}
			}

		case "no_skill":
			chainsNoSkill++
			if hasPrimaryLoad || hasDifferentLoad {
				falseNoSkillCount++
			} else {
				if isTTLOld {
					noSkillTTLOldCount++
					trueNoSkillCount++
				}
			}

		case "needs_context":
			chainsNeedsContext++

		case "failed":
			chainsFailed++
		}
	}

	metrics := ChainMetrics{
		TotalChains:            totalChains,
		ChainsResolved:         chainsResolved,
		ChainsNoSkill:          chainsNoSkill,
		ChainsNeedsContext:     chainsNeedsContext,
		ChainsAlreadyCovered:   chainsAlreadyCovered,
		ChainsFailed:           chainsFailed,
		AcceptanceRate:         makeRate(acceptedCount, chainsResolved),
		OverrideRate:           makeRate(overrideCount, chainsResolved),
		FalseNoSkillRate:       makeRate(falseNoSkillCount, chainsNoSkill),
		ReformulationRate:      makeRate(reformulationCount, totalChains),
		NeedsContextAnswerRate: makeRate(counts["clarification_answered"], counts["clarification_requested"]),
		BypassRate:             makeRate(counts["native_no_resolve"], counts["transcript_skill_uses"]),
		NegativeAfterLoad:      makeRate(counts["negative_after_load"], totalAttributedLoads),
		FirstValidDay:          firstValid,
	}

	// Offline-only ignore_rate and true_no_skill: report unknown when indeterminate
	if resolvedTTLOldCount > 0 {
		metrics.IgnoreRate = makeRate(ignoredCount, resolvedTTLOldCount)
	} else {
		metrics.IgnoreRate = RateMetric{Rate: nil, Numerator: 0, Denominator: 0, Status: "unknown"}
	}

	if noSkillTTLOldCount > 0 {
		metrics.TrueNoSkill = makeRate(trueNoSkillCount, noSkillTTLOldCount)
	} else {
		metrics.TrueNoSkill = RateMetric{Rate: nil, Numerator: 0, Denominator: 0, Status: "unknown"}
	}

	return metrics
}

func calculateCuts(chains []*rawEventChain, counts map[string]int64, firstValid map[string]string, now time.Time, by string) []FunnelCut {
	if by == "" {
		return nil
	}
	grouped := make(map[string][]*rawEventChain)
	for _, ch := range chains {
		if len(ch.Resolutions) == 0 {
			continue
		}
		newest := ch.Resolutions[len(ch.Resolutions)-1]
		key := "other"
		switch by {
		case "client":
			key = newest.Client
			if key == "" {
				key = "other"
			}
		case "operation":
			key = newest.Operation
			if key == "" {
				key = "unspecified"
			}
		case "snapshot":
			key = newest.CatalogSnapshot
			if key == "" {
				key = "unknown"
			}
		}
		grouped[key] = append(grouped[key], ch)
	}

	var keys []string
	for k := range grouped {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var cuts []FunnelCut
	for _, k := range keys {
		cutChains := grouped[k]
		cutMetrics := calculateChainMetrics(cutChains, counts, firstValid, now, "")
		cuts = append(cuts, FunnelCut{
			Key:     k,
			Metrics: cutMetrics,
		})
	}
	return cuts
}

// Chains returns disagreement chains (override, after_no_skill, reformulation) within the given window.
func (service UsageService) Chains(ctx context.Context, path string, since time.Time, kindFilter string) ([]DisagreementChain, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return nil, err
	}
	recorder, err := service.Telemetry.Open(root)
	if err != nil {
		return nil, err
	}
	defer recorder.Close(ctx)

	fromStr := since.UTC().Format("2006-01-02")
	toStr := time.Now().UTC().Format("2006-01-02")
	rawEvents, err := recorder.RawEvents(ctx, fromStr, toStr)
	if err != nil {
		return nil, err
	}

	chains, _, _ := service.buildChains(rawEvents)
	var result []DisagreementChain

	for _, ch := range chains {
		if len(ch.Resolutions) == 0 {
			continue
		}
		newest := ch.Resolutions[len(ch.Resolutions)-1]

		hasPrimaryLoad := false
		var differentSkill string
		for _, load := range ch.Loads {
			if newest.PrimaryID != "" && load.SkillID == newest.PrimaryID {
				hasPrimaryLoad = true
			} else if load.SkillID != "" {
				differentSkill = load.SkillID
			}
		}

		chainKind := ""
		loadedSkill := ""
		if newest.Status == "resolved" && differentSkill != "" {
			chainKind = "override"
			loadedSkill = differentSkill
		} else if newest.Status == "no_skill" && (hasPrimaryLoad || differentSkill != "") {
			chainKind = "after_no_skill"
			if differentSkill != "" {
				loadedSkill = differentSkill
			} else {
				loadedSkill = newest.PrimaryID
			}
		} else if ch.Reformulated {
			chainKind = "reformulation"
		}

		if chainKind == "" {
			continue
		}
		if kindFilter != "" && kindFilter != chainKind {
			continue
		}

		dc := DisagreementChain{
			ChainID:          ch.Key,
			SessionHash:      ch.SessionHash,
			ResolutionID:     newest.ResolutionID,
			EventID:          newest.EventID,
			OccurredAt:       newest.OccurredAt.UTC().Format(time.RFC3339),
			Kind:             chainKind,
			Client:           newest.Client,
			Operation:        newest.Operation,
			RecommendedSkill: newest.PrimaryID,
			LoadedSkill:      loadedSkill,
			ReasonCodes:      newest.ReasonCodes,
		}
		if loadedSkill != "" {
			topkIDs := newest.TopKSkillIDs
			topkMatched := newest.TopKMatched
			if len(topkIDs) == 0 {
				for _, l := range ch.Loads {
					if l.SkillID == loadedSkill && len(l.TopKSkillIDs) > 0 {
						topkIDs = l.TopKSkillIDs
						topkMatched = l.TopKMatched
						break
					}
				}
			}
			for i, id := range topkIDs {
				if id == loadedSkill {
					dc.TopKRank = i + 1
					prefix := fmt.Sprintf("%d:", dc.TopKRank)
					var matched []string
					for _, m := range topkMatched {
						if strings.HasPrefix(m, prefix) {
							matched = append(matched, strings.TrimPrefix(m, prefix))
						}
					}
					dc.TopKMatched = strings.Join(matched, ",")
					break
				}
			}
		}
		result = append(result, dc)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].OccurredAt > result[j].OccurredAt
	})
	return result, nil
}
