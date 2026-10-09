package app

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

const (
	FunnelRawRetentionDays    = 30
	FunnelRollupRetentionDays = 180
	MaxFunnelDays             = 180
	DefaultFunnelDays         = 30
)

// FunnelQuery specifies the time window, optional skill filter, and optional cut dimension for a funnel report.
type FunnelQuery struct {
	Since   time.Time
	Until   time.Time
	SkillID string
	By      string // "client", "operation", "snapshot"
}

// FunnelWindow records the normalized inclusive date range and day count.
type FunnelWindow struct {
	Since string `json:"since"`
	Until string `json:"until"`
	Days  int    `json:"days"`
}

// OverallFunnel aggregates telemetry metrics across the entire workspace.
type OverallFunnel struct {
	Resolutions               map[string]int64 `json:"resolutions"`
	TotalResolutions          int64            `json:"total_resolutions"`
	RecommendedPrimary        int64            `json:"recommended_primary"`
	RecommendedSupporting     int64            `json:"recommended_supporting"`
	Activations               map[string]int64 `json:"activations"`
	TotalActivations          int64            `json:"total_activations"`
	AcceptanceRate            *float64         `json:"acceptance_rate"`
	Overrides                 int64            `json:"overrides"`
	Misses                    int64            `json:"misses"`
	Unsolicited               int64            `json:"unsolicited"`
	BlockedByReview           int64            `json:"blocked_by_review"`
	ResolutionsReviewRequired int64            `json:"resolutions_review_required"`
	ResolutionsSetupRequired  int64            `json:"resolutions_setup_required"`
	Loads                     map[string]int64 `json:"loads"`
	TotalLoads                int64            `json:"total_loads"`
	Doctor                    map[string]int64 `json:"doctor"`
	TotalDoctor               int64            `json:"total_doctor"`
	DoctorFailureRate         *float64         `json:"doctor_failure_rate"`
	SetupFailed               int64            `json:"setup_failed"`
	SetupFailedRate           *float64         `json:"setup_failed_rate"`
	NegativeFeedback          int64            `json:"negative_feedback"`
	NegativeAfterLoad         int64            `json:"negative_after_load"`
	Transcripts               map[string]int64 `json:"transcripts"`
	UnlistedResourceReads     int64            `json:"unlisted_resource_reads"`
	UnsupportedMethodCalls    int64            `json:"unsupported_method_calls"`
	SnapshotExpiredRequests   int64            `json:"snapshot_expired_requests"`
	ToolsListBytes            int64            `json:"tools_list_bytes"`
	ChainMetrics              *ChainMetrics    `json:"chain_metrics,omitempty"`
}

// SkillFunnel holds telemetry metrics for a single skill.
type SkillFunnel struct {
	SkillID                   string           `json:"skill_id"`
	Name                      string           `json:"name,omitempty"`
	RecommendedPrimary        int64            `json:"recommended_primary"`
	RecommendedSupporting     int64            `json:"recommended_supporting"`
	Activations               map[string]int64 `json:"activations"`
	TotalActivations          int64            `json:"total_activations"`
	AcceptanceRate            *float64         `json:"acceptance_rate"`
	Overrides                 int64            `json:"overrides"`
	Misses                    int64            `json:"misses"`
	Unsolicited               int64            `json:"unsolicited"`
	BlockedByReview           int64            `json:"blocked_by_review"`
	ResolutionsReviewRequired int64            `json:"resolutions_review_required"`
	ResolutionsSetupRequired  int64            `json:"resolutions_setup_required"`
	Loads                     map[string]int64 `json:"loads"`
	TotalLoads                int64            `json:"total_loads"`
	Doctor                    map[string]int64 `json:"doctor"`
	TotalDoctor               int64            `json:"total_doctor"`
	DoctorFailureRate         *float64         `json:"doctor_failure_rate"`
	SetupFailed               int64            `json:"setup_failed"`
	SetupFailedRate           *float64         `json:"setup_failed_rate"`
	NegativeFeedback          int64            `json:"negative_feedback"`
	NegativeAfterLoad         int64            `json:"negative_after_load"`
	UnlistedResourceReads     int64            `json:"unlisted_resource_reads"`
	SnapshotExpiredRequests   int64            `json:"snapshot_expired_requests"`
	Transcripts               map[string]int64 `json:"transcripts"`
	ChainMetrics              *ChainMetrics    `json:"chain_metrics,omitempty"`
}

// FunnelReport is the top-level report returned by UsageService.
type FunnelReport struct {
	Window                    FunnelWindow      `json:"window"`
	RawRetentionDays          int               `json:"raw_retention_days"`
	RollupRetentionDays       int               `json:"rollup_retention_days"`
	MetricBasis               map[string]string `json:"metric_basis"`
	Overall                   *OverallFunnel    `json:"overall,omitempty"`
	Skills                    []SkillFunnel     `json:"skills,omitempty"`
	Skill                     *SkillFunnel      `json:"skill,omitempty"`
	Cuts                      []FunnelCut       `json:"cuts,omitempty"`
	DeadSkills                []string          `json:"dead_skills,omitempty"`
	RecommendedNeverActivated []string          `json:"recommended_never_activated,omitempty"`
	BlockedByReview           []string          `json:"blocked_by_review,omitempty"`
	NegativeAfterLoad         []string          `json:"negative_after_load,omitempty"`
	SetupFailures             []string          `json:"setup_failures,omitempty"`
}

// UsageService aggregates daily rollups into human and machine-readable funnel reports.
type UsageService struct {
	Telemetry TelemetryService
	Skills    SkillService
}

func defaultMetricBasis() map[string]string {
	return map[string]string{
		"resolutions":                 "host-reported",
		"recommended_primary":         "server-observed",
		"recommended_supporting":      "server-observed",
		"activations":                 "server-observed",
		"acceptance_rate":             "server-observed",
		"overrides":                   "server-observed",
		"misses":                      "server-observed",
		"unsolicited":                 "server-observed",
		"blocked_by_review":           "server-observed",
		"resolutions_review_required": "server-observed",
		"resolutions_setup_required":  "server-observed",
		"loads":                       "server-observed",
		"doctor":                      "terminal",
		"doctor_failure_rate":         "terminal",
		"setup_failed":                "host-reported",
		"setup_failed_rate":           "host-reported",
		"negative_feedback":           "host-reported",
		"negative_after_load":         "host-reported",
		"transcripts":                 "transcript",
	}
}

type accumulator struct {
	resolutions               map[string]int64
	recommendedPrimary        int64
	recommendedSupporting     int64
	activations               map[string]int64
	totalActivations          int64
	overrides                 int64
	misses                    int64
	unsolicited               int64
	blockedByReview           int64
	resolutionsReviewRequired int64
	resolutionsSetupRequired  int64
	loads                     map[string]int64
	totalLoads                int64
	doctor                    map[string]int64
	totalDoctor               int64
	setupFailed               int64
	negativeFeedback          int64
	negativeAfterLoad         int64
	unlistedResourceReads     int64
	unsupportedMethodCalls    int64
	snapshotExpiredRequests   int64
	toolsListBytes            int64
	transcripts               map[string]int64
}

func newAccumulator() *accumulator {
	return &accumulator{
		resolutions: map[string]int64{
			"resolved": 0, "no_skill": 0, "needs_context": 0, "already_covered": 0, "failed": 0,
		},
		activations: map[string]int64{
			"recommended": 0, "supporting": 0, "override": 0, "after_no_skill": 0, "after_needs_context": 0, "unsolicited": 0,
		},
		loads: map[string]int64{
			"entrypoint": 0, "reference": 0, "script": 0, "asset": 0, "resource": 0,
		},
		doctor: map[string]int64{
			"ready": 0, "setup_required": 0, "unsupported_platform": 0, "failed": 0,
		},
		transcripts: make(map[string]int64),
	}
}

func (a *accumulator) add(metric string, count int64) {
	switch {
	case strings.HasPrefix(metric, "resolution:"):
		status := strings.TrimPrefix(metric, "resolution:")
		a.resolutions[status] += count
	case metric == "recommended:primary":
		a.recommendedPrimary += count
	case metric == "recommended:supporting":
		a.recommendedSupporting += count
	case strings.HasPrefix(metric, "activation:"):
		attr := strings.TrimPrefix(metric, "activation:")
		a.activations[attr] += count
		a.totalActivations += count
		switch attr {
		case "override":
			a.overrides += count
		case "after_no_skill":
			a.misses += count
		case "unsolicited":
			a.unsolicited += count
		}
	case metric == "blocked:review_required":
		a.blockedByReview += count
	case metric == "setup:review_required":
		a.resolutionsReviewRequired += count
	case metric == "setup:setup_required":
		a.resolutionsSetupRequired += count
	case strings.HasPrefix(metric, "load:"):
		kind := strings.TrimPrefix(metric, "load:")
		a.loads[kind] += count
		a.totalLoads += count
	case strings.HasPrefix(metric, "doctor:"):
		status := strings.TrimPrefix(metric, "doctor:")
		a.doctor[status] += count
		a.totalDoctor += count
	case metric == "feedback:setup_failed":
		a.setupFailed += count
	case metric == "feedback:negative":
		a.negativeFeedback += count
	case metric == "feedback:negative_after_load":
		a.negativeAfterLoad += count
	case metric == "unlisted_resource_reads":
		a.unlistedResourceReads += count
	case metric == "unsupported_method_calls":
		a.unsupportedMethodCalls += count
	case metric == "snapshot_expired_requests":
		a.snapshotExpiredRequests += count
	case metric == "tools_list_bytes":
		a.toolsListBytes += count
	case strings.HasPrefix(metric, "transcript:"):
		tool := strings.TrimPrefix(metric, "transcript:")
		a.transcripts[tool] += count
	case metric == "native:no_resolve":
		a.transcripts["native_no_resolve"] += count
	case metric == "native:resolved_before":
		a.transcripts["native_resolved_before"] += count
	}
}

func normalizeFunnelWindow(q FunnelQuery) (since, until time.Time, days int, err error) {
	until = q.Until
	if until.IsZero() {
		until = time.Now().UTC()
	} else {
		until = until.UTC()
	}
	until = time.Date(until.Year(), until.Month(), until.Day(), 0, 0, 0, 0, time.UTC)

	since = q.Since
	if since.IsZero() {
		since = until.AddDate(0, 0, -DefaultFunnelDays)
	} else {
		since = since.UTC()
	}
	since = time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, time.UTC)

	if since.After(until) {
		return time.Time{}, time.Time{}, 0, NewInvalidRequestError("since must not be after until", "Pass `--since <date>` that comes before `--until <date>`.")
	}

	if until.Sub(since) > time.Duration(MaxFunnelDays)*24*time.Hour {
		since = until.AddDate(0, 0, -MaxFunnelDays)
	}
	days = int(until.Sub(since).Hours()/24) + 1
	return since, until, days, nil
}

func calcFunnelRates(acc *accumulator) (acceptance *float64, docFail *float64, setupFail *float64) {
	if acc.recommendedPrimary > 0 {
		rate := float64(acc.activations["recommended"]) / float64(acc.recommendedPrimary)
		acceptance = &rate
	}
	if acc.totalDoctor > 0 {
		fails := acc.doctor["setup_required"] + acc.doctor["unsupported_platform"] + acc.doctor["failed"]
		rate := float64(fails) / float64(acc.totalDoctor)
		docFail = &rate
	}
	if acc.totalActivations > 0 {
		rate := float64(acc.setupFailed) / float64(acc.totalActivations)
		setupFail = &rate
	}
	return
}

func buildSkillFunnel(id, name string, acc *accumulator) SkillFunnel {
	acceptRate, docFailRate, setupFailRate := calcFunnelRates(acc)
	return SkillFunnel{
		SkillID:                   id,
		Name:                      name,
		RecommendedPrimary:        acc.recommendedPrimary,
		RecommendedSupporting:     acc.recommendedSupporting,
		Activations:               acc.activations,
		TotalActivations:          acc.totalActivations,
		AcceptanceRate:            acceptRate,
		Overrides:                 acc.overrides,
		Misses:                    acc.misses,
		Unsolicited:               acc.unsolicited,
		BlockedByReview:           acc.blockedByReview,
		ResolutionsReviewRequired: acc.resolutionsReviewRequired,
		ResolutionsSetupRequired:  acc.resolutionsSetupRequired,
		Loads:                     acc.loads,
		TotalLoads:                acc.totalLoads,
		Doctor:                    acc.doctor,
		TotalDoctor:               acc.totalDoctor,
		DoctorFailureRate:         docFailRate,
		SetupFailed:               acc.setupFailed,
		SetupFailedRate:           setupFailRate,
		NegativeFeedback:          acc.negativeFeedback,
		NegativeAfterLoad:         acc.negativeAfterLoad,
		UnlistedResourceReads:     acc.unlistedResourceReads,
		SnapshotExpiredRequests:   acc.snapshotExpiredRequests,
		Transcripts:               acc.transcripts,
	}
}

func aggregateRollups(rows []telemetry.RollupRow, activeNames map[string]string) (*accumulator, map[string]*accumulator) {
	overallAcc := newAccumulator()
	skillAccs := make(map[string]*accumulator)

	for _, row := range rows {
		if row.SkillID == "" {
			overallAcc.add(row.Metric, row.Count)
		} else {
			overallAcc.add(row.Metric, row.Count)
			acc, exists := skillAccs[row.SkillID]
			if !exists {
				acc = newAccumulator()
				skillAccs[row.SkillID] = acc
			}
			acc.add(row.Metric, row.Count)
		}
	}

	for id := range activeNames {
		if _, exists := skillAccs[id]; !exists {
			skillAccs[id] = newAccumulator()
		}
	}
	return overallAcc, skillAccs
}

func buildFunnelLists(skillAccs map[string]*accumulator, activeNames map[string]string) (dead, recNeverAct, blocked, negAfterLoad, setupFail []string) {
	dead = make([]string, 0)
	recNeverAct = make([]string, 0)
	blocked = make([]string, 0)
	negAfterLoad = make([]string, 0)
	setupFail = make([]string, 0)

	type scoredSkill struct {
		id    string
		score int64
	}
	var blockedList []scoredSkill

	for id := range activeNames {
		acc := skillAccs[id]
		if acc == nil || (acc.recommendedPrimary == 0 && acc.recommendedSupporting == 0 && acc.totalLoads == 0 && acc.blockedByReview == 0) {
			dead = append(dead, id)
		}
	}
	sort.Strings(dead)

	for id, acc := range skillAccs {
		if acc.recommendedPrimary > 0 && acc.activations["recommended"] == 0 && acc.blockedByReview == 0 {
			recNeverAct = append(recNeverAct, id)
		}
		if acc.blockedByReview > 0 || acc.resolutionsReviewRequired > 0 {
			blockedList = append(blockedList, scoredSkill{id: id, score: acc.blockedByReview + acc.resolutionsReviewRequired})
		}
		if acc.negativeAfterLoad > 0 {
			negAfterLoad = append(negAfterLoad, id)
		}
		if acc.setupFailed > 0 {
			setupFail = append(setupFail, id)
		}
	}

	sort.Slice(recNeverAct, func(i, j int) bool {
		accI := skillAccs[recNeverAct[i]]
		accJ := skillAccs[recNeverAct[j]]
		if accI.recommendedPrimary != accJ.recommendedPrimary {
			return accI.recommendedPrimary > accJ.recommendedPrimary
		}
		return recNeverAct[i] < recNeverAct[j]
	})

	sort.Slice(blockedList, func(i, j int) bool {
		if blockedList[i].score != blockedList[j].score {
			return blockedList[i].score > blockedList[j].score
		}
		return blockedList[i].id < blockedList[j].id
	})
	for _, item := range blockedList {
		blocked = append(blocked, item.id)
	}

	sort.Slice(negAfterLoad, func(i, j int) bool {
		accI := skillAccs[negAfterLoad[i]]
		accJ := skillAccs[negAfterLoad[j]]
		if accI.negativeAfterLoad != accJ.negativeAfterLoad {
			return accI.negativeAfterLoad > accJ.negativeAfterLoad
		}
		return negAfterLoad[i] < negAfterLoad[j]
	})

	sort.Slice(setupFail, func(i, j int) bool {
		accI := skillAccs[setupFail[i]]
		accJ := skillAccs[setupFail[j]]
		if accI.setupFailed != accJ.setupFailed {
			return accI.setupFailed > accJ.setupFailed
		}
		return setupFail[i] < setupFail[j]
	})

	return dead, recNeverAct, blocked, negAfterLoad, setupFail
}

// Funnel produces an aggregated funnel report from daily telemetry rollups.
func (service UsageService) Funnel(ctx context.Context, path string, q FunnelQuery) (report FunnelReport, resultErr error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return FunnelReport{}, err
	}

	if q.SkillID != "" {
		if _, err := service.Skills.ReadSkill(ctx, root, q.SkillID); err != nil {
			return FunnelReport{}, err
		}
	}

	since, until, days, err := normalizeFunnelWindow(q)
	if err != nil {
		return FunnelReport{}, err
	}

	fromStr := since.Format("2006-01-02")
	toStr := until.Format("2006-01-02")

	report.Window = FunnelWindow{Since: fromStr, Until: toStr, Days: days}
	report.RawRetentionDays = FunnelRawRetentionDays
	report.RollupRetentionDays = FunnelRollupRetentionDays
	report.MetricBasis = defaultMetricBasis()

	recorder, err := service.Telemetry.Open(root)
	if err != nil {
		return FunnelReport{}, err
	}
	defer closeTelemetryRecorder(recorder, &resultErr)

	rows, err := recorder.Rollups(ctx, fromStr, toStr)
	if err != nil {
		return FunnelReport{}, err
	}

	activeList, _ := service.Skills.ListSkills(ctx, root, "active")
	activeNames := make(map[string]string, len(activeList.Skills))
	for _, s := range activeList.Skills {
		activeNames[s.ID] = s.Name
	}

	overallAcc, skillAccs := aggregateRollups(rows, activeNames)

	rawEvents, _ := recorder.RawEvents(ctx, fromStr, toStr)
	chains, rawCounts, firstValid := service.buildChains(rawEvents)

	if q.SkillID != "" {
		acc := skillAccs[q.SkillID]
		if acc == nil {
			acc = newAccumulator()
		}
		sf := buildSkillFunnel(q.SkillID, activeNames[q.SkillID], acc)
		skillMetrics := calculateChainMetrics(chains, rawCounts, firstValid, time.Now().UTC(), q.SkillID)
		sf.ChainMetrics = &skillMetrics
		report.Skill = &sf
		report.Skills = []SkillFunnel{sf}
		if q.By != "" {
			report.Cuts = calculateCuts(chains, rawCounts, firstValid, time.Now().UTC(), q.By)
		}
		return report, nil
	}

	var totalRes int64
	for _, c := range overallAcc.resolutions {
		totalRes += c
	}
	acceptRate, docFailRate, setupFailRate := calcFunnelRates(overallAcc)
	report.Overall = &OverallFunnel{
		Resolutions:               overallAcc.resolutions,
		TotalResolutions:          totalRes,
		RecommendedPrimary:        overallAcc.recommendedPrimary,
		RecommendedSupporting:     overallAcc.recommendedSupporting,
		Activations:               overallAcc.activations,
		TotalActivations:          overallAcc.totalActivations,
		AcceptanceRate:            acceptRate,
		Overrides:                 overallAcc.overrides,
		Misses:                    overallAcc.misses,
		Unsolicited:               overallAcc.unsolicited,
		BlockedByReview:           overallAcc.blockedByReview,
		ResolutionsReviewRequired: overallAcc.resolutionsReviewRequired,
		ResolutionsSetupRequired:  overallAcc.resolutionsSetupRequired,
		Loads:                     overallAcc.loads,
		TotalLoads:                overallAcc.totalLoads,
		Doctor:                    overallAcc.doctor,
		TotalDoctor:               overallAcc.totalDoctor,
		DoctorFailureRate:         docFailRate,
		SetupFailed:               overallAcc.setupFailed,
		SetupFailedRate:           setupFailRate,
		NegativeFeedback:          overallAcc.negativeFeedback,
		NegativeAfterLoad:         overallAcc.negativeAfterLoad,
		UnlistedResourceReads:     overallAcc.unlistedResourceReads,
		UnsupportedMethodCalls:    overallAcc.unsupportedMethodCalls,
		SnapshotExpiredRequests:   overallAcc.snapshotExpiredRequests,
		ToolsListBytes:            overallAcc.toolsListBytes,
		Transcripts:               overallAcc.transcripts,
	}

	skillsSlice := make([]SkillFunnel, 0, len(skillAccs))
	for id, acc := range skillAccs {
		skillsSlice = append(skillsSlice, buildSkillFunnel(id, activeNames[id], acc))
	}
	sort.Slice(skillsSlice, func(i, j int) bool {
		if skillsSlice[i].RecommendedPrimary != skillsSlice[j].RecommendedPrimary {
			return skillsSlice[i].RecommendedPrimary > skillsSlice[j].RecommendedPrimary
		}
		return skillsSlice[i].SkillID < skillsSlice[j].SkillID
	})
	overallMetrics := calculateChainMetrics(chains, rawCounts, firstValid, time.Now().UTC(), "")
	report.Overall.ChainMetrics = &overallMetrics

	for i := range skillsSlice {
		sm := calculateChainMetrics(chains, rawCounts, firstValid, time.Now().UTC(), skillsSlice[i].SkillID)
		skillsSlice[i].ChainMetrics = &sm
	}
	report.Skills = skillsSlice

	if q.By != "" {
		report.Cuts = calculateCuts(chains, rawCounts, firstValid, time.Now().UTC(), q.By)
	}

	report.DeadSkills, report.RecommendedNeverActivated, report.BlockedByReview, report.NegativeAfterLoad, report.SetupFailures = buildFunnelLists(skillAccs, activeNames)

	return report, nil
}
